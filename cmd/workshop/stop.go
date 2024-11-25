package main

import (
	"fmt"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"
)

type CmdStop struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdStop) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "stop <WORKSHOP>...",
		Args:  cobra.MinimumNArgs(1),
		Short: "Stop one or many workshops",
		Long: `
This command deactivates the workshops listed as arguments. For each one, it:

- Makes sure the workshop was actually started or is already stopped

- Deactivates the workshop and sets it to *Stopped*


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are stopped.


Notes:

- If a workshop wasn't yet started or even launched, an error occurs

- When interrupted, the command attempts to gracefully revert its actions

- To start a stopped workshop, use 'workshop start'
`,
		Example: `
# Stop the nimble and jazzy workshops in the current project directory
workshop stop nimble jazzy`,
		RunE: c.Run,
	}

	return cmd
}

func (c *CmdStop) Run(cmd *cobra.Command, av []string) error {
	av = strutil.Deduplicate(av)

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	c.skipAbort = true

	project, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	changeId, err := cli.Stop(project.Id, av)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId, false); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	for _, name := range av {
		fmt.Fprintf(Stdout, "%q stopped\n", name)
	}
	return nil
}
