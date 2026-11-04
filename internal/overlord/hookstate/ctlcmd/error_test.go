// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package ctlcmd_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
)

// commandExitCodeErrorSuite checks the command exit-code metadata error.
type commandExitCodeErrorSuite struct{}

var _ = check.Suite(&commandExitCodeErrorSuite{})

// TestError checks that exit-code metadata produces no diagnostic text.
func (commandExitCodeErrorSuite) TestError(c *check.C) {
	err := ctlcmd.CommandExitCodeError{ExitCode: 2}

	c.Check(err.Error(), check.Equals, "")
}
