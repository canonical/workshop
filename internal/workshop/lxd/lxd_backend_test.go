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
	"cmp"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os/user"
	"regexp"
	"testing"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
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
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "eecdd7227020df46220bdcb82a41d4a4b456b28623888fc3b58b7a057dcf058e00ffe163af0d51df74686169156c9806")
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
	c.Check(hex.EncodeToString(digest[:]), check.Equals, "10aacf882e4e6afa912c255edde2a6d462fbcb273500c42ee88f5ef1e41d08270cdde27e23b44580e878875ae986dfc7")
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
	c.Check(cfg["limits.memory"], check.Equals, lxdbackend.DefaultMemoryVM)

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

	// Check that more CPUs can be requested.
	file.CPUs = 5
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "6")

	file.CPUs = 7
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "7")
	file.CPUs = 0

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

func (f *LxdBeTests) TestCPUs(c *check.C) {
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name:    "test",
		Base:    "ubuntu@22.04",
		Runtime: workshop.RuntimeLXDContainer,
	}
	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "")

	file.CPUs = 3
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "3")

	file.Runtime = workshop.RuntimeLXDVM
	file.CPUs = 0
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "2")

	file.CPUs = 1
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "1")

	file.CPUs = 3
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.cpu"], check.Equals, "3")
}

func (f *LxdBeTests) TestMemory(c *check.C) {
	b := &lxdbackend.Backend{}
	file := &workshop.File{
		Name:    "test",
		Base:    "ubuntu@22.04",
		Runtime: workshop.RuntimeLXDContainer,
	}
	cfg, err := lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.memory"], check.Equals, "")

	file.Memory = "2GB"
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.memory"], check.Equals, "2GB")

	file.Memory = "1B"
	_, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Check(err, check.ErrorMatches, `invalid memory size "1B": must be at least 256MiB for containers`)

	file.Runtime = workshop.RuntimeLXDVM
	file.Memory = ""
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.memory"], check.Equals, "4GiB")

	file.Memory = "2GB"
	cfg, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Assert(err, check.IsNil)
	c.Check(cfg["limits.memory"], check.Equals, "2GB")

	file.Memory = "1kB"
	_, err = lxdbackend.DefaultConfig(b, f.project.ProjectId, "1000", "1000", file, b.FormatRevision(), "fakeimage12345")
	c.Check(err, check.ErrorMatches, `invalid memory size "1kB": must be at least 512MiB for virtual machines`)
}

func (f *LxdBeTests) TestDiskSize(c *check.C) {
	usr := &user.User{Uid: "1000", Gid: "1000"}

	file := &workshop.File{Name: "w", Base: "ubuntu@26.04", Runtime: workshop.RuntimeLXDContainer}
	devices, err := lxdbackend.DefaultDevices(usr, "pid", file)
	c.Assert(err, check.IsNil)
	c.Check(devices["root"]["size"], check.Equals, "")

	file.Disk = "20GiB"
	devices, err = lxdbackend.DefaultDevices(usr, "pid", file)
	c.Assert(err, check.IsNil)
	c.Check(devices["root"]["size"], check.Equals, "20GiB")

	// VMs start with the default size, and are expanded after snapshots.
	file.Runtime = workshop.RuntimeLXDVM
	devices, err = lxdbackend.DefaultDevices(usr, "pid", file)
	c.Assert(err, check.IsNil)
	c.Check(devices["root"]["size"], check.Equals, lxdbackend.DiskSizeVM)
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

func (f *LxdBeTests) TestCheckPoolSpace(c *check.C) {
	const MiB, GiB = lxdbackend.MiB, lxdbackend.GiB

	// Don't block anything if we don't know the pool size.
	c.Check(lxdbackend.CheckPoolSpace(0, 0, 1<<40), check.IsNil)

	c.Check(lxdbackend.CheckPoolSpace(1*GiB, 512*MiB, 512*MiB), check.IsNil)
	err := lxdbackend.CheckPoolSpace(1*GiB, 512*MiB, 512*MiB+1)
	c.Check(err, check.ErrorMatches, `(?s)storage pool "workshop" is 50% full \(512.0MiB available\).*`)
}

type reserveServer struct {
	lxd.InstanceServer
	exists     bool
	deleted    int
	deleteErr  error
	created    int
	createErr  error
	createdBy  string
	totalSpace uint64
	usedSpace  uint64
}

func (s *reserveServer) UseProject(string) lxd.InstanceServer { return s }

func (s *reserveServer) GetStoragePoolVolume(pool, volType, name string) (*api.StorageVolume, string, error) {
	if !s.exists {
		return nil, "", api.StatusErrorf(http.StatusNotFound, "Storage volume not found")
	}
	return &api.StorageVolume{Name: name}, "", nil
}

func (s *reserveServer) GetStoragePoolResources(pool string) (*api.ResourcesStoragePool, error) {
	res := &api.ResourcesStoragePool{}
	res.Space.Total = s.totalSpace
	res.Space.Used = s.usedSpace
	return res, nil
}

func (s *reserveServer) CreateStoragePoolVolume(pool string, volume api.StorageVolumesPost) (lxd.Operation, error) {
	s.created++
	if s.createErr != nil {
		// Simulate a concurrent caller winning the race.
		s.exists = s.createdBy == "other"
		return nil, s.createErr
	}
	s.exists = true
	return doneOperation{}, nil
}

func (s *reserveServer) DeleteStoragePoolVolume(pool, volType, name string) (lxd.Operation, error) {
	if pool != "workshop" || volType != "custom" || name != "workshop-reserve" {
		return nil, fmt.Errorf("unexpected volume %s/%s/%s", pool, volType, name)
	}
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if !s.exists {
		return nil, api.StatusErrorf(http.StatusNotFound, "Storage volume not found")
	}
	s.exists = false
	s.deleted++
	return doneOperation{}, nil
}

type doneOperation struct{ lxd.Operation }

func (doneOperation) Wait() error { return nil }

func (f *LxdBeTests) TestEnsurePoolSpace(c *check.C) {
	const GiB, MiB = lxdbackend.GiB, lxdbackend.MiB
	boom := errors.New("boom")

	for _, t := range []struct {
		summary   string
		driver    string
		exists    bool
		used      uint64
		createErr error
		createdBy string
		created   int
		err       string
	}{{
		summary: "already armed",
		exists:  true,
		used:    GiB,
	}, {
		summary: "arms the reserve",
		used:    GiB,
		created: 1,
	}, {
		summary: "no space for the reserve",
		used:    GiB + 1,
		err:     `(?s)storage pool "workshop" is .*`,
	}, {
		summary:   "armed concurrently",
		createErr: boom,
		createdBy: "other",
		created:   1,
	}, {
		summary:   "cannot arm",
		createErr: boom,
		created:   1,
		err:       `cannot create storage pool "workshop" reserve: boom`,
	}, {
		summary: "btrfs keeps a margin without a reserve",
		driver:  "btrfs",
		used:    GiB,
	}, {
		summary: "btrfs margin exceeded",
		driver:  "btrfs",
		used:    GiB + 1,
		err:     `(?s)storage pool "workshop" is .*`,
	}} {
		c.Logf("Summary: %s", t.summary)
		restore := lxdbackend.MockStoragePool("workshop", cmp.Or(t.driver, "zfs"))
		srv := &reserveServer{
			exists:     t.exists,
			createErr:  t.createErr,
			createdBy:  t.createdBy,
			totalSpace: 2*GiB + 256*MiB,
			usedSpace:  t.used,
		}
		err := lxdbackend.EnsurePoolSpace(srv, GiB)
		restore()
		if t.err == "" {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, t.err)
		}
		c.Check(srv.created, check.Equals, t.created)
	}
}

func (f *LxdBeTests) TestWithReserve(c *check.C) {
	full := errors.New("Failed writing backup.yaml: no space left on device")
	deleteErr := errors.New("cannot delete")

	for _, t := range []struct {
		summary   string
		exists    bool
		deleteErr error
		errs      []error
		calls     int
		deleted   int
		err       string
	}{{
		summary: "success",
		exists:  true,
		errs:    []error{nil},
		calls:   1,
	}, {
		summary: "other error",
		exists:  true,
		errs:    []error{errors.New("boom")},
		calls:   1,
		err:     "boom",
	}, {
		summary: "retry after releasing the reserve",
		exists:  true,
		errs:    []error{full, nil},
		calls:   2,
		deleted: 1,
	}, {
		summary: "retry after reserve released concurrently",
		errs:    []error{full, nil},
		calls:   2,
	}, {
		summary: "still full",
		errs:    []error{full, full},
		calls:   2,
		err:     regexp.QuoteMeta(full.Error()),
	}, {
		summary:   "cannot release the reserve",
		exists:    true,
		deleteErr: deleteErr,
		errs:      []error{full, full},
		calls:     2,
		err:       regexp.QuoteMeta(full.Error() + "\n" + deleteErr.Error()),
	}} {
		c.Logf("Summary: %s", t.summary)
		srv := &reserveServer{exists: t.exists, deleteErr: t.deleteErr}
		calls := 0
		err := lxdbackend.WithReserve(srv, func() error {
			err := t.errs[calls]
			calls++
			return err
		})
		if t.err == "" {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, t.err)
		}
		c.Check(calls, check.Equals, t.calls)
		c.Check(srv.deleted, check.Equals, t.deleted)
	}
}

func (f *LxdBeTests) TestCheckAvailableMemory(c *check.C) {
	const GiB = lxdbackend.GiB

	mi := &osutil.MemInfo{TotalUsable: 4 * GiB, Available: 2 * GiB}
	c.Check(lxdbackend.CheckAvailableMemory(mi, workshop.RuntimeLXDContainer), check.IsNil)
	c.Check(lxdbackend.CheckAvailableMemory(mi, workshop.RuntimeLXDVM), check.IsNil)

	mi.Available = GiB
	c.Check(lxdbackend.CheckAvailableMemory(mi, workshop.RuntimeLXDContainer), check.IsNil)
	err := lxdbackend.CheckAvailableMemory(mi, workshop.RuntimeLXDVM)
	c.Check(err, check.ErrorMatches, `(?s)host memory is 75% used \(1.0GiB available\).*`)
}
