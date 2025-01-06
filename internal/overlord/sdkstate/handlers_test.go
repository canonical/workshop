package sdkstate_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type sdkStateSuite struct {
	fs          afero.Fs
	backend     *fakebackend.FakeWorkshopBackend
	state       *state.State
	runner      *state.TaskRunner
	se          *overlord.StateEngine
	sdkmgr      *sdkstate.SdkManager
	repo        *interfaces.Repository
	ctx         context.Context
	project     workshop.Project
	installTime time.Time

	restoreProjectId   func()
	restoreInstallTime func()
}

var _ = check.Suite(&sdkStateSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func fakeHandler(task *state.Task, _ *tomb.Tomb) error {
	return nil
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, i := range tasks {
		i.Set("workshop", w)
		i.Set("project", p)
	}
}

var ErrTrigger = errors.New("error out")

var sdkYaml = `
name: test
base: ubuntu@22.04
plugs:
  plug:
    interface: test-interface
    attr: value
  plug2:
    interface: test-interface
    attr2: value2
`

var sdkYamlRev2 = `
name: test
base: ubuntu@22.04
plugs:
  plug:
    interface: test-interface
    attr: value
`

var sdkYamlViolatesPolicy = `
name: test-broken
base: ubuntu@22.04
plugs:
  plug:
    interface: test-interface
    attr: value
  plug2:
    interface: test-interface
    attr2: value2
slots:
  slot:
    interface: mount
    host-source: /root
`

func (s *sdkStateSuite) SetUpTest(c *check.C) {
	var err error
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)

	s.fs = afero.NewMemMapFs()
	ctx := context.WithValue(context.TODO(), workshop.ContextProjectId, "projectId")
	s.ctx = context.WithValue(ctx, workshop.ContextUser, "testuser")

	s.backend, err = fakebackend.New(c.MkDir())
	c.Check(err, check.IsNil)

	s.project = workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "projectId",
	}
	s.restoreProjectId = testutil.FakeFunc(func() (string, error) { return s.project.ProjectId, nil }, &workshop.NewProjectId)
	s.backend.CreateOrLoadProject(s.ctx, s.project.Path)

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)

	workshop.ReplaceBackend(s.state, s.backend)

	/* empty task handler */
	s.runner.AddHandler("fake-task", fakeHandler, nil)

	s.repo = interfaces.NewRepository()
	mockIface(c, s.repo, &ifacetest.TestInterface{InterfaceName: "test-interface"})
	s.sdkmgr = sdkstate.New(s.state, s.runner, s.repo)

	/* error-provoking task handler */
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return ErrTrigger
	}
	s.runner.AddHandler("error-trigger", erroringHandler, nil)

	s.se = overlord.NewStateEngine(s.state)
	s.se.StartUp()
	s.se.AddManager(s.sdkmgr)
	s.se.AddManager(s.runner)

	s.installTime = time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC)
	s.restoreInstallTime = testutil.FakeFunc(func() time.Time { return s.installTime }, &workshop.InstallTimeNow)

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{
		{Name: "test", Channel: "latest/stable"},
		{Name: "test-broken", Channel: "latest/stable"},
	}}
	err = s.backend.LaunchWorkshop(s.ctx, wf)
	c.Assert(err, check.IsNil)

	s.mockTestSdk(c, "test", sdkYaml, 1)
}

func (s *sdkStateSuite) mockTestSdk(c *check.C, name, sdkYaml string, rev int64) {
	sdkPath := filepath.Join(dirs.WorkshopSdksDir, name, strconv.FormatInt(rev, 10), "meta", "sdk.yaml")
	fs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	err = fs.MkdirAll(filepath.Dir(sdkPath), 0755)
	c.Assert(err, check.IsNil)
	err = afero.WriteFile(fs, sdkPath, []byte(sdkYaml), 0644)
	c.Assert(err, check.IsNil)
}

func mockIface(c *check.C, repo *interfaces.Repository, iface interfaces.Interface) {
	err := repo.AddInterface(iface)
	c.Assert(err, check.IsNil)
}

func (s *sdkStateSuite) TearDownTest(c *check.C) {
	s.restoreProjectId()
	s.restoreInstallTime()
}

func (s *sdkStateSuite) TestDoInstallSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	newSdk := sdk.Setup{Name: "test-2", Channel: "latest/stable", Revision: sdk.Revision{N: 2}, InstallTime: &s.installTime}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	t1 := s.state.NewTask("install-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)

	err := s.backend.CreateVolume(s.ctx, newSdk.VolumeName())
	c.Assert(err, check.IsNil)
	defer s.backend.DeleteVolume(s.ctx, newSdk.VolumeName())

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(chg.Err(), check.IsNil)
	c.Check(chg.Status(), check.Equals, state.DoneStatus)

	c.Assert(s.backend.WorkshopVolumeMountPoints, check.HasLen, 1)
	c.Assert(s.backend.WorkshopVolumeMountPoints["test-2-2"], check.Equals, "/var/lib/workshop/sdk/test-2/2")
}

func (s *sdkStateSuite) TestDoInstallSdkSuccessWhenLocked(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	newSdk := sdk.Setup{Name: "test-2", Channel: "latest/stable", Revision: sdk.Revision{N: 2}, InstallTime: &s.installTime}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	t1 := s.state.NewTask("install-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)

	err := s.backend.CreateVolume(s.ctx, newSdk.VolumeName())
	c.Assert(err, check.IsNil)
	defer s.backend.DeleteVolume(s.ctx, newSdk.VolumeName())

	// Lock the sdk pretending there is another concurrent doInstall that has
	// already captured the lock.
	l, err := sdk.OpenLock(newSdk.Name)
	c.Assert(err, check.IsNil)
	c.Assert(l.Lock(), check.IsNil)
	defer l.Close()

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(chg.Err(), check.IsNil)
	c.Check(chg.Status(), check.Equals, state.DoneStatus)

	c.Assert(s.backend.WorkshopVolumeMountPoints, check.HasLen, 1)
	c.Assert(s.backend.WorkshopVolumeMountPoints["test-2-2"], check.Equals, "/var/lib/workshop/sdk/test-2/2")
}

func (s *sdkStateSuite) TestUndoInstallSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	newSdk := sdk.Setup{Name: "test-2", Channel: "latest/stable", Revision: sdk.Revision{N: 2}, InstallTime: &s.installTime}

	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)

	t1 := s.state.NewTask("install-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())
	t1.WaitFor(t)

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(t1)

	err := s.backend.CreateVolume(s.ctx, newSdk.VolumeName())
	c.Assert(err, check.IsNil)
	defer s.backend.DeleteVolume(s.ctx, newSdk.VolumeName())

	chg := s.state.NewChange("sample", "...")
	chg.Set("workshop", "ws")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("user", "testuser")
	chg.AddTask(t)
	chg.AddTask(t1)
	chg.AddTask(terr)

	setWorkshopProject("ws", s.project, t, t1, terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.UndoneStatus)

	c.Assert(s.backend.WorkshopVolumeMountPoints, check.HasLen, 0)
}

func (s *sdkStateSuite) TestDoInstallSystemSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	newSdk := sdk.Setup{Name: sdk.System.String(), Revision: sdk.Revision{N: -1}}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	t1 := s.state.NewTask("install-local-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(chg.Err(), check.IsNil)
	wfs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	info, err := wfs.Stat("/var/lib/workshop/sdk/system/x1/meta/sdk.yaml")
	c.Assert(err, check.IsNil)
	c.Assert(info.Mode().Perm(), check.Equals, fs.FileMode(0644))
}

func (s *sdkStateSuite) TestUndoInstallSystemSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	newSdk := sdk.Setup{Name: sdk.System.String()}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	t1 := s.state.NewTask("install-local-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(t1)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1, terr)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)
	chg.AddTask(terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Check(chg.Err(), check.NotNil)
	wfs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, err = wfs.Stat("/var/lib/workshop/sdk/system/x1")
	c.Assert(osutil.IsDirNotExist(err), check.Equals, true)
}

func (s *sdkStateSuite) TestDoLinkSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	testSdk := sdk.Setup{Name: "test", Channel: "latest/stable", Revision: sdk.Revision{N: 1}, InstallTime: &s.installTime}

	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", testSdk)
	t1 := s.state.NewTask("link-sdk", "test")
	t1.Set("sdk-retrieve-task", t.ID())

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)

	s.state.Unlock()
	s.se.Ensure()
	s.se.Wait()
	s.state.Lock()

	c.Check(chg.Err(), check.Equals, nil)
	props, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	info := props.Sdks
	c.Check(info["test"], check.DeepEquals, testSdk)

	sdkInfo, err := props.SdkInfo(s.ctx, info["test"].Name)
	c.Assert(err, check.IsNil)
	c.Assert(sdkInfo.Plugs, check.HasLen, 2)
	c.Assert(sdkInfo.Slots, check.HasLen, 0)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 2)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.NotNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.NotNil)
}

func (s *sdkStateSuite) TestDoLinkSdkFailedPolicyCheck(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()
	s.mockTestSdk(c, "test-broken", sdkYamlViolatesPolicy, 2)

	testSdk := sdk.Setup{Name: "test-broken", Channel: "latest/stable", Revision: sdk.Revision{N: 2}, InstallTime: &s.installTime}

	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", testSdk)
	t1 := s.state.NewTask("link-sdk", "test-broken")
	t1.Set("sdk-retrieve-task", t.ID())

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*installation denied by "slot" slot rule of interface "mount".*`)

	// not in the fs (removed)
	wfs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, err = wfs.Stat(sdk.SdkCurrentPath("test-broken"))
	c.Check(osutil.IsDirNotExist(err), check.Equals, true)

	// not in the SDK list (unlinked)
	wp, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, ok := wp.Sdks["test-broken"]
	c.Check(ok, check.Equals, false)

	// not in the repo (removed)
	c.Check(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Check(s.repo.Slots(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
}

func (s *sdkStateSuite) TestUndoLinkSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	newSdk := sdk.Info{Workshop: "ws", Name: "test", Revision: sdk.Revision{N: 1}}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	link := s.state.NewTask("link-sdk", "test")
	link.Set("sdk-retrieve-task", t.ID())

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(link)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, link, t)

	chg.Set("user", "testuser")
	chg.AddTask(link)
	chg.AddTask(t)
	chg.AddTask(terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	props, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, ok := props.Sdks["test"]
	c.Check(ok, check.Equals, false)
	c.Check(link.Status(), check.Equals, state.UndoneStatus)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.IsNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.IsNil)
}

func (s *sdkStateSuite) TestUndoLinkSdkRestorePreviousRev(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	// Install a second revision.
	s.mockTestSdk(c, "test", sdkYamlRev2, 2)
	wp, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)

	// Link the first revision to emulate that an SDK has already been linked to
	// the previous rev, so that undo can update records properly.
	err = wp.LinkSdk(s.ctx, sdk.Setup{Name: "test", Revision: sdk.Revision{N: 1}})
	c.Assert(err, check.IsNil)

	refreshedSdk := sdk.Info{Workshop: "ws", Name: "test", Revision: sdk.Revision{N: 2}}
	retrieve := s.state.NewTask("fake-task", "retrieve")
	retrieve.Set("sdk-setup", refreshedSdk)
	link := s.state.NewTask("link-sdk", "test")
	link.Set("sdk-retrieve-task", retrieve.ID())
	link.WaitFor(retrieve)

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(link)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, link, retrieve)

	chg.Set("user", "testuser")
	chg.AddTask(retrieve)
	chg.AddTask(link)
	chg.AddTask(terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	wp, err = s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)

	setup, ok := wp.Sdks["test"]
	c.Assert(ok, check.Equals, true)
	c.Assert(setup.Revision.N, check.Equals, 1)
	c.Assert(setup.RevisionSequence, check.HasLen, 0)

	fs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)

	curpath, err := fs.ReadLink(sdk.SdkCurrentPath("test"))
	c.Assert(err, check.IsNil)
	c.Assert(strings.HasSuffix(curpath, sdk.SdkRevPath("test", "1")), check.Equals, true)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
}

func (s *sdkStateSuite) TestLinkSdkBadInterfacesFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	newSdk := sdk.Info{Workshop: "ws", Name: "test", Revision: sdk.Revision{N: 1}}
	t := s.state.NewTask("fake-task", "retrieve")
	t.Set("sdk-setup", newSdk)
	link := s.state.NewTask("link-sdk", "test")
	link.Set("sdk-retrieve-task", t.ID())

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(link)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, link, t)

	chg.Set("user", "testuser")
	chg.AddTask(link)
	chg.AddTask(t)
	chg.AddTask(terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*"test" SDK has bad plugs or slots: plug, plug2 \(unknown interface "test-interface"\).*`)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.IsNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.IsNil)
}
