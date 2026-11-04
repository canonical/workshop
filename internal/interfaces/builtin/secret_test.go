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

package builtin_test

import (
	"slices"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// secretSuite tests the behaviour of the built-in secret interface.
type secretSuite struct {
	iface     interfaces.Interface
	projectId string
}

var _ = check.Suite(&secretSuite{
	iface: builtin.MustInterface("secret"),
})

func (s *secretSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
}

// TestAutoConnectDefersToPolicy checks that the secret interface's
// AutoConnect does not override policy: the denial of auto-connection
// for secrets is enforced solely by the interface's base declaration.
func (s *secretSuite) TestAutoConnectDefersToPolicy(c *check.C) {
	c.Check(s.iface.AutoConnect(nil, nil), check.Equals, true)
}

// TestInterfaces checks that the secret interface is registered as a
// builtin interface. Registration happens as a side effect of this
// package's init, so this test asserts that importing the package is
// enough for the secret interface to be available by name.
func (s *secretSuite) TestInterfaces(c *check.C) {
	registered := slices.ContainsFunc(
		builtin.Interfaces(), func(i interfaces.Interface) bool {
			return i.Name() == "secret"
		})
	c.Check(
		registered, check.Equals, true,
		check.Commentf("secret interface is not registered as a builtin interface"),
	)
}

// TestName checks that the interface name is correct.
func (s *secretSuite) TestName(c *check.C) {
	c.Check(s.iface.Name(), check.Equals, "secret")
}

// TestBeforePrepareSlotDefaultsCollection checks that a valid secret slot
// which omits the optional collection is normalized to use the default host
// keyring collection.
func (s *secretSuite) TestBeforePrepareSlotDefaultsCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{
			"service":  "github",
			"username": "tlm",
		},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "default")
}

// TestBeforePrepareSlotKeepsCollection checks that an explicitly selected host
// keyring collection is accepted and preserved during slot preparation.
func (s *secretSuite) TestBeforePrepareSlotKeepsCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": "non-default-test",
		"attributes": map[string]any{"service": "ollama"},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "non-default-test")
}

// TestBeforePrepareSlotRejectsUnsupportedProvider checks that the builtin
// interface only accepts secret slots owned by the system SDK.
func (s *secretSuite) TestBeforePrepareSlotRejectsUnsupportedProvider(c *check.C) {
	slot := secretSlot(nil)
	slot.Sdk = &sdk.Info{Name: "producer", Type: sdk.Regular}

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret interface slots are only supported by the system SDK`)
}

// TestBeforePrepareSlotRejectsUnsupportedKeys checks that the secret schema
// does not silently accept unknown top-level slot keys.
func (s *secretSuite) TestBeforePrepareSlotRejectsUnsupportedKeys(c *check.C) {
	slot := secretSlot(map[string]any{"unknown": "value"})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `unsupported keys found in secret slot`)
}

// TestBeforePrepareSlotRequiresAttributes checks that every secret slot
// defines the keyring search attributes used to locate its secret.
func (s *secretSuite) TestBeforePrepareSlotRequiresAttributes(c *check.C) {
	slot := secretSlot(nil)

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret interface slot must contain "attributes"`)
}

// TestBeforePrepareSlotRequiresAttributesMap checks that keyring search
// attributes are represented as a mapping rather than a scalar value.
func (s *secretSuite) TestBeforePrepareSlotRequiresAttributesMap(c *check.C) {
	slot := secretSlot(map[string]any{"attributes": "service=github"})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret interface slot "attributes" must be a map`)
}

// TestBeforePrepareSlotRequiresAtLeastOneAttribute checks that a secret slot
// cannot perform an unconstrained keyring lookup.
func (s *secretSuite) TestBeforePrepareSlotRequiresAtLeastOneAttribute(c *check.C) {
	slot := secretSlot(map[string]any{"attributes": map[string]any{}})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `at least one attribute must be defined for a secret slot`)
}

// TestBeforePrepareSlotRequiresNamedAttributes checks that a keyring search
// attribute is named, so a malformed slot is rejected when the SDK is loaded
// rather than failing later during retrieval.
func (s *secretSuite) TestBeforePrepareSlotRequiresNamedAttributes(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{" ": "github"},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret slot attribute name cannot be empty`)
}

// TestBeforePrepareSlotRequiresStringAttributeValues checks that every keyring
// search attribute has the string representation required by the provider.
func (s *secretSuite) TestBeforePrepareSlotRequiresStringAttributeValues(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": int64(42)},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `attribute "service" must be of type string`)
}

// TestBeforePrepareSlotRequiresStringCollection checks that an optional
// keyring collection selector is represented as a string.
func (s *secretSuite) TestBeforePrepareSlotRequiresStringCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": int64(42),
		"attributes": map[string]any{"service": "github"},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret slot collection key must have a string value`)
}

// TestBeforePrepareSlotRequiresNonEmptyCollection checks that an explicitly
// selected keyring collection contains a usable name.
func (s *secretSuite) TestBeforePrepareSlotRequiresNonEmptyCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": " ",
		"attributes": map[string]any{"service": "github"},
	})

	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `secret slot collection value must contain a non empty string value`)
}

// TestMaintainConnectionUnchanged checks that a connection is kept while the
// slot's host keyring lookup is unchanged.
func (s *secretSuite) TestMaintainConnectionUnchanged(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github", "username": "tlm"},
		"collection": "login",
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github", "username": "tlm"},
		"collection": "login",
	}

	c.Check(s.iface.MaintainConnection(nil, nil, slot, old),
		check.Equals, true)
}

// TestMaintainConnectionIgnoresPlugAttrs checks that the plug side, which
// carries no definition attributes, does not affect the decision.
func (s *secretSuite) TestMaintainConnectionIgnoresPlugAttrs(c *check.C) {
	plug := &sdk.PlugInfo{Attrs: map[string]any{"unused": "value"}}
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github"},
	}

	c.Check(s.iface.MaintainConnection(plug, nil, slot, old),
		check.Equals, true)
}

// TestMaintainConnectionAttributeValueChanged checks that a changed keyring
// search attribute drops the connection.
func (s *secretSuite) TestMaintainConnectionAttributeValueChanged(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "gitlab"},
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github"},
	}

	c.Check(s.iface.MaintainConnection(nil, nil, slot, old),
		check.Equals, false)
}

// TestMaintainConnectionAttributeAdded checks that an added keyring search
// attribute drops the connection.
func (s *secretSuite) TestMaintainConnectionAttributeAdded(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github", "username": "tlm"},
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github"},
	}

	c.Check(s.iface.MaintainConnection(nil, nil, slot, old),
		check.Equals, false)
}

// TestMaintainConnectionAttributeRemoved checks that a removed keyring search
// attribute drops the connection.
func (s *secretSuite) TestMaintainConnectionAttributeRemoved(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github", "username": "tlm"},
	}

	c.Check(s.iface.MaintainConnection(nil, nil, slot, old),
		check.Equals, false)
}

// TestMaintainConnectionCollectionChanged checks that a changed keyring
// collection drops the connection.
func (s *secretSuite) TestMaintainConnectionCollectionChanged(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
		"collection": "other",
	})
	old := map[string]any{
		"attributes": map[string]any{"service": "github"},
		"collection": "login",
	}

	c.Check(s.iface.MaintainConnection(nil, nil, slot, old),
		check.Equals, false)
}

// TestMaintainConnectionMissingSnapshotChanged checks that an absent recorded
// snapshot against a populated slot counts as a change, and that the
// comparison is safe with a nil map.
func (s *secretSuite) TestMaintainConnectionMissingSnapshotChanged(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
	})

	c.Check(s.iface.MaintainConnection(nil, nil, slot, nil),
		check.Equals, false)
}

// TestMaintainConnectionMissingSnapshotUnchanged checks that absent snapshots
// on both sides compare equal, exercising the nil-map path without panicking.
func (s *secretSuite) TestMaintainConnectionMissingSnapshotUnchanged(c *check.C) {
	slot := secretSlot(nil)

	c.Check(s.iface.MaintainConnection(nil, nil, slot, nil),
		check.Equals, true)
}

func secretSlot(attrs map[string]any) *sdk.SlotInfo {
	return &sdk.SlotInfo{
		Sdk:       &sdk.Info{Name: "system", Type: sdk.System},
		Name:      "secret",
		Interface: "secret",
		Attrs:     attrs,
	}
}

func (s *secretSuite) TestSecretPlugSupportsAllRuntimes(c *check.C) {
	yaml := `name: consumer
base: ubuntu@22.04
plugs:
 secret:
  interface: secret
`
	for _, runtime := range []workshop.Runtime{workshop.RuntimeLXDContainer, workshop.RuntimeLXDVM} {
		info, warning := checkCompatible(c, yaml, s.projectId, "ws", runtime)
		c.Check(info.Plugs["secret"], check.NotNil)
		c.Check(warning, check.IsNil)
	}
}

func (s *secretSuite) TestSecretSlotSupportsAllRuntimes(c *check.C) {
	yaml := `name: system
type: system
base: ubuntu@22.04
slots:
 secret:
  interface: secret
  attributes:
   service: github
`
	for _, runtime := range []workshop.Runtime{workshop.RuntimeLXDContainer, workshop.RuntimeLXDVM} {
		info, warning := checkCompatible(c, yaml, s.projectId, "ws", runtime)
		c.Check(info.Slots["secret"], check.NotNil)
		c.Check(warning, check.IsNil)
	}
}
