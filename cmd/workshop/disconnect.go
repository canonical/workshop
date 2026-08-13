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

package main

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
)

type CmdDisconnect struct {
	waitMixin
	root   *CmdRoot
	forget bool
}

func (c *CmdDisconnect) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "disconnect <WORKSHOP>/<SDK>:<PLUG OR SLOT> [<WORKSHOP>/<SDK>]:[<SLOT>]",
		Args:    cobra.RangeArgs(1, 2),
		Short:   "Disconnect a plug or a slot",
		GroupID: GrpConnect,
		Long: `
This command disconnects a plug from its slot, or a slot from all its plugs.

- A single argument can be a fully qualified plug or slot reference;
  with two arguments, the first one is the plug, and the second one is the slot.

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument.

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.


  Notes:

- After an auto-connected plug is thus disconnected,
  it is reconnected during "workshop refresh"
  only if the "--forget" option was used with "workshop disconnect".
  Either way, "workshop restore" reconnects it.
`,
		Example: `
Disconnect the "mod-cache" mount interface plug of the "go" SDK
under the "nimble" workshop in the current project directory:
$ workshop disconnect nimble/go:mod-cache

A full version of the same command
that lists the target SDK ("system") and slot ("mount"):
$ workshop disconnect nimble/go:mod-cache nimble/system:mount

Disconnect all plugs connected to the "mount" slot of the "system" SDK
under the "nimble" workshop in the current project directory:
$ workshop disconnect nimble/system:mount`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.PersistentFlags().BoolVar(&c.forget, "forget",
		false,
		`Reconnect the plugs at "workshop refresh" if auto-connected initially.`)

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish.")

	return cmd
}

func (c *CmdDisconnect) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	plugRef, err := client.ParseShortPlugRef(av[0])
	if err != nil {
		return err
	}
	plugRef.ProjectId = project.Id

	slotRef := &client.SlotRef{}
	if len(av) > 1 {
		// check if the second arg is a short version of the host-provided slot reference
		if strings.HasPrefix(av[1], ":") {
			slotRef.Workshop = plugRef.Workshop
			slotRef.Sdk = "system"
			slotRef.Name = av[1][1:]
		} else {
			slotRef, err = client.ParseShortSlotRef(av[1])
			if err != nil {
				return err
			}
		}
		slotRef.ProjectId = plugRef.ProjectId
	}

	var opts = client.DisconnectOptions{Forget: c.forget}
	changeId, err := cli.Disconnect(plugRef.ProjectId, plugRef.Workshop, plugRef.Sdk, plugRef.Name,
		slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name, &opts)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	return nil
}

func (c *CmdDisconnect) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cli, err := c.root.noRetryClient()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	connections, err := cli.Connections(&client.ConnectionOptions{ProjectId: project.Id, All: true})
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Create map of endpoint strings and associated plugs
	connectedPlugs := make(map[string]client.Plug)
	for _, plug := range connections.Plugs {
		if len(plug.Connections) > 0 {
			ref := endpoint(plug.Workshop, plug.Sdk, plug.Name)
			connectedPlugs[ref] = plug
		}
	}

	var completion []string
	switch len(args) {
	case 0:
		// No arguments, show list of plugs
		completion = slices.Collect(maps.Keys(connectedPlugs))
	case 1:
		if plug, ok := connectedPlugs[args[0]]; ok {
			for _, slot := range plug.Connections {
				completion = append(completion, endpoint(slot.Workshop, slot.Sdk, slot.Name))
			}
		}
	}
	return completion, cobra.ShellCompDirectiveNoFileComp
}
