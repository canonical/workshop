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

package lxdbackend

import (
	"fmt"
	"runtime"
	"strconv"

	lxd "github.com/canonical/lxd/client"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/workshop"
)

// Default resource limits for workshop VMs. Containers get none: a cgroup
// limit is only a ceiling, not a reservation, so capping them buys nothing.
//
// LXD's units package parses and formats byte sizes but exports no constants,
// so define the two we need.
const (
	MiB = 1 << 20
	GiB = 1 << 30
)

// Kept free in the pool: zfs degrades as it fills, and a full pool cannot
// even delete instances. Nothing sized here may eat into it.
const poolReserve = 1 * GiB

// LXD's own defaults are one core and 1 GiB (QEMUDefaultCPUCores and
// QEMUDefaultMemSize). The memory half is too small for a development
// environment: 1 GiB configured leaves only ~893 MiB of guest MemTotal.
//
// Memory is the limit that matters. Oversubscribing CPU degrades gracefully
// (measured: 6x oversubscribed gave 92% CPU pressure and load 11.9, yet
// everything still completed, 2.6x slower), whereas oversubscribing memory
// falls off a cliff into the OOM killer with no swap to cushion it.
const (
	vmCPUFraction = 2
	vmCPUMin      = 1
	vmCPUMax      = 8

	vmMemoryFraction = 4
	// 768 MiB is marginal, 512 MiB boots only sometimes, 256 MiB never.
	vmMemoryMin = 1 * GiB
	vmMemoryMax = 8 * GiB
)

// defaultVMLimits returns limits.cpu and limits.memory for a VM.
//
// These derive from total memory and cores, never from what is free right now:
// sizing from available memory would make the same workshop.yaml produce a
// 4 GiB VM on a fresh boot and a 1 GiB VM an hour later. Whether there is room
// to start it is a separate question, see checkLaunchCapacity.
func defaultVMLimits(memTotal uint64, cores int) (cpu int, memory uint64) {
	cpu = clamp(cores/vmCPUFraction, vmCPUMin, vmCPUMax)
	if cores > 0 && cpu > cores {
		cpu = cores
	}

	return cpu, clamp(memTotal/vmMemoryFraction, uint64(vmMemoryMin), uint64(vmMemoryMax))
}

func clamp[T int | uint64](v, lo, hi T) T {
	return min(max(v, lo), hi)
}

// vmLimitsConfig returns the config carrying the default VM limits, or nil to
// leave LXD's defaults in place. The values are set explicitly so they are
// recorded in the instance config, which rebuilds rely on.
func vmLimitsConfig() map[string]string {
	mi, err := osutil.ReadMemInfo()
	if err != nil {
		return nil
	}

	cpu, memory := defaultVMLimits(mi.Total, runtime.NumCPU())

	return map[string]string{
		"limits.cpu":    strconv.Itoa(cpu),
		"limits.memory": fmt.Sprintf("%dMiB", memory/MiB),
	}
}

// Default root disk size for workshop VMs. Containers get none: their root
// filesystem just grows into the pool.
//
// On a copy-on-write pool a VM's root volume is sparse, so its nominal size
// costs nothing up front. The quota exists to bound a runaway guest: if a
// workshop fills its own disk it gets ENOSPC and only it is affected, whereas
// if the pool fills, LXD cannot write metadata and even deleting instances to
// recover can fail. So the quota must bite before the pool does.
//
// LXD's 10 GiB default takes no account of the pool: a workshop pool can be
// storagePoolMinimalGiB, where 10 GiB overcommits it twofold and protects
// nothing.
const (
	vmRootDiskFraction = 2
	vmRootDiskMin      = 10 * GiB
	vmRootDiskMax      = 50 * GiB
)

// defaultVMRootDiskSize returns the nominal size for a VM's root volume, or
// zero to leave LXD's default in place.
func defaultVMRootDiskSize(poolTotal uint64) uint64 {
	if poolTotal == 0 {
		return 0
	}

	size := clamp(poolTotal/vmRootDiskFraction, uint64(vmRootDiskMin), uint64(vmRootDiskMax))

	// However small the pool, a full guest disk must not mean a full pool.
	return min(size, poolTotal-min(poolTotal, poolReserve))
}

func vmRootDiskSize(conn lxd.InstanceServer, confinement workshop.Confinement) uint64 {
	if confinement != workshop.ConfinementVirtualMachine {
		return 0
	}

	res, err := conn.GetStoragePoolResources(storagePool)
	if err != nil {
		return 0
	}

	return defaultVMRootDiskSize(res.Space.Total)
}
