package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type workshopInit struct {
	BaseWorkshopSuite
}

var _ = check.Suite(&workshopInit{})

func (s *workshopInit) SetUpTest(c *check.C) {
	s.BaseWorkshopSuite.SetUpTest(c)
}

func (s *workshopInit) makeCmd(projectDir string) *CmdInit {
	return &CmdInit{
		root: &CmdRoot{cwd: projectDir},
		base: defaultBase,
	}
}

func (s *workshopInit) run(cmd *CmdInit, name string) error {
	return cmd.Run(nil, []string{name})
}

func (s *workshopInit) TestInitBaseUsage(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)

	bases := slices.Clone(workshop.SupportedBases)
	bases[len(bases)-1] = "and " + bases[len(bases)-1]
	line := fmt.Sprintf("\nThe supported bases are %s.\n", strings.Join(bases, ", "))

	help := cmd.Command().Long
	outdated := !strings.Contains(help, line)
	c.Check(outdated, check.Equals, false, check.Commentf(strings.TrimSpace(line)))
}

// --- Success cases ---

func (s *workshopInit) TestInitBasic(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go", "python"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	_, err = os.Stat(path)
	c.Assert(err, check.IsNil)

	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(content), "name: dev"), check.Equals, true)
	c.Assert(strings.Contains(string(content), "base: ubuntu@24.04"), check.Equals, true)
	c.Assert(strings.Contains(string(content), "go"), check.Equals, true)
	c.Assert(strings.Contains(string(content), "python"), check.Equals, true)

	c.Assert(s.stdout.String(), check.Matches, `"dev" workshop created at .*\n`)
}

func (s *workshopInit) TestInitEmptySdks(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	c.Check(path, testutil.FileEquals, `name: dev
base: ubuntu@24.04
`)
}

func (s *workshopInit) TestInitVM(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.vm = true

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	c.Check(path, testutil.FileEquals, `name: dev
base: ubuntu@24.04
confinement: virtual-machine
`)
}

func (s *workshopInit) TestInitWithSdkChannel(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go/1.26/stable", "python"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(string(content), check.Equals, `name: dev
base: ubuntu@24.04
sdks:
  - name: go
    channel: 1.26/stable
  - name: python
`)
}

func (s *workshopInit) TestInitWithCustomBase(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}
	cmd.base = "ubuntu@22.04"

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(content), "base: ubuntu@22.04"), check.Equals, true)
}

func (s *workshopInit) TestInitMultipleNamedWorkshops(c *check.C) {
	projectDir := c.MkDir()

	// Create first workshop.
	cmd1 := s.makeCmd(projectDir)
	cmd1.sdks = []string{"go"}
	err := s.run(cmd1, "dev")
	c.Assert(err, check.IsNil)

	s.ResetStdStreams()

	// Create second workshop.
	cmd2 := s.makeCmd(projectDir)
	cmd2.sdks = []string{"python"}
	err = s.run(cmd2, "test")
	c.Assert(err, check.IsNil)

	// Both should exist.
	_, err = os.Stat(workshop.Filepath(projectDir, "dev"))
	c.Assert(err, check.IsNil)
	_, err = os.Stat(workshop.Filepath(projectDir, "test"))
	c.Assert(err, check.IsNil)
}

func (s *workshopInit) TestInitSystemSdk(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"system", "go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(content), "system"), check.Equals, true)
}

func (s *workshopInit) TestInitTrySdk(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"try-uv", "go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(content), "try-uv"), check.Equals, true)
}

func (s *workshopInit) TestInitProjectSdk(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"project-uv", "go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(projectDir, "dev")
	content, err := os.ReadFile(path)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(content), "project-uv"), check.Equals, true)
}

// --- Failure: same name already exists ---

func (s *workshopInit) TestInitNameAlreadyExists(c *check.C) {
	projectDir := c.MkDir()

	// Pre-create a workshop with the same name.
	wsDir := filepath.Join(projectDir, workshop.Directory)
	c.Assert(os.MkdirAll(wsDir, 0755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(wsDir, "dev.yaml"), []byte("name: dev\nbase: ubuntu@24.04\n"), 0644), check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `cannot init: "dev" workshop already exists at ".*dev.yaml"`)
}

// --- Different name exists is OK ---

func (s *workshopInit) TestInitDifferentNameExists(c *check.C) {
	projectDir := c.MkDir()

	// Pre-create a different workshop.
	wsDir := filepath.Join(projectDir, workshop.Directory)
	c.Assert(os.MkdirAll(wsDir, 0755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(wsDir, "other.yaml"), []byte("name: other\nbase: ubuntu@24.04\n"), 0644), check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	// Both should exist.
	_, err = os.Stat(workshop.Filepath(projectDir, "other"))
	c.Assert(err, check.IsNil)
	_, err = os.Stat(workshop.Filepath(projectDir, "dev"))
	c.Assert(err, check.IsNil)
}

// --- Validation failures ---

func (s *workshopInit) TestInitInvalidBase(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}
	cmd.base = "foo@1.0"

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `base "foo@1.0" not supported`)
}

func (s *workshopInit) TestInitInvalidWorkshopName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, "99-bad")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "a workshop's name must.*")
}

func (s *workshopInit) TestInitWorkshopNameTooLong(c *check.C) {
	projectDir := c.MkDir()
	long := strings.Repeat("a", workshop.MAX_WORKSHOP_NAME_LENGTH+1)
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, long)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `workshop name ".*" too long`)
}

func (s *workshopInit) TestInitInvalidSdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"INVALID_SDK"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `invalid SDK name "INVALID_SDK"`)
}

func (s *workshopInit) TestInitEmptySdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{""}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `invalid SDK name ""`)
}

func (s *workshopInit) TestInitBlankSdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go", "  ", "uv"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `invalid SDK name ""`)
}

func (s *workshopInit) TestInitDuplicateSdk(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go", " go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `"go" SDK must only be included once`)
}

func (s *workshopInit) TestInitInvalidChannel(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go/latest/foo "}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `"go" SDK: invalid risk "foo" in channel "latest/foo"`)
}

func (s *workshopInit) TestInitReservedSdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"agent"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `"agent" is a reserved SDK name`)
}

func (s *workshopInit) TestInitReservedTrySdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"try-system"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `"system" is a reserved SDK name`)
}

func (s *workshopInit) TestInitReservedProjectSdkName(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"project-sketch"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `"sketch" is a reserved SDK name`)
}

func (s *workshopInit) TestInitWorkshopYamlExists(c *check.C) {
	projectDir := c.MkDir()
	err := os.WriteFile(filepath.Join(projectDir, "workshop.yaml"), []byte("name: old\n"), 0644)
	c.Assert(err, check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err = s.run(cmd, "dev")
	c.Assert(err, check.ErrorMatches, `cannot init: "workshop.yaml" already exists, move it to .workshop/old.yaml first to manage multiple workshops`)
}

func (s *workshopInit) TestInitDotWorkshopYamlExists(c *check.C) {
	projectDir := c.MkDir()
	err := os.WriteFile(filepath.Join(projectDir, ".workshop.yaml"), []byte("name: old\n"), 0644)
	c.Assert(err, check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err = s.run(cmd, "dev")
	c.Assert(err, check.ErrorMatches, `cannot init: ".workshop.yaml" already exists, move it to .workshop/old.yaml first to manage multiple workshops`)
}

func (s *workshopInit) TestInitWorkshopYamlExistsNoName(c *check.C) {
	projectDir := c.MkDir()
	err := os.WriteFile(filepath.Join(projectDir, "workshop.yaml"), []byte("base: ubuntu@24.04\n"), 0644)
	c.Assert(err, check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err = s.run(cmd, "dev")
	c.Assert(err, check.ErrorMatches, `cannot init: "workshop.yaml" already exists, move it to .workshop/ first to manage multiple workshops`)
}

// --- Corner cases ---

func (s *workshopInit) TestInitCreatesWorkshopDirectory(c *check.C) {
	projectDir := c.MkDir()
	wsDir := filepath.Join(projectDir, workshop.Directory)

	// Ensure .workshop/ doesn't exist yet.
	_, err := os.Stat(wsDir)
	c.Assert(os.IsNotExist(err), check.Equals, true)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err = s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	// .workshop/ directory should have been created.
	info, err := os.Stat(wsDir)
	c.Assert(err, check.IsNil)
	c.Assert(info.IsDir(), check.Equals, true)
}

func (s *workshopInit) TestInitDoesNotOverwriteExistingDirContents(c *check.C) {
	projectDir := c.MkDir()
	wsDir := filepath.Join(projectDir, workshop.Directory)
	c.Assert(os.MkdirAll(wsDir, 0755), check.IsNil)

	// Create an unrelated directory in .workshop/.
	otherFile := filepath.Join(wsDir, "tools")
	c.Assert(os.MkdirAll(otherFile, 0755), check.IsNil)

	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	// Other directory still exists.
	_, err = os.Stat(otherFile)
	c.Assert(err, check.IsNil)
}

func (s *workshopInit) TestInitOutputFormat(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	expectedPath := workshop.Filepath(projectDir, "dev")
	c.Assert(s.stdout.String(), check.Equals, "\"dev\" workshop created at "+expectedPath+"\n")
}

func (s *workshopInit) TestInitProjectFlagOverride(c *check.C) {
	projectDir := c.MkDir()
	otherDir := c.MkDir()

	// Use --project flag (simulated via root.prj).
	cmd := &CmdInit{
		root: &CmdRoot{cwd: otherDir, prj: projectDir},
		sdks: []string{"go"},
		base: defaultBase,
	}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	// File should be in projectDir, not otherDir.
	_, err = os.Stat(workshop.Filepath(projectDir, "dev"))
	c.Assert(err, check.IsNil)
	_, err = os.Stat(workshop.Filepath(otherDir, "dev"))
	c.Assert(os.IsNotExist(err), check.Equals, true)
}

func (s *workshopInit) TestInitYamlRoundTrip(c *check.C) {
	projectDir := c.MkDir()
	cmd := s.makeCmd(projectDir)
	cmd.sdks = []string{"go/1.26/stable", "system", "python"}

	err := s.run(cmd, "dev")
	c.Assert(err, check.IsNil)

	// The generated file should be parseable by the existing readWorkshop.
	project := workshop.Project{Path: projectDir, ProjectId: "test"}
	file, err := project.Workshop("dev")
	c.Assert(err, check.IsNil)
	c.Assert(file.Name, check.Equals, "dev")
	c.Assert(file.Base, check.Equals, "ubuntu@24.04")
	c.Assert(file.Sdks, check.HasLen, 3)
	c.Assert(file.Sdks[0].Name, check.Equals, "go")
	c.Assert(file.Sdks[0].Channel, check.Equals, "1.26/stable")
	c.Assert(file.Sdks[1].Name, check.Equals, "system")
	c.Assert(file.Sdks[2].Name, check.Equals, "python")
}
