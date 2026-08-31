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
	"github.com/canonical/workshop/internal/sdk"
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

// TestBeforePrepareSlotDelegatesToSystemSdk checks that the builtin interface
// routes slot preparation through the system SDK implementation, including
// normalization performed there.
func (s *secretSuite) TestBeforePrepareSlotDelegatesToSystemSdk(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{
			"service":  "github",
			"username": "tlm",
		},
	})

	err := (secretInterface{}).BeforePrepareSlot(slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "default")
}

// TestBeforePrepareSlotRejectsUnsupportedProvider checks that the builtin
// interface only routes secret slots owned by the system SDK.
func (s *secretSuite) TestBeforePrepareSlotRejectsUnsupportedProvider(c *check.C) {
	slot := secretSlot(nil)
	slot.Sdk = &sdk.Info{Name: "producer", Type: sdk.Regular}

	err := (secretInterface{}).BeforePrepareSlot(slot)
	c.Check(err, check.ErrorMatches, `secret interface slots are only supported by the system SDK`)
}

func secretSlot(attrs map[string]any) *sdk.SlotInfo {
	return &sdk.SlotInfo{
		Sdk:       &sdk.Info{Name: "system", Type: sdk.System},
		Name:      "secret",
		Interface: "secret",
		Attrs:     attrs,
	}
}
