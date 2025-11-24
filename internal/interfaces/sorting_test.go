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
	"sort"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/sdk"
)

type SortingSuite struct{}

var _ = Suite(&SortingSuite{})

func newConnRef(plugWs, plugSdk, plug, slotWs, slotSdk, slot string) *interfaces.ConnRef {
	return &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{Workshop: plugWs, Sdk: plugSdk, Name: plug},
		SlotRef: sdk.SlotRef{Workshop: slotWs, Sdk: slotSdk, Name: slot},
	}
}

func (s *SortingSuite) TestByInterfaceName(c *C) {
	list := []interfaces.Interface{
		&ifacetest.TestInterface{InterfaceName: "iface-2"},
		&ifacetest.TestInterface{InterfaceName: "iface-1"},
	}
	sort.Sort(interfaces.ByInterfaceName(list))
	c.Assert(list, DeepEquals, []interfaces.Interface{
		&ifacetest.TestInterface{InterfaceName: "iface-1"},
		&ifacetest.TestInterface{InterfaceName: "iface-2"},
	})
}

func (s *SortingSuite) TestByConnRef(c *C) {
	list := []*interfaces.ConnRef{
		newConnRef("abc", "name-1", "plug-3", "abc", "name-2", "slot-1"),
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-3"),
		newConnRef("def", "name-1", "plug-2", "abc", "name-2", "slot-2"),
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-4"),
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-1"),
		newConnRef("abc", "name-1", "plug-1", "def", "name-2_instance", "slot-1"),
		newConnRef("abc", "name-1_instance", "plug-1", "abc", "name-2", "slot-1"),
	}
	sort.Sort(interfaces.ByConnRef(list))

	c.Assert(list, DeepEquals, []*interfaces.ConnRef{
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-1"),
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-3"),
		newConnRef("abc", "name-1", "plug-1", "abc", "name-2", "slot-4"),
		newConnRef("abc", "name-1", "plug-1", "def", "name-2_instance", "slot-1"),
		newConnRef("abc", "name-1", "plug-3", "abc", "name-2", "slot-1"),
		newConnRef("abc", "name-1_instance", "plug-1", "abc", "name-2", "slot-1"),
		newConnRef("def", "name-1", "plug-2", "abc", "name-2", "slot-2"),
	})
}

func newSlotRef(workshop, sk, name string) *sdk.SlotRef {
	return &sdk.SlotRef{Workshop: workshop, Sdk: sk, Name: name}
}

type bySlotRef []*sdk.SlotRef

func (b bySlotRef) Len() int      { return len(b) }
func (b bySlotRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b bySlotRef) Less(i, j int) bool {
	return b[i].SortsBefore(*b[j])
}

func (s *SortingSuite) TestSortSlotRef(c *C) {
	list := []*sdk.SlotRef{
		newSlotRef("abc", "name-2", "slot-3"),
		newSlotRef("def", "name-2_instance", "slot-1"),
		newSlotRef("def", "name-2", "slot-2"),
		newSlotRef("abc", "name-2", "slot-4"),
		newSlotRef("def", "name-2", "slot-1"),
	}
	sort.Sort(bySlotRef(list))

	c.Assert(list, DeepEquals, []*sdk.SlotRef{
		newSlotRef("abc", "name-2", "slot-3"),
		newSlotRef("abc", "name-2", "slot-4"),
		newSlotRef("def", "name-2", "slot-1"),
		newSlotRef("def", "name-2", "slot-2"),
		newSlotRef("def", "name-2_instance", "slot-1"),
	})
}

type byPlugRef []*sdk.PlugRef

func (b byPlugRef) Len() int      { return len(b) }
func (b byPlugRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byPlugRef) Less(i, j int) bool {
	return b[i].SortsBefore(*b[j])
}

func newPlugRef(workshop, sk, name string) *sdk.PlugRef {
	return &sdk.PlugRef{Workshop: workshop, Sdk: sk, Name: name}
}

func (s *SortingSuite) TestSortPlugRef(c *C) {
	list := []*sdk.PlugRef{
		newPlugRef("abc", "name-2", "plug-3"),
		newPlugRef("def", "name-2_instance", "plug-1"),
		newPlugRef("abc", "name-2", "plug-4"),
		newPlugRef("def", "name-2", "plug-2"),
		newPlugRef("def", "name-2", "plug-1"),
	}
	sort.Sort(byPlugRef(list))

	c.Assert(list, DeepEquals, []*sdk.PlugRef{
		newPlugRef("abc", "name-2", "plug-3"),
		newPlugRef("abc", "name-2", "plug-4"),
		newPlugRef("def", "name-2", "plug-1"),
		newPlugRef("def", "name-2", "plug-2"),
		newPlugRef("def", "name-2_instance", "plug-1"),
	})
}
