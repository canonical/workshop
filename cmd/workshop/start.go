package main

import (
	"fmt"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/internal/progress"
)

type CmdStart struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdStart) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "start <WORKSHOP>...",
		Short: "Start one or many workshops",
		Long: `
This command activates the workshops listed as arguments. For each one, it:

- Makes sure the workshop was actually launched

- Activates the workshop for use and sets it to 'Ready'


If multiple workshops are listed and an error occurs,
the operation is aborted and no workshops are started.


Notes:

- If a workshop is already started or wasn't yet launched, an error occurs.

- When interrupted, the command attempts to gracefully revert its actions.

- To stop a started workshop, use 'workshop stop'.
`,
		Example: `
Start the 'nimble' and 'jazzy' workshops in the current project directory:
$ workshop start nimble jazzy

The name is optional if the project has only one workshop:
$ workshop start`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopNames([]string{"Stopped"}),
	}

	return cmd
}

func (c *CmdStart) Run(cmd *cobra.Command, av []string) error {
	av = strutil.Deduplicate(av)

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	c.skipAbort = true

	project, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	if len(av) == 0 {
		name, err := cli.SingleWorkshopName(project)
		if err != nil {
			return err
		}
		av = []string{name}
	}

	changeId, err := cli.Start(project.Id, av)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId, progress.DisplayModeDefault); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	for _, name := range av {
		fmt.Fprintf(Stdout, "%q started\n", name)
	}

	return nil
}
