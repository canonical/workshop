package x11_test

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/x11"
)

func Test(t *testing.T) { check.TestingT(t) }

type X11TestSuit struct {
	restore func()
}

var _ = check.Suite(&X11TestSuit{})

func FakeWorkshopdRunDir(dir string) func() {
	old := dirs.WorkshopdRunDir
	dirs.WorkshopdRunDir = dir
	return func() {
		dirs.WorkshopdRunDir = old
	}
}

func (x *X11TestSuit) SetUpTest(c *check.C) {
	x.restore = FakeWorkshopdRunDir(c.MkDir())
}

func (x *X11TestSuit) TearDownTest(c *check.C) {
	x.restore()
}

func (x *X11TestSuit) TestMigrateXAuthority(c *check.C) {
	user, err := user.Current()
	c.Assert(err, check.IsNil)

	xf, err := os.Create(filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.IsNil)
	defer xf.Close()

	_, err = xf.Write(constructTestCookie(displayNone))
	c.Assert(err, check.IsNil)

	err = x11.MigrateXauthority(user, filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.IsNil)

	c.Assert(filepath.Join(dirs.WorkshopdRunDir, user.Uid, "Xauthority", ".Xauthority"), testutil.FilePresent)
}

func (x *X11TestSuit) TestMigrateXAuthorityInvalidFile(c *check.C) {
	user, err := user.Current()
	c.Assert(err, check.IsNil)

	xf, err := os.Create(filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.IsNil)
	defer xf.Close()

	err = x11.MigrateXauthority(user, filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.ErrorMatches, "invalid Xauthority file: EOF")

	c.Assert(filepath.Join(dirs.WorkshopdRunDir, user.Uid, "Xauthority", ".Xauthority"), testutil.FileAbsent)
}

func (x *X11TestSuit) TestMigrateXAuthorityOwnershipFail(c *check.C) {
	user, err := user.Lookup("root")
	c.Assert(err, check.IsNil)

	xf, err := os.Create(filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.IsNil)
	defer xf.Close()

	err = x11.MigrateXauthority(user, filepath.Join(dirs.WorkshopdRunDir, ".workshop-Xauthority"))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "Xauthority file isn't owned by the current user "+user.Uid)
}
