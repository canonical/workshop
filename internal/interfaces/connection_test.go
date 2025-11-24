// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017 Canonical Ltd
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
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/utils"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type connSuite struct {
	testutil.BaseTest
	plug      *sdk.PlugInfo
	slot      *sdk.SlotInfo
	projectId string
}

var _ = check.Suite(&connSuite{})

func (s *connSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {}))
	s.projectId = "42424242"
	consumer := sdk.MockInfo(c, `
name: consumer
base: ubuntu@22.04
plugs:
    plug:
        interface: interface
        attr: value
        complex:
            c: d
`, s.projectId, "ws")
	s.plug = consumer.Plugs["plug"]
	producer := sdk.MockInfo(c, `
name: producer
base: ubuntu@22.04
slots:
    slot:
        interface: interface
        attr: value
        number: 100
        complex:
            a: b
`, s.projectId, "ws")
	s.slot = producer.Slots["slot"]
}

func (s *connSuite) TearDownTest(c *check.C) {
	s.BaseTest.TearDownTest(c)
}

// Make sure ConnectedPlug,ConnectedSlot, PlugInfo, SlotInfo implement Attrer.
var (
	_ interfaces.Attrer = (*interfaces.ConnectedPlug)(nil)
	_ interfaces.Attrer = (*interfaces.ConnectedSlot)(nil)
	_ interfaces.Attrer = (*sdk.PlugInfo)(nil)
	_ interfaces.Attrer = (*sdk.SlotInfo)(nil)
)

func (s *connSuite) TestStaticSlotAttrs(c *check.C) {
	slot := interfaces.NewConnectedSlot(s.slot, nil, nil)
	c.Assert(slot, check.NotNil)

	var val string
	var intVal int
	c.Assert(slot.StaticAttr("unknown", &val), check.ErrorMatches, `attribute "unknown" not found for slot "ws/producer:slot"`)

	attrs := slot.StaticAttrs()
	c.Assert(attrs, check.DeepEquals, map[string]interface{}{
		"attr":    "value",
		"number":  int64(100),
		"complex": map[string]interface{}{"a": "b"},
	})
	slot.StaticAttr("attr", &val)
	c.Assert(val, check.Equals, "value")

	c.Assert(slot.StaticAttr("unknown", &val), check.ErrorMatches, `attribute "unknown" not found for slot "ws/producer:slot"`)
	c.Check(slot.StaticAttr("attr", &intVal), check.ErrorMatches, `invalid attribute "attr" for slot "ws/producer:slot": expected int but found string`)
	c.Check(slot.StaticAttr("attr", val), check.ErrorMatches, `invalid attribute "attr" for slot "ws/producer:slot": internal error: value must be a pointer`)

	// static attributes passed via args take precedence over slot.Attrs
	slot2 := interfaces.NewConnectedSlot(s.slot, map[string]interface{}{"foo": "bar"}, nil)
	slot2.StaticAttr("foo", &val)
	c.Assert(val, check.Equals, "bar")
}

func (s *connSuite) TestSlotRef(c *check.C) {
	slot := interfaces.NewConnectedSlot(s.slot, nil, nil)
	c.Assert(slot, check.NotNil)
	c.Assert(slot.Ref(), check.DeepEquals, sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "producer", Name: "slot"})
}

func (s *connSuite) TestPlugRef(c *check.C) {
	plug := interfaces.NewConnectedPlug(s.plug, nil, nil)
	c.Assert(plug, check.NotNil)
	c.Assert(plug.Ref(), check.DeepEquals, sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "consumer", Name: "plug"})
}

func (s *connSuite) TestStaticPlugAttrs(c *check.C) {
	plug := interfaces.NewConnectedPlug(s.plug, nil, nil)
	c.Assert(plug, check.NotNil)

	var val string
	var intVal int
	c.Assert(plug.StaticAttr("unknown", &val), check.ErrorMatches, `attribute "unknown" not found for plug "ws/consumer:plug"`)

	attrs := plug.StaticAttrs()
	c.Assert(attrs, check.DeepEquals, map[string]interface{}{
		"attr":    "value",
		"complex": map[string]interface{}{"c": "d"},
	})
	plug.StaticAttr("attr", &val)
	c.Assert(val, check.Equals, "value")

	c.Assert(plug.StaticAttr("unknown", &val), check.ErrorMatches, `attribute "unknown" not found for plug "ws/consumer:plug"`)
	c.Check(plug.StaticAttr("attr", &intVal), check.ErrorMatches, `invalid attribute "attr" for plug "ws/consumer:plug": expected int but found string`)
	c.Check(plug.StaticAttr("attr", val), check.ErrorMatches, `invalid attribute "attr" for plug "ws/consumer:plug": internal error: value must be a pointer`)

	// static attributes passed via args take precedence over plug.Attrs
	plug2 := interfaces.NewConnectedPlug(s.plug, map[string]interface{}{"foo": "bar"}, nil)
	plug2.StaticAttr("foo", &val)
	c.Assert(val, check.Equals, "bar")
}

func (s *connSuite) TestDynamicSlotAttrs(c *check.C) {
	attrs := map[string]interface{}{
		"foo":    "bar",
		"number": int(101),
	}
	slot := interfaces.NewConnectedSlot(s.slot, nil, attrs)
	c.Assert(slot, check.NotNil)

	var strVal string
	var intVal int64
	var mapVal map[string]interface{}

	c.Assert(slot.Attr("foo", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "bar")

	c.Assert(slot.Attr("attr", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "value")

	c.Assert(slot.Attr("number", &intVal), check.IsNil)
	c.Assert(intVal, check.Equals, int64(101))

	err := slot.SetAttr("other", map[string]interface{}{"number-two": int(222)})
	c.Assert(err, check.IsNil)
	c.Assert(slot.Attr("other", &mapVal), check.IsNil)
	num := mapVal["number-two"]
	c.Assert(num, check.Equals, int64(222))

	c.Check(slot.Attr("unknown", &strVal), check.ErrorMatches, `attribute "unknown" not found for slot "ws/producer:slot"`)
	c.Check(slot.Attr("foo", &intVal), check.ErrorMatches, `invalid attribute "foo" for slot "ws/producer:slot": expected int64 but found string`)
	c.Check(slot.Attr("number", intVal), check.ErrorMatches, `invalid attribute "number" for slot "ws/producer:slot": internal error: value must be a pointer`)
}

func (s *connSuite) TestDottedPathSlot(c *check.C) {
	attrs := map[string]interface{}{
		"nested": map[string]interface{}{
			"foo": "bar",
		},
	}
	var strVal string

	slot := interfaces.NewConnectedSlot(s.slot, nil, attrs)
	c.Assert(slot, check.NotNil)

	// static attribute complex.a
	c.Assert(slot.Attr("complex.a", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "b")

	v, ok := slot.Lookup("complex.a")
	c.Assert(ok, check.Equals, true)
	c.Assert(v, check.Equals, "b")

	// dynamic attribute nested.foo
	c.Assert(slot.Attr("nested.foo", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "bar")

	v, ok = slot.Lookup("nested.foo")
	c.Assert(ok, check.Equals, true)
	c.Assert(v, check.Equals, "bar")

	_, ok = slot.Lookup("..")
	c.Assert(ok, check.Equals, false)
}

func (s *connSuite) TestDottedPathPlug(c *check.C) {
	attrs := map[string]interface{}{
		"a": "b",
		"nested": map[string]interface{}{
			"foo": "bar",
		},
	}
	var strVal string

	plug := interfaces.NewConnectedPlug(s.plug, nil, attrs)
	c.Assert(plug, check.NotNil)

	v, ok := plug.Lookup("a")
	c.Assert(ok, check.Equals, true)
	c.Assert(v, check.Equals, "b")

	// static attribute complex.c
	c.Assert(plug.Attr("complex.c", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "d")

	v, ok = plug.Lookup("complex.c")
	c.Assert(ok, check.Equals, true)
	c.Assert(v, check.Equals, "d")

	// dynamic attribute nested.foo
	c.Assert(plug.Attr("nested.foo", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "bar")

	v, ok = plug.Lookup("nested.foo")
	c.Assert(ok, check.Equals, true)
	c.Assert(v, check.Equals, "bar")

	_, ok = plug.Lookup("nested.x")
	c.Assert(ok, check.Equals, false)

	_, ok = plug.Lookup("nested.foo.y")
	c.Assert(ok, check.Equals, false)

	_, ok = plug.Lookup("..")
	c.Assert(ok, check.Equals, false)
}

func (s *connSuite) TestLookupFailure(c *check.C) {
	attrs := map[string]interface{}{}

	slot := interfaces.NewConnectedSlot(s.slot, nil, attrs)
	c.Assert(slot, check.NotNil)
	plug := interfaces.NewConnectedPlug(s.plug, nil, attrs)
	c.Assert(plug, check.NotNil)

	v, ok := slot.Lookup("a")
	c.Assert(ok, check.Equals, false)
	c.Assert(v, check.IsNil)

	v, ok = plug.Lookup("a")
	c.Assert(ok, check.Equals, false)
	c.Assert(v, check.IsNil)
}

func (s *connSuite) TestDynamicPlugAttrs(c *check.C) {
	attrs := map[string]interface{}{
		"foo":    "bar",
		"number": int(100),
	}
	plug := interfaces.NewConnectedPlug(s.plug, nil, attrs)
	c.Assert(plug, check.NotNil)

	var strVal string
	var intVal int64
	var mapVal map[string]interface{}

	c.Assert(plug.Attr("foo", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "bar")

	c.Assert(plug.Attr("attr", &strVal), check.IsNil)
	c.Assert(strVal, check.Equals, "value")

	c.Assert(plug.Attr("number", &intVal), check.IsNil)
	c.Assert(intVal, check.Equals, int64(100))

	err := plug.SetAttr("other", map[string]interface{}{"number-two": int(222)})
	c.Assert(err, check.IsNil)
	c.Assert(plug.Attr("other", &mapVal), check.IsNil)
	num := mapVal["number-two"]
	c.Assert(num, check.Equals, int64(222))

	c.Check(plug.Attr("unknown", &strVal), check.ErrorMatches, `attribute "unknown" not found for plug "ws/consumer:plug"`)
	c.Check(plug.Attr("foo", &intVal), check.ErrorMatches, `invalid attribute "foo" for plug "ws/consumer:plug": expected int64 but found string`)
	c.Check(plug.Attr("number", intVal), check.ErrorMatches, `invalid attribute "number" for plug "ws/consumer:plug": internal error: value must be a pointer`)
}

func (s *connSuite) TestOverwriteStaticAttrError(c *check.C) {
	attrs := map[string]interface{}{}

	plug := interfaces.NewConnectedPlug(s.plug, nil, attrs)
	c.Assert(plug, check.NotNil)
	slot := interfaces.NewConnectedSlot(s.slot, nil, attrs)
	c.Assert(slot, check.NotNil)

	err := plug.SetAttr("attr", "overwrite")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `cannot change attribute "attr" as it was statically specified in the "consumer" SDK details`)

	err = slot.SetAttr("attr", "overwrite")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `cannot change attribute "attr" as it was statically specified in the "producer" SDK details`)
}

func (s *connSuite) TestCopyAttributes(c *check.C) {
	orig := map[string]interface{}{
		"a": "A",
		"b": true,
		"c": int(100),
		"d": []interface{}{"x", "y", true},
		"e": map[string]interface{}{"e1": "E1"},
	}

	cpy := utils.CopyAttributes(orig)
	c.Check(cpy, check.DeepEquals, orig)

	cpy["d"].([]interface{})[0] = 999
	c.Check(orig["d"].([]interface{})[0], check.Equals, "x")
	cpy["e"].(map[string]interface{})["e1"] = "x"
	c.Check(orig["e"].(map[string]interface{})["e1"], check.Equals, "E1")
}

func (s *connSuite) TestNewConnectedPlugExplicitStaticAttrs(c *check.C) {
	staticAttrs := map[string]interface{}{
		"baz": "boom",
	}
	dynAttrs := map[string]interface{}{
		"foo": "bar",
	}
	plug := interfaces.NewConnectedPlug(s.plug, staticAttrs, dynAttrs)
	c.Assert(plug, check.NotNil)
	c.Assert(plug.StaticAttrs(), check.DeepEquals, map[string]interface{}{"baz": "boom"})
	c.Assert(plug.DynamicAttrs(), check.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *connSuite) TestNewConnectedSlotExplicitStaticAttrs(c *check.C) {
	staticAttrs := map[string]interface{}{
		"baz": "boom",
	}
	dynAttrs := map[string]interface{}{
		"foo": "bar",
	}
	slot := interfaces.NewConnectedSlot(s.slot, staticAttrs, dynAttrs)
	c.Assert(slot, check.NotNil)
	c.Assert(slot.StaticAttrs(), check.DeepEquals, map[string]interface{}{"baz": "boom"})
	c.Assert(slot.DynamicAttrs(), check.DeepEquals, map[string]interface{}{"foo": "bar"})
}
