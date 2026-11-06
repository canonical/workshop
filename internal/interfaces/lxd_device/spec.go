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

package lxd_device

import (
	"context"
	"encoding/json"
	"fmt"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

// lxdServerInfo retrieves GPU resources from LXD.
var lxdServerInfo = func(ctx context.Context) (*api.Resources, error) {
	conn, err := lxdbackend.ConnectLxd(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	resources, err := conn.GetServerResources()
	if err != nil {
		return nil, err
	}

	return resources, nil
}

// MockLxdServerInfo replaces the LXD server info function used by
// NewSpecification and returns a restore function.
func MockLxdServerInfo(f func(ctx context.Context) (*api.Resources, error)) func() {
	old := lxdServerInfo
	lxdServerInfo = f
	return func() { lxdServerInfo = old }
}

func NewSpecification(user string, sdk string) (*Specification, error) {
	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, workshop.ContextUser, user)

	resources, err := lxdServerInfo(ctx)
	if err != nil {
		return nil, err
	}

	return &Specification{
		devices:     make(map[string]map[string]string),
		config:      make(map[string]string),
		Profile:     workshop.NewSdkProfile(sdk),
		User:        usr,
		Environment: env,
		resources:   resources,
	}, nil
}

type Specification struct {
	Profile workshop.SdkProfile

	devices   map[string]map[string]string
	config    map[string]string
	resources *api.Resources

	User        *user.User
	Environment map[string]string
}

// AddPermanentSlot records side-effects of having a slot.
func (s *Specification) AddPermanentSlot(iface interfaces.Interface, slot *sdk.SlotInfo) error {
	if iface, ok := iface.(PermanentSlotMounter); ok {
		return iface.MountPermanentSlot(s, slot)
	}
	return nil
}

// AddPermanentPlug records side-effects of having a plug.
func (s *Specification) AddPermanentPlug(iface interfaces.Interface, plug *sdk.PlugInfo) error {
	if iface, ok := iface.(PermanentPlugMounter); ok {
		return iface.MountPermanentPlug(s, plug)
	}
	return nil
}

// AddConnectedSlot records side-effects of having a connected slot.
func (s *Specification) AddConnectedSlot(iface interfaces.Interface, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	if iface, ok := iface.(ConnectedSlotMounter); ok {
		return iface.MountConnectedSlot(s, plug, slot)
	}
	return nil
}

// AddConnectedPlug records side-effects of having a connected plug.
func (s *Specification) AddConnectedPlug(iface interfaces.Interface, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	if iface, ok := iface.(ConnectedPlugMounter); ok {
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
// see https://canonical.com/lxd/docs/latest/reference/devices_proxy/#device-proxy-device-conf:bind
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

func detectGpuVendor(vendorID string) string {
	switch vendorID {
	case "8086":
		return "intel"
	case "10de":
		return "nvidia"
	case "1002":
		return "amd"
	default:
		return "unknown"
	}
}

func (s *Specification) SetGpu(gpu workshop.Gpu) error {
	s.Profile.Gpu = &gpu

	gpus := s.resources.GPU
	if gpus.Total == 0 {
		logger.Debugf("GPU interface requested, but no GPU detected on the host system")
		return nil
	}

	for _, c := range gpus.Cards {
		vendor := detectGpuVendor(c.VendorID)
		switch vendor {
		case "nvidia", "amd":
			device := lxdbackend.DeviceName(s.Profile.Sdk, gpu.Name, vendor)
			// Add "all" devices per vendor. Indexes would be rather accurate but
			// the problem with indexes is that the AMD CDI start indexes from 0
			// regardless what index the card has in the system whilst NVIDIA CDI
			// indexes match the system indexes. LXD should include UUIDs
			// in the GPU resources which we can use in the spec instead of IDs.
			if _, ok := s.devices[device]; !ok {
				s.devices[device] = map[string]string{
					"type":    "gpu",
					"gputype": "physical",
					"id":      vendor + ".com/gpu=all",
				}
			}
		case "intel":
			// Intel GPUs are not yet supported by the GPU CDI spec, so we
			// fall back to exposing them as 'physical' GPUs with a specific
			// ID.
			if c.DRM == nil {
				logger.Debugf("Intel GPU detected without DRM info")
				continue
			}
			cardId := strconv.FormatUint(c.DRM.ID, 10)
			device := lxdbackend.DeviceName(s.Profile.Sdk, gpu.Name, vendor, cardId)
			s.devices[device] = map[string]string{
				"type":    "gpu",
				"gputype": "physical",
				"uid":     workshop.User.Uid,
				"gid":     workshop.User.Gid,
				"id":      cardId,
			}
		default:
			if c.DRM == nil {
				logger.Debugf("Unknown GPU vendor ID '%s' for GPU card without DRM info", c.VendorID)
				continue
			}
			logger.Debugf("Unknown GPU vendor ID '%s' for GPU card with ID %d", c.VendorID, c.DRM.ID)
		}
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

// virtualizationDevices are the host character devices exposed to the workshop
// by the virtualization interface. They enable hardware-accelerated virtual
// machines (KVM) together with vhost/vsock acceleration. /dev/net/tun is not
// listed here because LXD already provides it inside the container.
var virtualizationDevices = []string{
	"/dev/kvm",
	"/dev/vhost-net",
	"/dev/vhost-vsock",
	"/dev/vsock",
}

// SetVirtualization exposes the virtualization character devices to the
// workshop as LXD "unix-char" devices. Each device is owned by the workshop
// user (uid/gid) with mode 0660 so that the user can access them directly. The
// devices are marked non-required so that a host missing e.g. the vsock modules
// does not prevent the workshop from starting.
func (s *Specification) SetVirtualization(virt workshop.Virtualization) error {
	s.Profile.Virtualization = &virt

	name := lxdbackend.DeviceName(s.Profile.Sdk, virt.Name)
	s.devices[name] = map[string]string{"type": "none"}
	buf, err := json.Marshal(virt)
	if err != nil {
		return err
	}
	s.config[lxdbackend.DeviceConfigKey(name)] = string(buf)
	s.config[lxdbackend.DeviceTypeConfigKey(name)] = "virtualization"

	for _, dev := range virtualizationDevices {
		name := lxdbackend.DeviceName(s.Profile.Sdk, virt.Name, filepath.Base(dev))
		s.devices[name] = map[string]string{
			"type":     "unix-char",
			"source":   dev,
			"path":     dev,
			"uid":      workshop.User.Uid,
			"gid":      workshop.User.Gid,
			"mode":     "0660",
			"required": "false",
		}
		s.config[lxdbackend.DeviceTypeConfigKey(name)] = "virtualization"
	}

	return nil
}

func (s *Specification) AddCustomDevice(device workshop.CustomDevice) error {
	s.Profile.CustomDevices = append(s.Profile.CustomDevices, device)

	name := lxdbackend.DeviceName(s.Profile.Sdk, device.Name)
	lxdDevice := map[string]string{
		"type":              "unix-hotplug",
		"required":          "false",
		"ownership.inherit": "true",
	}
	// Only set the device identifiers that are provided so that an unset one does
	// not constrain which host devices match.
	if device.Subsystem != "" {
		lxdDevice["subsystem"] = device.Subsystem
	}
	if device.VendorID != "" {
		lxdDevice["vendorid"] = device.VendorID
	}
	if device.ProductID != "" {
		lxdDevice["productid"] = device.ProductID
	}

	s.devices[name] = lxdDevice
	s.config[lxdbackend.DeviceTypeConfigKey(name)] = "custom-device"

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
		if entry.Listen.Protocol == "unix" && !strings.HasPrefix(entry.Listen.Address, "@") {
			device["uid"] = workshop.User.Uid
			device["gid"] = workshop.User.Gid
		}
		device["security.uid"] = s.User.Uid
		device["security.gid"] = s.User.Gid
	case workshop.HostToWorkshop:
		device["bind"] = "host"
		if entry.Listen.Protocol == "unix" && !strings.HasPrefix(entry.Listen.Address, "@") {
			device["uid"] = s.User.Uid
			device["gid"] = s.User.Gid
		}
	}
	s.devices[name] = device
}
