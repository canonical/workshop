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
	"os/user"
	"path/filepath"
	"testing"

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

type mountSuite struct {
	iface          interfaces.Interface
	projectId      string
	restoreUserEnv func()
	env            map[string]string
}

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&mountSuite{
	iface: builtin.MustInterface("mount"),
})

func (s *mountSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
	testuser.HomeDir = c.MkDir()
	s.env = make(map[string]string)
	s.restoreUserEnv = osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, s.env, nil
	})
}

func (s *mountSuite) TearDownSuite(c *check.C) {
	s.restoreUserEnv()
}

func (s *mountSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "mount")
}

func (s *mountSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *mountSuite) TestInstallSystemSdkSlot(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
slots:
 mount:
`, s.projectId, "ws")

	ic := policy.InstallCandidate{
		Sdk:             info,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	c.Check(ic.Check(), check.IsNil)
}

func (s *mountSuite) TestInstallOtherSystemSdkSlot(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
slots:
 m:
  interface: mount
`, s.projectId, "ws")

	ic := policy.InstallCandidate{
		Sdk:             info,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	c.Check(ic.Check(), check.ErrorMatches, `installation not allowed by "m" slot rule of interface "mount"`)
}

func (s *mountSuite) TestInstallRegularSdkSlot(c *check.C) {
	info := sdk.MockInfo(c, `name: producer
base: ubuntu@22.04
slots:
 m:
  interface: mount
  workshop-source: /opt
`, s.projectId, "ws")

	ic := policy.InstallCandidate{
		Sdk:             info,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	c.Check(ic.Check(), check.IsNil)
}

func (s *mountSuite) TestInstallSystemSdkPlug(c *check.C) {
	info := sdk.MockInfo(c, `name: system
base: ubuntu@22.04
type: system
plugs:
 mount:
`, s.projectId, "ws")

	ic := policy.InstallCandidate{
		Sdk:             info,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	c.Check(ic.Check(), check.ErrorMatches, `installation not allowed by "mount" plug rule of interface "mount"`)
}

func (s *mountSuite) TestInstallRegularSdkPlug(c *check.C) {
	info := sdk.MockInfo(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /mnt
`, s.projectId, "ws")

	ic := policy.InstallCandidate{
		Sdk:             info,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	c.Check(ic.Check(), check.IsNil)
}

func (s *mountSuite) TestSanitizeSlotSystem(c *check.C) {
	const mockSdkYaml = `name: system
base: ubuntu@22.04
type: system
slots:
 mount-slot:
  interface: mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
}

func (s *mountSuite) TestSanitizeSlotSystemWorkshopSource(c *check.C) {
	const mockSdkYaml = `name: system
base: ubuntu@22.04
type: system
slots:
 mount-slot:
  interface: mount
  workshop-source: /opt
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `unknown attribute for system mount interface slot: "workshop-source"`)
}

func (s *mountSuite) TestSanitizeSlotSystemHostSource(c *check.C) {
	const mockSdkYaml = `name: system
base: ubuntu@22.04
type: system
slots:
 mount-slot:
  interface: mount
  host-source: /usr
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `unknown attribute for system mount interface slot: "host-source"`)
}

func (s *mountSuite) TestSanitizePlugHome(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: /home/workshop/.cache/mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["mode"], check.Equals, int64(0o775))
	c.Check(plug.Attrs["uid"], check.Equals, int64(1000))
	c.Check(plug.Attrs["gid"], check.Equals, int64(1000))
}

func (s *mountSuite) TestSanitizePlugRoot(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: /root/.cache/mount
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["mode"], check.Equals, int64(0o755))
	c.Check(plug.Attrs["uid"], check.Equals, int64(0))
	c.Check(plug.Attrs["gid"], check.Equals, int64(0))
}

func (s *mountSuite) TestSanitizePlugSDK(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: $SDK
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["workshop-target"], check.Equals, "/var/lib/workshop/sdk/mount-slot-sdk")
}

func (s *mountSuite) TestSanitizePlugSDKSubdir(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: ${SDK}/lib/x86_64-linux-gnu
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["workshop-target"], check.Equals, "/var/lib/workshop/sdk/mount-slot-sdk/lib/x86_64-linux-gnu")
}

func (s *mountSuite) TestSanitizePlugSDKUnclean(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: $SDK/
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, `mount plug "workshop-target" is not clean: "/var/lib/workshop/sdk/mount-slot-sdk/"`)
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
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, `mount plug must contain "workshop-target"`)
}

func (s *mountSuite) TestSanitizePlugSimpleTargetRelative(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: foo/bar
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, `mount plug "workshop-target" must be absolute: "foo/bar"`)
}

func (s *mountSuite) TestSanitizePlugSimpleTargetUnclean(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
plugs:
 mount-plug:
  interface: mount
  workshop-target: /usr/../etc/passwd
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	plug := info.Plugs["mount-plug"]
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, `mount plug "workshop-target" is not clean: "/usr/../etc/passwd"`)
}

func (s *mountSuite) TestSanitizePlugModeOwner(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0o642
  uid: 123
  gid: 456
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["mode"], check.Equals, int64(0o642))
	c.Check(plug.Attrs["uid"], check.Equals, int64(123))
	c.Check(plug.Attrs["gid"], check.Equals, int64(456))
}

func (s *mountSuite) TestSanitizePlugModeOwnerOldYAML(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0753
  uid: 1_000
  gid: 4_321
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
	c.Check(plug.Attrs["mode"], check.Equals, int64(0o753))
	c.Check(plug.Attrs["uid"], check.Equals, int64(1000))
	c.Check(plug.Attrs["gid"], check.Equals, int64(4321))
}

func (s *mountSuite) TestSanitizePlugModeInvalid(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: rwxrwxrwx
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "unknown value \"rwxrwxrwx\" in key \"mode\" for mount interface plug.*")
}

func (s *mountSuite) TestSanitizePlugModeNegative(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: -1
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "invalid value -01 in key \"mode\" for mount interface plug: permissions limited to 0777")
}

func (s *mountSuite) TestSanitizePlugModeSticky(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0o1000
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "invalid value 01000 in key \"mode\" for mount interface plug: permissions limited to 0777")
}

func (s *mountSuite) TestSanitizePlugUidInvalid(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  uid: workshop
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "unknown value \"workshop\" in key \"uid\" for mount interface plug.*")
}

func (s *mountSuite) TestSanitizePlugUidNegative(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  uid: -1
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "invalid value -1 in key \"uid\" for mount interface plug: must be between 0 and 0xffffffff")
}

func (s *mountSuite) TestSanitizePlugGidInvalid(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  gid: video
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "unknown value \"video\" in key \"gid\" for mount interface plug.*")
}

func (s *mountSuite) TestSanitizePlugGidNegative(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  gid: -1
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "invalid value -1 in key \"gid\" for mount interface plug: must be between 0 and 0xffffffff")
}

func (s *mountSuite) TestSanitizePlugReadOnlyBool(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  read-only: true
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *mountSuite) TestSanitizePlugReadOnlyString(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  read-only: "true"
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.IsNil)
}

func (s *mountSuite) TestSanitizePlugReadOnlyInvalid(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  read-only: "invalid"
`, s.projectId, "ws", "consumer", "mount")
	c.Assert(interfaces.BeforePreparePlug(s.iface, plug), check.ErrorMatches, "unknown value \"invalid\" in key \"read-only\" for mount interface plug.*")
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

func (s *mountSuite) TestSanitizeSlotSDKVariable(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
  workshop-source: $SDK/training
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.IsNil)
	c.Check(slot.Attrs["workshop-source"], check.Equals, "/var/lib/workshop/sdk/mount-slot-sdk/training")
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
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot must contain "workshop-source"`)
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
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot \"workshop-source\" must be absolute: "root"`)
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
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot \"workshop-source\" must be absolute: "../../../../../../../../root/"`)
}

func (s *mountSuite) TestSanitizeSlotUncleanSourceFails(c *check.C) {
	const mockSdkYaml = `name: mount-slot-sdk
base: ubuntu@22.04
slots:
 mount-slot:
  interface: mount
  workshop-source: /tmp/../etc/shadow
`
	info := sdk.MockInfo(c, mockSdkYaml, s.projectId, "ws")
	slot := info.Slots["mount-slot"]
	c.Assert(interfaces.BeforePrepareSlot(s.iface, slot), check.ErrorMatches, `mount slot \"workshop-source\" is not clean: "/tmp/../etc/shadow"`)
}

func (s *mountSuite) TestConnectHostWorkshopXdgUnset(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0
  uid: 0
  gid: 0
  read-only: false
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
 mount:
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	s.env["XDG_DATA_HOME"] = ""

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the mount specification.
	sourceDir := filepath.Join(testuser.HomeDir, ".local/share/workshop/id/42424242/ws/mount/consumer/mount")
	expectedMnt := workshop.Mount{
		Name:      plug.Name,
		Type:      workshop.HostWorkshop,
		What:      sourceDir,
		MakeWhat:  true,
		Where:     "/project/training",
		MakeWhere: true,
	}
	c.Assert(deviceSpec.Profile.Mounts, check.DeepEquals, map[string]workshop.Mount{plug.Name: expectedMnt})
}

func (s *mountSuite) TestConnectHostWorkshopXdgSet(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0o755
  # Use float to simulate passing through the state
  uid: 123.0
  gid: 321
  read-only: false
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
 mount:
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	s.env["XDG_DATA_HOME"] = c.MkDir()

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the mount specification.
	sourceDir := filepath.Join(s.env["XDG_DATA_HOME"], "/workshop/id/42424242/ws/mount/consumer/mount")
	expectedMnt := workshop.Mount{
		Name:      plug.Name,
		Type:      workshop.HostWorkshop,
		What:      sourceDir,
		MakeWhat:  true,
		Where:     "/project/training",
		MakeWhere: true,
		Mode:      0o755,
		Owner:     123,
		Group:     321,
	}
	c.Assert(deviceSpec.Profile.Mounts, check.DeepEquals, map[string]workshop.Mount{plug.Name: expectedMnt})
}

func (s *mountSuite) TestConnectWorkshopWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
  mode: 0
  uid: 0
  gid: 0
  read-only: false
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
 mount:
  interface: mount
  workshop-source: /var/lib/workshop/sdk/producer/training
`, s.projectId, "ws", "producer", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the mount specification.
	expectedMnt := workshop.Mount{
		Name:      plug.Name,
		Type:      workshop.WorkshopWorkshop,
		What:      "/var/lib/workshop/sdk/producer/training",
		Where:     "/project/training",
		MakeWhere: true,
	}
	c.Assert(deviceSpec.Profile.Mounts, check.DeepEquals, map[string]workshop.Mount{plug.Name: expectedMnt})
}

func (s *mountSuite) TestAutoConnectHostWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
 mount:
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	ic := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := ic.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *mountSuite) TestAutoConnectHostWorkshopExplicit(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	// Simulate a workshop definition file with:
	// connections:
	//   - plug: consumer:mount
	//     slot: system:mount
	connectedPlug.SetAttr("auto-explicit", "true")

	slot := builtin.MockSlot(c, `name: system
base: ubuntu@22.04
type: system
slots:
 mount:
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	ic := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := ic.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *mountSuite) TestAutoConnectWorkshopWorkshop(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
 mount:
  interface: mount
  workshop-source: $SDK/training
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	ic := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := ic.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "mount"`)
}

func (s *mountSuite) TestAutoConnectWorkshopWorkshopExplicit(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 mount:
  interface: mount
  workshop-target: /project/training
`, s.projectId, "ws", "consumer", "mount")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	// Simulate a workshop definition file with:
	// connections:
	//   - plug: consumer:mount
	//     slot: producer:mount
	connectedPlug.SetAttr("auto-explicit", "true")

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
 mount:
  interface: mount
  workshop-source: $SDK/training
`, s.projectId, "ws", "system", "mount")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	ic := policy.ConnectCandidate{
		Plug:            connectedPlug,
		Slot:            connectedSlot,
		BaseDeclaration: asserts.BuiltinBaseDeclaration(),
	}
	_, err := ic.CheckAutoConnect()
	c.Check(err, check.IsNil)
}
