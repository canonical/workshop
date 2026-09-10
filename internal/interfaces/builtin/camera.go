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

package builtin

import (
	"errors"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const cameraSummary = `allows sharing video cameras with SDKs`

const cameraBaseDeclarationSlots = `
  camera:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

const cameraBaseDeclarationPlugs = `
  camera:
    allow-installation:
      plug-sdk-type:
        - regular
      plug-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

type cameraInterface struct{}

func (iface *cameraInterface) Name() string {
	return "camera"
}

func (iface *cameraInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              cameraSummary,
		BaseDeclarationPlugs: cameraBaseDeclarationPlugs,
		BaseDeclarationSlots: cameraBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *cameraInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

func (iface *cameraInterface) LxdDeviceSupportsPlug(instance *lxd_device.Instance, plug *sdk.PlugInfo) error {
	if instance.Confinement != workshop.ConfinementContainer {
		return errors.New("camera interface only available to containers")
	}
	return nil
}

func (iface *cameraInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	return spec.SetCamera(workshop.Camera{Name: plug.Name()})
}

func init() {
	registerIface(&cameraInterface{})
}
