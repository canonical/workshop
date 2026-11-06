// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2026 Canonical Ltd
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
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const virtualizationSummary = `allows launching hardware-accelerated virtual machines`

const virtualizationBaseDeclarationSlots = `
  virtualization:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

const virtualizationBaseDeclarationPlugs = `
  virtualization:
    allow-installation:
      plug-sdk-type:
        - regular
      plug-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

type virtualizationInterface struct{}

func (iface *virtualizationInterface) Name() string {
	return "virtualization"
}

func (iface *virtualizationInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              virtualizationSummary,
		BaseDeclarationPlugs: virtualizationBaseDeclarationPlugs,
		BaseDeclarationSlots: virtualizationBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *virtualizationInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

func (iface *virtualizationInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	return spec.SetVirtualization(workshop.Virtualization{Name: plug.Name()})
}

func init() {
	registerIface(&virtualizationInterface{})
}
