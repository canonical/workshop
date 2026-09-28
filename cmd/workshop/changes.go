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
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
	"github.com/canonical/workshop/internal/timeutil"
)

type CmdChanges struct {
	cmdutil.ColorMixin
	root      *CmdRoot
	noHeaders bool
}

func (c *CmdChanges) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "changes",
		Args:    cobra.ExactArgs(0),
		Short:   "List recent changes to the workshops in a project",
		GroupID: GrpChanges,
		Long: `
Any substantial operation on a workshop is a change that consists of tasks;
the command lists details of recent changes for all workshops within a project.
For each change, it prints the following details:

- ID:      Uniquely identifies the change within the project

- Status:  Reflects the change's progress and affects the workshop's status

- Spawn:   Tells when the change was started

- Ready:   Tells when the change was successfully finished, if at all

- Summary: Lists actions, affected workshops, other information


Notes:

- Only successful changes display values in the "Ready" column

- To investigate the details of a specific change, use "workshop tasks" instead
`,
		Example: `
List changes for all workshops in the current project directory:
$ workshop changes`,
		Annotations: map[string]string{
			"related": "workshop info,workshop list",
		},

		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

func (c *CmdChanges) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	chngs, err := c.changes(cli)
	if err != nil {
		return err
	}

	if len(chngs) == 0 {
		return nil
	}

	w := tabWriter()
	if !c.noHeaders {
		esc := c.GetEscapes()
		fmt.Fprintf(w, "%sID\tSTATUS\tSPAWN\tREADY\tSUMMARY%s\n", esc.Bold, esc.End)
	}
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

	return nil
}

func (c *CmdChanges) changes(cli *client.Client) ([]*client.Change, error) {
	clientOpts := client.ChangesOptions{
		ProjectPath: c.root.project(),
		Selector:    client.ChangesAll,
	}

	chngs, err := cli.Changes(&clientOpts)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(chngs, func(a, b *client.Change) int {
		if a.SpawnTime.Before(b.SpawnTime) {
			return -1
		} else if a.SpawnTime.After(b.SpawnTime) {
			return 1
		}
		return 0
	})
	return chngs, nil
}
