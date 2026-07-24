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

// machineIDSuite tests reading machine identifiers from files.
type machineIDSuite struct{}

var _ = check.Suite(&machineIDSuite{})

// TestMachineIDFromPathReturnsNotFoundForEmptyFile verifies that empty machine
// ID files return [ErrorMachineIDNotFound].
func (s *machineIDSuite) TestMachineIDFromPathReturnsNotFoundForEmptyFile(
	c *check.C,
) {
	path := filepath.Join(c.MkDir(), "machine-id")
	err := os.WriteFile(path, []byte("\n"), 0644)
	c.Assert(err, check.IsNil)

	machineID, err := machineIDFromPath(path)

	c.Check(machineID, check.Equals, "")
	c.Check(errors.Is(err, ErrorMachineIDNotFound), check.Equals, true)
}

// TestMachineIDFromPathReturnsNotFoundForMissingFile verifies that a missing
// machine ID file returns [ErrorMachineIDNotFound].
func (s *machineIDSuite) TestMachineIDFromPathReturnsNotFoundForMissingFile(
	c *check.C,
) {
	path := filepath.Join(c.MkDir(), "machine-id")

	machineID, err := machineIDFromPath(path)

	c.Check(machineID, check.Equals, "")
	c.Check(errors.Is(err, ErrorMachineIDNotFound), check.Equals, true)
}

// TestMachineIDFromPathReturnsTrimmedID verifies that a machine ID is returned
// without its trailing newline.
func (s *machineIDSuite) TestMachineIDFromPathReturnsTrimmedID(c *check.C) {
	path := filepath.Join(c.MkDir(), "machine-id")
	err := os.WriteFile(path, []byte("0123456789abcdef0123456789abcdef\n"), 0644)
	c.Assert(err, check.IsNil)

	machineID, err := machineIDFromPath(path)

	c.Check(err, check.IsNil)
	c.Check(machineID, check.Equals, "0123456789abcdef0123456789abcdef")
}
