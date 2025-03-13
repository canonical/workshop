package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/progress"
)

type CmdConnect struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdConnect) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "connect <WORKSHOP>/<SDK>:<PLUG> [<WORKSHOP>/<SDK>][:<SLOT>]",
		Args:  cobra.RangeArgs(1, 2),
		Short: "Connect a plug to a slot",
		Long: `
This command connects a plug to a target slot
that is specified as the second argument or deduced from the context.

- If the second argument is omitted entirely, the target is assumed to be
  <WORKSHOP>/system:<PLUG>; <WORKSHOP> and <PLUG> come from the first argument.

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument.

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.
  However, if there are several candidate slots that match the interface,
  the command fails.

- If the target slot is compatible with the plug, the command attempts
  to connect them and returns the result.


  Notes:

- To be compatible, the plug and the slot must use the same interface.

- Multiple plugs can be connected to the same slot, but not vice versa.

- The 'workshop connections' output will list the connection as 'manual'.
`,
		Example: `
Connect the 'mod-cache' mount interface plug of the 'go' SDK
under the 'nimble' workshop in the current project directory:
$ workshop connect nimble/go:mod-cache :mount

A full version of the command that also lists the target SDK ('system'):
$ workshop connect nimble/go:mod-cache nimble/system:mount`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish.")

	return cmd
}

func (c *CmdConnect) Run(cmd *cobra.Command, av []string) error {
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
		// check if the second arg is a short version of the system-provided slot reference
		if strings.HasPrefix(av[1], ":") {
			slotRef.Workshop = plugRef.Workshop
			slotRef.Sdk = "system"
			slotRef.Name = av[1][1:]
		} else {
			slotRef, err = client.ParseShortSlotRef(av[1])
			if err != nil {
				// see if an SDK (empty slot) reference provided
				slotRef, err = client.ParseSlotSdkRef(av[1])
				if err != nil {
					return err
				}
			}
		}
		slotRef.ProjectId = plugRef.ProjectId
	} else {
		// workshop connect <workshop>/<sdk>:plug which means that the plug will
		// be attempted to connect to the same name slot in the system SDK (if
		// exists)
		slotRef.ProjectId = plugRef.ProjectId
		slotRef.Workshop = plugRef.Workshop
		slotRef.Sdk = "system"
		slotRef.Name = plugRef.Name
	}

	if plugRef.ProjectId != slotRef.ProjectId {
		return fmt.Errorf("cannot connect plugs and slots across different workshops")
	}

	if plugRef.Workshop != slotRef.Workshop {
		return fmt.Errorf("cannot connect plugs and slots across different workshops")
	}

	changeId, err := cli.Connect(plugRef.ProjectId, plugRef.Workshop, plugRef.Sdk, plugRef.Name,
		slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name, nil)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId, progress.DisplayModeDefault); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	return nil
}

func (c *CmdConnect) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cli, err := c.root.client()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	connections, err := cli.Connections(&client.ConnectionOptions{ProjectId: project.Id, All: true})
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	// Create map of endpoint string and associated plugs
	disconnectedPlugs := make(map[string]client.Plug)
	for _, plug := range connections.Plugs {
		if len(plug.Connections) == 0 {
			plugString := endpoint(plug.Workshop, plug.Sdk, plug.Name)
			disconnectedPlugs[plugString] = plug
		}
	}

	var completions []string
	switch len(args) {
	case 0:
		// No arguments, show list of plugs
		completions = slices.Collect(maps.Keys(disconnectedPlugs))
	case 1:
		plug, ok := disconnectedPlugs[args[0]]
		if !ok {
			break
		}
		for _, slot := range connections.Slots {
			if plug.Interface == slot.Interface && plug.Workshop == slot.Workshop {
				completions = append(completions, endpoint(slot.Workshop, slot.Sdk, slot.Name))
			}
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
