package hookstate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/hooktest"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type hookSuite struct {
	backend     *fakebackend.FakeWorkshopBackend
	state       *state.State
	runner      *state.TaskRunner
	se          *overlord.StateEngine
	hookmgr     *hookstate.HookManager
	ctx         context.Context
	project     workshop.Project
	mockHandler *hooktest.MockHandler
}

var _ = check.Suite(&hookSuite{})

func TestHookSuite(t *testing.T) { check.TestingT(t) }

func fakeHandler(task *state.Task, _ *tomb.Tomb) error {
	return nil
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, i := range tasks {
		i.Set("workshop", w)
		i.Set("project", p)
	}
}

func (s *hookSuite) SetUpTest(c *check.C) {
	var err error
	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)

	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")
	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)
	workshop.ReplaceBackend(s.state, s.backend)

	// empty task handler
	s.runner.AddHandler("fake-task", fakeHandler, nil)
	s.mockHandler = hooktest.NewMockHandler()
	s.hookmgr = hookstate.New(s.state, s.runner)
	s.hookmgr.Register(regexp.MustCompile("^fake-hook$"), func(context *hookstate.Context) hookstate.Handler {
		return s.mockHandler
	})

	// error-provoking task handler
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return errors.New("error out")
	}
	s.runner.AddHandler("error-trigger", erroringHandler, nil)

	s.se = overlord.NewStateEngine(s.state)
	s.se.AddManager(s.hookmgr)
	s.se.AddManager(s.runner)
	err = s.se.StartUp()
	c.Check(err, check.IsNil)
}

func (s *hookSuite) TestExecHookDoesNotExist(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	t1 := hookstate.Hook(s.state, "ws", "new", hookstate.SetupBase)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	// Launch a workshop provinding no hooks
	err := s.backend.LaunchWorkshop(s.ctx, &workshop.File{Name: "ws", Base: "ubuntu@20.04"})
	c.Check(err, check.IsNil)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(s.backend.ExecCalls, check.HasLen, 0)
	c.Check(t1.Status(), check.Equals, state.DoneStatus)
}

func (s *hookSuite) TestExecSaveState(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.SaveState)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")

	volume := workshop.WorkshopStateVolumeName("ws", s.project.ProjectId)
	err := s.backend.CreateVolume(s.ctx, volume)
	c.Assert(err, check.IsNil)
	defer func() {
		_ = s.backend.DeleteVolume(s.ctx, volume)
	}()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()
	c.Assert(chg.Err(), check.IsNil)
	c.Assert(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, testutil.DeepUnsortedMatches,
		[]string{"bash", "-ue", "-o", "pipefail", "/var/lib/workshop/sdk/one/current/sdk/hooks/save-state"})

	// Ensure that the save-state handler has created the required state
	// directory (reattach the volume to the workshop to check).
	ws, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer ws.Close()
	err = s.backend.AttachVolume(s.ctx, "ws", volume, dirs.WorkshopStateDir)
	c.Check(err, check.IsNil)
	info, err := ws.Stat("/var/lib/workshop/state/sdk/one")
	c.Check(err, check.IsNil)
	c.Assert(info.IsDir(), check.Equals, true)
	c.Assert(t1.Log(), check.HasLen, 0)
	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, testutil.DeepUnsortedMatches,
		[]string{"bash", "-ue", "-o", "pipefail", "/var/lib/workshop/sdk/one/current/sdk/hooks/save-state"})
	c.Assert(s.backend.ExecCalls[0].Args.Environment["SDK_STATE_DIR"], check.Equals, "/var/lib/workshop/state/sdk/one")
	c.Assert(s.backend.ExecCalls[0].Args.Environment["WORKSHOP_COOKIE"], check.NotNil)
	c.Assert(s.backend.ExecCalls[0].Args.Environment, check.HasLen, 2)
}

func (s *hookSuite) TestExecRestoreState(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.RestoreState)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")

	volume := workshop.WorkshopStateVolumeName("ws", s.project.ProjectId)
	err := s.backend.CreateVolume(s.ctx, volume)
	c.Assert(err, check.IsNil)
	defer func() {
		_ = s.backend.DeleteVolume(s.ctx, volume)
	}()
	// Setup state storage (must be already set by the save-state in a real use
	// case).
	vfs := s.backend.WorkshopVolumeContents[volume]
	c.Check(err, check.IsNil)
	err = os.MkdirAll(filepath.Join(vfs, "sdk", "one"), 0755)
	c.Check(err, check.IsNil)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, testutil.DeepUnsortedMatches,
		[]string{"bash", "-ue", "-o", "pipefail", "/var/lib/workshop/sdk/one/current/sdk/hooks/restore-state"})
	c.Assert(s.backend.ExecCalls[0].Args.Environment["SDK_STATE_DIR"], check.Equals, "/var/lib/workshop/state/sdk/one")
	c.Assert(s.backend.ExecCalls[0].Args.Environment["WORKSHOP_COOKIE"], check.NotNil)
	c.Assert(s.backend.ExecCalls[0].Args.Environment, check.HasLen, 2)
}

func (s *hookSuite) TestExecHandlesFailedHook(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.SaveState)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")

	volume := workshop.WorkshopStateVolumeName("ws", s.project.ProjectId)
	err := s.backend.CreateVolume(s.ctx, volume)
	c.Assert(err, check.IsNil)
	defer func() {
		_ = s.backend.DeleteVolume(s.ctx, volume)
	}()

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return errors.New("hook execution error")
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, testutil.DeepUnsortedMatches,
		[]string{"bash", "-ue", "-o", "pipefail", "/var/lib/workshop/sdk/one/current/sdk/hooks/save-state"})

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*hook execution error$")
}

func (s *hookSuite) TestExecHandlesHookTimedout(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.HookWithTimeout(s.state, "ws", "one", hookstate.FakeHook, 100*time.Millisecond)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				child, cancel := context.WithTimeout(ctx, args.Timeout)
				defer cancel()
				time.Sleep(200 * time.Millisecond)
				return child.Err()
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, testutil.DeepUnsortedMatches,
		[]string{"bash", "-ue", "-o", "pipefail", "/var/lib/workshop/sdk/one/current/sdk/hooks/fake-hook"})

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*context deadline exceeded$")
}

func (s *hookSuite) TestExecEnsureContextHandlerHappyPath(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.DoneStatus)
	c.Check(t1.Log(), check.HasLen, 0)

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, true)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, false)
}

func (s *hookSuite) TestExecEnsureContextHandlerUnhappyPath(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return errors.New("hook execution error")
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.launchWorkshop(c, "one")
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*hook execution error$")

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, true)
}

func (s *hookSuite) TestExecEnsureContextHandlerErrorFails(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)
	// The context handler will return an error that must be the final error of
	// the task.
	s.mockHandler.ErrorError = true

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return errors.New("hook execution error")
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*Error failed at user request$")

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, true)
}

func (s *hookSuite) TestExecEnsureContextHandlerIgnoresError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)
	s.mockHandler.IgnoreOriginalErr = true

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return errors.New("hook execution error")
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Log(), check.HasLen, 0)
	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, true)
}

func (s *hookSuite) TestHookTaskHandlerBeforeError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)
	s.mockHandler.BeforeError = true

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*Before failed at user request$")

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, false)

	c.Assert(s.backend.ExecCalls, check.HasLen, 0)
}

func (s *hookSuite) TestHookTaskHandlerDoneError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)
	s.mockHandler.DoneError = true

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*Done failed at user request$")

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, true)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, false)
}

func (s *hookSuite) TestHookWithMultipleHandlersIsError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "ws", "one", hookstate.FakeHook)
	s.hookmgr.Register(regexp.MustCompile("^fake-*"), func(context *hookstate.Context) hookstate.Handler {
		return s.mockHandler
	})

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, `.*2 handlers registered for hook "fake-hook".*`)

	c.Check(s.mockHandler.BeforeCalled, check.Equals, false)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, false)
}

func (s *hookSuite) launchWorkshop(c *check.C, newsdk string) {
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{{Name: "one", Channel: "latest/stable"}}}
	err := s.backend.LaunchWorkshop(s.ctx, wf)
	c.Check(err, check.IsNil)
	ws, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer ws.Close()
	c.Check(err, check.IsNil)
	err = ws.MkdirAll(sdk.SdkHooksDir(newsdk), 0744)
	c.Check(err, check.IsNil)
	_, err = ws.Create(sdk.SdkHookPath(newsdk, hookstate.SaveState.String()))
	c.Check(err, check.IsNil)
	_, err = ws.Create(sdk.SdkHookPath(newsdk, hookstate.RestoreState.String()))
	c.Check(err, check.IsNil)
	_, err = ws.Create(sdk.SdkHookPath(newsdk, hookstate.SetupBase.String()))
	c.Check(err, check.IsNil)
	_, err = ws.Create(sdk.SdkHookPath(newsdk, hookstate.FakeHook.String()))
	c.Check(err, check.IsNil)
}
