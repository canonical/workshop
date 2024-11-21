package main

import (
	"fmt"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"
)

type CmdLaunch struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdLaunch) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "launch <WORKSHOP>...",
		Args:  cobra.MinimumNArgs(1),
		Short: "Construct one or many workshops using their definitions",
		Long: `
This command constructs the workshops listed as arguments by going over their
definitions and installing their components. For each workshop, it:

- Checks the workshop definition and identifies necessary actions

- Retrieves the required components, such as base and SDKs

- Runs SDK setup hooks to initialise the working state

- On success, ties the workshop to the project and starts it


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are constructed.


Notes:

- Names listed as arguments must match respective 'name:' values in definitions

- To update an existing workshop, use 'workshop refresh' instead

- SDKs are installed in alphabetical order
`,

		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish")

	return cmd
}

func (c *CmdLaunch) Run(cmd *cobra.Command, av []string) error {
	av = strutil.Deduplicate(av)

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	changeId, err := cli.Launch(project.Id, av)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId, false); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	for _, i := range av {
		fmt.Fprintf(Stdout, "%q launched\n", i)
	}

	return nil
}
