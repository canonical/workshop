package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/client"
)

type CmdTasks struct {
	root *CmdRoot
}

func (c *CmdTasks) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "tasks [<CHANGE ID>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List tasks for a specific change",
		Long: `
Any substantial operation on a workshop is a change that consists of tasks;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- ID:      Uniquely identifies the task within the change
- Status:  Reflects the task's progress and affects the change's status
- Spawn:   Tells when the task was started
- Ready:   Tells when the task was finished
- Summary: Lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use 'workshop changes' instead
`,
		Example: `
List the tasks under change ID 42:
$ workshop tasks 42

List the tasks under the most recent change to the project:
$ workshop tasks`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	return cmd
}

const line = "......................................................................"

func (c *CmdTasks) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	var change *client.Change
	if len(av) > 0 {
		change, err = cli.Change(av[0], false)
		if err != nil {
			return err
		}
	} else {
		changesCmd := CmdChanges{
			root: c.root,
		}
		changes, err := changesCmd.changes(cli)
		if err != nil {
			return err
		}

		if len(changes) == 0 {
			return errors.New("cannot find any changes")
		}
		change = changes[len(changes)-1]
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
			fmt.Fprintf(w, "Status\tDuration\tSummary\n")

			for _, tsk := range tasks {
				duration := tsk.DoingTime.Round(time.Millisecond).String()
				if tsk.DoingTime == 0 {
					duration = "-"
				}

				fmt.Fprintln(w, strings.Join([]string{
					tsk.Status,
					duration,
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

func (c *CmdTasks) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cli, err := c.root.client()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	changesCmd := CmdChanges{
		root: c.root,
	}
	changes, err := changesCmd.changes(cli)
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	l := len(changes)
	num := min(l, 10)
	completions := make([]string, l)

	for _, chg := range changes[l-num : l] {
		completions = append(completions, fmt.Sprintf("%s\t%-5s %s\n", chg.ID, chg.Status, chg.Summary))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
