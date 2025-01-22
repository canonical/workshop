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
	"fmt"
	"path/filepath"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/systemd"
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

func (iface *sshAgentInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	env, err := systemd.UserEnvironment(spec.User)
	if err != nil {
		return err
	}

	sock := env["SSH_AUTH_SOCK"]
	if sock == "" {
		return fmt.Errorf(`cannot access ssh-agent for user %q: environment variable "SSH_AUTH_SOCK" not found`, spec.User.Username)
	}

	name := plug.Sdk().Name + "-" + plug.Name()

	fromSocket := sock
	toSocket := filepath.Join(dirs.WorkshopRunDir, name+".ssh")

	return spec.SetSshAgent(workshop.SshAgent{ProxyEntry: workshop.ProxyEntry{Name: name, Connect: fromSocket, Listen: toSocket}}, []string{"SSH_AUTH_SOCK=" + toSocket})
}

func init() {
	registerIface(&sshAgentInterface{})
}
