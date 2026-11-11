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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
)

// The reserve is a custom volume holding space in the storage pool, so that
// operations which free up space still work when the pool is otherwise full:
// LXD needs some space to stop instances, e.g. to update their backup.yaml
// files. Launching a workshop requires the reserve to be in place.
//
// Only ZFS pools get a reserve. Btrfs keeps separate metadata space, so
// stopping instances works on a pool full of data, and the reserve's data
// space can't be used for metadata when that runs out. Btrfs pools still keep
// a margin of the reserve's size before launches.
const (
	reserveVolume = "workshop-reserve"
	reserveSize   = 256 * MiB
)

func usesReserve() bool {
	return storagePoolDriver == "zfs"
}

// ensurePoolSpace reports whether the storage pool can absorb an operation
// that needs the given space, and arms the reserve if needed.
//
// This doesn't lock: concurrent callers may both try to arm the reserve, in
// which case the loser finds it in place. An operation releasing the reserve
// concurrently implies the pool is nearly full, so the check below would fail
// rather than re-arm it.
func ensurePoolSpace(conn lxd.InstanceServer, needed uint64) error {
	conn = conn.UseProject(api.ProjectDefaultName)
	armed := false
	if usesReserve() {
		var err error
		armed, err = reserveExists(conn)
		if err != nil {
			return err
		}
	}
	if !armed {
		needed += reserveSize
	}

	res, err := conn.GetStoragePoolResources(storagePool)
	if err != nil {
		return err
	}
	if err := checkPoolSpace(res.Space.Total, res.Space.Used, needed); err != nil {
		return err
	}

	if armed || !usesReserve() {
		return nil
	}
	if err := armReserve(conn); err != nil {
		// LXD may not report a conflicting creation as such once the
		// operation is under way, so check whether someone else armed it.
		if exists, _ := reserveExists(conn); exists {
			return nil
		}
		return fmt.Errorf("cannot create storage pool %q reserve: %w", storagePool, err)
	}
	return nil
}

func reserveExists(conn lxd.InstanceServer) (bool, error) {
	_, _, err := conn.GetStoragePoolVolume(storagePool, "custom", reserveVolume)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return false, nil
	}
	return err == nil, err
}

func armReserve(conn lxd.InstanceServer) error {
	// ZFS only allocates the size of a volume on demand, unless asked to
	// reserve it.
	op, err := conn.CreateStoragePoolVolume(storagePool, api.StorageVolumesPost{
		Name:        reserveVolume,
		Type:        "custom",
		ContentType: "filesystem",
		StorageVolumePut: api.StorageVolumePut{
			Config: map[string]string{
				"size":              strconv.Itoa(reserveSize),
				"zfs.reserve_space": "true",
			},
		},
	})
	if err != nil {
		return err
	}
	return op.Wait()
}

// releaseReserve deletes the reserve volume, and reports whether it existed.
func releaseReserve(conn lxd.InstanceServer) (bool, error) {
	conn = conn.UseProject(api.ProjectDefaultName)
	op, err := conn.DeleteStoragePoolVolume(storagePool, "custom", reserveVolume)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := op.Wait(); err != nil {
		return false, err
	}
	return true, nil
}

// RestoreStorageReserve arms the reserve, if it's missing and there's space.
func (s *Backend) RestoreStorageReserve(ctx context.Context) error {
	if !usesReserve() {
		return nil
	}
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	return ensurePoolSpace(conn, 0)
}

// withReserve runs f, and if it fails because the storage pool is full,
// releases the reserve and runs f again. The reserve is restored after the
// change completes, or before the next launch, if there's enough space by then.
//
// f is retried even if the reserve was already gone, since a concurrent
// operation may have just released it.
func withReserve(conn lxd.InstanceServer, f func() error) error {
	err := f()
	if err == nil || !isOutOfSpace(err) {
		return err
	}

	released, rerr := releaseReserve(conn)
	if released {
		logger.Noticef("Released storage pool %q reserve after error: %v", storagePool, err)
	}
	if err := f(); err != nil {
		return errors.Join(err, rerr)
	}
	return nil
}

func isOutOfSpace(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "no space left on device") || strings.Contains(msg, "out of space")
}

// isStorageFull reports whether err is caused by the storage pool or the
// instance's own quota being full.
func isStorageFull(err error) bool {
	return isOutOfSpace(err) || strings.Contains(err.Error(), "disk quota exceeded")
}
