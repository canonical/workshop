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
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type LxdBeTests struct {
	project workshop.Project
}

var _ = check.Suite(&LxdBeTests{})

func TestLxdBackendSuite(t *testing.T) { check.TestingT(t) }

func (s *LxdBeTests) SetUpTest(c *check.C) {
	dir := c.MkDir()
	s.project = workshop.Project{ProjectId: "42ws42ws", Path: dir}
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

var marshalledWorkshop = `name: test
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

func (f *LxdBeTests) TestDefaultWorkshopConfig(c *check.C) {
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
	c.Assert(cfg["raw.idmap"], check.Equals, "uid 1001 1000\ngid 1001 1000")
	c.Assert(cfg["security.nesting"], check.Equals, "true")
	c.Assert(cfg["user.workshop.project-id"], check.Equals, f.project.ProjectId)
	c.Assert(cfg["user.workshop.file"], check.Equals, marshalledWorkshop)
	c.Assert(cfg["user.workshop.format-revision"], check.Equals, b.FormatRevision().String())
	c.Assert(cfg["user.workshop.base-fingerprint"], check.Equals, "fakeimage12345")
}

// The cloud-init user-data must install and enable the systemd socket unit
// that SDK units connect to for LoadCredential secret resolution, along with
// its associated service unit.
func (f *LxdBeTests) TestWorkshopConfigSecretSocketUnits(c *check.C) {
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name: "test",
		Base: "ubuntu@22.04",
	}

	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1001", "1001", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)

	userData := cfg["cloud-init.user-data"]
	c.Check(userData, check.Matches, `(?s).*path: /etc/systemd/system/workshop-secret\.socket.*`)
	c.Check(userData, check.Matches, `(?s).*ListenStream=/var/lib/workshop/run/workshop\.socket\.secret.*`)
	c.Check(userData, check.Matches, `(?s).*Accept=yes.*`)
	c.Check(userData, check.Matches, `(?s).*path: /etc/systemd/system/workshop-secret@\.service.*`)
	c.Check(userData, check.Matches, `(?s).*ExecStart=/var/lib/workshop/bin/workshopctl get-secret --systemd.*`)
	c.Check(userData, check.Matches, `(?s).*StandardInput=socket.*`)
	c.Check(userData, check.Matches, `(?s).*StandardOutput=socket.*`)
	c.Check(userData, check.Matches, `(?s).*systemctl enable --now workshop-secret\.socket.*`)
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
