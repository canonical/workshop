//go:build integration
// +build integration

package lxdbackend_integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/workshop/lxd/tests/helper"
)

type wsProject struct {
	ctx      context.Context
	client   lxd.InstanceServer
	username string
}

var workshopMock = `name: test
base: ubuntu@22.04
`

var _ = check.Suite(&wsProject{})

func (f *wsProject) SetUpTest(c *check.C) {
	var err error
	f.username = "testuser"
	f.ctx = context.WithValue(context.Background(), workshop.ContextUser, f.username)
	be := lxdbackend.Backend{}
	f.client, err = be.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
}

func (f *wsProject) TearDownTest(c *check.C) {
	helper.CleanupLxdProject(c, f.client, "workshop."+f.username)
	helper.CleanupLxdProject(c, f.client, "workshop-stash."+f.username)
}

func TestWorkshopBackendIntegration(t *testing.T) { check.TestingT(t) }

func createWFile(c *check.C, projectDir, name, yaml string) {
	path := workshop.Filepath(projectDir, name)

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(yaml), 0644)
	c.Assert(err, check.IsNil)
}

func (f *wsProject) TestLxdBackendCreateProjectNoWorkshopFiles(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}

	projectDir := c.MkDir()

	// Execute
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)

	// Validate
	c.Assert(prj, check.IsNil)
	c.Assert(err, check.Equals, workshop.ErrNotProject)
	c.Assert(workshop.LockPath(projectDir), testutil.FileAbsent)
	projects, _ := be.Projects(f.ctx)
	c.Assert(projects[f.username], check.HasLen, 0)
}

func (f *wsProject) TestLxdBackendCreateProject(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}
	numCalls := 0
	ids := []string{"b8639dea", "d4352dea"}
	restore := testutil.FakeFunc(func() (string, error) { numCalls = numCalls + 1; return ids[numCalls-1], nil }, &workshop.NewProjectId)
	defer restore()
	projectDir, projectDir2 := c.MkDir(), c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	createWFile(c, projectDir2, "test", workshopMock)

	// Execute
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)

	// Validate
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(err, check.IsNil)

	lxdProject, _, _ := f.client.GetProject("workshop." + f.username)
	c.Assert(workshop.LockPath(projectDir), testutil.FilePresent)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir))

	// Execute
	prj, _, err = be.CreateOrLoadProject(f.ctx, projectDir2)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)

	// Validate
	lxdProject, _, _ = f.client.GetProject("workshop." + f.username)
	c.Assert(workshop.LockPath(projectDir2), testutil.FilePresent)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"},{"path":"%s","id":"d4352dea"}]`, projectDir, projectDir2))
}

func (f *wsProject) TestLxdBackendReconcileProjectIfNotRecovered(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}
	numCalls := 0
	ids := []string{"b8639dea"}
	restore := testutil.FakeFunc(func() (string, error) { numCalls = numCalls + 1; return ids[numCalls-1], nil }, &workshop.NewProjectId)
	defer restore()
	projectDir := c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)

	// Execute
	_, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(err, check.IsNil)

	os.RemoveAll(projectDir)

	// Validate
	projects, err := be.Projects(f.ctx)
	c.Assert(err, check.IsNil)
	c.Assert(projects[f.username], check.HasLen, 0)

	lxdProject, _, _ := f.client.GetProject("workshop." + f.username)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, `[]`)
}

func (f *wsProject) TestLxdBackendLoadProject(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}
	restore := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshop.NewProjectId)
	projectDir := c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(err, check.IsNil)
	// restore the new project id generator, we won't need it anymore
	// as we will be loading the project
	restore()

	// Execute (this time the project must be loaded)
	prj, created, err := be.CreateOrLoadProject(f.ctx, projectDir)

	// Validate
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(err, check.IsNil)
	c.Assert(created, check.Equals, false)
	lxdProject, _, _ := f.client.GetProject("workshop." + f.username)

	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir))
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryMoved(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was moved, but the project's settings were not
	// yet updated.
	be := lxdbackend.Backend{}
	projectDir := c.MkDir()
	newDir := projectDir + "_moved"
	err := f.client.UpdateProject("workshop."+f.username,
		api.ProjectPut{
			Config: map[string]string{
				"user.workshop.projects": fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir),
			},
		}, "")
	c.Assert(err, check.IsNil)

	createWFile(c, projectDir, "test", workshopMock)
	os.WriteFile(filepath.Join(projectDir, ".workshop.lock"), []byte("b8639dea"), 0644)
	err = os.Rename(projectDir, newDir)
	c.Assert(err, check.IsNil)

	prj, created, err := be.CreateOrLoadProject(f.ctx, newDir)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, newDir)
	c.Assert(err, check.IsNil)
	c.Assert(created, check.Equals, false)
	lxdProject, _, _ := f.client.GetProject("workshop." + f.username)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, newDir))
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryCopied(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was copied, but the project's settings were not
	// yet updated.
	be := lxdbackend.Backend{}
	restore := testutil.FakeFunc(func() (string, error) { return "abcdefgi", nil }, &workshop.NewProjectId)
	defer restore()
	projectDir := c.MkDir()
	newDir := c.MkDir()
	err := f.client.UpdateProject("workshop."+f.username,
		api.ProjectPut{
			Config: map[string]string{
				"user.workshop.projects": fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir),
			},
		}, "")
	c.Assert(err, check.IsNil)

	createWFile(c, projectDir, "test", workshopMock)
	createWFile(c, newDir, "test", workshopMock)
	os.WriteFile(filepath.Join(projectDir, ".workshop.lock"), []byte("b8639dea"), 0644)
	os.WriteFile(filepath.Join(newDir, ".workshop.lock"), []byte("b8639dea"), 0644)

	prj, created, err := be.CreateOrLoadProject(f.ctx, newDir)
	c.Assert(err, check.IsNil)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, newDir)
	c.Assert(created, check.Equals, true)
	c.Assert(filepath.Join(newDir, ".workshop.lock"), testutil.FileEquals, "abcdefgi")
	lxdProject, _, _ := f.client.GetProject("workshop." + f.username)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.Matches, fmt.Sprintf(`.*{"path":"%s","id":"abcdefgi"}.*`, newDir))
}

func (f *wsProject) TestLxdBackendListAvailableProjects(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}
	numCalls := 0
	ids := []string{"b8639dea", "d4352dea"}
	restore := testutil.FakeFunc(func() (string, error) { numCalls = numCalls + 1; return ids[numCalls-1], nil }, &workshop.NewProjectId)
	defer restore()
	projectDir, projectDir2 := c.MkDir(), c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	createWFile(c, projectDir2, "test", workshopMock)

	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)
	prj, _, err = be.CreateOrLoadProject(f.ctx, projectDir2)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)

	// Execute
	projects, err := be.Projects(f.ctx)

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(projects, check.DeepEquals, map[string][]workshop.Project{
		f.username: {
			{ProjectId: "b8639dea", Path: projectDir},
			{ProjectId: "d4352dea", Path: projectDir2},
		},
	})
	c.Assert(workshop.LockPath(projectDir), testutil.FilePresent)
	c.Assert(workshop.LockPath(projectDir2), testutil.FilePresent)
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryRemoved(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was removed
	be := lxdbackend.Backend{}
	projectDir := c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	_, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(err, check.IsNil)

	// Execute
	err = os.RemoveAll(projectDir)
	c.Assert(err, check.IsNil)
	projects, err := be.Projects(f.ctx)

	// Validate (if the directory does not exist, the project
	// needs to be removed from tracking)
	c.Assert(err, check.IsNil)
	c.Assert(projects[f.username], check.HasLen, 0)
}

func (f *wsProject) TestLxdBackendLoadProjectsAllUsers(c *check.C) {
	// Setup
	be := lxdbackend.Backend{}
	restoreId := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshop.NewProjectId)
	defer restoreId()

	restoreLookup := osutil.FakeUserLookup(func(username string) (*user.User, error) {
		if username == f.username {
			return &user.User{Username: username}, nil
		}
		return nil, user.UnknownUserError("not found")
	})
	defer restoreLookup()

	projectDir := c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)

	// Execute (this time the project must be loaded)
	projects, err := be.Projects(context.Background())

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(projects, testutil.DeepUnsortedMatches, map[string][]workshop.Project{
		f.username: {{ProjectId: "b8639dea", Path: projectDir}},
	})
}

func (f *wsProject) TestLxdBackendLoadProjectAsDifferentUser(c *check.C) {
	defer helper.CleanupLxdProject(c, f.client, "workshop.anotheruser")
	defer helper.CleanupLxdProject(c, f.client, "workshop-stash.anotheruser")

	// Setup
	be := lxdbackend.Backend{}
	restore := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshop.NewProjectId)
	projectDir := c.MkDir()

	createWFile(c, projectDir, "test", workshopMock)
	prj, created, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(created, check.Equals, true)
	c.Assert(err, check.IsNil)
	// restore the new project id generator, we won't need it anymore
	// as we will be loading the project
	restore()

	// Execute (this time the project must be loaded)
	ctx := context.WithValue(f.ctx, workshop.ContextUser, "anotheruser")
	prj, created, err = be.CreateOrLoadProject(ctx, projectDir)

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(created, check.Equals, true)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
}

func (f *wsProject) TestInitLxdProjectFails(c *check.C) {
	count := 0
	restore := osutil.FakeUserLookup(func(name string) (*user.User, error) {
		count++
		if name != "test@user" {
			return nil, errors.New("unexpected username!")
		}
		if count == 2 {
			return nil, errors.New("test error: unknown user")
		}
		return &user.User{
			Username: "test@user",
			Uid:      "1001",
		}, nil
	})
	defer restore()
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "test@user")

	be := lxdbackend.Backend{}
	_, err := be.LxdClient(ctx)
	c.Assert(err, check.ErrorMatches, "test error: unknown user")

	projects, err := f.client.GetProjectNames()
	c.Assert(err, check.IsNil)
	c.Assert(projects, check.Not(testutil.Contains), "workshop.test@user")
	c.Assert(projects, check.Not(testutil.Contains), "workshop-stash.test@user")
}

func (f *wsProject) TestPosixUsernameAsProjectSuffix(c *check.C) {
	restore := osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser0" {
			return nil, errors.New("unexpected username!")
		}
		return &user.User{
			Username: "testuser0",
			Uid:      "1001",
		}, nil
	})
	defer restore()
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "testuser0")

	be := lxdbackend.Backend{}
	p, err := be.LxdClient(ctx)
	c.Assert(err, check.IsNil)

	props, err := p.GetConnectionInfo()
	c.Assert(err, check.IsNil)
	c.Assert(props.Project, check.Equals, "workshop.testuser0")

	projects, err := p.GetProjectNames()
	c.Assert(err, check.IsNil)
	c.Assert(projects, testutil.Contains, "workshop.testuser0")
	c.Assert(projects, testutil.Contains, "workshop-stash.testuser0")
}

func (f *wsProject) TestNonPosixUsernameAsProjectSuffix(c *check.C) {
	restore := osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "ubuntu@canonical.com" {
			return nil, errors.New("unexpected username!")
		}
		return &user.User{
			Username: "ubuntu@canonical.com",
			Uid:      "1001",
		}, nil
	})
	defer restore()
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "ubuntu@canonical.com")

	be := lxdbackend.Backend{}
	p, err := be.LxdClient(ctx)
	c.Assert(err, check.IsNil)

	props, err := p.GetConnectionInfo()
	c.Assert(err, check.IsNil)
	c.Assert(props.Project, check.Equals, "workshop.1001")

	projects, err := p.GetProjectNames()
	c.Assert(err, check.IsNil)
	c.Assert(projects, testutil.Contains, "workshop.1001")
	c.Assert(projects, testutil.Contains, "workshop-stash.1001")
}
