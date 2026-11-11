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

//go:build integration

package lxdbackend_test

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

// Btrfs pools are tested on a throwaway pool, since the workshop pool only
// uses Btrfs on WSL.
type reserveSuite struct {
	conn lxd.InstanceServer
}

var _ = check.Suite(&reserveSuite{})

const testPool = "workshop-test-btrfs"

func (s *reserveSuite) SetUpTest(c *check.C) {
	conn, err := lxd.ConnectLXDUnix("", nil)
	c.Assert(err, check.IsNil)
	s.conn = conn.UseProject(api.ProjectDefaultName)

	op, err := s.conn.CreateStoragePool(api.StoragePoolsPost{
		Name:           testPool,
		Driver:         "btrfs",
		StoragePoolPut: api.StoragePoolPut{Config: map[string]string{"size": "2GiB"}},
	})
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)
}

func (s *reserveSuite) TearDownTest(c *check.C) {
	volumes, err := s.conn.GetStoragePoolVolumes(testPool)
	c.Check(err, check.IsNil)
	for _, vol := range volumes {
		op, err := s.conn.DeleteStoragePoolVolume(testPool, vol.Type, vol.Name)
		if c.Check(err, check.IsNil) {
			c.Check(op.Wait(), check.IsNil)
		}
	}
	op, err := s.conn.DeleteStoragePool(testPool)
	if c.Check(err, check.IsNil) {
		c.Check(op.Wait(), check.IsNil)
	}
	s.conn.Disconnect()
}

func (s *reserveSuite) createRandomVolume(name string, size int64) error {
	rng := rand.NewChaCha8([32]byte{})
	op, err := s.conn.CreateStoragePoolVolumeFromISO(testPool, lxd.StoragePoolVolumeBackupArgs{
		Name:       name,
		BackupFile: io.LimitReader(rng, size),
	})
	if err != nil {
		return err
	}
	return op.Wait()
}

func (s *reserveSuite) TestBtrfsNoReserve(c *check.C) {
	defer lxdbackend.MockStoragePool(testPool, "btrfs")()

	// Btrfs pools don't get a reserve.
	err := lxdbackend.EnsurePoolSpace(s.conn, 0)
	c.Assert(err, check.IsNil)
	_, _, err = s.conn.GetStoragePoolVolume(testPool, "custom", lxdbackend.ReserveVolume)
	c.Check(api.StatusErrorCheck(err, http.StatusNotFound), check.Equals, true, check.Commentf("%v", err))

	// Fill up the pool. Btrfs reports more available space than can be
	// written, so fill it in chunks until one fails.
	for i := 0; ; i++ {
		err := s.createRandomVolume(fmt.Sprintf("test-filler-%d", i), 64*lxdbackend.MiB)
		if err != nil {
			c.Assert(err, check.ErrorMatches, ".*no space left on device")
			break
		}
	}

	// The margin is still kept, once Btrfs reports the usage, which it does
	// lazily.
	for start := time.Now(); ; time.Sleep(time.Second) {
		err = lxdbackend.EnsurePoolSpace(s.conn, 0)
		if err != nil || time.Since(start) > time.Minute {
			break
		}
	}
	c.Check(err, check.ErrorMatches, `(?s)storage pool "workshop-test-btrfs" is [0-9]+% full.*`)
}
