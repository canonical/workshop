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
	"github.com/canonical/workshop/internal/fsutil"
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
	backend        *fakebackend.FakeWorkshopBackend
	state          *state.State
	runner         *state.TaskRunner
	se             *overlord.StateEngine
	wrkmgr         *workshopstate.WorkshopManager
	ctx            context.Context
	project        workshop.Project
	user           *user.User
	restoreUserEnv func()
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

	err = os.WriteFile(path, []byte(yaml), 0o644)
	c.Assert(err, check.IsNil)
}

var ErrTrigger = errors.New("error out")

func (s *workshopHandlers) SetUpTest(c *check.C) {
	var err error
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser")

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
	s.restoreUserEnv = osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		if name != "testuser" {
			return nil, nil, user.UnknownUserError("not found")
		}
		return s.user, nil, nil
	})

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
	s.restoreUserEnv()
}

func (s *workshopHandlers) TestStopPeriodicProgressUpdate(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsFocal)
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, image)
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
	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}

	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, image)
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
		_ = s.se.Ensure()
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(t1.Status(), check.Equals, state.UndoneStatus)
	c.Assert(s.backend.StashedWorkshops[s.project.ProjectId], check.HasLen, 0)
	c.Assert(s.backend.Workshops[s.project.ProjectId], check.HasLen, 1)
	c.Assert(s.backend.Workshops[s.project.ProjectId]["ws"], check.NotNil)
}

func (s *workshopHandlers) TestRemoveWorkshop(c *check.C) {
	cacheDir := dirs.CacheDir
	dirs.SetCacheDir(c.MkDir())
	defer dirs.SetCacheDir(cacheDir)

	restoreUserLookup := osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser" {
			return nil, user.UnknownUserError("not found")
		}
		return s.user, nil
	})
	defer restoreUserLookup()

	s.state.Lock()
	defer s.state.Unlock()
	wFiles := []*workshop.File{{
		Name: "ws", Base: "ubuntu@20.04",
		Sdks: []workshop.SdkRecord{
			{Name: "test", Channel: "latest/stable"},
			{Name: "test2", Channel: "latest/stable"},
		},
	}, {
		Name: "another-ws", Base: "ubuntu@20.04",
		Sdks: []workshop.SdkRecord{
			{Name: "test", Channel: "latest/stable"},
			{Name: "test2", Channel: "latest/stable"},
		},
	}}

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)

	for _, wf := range wFiles {
		image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}
		err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, image)
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
		plugs := []string{"plug1", "plug2"}
		for _, p := range plugs {
			err = os.MkdirAll(filepath.Join(sdkMountData, p), 0o744)
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
	t1.Set("workshop-base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
	chg.Set("user", "testuser")
	chg.AddTask(t1)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		s.se.Ensure()
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
	t1.Set("workshop-base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
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

func (s *workshopHandlers) TestCreateWorkshopCleanup(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)

	reset := s.backend.SetWorkshopFsCallback(func(ctx context.Context, name string) (fsutil.Fs, error) {
		return fsutil.Fs{}, errors.New("fs is unavailable")
	})
	defer reset()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("create-workshop", "...")
	t1.Set("workshop-file", wsJammy)
	t1.Set("workshop-base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage123"})
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
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(fs is unavailable\)`)
	_, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
}

func (s *workshopHandlers) TestRebuildWorkshopNoCleanup(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.createWFile(c, "ws", wsJammy)

	reset := s.backend.SetWorkshopFsCallback(func(ctx context.Context, name string) (fsutil.Fs, error) {
		return fsutil.Fs{}, errors.New("fs is unavailable")
	})
	defer reset()

	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("rebuild-workshop", "...")
	t1.Set("workshop-file", wsJammy)
	image := workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage999"}
	t1.Set("workshop-base", image)
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
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*\(fs is unavailable\)`)
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
	t1.Set("workshop-base", workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "fakeimage1234"})
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
