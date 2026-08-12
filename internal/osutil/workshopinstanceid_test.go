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

package osutil

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/check.v1"
)

// workshopInstanceIDSuite tests reading workshop instance identifiers from
// files.
type workshopInstanceIDSuite struct{}

var _ = check.Suite(&workshopInstanceIDSuite{})

// TestFromPathReturnsNotFoundForEmptyFile verifies that empty workshop instance
// ID files return [ErrorWorkshopInstanceIDNotFound].
func (workshopInstanceIDSuite) TestFromPathReturnsNotFoundForEmptyFile(
	c *check.C,
) {
	path := filepath.Join(c.MkDir(), "machine-id")
	err := os.WriteFile(path, []byte("\n"), 0644)
	c.Assert(err, check.IsNil)

	instanceID, err := workshopInstanceIDFromPath(path)

	c.Check(instanceID, check.Equals, "")
	c.Check(errors.Is(err, ErrorWorkshopInstanceIDNotFound), check.Equals, true)
}

// TestFromPathReturnsNotFoundForMissingFile verifies that a missing workshop
// instance ID file returns [ErrorWorkshopInstanceIDNotFound].
func (workshopInstanceIDSuite) TestFromPathReturnsNotFoundForMissingFile(
	c *check.C,
) {
	path := filepath.Join(c.MkDir(), "machine-id")

	instanceID, err := workshopInstanceIDFromPath(path)

	c.Check(instanceID, check.Equals, "")
	c.Check(errors.Is(err, ErrorWorkshopInstanceIDNotFound), check.Equals, true)
}

// TestFromPathReturnsTrimmedID verifies that a workshop instance ID is
// returned without its trailing newline.
func (workshopInstanceIDSuite) TestFromPathReturnsTrimmedID(c *check.C) {
	path := filepath.Join(c.MkDir(), "machine-id")
	err := os.WriteFile(path, []byte("0123456789abcdef0123456789abcdef\n"), 0644)
	c.Assert(err, check.IsNil)

	instanceID, err := workshopInstanceIDFromPath(path)

	c.Check(err, check.IsNil)
	c.Check(instanceID, check.Equals, "0123456789abcdef0123456789abcdef")
}
