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

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"
)

type CmdRemove struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdRemove) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "remove <WORKSHOP>...",
		Aliases: []string{"rm"},
		Short:   "Remove one or many workshops",
		GroupID: GrpCRUD,
		Long: `
This command removes the workshops listed as arguments. For each workshop, it:

- Checks that the workshop isn't "Off" or "Pending"
- Stops the workshop if it's not already "Stopped"
- Deletes the workshop but preserves its definition

The "workshop rm" command is an alias for "workshop remove".

Notes:

- If any listed workshop is "Off" or "Pending", none are removed.

- To rebuild a removed workshop from scratch, use "workshop launch".

- For mount interface plugs,
  non-default sources set by "workshop remount" aren't removed.
`,
		Example: `
Remove the "nimble" and "jazzy" workshops in the current project directory:
$ workshop remove nimble jazzy

The name is optional if the project has only one workshop:
$ workshop remove`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopNames([]string{"Ready", "Stopped"}),
	}

	return cmd
}

func (c *CmdRemove) Run(cmd *cobra.Command, av []string) error {
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

	changeId, err := cli.Remove(project.Id, av)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	for _, name := range av {
		fmt.Fprintf(Stdout, "%q removed\n", name)
	}

	return nil
}
