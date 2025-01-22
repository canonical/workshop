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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/systemd"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/x11"
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

func (iface *desktopInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	env, err := systemd.UserEnvironment(spec.User)
	if err != nil {
		return err
	}

	xdg := env["XDG_RUNTIME_DIR"]
	if xdg == "" {
		return fmt.Errorf("XDG_RUNTIME_DIR is either empty or unset for user %q", spec.User.Username)
	}

	desktop := workshop.Desktop{}
	var wsEnv []string

	// These variables will be inherited from the host
	wsEnv = append(wsEnv, "XDG_BACKEND")
	wsEnv = append(wsEnv, "XDG_SESSION_TYPE")
	wsEnv = append(wsEnv, "QT_QPA_PLATFORM")
	wsEnv = append(wsEnv, "ELECTRON_OZONE_PLATFORM_HINT")
	wsEnv = append(wsEnv, "WAYLAND_DISPLAY")
	wsEnv = append(wsEnv, "DISPLAY")

	wayland := env["WAYLAND_DISPLAY"]
	display := env["DISPLAY"]

	if wayland == "" && display == "" {
		return fmt.Errorf("neither DISPLAY nor WAYLAND_DISPLAY are set for user %q", spec.User.Username)
	}

	if wayland != "" {
		// Setup profile entries
		desktop.Wayland = &workshop.ProxyEntry{}
		desktop.Wayland.Name = plug.Sdk().Name + "-" + "wayland"
		desktop.Wayland.Connect = filepath.Join(xdg, wayland)
		desktop.Wayland.Listen = filepath.Join("/run/user/1000/", wayland)
	}

	// We pass through the X11 socket regardless of whether XAUTHORITY is present
	// on the host. This then gives users the option to modify their xhost
	// settings to allow connections from the container and container user.
	if display != "" {
		// Setup profile entries
		desktop.X11 = &workshop.ProxyEntry{}
		desktop.X11.Name = plug.Sdk().Name + "-" + "x11"
		desktop.X11.Connect = filepath.Join("/tmp/.X11-unix", "X"+strings.TrimPrefix(display, ":"))
		desktop.X11.Listen = desktop.X11.Connect
	}

	// We mount the Xauthority cookie inside a parent folder to ensure that it's
	// updated when the host cookie changes (ie. reboot).
	// https://discuss.linuxcontainers.org/t/mount-single-file/17975
	workshopdXauth := filepath.Join(dirs.WorkshopdRunDir, spec.User.Uid, "Xauthority")
	xauth := env["XAUTHORITY"]
	if xauth != "" {
		m := workshop.Mount{}
		m.Name = plug.Sdk().Name + "-" + "xauth"
		m.Type = 0
		m.What = workshopdXauth
		m.Where = filepath.Join(dirs.WorkshopRunDir, "Xauthority")
		spec.AddMountEntry(m)
	}

	// The .Xauthority cookie contains a 128bit key used to authenticate
	// consumers of the X11 socket. It is generated on each boot with a random
	// suffix, because of this we need to ensure there exists a
	// consistently-named copy of the cookie for the LXC profile. There are two
	// cases where we need to copy the cookie, one is on workshopd startup as we
	// iterate through the list of projects, the other is on connect because
	// this could be the first workshop launched, in which case the user would
	// not have had a project. We handle it here for the connect, presence of
	// the copied cookie after reboot is the responsibility of the interface
	// manager.
	if xauth != "" {
		wsEnv = append(wsEnv, "XAUTHORITY=/tmp/.Xauthority")
		if err := x11.MigrateXauthority(spec.User, xauth); err != nil {
			logger.Noticef("cannot migrate Xauthority file for user %s, X11 applications may not work: %v", spec.User.Username, err)
		}
	}

	return spec.SetDesktop(desktop, wsEnv)
}

func init() {
	registerIface(&desktopInterface{})
}
