package fsutil_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/testutil"
)

type fsSuite struct {
	path string
	fs   fsutil.Fs
}

func TestMain(m *testing.M) {
	// Ensure consistent file permissions for fsSuite.
	syscall.Umask(0o002)
	m.Run()
}

var _ = check.Suite(&fsSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (f *fsSuite) SetUpTest(c *check.C) {
	f.path = c.MkDir()
	f.fs = fsutil.NewBasePathFs(f.path)
}

func (f *fsSuite) TestMkdirAll(c *check.C) {
	for range 2 {
		c.Assert(f.fs.MkdirAll("one/two/three", os.ModePerm), check.IsNil)
		c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x one"})
		c.Check(filepath.Join(f.path, "one"), testutil.DirEquals, []string{"drwxrwxr-x two"})
		c.Check(filepath.Join(f.path, "one", "two"), testutil.DirEquals, []string{"drwxrwxr-x three"})
		c.Check(filepath.Join(f.path, "one", "two", "three"), testutil.DirEquals, []string{})
	}
}

func (f *fsSuite) TestMkdirAllExistingFile(c *check.C) {
	c.Assert(f.fs.WriteFile("file", nil, 0o666), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-rw-r-- file"})
	err := f.fs.MkdirAll("file/dir", os.ModePerm)
	c.Assert(err, testutil.ErrorIs, syscall.ENOTDIR)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-rw-r-- file"})
}

func (f *fsSuite) TestMkdirAllPerms(c *check.C) {
	c.Assert(f.fs.MkdirAll("one/two", 0o700), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwx------ one"})
	c.Check(filepath.Join(f.path, "one"), testutil.DirEquals, []string{"drwx------ two"})
	c.Check(filepath.Join(f.path, "one", "two"), testutil.DirEquals, []string{})

	c.Assert(f.fs.MkdirAll("one/two/three", 0o711), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwx------ one"})
	c.Check(filepath.Join(f.path, "one"), testutil.DirEquals, []string{"drwx------ two"})
	c.Check(filepath.Join(f.path, "one", "two"), testutil.DirEquals, []string{"drwx--x--x three"})
}

func (f *fsSuite) TestMkdirAllWeirdPaths(c *check.C) {
	c.Assert(f.fs.MkdirAll("/one/two/..", os.ModePerm), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x one"})
	c.Check(filepath.Join(f.path, "one"), testutil.DirEquals, []string{"drwxrwxr-x two"})
	c.Check(filepath.Join(f.path, "one", "two"), testutil.DirEquals, []string{})
}

func (f *fsSuite) TestMkdirAllChmodChown(c *check.C) {
	c.Assert(f.fs.MkdirAllChmodChown("one/two", os.ModePerm, os.Geteuid(), os.Getegid()), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxrwx one"})
	c.Check(filepath.Join(f.path, "one"), testutil.DirEquals, []string{"drwxrwxrwx two"})
	c.Check(filepath.Join(f.path, "one", "two"), testutil.DirEquals, []string{})
}

func (f *fsSuite) TestMkdirTemp(c *check.C) {
	dir, err := f.fs.MkdirTemp("", "test.*.d", os.ModePerm)
	c.Assert(err, check.IsNil)
	c.Check(dir, check.Matches, `test\.[0-9]*\.d`)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x " + dir})
}

func (f *fsSuite) TestMkdirTempBadPattern(c *check.C) {
	dir, err := f.fs.MkdirTemp("", "parent/dir", os.ModePerm)
	c.Assert(err, check.ErrorMatches, "mkdirtemp parent/dir: pattern contains path separator")
	c.Check(dir, check.Equals, "")
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestMkdirTempNoParent(c *check.C) {
	dir, err := f.fs.MkdirTemp("parent", "temp", os.ModePerm)
	c.Assert(err, check.ErrorMatches, fmt.Sprintf("stat %s/parent: no such file or directory", f.path))
	c.Check(dir, check.Equals, "")

	c.Assert(f.fs.Mkdir("parent", 0o555), check.IsNil)
	dir, err = f.fs.MkdirTemp("parent/", "temp", os.ModePerm)
	c.Assert(err, check.ErrorMatches, fmt.Sprintf("mkdir %s/parent/temp[0-9]*: permission denied", f.path))
	c.Check(dir, check.Equals, "")
}

func (f *fsSuite) TestMkdirTempCollisions(c *check.C) {
	fs := fsutil.Fs{FsBackend: &nameCollisionFs{f.fs.FsBackend, 5}}
	dir, err := fs.MkdirTemp("", "temp", os.ModePerm)
	c.Assert(err, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x " + dir})
	c.Assert(fs.Remove(dir), check.IsNil)

	fs = fsutil.Fs{FsBackend: &nameCollisionFs{f.fs.FsBackend, 10000}}
	dir, err = fs.MkdirTemp("", "temp", os.ModePerm)
	c.Assert(err, check.ErrorMatches, `mkdirtemp temp\*: file already exists`)
	c.Check(dir, check.Equals, "")
	c.Check(f.path, testutil.DirEquals, []string{})
}

type nameCollisionFs struct {
	fsutil.FsBackend
	collisions int
}

func (n *nameCollisionFs) Mkdir(path string, perm os.FileMode) error {
	if n.collisions > 0 {
		n.collisions--
		return os.ErrExist
	}
	return n.FsBackend.Mkdir(path, perm)
}

func (n *nameCollisionFs) OpenFile(path string, flag int, perm os.FileMode) (fsutil.File, error) {
	if n.collisions > 0 {
		n.collisions--
		return nil, os.ErrExist
	}
	return n.FsBackend.OpenFile(path, flag, perm)
}

func (f *fsSuite) TestCreateTemp(c *check.C) {
	file, err := f.fs.CreateTemp("", "test.*.png", 0o666)
	c.Assert(err, check.IsNil)
	name := filepath.Base(file.Name())
	c.Assert(file.Close(), check.IsNil)
	c.Check(name, check.Matches, `test\.[0-9]*\.png`)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-rw-r-- " + name})
}

func (f *fsSuite) TestCreateTempBadPattern(c *check.C) {
	file, err := f.fs.CreateTemp("", "dir/temp", os.ModePerm)
	c.Assert(err, check.ErrorMatches, "createtemp dir/temp: pattern contains path separator")
	c.Check(file, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestCreateTempCollisions(c *check.C) {
	fs := fsutil.Fs{FsBackend: &nameCollisionFs{f.fs.FsBackend, 5}}
	file, err := fs.CreateTemp("", "temp", 0o644)
	c.Assert(err, check.IsNil)
	name := filepath.Base(file.Name())
	c.Assert(file.Close(), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-r--r-- " + name})
	c.Assert(fs.Remove(name), check.IsNil)

	fs = fsutil.Fs{FsBackend: &nameCollisionFs{f.fs.FsBackend, 10000}}
	file, err = fs.CreateTemp("", "temp*.png", os.ModePerm)
	c.Assert(err, check.ErrorMatches, `createtemp temp\*.png: file already exists`)
	c.Check(file, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestWriteFile(c *check.C) {
	c.Assert(f.fs.WriteFile("file", []byte("content"), 0o644), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-r--r-- file"})
	content, err := f.fs.ReadFile("/file")
	c.Assert(err, check.IsNil)
	c.Check(string(content), check.Equals, "content")

	c.Assert(f.fs.WriteFile("/file", []byte("new"), 0o500), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-r--r-- file"})
	content, err = f.fs.ReadFile("/file")
	c.Assert(err, check.IsNil)
	c.Check(string(content), check.Equals, "new")
}

func (f *fsSuite) TestWriteFileNoDirectory(c *check.C) {
	err := f.fs.WriteFile("/var/tmp/file", []byte("content"), 0o644)
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestAtomicWriteTo(c *check.C) {
	c.Assert(f.fs.AtomicWriteTo(strings.NewReader("content"), "/file", 0o400), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-r-------- file"})
	content, err := f.fs.ReadFile("file")
	c.Assert(err, check.IsNil)
	c.Check(string(content), check.Equals, "content")

	c.Assert(f.fs.AtomicWriteTo(strings.NewReader("new"), "file", 0o500), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-r-x------ file"})
	content, err = f.fs.ReadFile("/file")
	c.Assert(err, check.IsNil)
	c.Check(string(content), check.Equals, "new")
}

func (f *fsSuite) TestAtomicWriteToNoDirectory(c *check.C) {
	err := f.fs.AtomicWriteTo(strings.NewReader("content"), "/var/tmp/file", 0o644)
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestAtomicWriteSourceError(c *check.C) {
	expected := errors.New("fake error")
	err := f.fs.AtomicWriteTo(errorSource{expected}, "/file", 0o644)
	c.Check(err, testutil.ErrorIs, expected)
	c.Check(f.path, testutil.DirEquals, []string{})
}

type errorSource struct {
	err error
}

func (s errorSource) WriteTo(w io.Writer) (int64, error) {
	return 0, s.err
}

func (f *fsSuite) TestAtomicWriteRenameFailed(c *check.C) {
	c.Assert(f.fs.Mkdir("/file", os.ModePerm), check.IsNil)
	err := f.fs.AtomicWriteTo(strings.NewReader("content"), "/file", 0o644)
	c.Check(err, testutil.ErrorIs, os.ErrExist)

	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x file"})
	c.Check(filepath.Join(f.path, "file"), testutil.DirEquals, []string{})
}

func (f *fsSuite) TestReadFileNoDirectory(c *check.C) {
	content, err := f.fs.ReadFile("/var/tmp/file")
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
	c.Check(content, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestRemoveIfExists(c *check.C) {
	c.Assert(f.fs.WriteFile("file", []byte("content"), 0o644), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"-rw-r--r-- file"})

	err := f.fs.RemoveIfExists("file")
	c.Assert(err, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})

	err = f.fs.RemoveIfExists("file")
	c.Assert(err, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestRemoveIfExistsEmptyDir(c *check.C) {
	c.Assert(f.fs.Mkdir("empty", os.ModePerm), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x empty"})

	err := f.fs.RemoveIfExists("empty")
	c.Assert(err, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})

	err = f.fs.RemoveIfExists("empty")
	c.Assert(err, check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{})
}

func (f *fsSuite) TestRemoveIfExistsNonEmptyDir(c *check.C) {
	c.Assert(f.fs.Mkdir("nonempty", os.ModePerm), check.IsNil)
	c.Assert(f.fs.WriteFile("nonempty/file", nil, 0o666), check.IsNil)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x nonempty"})
	c.Check(filepath.Join(f.path, "nonempty"), testutil.DirEquals, []string{"-rw-rw-r-- file"})

	err := f.fs.RemoveIfExists("nonempty")
	c.Check(err, testutil.ErrorIs, syscall.ENOTEMPTY)
	c.Check(f.path, testutil.DirEquals, []string{"drwxrwxr-x nonempty"})
	c.Check(filepath.Join(f.path, "nonempty"), testutil.DirEquals, []string{"-rw-rw-r-- file"})
}
