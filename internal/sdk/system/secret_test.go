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
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
)

type secretSuite struct{}

var _ = check.Suite(&secretSuite{})

// TestBeforePrepareSecretSlotDefaultsCollection checks that a valid secret slot
// which omits the optional collection is normalized to use the default host
// keyring collection.
func (s *secretSuite) TestBeforePrepareSecretSlotDefaultsCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{
			"service":  "github",
			"username": "tlm",
		},
	})

	err := system.BeforePrepareSecretSlot(slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "default")
}

// TestBeforePrepareSecretSlotKeepsCollection checks that an explicitly
// selected host keyring collection is accepted and preserved during slot
// preparation.
func (s *secretSuite) TestBeforePrepareSecretSlotKeepsCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": "non-default-test",
		"attributes": map[string]any{"service": "ollama"},
	})

	err := system.BeforePrepareSecretSlot(slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "non-default-test")
}

// TestBeforePrepareSecretSlotRejectsUnsupportedKeys checks that the system SDK
// secret schema does not silently accept unknown top-level slot keys.
func (s *secretSuite) TestBeforePrepareSecretSlotRejectsUnsupportedKeys(c *check.C) {
	slot := secretSlot(map[string]any{"unknown": "value"})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `unsupported keys found in secret slot`)
}

// TestBeforePrepareSecretSlotRequiresAttributes checks that every secret slot
// defines the keyring search attributes used to locate its secret.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresAttributes(c *check.C) {
	slot := secretSlot(nil)

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `secret interface slot must contain "attributes"`)
}

// TestBeforePrepareSecretSlotRequiresAttributesMap checks that keyring search
// attributes are represented as a mapping rather than a scalar value.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresAttributesMap(c *check.C) {
	slot := secretSlot(map[string]any{"attributes": "service=github"})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `secret interface slot "attributes" must be a map`)
}

// TestBeforePrepareSecretSlotRequiresAtLeastOneAttribute checks that a secret
// slot cannot perform an unconstrained keyring lookup.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresAtLeastOneAttribute(c *check.C) {
	slot := secretSlot(map[string]any{"attributes": map[string]any{}})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `at least one attribute must be defined for a secret slot`)
}

// TestBeforePrepareSecretSlotRequiresStringAttributeValues checks that every
// keyring search attribute has the string representation required by the
// secret provider.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresStringAttributeValues(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": int64(42)},
	})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `attribute "service" must be of type string`)
}

// TestBeforePrepareSecretSlotRequiresStringCollection checks that an optional
// keyring collection selector is represented as a string.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresStringCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": int64(42),
		"attributes": map[string]any{"service": "github"},
	})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `secret slot collection key must have a string value`)
}

// TestBeforePrepareSecretSlotRequiresNonEmptyCollection checks that an
// explicitly selected keyring collection contains a usable name.
func (s *secretSuite) TestBeforePrepareSecretSlotRequiresNonEmptyCollection(c *check.C) {
	slot := secretSlot(map[string]any{
		"collection": " ",
		"attributes": map[string]any{"service": "github"},
	})

	err := system.BeforePrepareSecretSlot(slot)
	c.Check(err, check.ErrorMatches, `secret slot collection value must contain a non empty string value`)
}

func secretSlot(attrs map[string]any) *sdk.SlotInfo {
	return &sdk.SlotInfo{
		Sdk:       &sdk.Info{Name: "system", Type: sdk.System},
		Name:      "secret",
		Interface: "secret",
		Attrs:     attrs,
	}
}
