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

package lxdbackend_test

import (
	"crypto/sha3"
	"encoding/hex"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type LxdBeTests struct {
	project         workshop.Project
	workshopCtlPath string
}

var _ = check.Suite(&LxdBeTests{})

func TestLxdBackendSuite(t *testing.T) { check.TestingT(t) }

func (s *LxdBeTests) SetUpTest(c *check.C) {
	dir := c.MkDir()
	s.project = workshop.Project{ProjectId: "42ws42ws", Path: dir}

	// These tests don't require workshopctl, but the snapshot tests do, and
	// the basename of the test binary appears in cloud-init.user-data.
	s.workshopCtlPath = dirs.WorkshopCtlPath
	dirs.WorkshopCtlPath = filepath.Join(dir, "integration.test")
}

func (s *LxdBeTests) TearDownTest(c *check.C) {
	dirs.WorkshopCtlPath = s.workshopCtlPath
}

func (f *LxdBeTests) TestReadProjectsSuccess(c *check.C) {
	configData := `[{"path":"/home/dmitry/Work/ros-tutorials","id":"01ac7c0e"},{"path":"/home/dmitry/Work/ros2-tutorials","id":"47b66ebc"}]`

	projects, err := lxdbackend.ReadProjects([]byte(configData))
	c.Assert(err, check.IsNil)
	c.Assert(projects, testutil.DeepUnsortedMatches, []workshop.Project{
		{
			Path:      "/home/dmitry/Work/ros-tutorials",
			ProjectId: "01ac7c0e",
		},
		{
			Path:      "/home/dmitry/Work/ros2-tutorials",
			ProjectId: "47b66ebc",
		},
	})

	projects, err = lxdbackend.ReadProjects([]byte("[]"))
	c.Assert(err, check.IsNil)
	c.Assert(projects, check.NotNil)
	c.Assert(projects, check.HasLen, 0)

	projects, err = lxdbackend.ReadProjects(nil)
	c.Assert(err, check.IsNil)
	c.Assert(projects, check.NotNil)
	c.Assert(projects, check.HasLen, 0)
}

var containerFile = `name: test
base: ubuntu@22.04
sdks:
    - name: one
      channel: latest/stable
      plugs:
        one-plug:
            bind: two:two-plug
        one-plug-two:
            bind: two:two-plug
    - name: two
      channel: latest/edge
`

func (f *LxdBeTests) TestDefaultContainerConfig(c *check.C) {
	// Setup
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name: "test",
		Base: "ubuntu@22.04",
		Sdks: []workshop.SdkRecord{
			{Name: "one", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{
				"one-plug":     {Bind: &workshop.PlugRef{Sdk: "two", Name: "two-plug"}},
				"one-plug-two": {Bind: &workshop.PlugRef{Sdk: "two", Name: "two-plug"}},
			}},
			{Name: "two", Channel: "latest/edge"},
		},
	}

	// Execute
	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1001", "1001", file, b.FormatRevision(), "fakeimage12345")

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(cfg["cloud-init.user-data"], check.Not(testutil.Contains), "GRUB_CMDLINE_LINUX")
	c.Assert(cfg["raw.idmap"], check.Equals, "uid 1001 1000\ngid 1001 1000\n")
	c.Assert(cfg["raw.lxc"], check.Equals, "lxc.mount.entry = tmpfs tmp tmpfs defaults")
	c.Assert(cfg["security.nesting"], check.Equals, "true")
	c.Assert(cfg["user.workshop.project-id"], check.Equals, f.project.ProjectId)
	c.Assert(cfg["user.workshop.file"], check.Equals, containerFile)
	c.Assert(cfg["user.workshop.format-revision"], check.Equals, b.FormatRevision().String())
	c.Assert(cfg["user.workshop.base-fingerprint"], check.Equals, "fakeimage12345")

	// Check hash here so it's easier to update snapshot-format.yaml.
	digest := sha3.Sum384([]byte(cfg["cloud-init.user-data"]))
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "8cb63e0464ae87dca0a4b43e73fa6420b8b81e758a313b166732c886113af229acbaf83af34ce8b5fc1006772aae84f4")
	// Check for syntax errors (e.g. whitespace).
	var config map[string]any
	err = yaml.Unmarshal([]byte(cfg["cloud-init.user-data"]), &config)
	c.Assert(err, check.IsNil)
}

var vmFile = `name: test
base: ubuntu@22.04
confinement: virtual-machine
`

func (f *LxdBeTests) TestDefaultVMConfig(c *check.C) {
	// Setup
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name:        "test",
		Base:        "ubuntu@22.04",
		Confinement: workshop.ConfinementVirtualMachine,
	}

	// Execute
	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1002", "1002", file, b.FormatRevision(), "fakeimage12345")

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(cfg["cloud-init.user-data"], testutil.Contains, "GRUB_CMDLINE_LINUX")
	c.Assert(cfg["raw.idmap"], testutil.Contains, "uid 1002 1000\n")
	c.Assert(cfg["raw.idmap"], testutil.Contains, "gid 1002 1000\n")
	_, ok := cfg["raw.lxc"]
	c.Assert(ok, check.Equals, false)
	_, ok = cfg["security.nesting"]
	c.Assert(ok, check.Equals, false)
	c.Assert(cfg["user.workshop.project-id"], check.Equals, f.project.ProjectId)
	c.Assert(cfg["user.workshop.file"], check.Equals, vmFile)
	c.Assert(cfg["user.workshop.format-revision"], check.Equals, b.FormatRevision().String())
	c.Assert(cfg["user.workshop.base-fingerprint"], check.Equals, "fakeimage12345")

	// Check hash here so it's easier to update snapshot-format.yaml.
	digest := sha3.Sum384([]byte(cfg["cloud-init.user-data"]))
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "b2197fbcdb2a08fbb712f3f2a525dac06c50e025793b652ea7f4bc5d8456775cc69bcbc73a12d91e07d18baa5cba0c12")
	// Check for syntax errors (e.g. whitespace).
	var config map[string]any
	err = yaml.Unmarshal([]byte(cfg["cloud-init.user-data"]), &config)
	c.Assert(err, check.IsNil)
}

func (f *LxdBeTests) TestCheckLxdVersion(c *check.C) {
	err := lxdbackend.CheckServerVersion("6.8")
	c.Assert(err, check.IsNil)

	err = lxdbackend.CheckServerVersion("6.9")
	c.Assert(err, check.IsNil)

	err = lxdbackend.CheckServerVersion("6.7")
	c.Assert(err, check.ErrorMatches, `(?s).*LXD server version.*is not supported.*`)

	err = lxdbackend.CheckServerVersion("5.9")
	c.Assert(err, check.ErrorMatches, `(?s).*LXD server version.*is not supported.*`)

	err = lxdbackend.CheckServerVersion("unreachable")
	c.Assert(err, check.ErrorMatches, ".*cannot parse LXD server version.*")

	err = lxdbackend.CheckServerVersion("6")
	c.Assert(err, check.ErrorMatches, ".*cannot parse LXD server version.*")

	err = lxdbackend.CheckServerVersion("6.x")
	c.Assert(err, check.ErrorMatches, ".*cannot parse LXD server version.*")

	err = lxdbackend.CheckServerVersion("6.8.1")
	c.Assert(err, check.IsNil)

	err = lxdbackend.CheckServerVersion("6.7.9")
	c.Assert(err, check.ErrorMatches, `(?s).*LXD server version.*is not supported.*`)
}
