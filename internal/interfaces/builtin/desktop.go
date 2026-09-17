// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2024 Canonical Ltd
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

package builtin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const desktopSummary = `allows SDKs to use the host's wayland compositor`

const desktopBaseDeclarationSlots = `
  desktop:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

const desktopDeclarationPlugs = `
  desktop:
    allow-installation:
      plug-sdk-type:
        - regular
      plug-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

type desktopInterface struct{}

func (iface *desktopInterface) Name() string {
	return "desktop"
}

func (iface *desktopInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              desktopSummary,
		BaseDeclarationPlugs: desktopDeclarationPlugs,
		BaseDeclarationSlots: desktopBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *desktopInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	return true
}

func (iface *desktopInterface) LxdDeviceSupportsPlug(instance *lxd_device.Instance, plug *sdk.PlugInfo) error {
	if instance.Confinement != workshop.ConfinementContainer {
		return errors.New("desktop interface only available to containers")
	}
	return nil
}

func (iface *desktopInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	desktop := workshop.Desktop{}

	wayland := spec.Environment["WAYLAND_DISPLAY"]
	display := spec.Environment["DISPLAY"]
	if wayland == "" && display == "" {
		return fmt.Errorf("neither DISPLAY nor WAYLAND_DISPLAY are set for user %q", spec.User.Username)
	}

	if wayland != "" {
		if !filepath.IsAbs(wayland) {
			xdg := spec.Environment["XDG_RUNTIME_DIR"]
			if xdg == "" {
				return fmt.Errorf("XDG_RUNTIME_DIR is either empty or unset for user %q", spec.User.Username)
			}
			wayland = filepath.Join(xdg, wayland)
		}

		desktop.Wayland = &workshop.ProxyEntry{
			Name: fmt.Sprintf("%s_wayland", plug.Name()),
			Connect: workshop.ProxyTarget{
				Address:  wayland,
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  filepath.Join(dirs.XdgRuntimeDirBase, workshop.User.Uid, "wayland-0"),
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		}
	}

	// We pass through the X11 socket regardless of whether XAUTHORITY is present
	// on the host. This then gives users the option to modify their xhost
	// settings to allow connections from the container and container user.
	if display != "" {
		var found bool
		display, found = strings.CutPrefix(display, ":")
		if !found {
			return fmt.Errorf("desktop interface requires local X server")
		}
		display, _, found = strings.Cut(display, ".")
		if found {
			logger.Noticef("desktop interface ignores screen number")
		}

		desktop.X11 = &workshop.ProxyEntry{
			Name: fmt.Sprintf("%s_x11", plug.Name()),
			Connect: workshop.ProxyTarget{
				Address:  filepath.Join("/tmp/.X11-unix", "X"+display),
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  "/tmp/.X11-unix/X0",
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		}
	}

	// We mount the Xauthority inside a parent folder to ensure that the mounted
	// cookie is updated when the host cookie changes (ie. reboot).
	// https://discuss.linuxcontainers.org/t/mount-single-file/17975
	workshopdXauth := filepath.Join(dirs.WorkshopdRunDir, spec.User.Uid, "Xauthority")
	xauth := spec.Environment["XAUTHORITY"]
	if xauth != "" {
		m := workshop.Mount{
			Name:      fmt.Sprintf("%s_xauth", plug.Name()),
			Type:      workshop.HostWorkshop,
			What:      workshopdXauth,
			Where:     filepath.Join(dirs.WorkshopRunDir, "Xauthority"),
			MakeWhere: true,
			Mode:      os.ModePerm &^ workshop.RootUmask,
			ReadOnly:  true,
		}
		spec.AddMountEntry(m)
	}

	return spec.SetDesktop(desktop)
}

func init() {
	registerIface(&desktopInterface{})
}
