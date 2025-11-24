// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2015-2017 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package ifacetest_test

import (
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/sdk"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type SpecificationSuite struct {
	iface    *ifacetest.TestInterface
	spec     *ifacetest.Specification
	plugInfo *sdk.PlugInfo
	plug     *interfaces.ConnectedPlug
	slotInfo *sdk.SlotInfo
	slot     *interfaces.ConnectedSlot
}

var _ = check.Suite(&SpecificationSuite{
	iface: &ifacetest.TestInterface{
		InterfaceName: "test",
		TestConnectedPlugCallback: func(spec *ifacetest.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
			spec.AddSnippet("connected-plug")
			return nil
		},
		TestConnectedSlotCallback: func(spec *ifacetest.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
			spec.AddSnippet("connected-slot")
			return nil
		},
		TestPermanentPlugCallback: func(spec *ifacetest.Specification, plug *sdk.PlugInfo) error {
			spec.AddSnippet("permanent-plug")
			return nil
		},
		TestPermanentSlotCallback: func(spec *ifacetest.Specification, slot *sdk.SlotInfo) error {
			spec.AddSnippet("permanent-slot")
			return nil
		},
	},
	plugInfo: &sdk.PlugInfo{
		Sdk:       &sdk.Info{Name: "sdk"},
		Name:      "name",
		Interface: "test",
	},
	slotInfo: &sdk.SlotInfo{
		Sdk:       &sdk.Info{Name: "sdk"},
		Name:      "name",
		Interface: "test",
	},
})

func (s *SpecificationSuite) SetUpTest(c *check.C) {
	s.spec = &ifacetest.Specification{}
	s.plug = interfaces.NewConnectedPlug(s.plugInfo, nil, nil)
	s.slot = interfaces.NewConnectedSlot(s.slotInfo, nil, nil)
}

// AddSnippet is not broken
func (s *SpecificationSuite) TestAddSnippet(c *check.C) {
	s.spec.AddSnippet("hello")
	s.spec.AddSnippet("world")
	c.Assert(s.spec.Snippets, check.DeepEquals, []string{"hello", "world"})
}

// The Specification can be used through the interfaces.Specification interface
func (s *SpecificationSuite) SpecificationIface(c *check.C) {
	var r interfaces.Specification = s.spec
	c.Assert(r.AddConnectedPlug(s.iface, s.plug, s.slot), check.IsNil)
	c.Assert(r.AddConnectedSlot(s.iface, s.plug, s.slot), check.IsNil)
	c.Assert(r.AddPermanentPlug(s.iface, s.plugInfo), check.IsNil)
	c.Assert(r.AddPermanentSlot(s.iface, s.slotInfo), check.IsNil)
	c.Assert(s.spec.Snippets, check.DeepEquals, []string{
		"connected-plug", "connected-slot", "permanent-plug", "permanent-slot",
	})
}
