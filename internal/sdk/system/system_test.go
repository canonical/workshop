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

package system_test

import (
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/testutil"
)

type systemSdk struct{}

var _ = check.Suite(&systemSdk{})

func Test(t *testing.T) { check.TestingT(t) }

func (f *systemSdk) SetUpSuite(c *check.C) {
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)
}

func (s *systemSdk) TestRetrieveSystemSdkSuccess(c *check.C) {
	setup := sdk.Setup{
		Name:     sdk.System.String(),
		Source:   sdk.SystemSource,
		Revision: system.SystemSdkRevision,
		Sha3_384: system.SystemSdkDigest,
	}
	file, err := os.Create(setup.Filepath())
	c.Assert(err, check.IsNil)
	defer file.Close()

	var done, total int64
	report := &progress.Reporter{Name: "1", Report: func(label string, d, t int64) {
		done = d
		total = t
	}}

	err = system.RetrieveSystemSdk(file, setup, report)
	c.Assert(err, check.IsNil)
	c.Check(int(done), testutil.IntGreaterThan, 0)
	c.Check(int(total), testutil.IntGreaterThan, 0)
	c.Check(done, check.Equals, total)

	file.Close()
	file, err = os.Open(setup.Filepath())
	c.Assert(err, check.IsNil)

	hash := sha3.New384()
	_, err = file.WriteTo(hash)
	c.Assert(err, check.IsNil)

	digest := hex.EncodeToString(hash.Sum(nil))
	c.Check(digest, check.Equals, system.SystemSdkDigest, check.Commentf("system SDK revision needs updating"))
	c.Check(system.SystemSdkRevision, check.Equals, sdk.R(2))
}

func (s *systemSdk) TestRetrieveSystemSdkWrongRevision(c *check.C) {
	setup := sdk.Setup{
		Name:     sdk.System.String(),
		Source:   sdk.SystemSource,
		Revision: sdk.R(system.SystemSdkRevision.N - 1),
		Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
	}
	err := system.RetrieveSystemSdk(nil, setup, nil)
	c.Check(err, check.ErrorMatches, fmt.Sprintf(`system SDK \(%s\) not available`, setup.Revision))

	setup.Revision.N += 2
	err = system.RetrieveSystemSdk(nil, setup, nil)
	c.Check(err, check.ErrorMatches, fmt.Sprintf(`system SDK \(%s\) not available`, setup.Revision))
}
