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

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"
)

type CmdRestore struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdRestore) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "restore <WORKSHOP>...",
		Short:   "Restore workshops to the state of the last launch or refresh",
		GroupID: GrpCRUD,
		Long: `
This command restores the container filesystem of the workshops listed
as arguments to the point of the last launch or refresh,
then resets the interface connections to default settings:

- Connections added at runtime with "workshop connect" are dropped,
  and the workshop returns to its definition's auto-connect defaults.

- A connection removed with "workshop disconnect" is re-established,
  whether or not "--forget" was used.

- A mount source set with "workshop remount" is reset
  to its default location.

Notes:

- The workshop must be "Ready" to be restored.

- Multiple workshops can be restored in a single command invocation;
  the operation is transactional, so if any workshop fails to restore,
  all are reverted.

- To update an existing workshop instead of reverting changes,
  use "workshop refresh".
`,
		Example: `
Restore the "nimble" and "jazzy" workshops in the current project directory:
$ workshop restore nimble jazzy

The name is optional if the project has only one workshop:
$ workshop restore`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopNames([]string{"Ready"}),
	}

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish.")
	cmd.PersistentFlags().BoolVar(&c.verbose, "verbose",
		false,
		"Combine stdout and stderr output from hooks.")

	return cmd
}

func (c *CmdRestore) Run(cmd *cobra.Command, av []string) error {
	av = strutil.Deduplicate(av)

	cli, err := c.root.client()
	if err != nil {
		return err
	}

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

	changeId, err := cli.Restore(project.Id, av, c.verbose)
	if err != nil {
		return err
	}

	_, err = c.wait(cli, changeId)

	switch {
	case err == nil:
		fmt.Fprintf(Stdout, "%s restored\n", strutil.Quoted(av))
	case errors.Is(err, errNoWait):
	case errors.Is(err, errUndone):
		fmt.Fprintf(Stdout, "%s restore aborted\n", strutil.Quoted(av))
	default:
		return fmt.Errorf("%v\n%s restore aborted", err, strutil.Quoted(av))
	}

	return nil
}
