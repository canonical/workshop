package lxd_device

import (
	"encoding/json"
	"fmt"
	"os/user"
	"strconv"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

func NewSpecification(user string, sdk string) (*Specification, error) {
	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return nil, err
	}

	return &Specification{
		devices:     make(map[string]map[string]string),
		config:      make(map[string]string),
		Profile:     workshop.NewSdkProfile(sdk),
		User:        usr,
		Environment: env,
	}, nil
}

type Specification struct {
	Profile workshop.SdkProfile

	devices map[string]map[string]string
	config  map[string]string

	User        *user.User
	Environment map[string]string
}

// AddPermanentSlot records side-effects of having a slot.
func (s *Specification) AddPermanentSlot(iface interfaces.Interface, slot *sdk.SlotInfo) error {
	return nil
}

// AddPermanentPlug records side-effects of having a plug.
func (s *Specification) AddPermanentPlug(iface interfaces.Interface, plug *sdk.PlugInfo) error {
	return nil
}

// AddConnectedSlot records side-effects of having a connected slot.
func (s *Specification) AddConnectedSlot(iface interfaces.Interface, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	type definer interface {
		MountConnectedSlot(spec *Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error
	}
	if iface, ok := iface.(definer); ok {
		return iface.MountConnectedSlot(s, plug, slot)
	}
	return nil
}

// AddConnectedPlug records side-effects of having a connected plug.
func (s *Specification) AddConnectedPlug(iface interfaces.Interface, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	type definer interface {
		MountConnectedPlug(spec *Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error
	}

	if iface, ok := iface.(definer); ok {
		return iface.MountConnectedPlug(s, plug, slot)
	}
	return nil
}

func (s *Specification) AddMountEntry(dev workshop.Mount) error {
	s.Profile.Mounts[dev.Name] = dev

	name := lxdbackend.DeviceName(s.Profile.Sdk, dev.Name)

	if dev.Type == workshop.WorkshopWorkshop {
		s.devices[name] = map[string]string{"type": "none"}
		buf, err := json.Marshal(dev)
		if err != nil {
			return err
		}
		s.config[lxdbackend.DeviceConfigKey(name)] = string(buf)
		s.config[lxdbackend.DeviceTypeConfigKey(name)] = "mount"
	}

	if dev.Type == workshop.HostWorkshop {
		s.devices[name] = map[string]string{
			"type":             "disk",
			"source":           dev.What,
			"user.make-source": strconv.FormatBool(dev.MakeWhat),
			"path":             dev.Where,
			"user.make-path":   strconv.FormatBool(dev.MakeWhere),
			"user.path-mode":   fmt.Sprintf("%#o", dev.Mode),
			"user.path-owner":  strconv.FormatUint(uint64(dev.Owner), 10),
			"user.path-group":  strconv.FormatUint(uint64(dev.Group), 10),
			"readonly":         strconv.FormatBool(dev.ReadOnly),
		}
	}

	return nil
}

// Tunnel, SSH Agent and Desktop are all of the lxc 'proxy' type
// These are network protocol proxy devices that open a port on the host or in a workhop.
// 'listen', 'connect' are the source and destination addresses (paths in the case of unix sockets),
// see https://documentation.ubuntu.com/lxd/en/latest/reference/devices_proxy/#device-proxy-device-conf:bind
// bind denotes where the port is open (can be: instance, host)

func (s *Specification) AddTunnelEntry(tunnel workshop.Tunnel) error {
	s.Profile.Tunnels = append(s.Profile.Tunnels, tunnel)
	s.addProxyEntry(&tunnel.ProxyEntry, "tunnel")
	return nil
}

func (s *Specification) SetSshAgent(agent workshop.SshAgent) error {
	s.Profile.Agent = &agent
	s.addProxyEntry(&agent.ProxyEntry, "ssh-agent")
	return nil
}

func (s *Specification) SetDesktop(desktop workshop.Desktop) error {
	s.Profile.Desktop = &desktop

	if desktop.Wayland != nil {
		s.addProxyEntry(desktop.Wayland, "desktop-wayland")
	}

	if desktop.X11 != nil {
		s.addProxyEntry(desktop.X11, "desktop-x11")
	}

	return nil
}

func (s *Specification) SetGpu(gpu workshop.Gpu) error {
	s.Profile.Gpu = &gpu

	// The default workshop user must be able to acces the GPU device.
	// Workshop assigns the GPU devices to workshop.workshop. A more
	// traditional way here would be to add dri devices to the video/render
	// groups, but it requires an additional workshop exec to find out the
	// groups' ids at the LXD profile generation time. Given that we are
	// solving the problem of access in a confined environment and workshop
	// is a passwordless sudo user anyway, it was decided that it is OK if
	// the workshop user owns GPU devices.

	// On another note, the render and video groups are not assigned to the
	// card*/render* dri devices by LXD properly. Both will be assigned to
	// the group provided in "gid"; there is no way to assign video to card*
	// and render to render* devices.
	name := lxdbackend.DeviceName(s.Profile.Sdk, gpu.Name)
	s.devices[name] = map[string]string{
		"type":    "gpu",
		"gputype": "physical",
		"uid":     workshop.User.Uid,
		"gid":     workshop.User.Gid,
	}
	return nil
}

func (s *Specification) SetCamera(camera workshop.Camera) error {
	s.Profile.Camera = &camera

	name := lxdbackend.DeviceName(s.Profile.Sdk, camera.Name)
	s.devices[name] = map[string]string{"type": "none"}
	buf, err := json.Marshal(camera)
	if err != nil {
		return err
	}
	s.config[lxdbackend.DeviceConfigKey(name)] = string(buf)
	s.config[lxdbackend.DeviceTypeConfigKey(name)] = "camera"

	for _, subsystem := range []string{"video4linux", "media"} {
		name := lxdbackend.DeviceName(s.Profile.Sdk, camera.Name, subsystem)
		s.devices[name] = map[string]string{
			"type":              "unix-hotplug",
			"subsystem":         subsystem,
			"required":          "false",
			"ownership.inherit": "true",
		}
		s.config[lxdbackend.DeviceTypeConfigKey(name)] = "camera"
	}

	return nil
}

func (s *Specification) addProxyEntry(entry *workshop.ProxyEntry, configKey string) {
	name := lxdbackend.DeviceName(s.Profile.Sdk, entry.Name)
	s.config[lxdbackend.DeviceTypeConfigKey(name)] = configKey
	device := map[string]string{
		"type":    "proxy",
		"connect": entry.Connect.Protocol + ":" + entry.Connect.Address,
		"listen":  entry.Listen.Protocol + ":" + entry.Listen.Address,
	}
	switch entry.Direction {
	case workshop.WorkshopToHost:
		device["bind"] = "instance"
		if entry.Listen.Protocol == "unix" {
			device["uid"] = workshop.User.Uid
			device["gid"] = workshop.User.Gid
		}
	case workshop.HostToWorkshop:
		device["bind"] = "host"
		if entry.Listen.Protocol == "unix" {
			device["uid"] = s.User.Uid
			device["gid"] = s.User.Gid
		}
	}
	s.devices[name] = device
}
