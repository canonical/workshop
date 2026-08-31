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

type slotSuite struct{}

var _ = check.Suite(&slotSuite{})

// TestBeforePrepareSlotRejectsNonSystemSdk checks that the system SDK
// preparation entry point cannot be used to process slots owned by another
// SDK type.
func (s *slotSuite) TestBeforePrepareSlotRejectsNonSystemSdk(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
	})
	slot.Sdk = &sdk.Info{Name: "producer", Type: sdk.Regular}

	err := system.BeforePrepareSlot(slot)
	c.Check(err, check.ErrorMatches, `system SDK slot preparation requires a system SDK slot`)
}

// TestBeforePrepareSlotDelegatesSecretSlot checks that secret slots are routed
// through the system SDK's secret-specific preparation logic, including its
// attribute normalization.
func (s *slotSuite) TestBeforePrepareSlotDelegatesSecretSlot(c *check.C) {
	slot := secretSlot(map[string]any{
		"attributes": map[string]any{"service": "github"},
	})

	err := system.BeforePrepareSlot(slot)
	c.Assert(err, check.IsNil)
	c.Check(slot.Attrs["collection"], check.Equals, "default")
}

// TestBeforePrepareSlotIgnoresUnhandledInterface checks that system SDK slots
// without interface-specific preparation logic are accepted without mutation.
func (s *slotSuite) TestBeforePrepareSlotIgnoresUnhandledInterface(c *check.C) {
	slot := secretSlot(nil)
	slot.Interface = "mount"

	err := system.BeforePrepareSlot(slot)
	c.Check(err, check.IsNil)
	c.Check(slot.Attrs, check.IsNil)
}
