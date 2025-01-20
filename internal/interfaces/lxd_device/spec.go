package lxd_device

import (
	"encoding/json"
	"fmt"
	"os/user"
	"strconv"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

func NewSpecification(user *user.User, sdk string) *Specification {
	return &Specification{
		devices: make(map[string]map[string]string),
		config:  make(map[string]string),
		Profile: workshop.NewSdkProfile(sdk),
		User:    user,
	}
}

type Specification struct {
	Profile workshop.SdkProfile

	devices map[string]map[string]string
	config  map[string]string

	User *user.User
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

	if dev.Type == workshop.WorkshopWorkshop {
		s.devices[dev.Name] = map[string]string{"type": "none"}
		buf, err := json.Marshal(dev)
		if err != nil {
			return err
		}
		s.config[lxdbackend.DeviceConfigKey(s.Profile.Sdk, dev.Name)] = string(buf)
		s.config[lxdbackend.DeviceTypeConfigKey(s.Profile.Sdk, dev.Name)] = "mount"
	}

	if dev.Type == workshop.HostWorkshop {
		s.devices[dev.Name] = map[string]string{"type": "disk", "source": dev.What,
			"path": dev.Where, "readonly": strconv.FormatBool(dev.ReadOnly)}
	}

	return nil
}

// Ssh Agent and Desktop are both of the lxc 'proxy' type
// These are network protocol proxy devices that open a port on the host or in a workhop.
// 'from', 'to' are the source and destination addresses (paths in the case of unix sockets),
// see https://documentation.ubuntu.com/lxd/en/latest/reference/devices_proxy/#device-proxy-device-conf:bind
// bind denotes where the port is open (can be: instance, host)

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
	s.devices[gpu.Name] = map[string]string{"type": "gpu", "gputype": "physical", "uid": "1000", "gid": "1000"}

	return nil
}

func (s *Specification) SetCamera(camera workshop.Camera) error {
	s.Profile.Camera = &camera

	s.devices[camera.Name] = map[string]string{"type": "none"}
	buf, err := json.Marshal(camera)
	if err != nil {
		return err
	}
	s.config[lxdbackend.DeviceConfigKey(s.Profile.Sdk, camera.Name)] = string(buf)
	s.config[lxdbackend.DeviceTypeConfigKey(s.Profile.Sdk, camera.Name)] = "camera"

	for i := 0; i < 10; i++ {
		// This name is unique because '/' is not permitted in plug names.
		name := fmt.Sprintf("%s/video%d", camera.Name, i)
		path := fmt.Sprintf("/dev/video%d", i)
		// The default workshop user must be able to acces the video devices.
		// Workshop assigns the devices to workshop.workshop. A more
		// traditional way here would be to add them device to the video
		// groups, but it requires an additional workshop exec to find out the
		// groups' ids at the LXD profile generation time. Given that we are
		// solving the problem of access in a confined environment and workshop
		// is a passwordless sudo user anyway, it was decided that it is OK if
		// the workshop user owns video devices.
		s.devices[name] = map[string]string{
			"type":     "unix-char",
			"source":   path,
			"path":     path,
			"required": "false",
			"uid":      "1000",
			"gid":      "1000",
		}
		s.config[lxdbackend.DeviceTypeConfigKey(s.Profile.Sdk, name)] = "camera"
	}

	return nil
}

func (s *Specification) addProxyEntry(entry *workshop.ProxyEntry, configKey string) {
	s.config[lxdbackend.DeviceTypeConfigKey(s.Profile.Sdk, entry.Name)] = configKey
	s.devices[entry.Name] = map[string]string{
		"type":    "proxy",
		"connect": "unix:" + entry.Connect,
		"listen":  "unix:" + entry.Listen,
		"uid":     "1000",
		"gid":     "1000",
		"bind":    "instance",
	}
}
