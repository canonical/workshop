// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package healthstate

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/canonical/x-go/strutil"

	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// ChangeInProgressError reports that an operation on a workshop cannot
// proceed because a conflicting change is already in progress for it.
type ChangeInProgressError struct {
	// ChangeID is the identifier of the change that blocks the operation.
	ChangeID string

	// ChangeKind is the kind of the conflicting change, such as "refresh".
	ChangeKind string

	// ProjectID identifies the project the targeted workshop belongs to.
	ProjectID string

	// Workshop is the name of the workshop the operation targeted.
	Workshop string
}

type Status int

const (
	UnknownStatus Status = iota
	ReadyStatus
	PendingStatus
	WaitingStatus
	ErrorStatus
	StoppedStatus
)

var (
	// ErrorWorkshopHealthError reports that the workshop is unhealthy because
	// a change has stopped in error.
	ErrorWorkshopHealthError = errors.New("unhealthy in error")

	// ErrorWorkshopHealthPending reports that another change is already in
	// progress for the workshop.
	ErrorWorkshopHealthPending = errors.New("other change in progress")

	// ErrorWorkshopHealthReady reports that the workshop is already running.
	ErrorWorkshopHealthReady = errors.New("already running")

	// ErrorWorkshopHealthStopped reports that the workshop is not running.
	ErrorWorkshopHealthStopped = errors.New("not running")

	// ErrorWorkshopHealthUnknown reports that the workshop health could not be
	// determined.
	ErrorWorkshopHealthUnknown = errors.New("health unknown")

	// ErrorWorkshopHealthWaiting reports that a change is paused waiting on an
	// error in the workshop.
	ErrorWorkshopHealthWaiting = errors.New("waiting on error")
)

var knownStatuses = []string{"Unknown", "Ready", "Pending", "Waiting", "Error", "Stopped"}

func StatusLookup(str string) (Status, error) {
	switch str {
	case "unknown":
		return UnknownStatus, nil
	case "ready":
		return ReadyStatus, nil
	case "pending":
		return PendingStatus, nil
	case "waiting":
		return WaitingStatus, nil
	case "error":
		return ErrorStatus, nil
	case "stopped":
		return StoppedStatus, nil
	}

	return -1, fmt.Errorf("invalid status %q, must be one of %s",
		str, strutil.Quoted(knownStatuses))
}

func (s Status) String() string {
	if s < 0 || int(s) >= len(knownStatuses) {
		return fmt.Sprintf("invalid (%d)", s)
	}
	return knownStatuses[s]
}

type HealthCheckResult int

const (
	CheckUnknown HealthCheckResult = iota
	CheckWaiting
	CheckOkay
	CheckError
)

var knownSetHealthStatuses = []string{"unknown", "waiting", "okay", "error"}

func SetHealthLookup(str string) (HealthCheckResult, error) {
	switch str {
	case "okay":
		return CheckOkay, nil
	case "waiting":
		return CheckWaiting, nil
	case "error":
		return CheckError, nil
	case "unknown":
		return CheckUnknown, nil
	}

	return -1, fmt.Errorf("invalid status %q, must be one of %s", str, strutil.Quoted(knownSetHealthStatuses))
}

func (s HealthCheckResult) String() string {
	if s < 0 || int(s) >= len(knownSetHealthStatuses) {
		return fmt.Sprintf("invalid (%d)", s)
	}
	return knownSetHealthStatuses[s]
}

// ChangeRef identifies a [state.Change] and its progress at a point in time.
type ChangeRef struct {
	// ID is the unique identifier of the change.
	ID string

	// Kind is the kind of the change, such as "refresh" or "launch".
	Kind string

	// Status is the status of the change when the reference was made, such
	// as "Wait" or "Doing".
	Status string
}

type HealthCheck struct {
	Sdk         string            `json:"sdk"`
	Timestamp   time.Time         `json:"timestamp,omitzero"`
	CheckResult HealthCheckResult `json:"check-result"`
	Message     string            `json:"message,omitempty"`
	Code        string            `json:"code,omitempty"`
}

type HealthState struct {
	// Cause carries the supporting detail behind Status, identifying what
	// is responsible for the current value.
	Cause HealthStateCause

	Timestamp time.Time              `json:"timestamp,omitzero"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Code      string                 `json:"code,omitempty"`
	SdkHealth map[string]HealthCheck `json:"sdk-health,omitempty"`
}

// HealthStateCause describes what is responsible for the status reported in a
// [HealthState], providing the supporting detail behind the status value.
type HealthStateCause struct {
	// ChangeRef identifies the change driving the current status, such as a
	// refresh waiting on error. It is nil when no change is involved, e.g.
	// when the workshop is simply running or stopped.
	ChangeRef *ChangeRef
}

// HasChangeRef reports whether a change is referenced by the cause.
func (c HealthStateCause) HasChangeRef() bool {
	return c.ChangeRef != nil
}

// HasStatusIn reports whether the health status matches any of the provided
// statuses.
func (h HealthState) HasStatusIn(statuses ...Status) bool {
	return slices.Contains(statuses, h.Status)
}

// Infers the state of a workshop based on the container's state and any of the
// operations in progress for the workshop. The state must be locked.
func WorkshopHealth(st *state.State, ws *workshop.Workshop) HealthState {
	var healthState = HealthState{
		Timestamp: time.Now().UTC(),
	}

	// Check the project directory exists.
	if !ws.Project.Exists() {
		st.Warnf("cannot find project directory %q for workshop %q", ws.Project.Path, ws.Name)

		healthState.Status = ErrorStatus
		healthState.Code = "missing-project"
		return healthState
	}

	err := conflict.CheckChangeConflict(
		st, ws.Project.ProjectId, ws.Name, []string{"exec", "get-secret"})
	var changeConflictError *conflict.ChangeConflictError
	switch {
	case errors.As(err, &changeConflictError):
		if changeConflictError.ChangeStatus == state.WaitStatus.String() {
			healthState.Code = "wait-on-error"
			healthState.Status = WaitingStatus
		} else {
			healthState.Status = PendingStatus
		}

		// Set the change information that is contributing to the current health
		// status.
		healthState.Cause.ChangeRef = &ChangeRef{
			ID:     changeConflictError.ChangeID,
			Kind:   changeConflictError.ChangeKind,
			Status: changeConflictError.ChangeStatus,
		}

		healthState.SdkHealth = sdksHealthCheckSummary(
			st, changeConflictError.ChangeID)
	case err != nil:
		healthState.Status = ErrorStatus
	case ws.Running:
		healthState.Status = ReadyStatus
	default:
		healthState.Status = StoppedStatus
	}

	return healthState
}

// Examine the tasks of the change to fetch possible check-health hook results
// for the workshop's SDKs.
func sdksHealthCheckSummary(st *state.State, changeID string) map[string]HealthCheck {
	chg := st.Change(changeID)

	var sdkChecks = map[string]HealthCheck{}
	for _, task := range chg.Tasks() {
		if task.Kind() == "run-hook" {
			var healthCheck HealthCheck
			if err := task.Get("health", &healthCheck); err == nil {
				sdkChecks[healthCheck.Sdk] = healthCheck
			}
		}
	}
	return sdkChecks
}

// Error implements the error interface, describing the change responsible for
// the conflict.
func (e ChangeInProgressError) Error() string {
	return fmt.Sprintf(
		"workshop %q has %q change in progress", e.Workshop, e.ChangeKind)
}

// CheckWorkshopHealth returns an error when the workshop's health is not one of
// the allowed statuses. A workshop blocked by a pending change, or a change
// waiting on error, yields a [ChangeInProgressError]; any other disallowed
// status yields the matching ErrorWorkshopHealth sentinel.
func CheckWorkshopHealth(
	st *state.State,
	ws *workshop.Workshop,
	allowedStatuses []Status,
) error {
	health := WorkshopHealth(st, ws)
	if health.HasStatusIn(allowedStatuses...) {
		return nil
	}

	if health.HasStatusIn(WaitingStatus, PendingStatus) &&
		health.Cause.HasChangeRef() {
		return ChangeInProgressError{
			ChangeID:   health.Cause.ChangeRef.ID,
			ChangeKind: health.Cause.ChangeRef.Kind,
			ProjectID:  ws.Project.ProjectId,
			Workshop:   ws.Name,
		}
	}

	switch health.Status {
	case ReadyStatus:
		return ErrorWorkshopHealthReady
	case PendingStatus:
		return ErrorWorkshopHealthPending
	case WaitingStatus:
		return ErrorWorkshopHealthWaiting
	case ErrorStatus:
		return ErrorWorkshopHealthError
	case StoppedStatus:
		return ErrorWorkshopHealthStopped
	default:
		return ErrorWorkshopHealthUnknown
	}
}

func Init(hookManager *hookstate.HookManager) {
	hookManager.Register(regexp.MustCompile("^check-health$"), newHealthHandler)
}

func newHealthHandler(ctx *hookstate.Context) hookstate.Handler {
	return &healthHandler{context: ctx}
}

type healthHandler struct {
	context *hookstate.Context
}

// Before is called just before the hook runs -- nothing to do beyond setting a marker
func (h *healthHandler) Before() error {
	h.context.Lock()
	defer h.context.Unlock()
	var health HealthCheck
	err := h.context.Get("health", &health)
	// the handler is being called for the first time
	// (the health is set already if it is a retry)
	if errors.Is(err, state.ErrNoState) {
		h.context.Set("health", HealthCheck{
			Timestamp:   time.Now().UTC(),
			Sdk:         h.context.Sdk(),
			CheckResult: CheckUnknown,
		})
	}
	var counter int
	err = h.context.Get("retry-counter", &counter)
	if errors.Is(err, state.ErrNoState) {
		h.context.Set("retry-counter", 0)
	}
	return nil
}

var retryTimeout = 1 * time.Second
var retriesAllowed = 10

func (h *healthHandler) Done() (err error) {
	var health HealthCheck
	var retryCounter int

	h.context.Lock()
	err = h.context.Get("health", &health)
	h.context.Unlock()

	if err != nil {
		// note it can't actually be state.ErrNoState because Before sets it
		return err
	}

	h.context.Lock()
	err = h.context.Get("retry-counter", &retryCounter)
	h.context.Unlock()

	if err != nil {
		// note it can't actually be state.ErrNoState because Before sets it
		return err
	}

	defer func() {
		if !h.context.IsEphemeral() {
			task, _ := h.context.Task()
			h.context.Lock()
			// update the hook's task to contain the latest health check result
			// this is used by external parties to read and report the health
			// check statuses e.g. workshop info command.
			task.Set("health", health)
			if _, ok := err.(*state.Retry); ok {
				h.context.Set("retry-counter", retryCounter+1)
			}
			h.context.Unlock()
		}
	}()

	if health.CheckResult == CheckUnknown {
		return fmt.Errorf("SDK %q health status is unknown", h.context.Sdk())
	}

	if retryCounter >= retriesAllowed && health.CheckResult == CheckWaiting {
		// if reached the maximum possible retries, reset the counter. This is
		// required for scenarios when a user provided --wait-on-error to
		// workshop refresh. If provided and check-health failed after multiple
		// attempts, refresh will wait on this error allowing to repeat the task
		// with --continue. In this case, we must to re-run the health check
		// from scratch.
		h.context.Lock()
		h.context.Set("retry-counter", 0)
		h.context.Unlock()
		return fmt.Errorf("SDK %q is not healthy after multiple checks", h.context.Sdk())
	}

	if health.CheckResult == CheckWaiting {
		return &state.Retry{After: retryTimeout, Reason: health.Message}
	}

	if health.CheckResult == CheckError {
		// see the comment about the maximum possible retries above
		h.context.Lock()
		h.context.Set("retry-counter", 0)
		h.context.Unlock()
		return errors.New(health.Message)
	}

	// the health check was reported as 'okay', we consider the check-health hook successful.
	return nil
}

func (h *healthHandler) Error(err error) (bool, error) {
	if errors.Is(err, context.DeadlineExceeded) {
		return false, fmt.Errorf("SDK %q health status check timed out", h.context.Sdk())
	}

	return false, nil
}
