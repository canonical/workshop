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
	"fmt"
	"path/filepath"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const sshAgentSummary = `allows sharing system's ssh-agent socket with SDKs`

const sshAgentBaseDeclarationSlots = `
  ssh-agent:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

const sshAgentDeclarationPlugs = `
  ssh-agent:
    allow-installation:
      plug-sdk-type:
        - regular
      plug-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

type sshAgentInterface struct{}

func (iface *sshAgentInterface) Name() string {
	return "ssh-agent"
}

func (iface *sshAgentInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              sshAgentSummary,
		BaseDeclarationPlugs: sshAgentDeclarationPlugs,
		BaseDeclarationSlots: sshAgentBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *sshAgentInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	return true
}

func (iface *sshAgentInterface) LxdDeviceSupportsPlug(instance *lxd_device.Instance, plug *sdk.PlugInfo) error {
	if instance.Confinement != workshop.ConfinementContainer {
		return errors.New("ssh-agent interface only available to containers")
	}
	return nil
}

func (iface *sshAgentInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	sock := spec.Environment["SSH_AUTH_SOCK"]
	if sock == "" {
		return fmt.Errorf(`cannot access ssh-agent for user %q: environment variable "SSH_AUTH_SOCK" not found`, spec.User.Username)
	}

	fromSocket := sock
	toSocket := filepath.Join(dirs.WorkshopRunDir, "ssh-agent.sock")

	return spec.SetSshAgent(workshop.SshAgent{
		ProxyEntry: workshop.ProxyEntry{
			Name: plug.Name(),
			Connect: workshop.ProxyTarget{
				Address:  fromSocket,
				Protocol: "unix",
			},
			Listen: workshop.ProxyTarget{
				Address:  toSocket,
				Protocol: "unix",
			},
			Direction: workshop.WorkshopToHost,
		},
	})
}

func init() {
	registerIface(&sshAgentInterface{})
}
