package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/timeutil"
)

type CmdTasks struct {
	root *CmdRoot
}

func (c *CmdTasks) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "tasks <CHANGE ID>",
		Args:  cobra.RangeArgs(1, 1),
		Short: "List tasks for a specific change",
		Long: `
Any substantial operation on a workshop is a *change* that consists of *tasks*;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- ID:      uniquely identifies the task within the change
- Status:  reflects the task's progress and affects the change's status
- Spawn:   tells when the task was started
- Ready:   tells when the task was finished
- Summary: lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use **workshop changes** instead
`,
		Example: `
# List the tasks under change ID 42
workshop tasks 42`,
		RunE: c.Run,
	}

	return cmd
}

const line = "......................................................................"

func (c *CmdTasks) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	change, err := cli.Change(av[0])
	if err != nil {
		return err
	}

	if change != nil {
		tasks := change.Tasks

		slices.SortFunc(tasks, func(a, b *client.Task) int {
			if a.SpawnTime.Before(b.SpawnTime) {
				return -1
			} else if a.SpawnTime.After(b.SpawnTime) {
				return 1
			}
			return 0
		})

		if len(tasks) > 0 {
			w := tabWriter()
			fmt.Fprintf(w, "ID\tStatus\tSpawn\tReady\tSummary\n")

			for _, tsk := range tasks {
				spawnTime := timeutil.Human(tsk.SpawnTime)
				readyTime := timeutil.Human(tsk.ReadyTime)
				if tsk.ReadyTime.IsZero() {
					readyTime = "-"
				}

				fmt.Fprintln(w, strings.Join([]string{
					tsk.ID,
					tsk.Status,
					spawnTime,
					readyTime,
					tsk.Summary}, "\t"))
			}
			w.Flush()

			for _, tsk := range tasks {
				if len(tsk.Log) == 0 {
					continue
				}
				fmt.Fprintln(os.Stdout)
				fmt.Fprintln(os.Stdout, line)
				fmt.Fprintln(os.Stdout, tsk.Summary)
				fmt.Fprintln(os.Stdout)
				for _, line := range tsk.Log {
					fmt.Fprintln(os.Stdout, line)
				}
			}
		}
	} else {
		return fmt.Errorf("change with id %q not found", av[0])
	}

	return nil
}
