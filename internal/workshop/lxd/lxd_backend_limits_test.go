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
	"os/user"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

const (
	mib = lxdbackend.MiB
	gib = lxdbackend.GiB
)

type limitsSuite struct{}

var _ = check.Suite(&limitsSuite{})

func (s *limitsSuite) TestDefaultVMLimits(c *check.C) {
	for _, t := range []struct {
		total  uint64
		cores  int
		cpu    int
		memory uint64
	}{
		{total: 4 * gib, cores: 2, cpu: 1, memory: 1 * gib},
		{total: 8 * gib, cores: 4, cpu: 2, memory: 2 * gib},
		{total: 16 * gib, cores: 8, cpu: 4, memory: 4 * gib},
		// Capped, so one workshop cannot claim a whole large machine.
		{total: 128 * gib, cores: 64, cpu: 8, memory: 8 * gib},
		// A single-core host must still yield one core, not zero.
		{total: 2 * gib, cores: 1, cpu: 1, memory: 1 * gib},
	} {
		cpu, memory := lxdbackend.DefaultVMLimits(t.total, t.cores)
		c.Check(cpu, check.Equals, t.cpu, check.Commentf("cores=%d", t.cores))
		c.Check(memory, check.Equals, t.memory, check.Commentf("total=%d", t.total))
	}
}

// Sizing must depend only on total memory, so the same workshop.yaml always
// produces the same VM.
func (s *limitsSuite) TestVMLimitsConfigIgnoresAvailableMemory(c *check.C) {
	dir := c.MkDir()
	write := func(avail string) func() {
		path := dir + "/meminfo"
		err := osutil.AtomicWriteFile(path,
			[]byte("MemTotal:       16777216 kB\nMemAvailable:   "+avail+" kB\n"), 0644, 0)
		c.Assert(err, check.IsNil)
		return osutil.MockProcMeminfo(path)
	}

	restore := write("512000")
	onBusy := lxdbackend.VMLimitsConfig()
	restore()

	restore = write("15728640")
	onIdle := lxdbackend.VMLimitsConfig()
	restore()

	c.Check(onBusy, check.DeepEquals, onIdle)
	c.Check(onBusy["limits.memory"], check.Equals, "4096MiB")
}

// If the host's memory cannot be read, fall back to LXD's defaults.
func (s *limitsSuite) TestVMLimitsConfigUnavailable(c *check.C) {
	restore := osutil.MockProcMeminfo(c.MkDir() + "/missing")
	defer restore()

	c.Check(lxdbackend.VMLimitsConfig(), check.IsNil)
}

func (s *limitsSuite) TestDefaultVMRootDiskSize(c *check.C) {
	for _, t := range []struct {
		pool, want uint64
	}{
		// A pool can be as small as storagePoolMinimalGiB, where LXD's
		// 10 GiB default would overcommit it and protect nothing.
		{pool: 5 * gib, want: 4 * gib},
		{pool: 16 * gib, want: 10 * gib},
		{pool: 30 * gib, want: 15 * gib},
		{pool: 500 * gib, want: 50 * gib},
		// Unknown pool size: leave LXD's default alone.
		{pool: 0, want: 0},
	} {
		c.Check(lxdbackend.DefaultVMRootDiskSize(t.pool), check.Equals, t.want,
			check.Commentf("pool=%d", t.pool))
	}
}

// The quota must always bite before the pool fills.
func (s *limitsSuite) TestVMRootDiskLeavesPoolHeadroom(c *check.C) {
	for _, pool := range []uint64{2 * gib, 5 * gib, 11 * gib, 30 * gib, 500 * gib} {
		size := lxdbackend.DefaultVMRootDiskSize(pool)
		c.Check(size < pool, check.Equals, true, check.Commentf("pool=%d size=%d", pool, size))
	}
}

func (s *limitsSuite) TestRootDiskQuotaOnlyAppliesToVMs(c *check.C) {
	usr := &user.User{Uid: "1000", Gid: "1000"}

	ctr := lxdbackend.DefaultDevices(usr, "pid", "w", workshop.ConfinementContainer, 10*gib)
	_, hasSize := ctr["root"]["size"]
	c.Check(hasSize, check.Equals, false)

	vm := lxdbackend.DefaultDevices(usr, "pid", "w", workshop.ConfinementVirtualMachine, 10*gib)
	c.Check(vm["root"]["size"], check.Equals, "10737418240")

	// Zero means "leave LXD's default alone".
	vm = lxdbackend.DefaultDevices(usr, "pid", "w", workshop.ConfinementVirtualMachine, 0)
	_, hasSize = vm["root"]["size"]
	c.Check(hasSize, check.Equals, false)
}
