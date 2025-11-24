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

package builtin_test

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/asserts"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/interfaces/policy"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type tunnelSuite struct {
	iface          interfaces.Interface
	projectId      string
	restoreUserEnv func()
}

var _ = check.Suite(&tunnelSuite{
	iface: builtin.MustInterface("tunnel"),
})

func (s *tunnelSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
	testuser.HomeDir = c.MkDir()
	s.restoreUserEnv = osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, nil, nil
	})
}

func (s *tunnelSuite) TearDownSuite(c *check.C) {
	s.restoreUserEnv()
}

func (s *tunnelSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "tunnel")
}

func (s *tunnelSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *tunnelSuite) TestSanitizePlugTCP(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 1.1.1.1:8080/tcp
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "1.1.1.1:8080/tcp")
}

func (s *tunnelSuite) TestSanitizePlugUDPOnly(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: udp
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "127.0.0.1/udp")
}

func (s *tunnelSuite) TestSanitizePlugNoTCP(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 0.0.0.0:5555
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "0.0.0.0:5555/tcp")
}

func (s *tunnelSuite) TestSanitizePlugNoHost(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 4321/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "127.0.0.1:4321/udp")
}

func (s *tunnelSuite) TestSanitizePlugHostOnly(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 1.0.0.1
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "1.0.0.1/tcp")
}

func (s *tunnelSuite) TestSanitizePlugNoPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 10.11.12.13/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "10.11.12.13/udp")
}

func (s *tunnelSuite) TestSanitizePlugPortOnly(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 9999
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "127.0.0.1:9999/tcp")
}

func (s *tunnelSuite) TestSanitizePlugLocalhost(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: localhost:0/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "127.0.0.1:0/udp")
}

func (s *tunnelSuite) TestSanitizePlugNoEndpoint(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel:
`, s.projectId, "ws", "tunnel-sdk", "tunnel")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "127.0.0.1/tcp")
}

func (s *tunnelSuite) TestSanitizePlugUnixPath(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: /tmp/unix.sock
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "/tmp/unix.sock")

	plug = builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: $HOME/.local/state/tunnel-sdk/unix.sock
`, s.projectId, "ws", "system", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "$HOME/.local/state/tunnel-sdk/unix.sock")

	plug = builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: $XDG_RUNTIME_DIR/tunnel-sdk.sock
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["endpoint"], check.Equals, "$XDG_RUNTIME_DIR/tunnel-sdk.sock")
}

func (s *tunnelSuite) TestSanitizePlugUnknownAttribute(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    workshop-target: /mnt
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `unknown attribute for tunnel interface plug: "workshop-target"`)
}

func (s *tunnelSuite) TestSanitizePlugWrongType(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: false
`, s.projectId, "ws", "system", "tunnel-plug")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `invalid attribute "endpoint" for plug "ws/system:tunnel-plug": expected string but found bool`)
}

func (s *tunnelSuite) TestSanitizePlugStrayColon(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: :9876/tcp
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `invalid IP address ":9876"`)
}

func (s *tunnelSuite) TestSanitizePlugStraySlash(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 10.2.3.4:80/
`, s.projectId, "ws", "system", "tunnel-plug")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `invalid port "80/": invalid syntax`)
}

func (s *tunnelSuite) TestSanitizePlugInvalidIPv4(c *check.C) {
	plug := builtin.MockPlug(c, `name: tunnel-sdk
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 10.2.3.4.5.6:7777/tcp
`, s.projectId, "ws", "tunnel-sdk", "tunnel-plug")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `invalid IP address "10.2.3.4.5.6"`)
}

func (s *tunnelSuite) TestSanitizeSlotUDP(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[2606:4700:4700::1111]:53/udp'
`, s.projectId, "ws", "system", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "[2606:4700:4700::1111]:53/udp")
}

func (s *tunnelSuite) TestSanitizeSlotNoTCP(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[::]:4567'
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "[::]:4567/tcp")
}

func (s *tunnelSuite) TestSanitizeSlotUDPOnly(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: udp
`, s.projectId, "ws", "system", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "127.0.0.1/udp")
}

func (s *tunnelSuite) TestSanitizeSlotHostOnly(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 2606:4700:4700::1001
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "2606:4700:4700::1001/tcp")
}

func (s *tunnelSuite) TestSanitizeSlotNoPort(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[::ffff:192.168.1.23]/udp'
`, s.projectId, "ws", "system", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "::ffff:192.168.1.23/udp")
}

func (s *tunnelSuite) TestSanitizeSlotPortOnly(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '11111'
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "127.0.0.1:11111/tcp")
}

func (s *tunnelSuite) TestSanitizeSlotLocalhost(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: ip6-localhost:1234/udp
`, s.projectId, "ws", "system", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "[::1]:1234/udp")
}

func (s *tunnelSuite) TestSanitizeSlotUnixAbstract(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '@abstract.sock'
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["endpoint"], check.Equals, "@abstract.sock")
}

func (s *tunnelSuite) TestSanitizeSlotUnknownAttribute(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    host-source: /home/user/snap
`, s.projectId, "ws", "system", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `unknown attribute for tunnel interface slot: "host-source"`)
}

func (s *tunnelSuite) TestSanitizeSlotStrayColon(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[fd12:3456:789a:1::1]:/udp'
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `invalid IP address "\[fd12:3456:789a:1::1\]:"`)
}

func (s *tunnelSuite) TestSanitizeSlotBracketedIPv4(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[2.2.2.2]:8080'
`, s.projectId, "ws", "system", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `invalid bracketed address "\[2\.2\.2\.2\]:8080"`)
}

func (s *tunnelSuite) TestSanitizeSlotBracketedHost(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[localhost]'
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `invalid bracketed address "\[localhost\]"`)
}

func (s *tunnelSuite) TestSanitizeSlotInvalidIPv6(c *check.C) {
	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: '[:::]:8080/udp'
`, s.projectId, "ws", "system", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `invalid IP address ":::"`)
}

func (s *tunnelSuite) TestSanitizeSlotInvalidPort(c *check.C) {
	slot := builtin.MockSlot(c, `name: tunnel-sdk
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 66666
`, s.projectId, "ws", "tunnel-sdk", "tunnel-slot")
	err := interfaces.BeforePrepareSlot(s.iface, slot)
	c.Check(err, check.ErrorMatches, `invalid port "66666": value out of range`)
}

func (s *tunnelSuite) TestConnectHostToWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 0.0.0.0:12345/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/udp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "127.0.0.1:54321",
			Protocol: "udp",
		},
		Listen: workshop.ProxyTarget{
			Address:  "0.0.0.0:12345",
			Protocol: "udp",
		},
		Direction: workshop.HostToWorkshop,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestConnectWorkshopToHost(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: /run/tunnel.sock
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "127.0.0.1:54321",
			Protocol: "tcp",
		},
		Listen: workshop.ProxyTarget{
			Address:  "/run/tunnel.sock",
			Protocol: "unix",
		},
		Direction: workshop.WorkshopToHost,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestConnectHostToHost(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:12345/tcp
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws")

	plug := info.Plugs["tunnel-plug"]
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := info.Slots["tunnel-slot"]
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "cannot connect system SDK to itself")
}

func (s *tunnelSuite) TestConnectWorkshopToWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:12345/tcp
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "cannot connect regular SDKs from the same workshop")
}

func (s *tunnelSuite) TestConnectUDPToTCP(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:12345/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "udp and tcp are incompatible")
}

func (s *tunnelSuite) TestConnectUnixToUDP(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: /tmp/sock
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/udp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "unix and udp are incompatible")
}

func (s *tunnelSuite) TestInferListenPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1/tcp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "127.0.0.1:54321",
			Protocol: "tcp",
		},
		Listen: workshop.ProxyTarget{
			Address:  "127.0.0.1:54321",
			Protocol: "tcp",
		},
		Direction: workshop.HostToWorkshop,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestInferConnectPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:12345/tcp
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1/tcp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "127.0.0.1:12345",
			Protocol: "tcp",
		},
		Listen: workshop.ProxyTarget{
			Address:  "127.0.0.1:12345",
			Protocol: "tcp",
		},
		Direction: workshop.WorkshopToHost,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestInferBothPorts(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1/udp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1/udp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "both ports unspecified")
}

func (s *tunnelSuite) TestInferPortFromSocket(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: /tmp/sock
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1/tcp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "both ports unspecified")
}

func (s *tunnelSuite) TestListenAnyHostPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:0/tcp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "port 0 not currently supported")
}

func (s *tunnelSuite) TestListenAnyWorkshopPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:0/tcp
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:54321/tcp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "port 0 not currently supported")
}

func (s *tunnelSuite) TestListenPrivilegedPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:22/tcp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:80/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, "port 22 is privileged")
}

func (s *tunnelSuite) TestConnectPrivilegedPort(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:22/tcp
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:80/tcp
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "127.0.0.1:80",
			Protocol: "tcp",
		},
		Listen: workshop.ProxyTarget{
			Address:  "127.0.0.1:22",
			Protocol: "tcp",
		},
		Direction: workshop.WorkshopToHost,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestExpandHomeDirectory(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: $HOME/.local/state/app/unix.sock
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: $HOME/app/unix.sock
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	socket := filepath.Join(testuser.HomeDir, ".local", "state", "app", "unix.sock")
	c.Assert(os.MkdirAll(filepath.Dir(socket), os.ModePerm), check.IsNil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "/home/workshop/app/unix.sock",
			Protocol: "unix",
		},
		Listen: workshop.ProxyTarget{
			Address:  socket,
			Protocol: "unix",
		},
		Direction: workshop.HostToWorkshop,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestExpandRuntimeDirectory(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: $XDG_RUNTIME_DIR/app.sock
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: $XDG_RUNTIME_DIR/app/unix.sock
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	testuser.Uid = "1111"
	defer func() { testuser.Uid = "" }()

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the tunnel specification.
	expectedEntry := workshop.ProxyEntry{
		Name: "tunnel-plug",
		Connect: workshop.ProxyTarget{
			Address:  "/run/user/1111/app/unix.sock",
			Protocol: "unix",
		},
		Listen: workshop.ProxyTarget{
			Address:  "/run/user/1000/app.sock",
			Protocol: "unix",
		},
		Direction: workshop.WorkshopToHost,
	}
	c.Check(deviceSpec.Profile.Tunnels, check.DeepEquals, []workshop.Tunnel{{ProxyEntry: expectedEntry}})
}

func (s *tunnelSuite) TestExpandOtherDirectory(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: $PWD/app.sock
`, s.projectId, "ws", "client", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: $PWD/app/unix.sock
`, s.projectId, "ws", "system", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "client")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, `unexpected variable "PWD"`)
}

func (s *tunnelSuite) TestUnauthorizedUnixPath(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: /etc/shadow
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: /run/app.sock
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "system")
	c.Assert(err, check.IsNil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, `user "testuser" cannot create socket "/etc/shadow" for security reasons`)

	fakeEtc := c.MkDir()
	shadowLink := filepath.Join(testuser.HomeDir, "etc", "shadow")
	c.Assert(os.Symlink(fakeEtc, filepath.Dir(shadowLink)), check.IsNil)
	plug.Attrs["endpoint"] = shadowLink
	connectedPlug = interfaces.NewConnectedPlug(plug, nil, nil)

	err = deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot)
	c.Check(err, check.ErrorMatches, fmt.Sprintf(`user "testuser" cannot create socket %q for security reasons`, shadowLink))
}

func (s *tunnelSuite) TestAutoConnectHostToWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "system", "web")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "service", "web")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *tunnelSuite) TestAutoConnectWorkshopToHost(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "client", "web")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "system", "web")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "tunnel"`)
}

func (s *tunnelSuite) TestAutoConnectHostToHost(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
slots:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8000/tcp
`, s.projectId, "ws")

	plug := info.Plugs["web"]
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := info.Slots["web"]
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "tunnel"`)
}

func (s *tunnelSuite) TestAutoConnectWorkshopToWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: client
base: ubuntu@22.04
plugs:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "client", "web")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "service", "web")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "tunnel"`)
}

func (s *tunnelSuite) TestAutoConnectDifferentName(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "tunnel"`)
}

func (s *tunnelSuite) TestAutoConnectExplicit(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  tunnel-plug:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "system", "tunnel-plug")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	// Simulate a workshop definition file with:
	// connections:
	//   - plug: system:tunnel-plug
	//     slot: service:tunnel-slot
	connectedPlug.SetAttr("auto-explicit", "true")

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  tunnel-slot:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "service", "tunnel-slot")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *tunnelSuite) TestAutoConnectExplicitAndSameName(c *check.C) {
	plug := builtin.MockPlug(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "system", "web")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	// Simulate a workshop definition file with:
	// connections:
	//   - plug: system:web
	//     slot: service:web
	connectedPlug.SetAttr("auto-explicit", "true")

	slot := builtin.MockSlot(c, `name: service
base: ubuntu@22.04
slots:
  web:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
`, s.projectId, "ws", "service", "web")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	cc := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := cc.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *tunnelSuite) TestAutoConnectLocalhost(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
plugs:
  loopback4:
    interface: tunnel
    endpoint: 127.0.0.1:8080/tcp
  loopback6:
    interface: tunnel
    endpoint: '[::1]:8080/tcp'
  wildcard4:
    interface: tunnel
    endpoint: 0.0.0.0:8080/tcp
  wildcard6:
    interface: tunnel
    endpoint: '[::]:8080/tcp'
`, s.projectId, "ws")

	iface, err := interfaces.ByName("tunnel")
	c.Assert(err, check.IsNil)

	c.Check(iface.AutoConnect(info.Plugs["loopback4"], nil), check.Equals, true)
	c.Check(iface.AutoConnect(info.Plugs["loopback6"], nil), check.Equals, true)
	c.Check(iface.AutoConnect(info.Plugs["wildcard4"], nil), check.Equals, false)
	c.Check(iface.AutoConnect(info.Plugs["wildcard6"], nil), check.Equals, false)
}
