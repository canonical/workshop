package workshopstate_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/progress"
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
	fs                afero.Fs
	backend           *fakebackend.FakeWorkshopBackend
	state             *state.State
	runner            *state.TaskRunner
	se                *overlord.StateEngine
	wrkmgr            *workshopstate.WorkshopManager
	ctx               context.Context
	project           workshop.Project
	homeDir           string
	lookupUserRestore func()
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
	s.fs = afero.NewMemMapFs()
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)

	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)
	s.homeDir = c.MkDir()
	s.lookupUserRestore = testutil.FakeFunc(func(name string) (*user.User, error) {
		u := &user.User{
			Name:     "testuser",
			Username: "testuser",
			Uid:      "1000",
			Gid:      "1000",
			HomeDir:  s.homeDir,
		}
		return u, nil
	}, &workshop.LookupUsername)

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)

	// empty task handler
	workshop.ReplaceBackend(s.state, s.backend)
	s.runner.AddHandler("fake-task", fakeHandler, nil)
	s.wrkmgr = workshopstate.New(s.state, s.runner)

	// error-provoking task handler
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return ErrTrigger
	}
	s.runner.AddHandler("error-trigger", erroringHandler, nil)

	s.se = overlord.NewStateEngine(s.state)
	s.se.AddManager(s.wrkmgr)
	s.se.AddManager(s.runner)
	err = s.se.StartUp()
	c.Check(err, check.IsNil)
}

func (s *workshopHandlers) TearDownTest(c *check.C) {
	s.lookupUserRestore()
}

func (s *workshopHandlers) TestStopPeriodicProgressUpdate(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsFocal)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	err := s.backend.LaunchWorkshop(s.ctx, wf)
	c.Check(err, check.IsNil)

	t1, err := s.wrkmgr.StopMany(s.ctx, []string{"ws"}, s.project.ProjectId, "1")
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
		s.se.Ensure()
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

	err := s.backend.LaunchWorkshop(s.ctx, wf)
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
		s.se.Ensure()
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
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{
		{Name: "test", Channel: "latest/stable"},
		{Name: "test2", Channel: "latest/stable"},
	}}

	err := s.backend.LaunchWorkshop(s.ctx, wf)
	c.Check(err, check.IsNil)

	// create content directories
	projectContent := filepath.Join(s.homeDir, ".local", "share", "workshop", "project", s.project.ProjectId, "mount")
	var plugs = []string{"ws_test_plug1.sdk", "ws_test_plug2.sdk", "another-ws_test_plug3.sdk"}
	for _, p := range plugs {
		err = os.MkdirAll(filepath.Join(projectContent, p), 0744)
		c.Assert(err, check.IsNil)
	}
	_, err = os.Create(filepath.Join(projectContent, "ws_test_plug4.sdk"))
	c.Assert(err, check.IsNil)

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("remove-workshop", "...")

	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	ws, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(ws, check.IsNil)
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)

	exist, _, _ := osutil.ExistsIsDir(filepath.Join(projectContent, plugs[0]))
	c.Assert(exist, check.Equals, false)

	exist, _, _ = osutil.ExistsIsDir(filepath.Join(projectContent, plugs[1]))
	c.Assert(exist, check.Equals, false)

	exist, _, _ = osutil.ExistsIsDir(filepath.Join(projectContent, plugs[2]))
	c.Assert(exist, check.Equals, true)

	exist, _, _ = osutil.ExistsIsDir(filepath.Join(projectContent, "ws_test_plug4.sdk"))
	c.Assert(exist, check.Equals, true)

}

func (s *workshopHandlers) TestCreateWorkshopNoWorkshopConfigurationFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.ErrorStatus)
	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*internal error: "ws" workshop configuration not found.*`)
}

func (s *workshopHandlers) TestCreateWorkshopWithSystemSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@22.04"}

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	t1.Set("workshop-file", wf)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
}

func (s *workshopHandlers) TestCreateWorkshopCleaunup(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@22.04"}

	reset := s.backend.SetWorkshopFsCallback(func(ctx context.Context, name string) (workshop.WorkshopFs, error) {
		return nil, errors.New("fs is unavailable")
	})
	defer reset()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	t1.Set("workshop-file", wf)
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
	_, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
}

func (s *workshopHandlers) TestDownloadBase(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.backend.DownloadBaseCallback = func(ctx context.Context, base string, report *progress.Reporter) error {
		report.Report("download finished", 100, 100)
		return nil
	}
	defer func() {
		s.backend.DownloadBaseCallback = nil
	}()

	s.createWFile(c, "ws", wsJammy)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@22.04"}

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("download-base", "...")
	t1.Set("workshop-base", wf.Base)
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	label, done, total := t1.Progress()
	c.Assert(label, check.Equals, "download finished")
	c.Assert(done, check.Equals, 100)
	c.Assert(total, check.Equals, 100)
}

func (s *workshopHandlers) TestAptCache(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	// Create volume for apt cache
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-apt-cache", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)

	volume := workshop.AptCacheVolumeName("ws", s.project.ProjectId)
	c.Assert(s.backend.WorkshopVolumes[volume], check.Equals, true)

	// Mount apt cache
	chg = s.state.NewChange("sample", "...")
	t1 = s.state.NewTask("mount-apt-cache", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)

	c.Assert(s.backend.WorkshopVolumeMountPoints[volume], check.Equals, dirs.AptCachePath)

	// Remove volume for apt cache
	chg = s.state.NewChange("sample", "...")
	t1 = s.state.NewTask("remove-apt-cache", "...")
	setWorkshopProject("ws", s.project, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.DoneStatus)

	c.Assert(s.backend.WorkshopVolumes[volume], check.Equals, false)
}
