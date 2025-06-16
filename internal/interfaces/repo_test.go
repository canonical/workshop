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
	"context"
	"fmt"
	"os/user"

	. "gopkg.in/check.v1"

	. "github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type RepositorySuite struct {
	testutil.BaseTest
	iface     Interface
	plug      *sdk.PlugInfo
	plugSelf  *sdk.PlugInfo
	slot      *sdk.SlotInfo
	emptyRepo *Repository
	// Repository pre-populated with s.iface
	testRepo  *Repository
	context   context.Context
	projectId string
	restore   func()
}

var _ = Suite(&RepositorySuite{
	iface: &ifacetest.TestInterface{
		InterfaceName: "interface",
	},
})

const consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
    plug:
        interface: interface
        label: label
        attr: value
`

const producerYaml = `
name: producer
base: ubuntu@22.04
slots:
    slot:
        interface: interface
        label: label
        attr: value
plugs:
    self:
        interface: interface
        label: label
`

func (s *RepositorySuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(snapInfo *sdk.Info) {}))

	consumer := sdk.MockInfo(c, consumerYaml, s.projectId, "ws")
	s.plug = consumer.Plugs["plug"]
	producer := sdk.MockInfo(c, producerYaml, s.projectId, "ws")
	s.slot = producer.Slots["slot"]
	s.plugSelf = producer.Plugs["self"]

	s.emptyRepo = NewRepository()
	s.testRepo = NewRepository()
	err := s.testRepo.AddInterface(s.iface)
	c.Assert(err, IsNil)
	s.projectId = "42424242"
	s.context = ifacetest.CreateTestContext("user", s.projectId)

	s.restore = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return &user.User{HomeDir: c.MkDir()}, nil
	})
}

func (s *RepositorySuite) TearDownTest(c *C) {
	s.BaseTest.TearDownTest(c)
	s.restore()
}

type instanceNameAndYaml struct {
	Name string
	Yaml string
}

func addPlugsSlotsFromInstances(c *C, repo *Repository, projectId string, iys []instanceNameAndYaml) []*sdk.Info {
	result := make([]*sdk.Info, 0, len(iys))
	for _, iy := range iys {
		info := sdk.MockInfo(c, iy.Yaml, projectId, "ws-"+iy.Name)
		if iy.Name != "" {
			c.Assert(sdk.Validate(info), IsNil)
		}

		result = append(result, info)
		for _, plugInfo := range info.Plugs {
			err := repo.AddPlug(plugInfo)
			c.Assert(err, IsNil)
		}
		for _, slotInfo := range info.Slots {
			err := repo.AddSlot(slotInfo)
			c.Assert(err, IsNil)
		}
	}
	return result
}

// Tests for Repository.AddInterface()

func (s *RepositorySuite) TestAddInterface(c *C) {
	// Adding a valid interfaces works
	err := s.emptyRepo.AddInterface(s.iface)
	c.Assert(err, IsNil)
	c.Assert(s.emptyRepo.Interface(s.iface.Name()), Equals, s.iface)
}

func (s *RepositorySuite) TestAddInterfaceClash(c *C) {
	iface1 := &ifacetest.TestInterface{InterfaceName: "iface"}
	iface2 := &ifacetest.TestInterface{InterfaceName: "iface"}
	err := s.emptyRepo.AddInterface(iface1)
	c.Assert(err, IsNil)
	// Adding an interface with the same name as another interface is not allowed
	err = s.emptyRepo.AddInterface(iface2)
	c.Assert(err, ErrorMatches, `cannot add interface: "iface", interface name is in use`)
	c.Assert(s.emptyRepo.Interface(iface1.Name()), Equals, iface1)
}

func (s *RepositorySuite) TestAddInterfaceInvalidName(c *C) {
	iface := &ifacetest.TestInterface{InterfaceName: "bad-name-"}
	// Adding an interface with invalid name is not allowed
	err := s.emptyRepo.AddInterface(iface)
	c.Assert(err, ErrorMatches, `invalid interface name: "bad-name-"`)
	c.Assert(s.emptyRepo.Interface(iface.Name()), IsNil)
}

// Tests for Repository.AllInterfaces()

func (s *RepositorySuite) TestAllInterfaces(c *C) {
	c.Assert(s.emptyRepo.AllInterfaces(), HasLen, 0)
	c.Assert(s.testRepo.AllInterfaces(), DeepEquals, []Interface{s.iface})

	// Add three interfaces in some non-sorted order.
	i1 := &ifacetest.TestInterface{InterfaceName: "i1"}
	i2 := &ifacetest.TestInterface{InterfaceName: "i2"}
	i3 := &ifacetest.TestInterface{InterfaceName: "i3"}
	c.Assert(s.emptyRepo.AddInterface(i3), IsNil)
	c.Assert(s.emptyRepo.AddInterface(i1), IsNil)
	c.Assert(s.emptyRepo.AddInterface(i2), IsNil)

	// The result is always sorted.
	c.Assert(s.emptyRepo.AllInterfaces(), DeepEquals, []Interface{i1, i2, i3})

}

func (s *RepositorySuite) TestAddBackend(c *C) {
	backend := &ifacetest.TestSecurityBackend{BackendName: "test"}
	c.Assert(s.emptyRepo.AddBackend(backend), IsNil)
	err := s.emptyRepo.AddBackend(backend)
	c.Assert(err, ErrorMatches, `cannot add backend "test", security system name is in use`)
}

func (s *RepositorySuite) TestBackends(c *C) {
	b1 := &ifacetest.TestSecurityBackend{BackendName: "b1"}
	b2 := &ifacetest.TestSecurityBackend{BackendName: "b2"}
	c.Assert(s.emptyRepo.AddBackend(b2), IsNil)
	c.Assert(s.emptyRepo.AddBackend(b1), IsNil)
	// The order of insertion is retained.
	c.Assert(s.emptyRepo.Backends(), DeepEquals, []SecurityBackend{b2, b1})
}

// Tests for Repository.Interface()

func (s *RepositorySuite) TestInterface(c *C) {
	// Interface returns nil when it cannot be found
	iface := s.emptyRepo.Interface(s.iface.Name())
	c.Assert(iface, IsNil)
	c.Assert(s.emptyRepo.Interface(s.iface.Name()), IsNil)
	err := s.emptyRepo.AddInterface(s.iface)
	c.Assert(err, IsNil)
	// Interface returns the found interface
	iface = s.emptyRepo.Interface(s.iface.Name())
	c.Assert(iface, Equals, s.iface)
}

func (s *RepositorySuite) TestInterfaceSearch(c *C) {
	ifaceA := &ifacetest.TestInterface{InterfaceName: "a"}
	ifaceB := &ifacetest.TestInterface{InterfaceName: "b"}
	ifaceC := &ifacetest.TestInterface{InterfaceName: "c"}
	err := s.emptyRepo.AddInterface(ifaceA)
	c.Assert(err, IsNil)
	err = s.emptyRepo.AddInterface(ifaceB)
	c.Assert(err, IsNil)
	err = s.emptyRepo.AddInterface(ifaceC)
	c.Assert(err, IsNil)
	// Interface correctly finds interfaces
	c.Assert(s.emptyRepo.Interface("a"), Equals, ifaceA)
	c.Assert(s.emptyRepo.Interface("b"), Equals, ifaceB)
	c.Assert(s.emptyRepo.Interface("c"), Equals, ifaceC)
}

// Tests for Repository.AddPlug()

func (s *RepositorySuite) TestAddPlug(c *C) {
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 0)
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 1)
}

func (s *RepositorySuite) TestAddPlugClashingPlug(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddPlug(s.plug)
	c.Assert(err, ErrorMatches, `sdk "consumer" has plugs conflicting on name "plug"`)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 1)
}

func (s *RepositorySuite) TestAddPlugClashingSlot(c *C) {
	sdkInfo := &sdk.Info{ProjectId: s.projectId, Workshop: "ws", Name: "sdk"}
	plug := &sdk.PlugInfo{
		Sdk:       sdkInfo,
		Name:      "clashing",
		Interface: "interface",
	}
	slot := &sdk.SlotInfo{
		Sdk:       sdkInfo,
		Name:      "clashing",
		Interface: "interface",
	}
	err := s.testRepo.AddSlot(slot)
	c.Assert(err, IsNil)
	err = s.testRepo.AddPlug(plug)
	c.Assert(err, ErrorMatches, `sdk "sdk" has plug and slot conflicting on name "clashing"`)
	c.Assert(s.testRepo.AllSlots(""), HasLen, 1)
	c.Assert(s.testRepo.Slot(slot.Sdk.ProjectId, slot.Sdk.Workshop, slot.Sdk.Name, slot.Name), DeepEquals, slot)
}

func (s *RepositorySuite) TestAddPlugFailsWithInvalidPlugName(c *C) {
	plug := &sdk.PlugInfo{
		Sdk:       &sdk.Info{Name: "sdk"},
		Name:      "bad-name-",
		Interface: "interface",
	}
	err := s.testRepo.AddPlug(plug)
	c.Assert(err, ErrorMatches, `invalid plug name: "bad-name-"`)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 0)
}

func (s *RepositorySuite) TestAddPlugFailsWithUnknownInterface(c *C) {
	err := s.emptyRepo.AddPlug(s.plug)
	c.Assert(err, ErrorMatches, `cannot add plug, interface "interface" is not known`)
	c.Assert(s.emptyRepo.AllPlugs(""), HasLen, 0)
}

func (s *RepositorySuite) TestAddPlugParallelInstance(c *C) {
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 0)

	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 1)

	consumer := sdk.MockInfo(c, consumerYaml, s.projectId, "ws-instance")
	err = s.testRepo.AddPlug(consumer.Plugs["plug"])
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 2)

	c.Assert(s.testRepo.Plug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name), DeepEquals, s.plug)
	c.Assert(s.testRepo.Plug(consumer.ProjectId, consumer.Workshop, consumer.Name, "plug"), DeepEquals, consumer.Plugs["plug"])
}

// Tests for Repository.Plug()

func (s *RepositorySuite) TestPlug(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	c.Assert(s.emptyRepo.Plug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name), IsNil)
	c.Assert(s.testRepo.Plug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name), DeepEquals, s.plug)
}

func (s *RepositorySuite) TestPlugSearch(c *C) {
	addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "xx", Yaml: `
name: xx
base: ubuntu@22.04
plugs:
    a: interface
    b: interface
    c: interface
`},
		{Name: "yy", Yaml: `
name: yy
base: ubuntu@22.04
plugs:
    a: interface
    b: interface
    c: interface
`},
		{Name: "zz_instance", Yaml: `
name: zz
base: ubuntu@22.04
plugs:
    a: interface
    b: interface
    c: interface
`},
	})
	// Plug() correctly finds plugs
	c.Assert(s.testRepo.Plug(s.projectId, "ws-xx", "xx", "a"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-xx", "xx", "b"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-xx", "xx", "c"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-yy", "yy", "a"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-yy", "yy", "b"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-yy", "yy", "c"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-zz_instance", "zz", "a"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-zz_instance", "zz", "b"), Not(IsNil))
	c.Assert(s.testRepo.Plug(s.projectId, "ws-zz_instance", "zz", "c"), Not(IsNil))
}

// Tests for Repository.RemovePlug()

func (s *RepositorySuite) TestRemovePlugSucceedsWhenPlugExistsAndDisconnected(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.RemovePlug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 0)
}

func (s *RepositorySuite) TestRemovePlugFailsWhenPlugDoesntExist(c *C) {
	err := s.emptyRepo.RemovePlug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	c.Assert(err, ErrorMatches, `cannot remove plug "plug" from sdk "consumer", no such plug`)
}

func (s *RepositorySuite) TestRemovePlugFailsWhenPlugIsConnected(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	// Removing a plug used by a slot returns an appropriate error
	err = s.testRepo.RemovePlug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	c.Assert(err, ErrorMatches, `cannot remove plug "plug" from sdk "consumer", it is still connected`)
	// The plug is still there
	slot := s.testRepo.Plug(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	c.Assert(slot, Not(IsNil))
}

// Tests for Repository.AllPlugs()

func (s *RepositorySuite) TestAllPlugsWithoutInterfaceName(c *C) {
	sdks := addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "sdk-a", Yaml: `
name: sdk-a
base: ubuntu@22.04
plugs:
    name-a: interface
`},
		{Name: "sdk-b", Yaml: `
name: sdk-b
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: interface
    name-c: interface
`},
		{Name: "sdk-b_instance", Yaml: `
name: sdk-b
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: interface
    name-c: interface
`},
	})
	c.Assert(sdks, HasLen, 3)
	// The result is sorted by sdk and name
	allPlugs := s.testRepo.AllPlugs("")
	c.Assert(allPlugs, DeepEquals, []*sdk.PlugInfo{
		sdks[0].Plugs["name-a"],
		sdks[1].Plugs["name-a"],
		sdks[1].Plugs["name-b"],
		sdks[1].Plugs["name-c"],
		sdks[2].Plugs["name-a"],
		sdks[2].Plugs["name-b"],
		sdks[2].Plugs["name-c"],
	})
}

func (s *RepositorySuite) TestAllPlugsWithInterfaceName(c *C) {
	// Add another interface so that we can look for it
	err := s.testRepo.AddInterface(&ifacetest.TestInterface{InterfaceName: "other-interface"})
	c.Assert(err, IsNil)
	snaps := addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "sdk-a", Yaml: `
name: sdk-a
base: ubuntu@22.04
plugs:
    name-a: interface
`},
		{Name: "sdk-b", Yaml: `
name: sdk-b
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: other-interface
    name-c: interface
`},
		{Name: "sdk-b_instance", Yaml: `
name: sdk-b
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: other-interface
    name-c: interface
`},
	})
	c.Assert(snaps, HasLen, 3)
	c.Assert(s.testRepo.AllPlugs("other-interface"), DeepEquals, []*sdk.PlugInfo{
		snaps[1].Plugs["name-b"],
		snaps[2].Plugs["name-b"],
	})
}

// Tests for Repository.Plugs()

func (s *RepositorySuite) TestPlugs(c *C) {
	snaps := addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "sdk-a", Yaml: `
name: sdk-a
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: interface
    name-c: interface
`},
		{Name: "sdk-b", Yaml: `
name: sdk-b
base: ubuntu@22.04
plugs:
    name-a: interface
    name-b: interface
    name-c: interface
`},
	})
	c.Assert(snaps, HasLen, 2)
	// The result is sorted by sdk and name
	c.Assert(s.testRepo.Plugs(s.projectId, "ws-sdk-a", "sdk-a"), DeepEquals, []*sdk.PlugInfo{
		snaps[0].Plugs["name-a"],
		snaps[0].Plugs["name-b"],
		snaps[0].Plugs["name-c"],
	})
	c.Assert(s.testRepo.Plugs(s.projectId, "ws-sdk-b", "sdk-b"), DeepEquals, []*sdk.PlugInfo{
		snaps[1].Plugs["name-a"],
		snaps[1].Plugs["name-b"],
		snaps[1].Plugs["name-c"],
	})
	// The result is empty if the sdk is not known
	c.Assert(s.testRepo.Plugs(s.projectId, "ws-sdk-a", "sdk-x"), HasLen, 0)
	c.Assert(s.testRepo.Plugs(s.projectId, "ws-sdk-b", "sdk-b_other"), HasLen, 0)
}

// Tests for Repository.AllSlots()

func (s *RepositorySuite) TestAllSlots(c *C) {
	err := s.testRepo.AddInterface(&ifacetest.TestInterface{InterfaceName: "other-interface"})
	c.Assert(err, IsNil)
	snaps := addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "sdk-a", Yaml: `
name: sdk-a
base: ubuntu@22.04
slots:
    name-a: interface
    name-b: interface
`},
		{Name: "sdk-b", Yaml: `
name: sdk-b
base: ubuntu@22.04
slots:
    name-a: other-interface
`},
		{Name: "sdk-b_instance", Yaml: `
name: sdk-b
base: ubuntu@22.04
slots:
    name-a: other-interface
`},
	})
	c.Assert(snaps, HasLen, 3)
	// AllSlots("") returns all slots, sorted by sdk and slot name
	c.Assert(s.testRepo.AllSlots(""), DeepEquals, []*sdk.SlotInfo{
		snaps[0].Slots["name-a"],
		snaps[0].Slots["name-b"],
		snaps[1].Slots["name-a"],
		snaps[2].Slots["name-a"],
	})
	// AllSlots("") returns all slots, sorted by sdk and slot name
	c.Assert(s.testRepo.AllSlots("other-interface"), DeepEquals, []*sdk.SlotInfo{
		snaps[1].Slots["name-a"],
		snaps[2].Slots["name-a"],
	})
}

// Tests for Repository.Slots()

func (s *RepositorySuite) TestSlots(c *C) {
	snaps := addPlugsSlotsFromInstances(c, s.testRepo, s.projectId, []instanceNameAndYaml{
		{Name: "sdk-a", Yaml: `
name: sdk-a
base: ubuntu@22.04
slots:
    name-a: interface
    name-b: interface
`},
		{Name: "sdk-b", Yaml: `
name: sdk-b
base: ubuntu@22.04
slots:
    name-a: interface
`},
	})
	// Slots("sdk-a") returns slots present in that sdk
	c.Assert(s.testRepo.Slots(s.projectId, "ws-sdk-a", "sdk-a"), DeepEquals, []*sdk.SlotInfo{
		snaps[0].Slots["name-a"],
		snaps[0].Slots["name-b"],
	})
	// Slots("sdk-b") returns slots present in that sdk
	c.Assert(s.testRepo.Slots(s.projectId, "ws-sdk-b", "sdk-b"), DeepEquals, []*sdk.SlotInfo{
		snaps[1].Slots["name-a"],
	})
	// Slots("sdk-c") returns no slots (because that sdk doesn't exist)
	c.Assert(s.testRepo.Slots(s.projectId, "ws-sdk-a", "sdk-c"), HasLen, 0)
	// Slots("sdk-b_other") returns no slots (the sdk does not exist)
	c.Assert(s.testRepo.Slots(s.projectId, "ws-sdk-a", "sdk-b_other"), HasLen, 0)
	// Slots("") returns no slots
	c.Assert(s.testRepo.Slots(s.projectId, "ws-sdk-a", ""), HasLen, 0)
}

// Tests for Repository.Slot()

func (s *RepositorySuite) TestSlotSucceedsWhenSlotExists(c *C) {
	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	slot := s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(slot, DeepEquals, s.slot)
}

func (s *RepositorySuite) TestSlotFailsWhenSlotDoesntExist(c *C) {
	slot := s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(slot, IsNil)
}

// Tests for Repository.AddSlot()

func (s *RepositorySuite) TestAddSlotFailsWhenInterfaceIsUnknown(c *C) {
	err := s.emptyRepo.AddSlot(s.slot)
	c.Assert(err, ErrorMatches, `cannot add slot, interface "interface" is not known`)
}

func (s *RepositorySuite) TestAddSlotFailsWhenSlotNameIsInvalid(c *C) {
	slot := &sdk.SlotInfo{
		Sdk:       &sdk.Info{Name: "sdk"},
		Name:      "bad-name-",
		Interface: "interface",
	}
	err := s.emptyRepo.AddSlot(slot)
	c.Assert(err, ErrorMatches, `invalid slot name: "bad-name-"`)
	c.Assert(s.emptyRepo.AllSlots(""), HasLen, 0)
}

func (s *RepositorySuite) TestAddSlotClashingSlot(c *C) {
	// Adding the first slot succeeds
	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// Adding the slot again fails with appropriate error
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, ErrorMatches, `sdk "producer" has slots conflicting on name "slot"`)
}

func (s *RepositorySuite) TestAddSlotClashingPlug(c *C) {
	snapInfo := &sdk.Info{Name: "sdk"}
	plug := &sdk.PlugInfo{
		Sdk:       snapInfo,
		Name:      "clashing",
		Interface: "interface",
	}
	slot := &sdk.SlotInfo{
		Sdk:       snapInfo,
		Name:      "clashing",
		Interface: "interface",
	}
	err := s.testRepo.AddPlug(plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(slot)
	c.Assert(err, ErrorMatches, `sdk "sdk" has plug and slot conflicting on name "clashing"`)
	c.Assert(s.testRepo.AllPlugs(""), HasLen, 1)
	c.Assert(s.testRepo.Plug(plug.Sdk.ProjectId, plug.Sdk.Workshop, plug.Sdk.Name, plug.Name), DeepEquals, plug)
}

func (s *RepositorySuite) TestAddSlotStoresCorrectData(c *C) {
	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	slot := s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	// The added slot has the same data
	c.Assert(slot, DeepEquals, s.slot)
}

func (s *RepositorySuite) TestAddSlotParallelInstance(c *C) {
	c.Assert(s.testRepo.AllSlots(""), HasLen, 0)

	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllSlots(""), HasLen, 1)

	producer := sdk.MockInfo(c, producerYaml, s.projectId, "ws-instance")
	err = s.testRepo.AddSlot(producer.Slots["slot"])
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.AllSlots(""), HasLen, 2)

	c.Assert(s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name), DeepEquals, s.slot)
	c.Assert(s.testRepo.Slot(producer.ProjectId, producer.Workshop, producer.Name, "slot"), DeepEquals, producer.Slots["slot"])
}

// Tests for Repository.RemoveSlot()

func (s *RepositorySuite) TestRemoveSlotSuccedsWhenSlotExistsAndDisconnected(c *C) {
	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// Removing a vacant slot simply works
	err = s.testRepo.RemoveSlot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, IsNil)
	// The slot is gone now
	slot := s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(slot, IsNil)
}

func (s *RepositorySuite) TestRemoveSlotFailsWhenSlotDoesntExist(c *C) {
	// Removing a slot that doesn't exist returns an appropriate error
	err := s.testRepo.RemoveSlot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, `cannot remove slot "slot" from sdk "producer", no such slot`)
}

func (s *RepositorySuite) TestRemoveSlotFailsWhenSlotIsConnected(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	// Removing a slot occupied by a plug returns an appropriate error
	err = s.testRepo.RemoveSlot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, ErrorMatches, `cannot remove slot "slot" from sdk "producer", it is still connected`)
	// The slot is still there
	slot := s.testRepo.Slot(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(slot, Not(IsNil))
}

// Tests for Repository.Connect()

func (s *RepositorySuite) TestConnectFailsWhenPlugDoesNotExist(c *C) {
	err := s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// Connecting an unknown plug returns an appropriate error
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, ErrorMatches, `cannot connect plug "ws/consumer:plug": plug not found`)
	e, _ := err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

func (s *RepositorySuite) TestConnectFailsWhenSlotDoesNotExist(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	// Connecting to an unknown slot returns an error
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, ErrorMatches, `cannot connect slot "ws/producer:slot": slot not found`)
	e, _ := err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

func (s *RepositorySuite) TestConnectSucceedsWhenIdenticalConnectExists(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	connRef := NewConnRef(s.plug, s.slot)
	conn, err := s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)
	c.Assert(conn.Plug, NotNil)
	c.Assert(conn.Slot, NotNil)
	c.Assert(conn.Plug.Name(), Equals, "plug")
	c.Assert(conn.Slot.Name(), Equals, "slot")
	// Connecting exactly the same thing twice succeeds without an error but does nothing.
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	// Only one connection is actually present.
	c.Assert(s.testRepo.Interfaces(), DeepEquals, &Interfaces{
		Plugs:       []*sdk.PlugInfo{s.plug},
		Slots:       []*sdk.SlotInfo{s.slot},
		Connections: []*ConnRef{NewConnRef(s.plug, s.slot)},
	})
}

func (s *RepositorySuite) TestConnectFailsWhenSlotAndPlugAreIncompatible(c *C) {
	otherInterface := &ifacetest.TestInterface{InterfaceName: "other-interface"}
	err := s.testRepo.AddInterface(otherInterface)
	plug := &sdk.PlugInfo{
		Sdk: &sdk.Info{
			ProjectId: s.projectId,
			Workshop:  "ws",
			Name:      "consumer"},
		Name:      "plug",
		Interface: "other-interface",
	}
	c.Assert(err, IsNil)
	err = s.testRepo.AddPlug(plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// Connecting a plug to an incompatible slot fails with an appropriate error
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, ErrorMatches, `cannot connect plug "ws/consumer:plug" \("other-interface" interface\) to "ws/producer:slot" \("interface" interface\)`)
}

func (s *RepositorySuite) TestConnectSucceeds(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// Connecting a plug works okay
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
}

// Tests for Repository.Disconnect() and DisconnectAll()

// Disconnect fails if any argument is empty
func (s *RepositorySuite) TestDisconnectFailsOnEmptyArgs(c *C) {
	err1 := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, "")
	err2 := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, "", s.slot.Name)
	err3 := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, "", s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	err4 := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, "", s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err1, ErrorMatches, `cannot disconnect, slot name is empty`)
	c.Assert(err2, ErrorMatches, `cannot disconnect, slot SDK name is empty`)
	c.Assert(err3, ErrorMatches, `cannot disconnect, plug name is empty`)
	c.Assert(err4, ErrorMatches, `cannot disconnect, plug SDK name is empty`)
}

// Disconnect fails if plug doesn't exist
func (s *RepositorySuite) TestDisconnectFailsWithoutPlug(c *C) {
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	err := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, ErrorMatches, `SDK "ws/consumer" has no plug named "plug"`)
	e, _ := err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

// Disconnect fails if slot doesn't exist
func (s *RepositorySuite) TestDisconnectFailsWithutSlot(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	err := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, ErrorMatches, `SDK "ws/producer" has no slot named "slot"`)
	e, _ := err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

// Disconnect fails if there's no connection to disconnect
func (s *RepositorySuite) TestDisconnectFailsWhenNotConnected(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	err := s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, ErrorMatches, `cannot disconnect "ws/consumer:plug" from "ws/producer:slot": not connected`)
	e, _ := err.(*NotConnectedError)
	c.Check(e, NotNil)
}

// Disconnect works when plug and slot exist and are connected
func (s *RepositorySuite) TestDisconnectSucceeds(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	_, err := s.testRepo.Connect(NewConnRef(s.plug, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	_, err = s.testRepo.Connect(NewConnRef(s.plug, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	err = s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, IsNil)
	c.Assert(s.testRepo.Interfaces(), DeepEquals, &Interfaces{
		Plugs: []*sdk.PlugInfo{s.plug},
		Slots: []*sdk.SlotInfo{s.slot},
	})
}

// Tests for Repository.Connected

// Connected fails if sdk name is empty
func (s *RepositorySuite) TestConnectedFailsWithEmptyWorkshopName(c *C) {
	_, err := s.testRepo.Connected(s.projectId, "ws", "", s.plug.Name)
	c.Check(err, ErrorMatches, "internal error: cannot obtain SDK name while computing connections")
}

func (s *RepositorySuite) TestConnectedFailsWithEmptySdkName(c *C) {
	_, err := s.testRepo.Connected(s.projectId, "", "ws", s.plug.Name)
	c.Check(err, ErrorMatches, "internal error: cannot obtain workshop name while computing connections")
}

// Connected fails if plug or slot name is empty
func (s *RepositorySuite) TestConnectedFailsWithEmptyPlugSlotName(c *C) {
	_, err := s.testRepo.Connected(s.projectId, "ws", s.plug.Sdk.Name, "")
	c.Check(err, ErrorMatches, "plug or slot name is empty")
}

// Connected fails if plug or slot doesn't exist
func (s *RepositorySuite) TestConnectedFailsWithoutPlugOrSlot(c *C) {
	_, err1 := s.testRepo.Connected(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	_, err2 := s.testRepo.Connected(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Check(err1, ErrorMatches, `SDK "ws/consumer" has no plug or slot named "plug"`)
	e, _ := err1.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
	c.Check(err2, ErrorMatches, `SDK "ws/producer" has no plug or slot named "slot"`)
	e, _ = err1.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

// Connected finds connections when asked from plug or from slot side
func (s *RepositorySuite) TestConnectedFindsConnections(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	_, err := s.testRepo.Connect(NewConnRef(s.plug, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	conns, err := s.testRepo.Connected(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plug, s.slot)})

	conns, err = s.testRepo.Connected(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plug, s.slot)})
}

// Connected finds connections when asked from plug or from slot side
func (s *RepositorySuite) TestConnections(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	_, err := s.testRepo.Connect(NewConnRef(s.plug, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	conns, err := s.testRepo.Connections(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plug, s.slot)})

	conns, err = s.testRepo.Connections(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plug, s.slot)})

	conns, err = s.testRepo.Connections(s.projectId, "ws", "abc")
	c.Assert(err, IsNil)
	c.Assert(conns, HasLen, 0)
}

func (s *RepositorySuite) TestConnectionsWithSelfConnected(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plugSelf), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	_, err := s.testRepo.Connect(NewConnRef(s.plugSelf, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	conns, err := s.testRepo.Connections(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plugSelf.Sdk.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plugSelf, s.slot)})

	conns, err = s.testRepo.Connections(s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name)
	c.Assert(err, IsNil)
	c.Check(conns, DeepEquals, []*ConnRef{NewConnRef(s.plugSelf, s.slot)})
}

// Tests for Repository.DisconnectAll()

func (s *RepositorySuite) TestDisconnectAll(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)
	_, err := s.testRepo.Connect(NewConnRef(s.plug, s.slot), nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	conns := []*ConnRef{NewConnRef(s.plug, s.slot)}
	s.testRepo.DisconnectAll(conns)
	c.Assert(s.testRepo.Interfaces(), DeepEquals, &Interfaces{
		Plugs: []*sdk.PlugInfo{s.plug},
		Slots: []*sdk.SlotInfo{s.slot},
	})
}

// Tests for Repository.Interfaces()

func (s *RepositorySuite) TestInterfacesSmokeTest(c *C) {
	err := s.testRepo.AddPlug(s.plug)
	c.Assert(err, IsNil)
	err = s.testRepo.AddSlot(s.slot)
	c.Assert(err, IsNil)
	// After connecting the result is as expected
	connRef := NewConnRef(s.plug, s.slot)
	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	ifaces := s.testRepo.Interfaces()
	c.Assert(ifaces, DeepEquals, &Interfaces{
		Plugs:       []*sdk.PlugInfo{s.plug},
		Slots:       []*sdk.SlotInfo{s.slot},
		Connections: []*ConnRef{NewConnRef(s.plug, s.slot)},
	})
	// After disconnecting the connections become empty
	err = s.testRepo.Disconnect(s.plug.Sdk.ProjectId, s.plug.Sdk.Workshop, s.plug.Sdk.Name, s.plug.Name, s.slot.Sdk.ProjectId, s.slot.Sdk.Workshop, s.slot.Sdk.Name, s.slot.Name)
	c.Assert(err, IsNil)
	ifaces = s.testRepo.Interfaces()
	c.Assert(ifaces, DeepEquals, &Interfaces{
		Plugs: []*sdk.PlugInfo{s.plug},
		Slots: []*sdk.SlotInfo{s.slot},
	})
}

// Tests for Repository.SnapSpecification

const testSecurity SecuritySystem = "test"

var testInterface = &ifacetest.TestInterface{
	InterfaceName: "interface",
	TestPermanentPlugCallback: func(spec *ifacetest.Specification, plug *sdk.PlugInfo) error {
		spec.AddSnippet("static plug snippet")
		return nil
	},
	TestConnectedPlugCallback: func(spec *ifacetest.Specification, plug *ConnectedPlug, slot *ConnectedSlot) error {
		spec.AddSnippet("connection-specific plug snippet")
		return nil
	},
	TestPermanentSlotCallback: func(spec *ifacetest.Specification, slot *sdk.SlotInfo) error {
		spec.AddSnippet("static slot snippet")
		return nil
	},
	TestConnectedSlotCallback: func(spec *ifacetest.Specification, plug *ConnectedPlug, slot *ConnectedSlot) error {
		spec.AddSnippet("connection-specific slot snippet")
		return nil
	},
}

func (s *RepositorySuite) TestSdkSpecification(c *C) {
	repo := s.emptyRepo
	backend := &ifacetest.TestSecurityBackend{BackendName: testSecurity}
	c.Assert(repo.AddBackend(backend), IsNil)
	c.Assert(repo.AddInterface(testInterface), IsNil)
	c.Assert(repo.AddPlug(s.plug), IsNil)
	c.Assert(repo.AddSlot(s.slot), IsNil)

	spec, err := repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{"static plug snippet"})

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{"static slot snippet"})

	// Establish connection between plug and slot
	connRef := NewConnRef(s.plug, s.slot)
	_, err = repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{
		"static plug snippet",
		"connection-specific plug snippet",
	})

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{
		"static slot snippet",
		"connection-specific slot snippet",
	})
}

func (s *RepositorySuite) TestSdkSpecificationBoundPlugs(c *C) {
	repo := s.emptyRepo
	backend := &ifacetest.TestSecurityBackend{BackendName: testSecurity}
	c.Assert(repo.AddBackend(backend), IsNil)
	c.Assert(repo.AddInterface(testInterface), IsNil)
	// the plug's connection is bound which means it has the "bind" dynamic
	// attribute that points to the connection it is bound to
	bref := ConnRef{
		PlugRef: s.plug.Ref(),
		SlotRef: s.slot.Ref(),
	}
	bref.PlugRef.Name = "some-plug"
	s.plug.Attrs["bind"] = bref.ID()
	c.Assert(repo.AddPlug(s.plug), IsNil)
	c.Assert(repo.AddSlot(s.slot), IsNil)

	spec, err := repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{"static plug snippet"})

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{"static slot snippet"})

	// Establish connection between plug and slot
	connRef := NewConnRef(s.plug, s.slot)
	_, err = repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	// Ensure that the connection snippet is not generated for the bound plug's
	// connection (it will use the bind's plug connection instead).
	spec, err = repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{
		"static plug snippet",
	})

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, IsNil)
	c.Check(spec.(*ifacetest.Specification).Snippets, DeepEquals, []string{
		"static slot snippet",
	})
}

func (s *RepositorySuite) TestSdkSpecificationFailureWithConnectionSnippets(c *C) {
	var testSecurity SecuritySystem = "security"
	backend := &ifacetest.TestSecurityBackend{BackendName: testSecurity}
	iface := &ifacetest.TestInterface{
		InterfaceName: "interface",
		TestConnectedSlotCallback: func(spec *ifacetest.Specification, plug *ConnectedPlug, slot *ConnectedSlot) error {
			return fmt.Errorf("cannot compute snippet for provider")
		},
		TestConnectedPlugCallback: func(spec *ifacetest.Specification, plug *ConnectedPlug, slot *ConnectedSlot) error {
			return fmt.Errorf("cannot compute snippet for consumer")
		},
	}
	repo := s.emptyRepo

	c.Assert(repo.AddBackend(backend), IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	c.Assert(repo.AddPlug(s.plug), IsNil)
	c.Assert(repo.AddSlot(s.slot), IsNil)
	connRef := NewConnRef(s.plug, s.slot)
	_, err := repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	spec, err := repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, ErrorMatches, "cannot compute snippet for consumer")
	c.Assert(spec, IsNil)

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, ErrorMatches, "cannot compute snippet for provider")
	c.Assert(spec, IsNil)
}

func (s *RepositorySuite) TestSdkSpecificationFailureWithPermanentSnippets(c *C) {
	var testSecurity SecuritySystem = "security"
	iface := &ifacetest.TestInterface{
		InterfaceName: "interface",
		TestPermanentSlotCallback: func(spec *ifacetest.Specification, slot *sdk.SlotInfo) error {
			return fmt.Errorf("cannot compute snippet for provider")
		},
		TestPermanentPlugCallback: func(spec *ifacetest.Specification, plug *sdk.PlugInfo) error {
			return fmt.Errorf("cannot compute snippet for consumer")
		},
	}
	backend := &ifacetest.TestSecurityBackend{BackendName: testSecurity}
	repo := s.emptyRepo
	c.Assert(repo.AddBackend(backend), IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	c.Assert(repo.AddPlug(s.plug), IsNil)
	c.Assert(repo.AddSlot(s.slot), IsNil)
	connRef := NewConnRef(s.plug, s.slot)
	_, err := repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	spec, err := repo.SdkSpecification(s.context, testSecurity, s.plug.Sdk.Ref())
	c.Assert(err, ErrorMatches, "cannot compute snippet for consumer")
	c.Assert(spec, IsNil)

	spec, err = repo.SdkSpecification(s.context, testSecurity, s.slot.Sdk.Ref())
	c.Assert(err, ErrorMatches, "cannot compute snippet for provider")
	c.Assert(spec, IsNil)
}

func (s *RepositorySuite) TestAutoConnectCandidatePlugsAndSlots(c *C) {
	// Add two interfaces, one with automatic connections, one with manual
	repo := s.emptyRepo
	err := repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "auto"})
	c.Assert(err, IsNil)
	err = repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "manual"})
	c.Assert(err, IsNil)

	policyCheck := func(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) {
		return slot.Interface() == "auto", nil
	}

	// Add a pair of snaps with plugs/slots using those two interfaces
	consumer := sdk.MockInfo(c, `
name: consumer
base: ubuntu@22.04
plugs:
    auto:
    manual:
`, s.projectId, "ws")
	producer := sdk.MockInfo(c, `
name: producer
base: ubuntu@22.04
slots:
    auto:
    manual:
`, s.projectId, "ws")
	err = repo.AddSdk(producer)
	c.Assert(err, IsNil)
	err = repo.AddSdk(consumer)
	c.Assert(err, IsNil)

	candidateSlots := repo.AutoConnectCandidateSlots(s.projectId, "ws", "consumer", "auto", policyCheck)
	c.Assert(candidateSlots, HasLen, 1)
	c.Check(candidateSlots[0].Sdk.Name, Equals, "producer")
	c.Check(candidateSlots[0].Interface, Equals, "auto")
	c.Check(candidateSlots[0].Name, Equals, "auto")

	candidatePlugs := repo.AutoConnectCandidatePlugs(s.projectId, "ws", "producer", "auto", policyCheck)
	c.Assert(candidatePlugs, HasLen, 1)
	c.Check(candidatePlugs[0].Sdk.Name, Equals, "consumer")
	c.Check(candidatePlugs[0].Interface, Equals, "auto")
	c.Check(candidatePlugs[0].Name, Equals, "auto")
}

func (s *RepositorySuite) TestAutoConnectCandidatePlugsAndSlotsSymmetry(c *C) {
	repo := s.emptyRepo
	// Add a "auto" interface
	err := repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "auto"})
	c.Assert(err, IsNil)

	policyCheck := func(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) {
		return slot.Interface() == "auto", nil
	}

	// Add a producer sdk for "auto"
	producer := sdk.MockInfo(c, `
name: producer
base: ubuntu@22.04
slots:
    auto:
`, s.projectId, "ws")
	err = repo.AddSdk(producer)
	c.Assert(err, IsNil)

	// Add two consumers snaps for "auto"
	consumer1 := sdk.MockInfo(c, `
name: consumer1
base: ubuntu@22.04
plugs:
    auto:
`, s.projectId, "ws")

	err = repo.AddSdk(consumer1)
	c.Assert(err, IsNil)

	// Add two consumers snaps for "auto"
	consumer2 := sdk.MockInfo(c, `
name: consumer2
base: ubuntu@22.04
plugs:
    auto:
`, s.projectId, "ws")

	err = repo.AddSdk(consumer2)
	c.Assert(err, IsNil)

	// Both can auto-connect
	candidateSlots := repo.AutoConnectCandidateSlots(s.projectId, "ws", "consumer1", "auto", policyCheck)
	c.Assert(candidateSlots, HasLen, 1)
	c.Check(candidateSlots[0].Sdk.Name, Equals, "producer")
	c.Check(candidateSlots[0].Interface, Equals, "auto")
	c.Check(candidateSlots[0].Name, Equals, "auto")

	candidateSlots = repo.AutoConnectCandidateSlots(s.projectId, "ws", "consumer2", "auto", policyCheck)
	c.Assert(candidateSlots, HasLen, 1)
	c.Check(candidateSlots[0].Sdk.Name, Equals, "producer")
	c.Check(candidateSlots[0].Interface, Equals, "auto")
	c.Check(candidateSlots[0].Name, Equals, "auto")

	// Plugs candidates seen from the producer (for example if
	// it's installed after) should be the same
	candidatePlugs := repo.AutoConnectCandidatePlugs(s.projectId, "ws", "producer", "auto", policyCheck)
	c.Assert(candidatePlugs, HasLen, 2)
}

// Tests for AddSdk and RemoveSnap

type AddRemoveSuite struct {
	testutil.BaseTest
	repo      *Repository
	projectId string
}

var _ = Suite(&AddRemoveSuite{})

func (s *AddRemoveSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(snapInfo *sdk.Info) {}))

	s.repo = NewRepository()
	err := s.repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "iface"})
	c.Assert(err, IsNil)
	err = s.repo.AddInterface(&ifacetest.TestInterface{
		InterfaceName:             "invalid",
		BeforePreparePlugCallback: func(plug *sdk.PlugInfo) error { return fmt.Errorf("plug is invalid") },
		BeforePrepareSlotCallback: func(slot *sdk.SlotInfo) error { return fmt.Errorf("slot is invalid") },
	})
	c.Assert(err, IsNil)
}

func (s *AddRemoveSuite) TearDownTest(c *C) {
	s.BaseTest.TearDownTest(c)
	s.projectId = "42424242"
}

func (s *AddRemoveSuite) addSdk(c *C, yaml string, projectId string) (*sdk.Info, error) {
	sdkInfo := sdk.MockInfo(c, yaml, projectId, "ws")
	return sdkInfo, s.repo.AddSdk(sdkInfo)
}

func (s *AddRemoveSuite) TestAddSnapSkipsUnknownInterfaces(c *C) {
	info, err := s.addSdk(c, `
name: bogus
base: ubuntu@22.04
plugs:
  bogus-plug:
slots:
  bogus-slot:
`, s.projectId)
	c.Assert(err, IsNil)
	// the sdk knowns about the bogus plug and slot
	c.Assert(info.Plugs["bogus-plug"], NotNil)
	c.Assert(info.Slots["bogus-slot"], NotNil)
	// but the repository ignores them
	c.Assert(s.repo.Plug(s.projectId, "ws", "bogus", "bogus-plug"), IsNil)
	c.Assert(s.repo.Slot(s.projectId, "ws", "bogus", "bogus-slot"), IsNil)
}

type DisconnectSdkSuite struct {
	testutil.BaseTest
	repo               *Repository
	s1, s2, s2Instance *sdk.Info
}

var _ = Suite(&DisconnectSdkSuite{})

func (s *DisconnectSdkSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(snapInfo *sdk.Info) {}))

	s.repo = NewRepository()

	err := s.repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "iface-a"})
	c.Assert(err, IsNil)
	err = s.repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "iface-b"})
	c.Assert(err, IsNil)

	s.s1 = sdk.MockInfo(c, `
name: s1
base: ubuntu@22.04
plugs:
    iface-a:
slots:
    iface-b:
`, "42424242", "ws")
	err = s.repo.AddSdk(s.s1)
	c.Assert(err, IsNil)

	s.s2 = sdk.MockInfo(c, `
name: s2
base: ubuntu@22.04
plugs:
    iface-b:
slots:
    iface-a:
`, "42424242", "ws")
	c.Assert(err, IsNil)
	err = s.repo.AddSdk(s.s2)
	c.Assert(err, IsNil)
	s.s2Instance = sdk.MockInfo(c, `
name: s2-instance
base: ubuntu@22.04
plugs:
    iface-b:
slots:
    iface-a:
`, "42424242", "ws")
	c.Assert(err, IsNil)
	err = s.repo.AddSdk(s.s2Instance)
	c.Assert(err, IsNil)
}

func (s *DisconnectSdkSuite) TearDownTest(c *C) {
	s.BaseTest.TearDownTest(c)
}

func (s *DisconnectSdkSuite) TestNotConnected(c *C) {
	affected, err := s.repo.DisconnectSdk("42424242", "ws", "s1")
	c.Assert(err, IsNil)
	c.Check(affected, HasLen, 0)
}

func (s *DisconnectSdkSuite) TestOutgoingConnection(c *C) {
	connRef := &ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-a"},
		SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2", Name: "iface-a"}}
	_, err := s.repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	// Disconnect s1 with which has an outgoing connection to s2
	affected, err := s.repo.DisconnectSdk("42424242", "ws", "s1")
	c.Assert(err, IsNil)
	c.Check(affected, testutil.Contains, s.s1)
	c.Check(affected, testutil.Contains, s.s2)
}

func (s *DisconnectSdkSuite) TestIncomingConnection(c *C) {
	connRef := &ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2", Name: "iface-b"},
		SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-b"}}
	_, err := s.repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	// Disconnect s1 with which has an incoming connection from s2
	affected, err := s.repo.DisconnectSdk("42424242", "ws", "s1")
	c.Assert(err, IsNil)
	c.Check(affected, testutil.DeepContains, s.s1)
	c.Check(affected, testutil.DeepContains, s.s2)
}

func (s *DisconnectSdkSuite) TestCrossConnection(c *C) {
	// This test is symmetric wrt s1 <-> s2 connections
	for _, sdkName := range []string{"s1", "s2"} {
		connRef1 := &ConnRef{
			PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-a"},
			SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2", Name: "iface-a"}}
		_, err := s.repo.Connect(connRef1, nil, nil, nil, nil, nil)
		c.Assert(err, IsNil)
		connRef2 := &ConnRef{
			PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2", Name: "iface-b"},
			SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-b"}}
		_, err = s.repo.Connect(connRef2, nil, nil, nil, nil, nil)
		c.Assert(err, IsNil)
		affected, err := s.repo.DisconnectSdk("42424242", "ws", sdkName)
		c.Assert(err, IsNil)
		c.Check(affected, testutil.DeepContains, s.s1)
		c.Check(affected, testutil.DeepContains, s.s2)
	}
}

func (s *DisconnectSdkSuite) TestParallelInstances(c *C) {
	_, err := s.repo.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-a"},
		SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2-instance", Name: "iface-a"}}, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	affected, err := s.repo.DisconnectSdk("42424242", "ws", "s1")
	c.Assert(err, IsNil)
	c.Check(affected, testutil.DeepContains, s.s1)
	c.Check(affected, testutil.DeepContains, s.s2Instance)

	_, err = s.repo.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s2-instance", Name: "iface-b"},
		SlotRef: sdk.SlotRef{ProjectId: "42424242", Workshop: "ws", Sdk: "s1", Name: "iface-b"}}, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	affected, err = s.repo.DisconnectSdk("42424242", "ws", "s1")
	c.Assert(err, IsNil)
	c.Check(affected, testutil.DeepContains, s.s1)
	c.Check(affected, testutil.DeepContains, s.s2Instance)
}

func mountPolicyCheck(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) {
	return true, nil
}

func mountAutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	return plug.Attrs["mount"] == slot.Attrs["mount"]
}

// internal helper that creates a new repository with two SDKs, one
// has a mount plug and one a mount slot
func makeMountConnectionTestSdks(c *C, projectId, plugMountToken, slotMountToken string) *Repository {
	repo := NewRepository()
	err := repo.AddInterface(&ifacetest.TestInterface{InterfaceName: "mount", AutoConnectCallback: mountAutoConnect})
	c.Assert(err, IsNil)

	plugSdk := sdk.MockInfo(c, fmt.Sprintf(`
name: mount-plug-sdk
base: ubuntu@22.04
plugs:
  imported-content:
    interface: mount
    mount: %s
`, plugMountToken), projectId, "ws-importer")
	slotSdk := sdk.MockInfo(c, fmt.Sprintf(`
name: mount-slot-sdk
base: ubuntu@22.04
slots:
  exported-content:
    interface: mount
    mount: %s
`, slotMountToken), projectId, "ws-exporter")

	err = repo.AddSdk(plugSdk)
	c.Assert(err, IsNil)
	err = repo.AddSdk(slotSdk)
	c.Assert(err, IsNil)

	return repo
}

func (s *RepositorySuite) TestAutoConnectMountInterfaceSimple(c *C) {
	repo := makeMountConnectionTestSdks(c, s.projectId, "mylib", "mylib")
	candidateSlots := repo.AutoConnectCandidateSlots(s.projectId, "ws-importer", "mount-plug-sdk", "imported-content", mountPolicyCheck)
	c.Assert(candidateSlots, HasLen, 1)
	c.Check(candidateSlots[0].Name, Equals, "exported-content")
	candidatePlugs := repo.AutoConnectCandidatePlugs(s.projectId, "ws-exporter", "mount-slot-sdk", "exported-content", mountPolicyCheck)
	c.Assert(candidatePlugs, HasLen, 1)
	c.Check(candidatePlugs[0].Name, Equals, "imported-content")
}

func (s *RepositorySuite) TestAutoConnectMountInterfaceNoMatches(c *C) {
	repo := makeMountConnectionTestSdks(c, s.projectId, "mylib", "otherlib")
	candidateSlots := repo.AutoConnectCandidateSlots(s.projectId, "ws-importer", "mount-plug-sdk", "imported-content", mountPolicyCheck)
	c.Check(candidateSlots, HasLen, 0)
	candidatePlugs := repo.AutoConnectCandidatePlugs(s.projectId, "ws-exporter", "mount-slot-sdk", "exported-content", mountPolicyCheck)
	c.Assert(candidatePlugs, HasLen, 0)
}

const ifacehooksSnap1 = `
name: s1
base: ubuntu@22.04
plugs:
  consumer:
    interface: iface2
    attr0: val0
`

const ifacehooksSnap2 = `
name: s2
base: ubuntu@22.04
slots:
  producer:
    interface: iface2
    attr0: val0
`

func (s *RepositorySuite) TestBeforeConnectValidation(c *C) {
	err := s.emptyRepo.AddInterface(&ifacetest.TestInterface{
		InterfaceName: "iface2",
		BeforeConnectSlotCallback: func(slot *ConnectedSlot) error {
			var val string
			if err := slot.Attr("attr1", &val); err != nil {
				return err
			}
			return slot.SetAttr("attr1", fmt.Sprintf("%s-validated", val))
		},
		BeforeConnectPlugCallback: func(plug *ConnectedPlug) error {
			var val string
			if err := plug.Attr("attr1", &val); err != nil {
				return err
			}
			return plug.SetAttr("attr1", fmt.Sprintf("%s-validated", val))
		},
	})
	c.Assert(err, IsNil)

	s1 := sdk.MockInfo(c, ifacehooksSnap1, s.projectId, "ws-s1")
	c.Assert(s.emptyRepo.AddSdk(s1), IsNil)
	s2 := sdk.MockInfo(c, ifacehooksSnap2, s.projectId, "ws-s2")
	c.Assert(s.emptyRepo.AddSdk(s2), IsNil)

	plugDynAttrs := map[string]interface{}{"attr1": "val1"}
	slotDynAttrs := map[string]interface{}{"attr1": "val1"}

	policyCheck := func(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) { return true, nil }
	conn, err := s.emptyRepo.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws-s1", Sdk: "s1", Name: "consumer"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws-s2", Sdk: "s2", Name: "producer"}}, nil, plugDynAttrs, nil, slotDynAttrs, policyCheck)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)

	c.Assert(conn.Plug, NotNil)
	c.Assert(conn.Slot, NotNil)

	c.Assert(conn.Plug.StaticAttrs(), DeepEquals, map[string]interface{}{"attr0": "val0"})
	c.Assert(conn.Plug.DynamicAttrs(), DeepEquals, map[string]interface{}{"attr1": "val1-validated"})
	c.Assert(conn.Slot.StaticAttrs(), DeepEquals, map[string]interface{}{"attr0": "val0"})
	c.Assert(conn.Slot.DynamicAttrs(), DeepEquals, map[string]interface{}{"attr1": "val1-validated"})
}

func (s *RepositorySuite) TestBeforeConnectValidationFailure(c *C) {
	err := s.emptyRepo.AddInterface(&ifacetest.TestInterface{
		InterfaceName: "iface2",
		BeforeConnectSlotCallback: func(slot *ConnectedSlot) error {
			return fmt.Errorf("invalid slot")
		},
		BeforeConnectPlugCallback: func(plug *ConnectedPlug) error {
			return fmt.Errorf("invalid plug")
		},
	})
	c.Assert(err, IsNil)

	s1 := sdk.MockInfo(c, ifacehooksSnap1, s.projectId, "ws-s1")
	c.Assert(s.emptyRepo.AddSdk(s1), IsNil)
	s2 := sdk.MockInfo(c, ifacehooksSnap2, s.projectId, "ws-s2")
	c.Assert(s.emptyRepo.AddSdk(s2), IsNil)

	plugDynAttrs := map[string]interface{}{"attr1": "val1"}
	slotDynAttrs := map[string]interface{}{"attr1": "val1"}

	policyCheck := func(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) { return true, nil }

	conn, err := s.emptyRepo.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws-s1", Sdk: "s1", Name: "consumer"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws-s2", Sdk: "s2", Name: "producer"}}, nil, plugDynAttrs, nil, slotDynAttrs, policyCheck)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, `cannot connect plug "ws-s1/s1:consumer": invalid plug`)
	c.Assert(conn, IsNil)
}

func (s *RepositorySuite) TestBeforeConnectValidationPolicyCheckFailure(c *C) {
	err := s.emptyRepo.AddInterface(&ifacetest.TestInterface{
		InterfaceName:             "iface2",
		BeforeConnectSlotCallback: func(slot *ConnectedSlot) error { return nil },
		BeforeConnectPlugCallback: func(plug *ConnectedPlug) error { return nil },
	})
	c.Assert(err, IsNil)

	s1 := sdk.MockInfo(c, ifacehooksSnap1, s.projectId, "ws-s1")
	c.Assert(s.emptyRepo.AddSdk(s1), IsNil)
	s2 := sdk.MockInfo(c, ifacehooksSnap2, s.projectId, "ws-s2")
	c.Assert(s.emptyRepo.AddSdk(s2), IsNil)

	plugDynAttrs := map[string]interface{}{"attr1": "val1"}
	slotDynAttrs := map[string]interface{}{"attr1": "val1"}

	policyCheck := func(plug *ConnectedPlug, slot *ConnectedSlot) (bool, error) {
		return false, fmt.Errorf("policy check failed")
	}

	conn, err := s.emptyRepo.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws-s1", Sdk: "s1", Name: "consumer"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws-s2", Sdk: "s2", Name: "producer"}}, nil, plugDynAttrs, nil, slotDynAttrs, policyCheck)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, `policy check failed`)
	c.Assert(conn, IsNil)
}

func (s *RepositorySuite) TestConnection(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)

	connRef := NewConnRef(s.plug, s.slot)

	_, err := s.testRepo.Connection(connRef)
	c.Assert(err, ErrorMatches, `no connection from "ws/consumer:plug" to "ws/producer:slot"`)

	_, err = s.testRepo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	conn, err := s.testRepo.Connection(connRef)
	c.Assert(err, IsNil)
	c.Assert(conn.Plug.Name(), Equals, "plug")
	c.Assert(conn.Slot.Name(), Equals, "slot")

	_, err = s.testRepo.Connection(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "a", Name: "b"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "producer", Name: "slot"}})
	c.Assert(err, ErrorMatches, `SDK "ws/a" has no plug named "b"`)
	e, _ := err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)

	_, err = s.testRepo.Connection(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "a", Name: "b"}})
	c.Assert(err, ErrorMatches, `SDK "ws/a" has no slot named "b"`)
	e, _ = err.(*NoPlugOrSlotError)
	c.Check(e, NotNil)
}

func (s *RepositorySuite) TestConnectWithStaticAttrs(c *C) {
	c.Assert(s.testRepo.AddPlug(s.plug), IsNil)
	c.Assert(s.testRepo.AddSlot(s.slot), IsNil)

	connRef := NewConnRef(s.plug, s.slot)

	plugAttrs := map[string]interface{}{"foo": "bar"}
	slotAttrs := map[string]interface{}{"boo": "baz"}
	_, err := s.testRepo.Connect(connRef, plugAttrs, nil, slotAttrs, nil, nil)
	c.Assert(err, IsNil)

	conn, err := s.testRepo.Connection(connRef)
	c.Assert(err, IsNil)
	c.Assert(conn.Plug.Name(), Equals, "plug")
	c.Assert(conn.Slot.Name(), Equals, "slot")
	c.Assert(conn.Plug.StaticAttrs(), DeepEquals, plugAttrs)
	c.Assert(conn.Slot.StaticAttrs(), DeepEquals, slotAttrs)
}

func (s *RepositorySuite) TestInfo(c *C) {
	r := s.emptyRepo

	// Add some test interfaces.
	i1 := &ifacetest.TestInterface{InterfaceName: "i1", InterfaceStaticInfo: StaticInfo{Summary: "i1 summary", DocURL: "http://example.com/i1"}}
	i2 := &ifacetest.TestInterface{InterfaceName: "i2", InterfaceStaticInfo: StaticInfo{Summary: "i2 summary", DocURL: "http://example.com/i2"}}
	i3 := &ifacetest.TestInterface{InterfaceName: "i3", InterfaceStaticInfo: StaticInfo{Summary: "i3 summary", DocURL: "http://example.com/i3"}}
	c.Assert(r.AddInterface(i1), IsNil)
	c.Assert(r.AddInterface(i2), IsNil)
	c.Assert(r.AddInterface(i3), IsNil)

	// Add some test snaps.
	s1 := sdk.MockInfo(c, `
name: s1
base: ubuntu@22.04
plugs:
  i1:
  i2:
`, s.projectId, "ws")
	c.Assert(r.AddSdk(s1), IsNil)

	s2 := sdk.MockInfo(c, `
name: s2
base: ubuntu@22.04
slots:
  i1:
  i3:
`, s.projectId, "ws")
	c.Assert(r.AddSdk(s2), IsNil)

	s3 := sdk.MockInfo(c, `
name: system
base: ubuntu@22.04
type: system
slots:
  i2:
`, s.projectId, "ws")
	c.Assert(r.AddSdk(s3), IsNil)
	s4 := sdk.MockInfo(c, `
name: s4
base: ubuntu@22.04
plugs:
  i2:
`, s.projectId, "ws")
	c.Assert(r.AddSdk(s4), IsNil)

	// Connect a few things for the tests below.
	_, err := r.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "s1", Name: "i1"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "s2", Name: "i1"}}, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	_, err = r.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "s1", Name: "i1"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "s2", Name: "i1"}}, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)
	_, err = r.Connect(&ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "s1", Name: "i2"},
		SlotRef: sdk.SlotRef{ProjectId: s.projectId, Workshop: "ws", Sdk: "system", Name: "i2"}}, nil, nil, nil, nil, nil)
	c.Assert(err, IsNil)

	// Without any names or options we get the summary of all the interfaces.
	infos := r.Info(nil)
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i1", Summary: "i1 summary"},
		{Name: "i2", Summary: "i2 summary"},
		{Name: "i3", Summary: "i3 summary"},
	})

	// We can choose specific interfaces, unknown names are just skipped.
	infos = r.Info(&InfoOptions{Names: []string{"i2", "i4"}})
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i2", Summary: "i2 summary"},
	})

	// We can ask for documentation.
	infos = r.Info(&InfoOptions{Names: []string{"i2"}, Doc: true})
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i2", Summary: "i2 summary", DocURL: "http://example.com/i2"},
	})

	// We can ask for a list of plugs.
	infos = r.Info(&InfoOptions{Names: []string{"i2"}, Plugs: true})
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i2", Summary: "i2 summary", Plugs: []*sdk.PlugInfo{s1.Plugs["i2"], s4.Plugs["i2"]}},
	})

	// We can ask for a list of slots.
	infos = r.Info(&InfoOptions{Names: []string{"i2"}, Slots: true})
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i2", Summary: "i2 summary", Slots: []*sdk.SlotInfo{s3.Slots["i2"]}},
	})

	// We can also ask for only those interfaces that have connected plugs or slots.
	infos = r.Info(&InfoOptions{Connected: true})
	c.Assert(infos, DeepEquals, []*Info{
		{Name: "i1", Summary: "i1 summary"},
		{Name: "i2", Summary: "i2 summary"},
	})
}
