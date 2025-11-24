// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2020 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package osutil_test

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
	"github.com/canonical/workshop/internal/testutil"
)

type cpSuite struct {
	testutil.BaseTest

	dir  string
	f1   string
	f2   string
	data []byte
	log  []string
	errs []error
}

var _ = Suite(&cpSuite{})

func (s *cpSuite) mockCopyFile(fin, fout osutil.Fileish, fi os.FileInfo) error {
	return s.µ("copyfile")
}

func (s *cpSuite) mockOpenFile(name string, flag int, perm os.FileMode) (osutil.Fileish, error) {
	return &mockfile{s}, s.µ("open")
}

func (s *cpSuite) µ(msg string) (err error) {
	s.log = append(s.log, msg)
	if len(s.errs) > 0 {
		err = s.errs[0]
		if len(s.errs) > 1 {
			s.errs = s.errs[1:]
		}
	}

	return err
}

func (s *cpSuite) SetUpTest(c *C) {
	s.errs = nil
	s.log = nil
	s.dir = c.MkDir()
	s.f1 = filepath.Join(s.dir, "f1")
	s.f2 = filepath.Join(s.dir, "f2")
	s.data = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	c.Assert(os.WriteFile(s.f1, s.data, 0o644), IsNil)
}

func (s *cpSuite) mock() {
	s.AddCleanup(osutil.MockCopyFile(s.mockCopyFile))
	s.AddCleanup(osutil.MockOpenFile(s.mockOpenFile))
}

func (s *cpSuite) TestCp(c *C) {
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagDefault), IsNil)
	c.Check(s.f2, testutil.FileEquals, s.data)
}

func (s *cpSuite) TestCpNoOverwrite(c *C) {
	_, err := os.Create(s.f2)
	c.Assert(err, IsNil)
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagDefault), NotNil)
}

func (s *cpSuite) TestCpOverwrite(c *C) {
	_, err := os.Create(s.f2)
	c.Assert(err, IsNil)
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagOverwrite), IsNil)
	c.Check(s.f2, testutil.FileEquals, s.data)
}

func (s *cpSuite) TestCpOverwriteTruncates(c *C) {
	c.Assert(os.WriteFile(s.f2, []byte("xxxxxxxxxxxxxxxx"), 0o644), IsNil)
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagOverwrite), IsNil)
	c.Check(s.f2, testutil.FileEquals, s.data)
}

func (s *cpSuite) TestCpSync(c *C) {
	s.mock()
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagDefault), IsNil)
	c.Check(strings.Join(s.log, ":"), Not(Matches), `.*:sync(:.*)?`)

	s.log = nil
	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), IsNil)
	c.Check(strings.Join(s.log, ":"), Matches, `(.*:)?sync(:.*)?`)
}

func (s *cpSuite) TestCpCantOpen(c *C) {
	s.mock()
	s.errs = []error{errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `unable to open \S+/f1: xyzzy`)
}

func (s *cpSuite) TestCpCantStat(c *C) {
	s.mock()
	s.errs = []error{nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `unable to stat \S+/f1: xyzzy`)
}

func (s *cpSuite) TestCpCantCreate(c *C) {
	s.mock()
	s.errs = []error{nil, nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `unable to create \S+/f2: xyzzy`)
}

func (s *cpSuite) TestCpCantCopy(c *C) {
	s.mock()
	s.errs = []error{nil, nil, nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `unable to copy \S+/f1 to \S+/f2: xyzzy`)
}

func (s *cpSuite) TestCpCantSync(c *C) {
	s.mock()
	s.errs = []error{nil, nil, nil, nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `unable to sync \S+/f2: xyzzy`)
}

func (s *cpSuite) TestCpCantStop2(c *C) {
	s.mock()
	s.errs = []error{nil, nil, nil, nil, nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `when closing \S+/f2: xyzzy`)
}

func (s *cpSuite) TestCpCantStop1(c *C) {
	s.mock()
	s.errs = []error{nil, nil, nil, nil, nil, nil, errors.New("xyzzy"), nil}

	c.Check(osutil.CopyFile(s.f1, s.f2, osutil.CopyFlagSync), ErrorMatches, `when closing \S+/f1: xyzzy`)
}

type mockfile struct {
	s *cpSuite
}

var mockst = mockstat{}

func (f *mockfile) Close() error               { return f.s.µ("close") }
func (f *mockfile) Sync() error                { return f.s.µ("sync") }
func (f *mockfile) Fd() uintptr                { f.s.µ("fd"); return 42 }
func (f *mockfile) Read([]byte) (int, error)   { return 0, f.s.µ("read") }
func (f *mockfile) Write([]byte) (int, error)  { return 0, f.s.µ("write") }
func (f *mockfile) Stat() (os.FileInfo, error) { return mockst, f.s.µ("stat") }

type mockstat struct{}

func (mockstat) Name() string       { return "mockstat" }
func (mockstat) Size() int64        { return 42 }
func (mockstat) Mode() os.FileMode  { return 0o644 }
func (mockstat) ModTime() time.Time { return time.Now() }
func (mockstat) IsDir() bool        { return false }
func (mockstat) Sys() interface{}   { return nil }

func (s *cpSuite) TestCopySpecialFileSimple(c *C) {
	sync := testutil.FakeCommand(c, "sync", "")
	defer sync.Restore()

	src := filepath.Join(c.MkDir(), "fifo")
	err := syscall.Mkfifo(src, 0o644)
	c.Assert(err, IsNil)
	dir := c.MkDir()
	dst := filepath.Join(dir, "copied-fifo")

	err = osutil.CopySpecialFile(src, dst)
	c.Assert(err, IsNil)

	st, err := os.Stat(dst)
	c.Assert(err, IsNil)
	c.Check((st.Mode() & os.ModeNamedPipe), Equals, os.ModeNamedPipe)
	c.Check(sync.Calls(), DeepEquals, [][]string{{"sync", dir}})
}

func (s *cpSuite) TestCopySpecialFileErrors(c *C) {
	err := osutil.CopySpecialFile("no-such-file", "no-such-target")
	c.Assert(err, ErrorMatches, "failed to copy device node:.*cp:.*stat.*no-such-file.*")
}

func (s *cpSuite) TestCopyPreserveAll(c *C) {
	src := filepath.Join(c.MkDir(), "meep")
	dst := filepath.Join(c.MkDir(), "copied-meep")

	err := os.WriteFile(src, []byte(nil), 0o644)
	c.Assert(err, IsNil)

	// Give the file a different mtime to ensure CopyFlagPreserveAll
	// really works.
	//
	// You wonder why "touch" is used? And want to me about
	// syscall.Utime()? Well, syscall not implemented on armhf
	// Aha, syscall.Utimes() then? No, not implemented on arm64
	// Really, this is a just a test, touch is good enough!
	err = exec.Command("touch", src, "-d", "2007-08-23 08:21:42").Run()
	c.Assert(err, IsNil)

	err = osutil.CopyFile(src, dst, osutil.CopyFlagPreserveAll)
	c.Assert(err, IsNil)

	// ensure that the mtime got preserved
	st1, err := os.Stat(src)
	c.Assert(err, IsNil)
	st2, err := os.Stat(dst)
	c.Assert(err, IsNil)
	c.Assert(st1.ModTime(), Equals, st2.ModTime())
}

func (s *cpSuite) TestCopyPreserveAllSync(c *C) {
	dir := c.MkDir()
	mocked := testutil.FakeCommand(c, "cp", "").Also("sync", "")
	defer mocked.Restore()

	src := filepath.Join(dir, "meep")
	dst := filepath.Join(dir, "copied-meep")

	err := os.WriteFile(src, []byte(nil), 0o644)
	c.Assert(err, IsNil)

	err = osutil.CopyFile(src, dst, osutil.CopyFlagPreserveAll|osutil.CopyFlagSync)
	c.Assert(err, IsNil)

	c.Check(mocked.Calls(), DeepEquals, [][]string{
		{"cp", "-av", src, dst},
		{"sync"},
	})
}

func (s *cpSuite) TestCopyPreserveAllSyncCpFailure(c *C) {
	dir := c.MkDir()
	mocked := testutil.FakeCommand(c, "cp", "echo OUCH: cp failed.;exit 42").Also("sync", "")
	defer mocked.Restore()

	src := filepath.Join(dir, "meep")
	dst := filepath.Join(dir, "copied-meep")

	err := os.WriteFile(src, []byte(nil), 0o644)
	c.Assert(err, IsNil)

	err = osutil.CopyFile(src, dst, osutil.CopyFlagPreserveAll|osutil.CopyFlagSync)
	c.Assert(err, ErrorMatches, `failed to copy all: "OUCH: cp failed." \(42\)`)
	c.Check(mocked.Calls(), DeepEquals, [][]string{
		{"cp", "-av", src, dst},
	})
}

func (s *cpSuite) TestCopyPreserveAllSyncSyncFailure(c *C) {
	dir := c.MkDir()
	mocked := testutil.FakeCommand(c, "cp", "").Also("sync", "echo OUCH: sync failed.;exit 42")
	defer mocked.Restore()

	src := filepath.Join(dir, "meep")
	dst := filepath.Join(dir, "copied-meep")

	err := os.WriteFile(src, []byte(nil), 0o644)
	c.Assert(err, IsNil)

	err = osutil.CopyFile(src, dst, osutil.CopyFlagPreserveAll|osutil.CopyFlagSync)
	c.Assert(err, ErrorMatches, `failed to sync: "OUCH: sync failed." \(42\)`)

	c.Check(mocked.Calls(), DeepEquals, [][]string{
		{"cp", "-av", src, dst},
		{"sync"},
	})
}

// To check file ownership properly, run:
// $ go test -c ./internal/osutil
// $ sudo ./osutil.test -check.f TestCopyAllChown
func (s *cpSuite) TestCopyAllChown(c *C) {
	root := c.MkDir()

	c.Assert(os.Mkdir(filepath.Join(root, "a"), 0o755), IsNil)
	c.Assert(os.Mkdir(filepath.Join(root, "a", "dir"), 0o700), IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "a", "file"), []byte("foo"), os.ModePerm), IsNil)
	c.Assert(os.Chmod(filepath.Join(root, "a", "file"), os.ModePerm), IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "a", "script"), []byte("#!/bin/sh\n"), 0o705), IsNil)
	c.Assert(syscall.Mkfifo(filepath.Join(root, "a", "pipe"), 0o644), IsNil)
	c.Assert(os.Mkdir(filepath.Join(root, "target"), os.ModePerm), IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "target", "file"), []byte("bar"), 0o644), IsNil)
	c.Assert(os.Symlink(filepath.Join(root, "target"), filepath.Join(root, "a", "link")), IsNil)

	u, err := osutil.UserMaybeSudoUser()
	c.Assert(err, IsNil)
	uid, gid, err := osutil.UidGid(u)
	c.Assert(err, IsNil)

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Lchown(path, int(uid), int(gid))
	})
	c.Assert(err, IsNil)

	b := filepath.Join(c.MkDir(), "b")
	c.Assert(os.Mkdir(b, 0o700), IsNil)
	c.Assert(osutil.CopyAllChown(filepath.Join(root, "a"), b, uid, gid), IsNil)

	c.Check(filepath.Dir(b), testutil.DirEquals, []string{"drwxr-xr-x b"})
	c.Check(b, testutil.DirEquals, []string{
		"drwx------ dir",
		"-rwxrwxrwx file",
		"Lrwxrwxrwx link",
		"-rwx---r-x script",
	})
	c.Check(filepath.Join(b, "file"), testutil.FileEquals, "foo")
	c.Check(filepath.Join(b, "dir"), testutil.DirEquals, []string{})
	c.Check(filepath.Join(b, "link"), testutil.DirEquals, []string{"-rw-r--r-- file"})
	c.Check(filepath.Join(b, "link", "file"), testutil.FileEquals, "bar")
	c.Check(filepath.Join(b, "script"), testutil.FileEquals, "#!/bin/sh\n")

	err = filepath.WalkDir(b, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		fuid, fgid, err := sys.FileOwner(info)
		if err != nil {
			return err
		}

		c.Check(fuid, Equals, uid)
		c.Check(fgid, Equals, gid)
		return nil
	})
	c.Assert(err, IsNil)
}

func (s *cpSuite) TestCopyAllChownNoSource(c *C) {
	root := c.MkDir()
	c.Assert(os.Mkdir(filepath.Join(root, "b"), 0o700), IsNil)

	u, err := user.Current()
	c.Assert(err, IsNil)
	uid, gid, err := osutil.UidGid(u)
	c.Assert(err, IsNil)
	err = osutil.CopyAllChown(filepath.Join(root, "a"), filepath.Join(root, "b"), uid, gid)
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
}

func (s *cpSuite) TestCopyAllChownNoTarget(c *C) {
	root := c.MkDir()

	c.Assert(os.Mkdir(filepath.Join(root, "a"), os.ModePerm), IsNil)

	u, err := user.Current()
	c.Assert(err, IsNil)
	uid, gid, err := osutil.UidGid(u)
	c.Assert(err, IsNil)
	err = osutil.CopyAllChown(filepath.Join(root, "a"), filepath.Join(root, "b"), uid, gid)
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
}
