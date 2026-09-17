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
	"context"
	"os/user"

	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type customDeviceSuite struct {
	iface             interfaces.Interface
	projectId         string
	restoreUserLookup func()
	restoreUserEnv    func()
	restoreLxdInfo    func()
}

var _ = check.Suite(&customDeviceSuite{
	iface: builtin.MustInterface("custom-device"),
})

func (s *customDeviceSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return &testuser, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})
	s.restoreLxdInfo = lxd_device.MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
}

func (s *customDeviceSuite) TearDownSuite(c *check.C) {
	s.restoreLxdInfo()
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *customDeviceSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "custom-device")
}

func (s *customDeviceSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *customDeviceSuite) TestCustomDeviceInterface(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: accel
`, s.projectId, "ws", "consumer", "mydevice")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  custom-device:
`, s.projectId, "ws", "producer", "custom-device")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the device specification.
	expectedDevices := []workshop.CustomDevice{{Name: "mydevice", Subsystem: "accel"}}
	c.Assert(deviceSpec.Profile.CustomDevices, check.DeepEquals, expectedDevices)
}

func (s *customDeviceSuite) TestCustomDeviceInterfaceWithFilters(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    vendorid: "0403"
    productid: "6001"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@24.04
slots:
  custom-device:
`, s.projectId, "ws", "producer", "custom-device")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	expectedDevices := []workshop.CustomDevice{{
		Name:      "mydevice",
		Subsystem: "tty",
		VendorID:  "0403",
		ProductID: "6001",
	}}
	c.Assert(deviceSpec.Profile.CustomDevices, check.DeepEquals, expectedDevices)
}

func (s *customDeviceSuite) TestCustomDeviceInterfaceWithVendorIdFilter(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    vendorid: "0403"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@24.04
slots:
  custom-device:
`, s.projectId, "ws", "producer", "custom-device")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	expectedDevices := []workshop.CustomDevice{{
		Name:      "mydevice",
		Subsystem: "tty",
		VendorID:  "0403",
	}}
	c.Assert(deviceSpec.Profile.CustomDevices, check.DeepEquals, expectedDevices)
}

func (s *customDeviceSuite) TestCustomDeviceInterfaceWithoutSubsystem(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    vendorid: "0403"
    productid: "6001"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@24.04
slots:
  custom-device:
`, s.projectId, "ws", "producer", "custom-device")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	expectedDevices := []workshop.CustomDevice{{
		Name:      "mydevice",
		VendorID:  "0403",
		ProductID: "6001",
	}}
	c.Assert(deviceSpec.Profile.CustomDevices, check.DeepEquals, expectedDevices)
}

func (s *customDeviceSuite) TestSanitizePlugUnknownAttribute(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  mydevice:
    interface: custom-device
    workshop-target: /mnt
`, s.projectId, "ws", "consumer", "mydevice")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `unknown attribute for custom-device interface plug: "workshop-target"`)
}

func (s *customDeviceSuite) TestSanitizePlugNoFilters(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  mydevice:
    interface: custom-device
`, s.projectId, "ws", "consumer", "mydevice")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `custom-device plug must contain "subsystem", "vendorid", or "productid"`)
}

func (s *customDeviceSuite) TestSanitizePlugWithFilters(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    vendorid: "0403"
    productid: "6001"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Check(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *customDeviceSuite) TestSanitizePlugWithVendorIdFilter(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    vendorid: "0403"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Check(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *customDeviceSuite) TestSanitizePlugProductIdRequiresVendorId(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    productid: "6001"
`, s.projectId, "ws", "consumer", "mydevice")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `custom-device plug contains "productid" without "vendorid"`)
}

func (s *customDeviceSuite) TestSanitizePlugInvalidProductId(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    productid: "60019"
`, s.projectId, "ws", "consumer", "mydevice")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `custom-device plug "productid" must be a hexadecimal number`)
}

func (s *customDeviceSuite) TestSanitizePlugInvalidVendorId(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
    vendorid: "wxyz"
`, s.projectId, "ws", "consumer", "mydevice")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `custom-device plug "vendorid" must be a hexadecimal number`)
}

func (s *customDeviceSuite) TestSanitizePlugSubsystemOnly(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: tty
`, s.projectId, "ws", "consumer", "mydevice")
	c.Check(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *customDeviceSuite) TestSanitizePlugNormalizesIds(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@24.04
plugs:
  mydevice:
    interface: custom-device
    vendorid: "EF01"
    productid: "ABCD"
`, s.projectId, "ws", "consumer", "mydevice")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["vendorid"], check.Equals, "ef01")
	c.Check(plug.Attrs["productid"], check.Equals, "abcd")
}

func (s *customDeviceSuite) TestCustomDeviceInterfaceRequiresContainer(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  mydevice:
    interface: custom-device
    subsystem: accel
`, s.projectId, "ws", "consumer", "mydevice")

	container := &lxd_device.Instance{Confinement: workshop.ConfinementContainer}
	c.Check(container.SupportsPlug(s.iface, plug), check.IsNil)

	vm := &lxd_device.Instance{Confinement: workshop.ConfinementVirtualMachine}
	c.Check(vm.SupportsPlug(s.iface, plug), check.ErrorMatches, `custom-device interface only available to containers`)
}
