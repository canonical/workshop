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

package lxdbackend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unsafe"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil/sys"
	"github.com/canonical/workshop/internal/workshop"
)

func ProfileName(pid, workshop, sdk string) string {
	return strings.Join([]string{InstanceName(workshop, pid), sdk}, "-")
}

func DeviceName(parts ...string) string {
	return strings.Join(parts, "_")
}

func DeviceConfigKey(name string) string {
	return fmt.Sprintf("user.workshop.%s", name)
}

func DeviceTypeConfigKey(name string) string {
	return fmt.Sprintf("user.workshop.%s.type", name)
}

func Profile(conn lxd.InstanceServer, pid, wp, profile string) (workshop.SdkProfile, error) {
	lxdp, _, err := LxdProfile(conn, pid, wp, profile)
	if err != nil {
		return workshop.SdkProfile{}, err
	}

	return LxdToSdkProfile(profile, lxdp.Devices, lxdp.Config)
}

func LxdProfile(conn lxd.InstanceServer, pid, wp, profile string) (*api.Profile, string, error) {
	name := ProfileName(pid, wp, profile)
	lxdp, etag, err := conn.GetProfile(name)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil, "", workshop.ErrSdkProfileNotFound
		}
		return nil, "", fmt.Errorf("cannot load %q profile (%w)", profile, err)
	}
	return lxdp, etag, nil
}

func LxdToSdkProfile(profile string, devs map[string]map[string]string, config map[string]string) (workshop.SdkProfile, error) {
	var pr = workshop.NewSdkProfile(profile)
	for devname, dev := range devs {
		name := strings.TrimPrefix(devname, DeviceName(profile, ""))

		switch dev["type"] {
		case "disk":
			mount := workshop.Mount{
				Name:  name,
				Type:  workshop.HostWorkshop,
				What:  dev["source"],
				Where: dev["path"],
			}

			var err error
			mount.MakeWhat, err = boolFromString(dev["user.make-source"])
			if err != nil {
				return pr, err
			}

			mount.MakeWhere, err = boolFromString(dev["user.make-path"])
			if err != nil {
				return pr, err
			}

			mode, err := uintFromString(dev["user.path-mode"], unsafe.Sizeof(mount.Mode))
			if err != nil {
				return pr, err
			}
			mount.Mode = os.FileMode(mode)

			owner, err := uintFromString(dev["user.path-owner"], unsafe.Sizeof(mount.Owner))
			if err != nil {
				return pr, err
			}
			mount.Owner = sys.UserID(owner)

			group, err := uintFromString(dev["user.path-group"], unsafe.Sizeof(mount.Group))
			if err != nil {
				return pr, err
			}
			mount.Group = sys.GroupID(group)

			mount.ReadOnly, err = boolFromString(dev["readonly"])
			if err != nil {
				return pr, err
			}

			pr.Mounts[name] = mount
		case "gpu":
			pr.Gpu = &workshop.Gpu{Name: name}
		case "proxy":
			devtype := config[DeviceTypeConfigKey(devname)]
			switch devtype {
			case "tunnel":
				proxyEntry, err := proxyEntryFromLxdDevice(name, dev)
				if err != nil {
					return pr, err
				}
				pr.Tunnels = append(pr.Tunnels, workshop.Tunnel{ProxyEntry: *proxyEntry})
			case "ssh-agent":
				proxyEntry, err := proxyEntryFromLxdDevice(name, dev)
				if err != nil {
					return pr, err
				}
				pr.Agent = &workshop.SshAgent{ProxyEntry: *proxyEntry}
			case "desktop-wayland":
				if pr.Desktop == nil {
					pr.Desktop = &workshop.Desktop{}
				}
				proxyEntry, err := proxyEntryFromLxdDevice(name, dev)
				if err != nil {
					return pr, err
				}
				pr.Desktop.Wayland = proxyEntry
			case "desktop-x11":
				if pr.Desktop == nil {
					pr.Desktop = &workshop.Desktop{}
				}
				proxyEntry, err := proxyEntryFromLxdDevice(name, dev)
				if err != nil {
					return pr, err
				}
				pr.Desktop.X11 = proxyEntry
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type: %q", profile, devtype)
			}
		case "unix-hotplug":
			devtype := config[DeviceTypeConfigKey(devname)]
			switch devtype {
			case "camera":
				// Ignore the real camera devices, config is under type=none.
			case "custom-device":
				pr.CustomDevices = append(pr.CustomDevices, workshop.CustomDevice{
					Name:      name,
					Subsystem: dev["subsystem"],
					VendorID:  dev["vendorid"],
					ProductID: dev["productid"],
				})
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, devtype)
			}
		case "unix-char":
			devtype := config[DeviceTypeConfigKey(devname)]
			switch devtype {
			case "virtualization":
				// Ignore the real virtualization devices, config is under type=none.
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, devtype)
			}
		case "none":
			cfg, exist := config[DeviceConfigKey(devname)]
			if !exist {
				logger.Noticef("On reading %q SDK profile: unknown device %q", profile, devname)
				continue
			}

			devtype := config[DeviceTypeConfigKey(devname)]
			switch devtype {
			case "camera":
				var camera workshop.Camera
				if err := json.Unmarshal([]byte(cfg), &camera); err != nil {
					return pr, err
				}
				pr.Camera = &camera
			case "virtualization":
				var virt workshop.Virtualization
				if err := json.Unmarshal([]byte(cfg), &virt); err != nil {
					return pr, err
				}
				pr.Virtualization = &virt
			case "mount":
				var mnt workshop.Mount
				if err := json.Unmarshal([]byte(cfg), &mnt); err != nil {
					return pr, err
				}
				pr.Mounts[name] = mnt
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, devtype)
			}
		default:
			logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, dev["type"])
		}
	}
	return pr, nil
}

func boolFromString(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

func uintFromString(s string, byteSize uintptr) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 0, int(byteSize*8))
}

// Constructs a ProxyEntry from an LXD device entry
func proxyEntryFromLxdDevice(name string, dev map[string]string) (*workshop.ProxyEntry, error) {
	connect := strings.SplitN(dev["connect"], ":", 2)
	listen := strings.SplitN(dev["listen"], ":", 2)
	if len(connect) != 2 {
		return nil, fmt.Errorf("internal error: cannot deserialise proxy device in lxd profile: connect entry %q invalid", connect)
	}
	if len(listen) != 2 {
		return nil, fmt.Errorf("internal error: cannot deserialise proxy device in lxd profile: listen entry %q invalid", listen)
	}

	var direction workshop.ProxyDirection
	switch dev["bind"] {
	case "instance":
		direction = workshop.WorkshopToHost
	case "host":
		direction = workshop.HostToWorkshop
	default:
		return nil, fmt.Errorf("internal error: cannot deserialise proxy device in lxd profile: bind entry %q invalid", dev["bind"])
	}

	return &workshop.ProxyEntry{
		Name: name,
		Connect: workshop.ProxyTarget{
			Address:  connect[1],
			Protocol: connect[0],
		},
		Listen: workshop.ProxyTarget{
			Address:  listen[1],
			Protocol: listen[0],
		},
		Direction: direction,
	}, nil
}
