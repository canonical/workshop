//go:build integration
// +build integration

package workshopbackend_test

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshopbackend"
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
	f.username = "testuser"
	f.ctx = context.WithValue(context.Background(), workshopbackend.ContextUser, f.username)
	be := workshopbackend.LxdBackend{}
	f.client, _ = be.LxdClient(f.ctx)
	err := workshopbackend.InitProject(f.client, f.username)
	c.Assert(err, check.IsNil)
}

func (f *wsProject) TearDownTest(c *check.C) {
	cleanUpLxdProject(c, f.client, workshopbackend.LxdProjectName(f.username))
	cleanUpLxdProject(c, f.client, workshopbackend.LxdSystemProjectName(f.username))
}

func TestWorkshopBackendIntegration(t *testing.T) { check.TestingT(t) }

func (f *wsProject) TestLxdBackendCreateProjectNoWorkshopFiles(c *check.C) {
	// Setup
	be := workshopbackend.LxdBackend{}

	projectDir := c.MkDir()

	// Execute
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)

	// Validate
	c.Assert(prj, check.IsNil)
	c.Assert(err, check.Equals, workshopbackend.ErrNotAProject)
	c.Assert(workshopbackend.LockPath(projectDir), testutil.FileAbsent)
	projects, _ := be.Projects(f.ctx)
	c.Assert(projects[f.username], check.HasLen, 0)
}

func (f *wsProject) TestLxdBackendCreateProject(c *check.C) {
	// Setup
	be := workshopbackend.LxdBackend{}
	numCalls := 0
	ids := []string{"b8639dea", "d4352dea"}
	restore := testutil.FakeFunc(func() (string, error) { numCalls = numCalls + 1; return ids[numCalls-1], nil }, &workshopbackend.NewProjectId)
	defer restore()
	projectDir, projectDir2 := c.MkDir(), c.MkDir()

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	os.WriteFile(filepath.Join(projectDir2, ".workshop.test.yaml"), []byte(workshopMock), 0644)

	// Execute
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)

	// Validate
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(err, check.IsNil)

	lxdProject, _, _ := f.client.GetProject(workshopbackend.LxdProjectName("testuser"))
	c.Assert(workshopbackend.LockPath(projectDir), testutil.FilePresent)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir))

	// Execute
	prj, _, err = be.CreateOrLoadProject(f.ctx, projectDir2)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)

	// Validate
	lxdProject, _, _ = f.client.GetProject(workshopbackend.LxdProjectName("testuser"))
	c.Assert(workshopbackend.LockPath(projectDir2), testutil.FilePresent)
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"},{"path":"%s","id":"d4352dea"}]`, projectDir, projectDir2))
}

func (f *wsProject) TestLxdBackendLoadProject(c *check.C) {
	// Setup
	be := workshopbackend.LxdBackend{}
	restore := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshopbackend.NewProjectId)
	projectDir := c.MkDir()

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
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
	lxdProject, _, _ := f.client.GetProject(workshopbackend.LxdProjectName("testuser"))

	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir))
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryMoved(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was moved, but the project's settings were not
	// yet updated.
	be := workshopbackend.LxdBackend{}
	projectDir := c.MkDir()
	newDir := projectDir + "_moved"
	f.client.UpdateProject(workshopbackend.LxdProjectName(f.username),
		api.ProjectPut{
			Config: map[string]string{
				"user.workshop.projects": fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir),
			},
		}, "")

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	os.WriteFile(filepath.Join(projectDir, ".workshop.lock"), []byte("b8639dea"), 0644)
	err := os.Rename(projectDir, newDir)
	c.Assert(err, check.IsNil)

	prj, created, err := be.CreateOrLoadProject(f.ctx, newDir)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, newDir)
	c.Assert(err, check.IsNil)
	c.Assert(created, check.Equals, false)
	lxdProject, _, _ := f.client.GetProject(workshopbackend.LxdProjectName(f.username))
	c.Assert(lxdProject.Config["user.workshop.projects"], check.DeepEquals, fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, newDir))
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryCopied(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was copied, but the project's settings were not
	// yet updated.
	be := workshopbackend.LxdBackend{}
	restore := testutil.FakeFunc(func() (string, error) { return "abcdefgi", nil }, &workshopbackend.NewProjectId)
	defer restore()
	projectDir := c.MkDir()
	newDir := c.MkDir()
	f.client.UpdateProject(workshopbackend.LxdProjectName(f.username),
		api.ProjectPut{
			Config: map[string]string{
				"user.workshop.projects": fmt.Sprintf(`[{"path":"%s","id":"b8639dea"}]`, projectDir),
			},
		}, "")

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	os.WriteFile(filepath.Join(newDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	os.WriteFile(filepath.Join(projectDir, ".workshop.lock"), []byte("b8639dea"), 0644)
	os.WriteFile(filepath.Join(newDir, ".workshop.lock"), []byte("b8639dea"), 0644)

	prj, created, err := be.CreateOrLoadProject(f.ctx, newDir)
	c.Assert(err, check.IsNil)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, newDir)
	c.Assert(created, check.Equals, true)
	c.Assert(filepath.Join(newDir, ".workshop.lock"), testutil.FileEquals, "abcdefgi")
	lxdProject, _, _ := f.client.GetProject(workshopbackend.LxdProjectName(f.username))
	c.Assert(lxdProject.Config["user.workshop.projects"], check.Matches, fmt.Sprintf(`.*{"path":"%s","id":"abcdefgi"}.*`, newDir))
}

func (f *wsProject) TestLxdBackendListAvailableProjects(c *check.C) {
	// Setup
	be := workshopbackend.LxdBackend{}
	numCalls := 0
	ids := []string{"b8639dea", "d4352dea"}
	restore := testutil.FakeFunc(func() (string, error) { numCalls = numCalls + 1; return ids[numCalls-1], nil }, &workshopbackend.NewProjectId)
	defer restore()
	projectDir, projectDir2 := c.MkDir(), c.MkDir()

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	os.WriteFile(filepath.Join(projectDir2, ".workshop.test.yaml"), []byte(workshopMock), 0644)

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
	c.Assert(projects, check.DeepEquals, map[string][]*workshopbackend.Project{
		f.username: {
			{ProjectId: "b8639dea", Path: projectDir},
			{ProjectId: "d4352dea", Path: projectDir2},
		},
	})
	c.Assert(workshopbackend.LockPath(projectDir), testutil.FilePresent)
	c.Assert(workshopbackend.LockPath(projectDir2), testutil.FilePresent)
}

func (f *wsProject) TestLxdBackendLoadProjectDirectoryRemoved(c *check.C) {
	// Setup
	// We pre-create a project to emulate the scenario when
	// the directory was removed
	be := workshopbackend.LxdBackend{}
	projectDir := c.MkDir()

	err := os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	c.Assert(err, check.IsNil)
	_, _, err = be.CreateOrLoadProject(f.ctx, projectDir)
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
	be := workshopbackend.LxdBackend{}
	restoreId := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshopbackend.NewProjectId)
	defer restoreId()

	restoreLookup := testutil.FakeFunc(func(username string) (*user.User, error) {
		if username == f.username {
			return &user.User{Name: username}, nil
		}
		return nil, user.UnknownUserError("not found")
	}, &workshopbackend.LookupUsername)
	defer restoreLookup()

	projectDir := c.MkDir()

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(err, check.IsNil)

	// Execute (this time the project must be loaded)
	projects, err := be.Projects(context.Background())

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(projects, testutil.DeepUnsortedMatches, map[string][]*workshopbackend.Project{
		f.username: {{ProjectId: "b8639dea", Path: projectDir}},
	})
}

func (f *wsProject) TestLxdBackendLoadProjectAsDifferentUser(c *check.C) {
	// Setup
	be := workshopbackend.LxdBackend{}
	restore := testutil.FakeFunc(func() (string, error) { return "b8639dea", nil }, &workshopbackend.NewProjectId)
	projectDir := c.MkDir()

	os.WriteFile(filepath.Join(projectDir, ".workshop.test.yaml"), []byte(workshopMock), 0644)
	prj, _, err := be.CreateOrLoadProject(f.ctx, projectDir)
	c.Assert(prj, check.NotNil)
	c.Assert(prj.Path, check.Equals, projectDir)
	c.Assert(err, check.IsNil)
	// restore the new project id generator, we won't need it anymore
	// as we will be loading the project
	restore()

	// Execute (this time the project must be loaded)
	ctx := context.WithValue(f.ctx, workshopbackend.ContextUser, "anotheruser")
	prj, created, err := be.CreateOrLoadProject(ctx, projectDir)

	// Validate
	c.Assert(err, check.ErrorMatches, "project already exists")
	c.Assert(prj, check.IsNil)
	c.Assert(created, check.Equals, false)
}
