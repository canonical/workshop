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

package hookstate_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/hooktest"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type hookSuite struct {
	backend     *fakebackend.FakeWorkshopBackend
	state       *state.State
	runner      *state.TaskRunner
	se          *overlord.StateEngine
	hookmgr     *hookstate.HookManager
	user        *user.User
	ctx         context.Context
	project     workshop.Project
	mockHandler *hooktest.MockHandler

	restoreUserEnv    func()
	restoreUserLookup func()
}

var _ = check.Suite(&hookSuite{})

type statErrorFs struct {
	fsutil.Fs
	err error
}

func (f statErrorFs) Stat(string) (os.FileInfo, error) {
	return nil, f.err
}

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
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)

	var err error
	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)

	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")
	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)

	// Real UID and GID are required to create SDK state storage directories.
	// TODO: make filesystem operations more secure (e.g. drop privileges if possible) and easy to test.
	actual, err := user.Current()
	c.Assert(err, check.IsNil)
	s.user = &user.User{Username: "testuser", HomeDir: c.MkDir(), Uid: actual.Uid, Gid: actual.Gid}
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

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

func (s *hookSuite) TearDownTest(c *check.C) {
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *hookSuite) TestExecHookDoesNotExist(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	t1 := hookstate.Hook(s.state, "new", 0, hookstate.SetupBase)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	// Launch a workshop provinding no hooks
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Assert(s.backend.ExecCalls, check.HasLen, 0)
	c.Check(t1.Status(), check.Equals, state.DoneStatus)
}

func (s *hookSuite) TestExecHookSkipsStrayHooksFile(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.SetupBase)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)

	fs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer fs.Close()
	err = fs.MkdirAll(filepath.Dir(sdk.SdkHooksDir("one")), 0755)
	c.Check(err, check.IsNil)
	err = fs.WriteFile(sdk.SdkHooksDir("one"), []byte("not a directory"), 0644)
	c.Check(err, check.IsNil)

	s.state.Unlock()
	err = s.se.Ensure()
	c.Assert(err, check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.DoneStatus)
	c.Check(t1.Log(), check.HasLen, 0)
	c.Check(s.backend.ExecCalls, check.HasLen, 0)
}

func (s *hookSuite) TestExecHookStatError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.SetupBase)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)

	fs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)

	statErr := &os.PathError{
		Op:   "stat",
		Path: sdk.SdkHookPath("one", hookstate.SetupBase.String()),
		Err:  syscall.EIO,
	}

	restore := s.backend.SetWorkshopFsCallback(func(context.Context, string) (fsutil.Fs, error) {
		return fsutil.Fs{FsBackend: statErrorFs{Fs: fs, err: statErr}}, nil
	})
	defer restore()

	s.state.Unlock()
	err = s.se.Ensure()
	c.Assert(err, check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Check(t1.Log()[0], check.Matches, `(?s).*cannot read hook: stat .*/setup-base: input/output error.*`)
	c.Check(s.backend.ExecCalls, check.HasLen, 0)
}

func (s *hookSuite) TestExecSetupProject(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.SetupProject)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("verbose", true)
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")

	s.state.Unlock()
	c.Assert(s.se.Ensure(), check.IsNil)
	s.se.Wait()
	s.state.Lock()
	c.Assert(chg.Err(), check.IsNil)
	c.Assert(s.backend.ExecCalls, check.HasLen, 1)
	c.Check(s.backend.ExecCalls[0].Args.Command, check.DeepEquals, []string{
		"sudo",
		"--user=#1000",
		"--group=#1000",
		"--preserve-env=DBUS_SESSION_BUS_ADDRESS",
		"--preserve-env=HOME",
		"--preserve-env=SDK",
		"--preserve-env=WORKSHOP_COOKIE",
		"--preserve-env=XDG_RUNTIME_DIR",
		"--",
		"bash",
		"-l",
		"-c",
		`exec -- "$0" "$@"`,
		"bash",
		"-o",
		"errexit",
		"-o",
		"pipefail",
		"-o",
		"xtrace",
		"/var/lib/workshop/sdk/one/sdk/hooks/setup-project",
	})
	c.Check(s.backend.ExecCalls[0].Args.WorkDir, check.Equals, "/project")
}

func (s *hookSuite) TestExecSaveState(c *check.C) {
	s.launchWorkshop(c, "one")

	storage := workshop.StateStorageDir(s.project.ProjectId, "ws")
	err := os.MkdirAll(storage, 0755)
	c.Assert(err, check.IsNil)
	defer func() { _ = os.RemoveAll(storage) }()

	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.SaveState)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()
	c.Assert(chg.Err(), check.IsNil)

	// Ensure that the save-state handler has created the required state
	// directory (reattach the volume to the workshop to check).
	ws, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer ws.Close()
	mount := workshop.Mount{
		Name:  workshop.ConfigStateStorageDevice,
		Type:  workshop.HostWorkshop,
		What:  storage,
		Where: dirs.WorkshopStateDir,
	}
	err = s.backend.AddWorkshopMount(s.ctx, "ws", mount)
	c.Check(err, check.IsNil)
	info, err := ws.Stat("/var/lib/workshop/state/sdk/one")
	c.Check(err, check.IsNil)
	c.Assert(info.IsDir(), check.Equals, true)
	c.Assert(t1.Log(), check.HasLen, 0)
	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, check.DeepEquals,
		[]string{"sudo", "--user=#0", "--group=#0", "--preserve-env=SDK", "--preserve-env=SDK_STATE_DIR", "--preserve-env=WORKSHOP_COOKIE", "--", "bash", "-l", "-c", `exec -- "$0" "$@"`, "bash", "-o", "errexit", "-o", "pipefail", "/var/lib/workshop/sdk/one/sdk/hooks/save-state"})
	c.Assert(s.backend.ExecCalls[0].Args.Environment["SDK_STATE_DIR"], check.Equals, "/var/lib/workshop/state/sdk/one")
	c.Assert(s.backend.ExecCalls[0].Args.Environment["WORKSHOP_COOKIE"], check.NotNil)
	c.Assert(s.backend.ExecCalls[0].Args.Environment, check.HasLen, 3)
}

func (s *hookSuite) TestExecRestoreState(c *check.C) {
	s.launchWorkshop(c, "one")

	// Setup state storage (must be already set by the save-state in a real use
	// case).
	storage := workshop.StateStorageDir(s.project.ProjectId, "ws")
	err := os.MkdirAll(filepath.Join(storage, "sdk", "one"), 0755)
	c.Assert(err, check.IsNil)
	defer func() { _ = os.RemoveAll(storage) }()

	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.RestoreState)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("verbose", true)
	chg.AddTask(t1)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, check.DeepEquals,
		[]string{"sudo", "--user=#0", "--group=#0", "--preserve-env=SDK", "--preserve-env=SDK_STATE_DIR", "--preserve-env=WORKSHOP_COOKIE", "--", "bash", "-l", "-c", `exec -- "$0" "$@"`, "bash", "-o", "errexit", "-o", "pipefail", "-o", "xtrace", "/var/lib/workshop/sdk/one/sdk/hooks/restore-state"})
	c.Assert(s.backend.ExecCalls[0].Args.Environment["SDK_STATE_DIR"], check.Equals, "/var/lib/workshop/state/sdk/one")
	c.Assert(s.backend.ExecCalls[0].Args.Environment["WORKSHOP_COOKIE"], check.NotNil)
	c.Assert(s.backend.ExecCalls[0].Args.Environment, check.HasLen, 3)
}

func (s *hookSuite) TestExecHandlesFailedHook(c *check.C) {
	s.launchWorkshop(c, "one")

	storage := workshop.StateStorageDir(s.project.ProjectId, "ws")
	err := os.MkdirAll(storage, 0755)
	c.Assert(err, check.IsNil)
	defer func() { _ = os.RemoveAll(storage) }()

	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.SaveState)

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

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(s.backend.ExecCalls, check.HasLen, 1)
	c.Assert(s.backend.ExecCalls[0].Args.Command, check.DeepEquals,
		[]string{"sudo", "--user=#0", "--group=#0", "--preserve-env=SDK", "--preserve-env=SDK_STATE_DIR", "--preserve-env=WORKSHOP_COOKIE", "--", "bash", "-l", "-c", `exec -- "$0" "$@"`, "bash", "-o", "errexit", "-o", "pipefail", "/var/lib/workshop/sdk/one/sdk/hooks/save-state"})

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*hook execution error$")
}

func (s *hookSuite) TestExecHandlesHookTimedout(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 100*time.Millisecond, hookstate.FakeHook)

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
	c.Assert(s.backend.ExecCalls[0].Args.Command, check.DeepEquals,
		[]string{"sudo", "--user=#0", "--group=#0", "--preserve-env=SDK", "--preserve-env=WORKSHOP_COOKIE", "--", "bash", "-l", "-c", `exec -- "$0" "$@"`, "bash", "-o", "errexit", "-o", "pipefail", "/var/lib/workshop/sdk/one/sdk/hooks/fake-hook"})

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 1)
	c.Assert(t1.Log()[0], check.Matches, ".*context deadline exceeded$")
}

func (s *hookSuite) TestExecEnsureContextHandlerHappyPath(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)

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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)

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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)
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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)
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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)
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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)
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
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)
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
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)
	ws, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Check(err, check.IsNil)
	defer ws.Close()
	c.Check(err, check.IsNil)
	err = ws.MkdirAll(sdk.SdkHooksDir(newsdk), 0744)
	c.Check(err, check.IsNil)
	err = ws.WriteFile(sdk.SdkHookPath(newsdk, hookstate.SaveState.String()), nil, 0644)
	c.Check(err, check.IsNil)
	err = ws.WriteFile(sdk.SdkHookPath(newsdk, hookstate.RestoreState.String()), nil, 0644)
	c.Check(err, check.IsNil)
	err = ws.WriteFile(sdk.SdkHookPath(newsdk, hookstate.SetupBase.String()), nil, 0644)
	c.Check(err, check.IsNil)
	err = ws.WriteFile(sdk.SdkHookPath(newsdk, hookstate.SetupProject.String()), nil, 0644)
	c.Check(err, check.IsNil)
	err = ws.WriteFile(sdk.SdkHookPath(newsdk, hookstate.FakeHook.String()), nil, 0644)
	c.Check(err, check.IsNil)
}

func (s *hookSuite) TestExecCombinedHookOutputStoredInLogs(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	t1 := hookstate.Hook(s.state, "one", 0, hookstate.FakeHook)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.launchWorkshop(c, "one")
	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				_, err := args.Stdout.Write([]byte("hello from hook!\n"))
				c.Assert(err, check.IsNil)
				_, err = args.Stdout.Write([]byte("stdout message\n"))
				c.Assert(err, check.IsNil)
				_, err = args.Stderr.Write([]byte("\terror message\n"))
				c.Assert(err, check.IsNil)
				return &workshop.ErrExec{Status: 1}
			},
		}, nil
	}
	defer func() {
		s.backend.ExecCallback = fakebackend.DoExecDefault
	}()

	s.state.Unlock()
	err := s.se.Ensure()
	c.Assert(err, check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(t1.Log(), check.HasLen, 2)
	c.Assert(t1.Log()[0], check.Matches, ".*hello from hook!\nstdout message\n\terror message\n$")
	c.Assert(t1.Log()[1], check.Matches, ".* ERROR command exit code 1$")

	cached := s.state.Cached(hookstate.HookLogKey(t1.ID()))
	hookOutErr, ok := cached.(*hookstate.HookLog)
	c.Assert(ok, check.Equals, true)
	c.Assert(hookOutErr.Output().String(), check.Equals, "hello from hook!\nstdout message\n\terror message\n")

	c.Check(s.mockHandler.BeforeCalled, check.Equals, true)
	c.Check(s.mockHandler.DoneCalled, check.Equals, false)
	c.Check(s.mockHandler.ErrorCalled, check.Equals, true)
}
