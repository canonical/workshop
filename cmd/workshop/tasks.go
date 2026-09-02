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
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
)

type CmdTasks struct {
	cmdutil.ColorMixin
	root      *CmdRoot
	noHeaders bool
}

func (c *CmdTasks) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "tasks [<CHANGE ID>]",
		Args:    cobra.MaximumNArgs(1),
		Short:   "List tasks for a specific change",
		GroupID: GrpChanges,
		Long: `
Any substantial operation on a workshop is a change that consists of tasks;
the command lists individual tasks that comprise a specific change.
For each task, it prints the following details:

- Status:    Reflects the task's progress and affects the status of the change
- Duration:  Tells how long the task has been running
- Summary:   Lists actions, affected SDKs and workshops, other information


Notes:

- The command may print additional log details for tasks that store them

- To investigate recent changes in a project, use "workshop changes" instead
`,
		Example: `
List the tasks under change ID 42:
$ workshop tasks 42

List the tasks under the most recent change to the project:
$ workshop tasks`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.PersistentFlags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

const hrule = "......................................................................"

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
		if change == nil {
			return fmt.Errorf("change with id %q not found", av[0])
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
		for _, recent := range slices.Backward(changes) {
			if len(recent.Tasks) > 0 {
				change = recent
				break
			}
		}
		if change == nil {
			return errors.New("cannot find any nonempty changes")
		}
	}

	tasks := change.Tasks
	if len(tasks) == 0 {
		return nil
	}

	slices.SortFunc(tasks, func(a, b *client.Task) int {
		return a.SpawnTime.Compare(b.SpawnTime)
	})

	var maxDur int
	if !c.noHeaders {
		maxDur = len("DURATION")
	}
	for _, tsk := range tasks {
		maxDur = max(maxDur, len(tsk.DoingTime.Round(time.Millisecond).String()))
	}
	w := tabWriter()
	if !c.noHeaders {
		esc := c.GetEscapes()
		fmt.Fprintf(w, "%sSTATUS\t%*s\tSUMMARY%s\n", esc.Bold, maxDur, "DURATION", esc.End)
	}
	for _, tsk := range tasks {
		duration := tsk.DoingTime.Round(time.Millisecond).String()
		if tsk.DoingTime == 0 {
			duration = "-"
		}

		fmt.Fprintf(w, "%s\t%*s\t%s\n",
			tsk.Status,
			maxDur,
			duration,
			tsk.Summary)
	}
	w.Flush()

	for _, tsk := range tasks {
		if len(tsk.Log) == 0 {
			continue
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, hrule)
		fmt.Fprintln(os.Stdout, tsk.Summary)
		fmt.Fprintln(os.Stdout)
		for _, line := range tsk.Log {
			fmt.Fprintln(os.Stdout, line)
		}
	}

	return nil
}

func (c *CmdTasks) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cli, err := c.root.noRetryClient()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	changesCmd := CmdChanges{
		root: c.root,
	}
	changes, err := changesCmd.changes(cli)
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	l := len(changes)
	num := min(l, 10)
	completions := make([]string, 0, num)

	for _, chg := range changes[l-num : l] {
		description := fmt.Sprintf("%-5s %s", chg.Status, chg.Summary)
		completions = append(completions, cobra.CompletionWithDesc(chg.ID, description))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
