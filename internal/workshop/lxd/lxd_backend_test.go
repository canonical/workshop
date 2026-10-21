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
	"fmt"
	"os/user"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
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

	// When updating this hash, please carefully consider whether the snapshot
	// format revision number needs to be bumped. If it isn't bumped, the
	// cloud-config changes won't apply to new workshops until the user
	// downloads a new base image or system SDK.
	digest := sha3.Sum384([]byte(cfg["cloud-init.user-data"]))
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "1967cb9cc4923e22695c921006c333aef19fa6f13246cb5969f756482e09d0819afd020e05612a2ad0dda1b6944ab1f6")
}

var vmFile = `name: test
base: ubuntu@22.04
runtime: lxd-vm
`

func (f *LxdBeTests) TestDefaultVMConfig(c *check.C) {
	// Setup
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name:    "test",
		Base:    "ubuntu@22.04",
		Runtime: workshop.RuntimeLXDVM,
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

	// When updating this hash, please carefully consider whether the snapshot
	// format revision number needs to be bumped. If it isn't bumped, the
	// cloud-config changes won't apply to new workshops until the user
	// downloads a new base image or system SDK.
	digest := sha3.Sum384([]byte(cfg["cloud-init.user-data"]))
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "17ef3fd3363854da056a2330777f9d3bf92e7fd3984c4e4c2fd3ada47f34b467603d29cd14c29af157054373682b7f82")
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

func (f *LxdBeTests) TestVMLimits(c *check.C) {
	// Check baseline limits.
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name:    "test",
		Base:    "ubuntu@22.04",
		Runtime: workshop.RuntimeLXDVM,
	}
	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.max_bus_ports"], check.Equals, "8")
	c.Check(cfg["limits.cpu"], check.Equals, "2")
	c.Check(cfg["limits.memory"], check.Equals, "4GiB")

	// Check per-SDK limits.
	for i := range 64 {
		regularSdks := i + 1
		file.Sdks = append(file.Sdks, workshop.SdkRecord{Name: fmt.Sprintf("sdk%v", regularSdks)})

		cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
		c.Assert(err, check.IsNil)

		maxBusPorts := min(8+4*regularSdks, 192)
		c.Check(cfg["limits.max_bus_ports"], check.Equals, fmt.Sprint(maxBusPorts), check.Commentf("regularSDKs = %v", regularSdks))

		vCPUs := 6
		switch {
		case maxBusPorts <= 64:
			vCPUs = 2
		case maxBusPorts <= 96:
			vCPUs = 3
		case maxBusPorts <= 128:
			vCPUs = 4
		case maxBusPorts <= 160:
			vCPUs = 5
		}
		c.Check(cfg["limits.cpu"], check.Equals, fmt.Sprint(vCPUs), check.Commentf("regularSDKs = %v", regularSdks))
	}

	// Check the system SDK is accounted for already.
	file.Sdks = file.Sdks[:14]
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.max_bus_ports"], check.Equals, "64")
	c.Check(cfg["limits.cpu"], check.Equals, "2")

	file.Sdks = append(file.Sdks, workshop.SdkRecord{Name: "system", Source: sdk.SystemSource})
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.max_bus_ports"], check.Equals, "64")
	c.Check(cfg["limits.cpu"], check.Equals, "2")
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

func (f *LxdBeTests) TestDefaultVMDevices(c *check.C) {
	usr := &user.User{Uid: "1000", Gid: "1000"}

	ctr := lxdbackend.DefaultDevices(usr, "pid", "w", workshop.RuntimeLXDContainer)
	c.Check(ctr["root"]["size"], check.Equals, "")

	vm := lxdbackend.DefaultDevices(usr, "pid", "w", workshop.RuntimeLXDVM)
	c.Check(vm["root"]["size"], check.Equals, "10GiB")
}
