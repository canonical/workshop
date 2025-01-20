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
	"path/filepath"
	"testing"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"gopkg.in/check.v1"
)

type mountSuite struct {
	iface     interfaces.Interface
	projectId string
}

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&mountSuite{
	iface: builtin.MustInterface("mount"),
})

func (s *mountSuite) SetUpTest(c *check.C) {
	s.projectId = "42424242"
}

func (s *mountSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "mount")
}

func (s *mountSuite) TestSanitizeSlotSimple(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
}

func (s *mountSuite) TestSanitizePlugSimple(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: import
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *mountSuite) TestSanitizePlugSimpleNoTarget(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "mount plug must contain target path")
}

func (s *mountSuite) TestSanitizePlugSimpleTargetRelative(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: ../foo
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "mount interface path is not clean:.*")
}

func (s *mountSuite) TestSanitizeSlotOK(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
  workshop-source: /images/low-res
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
}

func (s *mountSuite) TestSanitizeSlotNoSource(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
}

func (s *mountSuite) TestSanitizeSlotAbsSourceFails(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
  workshop-source: root
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot \"workshop-source\" must be absolute`)
}

func (s *mountSuite) TestSanitizeSlotNonLocalSourceFails(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
  workshop-source: ../../../../../../../../root/
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot \"workshop-source\" must be absolute`)
}

func (s *mountSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *mountSuite) TestMountInterface(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  workshop-target: /project/training
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
 mount:
`, s.projectId, "ws", "producer", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec := lxd_device.NewSpecification(&testuser, "consumer")

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the mount specification.
	sourceDir := filepath.Join(testuser.HomeDir, ".local/share/workshop/project/42424242/mount/ws_consumer_mount.sdk")
	expectedMnt := workshop.Mount{Name: plug.Name, What: sourceDir, Where: "/project/training", Type: workshop.HostWorkshop}
	c.Assert(deviceSpec.Profile.Mounts, check.DeepEquals, map[string]workshop.Mount{plug.Name: expectedMnt})
}

func (s *mountSuite) TestMountInterfaceReadOnlyBool(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  workshop-target: /project/training
  read-only: true
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *mountSuite) TestMountInterfaceReadOnlyString(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  workshop-target: /project/training
  read-only: "true"
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *mountSuite) TestMountInterfaceReadOnlyInvalid(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  workshop-target: /project/training
  read-only: "invalid"
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "unknown value \"invalid\" in key \"read-only\" for mount interface plug.*")
}
