package builtin_test

import (
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type desktopSuite struct {
	iface     interfaces.Interface
	projectId string
}

var _ = check.Suite(&desktopSuite{
	iface: builtin.MustInterface("desktop"),
})

func (s *desktopSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
	testuser.HomeDir = c.MkDir()
}

func (s *desktopSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "desktop")
}

func (s *desktopSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *desktopSuite) TestDesktopInterfaceWayland(c *check.C) {
	env := map[string]string{"XDG_RUNTIME_DIR": "/tmp", "WAYLAND_DISPLAY": "wayland-1"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedProxy := &workshop.Desktop{
		Wayland: &workshop.ProxyEntry{
			Name: "desktop_wayland",
			Connect: workshop.ProxyTarget{
				Address:  "/tmp/wayland-1",
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  "/run/user/1000/wayland-0-inside-workshop",
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		},
	}
	c.Assert(deviceSpec.Profile.Desktop, check.DeepEquals, expectedProxy)
}

func (s *desktopSuite) TestDesktopInterfaceX11(c *check.C) {
	env := map[string]string{"DISPLAY": ":0"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedProxy := &workshop.Desktop{
		X11: &workshop.ProxyEntry{
			Name: "desktop_x11",
			Connect: workshop.ProxyTarget{
				Address:  "/tmp/.X11-unix/X0",
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  "/tmp/.X11-unix/X0",
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		},
	}
	c.Assert(deviceSpec.Profile.Desktop, check.DeepEquals, expectedProxy)
}

func (s *desktopSuite) TestDesktopInterfaceBoth(c *check.C) {
	env := map[string]string{"DISPLAY": ":2.0", "WAYLAND_DISPLAY": "/var/run/wayland-0"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedProxy := &workshop.Desktop{
		X11: &workshop.ProxyEntry{
			Name: "desktop_x11",
			Connect: workshop.ProxyTarget{
				Address:  "/tmp/.X11-unix/X2",
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  "/tmp/.X11-unix/X0",
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		},
		Wayland: &workshop.ProxyEntry{
			Name: "desktop_wayland",
			Connect: workshop.ProxyTarget{
				Address:  "/var/run/wayland-0",
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  "/run/user/1000/wayland-0-inside-workshop",
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		},
	}
	c.Assert(deviceSpec.Profile.Desktop, check.DeepEquals, expectedProxy)
}

func (s *desktopSuite) TestDesktopInterfaceXauth(c *check.C) {
	env := map[string]string{"DISPLAY": ":0", "XAUTHORITY": "/tmp/.Xauthority"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedMount := &workshop.Mount{
		Name:      "desktop_xauth",
		Type:      workshop.HostWorkshop,
		What:      filepath.Join(dirs.WorkshopdRunDir, deviceSpec.User.Uid, "Xauthority"),
		Where:     "/var/lib/workshop/run/Xauthority",
		MakeWhere: true,
		Mode:      0o755,
		ReadOnly:  true,
	}
	c.Assert(deviceSpec.Profile.Mounts["desktop_xauth"], check.DeepEquals, *expectedMount)
}

func (s *desktopSuite) TestDesktopInterfaceXauthFail(c *check.C) {
	env := map[string]string{"DISPLAY": ":0"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	_, ok := deviceSpec.Profile.Mounts["desktop_xauth"]
	c.Assert(!ok, check.Equals, true)
}

func (s *desktopSuite) TestDesktopEnvWaylandFail(c *check.C) {
	env := map[string]string{}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.ErrorMatches, "neither DISPLAY nor WAYLAND_DISPLAY.*")
}

func (s *desktopSuite) TestDesktopEnvXDGFail(c *check.C) {
	env := map[string]string{"XDG_RUNTIME_DIR": "", "WAYLAND_DISPLAY": "wayland-7"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.ErrorMatches, "XDG_RUNTIME_DIR is either empty.*")
}

func (s *desktopSuite) TestDesktopEnvX11Fail(c *check.C) {
	env := map[string]string{"DISPLAY": "remote:0"}
	defer osutil.FakeUserAndEnv(func(name string) (*user.User, map[string]string, error) {
		return &testuser, env, nil
	})()

	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 desktop:
  interface: desktop
`, s.projectId, "ws", "consumer", "desktop")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  desktop:
`, s.projectId, "ws", "producer", "desktop")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.ErrorMatches, "desktop interface requires local X server")
}
