package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
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
  <WORKSHOP>/system:<PLUG>; <WORKSHOP> and <PLUG> come from the first argument

- If the second argument only names the slot itself, the target is
  <WORKSHOP>/system:<SLOT>; <WORKSHOP> comes from the first argument

- If the second argument only names the workshop and SDK, the target is
  <WORKSHOP>/<SDK>:<INTERFACE>;
  <INTERFACE> is the interface in the plug's definition.
  However, if there are several candidate slots that match the interface,
  the command fails

- If the target slot is compatible with the plug, the command attempts
  to connect them and returns the result


  Notes:

- To be compatible, the plug and the slot must use the same interface

- Multiple plugs can be connected to the same slot, but not vice versa

- The 'workshop connections' output will list the connection as 'manual'
`,
		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish")

	return cmd
}

func (c *CmdConnect) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project)
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

	if _, err := c.wait(cli, changeId, false); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	return nil
}
