package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/timeutil"
)

type CmdChanges struct {
	root *CmdRoot
}

func (c *CmdChanges) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "changes",
		Args:  cobra.NoArgs,
		Short: "List recent changes to the workshops in a project",
		Long: `
Any substantial operation on a workshop is a *change* that consists of *tasks*;
the command lists details of recent changes for all workshops within a project.
For each change, it prints the following details:

- ID:      uniquely identifies the change within the project

- Status:  reflects the change's progress and affects the workshop's status

- Spawn:   tells when the change was started

- Ready:   tells when the change was *successfully* finished, if at all

- Summary: lists actions, affected workshops, other information


Notes:

- Only successful changes display values in the *Ready* column

- To investigate the details of a specific change, use 'workshop tasks' instead
`,
		Example: `
# List changes for all workshops in the current project directory
workshop changes`,

		RunE: c.Run,
	}

	return cmd
}

func (c *CmdChanges) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	clientOpts := client.ChangesOptions{
		ProjectPath: c.root.project,
		Selector:    client.ChangesAll,
	}

	chngs, err := cli.Changes(&clientOpts)
	if err != nil {
		return err
	}

	slices.SortFunc(chngs, func(a, b *client.Change) int {
		if a.SpawnTime.Before(b.SpawnTime) {
			return -1
		} else if a.SpawnTime.After(b.SpawnTime) {
			return 1
		}
		return 0
	})

	if len(chngs) > 0 {
		w := tabWriter()
		fmt.Fprintf(w, "ID\tStatus\tSpawn\tReady\tSummary\n")

		for _, chg := range chngs {
			spawnTime := timeutil.Human(chg.SpawnTime)
			readyTime := timeutil.Human(chg.ReadyTime)
			if chg.ReadyTime.IsZero() {
				readyTime = "-"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				chg.ID,
				chg.Status,
				spawnTime,
				readyTime,
				chg.Summary)
		}
		w.Flush()
	}

	return nil
}
