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

	"github.com/canonical/workshop/internal/overlord/healthstate"
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
	project *workshop.Project

	lookupUserRestore func()
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
	s.lookupUserRestore = testutil.FakeFunc(func(name string) (*user.User, error) {
		u := &user.User{
			Name:     "testuser",
			Username: "testuser",
			Uid:      "1000",
			Gid:      "1000",
			HomeDir:  c.MkDir(),
		}
		return u, nil
	}, &workshop.LookupUsername)
	s.project, _, _ = s.backend.CreateOrLoadProject(ctx, c.MkDir())
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)
	sdk.ReplaceStore(s.state, sdk.NewFakeStore())
}

func (s *managerSuite) TearDownTest(c *check.C) {
	s.lookupUserRestore()
}

func (s *managerSuite) TestAddHandlers(c *check.C) {
	workshopstate.New(s.state, s.runner)

	c.Assert(s.runner.KnownTaskKinds(), testutil.DeepUnsortedMatches, []string{
		"download-base",
		"create-workshop",
		"start-workshop",
		"stop-workshop",
		"remove-workshop",
		"mount-project",
		"remove-workshop-stash",
		"stash-workshop",
		"create-state-storage",
		"remove-state-storage",
	})
}

func (s *managerSuite) launchWorkshopWithSDKs(c *check.C, ws string, sdks []workshop.SdkRecord) *workshop.Workshop {
	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	var workshopFile = bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	path := workshop.Filepath(s.project.Path, ws)

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, workshopFile.Bytes(), 0644)
	c.Assert(err, check.IsNil)

	wf := workshop.File{Name: ws, Base: "ubuntu@22.04"}
	err = s.backend.LaunchWorkshop(s.ctx, &wf)
	c.Assert(err, check.IsNil)

	workshop, err := s.backend.Workshop(s.ctx, ws)
	c.Assert(err, check.IsNil)
	workshop.Running = true
	return workshop
}

func (s *managerSuite) TestWorkshopHealthReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, "test", nil)
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.ReadyStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *managerSuite) TestWorkshopHealthStopped(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, "test", nil)
	c.Assert(s.backend.StopWorkshop(s.ctx, "test", true), check.IsNil)
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.StoppedStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *managerSuite) TestWorkshopHealthMissingProject(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	workshop := s.launchWorkshopWithSDKs(c, "test", nil)
	c.Assert(os.RemoveAll(s.project.Path), check.IsNil)
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.ErrorStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.Equals, "missing-project")

	warnings := s.state.AllWarnings()
	c.Check(warnings, check.HasLen, 1)
	warning := fmt.Sprintf(projectNotFound, s.project.Path)
	c.Check(warnings[0].String(), check.Equals, warning)
}

func (s *managerSuite) TestWorkshopHealthMissingFile(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	testWorkshop := s.launchWorkshopWithSDKs(c, "test", nil)
	c.Assert(os.RemoveAll(testWorkshop.Filepath()), check.IsNil)
	health := s.manager.WorkshopHealth(testWorkshop)

	c.Assert(health.Status, check.Equals, healthstate.ErrorStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.Equals, "missing-file")

	warnings := s.state.AllWarnings()
	c.Check(warnings, check.HasLen, 1)
	warning := fmt.Sprintf(fileNotFound, testWorkshop.Filepath())
	c.Check(warnings[0].String(), check.Equals, warning)
}

func (s *managerSuite) TestWorkshopHealthOperationInProgress(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("launch", "test")
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, "test", nil)
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.PendingStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *managerSuite) TestWorkshopHealthOperationInProgressWithNotes(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("refresh", "test")
	chg.SetStatus(state.WaitStatus)
	task := s.state.NewTask("create-workshop", "test task")
	task.Set("workshop", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, "test", nil)
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.PendingStatus)
	c.Check(health.SdkHealth, check.HasLen, 0)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.Equals, "wait-on-error")
}

func (s *managerSuite) TestWorkshopHealthSdkHealth(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("launch", "test")
	task := s.state.NewTask("run-hook", "test task")
	healthCheck := healthstate.HealthCheck{
		Sdk:         "test",
		CheckResult: healthstate.CheckWaiting,
		Message:     "still waiting",
		Code:        "how-much-longer",
	}
	task.Set("health", healthCheck)
	task.Set("workshop", "test")
	chg.Set("project-id", s.project.ProjectId)
	chg.AddTask(task)

	workshop := s.launchWorkshopWithSDKs(c, "test", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	health := s.manager.WorkshopHealth(workshop)

	c.Assert(health.Status, check.Equals, healthstate.PendingStatus)
	c.Assert(health.SdkHealth, check.HasLen, 1)
	c.Assert(health.SdkHealth["test"], check.DeepEquals, healthCheck)
	c.Check(health.Message, check.HasLen, 0)
	c.Check(health.Code, check.HasLen, 0)
}

func (s *managerSuite) TestRefreshManyOK(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	s.launchWorkshopWithSDKs(c, "test-2", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})

	_, err := s.manager.RefreshMany(s.ctx, []string{"test-1", "test-2"}, s.project.ProjectId)
	c.Assert(err, check.IsNil)
}

func (s *managerSuite) TestRefreshRequireStatusReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	workshop2 := s.launchWorkshopWithSDKs(c, "test-2", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})
	err := s.backend.StopWorkshop(s.ctx, workshop2.Name, true)
	c.Assert(err, check.IsNil)

	_, err = s.manager.RefreshMany(s.ctx, []string{"test-1", "test-2"}, s.project.ProjectId)
	c.Assert(err, check.ErrorMatches, refreshNotReady)
}

func (s *managerSuite) TestRefreshRequireWorkshopExistence(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	s.launchWorkshopWithSDKs(c, "test-1", []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}})

	_, err := s.manager.RefreshMany(s.ctx, []string{"test-1", "test-2"}, s.project.ProjectId)
	c.Assert(err, check.ErrorMatches, refreshNotFound)
}
