/*
 * Copyright (C) 2019 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package healthstate_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

func TestHealthState(t *testing.T) { check.TestingT(t) }

type healthSuite struct {
	testutil.BaseTest
	se      *overlord.StateEngine
	state   *state.State
	runner  *state.TaskRunner
	backend *fakebackend.FakeWorkshopBackend
	hookMgr *hookstate.HookManager
	project workshop.Project
	ctx     context.Context
}

var _ = check.Suite(&healthSuite{})

func (s *healthSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	var err error

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)

	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")
	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)

	s.hookMgr = hookstate.New(s.state, s.runner)

	s.se = overlord.NewStateEngine(s.state)
	s.se.AddManager(s.hookMgr)
	s.se.AddManager(s.runner)

	s.se.AddManager(s.hookMgr)
	s.se.AddManager(s.runner)

	healthstate.Init(s.hookMgr)

	c.Assert(s.se.StartUp(), check.IsNil)
}

func (s *healthSuite) TearDownTest(c *check.C) {
	s.se.Stop()
	s.BaseTest.TearDownTest(c)
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, task := range tasks {
		task.Set("workshop", w)
		task.Set("project", p)
	}
}

func ensureTaskHealthIsSet(t *state.Task, expected *healthstate.HealthCheck, c *check.C) {
	var health healthstate.HealthCheck
	err := t.Get("health", &health)
	c.Assert(err, check.IsNil)
	c.Assert(expected, check.DeepEquals, &health)
}

var (
	oneSdk = []workshop.SdkRecord{{Name: "one", Channel: "latest/stable"}}

	oneSdkHealthCheck = map[string]map[string]string{
		"one": map[string]string{hookstate.CheckHealth.String(): ""},
	}
)

func (s *healthSuite) launchWorkshopWithSDKs(c *check.C, sdks []workshop.SdkRecord, hooks map[string]map[string]string) *workshop.Workshop {
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: sdks}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)
	ws, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer ws.Close()

	for _, sk := range sdks {
		err = ws.MkdirAll(sdk.SdkHooksDir(sk.Name), 0744)
		c.Check(err, check.IsNil)

		for name, hook := range hooks[sk.Name] {
			c.Assert(ws.WriteFile(sdk.SdkHookPath(sk.Name, name), []byte(hook), 0644), check.IsNil)
		}
	}

	workshop, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	workshop.Running = true
	return workshop
}

func (*healthSuite) TestStatusHappy(c *check.C) {
	for i, str := range healthstate.KnownStatuses {
		status, err := healthstate.SetHealthLookup(str)
		c.Check(err, check.IsNil, check.Commentf("%v", str))
		c.Check(status, check.Equals, healthstate.HealthCheckResult(i), check.Commentf("%v", str))
	}
}

func (*healthSuite) TestStatusUnhappy(c *check.C) {
	status, err := healthstate.SetHealthLookup("rabbits")
	c.Check(status, check.Equals, healthstate.HealthCheckResult(-1))
	c.Check(err, check.ErrorMatches, `invalid status "rabbits".*`)
	c.Check(status.String(), check.Equals, "invalid (-1)")
}

// HasChangeRef reports true when a change is referenced by the cause.
func (*healthSuite) TestHasChangeRefPresent(c *check.C) {
	cause := healthstate.HealthStateCause{ChangeRef: &healthstate.ChangeRef{}}

	c.Check(cause.HasChangeRef(), check.Equals, true)
}

// HasChangeRef reports false when no change is referenced by the cause.
func (*healthSuite) TestHasChangeRefAbsent(c *check.C) {
	cause := healthstate.HealthStateCause{}

	c.Check(cause.HasChangeRef(), check.Equals, false)
}

// HasStatusIn reports true when the health status is among those provided.
func (*healthSuite) TestHasStatusInMatch(c *check.C) {
	health := healthstate.HealthState{Status: healthstate.WaitingStatus}

	c.Check(health.HasStatusIn(healthstate.WaitingStatus), check.Equals, true)
	c.Check(health.HasStatusIn(
		healthstate.ReadyStatus, healthstate.WaitingStatus,
	), check.Equals, true)
}

// HasStatusIn reports false when the health status is not among those provided.
func (*healthSuite) TestHasStatusInNoMatch(c *check.C) {
	health := healthstate.HealthState{Status: healthstate.WaitingStatus}

	c.Check(health.HasStatusIn(healthstate.ReadyStatus), check.Equals, false)
}

// HasStatusIn reports false when no statuses are provided.
func (*healthSuite) TestHasStatusInEmpty(c *check.C) {
	health := healthstate.HealthState{Status: healthstate.WaitingStatus}

	c.Check(health.HasStatusIn(), check.Equals, false)
}

// ChangeInProgressError describes the workshop and the kind of change that is
// blocking the operation.
func (*healthSuite) TestChangeInProgressErrorMessage(c *check.C) {
	err := healthstate.ChangeInProgressError{
		Workshop:   "ws",
		ChangeKind: "refresh",
	}

	c.Check(err.Error(), check.Equals,
		`workshop "ws" has "refresh" change in progress`)
}

func (s *healthSuite) TestWorkshopHealthReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.ReadyStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *healthSuite) TestWorkshopHealthStopped(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	c.Assert(s.backend.StopWorkshop(s.ctx, "ws", true), check.IsNil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.StoppedStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *healthSuite) TestWorkshopHealthMissingProject(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	c.Assert(os.RemoveAll(s.project.Path), check.IsNil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.ErrorStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.Equals, "missing-project")

	warnings := s.state.AllWarnings()
	c.Check(warnings, check.HasLen, 1)
	warning := fmt.Sprintf(`cannot find project directory %q for workshop "ws"`, s.project.Path)
	c.Check(warnings[0].String(), check.Equals, warning)
}

func (s *healthSuite) TestWorkshopHealthOperationInProgress(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("launch", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop-file", "name: ws\nbase: ubuntu@20.04\n")
	setWorkshopProject("ws", s.project, task)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.PendingStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
	c.Check(health.Cause.ChangeRef, check.DeepEquals, &healthstate.ChangeRef{
		ID:     chg.ID(),
		Kind:   "launch",
		Status: chg.Status().String(),
	})
}

func (s *healthSuite) TestWorkshopHealthOperationWaitingWithNotes(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("refresh", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.SetStatus(state.WaitStatus)
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop-file", "name: ws\nbase: ubuntu@20.04\n")
	setWorkshopProject("ws", s.project, task)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.WaitingStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.Equals, "wait-on-error")
	c.Check(health.Cause.ChangeRef, check.DeepEquals, &healthstate.ChangeRef{
		ID:     chg.ID(),
		Kind:   "refresh",
		Status: state.WaitStatus.String(),
	})
}

func (s *healthSuite) TestWorkshopHealthSdkHealth(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("launch", "test")
	chg.Set("project-id", s.project.ProjectId)
	task := s.state.NewTask("run-hook", "test task")
	healthCheck := healthstate.HealthCheck{
		Sdk:         "one",
		CheckResult: healthstate.CheckWaiting,
		Message:     "still waiting",
		Code:        "how-much-longer",
	}
	task.Set("health", healthCheck)
	setWorkshopProject("ws", s.project, task)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, oneSdk, nil)
	health := healthstate.WorkshopHealth(s.state, workshop)

	c.Assert(health.Status, check.Equals, healthstate.PendingStatus)
	c.Assert(health.SdkHealth, check.HasLen, 1)
	c.Assert(health.SdkHealth["one"], check.DeepEquals, healthCheck)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *healthSuite) TestCheckStatusReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	workshop := s.launchWorkshopWithSDKs(c, nil, nil)

	// Ready status should not return an error
	err := healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ReadyStatus})
	c.Assert(err, check.IsNil)

	// All other status' should return an error
	err = healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ErrorStatus, healthstate.PendingStatus, healthstate.WaitingStatus, healthstate.StoppedStatus, healthstate.UnknownStatus})
	c.Assert(err, check.ErrorMatches, "already running")
}

func (s *healthSuite) TestCheckStatusPending(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("refresh", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.SetStatus(state.DoingStatus)
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop-file", "name: ws\nbase: ubuntu@20.04\n")
	setWorkshopProject("ws", s.project, task)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)

	// Pending status should not return an error
	err := healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.PendingStatus})
	c.Assert(err, check.IsNil)

	// All other status' should return a structured change-in-progress error
	// identifying the blocking change.
	err = healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ErrorStatus, healthstate.ReadyStatus, healthstate.WaitingStatus, healthstate.StoppedStatus, healthstate.UnknownStatus})
	var conflictErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, healthstate.ChangeInProgressError{
		ChangeID:   chg.ID(),
		ChangeKind: "refresh",
		ProjectID:  s.project.ProjectId,
		Workshop:   "ws",
	})
}

func (s *healthSuite) TestCheckStatusWaiting(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("refresh", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.SetStatus(state.WaitStatus)
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop-file", "name: ws\nbase: ubuntu@20.04\n")
	setWorkshopProject("ws", s.project, task)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, nil, nil)

	// Waiting status should not return an error
	err := healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.WaitingStatus})
	c.Assert(err, check.IsNil)

	// All other status' should return a structured change-in-progress error
	// identifying the blocking change.
	err = healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ErrorStatus, healthstate.ReadyStatus, healthstate.PendingStatus, healthstate.StoppedStatus, healthstate.UnknownStatus})
	var conflictErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, healthstate.ChangeInProgressError{
		ChangeID:   chg.ID(),
		ChangeKind: "refresh",
		ProjectID:  s.project.ProjectId,
		Workshop:   "ws",
	})
}

func (s *healthSuite) TestCheckStatusError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	c.Assert(os.RemoveAll(s.project.Path), check.IsNil)

	// Error status should not return an error
	err := healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ErrorStatus})
	c.Assert(err, check.IsNil)

	// All other status' should return an error
	err = healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ReadyStatus, healthstate.PendingStatus, healthstate.WaitingStatus, healthstate.StoppedStatus, healthstate.UnknownStatus})
	c.Assert(err, check.ErrorMatches, "unhealthy in error")
}

func (s *healthSuite) TestCheckStatusStopped(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	workshop := s.launchWorkshopWithSDKs(c, nil, nil)
	c.Assert(s.backend.StopWorkshop(s.ctx, "ws", true), check.IsNil)

	// Stopped status should not return an error
	err := healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.StoppedStatus})
	c.Assert(err, check.IsNil)

	// All other status' should return an error
	err = healthstate.CheckWorkshopHealth(s.state, workshop, []healthstate.Status{healthstate.ReadyStatus, healthstate.PendingStatus, healthstate.WaitingStatus, healthstate.ErrorStatus, healthstate.UnknownStatus})
	c.Assert(err, check.ErrorMatches, "not running")
}

func (s *healthSuite) TestExecCheckHealthNotProvided(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshopWithSDKs(c, oneSdk, nil)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(s.backend.ExecCalls, check.HasLen, 0)
	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
}

func (s *healthSuite) TestExecCheckHealthSetHealthNotCalled(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshopWithSDKs(c, oneSdk, oneSdkHealthCheck)
	restore := healthstate.FakeRetryTimeout(0 * time.Second)
	defer restore()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(t1.Log()[0], check.Matches, `.*SDK "one" health status is unknown`)
}

func (s *healthSuite) TestExecCheckHealthSetHealthError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	now := time.Now().UTC()
	result := healthstate.HealthCheck{
		Timestamp:   now,
		Sdk:         "one",
		CheckResult: healthstate.CheckError,
		Message:     "something went wrong",
		Code:        "error-error",
	}

	var hookContext *hookstate.Context
	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				// emulate workshopctl set-health --code=<code> error <message>
				var err error
				hookContext, err = s.hookMgr.Context(args.Environment["WORKSHOP_COOKIE"])
				c.Assert(err, check.IsNil)
				hookContext.Lock()
				hookContext.Set("health", result)
				hookContext.Unlock()
				return nil
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.launchWorkshopWithSDKs(c, oneSdk, oneSdkHealthCheck)
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(t1.Log()[0], check.Matches, `.*something went wrong`)
	s.state.Unlock()
	hookContext.Lock()
	var counter int
	err := hookContext.Get("retry-counter", &counter)
	hookContext.Unlock()
	s.state.Lock()
	c.Assert(err, check.IsNil)
	c.Assert(counter, check.Equals, 0)

	ensureTaskHealthIsSet(t1, &result, c)
}

func (s *healthSuite) TestExecCheckHealthSetHealthWaiting(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	restore := healthstate.FakeRetryTimeout(0 * time.Second)
	defer restore()

	now := time.Now().UTC()
	resultWait := healthstate.HealthCheck{
		Timestamp:   now,
		Sdk:         "one",
		CheckResult: healthstate.CheckWaiting,
		Message:     "not ready yet",
		Code:        "wait-for-me",
	}

	nowOkay := time.Now().UTC()
	resultOkay := healthstate.HealthCheck{
		Timestamp:   nowOkay,
		Sdk:         "one",
		CheckResult: healthstate.CheckOkay,
	}

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				hookCtx, err := s.hookMgr.Context(args.Environment["WORKSHOP_COOKIE"])
				c.Assert(err, check.IsNil)
				hookCtx.Lock()
				var counter int
				hookCtx.Get("retry-counter", &counter)
				if counter == 0 {
					// emulate workshopctl set-health --code=<code> error <message>
					hookCtx.Set("health", resultWait)
				} else {
					// emulate workshopctl set-health okay
					hookCtx.Set("health", resultOkay)
				}
				hookCtx.Unlock()
				return nil
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.launchWorkshopWithSDKs(c, oneSdk, oneSdkHealthCheck)
	s.state.Unlock()
	for i := 0; i < 2; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	c.Assert(t1.Log(), check.HasLen, 0)
	ensureTaskHealthIsSet(t1, &resultOkay, c)
}

func (s *healthSuite) TestExecCheckHealthSetHealthExceededAttempts(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	restore := healthstate.FakeRetryTimeout(0 * time.Second)
	defer restore()

	restoreAttempts := healthstate.FakeRetryAttempts(2)
	defer restoreAttempts()

	now := time.Now().UTC()
	resultWait := healthstate.HealthCheck{
		Timestamp:   now,
		Sdk:         "one",
		CheckResult: healthstate.CheckWaiting,
		Message:     "not ready yet",
		Code:        "wait-for-me",
	}

	var hookContext *hookstate.Context
	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				var err error
				hookContext, err = s.hookMgr.Context(args.Environment["WORKSHOP_COOKIE"])
				c.Assert(err, check.IsNil)
				hookContext.Lock()
				// emulate workshopctl set-health --code=<code> error <message>
				hookContext.Set("health", resultWait)
				hookContext.Unlock()
				return nil
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.launchWorkshopWithSDKs(c, oneSdk, oneSdkHealthCheck)
	s.state.Unlock()
	for i := 0; i < 3; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(t1.Log()[0], check.Matches, `.*SDK \"one\" is not healthy after multiple checks`)
	c.Assert(t1.Log(), check.HasLen, 1)

	s.state.Unlock()
	hookContext.Lock()
	var counter int
	err := hookContext.Get("retry-counter", &counter)
	hookContext.Unlock()
	s.state.Lock()
	c.Assert(err, check.IsNil)
	c.Assert(counter, check.Equals, 0)
}

func (s *healthSuite) TestExecCheckHealthTimeout(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.CheckHealth)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return context.DeadlineExceeded
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.launchWorkshopWithSDKs(c, oneSdk, oneSdkHealthCheck)
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(t1.Log()[0], check.Matches, `.*SDK "one" health status check timed out`)
}
