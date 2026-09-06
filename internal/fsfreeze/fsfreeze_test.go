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

package fsfreeze_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/fsfreeze"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/testutil"
)

func Test(t *testing.T) { check.TestingT(t) }

type fsfreezeSuite struct {
	scratch string

	frozen  map[string]error
	canThaw chan struct{}
	hangup  bool

	restoreFilesystems func()
	restoreMountinfo   func()
	restoreTimeout     func()
	restoreFsFreeze    func()
	restoreFsThaw      func()
}

var _ = check.Suite(&fsfreezeSuite{})

const fakeFilesystems = `
nodev	sysfs
nodev	tmpfs
nodev	proc
	ext4
	vfat
	fuseblk
nodev	fuse
	btrfs
nodev	virtiofs
nodev	9p
nodev	nfs4
nodev	zfs
`

func (s *fsfreezeSuite) SetUpSuite(c *check.C) {
	s.scratch = c.MkDir()

	err := os.WriteFile(filepath.Join(s.scratch, "filesystems"), []byte(fakeFilesystems[1:]), 0644)
	c.Assert(err, check.IsNil)
	s.restoreFilesystems = fsfreeze.MockFilesystems(filepath.Join(s.scratch, "filesystems"))

	s.restoreMountinfo = fsfreeze.MockMountinfo(filepath.Join(s.scratch, "mountinfo"))

	s.restoreTimeout = fsfreeze.MockTimeout(50 * time.Millisecond)

	s.restoreFsFreeze = fsfreeze.MockFsFreeze(s.fsFreeze)
	s.restoreFsThaw = fsfreeze.MockFsThaw(s.fsThaw)
}

func (s *fsfreezeSuite) SetUpTest(c *check.C) {
	s.frozen = map[string]error{}
	s.canThaw = make(chan struct{})
	s.hangup = false
}

func (s *fsfreezeSuite) TearDownSuite(c *check.C) {
	s.restoreFsThaw()
	s.restoreFsFreeze()
	s.restoreMountinfo()
	s.restoreFilesystems()
}

func (s *fsfreezeSuite) fsFreeze(file *os.File) error {
	if err := s.frozen[file.Name()]; err != nil {
		return err
	}
	s.frozen[file.Name()] = unix.EBUSY
	return nil
}

func (s *fsfreezeSuite) fsThaw(file *os.File) error {
	// Ensure the tests have finished checking s.frozen before we touch it
	// again. If the freeze times out, the test is very likely to be past that
	// point, but `go test -race` can detect the lack of synchronization.
	<-s.canThaw

	if err := s.frozen[file.Name()]; !errors.Is(err, unix.EBUSY) {
		return err
	}
	delete(s.frozen, file.Name())
	return nil
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsOK(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	thaw, result, err := s.freezeLocalFilesystems()
	c.Assert(err, check.IsNil)

	c.Check(s.frozen, check.HasLen, 2)
	c.Check(s.frozen["/"], testutil.ErrorIs, unix.EBUSY)
	c.Check(s.frozen[dir], testutil.ErrorIs, unix.EBUSY)
	close(s.canThaw)

	thaw.Close()
	c.Assert(<-result, check.IsNil)

	c.Check(s.frozen, check.HasLen, 0)
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsUnsupported(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	s.frozen[dir] = unix.EOPNOTSUPP

	thaw, result, err := s.freezeLocalFilesystems()
	c.Assert(err, check.IsNil)

	c.Check(s.frozen, check.HasLen, 2)
	c.Check(s.frozen["/"], testutil.ErrorIs, unix.EBUSY)
	c.Check(s.frozen[dir], testutil.ErrorIs, unix.EOPNOTSUPP)
	close(s.canThaw)

	thaw.Close()
	c.Assert(<-result, check.IsNil)

	c.Check(s.frozen, check.HasLen, 1)
	c.Check(s.frozen[dir], testutil.ErrorIs, unix.EOPNOTSUPP)
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsRootUnsupported(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	s.frozen["/"] = unix.EOPNOTSUPP
	close(s.canThaw)

	thaw, result, err := s.freezeLocalFilesystems()
	c.Check(err, check.ErrorMatches, "operation not supported")

	if err == nil {
		thaw.Close()
		c.Assert(<-result, check.ErrorMatches, "operation not supported")
	}

	c.Check(s.frozen, check.HasLen, 1)
	c.Check(s.frozen["/"], testutil.ErrorIs, unix.EOPNOTSUPP)
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsError(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	s.frozen[dir] = errors.New("cannot freeze")
	close(s.canThaw)

	thaw, result, err := s.freezeLocalFilesystems()
	c.Check(err, check.ErrorMatches, "cannot freeze")

	if err == nil {
		thaw.Close()
		c.Assert(<-result, check.ErrorMatches, "cannot freeze")
	}

	c.Check(s.frozen, check.HasLen, 1)
	c.Check(s.frozen[dir], check.ErrorMatches, "cannot freeze")
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsHangup(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	s.hangup = true
	close(s.canThaw)

	thaw, result, err := s.freezeLocalFilesystems()
	c.Check(err, check.ErrorMatches, "cannot report readiness: broken pipe")

	if err == nil {
		thaw.Close()
		c.Assert(<-result, check.ErrorMatches, "cannot report readiness: broken pipe")
	}

	c.Check(s.frozen, check.HasLen, 0)
}

func (s *fsfreezeSuite) TestFreezeLocalFilesystemsTimeout(c *check.C) {
	dir := c.MkDir()
	mountinfo := fmt.Sprintf(`
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / %s rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:], dir)
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	thaw, result, err := s.freezeLocalFilesystems()
	c.Assert(err, check.IsNil)

	c.Check(s.frozen, check.HasLen, 2)
	c.Check(s.frozen["/"], testutil.ErrorIs, unix.EBUSY)
	c.Check(s.frozen[dir], testutil.ErrorIs, unix.EBUSY)
	close(s.canThaw)

	c.Check(<-result, check.ErrorMatches, "context deadline exceeded")
	thaw.Close()

	c.Check(s.frozen, check.HasLen, 0)
}

// freezeLocalFilesystems simulates calling fsfreeze.FreezeLocalFilesystems as
// a subprocess, using pipes to receive the ready signal and return. On
// success, returns an io.Closer that signals the "subprocess" to thaw the
// filesystems, and a channel to receive the error it reports (if any). If the
// ready signal isn't received promptly, we wait for the "subprocess" to
// timeout, close the remaining pipes and return an error.
func (s *fsfreezeSuite) freezeLocalFilesystems() (io.Closer, <-chan error, error) {
	rev := revert.New()
	defer rev.Fail()

	stdin, thaw := io.Pipe()
	rev.Add(func() {
		thaw.Close()
	})

	var ready <-chan struct{}
	var stdout io.Writer
	if s.hangup {
		stdout = closedWriter{}
	} else {
		w := fsfreeze.NewReadyWriter()
		stdout = w
		ready = w.Ready()
	}

	result := make(chan error, 1)
	go func() {
		defer stdin.Close()
		result <- fsfreeze.FreezeLocalFilesystems(stdin, stdout)
		close(result)
	}()

	select {
	case <-ready:
		rev.Success()
		return thaw, result, nil
	case err := <-result:
		return nil, nil, err
	}
}

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) {
	return 0, unix.EPIPE
}

func (s *fsfreezeSuite) TestLocalMountsOK(c *check.C) {
	mountinfo := `
23 28 0:22 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc proc rw
24 28 0:23 / /sys rw,nosuid,nodev,noexec,relatime shared:2 - sysfs sysfs rw
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw,discard
41 28 0:37 / /tmp rw,nosuid,nodev shared:20 - tmpfs tmpfs rw
44 28 252:16 / /boot rw,relatime shared:22 - ext4 /dev/vda16 rw
46 44 252:15 / /boot/efi rw,relatime shared:24 - vfat /dev/vda15 rw
55 28 0:46 / /usr/local/lib/workshop/guest ro,relatime shared:30 - virtiofs workshop.bin ro
62 28 0:51 / /mnt/share rw,relatime shared:40 - nfs4 server:/export rw
70 28 252:32 / /mnt/backup ro,relatime shared:50 - ext4 /dev/vdc ro
75 28 0:70 /@home /home rw,relatime shared:22 - btrfs /dev/vdd1 rw,subvol=/@home
`[1:]
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	mounts, root, err := fsfreeze.LocalMounts()
	c.Assert(err, check.IsNil)
	c.Check(mountDirs(mounts), check.DeepEquals, []string{"/home", "/mnt/backup", "/boot/efi", "/boot", "/"})
	c.Check(root, check.Equals, "252:1")
}

func (s *fsfreezeSuite) TestLocalMountsCompactsBySuperblock(c *check.C) {
	mountinfo := `
28 1 252:1 / / rw,relatime shared:1 - ext4 /dev/vda1 rw
44 28 252:16 / /srv rw,relatime shared:22 - ext4 /dev/vda16 rw
45 44 252:1 /project /srv/project rw,relatime shared:1 - ext4 /dev/vda1 rw
`[1:]
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	mounts, root, err := fsfreeze.LocalMounts()
	c.Assert(err, check.IsNil)
	// It's important that / is chosen over /project, because /srv could be
	// backed by a loop file on /, and freezing /project also freezes /.
	c.Check(mountDirs(mounts), check.DeepEquals, []string{"/srv", "/"})
	c.Check(root, check.Equals, "252:1")
}

func mountDirs(mounts []*osutil.MountInfoEntry) []string {
	dirs := make([]string, 0, len(mounts))
	for _, m := range mounts {
		dirs = append(dirs, m.MountDir)
	}
	return dirs
}

func (s *fsfreezeSuite) TestLocalMountsRequiresRootFS(c *check.C) {
	mountinfo := `
23 28 0:22 / /proc rw,relatime shared:12 - proc proc rw
`[1:]
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	_, _, err = fsfreeze.LocalMounts()
	c.Check(err, check.ErrorMatches, "root filesystem not mounted")
}

func (s *fsfreezeSuite) TestLocalMountsRequiresLocalRootFS(c *check.C) {
	mountinfo := `
28 1 0:51 / / rw,relatime shared:1 - nfs4 server:/export rw
44 28 252:16 / /boot rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:]
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	_, _, err = fsfreeze.LocalMounts()
	c.Check(err, check.ErrorMatches, `root filesystem 0:51 \(nfs4\) is not freezable`)
}

func (s *fsfreezeSuite) TestLocalMountsSupportForZFS(c *check.C) {
	mountinfo := `
28 1 0:60 / / rw,relatime shared:1 - zfs rpool/ROOT/ubuntu rw,xattr
44 28 252:16 / /boot rw,relatime shared:22 - ext4 /dev/vda16 rw
`[1:]
	err := os.WriteFile(filepath.Join(s.scratch, "mountinfo"), []byte(mountinfo), 0644)
	c.Assert(err, check.IsNil)

	_, _, err = fsfreeze.LocalMounts()
	c.Check(err, check.ErrorMatches, `root filesystem 0:60 \(zfs\) is not freezable`)
}
