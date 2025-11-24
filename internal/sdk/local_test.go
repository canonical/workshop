package sdk_test

import (
	"crypto/sha3"
	"encoding/hex"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type localSdk struct {
	target string
	user   *user.User
	uid    sys.UserID
	gid    sys.GroupID
}

var _ = check.Suite(&localSdk{})

func (s *localSdk) SetUpTest(c *check.C) {
	s.target = c.MkDir()

	var err error
	s.user, err = user.Current()
	c.Assert(err, check.IsNil)

	s.uid, s.gid, err = osutil.UidGid(s.user)
	c.Assert(err, check.IsNil)
}

func (s *localSdk) createSource(c *check.C, contents string) string {
	source := filepath.Join(c.MkDir(), "source")
	c.Assert(os.Mkdir(source, 0o755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(source, "contents"), []byte(contents), 0o644), check.IsNil)
	return source
}

func (s *localSdk) createRevision(c *check.C, revision, contents string) string {
	c.Assert(os.Mkdir(filepath.Join(s.target, revision), 0o755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(s.target, revision, "contents"), []byte(contents), 0o644), check.IsNil)

	digest, err := osutil.HashDirEntries(sha3.New384(), filepath.Join(s.target, revision))
	c.Assert(err, check.IsNil)
	name := hex.EncodeToString(digest)

	c.Assert(os.Rename(filepath.Join(s.target, revision), filepath.Join(s.target, name)), check.IsNil)
	c.Assert(os.Symlink(name, filepath.Join(s.target, revision)), check.IsNil)

	return name
}

func dirEntries(names ...string) []string {
	slices.Sort(names)
	for i, name := range names {
		r, err := sdk.ParseRevision(name)
		if err == nil && r.Local() && r.String() == name {
			names[i] = "Lrwxrwxrwx " + name
		} else {
			names[i] = "drwxr-xr-x " + name
		}
	}
	return names
}

func (s *localSdk) TestCommitSuccess(c *check.C) {
	one := s.createSource(c, "1")
	revision, digest1, err := sdk.CommitRevision(s.user, one, s.target, sdk.Revision{})
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-1))

	checkRev1 := func() {
		c.Check(filepath.Join(s.target, "x1"), testutil.SymlinkTargetEquals, digest1)
		c.Check(filepath.Join(s.target, digest1), testutil.DirEquals, []string{"-rw-r--r-- contents"})
		c.Check(filepath.Join(s.target, digest1, "contents"), testutil.FileEquals, "1")
	}
	c.Check(s.target, testutil.DirEquals, dirEntries(digest1, "x1"))
	checkRev1()

	two := s.createSource(c, "2")
	revision, digest2, err := sdk.CommitRevision(s.user, two, s.target, revision)
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-2))

	checkRev2 := func() {
		c.Check(filepath.Join(s.target, "x2"), testutil.SymlinkTargetEquals, digest2)
		c.Check(filepath.Join(s.target, digest2), testutil.DirEquals, []string{"-rw-r--r-- contents"})
		c.Check(filepath.Join(s.target, digest2, "contents"), testutil.FileEquals, "2")
	}
	c.Check(s.target, testutil.DirEquals, dirEntries(digest1, digest2, "x1", "x2"))
	checkRev1()
	checkRev2()

	oneAgain := s.createSource(c, "1")
	revision, digest1Again, err := sdk.CommitRevision(s.user, oneAgain, s.target, revision)
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-1))
	c.Check(digest1Again, check.Equals, digest1)

	c.Check(s.target, testutil.DirEquals, dirEntries(digest1, digest2, "x1", "x2"))
	checkRev1()
	checkRev2()

	hash1, err := osutil.HashDirEntries(sha3.New384(), filepath.Join(s.target, digest1))
	c.Assert(err, check.IsNil)
	c.Check(hex.EncodeToString(hash1), check.Equals, digest1)
	hash2, err := osutil.HashDirEntries(sha3.New384(), filepath.Join(s.target, digest2))
	c.Assert(err, check.IsNil)
	c.Check(hex.EncodeToString(hash2), check.Equals, digest2)
}

func (s *localSdk) TestCommitIncreasesRevision(c *check.C) {
	s.createRevision(c, "x11", "11")
	s.createRevision(c, "x111", "111")
	s.createRevision(c, "x1111", "1111")

	source := s.createSource(c, "1112")
	revision, _, err := sdk.CommitRevision(s.user, source, s.target, sdk.R(-111))
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-1112))
}

func (s *localSdk) TestCommitIgnoresUnusualRevisions(c *check.C) {
	s.createRevision(c, "10", "10")
	s.createRevision(c, "-11", "11")
	s.createRevision(c, "x+12", "12")
	s.createRevision(c, "x013", "13")

	source := s.createSource(c, "1")
	revision, _, err := sdk.CommitRevision(s.user, source, s.target, sdk.Revision{})
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-1))
}

func (s *localSdk) TestCommitRemovesOldRevisions(c *check.C) {
	digest42 := s.createRevision(c, "x42", "42")
	s.createRevision(c, "x43", "43")
	digest44 := s.createRevision(c, "x44", "44")

	t3 := time.Now()
	t2 := t3.Add(-time.Minute)
	t1 := t2.Add(-time.Minute)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x42"), time.Time{}, t2), check.IsNil)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x43"), time.Time{}, t1), check.IsNil)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x44"), time.Time{}, t3), check.IsNil)

	source := s.createSource(c, "45")
	revision, digest45, err := sdk.CommitRevision(s.user, source, s.target, sdk.R(-44))
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-45))
	c.Check(s.target, testutil.DirEquals, dirEntries(digest42, digest44, digest45, "x42", "x44", "x45"))
}

func (s *localSdk) TestCommitKeepsInstalled(c *check.C) {
	s.createRevision(c, "x42", "42")
	digest43 := s.createRevision(c, "x43", "43")
	digest44 := s.createRevision(c, "x44", "44")

	t3 := time.Now()
	t2 := t3.Add(-time.Minute)
	t1 := t2.Add(-time.Minute)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x42"), time.Time{}, t2), check.IsNil)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x43"), time.Time{}, t1), check.IsNil)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x44"), time.Time{}, t3), check.IsNil)

	source := s.createSource(c, "45")
	revision, digest45, err := sdk.CommitRevision(s.user, source, s.target, sdk.R(-43))
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-45))
	c.Check(s.target, testutil.DirEquals, dirEntries(digest43, digest44, digest45, "x43", "x44", "x45"))
}

func (s *localSdk) TestCommitExistingUpdatesTimestamp(c *check.C) {
	digest41 := s.createRevision(c, "x41", "41")
	digest42 := s.createRevision(c, "x42", "42")
	digest43 := s.createRevision(c, "x43", "43")
	digest44 := s.createRevision(c, "x44", "44")

	old := time.Now().Add(-time.Minute)
	c.Assert(sys.Lchtimes(filepath.Join(s.target, "x43"), time.Time{}, old), check.IsNil)
	info, err := os.Lstat(filepath.Join(s.target, "x43"))
	c.Assert(err, check.IsNil)
	c.Check(info.ModTime().Compare(old), check.Equals, 0)

	source := s.createSource(c, "43")
	revision, digest43Again, err := sdk.CommitRevision(s.user, source, s.target, sdk.R(-44))
	c.Assert(err, check.IsNil)
	c.Check(revision, check.Equals, sdk.R(-43))
	c.Check(digest43Again, check.Equals, digest43)
	c.Check(s.target, testutil.DirEquals, dirEntries(digest41, digest42, digest43, digest44, "x41", "x42", "x43", "x44"))

	info, err = os.Lstat(filepath.Join(s.target, "x43"))
	c.Assert(err, check.IsNil)
	c.Check(info.ModTime().Compare(old), check.Equals, 1)
}
