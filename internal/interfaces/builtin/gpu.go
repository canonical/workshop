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

const gpuSummary = `allows sharing system GPUs with SDKs`

const gpuBaseDeclarationSlots = `
  gpu:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    allow-auto-connection: true
`

const gpuBaseDeclarationPlugs = `
  gpu:
    allow-installation:
      plug-sdk-type:
        - regular
      plug-names:
        - $INTERFACE
    allow-connection: true
    allow-auto-connection: true
`

type gpuInterface struct{}

func (iface *gpuInterface) Name() string {
	return "gpu"
}

func (iface *gpuInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              gpuSummary,
		BaseDeclarationPlugs: gpuBaseDeclarationPlugs,
		BaseDeclarationSlots: gpuBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *gpuInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

func (iface *gpuInterface) LxdDeviceSupportsPlug(instance *lxd_device.Instance, plug *sdk.PlugInfo) error {
	if instance.Confinement != workshop.ConfinementContainer {
		return errors.New("gpu interface only available to containers")
	}
	return nil
}

func (iface *gpuInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	// Add the GPU entry here. In case of a nvidia card, the device caps and
	// runtime will be passed through by the initial workshop configuration (see
	// defaultConfig). This is required as adding nvidia.* entries does not take
	// effect unless the workshop is restarted.
	spec.SetGpu(workshop.Gpu{Name: plug.Name()})
	return nil
}

func init() {
	registerIface(&gpuInterface{})
}
