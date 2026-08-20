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

package workshopstate_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

var wsFocal = `name: ws
base: ubuntu@20.04
`

var wsJammy = `name: ws
base: ubuntu@22.04
`

type workshopHandlers struct {
	backend *fakebackend.FakeWorkshopBackend
	state   *state.State
	runner  *state.TaskRunner
	se      *overlord.StateEngine
	sdkmgr  *sdkstate.SdkManager
	wrkmgr  *workshopstate.WorkshopManager
	ctx     context.Context
	project workshop.Project
	user    *user.User

	restoreUserLookup func()
	restoreUserEnv    func()
}

var _ = check.Suite(&workshopHandlers{})

func fakeHandler(task *state.Task, _ *tomb.Tomb) error {
	return nil
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, i := range tasks {
		i.Set("workshop", w)
		i.Set("project", p)
	}
}

func (s *workshopHandlers) createWFile(c *check.C, name string, yaml string) {
	path := workshop.Filepath(s.project.Path, name)

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(yaml), 0644)
	c.Assert(err, check.IsNil)
}

var ErrTrigger = errors.New("error out")

func (s *workshopHandlers) SetUpTest(c *check.C) {
	var err error
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")

	dirs.SetRootDir(c.MkDir())

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)

	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)

	// Real UID and GID are required to create the apt cache directory
	actual, err := user.Current()
	c.Assert(err, check.IsNil)
	s.user = &user.User{
		HomeDir: c.MkDir(),
		Uid:     actual.Uid,
		Gid:     actual.Gid,
	}
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser" {
			return nil, user.UnknownUserError("not found")
		}
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)

	sdk.ReplaceStore(s.state, sdk.NewFakeStore())
	s.sdkmgr = sdkstate.New(s.state, s.runner, interfaces.NewRepository())

	// empty task handler
	workshop.ReplaceBackend(s.state, s.backend)
	s.runner.AddHandler("fake-task", fakeHandler, nil)
	s.wrkmgr = workshopstate.New(s.state, s.runner)

	// error-provoking task handler
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return ErrTrigger
	}
	s.runner.AddHandler("error-trigger", handlersetup.OnDo(erroringHandler), nil)

	s.se = overlord.NewStateEngine(s.state)
	s.se.AddManager(s.sdkmgr)
	s.se.AddManager(s.wrkmgr)
	s.se.AddManager(s.runner)
	err = s.se.StartUp()
	c.Check(err, check.IsNil)
}

func (s *workshopHandlers) TearDownTest(c *check.C) {
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *workshopHandlers) TestStopPeriodicProgressUpdate(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsFocal)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)

	t1, err := s.wrkmgr.StopMany(s.ctx, []string{"ws"}, s.project.ProjectId)
	c.Check(err, check.IsNil)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t1[0].Tasks()...)
	chg.Set("user", "testuser")
	chg.AddAll(t1[0])

	oldInterval := workshopstate.StopLogInterval
	workshopstate.StopLogInterval = 100 * time.Millisecond

	restore := testutil.FakeFunc(func(_ workshop.Backend, ctx context.Context, name string, force bool) error {
		time.Sleep(150 * time.Millisecond)
		return nil
	}, &workshopstate.StopWorkshop)
	defer restore()

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1[0].Tasks()[0].Log()[0], check.Matches, ".*Still waiting for \"ws\" to stop; no change in the last 30 seconds...")
	c.Assert(t1[0].Tasks()[0].Log(), check.HasLen, 1)
	c.Check(chg.Err(), check.Equals, nil)
	workshopstate.StopLogInterval = oldInterval
}

func (s *workshopHandlers) TestUndoStash(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{
		{Name: "test", Channel: "latest/stable"},
		{Name: "test2", Channel: "latest/stable"},
	}}

	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Check(err, check.IsNil)

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("stash-workshop", "...")
	t2 := s.state.NewTask("error-trigger", "...")
	t2.WaitFor(t1)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t2)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.UndoneStatus)
	c.Assert(s.backend.StashedWorkshops[s.project.ProjectId], check.HasLen, 0)
	c.Assert(s.backend.Workshops[s.project.ProjectId], check.HasLen, 1)
	c.Assert(s.backend.Workshops[s.project.ProjectId]["ws"], check.NotNil)
}

func (s *workshopHandlers) TestRemoveWorkshop(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	wFiles := []*workshop.File{{
		Name: "ws", Base: "ubuntu@20.04",
		Sdks: []workshop.SdkRecord{
			{Name: "test", Channel: "latest/stable"},
			{Name: "test2", Channel: "latest/stable"},
		}}, {
		Name: "another-ws", Base: "ubuntu@20.04",
		Sdks: []workshop.SdkRecord{
			{Name: "test", Channel: "latest/stable"},
			{Name: "test2", Channel: "latest/stable"},
		},
	}}

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)

	for _, wf := range wFiles {
		snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, "fakeimage123")
		err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
		c.Check(err, check.IsNil)

		// create content directories
		chg := s.state.NewChange("sample", "...")
		t1 := s.state.NewTask("create-workshop-storage", "...")
		setWorkshopProject(wf.Name, s.project, t1)
		chg.Set("user", "testuser")
		chg.AddTask(t1)

		s.state.Unlock()
		for i := 0; i < 6; i = i + 1 {
			c.Assert(s.se.Ensure(), check.IsNil)
			s.se.Wait()
		}
		s.state.Lock()

		c.Assert(t1.Status(), check.Equals, state.DoneStatus)

		sdkMountData := workshop.SdkMountDir(userDataDir, s.project.ProjectId, wf.Name, "test")
		var plugs = []string{"plug1", "plug2"}
		for _, p := range plugs {
			err = os.MkdirAll(filepath.Join(sdkMountData, p), 0744)
			c.Assert(err, check.IsNil)
		}
	}

	for _, wf := range wFiles {
		// Make sure content exists, we only care about this at a directory level
		workshopUserData := workshop.UserData(userDataDir, s.project.ProjectId, wf.Name)
		c.Check(workshopUserData, testutil.DirEquals, []string{
			"drwxr--r-- mount",
		})

		aptCache := workshop.AptCacheDir(s.project.ProjectId, wf.Name)
		c.Check(aptCache, testutil.DirEquals, []string{})

		chg := s.state.NewChange("sample", "...")
		t1 := s.state.NewTask("remove-workshop", "...")
		t2 := s.state.NewTask("remove-workshop-storage", "...")
		t2.WaitFor(t1)
		setWorkshopProject(wf.Name, s.project, t1, t2)
		chg.Set("user", "testuser")
		chg.AddTask(t1)
		chg.AddTask(t2)

		s.state.Unlock()
		for i := 0; i < 6; i = i + 1 {
			c.Assert(s.se.Ensure(), check.IsNil)
			s.se.Wait()
		}
		s.state.Lock()

		c.Assert(t1.Status(), check.Equals, state.DoneStatus)
		ws, err := s.backend.Workshop(s.ctx, wf.Name)
		c.Assert(ws, check.IsNil)
		c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)

		c.Check(workshopUserData, testutil.FileAbsent)
		c.Check(aptCache, testutil.FileAbsent)
	}

	// Make sure project directories are cleaned up
	projectContent := workshop.ProjectUserData(userDataDir, s.project.ProjectId)
	c.Check(projectContent, testutil.FileAbsent)

	projectCache := workshop.ProjectCacheDir(s.project.ProjectId)
	c.Check(projectCache, testutil.FileAbsent)
}

func (s *workshopHandlers) TestCreateWorkshopNoWorkshopDefinitionFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*internal error: "ws" workshop definition not found.*`)
}

func (s *workshopHandlers) TestCreateWorkshopWithSystemSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	t1.Set("workshop-file", wsJammy)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
}

func (s *workshopHandlers) TestCreateWorkshopCleanup(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)

	reset := s.backend.SetLaunchOrRebuildCallback(func(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error {
		return errors.New("contrived error")
	})
	defer reset()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	t1.Set("workshop-file", wsJammy)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(contrived error\)`)
	_, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
}

func (s *workshopHandlers) TestRebuildWorkshopNoCleanup(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)

	reset := s.backend.SetLaunchOrRebuildCallback(func(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error {
		return errors.New("contrived error")
	})
	defer reset()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("rebuild-workshop", "...")
	t1.Set("workshop-file", wsJammy)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	image := workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"}
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", image)
	chg.Set("ws_new_sdks", []sdk.Setup{})
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(contrived error\)`)
	ws, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	c.Check(ws.Image, check.Equals, image)
}

func (s *workshopHandlers) TestDownloadBase(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.backend.DownloadBaseCallback = func(ctx context.Context, image workshop.BaseImage, report *progress.Reporter) error {
		c.Check(image.Name, check.Equals, "ubuntu@22.04")
		c.Check(image.Fingerprint, check.Equals, "fakeimage1234")
		report.Report("download finished", 100, 100)
		return nil
	}
	defer func() {
		s.backend.DownloadBaseCallback = nil
	}()

	s.createWFile(c, "ws", wsJammy)

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("download-base", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage1234"})
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	label, done, total := t1.Progress()
	c.Assert(label, check.Equals, "download finished")
	c.Assert(done, check.Equals, int64(100))
	c.Assert(total, check.Equals, int64(100))
}

func (s *workshopHandlers) TestConfigureTimezoneBrokenHost(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	restoreTimezone := osutil.FakeTimezone(func() (string, error) {
		return "", errors.New("timedatectl: command not found")
	})
	defer restoreTimezone()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("configure-timezone", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	warnings := s.state.AllWarnings()
	c.Assert(warnings, check.HasLen, 1)
	c.Check(warnings[0].String(), check.Equals, "cannot determine system time zone: timedatectl: command not found")
}

func (s *workshopHandlers) TestConfigureTimezoneBrokenWorkshop(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	restoreTimezone := osutil.FakeTimezone(func() (string, error) {
		return "Antarctica/Troll", nil
	})
	defer restoreTimezone()

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		return workshop.ExecContext{}, errors.New("timedatectl: command not found")
	}

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("configure-timezone", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(timedatectl: command not found\)`)
}

func (s *workshopHandlers) TestConfigureTimezoneBrokenDpkg(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	restoreTimezone := osutil.FakeTimezone(func() (string, error) {
		return "Antarctica/Troll", nil
	})
	defer restoreTimezone()

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		if args.Command[0] != "dpkg-reconfigure" {
			return fakebackend.DoExecDefault(ctx, name, args)
		}
		return workshop.ExecContext{}, errors.New("dpkg-reconfigure: read only filesystem")
	}

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("configure-timezone", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(dpkg-reconfigure: read only filesystem\)`)
}

func (s *workshopHandlers) TestConfigureTimezoneMissingDpkg(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	restoreTimezone := osutil.FakeTimezone(func() (string, error) {
		return "Antarctica/Troll", nil
	})
	defer restoreTimezone()

	s.backend.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		if args.Command[0] != "dpkg-reconfigure" {
			return fakebackend.DoExecDefault(ctx, name, args)
		}
		exectx := workshop.ExecContext{
			WaitExecution: func(ctx context.Context) error {
				return &workshop.ErrExec{Status: osutil.CommandNotFound}
			},
		}
		return exectx, nil
	}

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("configure-timezone", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	c.Check(s.backend.ExecCalls, check.HasLen, 2)
}

func setWorkshopManifest(chg *state.Change, age handlersetup.Age, manifest workshopstate.Manifest) {
	chg.Set(handlersetup.WorkshopFormatKey(manifest.File.Name, age), manifest.Format)
	chg.Set(handlersetup.WorkshopBaseKey(manifest.File.Name, age), manifest.Image)
	chg.Set(handlersetup.WorkshopSdksKey(manifest.File.Name, age), manifest.Sdks)
}

func (s *workshopHandlers) launchWorkshop(c *check.C, manifest workshopstate.Manifest) {
	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.NewWorkshop, manifest)

	create := s.state.NewTask("create-workshop", "...")
	handlersetup.SetWorkshopFile(create, manifest.File)
	setWorkshopProject(manifest.File.Name, s.project, create)
	chg.AddTask(create)

	prev := create
	for _, setup := range manifest.Sdks {
		t := s.state.NewTask("snapshot-sdk", "...")
		t.Set("sdk", setup.Name)
		setWorkshopProject(manifest.File.Name, s.project, t)
		t.WaitFor(prev)
		chg.AddTask(t)
		prev = t
	}

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.IsReady(), check.Equals, true)
}

var (
	testSdk = sdk.Setup{
		Name:      "test-sdk",
		PackageID: "0CT76RxrmdAGQwbVIAbBel0EBoIv8lP7",
		Channel:   "latest/stable",
		Revision:  sdk.R(1),
		Sha3_384:  "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
	}

	testSdk2 = sdk.Setup{
		Name:      "test-sdk-2",
		PackageID: "1AiTeMJjEGJyaBU8qiFiQDEfKjGTx3jp",
		Channel:   "latest/stable",
		Revision:  sdk.R(1),
		Sha3_384:  "d4089378c26310627268153caa216240311f2a3193c778e96ed6dd895dc10c82db50f4f39676b29d23d9813b21e14b9b",
	}
)

func (s *workshopHandlers) TestSnapshotRemovedAfterRemove(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk, testSdk2},
	}
	snapshot1 := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks[:1])
	snapshot2 := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks)

	s.launchWorkshop(c, manifest)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	s2, err := s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.IsNil)
	c.Check(s2.Snapshot, check.DeepEquals, snapshot2)
	c.Check(s2.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	chg := s.state.NewChange("remove", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.OldWorkshop, manifest)

	t := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(t.IsClean(), check.Equals, true)

	_, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
	_, err = s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
}

func (s *workshopHandlers) TestSnapshotRemovedAfterFailedLaunch(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk, testSdk2},
	}
	snapshot1 := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks[:1])
	snapshot2 := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.NewWorkshop, manifest)

	t := s.state.NewTask("create-workshop", "...")
	handlersetup.SetWorkshopFile(t, manifest.File)
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	t1 := s.state.NewTask("snapshot-sdk", "...")
	t1.Set("sdk", "test-sdk")
	setWorkshopProject("ws", s.project, t1)
	t1.WaitFor(t)
	chg.AddTask(t1)

	t2 := s.state.NewTask("snapshot-sdk", "...")
	t2.Set("sdk", "test-sdk-2")
	setWorkshopProject("ws", s.project, t2)
	t2.WaitFor(t1)
	chg.AddTask(t2)

	terr := s.state.NewTask("error-trigger", "...")
	setWorkshopProject("ws", s.project, terr)
	terr.WaitFor(t2)
	chg.AddTask(terr)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 12 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.NotNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(t.Status(), check.Equals, state.UndoneStatus)
	c.Assert(t.IsClean(), check.Equals, true)

	_, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
	_, err = s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
}

func (s *workshopHandlers) TestSnapshotRemovedAfterRefresh(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	current := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(current.Format, current.Image, current.Sdks)

	latest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk2},
	}
	snapshot2 := workshop.SdkSnapshot(latest.Format, latest.Image, latest.Sdks)

	s.launchWorkshop(c, current)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	chg := s.state.NewChange("refresh", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.OldWorkshop, current)
	setWorkshopManifest(chg, handlersetup.NewWorkshop, latest)

	stash := s.state.NewTask("stash-workshop", "...")
	setWorkshopProject("ws", s.project, stash)
	chg.AddTask(stash)

	rebuild := s.state.NewTask("rebuild-workshop", "...")
	handlersetup.SetWorkshopFile(rebuild, latest.File)
	setWorkshopProject("ws", s.project, rebuild)
	rebuild.WaitFor(stash)
	chg.AddTask(rebuild)

	snapshot := s.state.NewTask("snapshot-sdk", "...")
	snapshot.Set("sdk", "test-sdk-2")
	setWorkshopProject("ws", s.project, snapshot)
	snapshot.WaitFor(rebuild)
	chg.AddTask(snapshot)

	removeStash := s.state.NewTask("remove-workshop-stash", "...")
	setWorkshopProject("ws", s.project, removeStash)
	removeStash.WaitFor(snapshot)
	chg.AddTask(removeStash)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(removeStash.IsClean(), check.Equals, true)

	_, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
	s2, err := s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.IsNil)
	c.Check(s2.Snapshot, check.DeepEquals, snapshot2)
	c.Check(s2.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})
}

func (s *workshopHandlers) TestSnapshotRemovedAfterFailedRefresh(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	current := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(current.Format, current.Image, current.Sdks)

	latest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk2},
	}
	snapshot2 := workshop.SdkSnapshot(latest.Format, latest.Image, latest.Sdks)

	s.launchWorkshop(c, current)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	chg := s.state.NewChange("refresh", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.OldWorkshop, current)
	setWorkshopManifest(chg, handlersetup.NewWorkshop, latest)

	stash := s.state.NewTask("stash-workshop", "...")
	setWorkshopProject("ws", s.project, stash)
	chg.AddTask(stash)

	rebuild := s.state.NewTask("rebuild-workshop", "...")
	handlersetup.SetWorkshopFile(rebuild, latest.File)
	setWorkshopProject("ws", s.project, rebuild)
	rebuild.WaitFor(stash)
	chg.AddTask(rebuild)

	snapshot := s.state.NewTask("snapshot-sdk", "...")
	snapshot.Set("sdk", "test-sdk-2")
	setWorkshopProject("ws", s.project, snapshot)
	snapshot.WaitFor(rebuild)
	chg.AddTask(snapshot)

	terr := s.state.NewTask("error-trigger", "...")
	setWorkshopProject("ws", s.project, terr)
	terr.WaitFor(snapshot)
	chg.AddTask(terr)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 12 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.NotNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(stash.Status(), check.Equals, state.UndoneStatus)
	c.Assert(stash.IsClean(), check.Equals, true)

	s1, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})
	_, err = s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
}

func (s *workshopHandlers) TestSnapshotRemovedAfterRemoveMidRefresh(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	current := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(current.Format, current.Image, current.Sdks)

	latest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk2},
	}
	snapshot2 := workshop.SdkSnapshot(latest.Format, latest.Image, latest.Sdks)

	s.launchWorkshop(c, current)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	// First, refresh and wait on error.
	refresh := s.state.NewChange("refresh", "...")
	refresh.Set("user", "testuser")
	refresh.Set("project-id", s.project.ProjectId)
	refresh.Set("wait-setup", conflict.ChangeSetup{Mode: conflict.ChangeWaitOnError.String()})
	setWorkshopManifest(refresh, handlersetup.OldWorkshop, current)
	setWorkshopManifest(refresh, handlersetup.NewWorkshop, latest)

	stash := s.state.NewTask("stash-workshop", "...")
	setWorkshopProject("ws", s.project, stash)
	refresh.AddTask(stash)

	rebuild := s.state.NewTask("rebuild-workshop", "...")
	handlersetup.SetWorkshopFile(rebuild, latest.File)
	setWorkshopProject("ws", s.project, rebuild)
	rebuild.WaitFor(stash)
	refresh.AddTask(rebuild)

	snapshot := s.state.NewTask("snapshot-sdk", "...")
	snapshot.Set("sdk", "test-sdk-2")
	setWorkshopProject("ws", s.project, snapshot)
	snapshot.WaitFor(rebuild)
	refresh.AddTask(snapshot)

	terr := s.state.NewTask("error-trigger", "...")
	setWorkshopProject("ws", s.project, terr)
	terr.WaitFor(snapshot)
	refresh.AddTask(terr)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(refresh.Err(), check.IsNil)
	c.Assert(refresh.Status(), check.Equals, state.WaitStatus)
	c.Assert(terr.Status(), check.Equals, state.WaitStatus)

	s1, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	s2, err := s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.IsNil)
	c.Check(s2.Snapshot, check.DeepEquals, snapshot2)
	c.Check(s2.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})

	// Next, remove the partially-refreshed workshop.
	err = conflict.CheckChangeConflict(
		s.state,
		s.project.ProjectId,
		"ws",
		[]string{"exec"},
	)
	c.Assert(err, check.DeepEquals, &conflict.ChangeConflictError{
		ProjectId:    s.project.ProjectId,
		Workshop:     "ws",
		ChangeKind:   refresh.Kind(),
		ChangeStatus: state.WaitStatus.String(),
		ChangeID:     refresh.ID(),
	})
	conflict.BackgroundDiscard(refresh, "ws")

	remove := s.state.NewChange("remove", "...")
	remove.Set("user", "testuser")
	setWorkshopManifest(remove, handlersetup.OldStash, current)
	setWorkshopManifest(remove, handlersetup.OldWorkshop, latest)

	removeWorkshop := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, removeWorkshop)
	remove.AddTask(removeWorkshop)

	removeStash := s.state.NewTask("remove-workshop-stash", "...")
	setWorkshopProject("ws", s.project, removeStash)
	removeStash.WaitFor(removeWorkshop)
	remove.AddTask(removeStash)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(refresh.Err(), check.IsNil)
	c.Assert(refresh.IsReady(), check.Equals, true)
	c.Assert(remove.Err(), check.IsNil)
	c.Assert(remove.IsReady(), check.Equals, true)
	c.Assert(removeWorkshop.IsClean(), check.Equals, true)
	c.Assert(removeStash.IsClean(), check.Equals, true)

	_, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
	_, err = s.backend.Snapshot(s.ctx, snapshot2)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
}

func (s *workshopHandlers) TestSnapshotExitCleanupAfterSuccessfulLaunch(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.NewWorkshop, manifest)

	t := s.state.NewTask("create-workshop", "...")
	handlersetup.SetWorkshopFile(t, manifest.File)
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	t1 := s.state.NewTask("snapshot-sdk", "...")
	t1.Set("sdk", "test-sdk")
	setWorkshopProject("ws", s.project, t1)
	t1.WaitFor(t)
	chg.AddTask(t1)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(t.IsClean(), check.Equals, true)

	s1, err := s.backend.Snapshot(s.ctx, snapshot)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws"})
}

func (s *workshopHandlers) TestSnapshotNotRemovedBeforeCooldown(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks)

	s.launchWorkshop(c, manifest)

	chg := s.state.NewChange("remove", "...")
	chg.Set("user", "testuser")
	setWorkshopManifest(chg, handlersetup.OldWorkshop, manifest)

	t := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	// Set cooldown to a large value so it never passes.
	defer workshopstate.FakeSnapshotCooldownTime(24 * time.Hour)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	// The task should not be clean (cleanup not performed).
	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.IsReady(), check.Equals, true)
	c.Assert(t.IsClean(), check.Equals, false)

	// The snapshot should still exist.
	s1, err := s.backend.Snapshot(s.ctx, snapshot)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot)
	c.Check(s1.Workshops[s.project.ProjectId], check.HasLen, 0)
}

func (s *workshopHandlers) TestSnapshotExitCleanupIfUsedAgain(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest1 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(manifest1.Format, manifest1.Image, manifest1.Sdks)

	manifest2 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws2", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}

	s.launchWorkshop(c, manifest1)

	// Launch ws2.
	launch := s.state.NewChange("launch", "...")
	launch.Set("user", "testuser")
	setWorkshopManifest(launch, handlersetup.NewWorkshop, manifest2)

	create := s.state.NewTask("create-workshop", "...")
	handlersetup.SetWorkshopFile(create, manifest2.File)
	setWorkshopProject("ws2", s.project, create)
	launch.AddTask(create)

	snapshot := s.state.NewTask("snapshot-sdk", "...")
	snapshot.Set("sdk", "test-sdk")
	setWorkshopProject("ws2", s.project, snapshot)
	snapshot.WaitFor(create)
	launch.AddTask(snapshot)

	// Remove ws.
	remove := s.state.NewChange("remove", "...")
	remove.Set("user", "testuser")
	setWorkshopManifest(remove, handlersetup.OldWorkshop, manifest1)
	removeWorkshop := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, removeWorkshop)
	remove.AddTask(removeWorkshop)

	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(launch.Err(), check.IsNil)
	c.Assert(launch.IsReady(), check.Equals, true)
	c.Assert(remove.Err(), check.IsNil)
	c.Assert(remove.IsReady(), check.Equals, true)
	c.Assert(removeWorkshop.IsClean(), check.Equals, true)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.DeepEquals, []string{"ws2"})
}

func (s *workshopHandlers) TestSnapshotRetriesCleanupIfBlockingChangesArePresent(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest1 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(manifest1.Format, manifest1.Image, manifest1.Sdks)

	manifest2 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws2", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}

	s.launchWorkshop(c, manifest1)

	// Remove ws.
	remove := s.state.NewChange("remove", "...")
	remove.Set("user", "testuser")
	setWorkshopManifest(remove, handlersetup.OldWorkshop, manifest1)

	removeWorkshop := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, removeWorkshop)
	remove.AddTask(removeWorkshop)

	// Launch ws2 that using the same snapshot.
	launch := s.state.NewChange("launch", "...")
	launch.Set("user", "testuser")
	setWorkshopManifest(launch, handlersetup.NewWorkshop, manifest2)

	create := s.state.NewTask("create-workshop", "...")
	handlersetup.SetWorkshopFile(create, manifest2.File)
	setWorkshopProject("ws2", s.project, create)
	create.SetToWait(state.DoStatus)
	launch.AddTask(create)

	terr := s.state.NewTask("error-trigger", "...")
	setWorkshopProject("ws2", s.project, terr)
	terr.WaitFor(create)
	launch.AddTask(terr)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(launch.Err(), check.IsNil)
	c.Assert(launch.IsReady(), check.Equals, false)
	c.Assert(remove.Err(), check.IsNil)
	c.Assert(remove.IsReady(), check.Equals, true)
	c.Assert(removeWorkshop.IsClean(), check.Equals, false)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.HasLen, 0)

	// Finish the blocking launch change so cleanup can proceed.
	waited := create.WaitedStatus()
	create.SetStatus(waited)
	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(launch.Err(), check.NotNil)
	c.Assert(launch.IsReady(), check.Equals, true)
	c.Assert(remove.Err(), check.IsNil)
	c.Assert(remove.IsReady(), check.Equals, true)
	c.Assert(removeWorkshop.IsClean(), check.Equals, true)

	_, err = s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.Equals, workshop.ErrSnapshotNotFound)
}

func (s *workshopHandlers) TestSnapshotCleanupPerformedByLatestUser(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest1 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot1 := workshop.SdkSnapshot(manifest1.Format, manifest1.Image, manifest1.Sdks)

	manifest2 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws2", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}

	s.launchWorkshop(c, manifest1)
	s.launchWorkshop(c, manifest2)

	chg1 := s.state.NewChange("remove", "...")
	chg1.Set("user", "testuser")
	setWorkshopManifest(chg1, handlersetup.OldWorkshop, manifest1)

	t1 := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, t1)
	chg1.AddTask(t1)

	chg2 := s.state.NewChange("remove", "...")
	chg2.Set("user", "testuser")
	setWorkshopManifest(chg2, handlersetup.OldWorkshop, manifest2)

	t2 := s.state.NewTask("remove-workshop", "...")
	t2.WaitFor(t1)
	setWorkshopProject("ws2", s.project, t2)
	chg2.AddTask(t2)

	// Use default cooldown (1h), so t2 will not be clean (cooldown not passed).
	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg1.Err(), check.IsNil)
	c.Assert(chg1.IsReady(), check.Equals, true)
	c.Assert(chg2.Err(), check.IsNil)
	c.Assert(chg2.IsReady(), check.Equals, true)
	// The first task should be clean (cleanup skipped due to newer task)
	c.Assert(t1.IsClean(), check.Equals, true)
	// The second task should not be clean (cleanup not performed, cooldown not passed)
	c.Assert(t2.IsClean(), check.Equals, false)

	s1, err := s.backend.Snapshot(s.ctx, snapshot1)
	c.Assert(err, check.IsNil)
	c.Check(s1.Snapshot, check.DeepEquals, snapshot1)
	c.Check(s1.Workshops[s.project.ProjectId], check.HasLen, 0)
}

func (s *workshopHandlers) TestSnapshotCleanupWaitsForDependentSnapshots(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	manifest1 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk, testSdk2},
	}
	snapshot1 := workshop.SdkSnapshot(manifest1.Format, manifest1.Image, manifest1.Sdks)

	manifest2 := workshopstate.Manifest{
		File:   &workshop.File{Name: "ws2", Base: "ubuntu@22.04"},
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"},
		Sdks:   []sdk.Setup{testSdk},
	}
	snapshot2 := workshop.SdkSnapshot(manifest2.Format, manifest2.Image, manifest2.Sdks)

	s.launchWorkshop(c, manifest1)
	s.launchWorkshop(c, manifest2)

	chg1 := s.state.NewChange("remove", "...")
	chg1.Set("user", "testuser")
	setWorkshopManifest(chg1, handlersetup.OldWorkshop, manifest1)

	t1 := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws", s.project, t1)
	chg1.AddTask(t1)

	chg2 := s.state.NewChange("remove", "...")
	chg2.Set("user", "testuser")
	setWorkshopManifest(chg2, handlersetup.OldWorkshop, manifest2)

	t2 := s.state.NewTask("remove-workshop", "...")
	setWorkshopProject("ws2", s.project, t2)
	// We remove ws2 after ws, so ws2 is responsible for cleaning up the
	// test-sdk snapshot, but still has to wait for ws to clean up test-sdk-2.
	t2.WaitFor(t1)
	chg2.AddTask(t2)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg1.Err(), check.IsNil)
	c.Assert(chg1.IsReady(), check.Equals, true)
	c.Assert(chg2.Err(), check.IsNil)
	c.Assert(chg2.IsReady(), check.Equals, true)
	c.Assert(t1.IsClean(), check.Equals, false)
	c.Assert(t2.IsClean(), check.Equals, false)

	c.Assert(s.backend.RemovedSnapshots, check.HasLen, 0)

	// Drop the cooldown so both cleanup handlers run simultaneously.
	defer workshopstate.FakeSnapshotCooldownTime(0)()

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg1.Err(), check.IsNil)
	c.Assert(chg1.IsReady(), check.Equals, true)
	c.Assert(chg2.Err(), check.IsNil)
	c.Assert(chg2.IsReady(), check.Equals, true)
	c.Assert(t1.IsClean(), check.Equals, true)
	c.Assert(t2.IsClean(), check.Equals, true)

	c.Assert(s.backend.RemovedSnapshots, check.DeepEquals, []workshop.Snapshot{snapshot1, snapshot2})
}
