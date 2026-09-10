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
	"errors"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type capacitySuite struct{}

var _ = check.Suite(&capacitySuite{})

// Every estimate must exceed what was actually measured on a zfs pool.
func (s *capacitySuite) TestLaunchSpaceCoversMeasuredImages(c *check.C) {
	for _, t := range []struct {
		compressed, measured int64
		vm                   bool
	}{
		// ubuntu 22.04, 24.04, 26.04 containers, then the same as VMs.
		{compressed: 508199731, measured: 741592576},
		{compressed: 279143546, measured: 524066304},
		{compressed: 318322114, measured: 589535232},
		{compressed: 703470571, measured: 910726656, vm: true},
		{compressed: 624818913, measured: 884227584, vm: true},
		{compressed: 863284428, measured: 1150876160, vm: true},
	} {
		got, unpacks := lxdbackend.LaunchSpace(lxdbackend.LaunchSource{ImageSize: t.compressed, VM: t.vm})
		c.Check(unpacks, check.Equals, true)
		c.Check(got > uint64(t.measured), check.Equals, true,
			check.Commentf("compressed=%d vm=%v got=%d measured=%d", t.compressed, t.vm, got, t.measured))
	}
}

// An image that has never been unpacked here costs far more than a clone, and
// one that has not even been downloaded has to be guessed at generously.
func (s *capacitySuite) TestLaunchSpace(c *check.C) {
	clone, unpacks := lxdbackend.LaunchSpace(lxdbackend.LaunchSource{Clone: true})
	c.Check(clone, check.Equals, uint64(lxdbackend.CloneFootprint))
	c.Check(unpacks, check.Equals, false)

	unknown, unpacks := lxdbackend.LaunchSpace(lxdbackend.LaunchSource{})
	c.Check(unpacks, check.Equals, true)
	c.Check(unknown > clone, check.Equals, true)

	unknownVM, _ := lxdbackend.LaunchSpace(lxdbackend.LaunchSource{VM: true})
	c.Check(unknownVM > unknown, check.Equals, true)
}

// The point of sizing each launch: the same free space is fine for a clone and
// insufficient for an unpack.
func (s *capacitySuite) TestCheckPoolSpace(c *check.C) {
	total := uint64(lxdbackend.PoolReserve + 600*mib)

	c.Check(lxdbackend.CheckPoolSpace(total, 0, lxdbackend.LaunchSource{Clone: true}), check.IsNil)

	err := lxdbackend.CheckPoolSpace(total, 0, lxdbackend.LaunchSource{ImageSize: 400 * mib})
	c.Assert(err, check.NotNil)
	c.Check(errors.Is(err, lxdbackend.ErrInsufficientResources), check.Equals, true)
	c.Check(err, check.ErrorMatches, ".*has not been used before.*")
}

// The reserve must be kept free even when the footprint itself would fit.
func (s *capacitySuite) TestCheckPoolSpaceKeepsReserve(c *check.C) {
	clone := lxdbackend.LaunchSource{Clone: true}

	err := lxdbackend.CheckPoolSpace(lxdbackend.CloneFootprint, 0, clone)
	c.Check(errors.Is(err, lxdbackend.ErrInsufficientResources), check.Equals, true)

	c.Check(lxdbackend.CheckPoolSpace(lxdbackend.CloneFootprint+lxdbackend.PoolReserve, 0, clone), check.IsNil)
}

// A pool whose usage LXD cannot report must not block launches.
func (s *capacitySuite) TestCheckPoolSpaceUnknownTotal(c *check.C) {
	c.Check(lxdbackend.CheckPoolSpace(0, 0, lxdbackend.LaunchSource{}), check.IsNil)
}

func (s *capacitySuite) TestCheckAvailableMemory(c *check.C) {
	mi := &osutil.MemInfo{Total: 8 * gib, Available: 4 * gib, SwapTotal: 2 * gib}
	c.Check(lxdbackend.CheckAvailableMemory(mi, workshop.ConfinementContainer), check.IsNil)

	// Measured cliff: 39 MiB available OOM-killed three processes.
	mi = &osutil.MemInfo{Total: 1443 * mib, Available: 39 * mib}
	err := lxdbackend.CheckAvailableMemory(mi, workshop.ConfinementContainer)
	c.Assert(errors.Is(err, lxdbackend.ErrInsufficientResources), check.Equals, true)
	c.Check(err, check.ErrorMatches, "(?s).*no swap.*")

	// The swap warning is specific to swapless systems.
	mi = &osutil.MemInfo{Total: 1443 * mib, Available: 39 * mib, SwapTotal: 4 * gib}
	err = lxdbackend.CheckAvailableMemory(mi, workshop.ConfinementContainer)
	c.Check(err, check.Not(check.ErrorMatches), "(?s).*no swap.*")
}

// A VM is charged its whole configured size, so it needs more headroom than a
// container, and the gate must sit between the two measured outcomes for a
// 1 GiB guest: 1947 MiB of host memory was healthy, 1443 MiB OOM-killed.
func (s *capacitySuite) TestLaunchMemoryNeeded(c *check.C) {
	mi := &osutil.MemInfo{Total: 16 * gib}
	c.Check(lxdbackend.LaunchMemoryNeeded(mi, workshop.ConfinementVirtualMachine) >
		lxdbackend.LaunchMemoryNeeded(mi, workshop.ConfinementContainer), check.Equals, true)

	// A 4 GiB host gets the 1 GiB floor, matching the measured guest.
	needed := lxdbackend.LaunchMemoryNeeded(&osutil.MemInfo{Total: 4 * gib}, workshop.ConfinementVirtualMachine)
	c.Check(needed > 1443*mib, check.Equals, true)
	c.Check(needed < 1947*mib, check.Equals, true)
}
