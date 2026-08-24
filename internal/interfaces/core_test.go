// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
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

package interfaces_test

import (
	"fmt"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

func Test(t *testing.T) {
	TestingT(t)
}

type CoreSuite struct {
	testutil.BaseTest
	projectId string
}

var _ = Suite(&CoreSuite{})

func (s *CoreSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {}))
	s.projectId = "42424242"
}

func (s *CoreSuite) TearDownTest(c *C) {
	s.BaseTest.TearDownTest(c)
}

// PlugRef.String works as expected
func (s *CoreSuite) TestPlugRefString(c *C) {
	ref := sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "sdk", Name: "plug"}
	c.Check(ref.String(), Equals, s.projectId+"/ws/sdk:plug")
	refPtr := &sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "sdk", Name: "plug"}
	c.Check(refPtr.String(), Equals, s.projectId+"/ws/sdk:plug")
}

// SlotRef.String works as expected
func (s *CoreSuite) TestSlotRefString(c *C) {
	ref := sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "sdk", Name: "slot"}
	c.Check(ref.String(), Equals, s.projectId+"/ws/sdk:slot")
	refPtr := &sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "sdk", Name: "slot"}
	c.Check(refPtr.String(), Equals, s.projectId+"/ws/sdk:slot")
}

// ConnRef.ID works as expected
func (s *CoreSuite) TestConnRefID(c *C) {
	conn := &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "producer", Name: "slot"},
	}
	c.Check(conn.ID(), Equals, fmt.Sprintf("%s/ws/consumer:plug %s/ws/producer:slot", s.projectId, s.projectId))
}

// ParseConnRef works as expected
func (s *CoreSuite) TestParseConnRef(c *C) {
	ref, err := interfaces.ParseConnRef("42424242/ws/consumer:plug 42424242/ws/producer:slot")
	c.Assert(err, IsNil)
	c.Check(ref, DeepEquals, &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "producer", Name: "slot"},
	})
	_, err = interfaces.ParseConnRef("garbage")
	c.Assert(err, ErrorMatches, `malformed connection identifier: "garbage"`)
	_, err = interfaces.ParseConnRef("ws:plug:garbage ws:slot")
	c.Assert(err, ErrorMatches, `malformed connection identifier: ".*"`)
	_, err = interfaces.ParseConnRef("ws:plug ws:slot:garbage")
	c.Assert(err, ErrorMatches, `malformed connection identifier: ".*"`)
}

type simpleIface struct {
	name string
}

func (si simpleIface) Name() string                                            { return si.name }
func (si simpleIface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool { return false }

func (s *CoreSuite) TestByName(c *C) {
	// setup a mock interface using builtin - this will also trigger init() in
	// builtin package which set ByName to a real implementation
	r := builtin.MockInterface(simpleIface{name: "mock-network"})
	defer r()

	_, err := interfaces.ByName("no-such-interface")
	c.Assert(err, ErrorMatches, "interface \"no-such-interface\" not found")

	iface, err := interfaces.ByName("mock-network")
	c.Assert(err, IsNil)
	c.Assert(iface.Name(), Equals, "mock-network")
}

// TestCompareByName verifies that CompareByName orders interfaces by name
// with strings.Compare semantics: negative when the first sorts before the
// second, zero when the names are equal, positive when it sorts after.
func (s *CoreSuite) TestCompareByName(c *C) {
	before := simpleIface{name: "camera"}
	after := simpleIface{name: "mount"}
	c.Check(interfaces.CompareByName(before, after) < 0, Equals, true)
	c.Check(interfaces.CompareByName(after, before) > 0, Equals, true)
	c.Check(interfaces.CompareByName(before, simpleIface{name: "camera"}), Equals, 0)
}

type serviceSnippetIface struct {
	simpleIface

	sanitizerErr error

	snips []string
}

func (ssi serviceSnippetIface) BeforePreparePlug(plug *sdk.PlugInfo) error {
	return ssi.sanitizerErr
}

func (ssi serviceSnippetIface) ServicePermanentPlug(plug *sdk.PlugInfo) []string {
	return ssi.snips
}

func (s *CoreSuite) TestPermanentPlugServiceSnippets(c *C) {
	// setup a mock interface using builtin - this will also trigger init() in
	// builtin package which set ByName to a real implementation
	ssi := serviceSnippetIface{
		simpleIface: simpleIface{name: "mock-service-snippets"},
		snips:       []string{"foo1", "foo2"},
	}
	r := builtin.MockInterface(ssi)
	defer r()

	iface, err := interfaces.ByName("mock-service-snippets")
	c.Assert(err, IsNil)
	c.Assert(iface.Name(), Equals, "mock-service-snippets")

	info := sdk.MockInfo(c, `
name: sdk
base: ubuntu@22.04
plugs:
  plug:
    interface: mock-service-snippets
`, s.projectId, "ws")
	plug := info.Plugs["plug"]

	snips, err := interfaces.PermanentPlugServiceSnippets(iface, plug)
	c.Assert(err, IsNil)
	c.Assert(snips, DeepEquals, []string{"foo1", "foo2"})
}

func (s *CoreSuite) TestPermanentPlugServiceSnippetsSanitizesPlugs(c *C) {
	// setup a mock interface using builtin - this will also trigger init() in
	// builtin package which set ByName to a real implementation
	ssi := serviceSnippetIface{
		simpleIface:  simpleIface{name: "unclean-service-snippets"},
		sanitizerErr: fmt.Errorf("cannot sanitize: foo"),
	}
	r := builtin.MockInterface(ssi)
	defer r()

	info := sdk.MockInfo(c, `
name: sdk
base: ubuntu@22.04
plugs:
  plug:
    interface: unclean-service-snippets
`, s.projectId, "ws")
	plug := info.Plugs["plug"]

	iface, err := interfaces.ByName("unclean-service-snippets")
	c.Assert(err, IsNil)
	c.Assert(iface.Name(), Equals, "unclean-service-snippets")

	_, err = interfaces.PermanentPlugServiceSnippets(iface, plug)
	c.Assert(err, ErrorMatches, "cannot sanitize: foo")
}

func (s *CoreSuite) TestSanitizePlug(c *C) {
	info := sdk.MockInfo(c, `
name: sdk
base: ubuntu@22.04
plugs:
  plug:
    interface: iface
`, s.projectId, "ws")
	plug := info.Plugs["plug"]
	c.Assert(interfaces.BeforePreparePlug(&ifacetest.TestInterface{
		InterfaceName: "iface",
	}, plug), IsNil)
	c.Assert(interfaces.BeforePreparePlug(&ifacetest.TestInterface{
		InterfaceName:             "iface",
		BeforePreparePlugCallback: func(plug *sdk.PlugInfo) error { return fmt.Errorf("broken") },
	}, plug), ErrorMatches, "broken")
	c.Assert(interfaces.BeforePreparePlug(&ifacetest.TestInterface{
		InterfaceName: "other",
	}, plug), ErrorMatches, `cannot sanitize plug "ws/sdk:plug" \(interface "iface"\) using interface "other"`)
}

func (s *CoreSuite) TestSanitizeSlot(c *C) {
	info := sdk.MockInfo(c, `
name: sdk
base: ubuntu@22.04
slots:
  slot:
    interface: iface
`, s.projectId, "ws")
	slot := info.Slots["slot"]
	c.Assert(interfaces.BeforePrepareSlot(&ifacetest.TestInterface{
		InterfaceName: "iface",
	}, slot), IsNil)
	c.Assert(interfaces.BeforePrepareSlot(&ifacetest.TestInterface{
		InterfaceName:             "iface",
		BeforePrepareSlotCallback: func(slot *sdk.SlotInfo) error { return fmt.Errorf("broken") },
	}, slot), ErrorMatches, "broken")
	c.Assert(interfaces.BeforePrepareSlot(&ifacetest.TestInterface{
		InterfaceName: "other",
	}, slot), ErrorMatches, fmt.Sprintf(`cannot sanitize slot "%s/ws/sdk:slot" \(interface "iface"\) using interface "other"`, s.projectId))
}
