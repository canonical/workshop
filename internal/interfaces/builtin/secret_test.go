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

package builtin

import (
	"slices"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
)

// secretSuite tests the behaviour of the built-in secret interface.
// Each test constructs its own interface value rather than sharing
// suite state, keeping the tests independent of each other.
type secretSuite struct{}

var _ = check.Suite(&secretSuite{})

// TestAutoConnectDefersToPolicy checks that the secret interface's
// AutoConnect does not override policy: the denial of auto-connection
// for secrets is enforced solely by the interface's base declaration.
func (s *secretSuite) TestAutoConnectDefersToPolicy(c *check.C) {
	iface := secretInterface{}
	c.Check(iface.AutoConnect(nil, nil), check.Equals, true)
}

// TestInterfaces checks that the secret interface is registered as a
// builtin interface. Registration happens as a side effect of this
// package's init, so this test asserts that importing the package is
// enough for the secret interface to be available by name.
func (s *secretSuite) TestInterfaces(c *check.C) {
	registered := slices.ContainsFunc(
		Interfaces(), func(i interfaces.Interface) bool {
			return i.Name() == "secret"
		})
	c.Check(
		registered, check.Equals, true,
		check.Commentf("secret interface is not registered as a builtin interface"),
	)
}

// TestName checks that the interface name is correct.
func (s *secretSuite) TestName(c *check.C) {
	iface := secretInterface{}
	c.Check(iface.Name(), check.Equals, "secret")
}
