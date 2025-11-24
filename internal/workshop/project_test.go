package workshop_test

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type projectSuite struct{}

var _ = check.Suite(&projectSuite{})

var w = `name: %s
base: ubuntu@22.04
`

var wb = `name: wb
base: ubuntu@22.04
connections:
  - plug:
	- system:plug
`

func createWorkshop(dir, name string) error {
	return os.WriteFile(filepath.Join(dir, workshop.Filename(name)), []byte(fmt.Sprintf(w, name)), 0o644)
}

func (p *projectSuite) TestSomeWorkshopFilesBroken(c *check.C) {
	d := c.MkDir()
	project := &workshop.Project{Path: d, ProjectId: "42424242"}

	w := filepath.Join(d, workshop.Directory)
	c.Assert(os.MkdirAll(w, os.ModePerm), check.IsNil)

	c.Assert(createWorkshop(w, "w1"), check.IsNil)
	c.Assert(createWorkshop(w, "w2"), check.IsNil)
	c.Assert(os.MkdirAll(filepath.Join(w, "test-dir.yaml"), 0o755), check.IsNil)
	// broken workshop
	c.Assert(os.WriteFile(filepath.Join(w, "wb.yaml"), []byte(wb), 0o644), check.IsNil)
	// no match with the filename pattern
	c.Assert(os.WriteFile(filepath.Join(w, "test-file.yml"), []byte{}, 0o644), check.IsNil)
	fls, err := project.ReadWorkshops()
	c.Assert(err, check.IsNil)
	c.Assert(fls, check.HasLen, 3)
	c.Assert(fls, testutil.DeepUnsortedMatches, map[string]string{"w1": filepath.Join(w, "w1.yaml"), "w2": filepath.Join(w, "w2.yaml"), "wb": filepath.Join(w, "wb.yaml")})
}

func (p *projectSuite) TestMixedWorkshopFileConventions(c *check.C) {
	d := c.MkDir()
	project := &workshop.Project{Path: d, ProjectId: "42424242"}

	w := filepath.Join(d, workshop.Directory)
	c.Assert(os.MkdirAll(w, os.ModePerm), check.IsNil)

	c.Assert(createWorkshop(w, "ws"), check.IsNil)
	c.Assert(createWorkshop(d, "workshop"), check.IsNil)
	fls, err := project.ReadWorkshops()
	c.Assert(fls, check.IsNil)
	path := filepath.Join(d, "workshop.yaml")
	message := fmt.Sprintf(`multiple workshops found, but %q not in ".workshop" subdirectory`, path)
	c.Assert(err, check.ErrorMatches, message)
}

func (p *projectSuite) TestSingleWorkshopFileBroken(c *check.C) {
	d := c.MkDir()
	project := &workshop.Project{Path: d, ProjectId: "42424242"}

	c.Assert(os.WriteFile(filepath.Join(d, ".workshop.yaml"), []byte(wb), 0o644), check.IsNil)
	fls, err := project.ReadWorkshops()
	c.Assert(fls, check.IsNil)
	c.Assert(err, check.NotNil)
}

func (p *projectSuite) TestTrackRelativePath(c *check.C) {
	tracker := workshop.ProjectTracker{}
	_, _, err := tracker.Track("relative/path")
	c.Assert(err, check.ErrorMatches, "absolute project path must be used")
}

func (p *projectSuite) TestTrackNewProject(c *check.C) {
	tracker := workshop.ProjectTracker{}

	// Single-workshop project
	d := c.MkDir()
	_, err := os.Create(filepath.Join(d, ".workshop.yaml"))
	c.Assert(err, check.IsNil)

	project, result, err := tracker.Track(d)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, d)
	c.Check(project.ProjectId, check.Not(check.Equals), "")
	c.Check(tracker.Projects, check.HasLen, 1)
	c.Assert(workshop.LockPath(d), testutil.FilePresent)

	// Multi-workshop project
	d = c.MkDir()
	c.Assert(os.Mkdir(filepath.Join(d, ".workshop"), os.ModePerm), check.IsNil)
	_, err = os.Create(filepath.Join(d, ".workshop", "dev.yaml"))
	c.Assert(err, check.IsNil)
	_, err = os.Create(filepath.Join(d, ".workshop", "prod.yaml"))
	c.Assert(err, check.IsNil)

	project, result, err = tracker.Track(d)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, d)
	c.Check(project.ProjectId, check.Not(check.Equals), "")
	c.Check(tracker.Projects, check.HasLen, 2)
	c.Assert(workshop.LockPath(d), testutil.FilePresent)

	// Mixed convention project
	d = c.MkDir()
	_, err = os.Create(filepath.Join(d, ".workshop.yaml"))
	c.Assert(err, check.IsNil)
	c.Assert(os.Mkdir(filepath.Join(d, ".workshop"), os.ModePerm), check.IsNil)
	_, err = os.Create(filepath.Join(d, ".workshop", "dev.yaml"))
	c.Assert(err, check.IsNil)

	project, result, err = tracker.Track(d)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, d)
	c.Check(project.ProjectId, check.Not(check.Equals), "")
	c.Check(tracker.Projects, check.HasLen, 3)
	c.Assert(workshop.LockPath(d), testutil.FilePresent)

	// Invalid project
	d = c.MkDir()
	path := filepath.Join(d, ".workshop", "prod.yaml")
	c.Assert(os.MkdirAll(path, os.ModePerm), check.IsNil)

	_, _, err = tracker.Track(d)
	c.Assert(err, check.ErrorMatches, `not a project \(no workshop files found\)`)
	c.Assert(tracker.Projects, check.HasLen, 3)

	// Workshop named "workshop"
	d = c.MkDir()
	subdir := filepath.Join(d, ".workshop")
	c.Assert(os.Mkdir(subdir, os.ModePerm), check.IsNil)
	_, err = os.Create(filepath.Join(subdir, "workshop.yaml"))
	c.Assert(err, check.IsNil)

	project, result, err = tracker.Track(subdir)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, d)
	c.Check(project.ProjectId, check.Not(check.Equals), "")
	c.Check(tracker.Projects, check.HasLen, 4)
	c.Assert(workshop.LockPath(d), testutil.FilePresent)
}

func (p *projectSuite) TestTrackProjectSubDirectory(c *check.C) {
	root := c.MkDir()
	cases := []struct {
		project   string
		lockFile  bool
		cwd       string
		isSymlink bool
		expected  string
	}{
		// nested directory
		{"/home/user", true, "/home/user/nested", false, "/home/user"},

		// nested directory
		{"/home/user", true, "/home/user/test/very/deeply", false, "/home/user"},

		// same level
		{"/home/user/same", true, "/home/user/same", false, "/home/user/same"},
		// same level, symlink
		{"/home/user/same", true, "/home/user/samelink", true, "/home/user/same"},

		// different cwd
		{"/home/user/different", true, "/home", false, "/home"},

		// project is in root
		{"/", true, "/home/user/notroot", false, ""},

		// .lock does not exist
		{"/home/user/nolock", false, "/home/user/test/nolock", false, "/home/user/test/nolock"},

		// path is unclean (lock exists)
		{"/home/user/unclean", true, "/home/user/unclean/", false, "/home/user/unclean"},

		// path is unclean (no lock)
		{"/home/user/unclean", false, "/home/user/unclean/", false, "/home/user/unclean"},

		// path is unclean (no lock, symlink)
		{"/home/user/projectdir", false, "/home/user/symlinktest/", true, "/home/user/projectdir"},
	}

	for _, i := range cases {
		c.Assert(os.MkdirAll(filepath.Join(root, i.project), 0o755), check.IsNil)
		if i.lockFile == true {
			project := &workshop.Project{Path: filepath.Join(root, i.project), ProjectId: "42424242"}
			c.Assert(workshop.UpdateLock(project), check.IsNil)
		}
		if i.isSymlink == true {
			err := os.Symlink(filepath.Join(root, i.project), filepath.Join(root, i.cwd))
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(os.MkdirAll(filepath.Join(root, i.cwd), 0o755), check.IsNil)
		}
		_, err := os.Create(filepath.Join(root, i.expected, "workshop.yaml"))
		c.Assert(err, check.IsNil)

		tracker := workshop.ProjectTracker{}
		// note: no filepath.join here as it calls Clean on exist for the path
		// the data must come unclean for the Track input and the test
		// must ensure it returns a clean one on every condition
		project, result, err := tracker.Track(fmt.Sprintf("%s%s", root, i.cwd))
		c.Assert(err, check.IsNil)
		c.Check(result, check.Equals, workshop.ProjectAdded)
		c.Check(project.Path, check.Equals, fmt.Sprintf("%s%s", root, i.expected))

		os.RemoveAll(filepath.Join(root, i.project))
		os.RemoveAll(filepath.Join(root, i.cwd))
	}
}

func (p *projectSuite) TestTrackNestedProjects(c *check.C) {
	outer := c.MkDir()
	_, err := os.Create(filepath.Join(outer, "workshop.yaml"))
	c.Assert(err, check.IsNil)
	project := &workshop.Project{Path: outer, ProjectId: "42424242"}
	c.Assert(workshop.UpdateLock(project), check.IsNil)

	inner := filepath.Join(outer, "vendor", "product")
	c.Assert(os.MkdirAll(inner, os.ModePerm), check.IsNil)
	_, err = os.Create(filepath.Join(inner, "workshop.yaml"))
	c.Assert(err, check.IsNil)

	tracker := workshop.ProjectTracker{}
	project, result, err := tracker.Track(inner)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, inner)
}

func (p *projectSuite) TestTrackExistingProject(c *check.C) {
	d := c.MkDir()
	original := filepath.Join(d, "original")
	c.Assert(os.Mkdir(original, os.ModePerm), check.IsNil)

	first := workshop.Project{Path: original, ProjectId: "42424242"}
	c.Assert(workshop.UpdateLock(&first), check.IsNil)

	projects := []workshop.Project{{Path: original, ProjectId: "42424242"}}
	tracker := workshop.ProjectTracker{Projects: projects}

	// Existing project with .lock file.
	project, result, err := tracker.Track(original)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectFound)
	c.Check(project, check.DeepEquals, &first)
	c.Check(tracker.Projects, check.DeepEquals, []workshop.Project{first})

	// Copy of existing project with .lock file.
	copied := filepath.Join(d, "copied")
	c.Assert(os.CopyFS(copied, os.DirFS(original)), check.IsNil)

	project, result, err = tracker.Track(copied)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project.Path, check.Equals, copied)
	c.Check(project.ProjectId, check.Not(check.Equals), "")
	c.Check(project.ProjectId, check.Not(check.Equals), first.ProjectId)
	second := workshop.Project{Path: copied, ProjectId: project.ProjectId}
	c.Check(tracker.Projects, testutil.DeepUnsortedMatches, []workshop.Project{first, second})

	// Relocation of existing project with .lock file.
	moved := filepath.Join(d, "moved")
	c.Assert(os.Rename(original, moved), check.IsNil)
	first.Path = moved

	project, result, err = tracker.Track(moved)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectMoved)
	c.Check(project, check.DeepEquals, &first)
	c.Check(tracker.Projects, testutil.DeepUnsortedMatches, []workshop.Project{first, second})
}

func (p *projectSuite) TestTrackRecoversProjectIds(c *check.C) {
	d := c.MkDir()
	expected := workshop.Project{Path: d, ProjectId: "42424242"}

	projects := []workshop.Project{{Path: d, ProjectId: "42424242"}}
	tracker := workshop.ProjectTracker{Projects: projects}

	// Existing project without .lock file.
	project, result, err := tracker.Track(d)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectFound)
	c.Check(project, check.DeepEquals, &expected)
	c.Check(tracker.Projects, check.DeepEquals, []workshop.Project{expected})
	c.Assert(workshop.LockPath(d), testutil.FilePresent)

	// Unknown directory with .lock file.
	tracker = workshop.ProjectTracker{}

	_, _, err = tracker.Track(d)
	c.Assert(err, check.ErrorMatches, `not a project \(no workshop files found\)`)
	c.Assert(tracker.Projects, check.HasLen, 0)

	// Unknown project with .lock file.
	_, err = os.Create(filepath.Join(d, "workshop.yaml"))
	c.Assert(err, check.IsNil)

	project, result, err = tracker.Track(d)
	c.Assert(err, check.IsNil)
	c.Check(result, check.Equals, workshop.ProjectAdded)
	c.Check(project, check.DeepEquals, &expected)
	c.Check(tracker.Projects, check.DeepEquals, []workshop.Project{expected})
}
