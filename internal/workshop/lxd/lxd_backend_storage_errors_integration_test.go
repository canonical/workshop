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
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"runtime"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

// storageErrorsSuite checks that the errors LXD reports when storage is full
// are detected as such, on throwaway pools for each driver Workshop uses.
type storageErrorsSuite struct {
	conn        lxd.InstanceServer
	pool        string
	fingerprint string
}

var _ = check.Suite(&storageErrorsSuite{})

const (
	errorsPool     = "workshop-test-errors"
	errorsInstance = "workshop-test-errors"
	errorsImage    = "workshop-test-errors"
	// The image holds a file larger than the quota.
	errorsFillSize = 40 * lxdbackend.MiB
	errorsQuota    = "32MiB"
)

func (s *storageErrorsSuite) SetUpTest(c *check.C) {
	conn, err := lxd.ConnectLXDUnix("", nil)
	c.Assert(err, check.IsNil)
	s.conn = conn.UseProject(api.ProjectDefaultName)
	s.fingerprint = s.createImage(c)
}

func (s *storageErrorsSuite) TearDownTest(c *check.C) {
	if s.pool != "" {
		op, err := s.conn.DeleteInstance(errorsInstance, false)
		if err == nil {
			c.Check(op.Wait(), check.IsNil)
		}
		volumes, err := s.conn.GetStoragePoolVolumes(s.pool)
		c.Check(err, check.IsNil)
		for _, vol := range volumes {
			if vol.Type != "custom" {
				continue
			}
			op, err := s.conn.DeleteStoragePoolVolume(s.pool, vol.Type, vol.Name)
			if c.Check(err, check.IsNil) {
				c.Check(op.Wait(), check.IsNil)
			}
		}
	}
	if s.fingerprint != "" {
		op, err := s.conn.DeleteImage(s.fingerprint)
		if c.Check(err, check.IsNil) {
			c.Check(op.Wait(), check.IsNil)
		}
	}
	if s.pool != "" {
		op, err := s.conn.DeleteStoragePool(s.pool)
		if c.Check(err, check.IsNil) {
			c.Check(op.Wait(), check.IsNil)
		}
	}
	s.conn.Disconnect()
	s.pool, s.fingerprint = "", ""
}

// createImage creates a container image that holds nothing but a file of
// errorsFillSize random bytes.
func (s *storageErrorsSuite) createImage(c *check.C) string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	c.Assert(arch, check.Not(check.Equals), "")
	metadata := fmt.Sprintf("architecture: %s\ncreation_date: %d\n", arch, time.Now().Unix())

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	c.Assert(tw.WriteHeader(&tar.Header{Name: "metadata.yaml", Mode: 0644, Size: int64(len(metadata))}), check.IsNil)
	_, err := tw.Write([]byte(metadata))
	c.Assert(err, check.IsNil)
	c.Assert(tw.WriteHeader(&tar.Header{Name: "rootfs/", Mode: 0755, Typeflag: tar.TypeDir}), check.IsNil)
	c.Assert(tw.WriteHeader(&tar.Header{Name: "rootfs/fill", Mode: 0644, Size: errorsFillSize}), check.IsNil)
	_, err = io.Copy(tw, io.LimitReader(rand.NewChaCha8([32]byte{}), errorsFillSize))
	c.Assert(err, check.IsNil)
	c.Assert(tw.Close(), check.IsNil)

	op, err := s.conn.CreateImage(api.ImagesPost{Aliases: []api.ImageAlias{{Name: errorsImage}}}, &lxd.ImageCreateArgs{MetaFile: &buf, MetaName: "image.tar"})
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)
	fingerprint, _ := op.Get().Metadata["fingerprint"].(string)
	c.Assert(fingerprint, check.Not(check.Equals), "")
	return fingerprint
}

// launch creates a pool with the given driver and a stopped instance on it.
func (s *storageErrorsSuite) launch(c *check.C, driver string) {
	op, err := s.conn.CreateStoragePool(api.StoragePoolsPost{
		Name:           errorsPool,
		Driver:         driver,
		StoragePoolPut: api.StoragePoolPut{Config: map[string]string{"size": "1GiB"}},
	})
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)
	s.pool = errorsPool

	op, err = s.conn.CreateInstance(api.InstancesPost{
		Name: errorsInstance,
		InstancePut: api.InstancePut{Devices: map[string]map[string]string{
			"root": {"type": "disk", "path": "/", "pool": errorsPool},
		}},
		Source: api.InstanceSource{Type: api.SourceTypeImage, Fingerprint: s.fingerprint},
	})
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)
}

// updateConfig updates the instance's config, which writes its backup.yaml
// into its volume, as disabling autostart does.
func (s *storageErrorsSuite) updateConfig(c *check.C, update func(*api.InstancePut)) error {
	inst, etag, err := s.conn.GetInstance(errorsInstance)
	c.Assert(err, check.IsNil)
	put := inst.Writable()
	update(&put)
	op, err := s.conn.UpdateInstance(errorsInstance, put, etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (s *storageErrorsSuite) testQuotaExceeded(c *check.C, driver string) {
	s.launch(c, driver)

	// Set a quota below the image's size (Btrfs counts it, and reports the
	// quota as exceeded while applying it), and write more than the quota
	// (ZFS only counts the instance's own writes).
	_ = s.updateConfig(c, func(put *api.InstancePut) {
		put.Devices["root"]["size"] = errorsQuota
	})
	fill := make([]byte, errorsFillSize)
	_, _ = rand.NewChaCha8([32]byte{1}).Read(fill)
	_ = s.conn.CreateInstanceFile(errorsInstance, "/fill2", lxd.InstanceFileArgs{
		Content: bytes.NewReader(fill),
		Type:    "file",
		Mode:    0644,
	})

	err := s.updateConfig(c, func(put *api.InstancePut) {
		put.Config["boot.autostart"] = "false"
	})
	c.Assert(err, check.ErrorMatches, ".*disk quota exceeded.*")
	c.Check(lxdbackend.IsStorageFull(err), check.Equals, true)
}

func (s *storageErrorsSuite) TestZFSQuotaExceeded(c *check.C) {
	s.testQuotaExceeded(c, "zfs")
}

func (s *storageErrorsSuite) TestBtrfsQuotaExceeded(c *check.C) {
	s.testQuotaExceeded(c, "btrfs")
}

func (s *storageErrorsSuite) testPoolFull(c *check.C, driver string) error {
	s.launch(c, driver)

	rng := rand.NewChaCha8([32]byte{})
	for i := 0; ; i++ {
		op, err := s.conn.CreateStoragePoolVolumeFromISO(errorsPool, lxd.StoragePoolVolumeBackupArgs{
			Name:       fmt.Sprintf("test-filler-%d", i),
			BackupFile: io.LimitReader(rng, 64*lxdbackend.MiB),
		})
		if err == nil {
			err = op.Wait()
		}
		if err != nil {
			c.Assert(lxdbackend.IsStorageFull(err), check.Equals, true, check.Commentf("%v", err))
			break
		}
	}

	return s.updateConfig(c, func(put *api.InstancePut) {
		put.Config["boot.autostart"] = "false"
	})
}

func (s *storageErrorsSuite) TestZFSPoolFull(c *check.C) {
	err := s.testPoolFull(c, "zfs")
	c.Assert(err, check.ErrorMatches, ".*no space left on device.*")
	c.Check(lxdbackend.IsStorageFull(err), check.Equals, true)
}

func (s *storageErrorsSuite) TestBtrfsPoolFull(c *check.C) {
	// Btrfs keeps separate metadata space, so small writes still succeed.
	err := s.testPoolFull(c, "btrfs")
	if err != nil {
		c.Check(lxdbackend.IsStorageFull(err), check.Equals, true, check.Commentf("%v", err))
	}
}
