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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	check "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type projectPathSuite struct{}

var _ = check.Suite(&projectPathSuite{})

type mountEntry struct{ dir, mountRoot string }

// fakeMounts pretends that a filesystem is mounted at each dir, exposing the
// subtree mountRoot of its superblock. The dirs must share a filesystem.
func (s *projectPathSuite) fakeMounts(c *check.C, mounts ...mountEntry) (devID string, restore func()) {
	var lines []string
	for i, m := range mounts {
		info, err := os.Stat(m.dir)
		c.Assert(err, check.IsNil)
		st := info.Sys().(*syscall.Stat_t)
		devID = fmt.Sprintf("%d:%d", unix.Major(st.Dev), unix.Minor(st.Dev))
		lines = append(lines, fmt.Sprintf("%d 24 %s %s %s rw,relatime - ext4 /dev/sda1 rw\n",
			31+i, devID, m.mountRoot, m.dir))
	}

	f := filepath.Join(c.MkDir(), "mountinfo")
	c.Assert(os.WriteFile(f, []byte(strings.Join(lines, "")), 0644), check.IsNil)
	return devID, lxdbackend.FakeMountInfo(f)
}

func (s *projectPathSuite) fakeMount(c *check.C, dir, mountRoot string) (devID string, restore func()) {
	return s.fakeMounts(c, mountEntry{dir, mountRoot})
}

func (s *projectPathSuite) TestHostProjectPathRebasesOntoMountPoint(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code")
	defer restore()

	proj := filepath.Join(d, "proj")
	c.Assert(os.Mkdir(proj, 0755), check.IsNil)
	c.Assert(os.WriteFile(workshop.LockPath(proj), []byte("42424242"), 0644), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, proj)
}

func (s *projectPathSuite) TestHostProjectPathWholeFilesystemMount(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/")
	defer restore()

	proj := filepath.Join(d, "user", "proj")
	c.Assert(os.MkdirAll(proj, 0755), check.IsNil)
	c.Assert(os.WriteFile(workshop.LockPath(proj), []byte("42424242"), 0644), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/user/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, proj)
}

func (s *projectPathSuite) TestHostProjectPathWithoutLock(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code")
	defer restore()

	proj := filepath.Join(d, "proj")
	c.Assert(os.Mkdir(proj, 0755), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, proj)
}

func (s *projectPathSuite) TestHostProjectPathRejectsOtherProject(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code")
	defer restore()

	proj := filepath.Join(d, "proj")
	c.Assert(os.Mkdir(proj, 0755), check.IsNil)
	c.Assert(os.WriteFile(workshop.LockPath(proj), []byte("24242424"), 0644), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")
}

func (s *projectPathSuite) TestHostProjectPathOtherFilesystem(c *check.C) {
	d := c.MkDir()
	_, restore := s.fakeMount(c, d, "/code")
	defer restore()

	c.Assert(os.Mkdir(filepath.Join(d, "proj"), 0755), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj", "0:999")
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")
}

func (s *projectPathSuite) TestHostProjectPathPartialComponentIsNotAPrefix(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code")
	defer restore()

	c.Assert(os.Mkdir(filepath.Join(d, "proj"), 0755), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/codex/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")
}

func (s *projectPathSuite) TestHostProjectPathMissingAndNotADirectory(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code")
	defer restore()

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")

	c.Assert(os.WriteFile(filepath.Join(d, "proj"), nil, 0644), check.IsNil)
	path, err = lxdbackend.HostProjectPath("42424242", "/code/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")
}

func (s *projectPathSuite) TestHostProjectPathUnusableFsRoot(c *check.C) {
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/")
	defer restore()

	for _, fsRoot := range []string{"", "code/proj"} {
		path, err := lxdbackend.HostProjectPath("42424242", fsRoot, devID)
		c.Assert(err, check.IsNil)
		c.Check(path, check.Equals, "")
	}
}

func (s *projectPathSuite) TestHostProjectPathMultipleMounts(c *check.C) {
	// findmnt reports one superblock subtree, which a host is free to mount in
	// several places at once, so the project may be under any of them.
	other, home := c.MkDir(), c.MkDir()
	devID, restore := s.fakeMounts(c, mountEntry{other, "/user"}, mountEntry{home, "/"})
	defer restore()

	proj := filepath.Join(home, "user", "proj")
	c.Assert(os.MkdirAll(proj, 0755), check.IsNil)
	c.Assert(os.WriteFile(workshop.LockPath(proj), []byte("42424242"), 0644), check.IsNil)

	path, err := lxdbackend.HostProjectPath("42424242", "/user/proj", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, proj)
}

func (s *projectPathSuite) TestHostProjectPathBrokenMountInfo(c *check.C) {
	f := filepath.Join(c.MkDir(), "mountinfo")
	c.Assert(os.WriteFile(f, []byte("31 24 ...truncated"), 0644), check.IsNil)
	defer lxdbackend.FakeMountInfo(f)()

	_, err := lxdbackend.HostProjectPath("42424242", "/code/proj", "8:1")
	c.Check(err, check.ErrorMatches, "incorrect number of fields, .*")
}

func (s *projectPathSuite) TestHostProjectPathDeleted(c *check.C) {
	// The bind mount of a deleted project keeps serving its inode, so the
	// mount point is still a readable directory on the right filesystem.
	d := c.MkDir()
	devID, restore := s.fakeMount(c, d, "/code/proj//deleted")
	defer restore()

	path, err := lxdbackend.HostProjectPath("42424242", "/code/proj//deleted", devID)
	c.Assert(err, check.IsNil)
	c.Check(path, check.Equals, "")
}
