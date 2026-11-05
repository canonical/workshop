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
	"io"
	"net/http"
	"strconv"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
)

// The reserve is a custom volume holding space in the storage pool, so that
// operations which free up space still work when the pool is otherwise full:
// LXD needs some space to stop or remove instances, e.g. to update their
// backup.yaml files. Launching a workshop requires the reserve to be in place.
const (
	reserveVolume = "workshop-reserve"
	reserveSize   = 256 * MiB
)

// ensurePoolSpace reports whether the storage pool can absorb an operation
// that needs the given space, and arms the reserve if needed.
func ensurePoolSpace(conn lxd.InstanceServer, needed uint64) error {
	conn = conn.UseProject(api.ProjectDefaultName)
	_, _, err := conn.GetStoragePoolVolume(storagePool, "custom", reserveVolume)
	armed := err == nil
	if err != nil && !api.StatusErrorCheck(err, http.StatusNotFound) {
		return err
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

	if armed {
		return nil
	}
	if err := armReserve(conn); err != nil {
		return fmt.Errorf("cannot create storage pool %q reserve: %w", storagePool, err)
	}
	return nil
}

func armReserve(conn lxd.InstanceServer) error {
	var op lxd.Operation
	var err error
	switch storagePoolDriver {
	case "zfs":
		// ZFS only allocates the size of a volume on demand, unless asked to
		// reserve it.
		op, err = conn.CreateStoragePoolVolume(storagePool, api.StorageVolumesPost{
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
	default:
		// Other drivers have no such option, so the reserve has to be filled
		// with data. An ISO volume is the simplest way to upload it.
		op, err = conn.CreateStoragePoolVolumeFromISO(storagePool, lxd.StoragePoolVolumeBackupArgs{
			Name:       reserveVolume,
			BackupFile: io.LimitReader(zeroReader{}, reserveSize),
		})
	}
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

// withReserve runs f, and if it fails because the storage pool is full,
// releases the reserve and runs f again. The reserve is re-armed before the
// next launch, if there's enough space by then.
func withReserve(conn lxd.InstanceServer, f func() error) error {
	err := f()
	if err == nil || !isOutOfSpace(err) {
		return err
	}

	released, rerr := releaseReserve(conn)
	if rerr != nil {
		return errors.Join(err, rerr)
	}
	if !released {
		return err
	}
	logger.Noticef("Released storage pool %q reserve after error: %v", storagePool, err)
	return f()
}

func isOutOfSpace(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "no space left on device") || strings.Contains(msg, "out of space")
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
