package workshopstate_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type managerSuite struct {
	state   *state.State
	backend workshop.Backend
	runner  *state.TaskRunner
	manager *workshopstate.WorkshopManager
	ctx     context.Context
	project workshop.Project

	restoreUserEnv func()
}

var _ = check.Suite(&managerSuite{})

func (s *managerSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.runner = state.NewTaskRunner(s.state)
	s.manager = workshopstate.New(s.state, s.runner)
	ctx := context.WithValue(context.TODO(), workshop.ContextUser, "testuser")
	s.restoreUserEnv = osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &user.User{HomeDir: c.MkDir()}, nil, nil
	})

	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)
	sdk.ReplaceStore(s.state, sdk.NewFakeStore())
}

func (s *managerSuite) TearDownTest(c *check.C) {
	s.restoreUserEnv()
}

func (s *managerSuite) TestAddHandlers(c *check.C) {
	workshopstate.New(s.state, s.runner)

	c.Assert(s.runner.KnownTaskKinds(), testutil.DeepUnsortedMatches, []string{
		"download-base",
		"create-workshop",
		"rebuild-workshop",
		"start-workshop",
		"stop-workshop",
		"remove-workshop",
		"mount-project",
		"create-workshop-storage",
		"remove-workshop-storage",
		"remove-workshop-stash",
		"stash-workshop",
		"create-state-storage",
		"remove-state-storage",
	})
}

func (s *managerSuite) launchWorkshopWithSDKs(c *check.C, ws string, sdks []workshop.SdkRecord) *workshop.Workshop {
	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	workshopFile := bytes.NewBuffer([]byte{})
	c.Assert(t.Execute(workshopFile, sdks), check.IsNil)

	path := workshop.Filepath(s.project.Path, ws)
	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, workshopFile.Bytes(), 0o644)
	c.Assert(err, check.IsNil)

	wf := workshop.File{Name: ws, Base: "ubuntu@22.04"}
	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}
	err = s.backend.LaunchOrRebuildWorkshop(s.ctx, &wf, image)
	c.Assert(err, check.IsNil)

	workshop, err := s.backend.Workshop(s.ctx, ws)
	c.Assert(err, check.IsNil)
	workshop.Running = true
	return workshop
}

func (s *managerSuite) TestRefreshManyOK(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	s.launchWorkshopWithSDKs(c, "test-2", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})

	_, err := s.manager.RefreshMany(s.ctx, s.project.ProjectId, []string{"test-1", "test-2"}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
}

func (s *managerSuite) TestRefreshRequireStatusReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	workshop2 := s.launchWorkshopWithSDKs(c, "test-2", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	err := s.backend.StopWorkshop(s.ctx, workshop2.Name, true)
	c.Assert(err, check.IsNil)

	_, err = s.manager.RefreshMany(s.ctx, s.project.ProjectId, []string{"test-1", "test-2"}, conflict.RefreshUpdate)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": workshop not running`)
}

func (s *managerSuite) TestRefreshRequireWorkshopExistence(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	workshop2 := s.launchWorkshopWithSDKs(c, "test-2", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	err := s.backend.RemoveWorkshop(s.ctx, workshop2.Name)
	c.Assert(err, check.IsNil)

	_, err = s.manager.RefreshMany(s.ctx, s.project.ProjectId, []string{"test-1", "test-2"}, conflict.RefreshUpdate)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": workshop not launched`)
}
