package fsutil_test

import (
	"cmp"
	"io"
	"os"
	"time"

	"github.com/pkg/sftp"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/testutil"
)

type sftpSuite struct {
	path   string
	server *sftp.Server
	client *sftp.Client
	fs     fsutil.Fs
}

type conn struct {
	*io.PipeReader
	*io.PipeWriter
}

func (c conn) Close() error {
	return cmp.Or(c.PipeReader.Close(), c.PipeWriter.Close())
}

var _ = check.Suite(&sftpSuite{})

func (s *sftpSuite) SetUpTest(c *check.C) {
	s.path = c.MkDir()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	sc := conn{sr, sw}
	cc := conn{cr, cw}

	var err error
	s.server, err = sftp.NewServer(sc, sftp.WithServerWorkingDirectory(s.path))
	c.Assert(err, check.IsNil)
	go func() {
		c.Check(s.server.Serve(), check.IsNil)
	}()

	s.client, err = sftp.NewClientPipe(cc, cc)
	c.Assert(err, check.IsNil)
	s.fs = fsutil.NewSftpFs(s.client, 0o002)
}

func (s *sftpSuite) TearDownTest(c *check.C) {
	c.Check(s.fs.Close(), check.IsNil)
	c.Check(s.server.Close(), check.IsNil)
}

func (s *sftpSuite) TestMkdir(c *check.C) {
	c.Assert(s.fs.Mkdir("testdir", os.ModePerm), check.IsNil)
	c.Check(s.path, testutil.DirEquals, []string{"drwxrwxr-x testdir"})

	err := s.fs.Mkdir("testdir", os.ModePerm)
	c.Assert(err, testutil.ErrorIs, os.ErrExist)

	err = s.fs.Mkdir("notexist/dir", os.ModePerm)
	c.Assert(err, check.ErrorMatches, `mkdir notexist/dir: file does not exist`)
}

func (s *sftpSuite) TestMkdirChmodChown(c *check.C) {
	c.Assert(s.fs.MkdirChmodChown("testdir", 0o717, os.Geteuid(), os.Getegid()), check.IsNil)
	c.Check(s.path, testutil.DirEquals, []string{"drwx--xrwx testdir"})
}

func (s *sftpSuite) TestOpen(c *check.C) {
	file, err := s.fs.Open("notexist")
	c.Assert(err, check.ErrorMatches, `open notexist: file does not exist`)
	c.Check(file, check.IsNil)
}

func (s *sftpSuite) TestOpenFile(c *check.C) {
	file, err := s.fs.OpenFile("testfile", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	c.Assert(err, check.IsNil)
	c.Assert(file.Close(), check.IsNil)
	c.Check(s.path, testutil.DirEquals, []string{"-rw-r--r-- testfile"})

	file, err = s.fs.OpenFile("testfile2", os.O_RDWR|os.O_CREATE|os.O_EXCL, os.ModePerm)
	c.Assert(err, check.IsNil)
	c.Assert(file.Close(), check.IsNil)
	c.Check(s.path, testutil.DirEquals, []string{"-rw-r--r-- testfile", "-rwxrwxr-x testfile2"})

	file, err = s.fs.OpenFile("testfile2", os.O_RDWR|os.O_CREATE|os.O_EXCL, os.ModePerm)
	c.Assert(err, testutil.ErrorIs, os.ErrExist)
	c.Check(file, check.IsNil)

	file, err = s.fs.OpenFile("testfile", os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	c.Assert(err, check.IsNil)
	c.Assert(file.Close(), check.IsNil)
	c.Check(s.path, testutil.DirEquals, []string{"-rwxrwxr-x testfile", "-rwxrwxr-x testfile2"})
}

func (s *sftpSuite) TestSymlink(c *check.C) {
	err := s.fs.Symlink("testfile", "notexist/testlink")
	c.Assert(err, check.ErrorMatches, `symlink testfile notexist/testlink: file does not exist`)
}

func (s *sftpSuite) TestReadDir(c *check.C) {
	infos, err := s.fs.ReadDir("notexist")
	c.Assert(err, check.ErrorMatches, `open notexist: file does not exist`)
	c.Check(infos, check.IsNil)
}

func (s *sftpSuite) TestStat(c *check.C) {
	info, err := s.fs.Stat("notexist")
	c.Assert(err, check.ErrorMatches, `stat notexist: file does not exist`)
	c.Check(info, check.IsNil)
}

func (s *sftpSuite) TestLstat(c *check.C) {
	info, err := s.fs.Lstat("notexist")
	c.Assert(err, check.ErrorMatches, `lstat notexist: file does not exist`)
	c.Check(info, check.IsNil)
}

func (s *sftpSuite) TestReadlink(c *check.C) {
	link, err := s.fs.Readlink("notexist")
	c.Assert(err, check.ErrorMatches, `readlink notexist: file does not exist`)
	c.Check(link, check.Equals, "")
}

func (s *sftpSuite) TestRename(c *check.C) {
	err := s.fs.Rename("notexist", "newname")
	c.Assert(err, check.ErrorMatches, `rename notexist newname: file does not exist`)
}

func (s *sftpSuite) TestChmod(c *check.C) {
	err := s.fs.Chmod("notexist", 0o644)
	c.Assert(err, check.ErrorMatches, `chmod notexist: file does not exist`)
}

func (s *sftpSuite) TestChown(c *check.C) {
	err := s.fs.Chown("notexist", os.Geteuid(), os.Getegid())
	c.Assert(err, check.ErrorMatches, `chown notexist: file does not exist`)
}

func (s *sftpSuite) TestChtimes(c *check.C) {
	err := s.fs.Chtimes("notexist", time.Now(), time.Now())
	c.Assert(err, check.ErrorMatches, `chtimes notexist: file does not exist`)
}

func (s *sftpSuite) TestRemove(c *check.C) {
	err := s.fs.Remove("notexist")
	c.Assert(err, check.ErrorMatches, `remove notexist: file does not exist`)
}

func (s *sftpSuite) TestRemoveAll(c *check.C) {
	c.Assert(s.fs.RemoveAll("notexist"), check.IsNil)

	c.Assert(s.fs.Mkdir("restricted", 0), check.IsNil)
	err := s.fs.RemoveAll("restricted")
	c.Assert(err, check.ErrorMatches, "open restricted: permission denied")
}
