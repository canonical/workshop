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

type Status int

const (
	UnknownStatus Status = iota
	ReadyStatus
	PendingStatus
	WaitingStatus
	ErrorStatus
	StoppedStatus
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

type HealthCheck struct {
	Sdk         string            `json:"sdk"`
	Timestamp   time.Time         `json:"timestamp,omitzero"`
	CheckResult HealthCheckResult `json:"check-result"`
	Message     string            `json:"message,omitempty"`
	Code        string            `json:"code,omitempty"`
}

type HealthState struct {
	Timestamp time.Time              `json:"timestamp,omitzero"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Code      string                 `json:"code,omitempty"`
	SdkHealth map[string]HealthCheck `json:"sdk-health,omitempty"`
}

// Infers the state of a workshop based on the container's state and any of the
// operations in progress for the workshop. The state must be locked.
func WorkshopHealth(st *state.State, ws *workshop.Workshop) HealthState {
	healthState := HealthState{
		Timestamp: time.Now(),
	}

	// Check the project directory exists.
	if !ws.Project.Exists() {
		st.Warnf("cannot find project directory %q for workshop %q", ws.Project.Path, ws.Name)

		healthState.Status = ErrorStatus
		healthState.Code = "missing-project"
		return healthState
	}

	if err := conflict.CheckChangeConflict(st, ws.Project.ProjectId, ws.Name, []string{"exec"}); err != nil {
		conflict, ok := err.(*conflict.ChangeConflictError)
		if !ok || conflict.ChangeID == "" {
			healthState.Status = ErrorStatus
			return healthState
		}

		change := st.Change(conflict.ChangeID)
		if change.Status() == state.WaitStatus {
			healthState.Code = "wait-on-error"
			healthState.Status = WaitingStatus
		} else {
			healthState.Status = PendingStatus
		}

		healthState.SdkHealth = sdksHealthCheckSummary(change)
	} else {
		if ws.Running {
			healthState.Status = ReadyStatus
		} else {
			healthState.Status = StoppedStatus
		}
	}
	return healthState
}

// Examine the tasks of the change to fetch possible check-health hook results
// for the workshop's SDKs.
func sdksHealthCheckSummary(chg *state.Change) map[string]HealthCheck {
	sdkChecks := map[string]HealthCheck{}
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

// Checks the provided workshop has one of the allowed health statuses.
func CheckWorkshopHealth(st *state.State, ws *workshop.Workshop, allowedStatuses []Status) error {
	health := WorkshopHealth(st, ws)

	if !slices.Contains(allowedStatuses, health.Status) {
		switch health.Status {
		case ReadyStatus:
			return errors.New("workshop already running")
		case PendingStatus:
			return errors.New("other changes in progress")
		case WaitingStatus:
			return errors.New("waiting on error")
		case ErrorStatus:
			return errors.New("workshop unhealthy")
		case StoppedStatus:
			return errors.New("workshop not running")
		default:
			return errors.New("workshop health unknown")
		}
	}
	return nil
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
			Timestamp:   time.Now(),
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

var (
	retryTimeout   = 1 * time.Second
	retriesAllowed = 10
)

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
		// attemps, refresh will wait on this error allowing to repeat the task
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
