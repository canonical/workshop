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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil"
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
    allow-auto-connection: false
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
	user, err := workshop.LookupUsername(spec.User())
	if err != nil {
		return err
	}

	uid, _, err := osutil.UidGid(user)
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "-E", "-u", spec.User(), "systemctl", "--user", "show-environment")
	// XDG_RUNTIME_DIR may not be set if a command invoked by sudo or
	// systemd-run; set it here to the default location. It is required for the
	// systemctl to work with --user. See:
	// https://unix.stackexchange.com/questions/346841/why-does-sudo-i-not-set-xdg-runtime-dir-for-the-target-user
	defaultXdg := filepath.Join(dirs.XdgRuntimeDirBase, strconv.FormatUint(uint64(uid), 10))
	cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+defaultXdg)
	out, errOut, err := osutil.RunCmd(cmd)
	if err != nil {
		return fmt.Errorf(string(errOut))
	}

	rawEnv := strings.FieldsFunc(string(out), func(r rune) bool { return r == '\n' })
	env, err := osutil.ParseEnvironment(rawEnv)
	if err != nil {
		return err
	}

	sock, ok := env["SSH_AUTH_SOCK"]
	if !ok {
		return fmt.Errorf("user %q does not have SSH_AUTH_SOCK set. ssh-agent is not running?", user.Username)
	}

	name := plug.Sdk().Name + "-" + plug.Name()

	fromSocket := sock
	toSocket := filepath.Join(dirs.WorkshopRunDir, name+".ssh")

	return spec.AddProxyEntry(workshop.Proxy{
		Name:    name,
		Connect: fromSocket,
		Listen:  toSocket,
		Env:     []string{"SSH_AUTH_SOCK=" + strings.TrimPrefix(toSocket, "unix:")}})
}

func init() {
	registerIface(&sshAgentInterface{})
}
