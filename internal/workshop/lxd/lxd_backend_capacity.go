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
	"errors"
	"fmt"
	"net/http"
	"runtime"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/lxd/shared/units"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/workshop"
)

// Space and memory a launch is expected to need.
//
// The first launch from an image is the only expensive one: LXD unpacks it
// into an `image` volume and takes a readonly snapshot. Later launches clone
// that snapshot, which on a copy-on-write pool is nearly free.
//
// Unpacked sizes measured as the pool delta across a first launch on zfs:
//
//	image                compressed    unpacked     ratio
//	ubuntu 22.04 (ctr)    484.7 MiB    707.2 MiB    1.46
//	ubuntu 24.04 (ctr)    266.4 MiB    499.8 MiB    1.88
//	ubuntu 26.04 (ctr)    303.6 MiB    562.2 MiB    1.85
//	ubuntu 22.04 (vm)     670.9 MiB    868.5 MiB    1.29
//	ubuntu 24.04 (vm)     595.9 MiB    843.3 MiB    1.42
//	ubuntu 26.04 (vm)     823.3 MiB   1097.6 MiB    1.33
//
// The ratios below round those up. They assume a copy-on-write pool, which is
// what workshop creates.
const (
	containerUnpackRatio = 2.5
	vmUnpackRatio        = 2.0

	// Assumed unpacked size when an image has not been downloaded yet and
	// its real size cannot be read.
	unknownContainerImage = 1 * GiB
	unknownVMImage        = 2 * GiB

	// Space a clone needs for what it writes while booting. Measured at
	// under 10 MiB; rounded up because it is cheap to do so.
	cloneFootprint = 128 * MiB

	// Memory an idle container adds. Measured at 264 MiB for the first and
	// ~112 MiB by the sixth as page cache is shared; the higher figure is
	// used because real workshops share less.
	containerFootprint = 320 * MiB

	// QEMU's own cost on top of the guest's RAM.
	vmMemoryOverhead = 150 * MiB

	// Kept available so a launch cannot push the host into OOM.
	memoryReserve = 512 * MiB
)

// ErrInsufficientResources is returned when the host cannot fit another
// workshop. The request was fine; there is just no room for it.
var ErrInsufficientResources = errors.New("insufficient resources")

// checkLaunchCapacity reports whether there is room to launch the snapshot.
//
// Unlike checkStorageSpace, this knows how big the particular launch will be:
// a pool with 600 MiB free is fine for a clone and hopeless for an unpack.
func checkLaunchCapacity(conn lxd.InstanceServer, snapshot workshop.Snapshot) error {
	if err := checkLaunchMemory(snapshot.Image.Confinement); err != nil {
		return err
	}

	src, err := readLaunchSource(conn, snapshot)
	if err != nil {
		// Never block a launch because the estimate failed;
		// checkStorageSpace still guards the gross case.
		return nil
	}

	res, err := conn.GetStoragePoolResources(storagePool)
	if err != nil {
		return nil
	}

	return checkPoolSpace(res.Space.Total, res.Space.Used, *src)
}

// launchSource is what the pool-space decision needs to know about what is
// being launched. It is plain data so the decision can be tested directly.
type launchSource struct {
	// Clone is set when the launch copies something already in the pool:
	// an SDK snapshot, or an image that has been unpacked before.
	Clone bool

	// ImageSize is the compressed size of the image to unpack, or zero if
	// it has not been downloaded and its size cannot be read.
	ImageSize int64

	// VM is set for virtual-machine images, which unpack differently.
	VM bool
}

// readLaunchSource asks LXD what the launch will involve.
func readLaunchSource(conn lxd.InstanceServer, snapshot workshop.Snapshot) (*launchSource, error) {
	src := &launchSource{VM: snapshot.Image.Confinement == workshop.ConfinementVirtualMachine}

	if !snapshot.IsBase() {
		// Cloning an SDK snapshot already in the pool.
		src.Clone = true
		return src, nil
	}

	// Image volumes live in the default project, as workshop projects are
	// created with features.images=false.
	conn = conn.UseProject("")

	fingerprint := snapshot.Image.Fingerprint
	_, _, err := conn.GetStoragePoolVolume(storagePool, "image", fingerprint)
	if err == nil {
		// Cloning the image's readonly snapshot.
		src.Clone = true
		return src, nil
	}
	if !api.StatusErrorCheck(err, http.StatusNotFound) {
		return nil, err
	}

	image, _, err := conn.GetImage(fingerprint)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		// Not downloaded yet, so leave imageSize at zero.
		return src, nil
	}
	if err != nil {
		return nil, err
	}

	src.ImageSize = image.Size

	return src, nil
}

// launchSpace returns the pool space a launch needs, and whether it is
// dominated by unpacking an image.
func launchSpace(src launchSource) (needed uint64, unpacks bool) {
	if src.Clone {
		return cloneFootprint, false
	}

	ratio, unknown := containerUnpackRatio, uint64(unknownContainerImage)
	if src.VM {
		ratio, unknown = vmUnpackRatio, unknownVMImage
	}

	if src.ImageSize == 0 {
		// Over-estimate: deferring a launch beats failing part-way
		// through one.
		return unknown, true
	}

	return uint64(float64(src.ImageSize)*ratio) + cloneFootprint, true
}

// checkPoolSpace decides whether a pool of the given size can absorb the
// launch. total and used come straight from LXD.
func checkPoolSpace(total, used uint64, src launchSource) error {
	if total == 0 {
		// Cannot determine usage.
		return nil
	}

	avail := total - min(used, total)
	needed, unpacks := launchSpace(src)
	needed += poolReserve
	if avail >= needed {
		return nil
	}

	hint := "free up space or expand the pool with `lxc storage set " + storagePool + " size=<N>GiB`"
	if unpacks {
		hint = "this base image has not been used before, so it must be unpacked first; " + hint
	}

	return fmt.Errorf("%w: storage pool %q needs %s to launch this workshop but only %s is available; %s",
		ErrInsufficientResources, storagePool, byteSize(needed), byteSize(avail), hint)
}

func checkLaunchMemory(confinement workshop.Confinement) error {
	mi, err := osutil.ReadMemInfo()
	if err != nil {
		// Not being able to read /proc/meminfo is no reason to refuse.
		return nil
	}

	return checkAvailableMemory(mi, confinement)
}

// launchMemoryNeeded is how much memory must be available to start an
// instance, including the reserve.
func launchMemoryNeeded(mi *osutil.MemInfo, confinement workshop.Confinement) uint64 {
	if confinement != workshop.ConfinementVirtualMachine {
		return containerFootprint + memoryReserve
	}

	// A VM is charged its whole configured size, sized by the same rule
	// that will be applied when it is created.
	_, memory := defaultVMLimits(mi.Total, runtime.NumCPU())

	return memory + vmMemoryOverhead + memoryReserve
}

// checkAvailableMemory gates on MemAvailable rather than memory pressure. PSI
// registers under sustained thrashing, but a sharp allocation on a swapless
// host goes straight to the OOM killer without stalling first: it read 0.00
// across a run that killed three processes. MemAvailable predicted both.
func checkAvailableMemory(mi *osutil.MemInfo, confinement workshop.Confinement) error {
	needed := launchMemoryNeeded(mi, confinement)
	if mi.Available >= needed {
		return nil
	}

	var noSwap string
	if mi.SwapTotal == 0 {
		noSwap = "\nThis system has no swap, so running out of memory terminates processes rather than slowing down."
	}

	return fmt.Errorf("%w: launching this workshop needs about %s of memory but only %s is available; "+
		"stop another workshop with `workshop stop` or free up memory on the host%s",
		ErrInsufficientResources, byteSize(needed), byteSize(mi.Available), noSwap)
}

func byteSize(b uint64) string {
	return units.GetByteSizeStringIEC(int64(b), 1)
}
