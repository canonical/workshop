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
	"strings"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
)

type CmdRefresh struct {
	waitMixin
	root        *CmdRoot
	WaitOnError bool
	Continue    bool
	Abort       bool
}

func (c *CmdRefresh) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "refresh [--abort|--continue|--wait-on-error] <WORKSHOP>...",
		Short:   "Update workshops according to their definitions",
		GroupID: GrpCRUD,
		Long: `
This command updates the workshops listed as arguments. For each workshop,
it checks the workshop definition and applies any required updates
to the base image, SDKs, and interface connections:

- Connections added at runtime with "workshop connect" are dropped,
  and the workshop returns to its definition's auto-connect defaults.

- A connection removed with "workshop disconnect" without "--forget"
  stays disconnected after refresh.

The "--wait-on-error" option pauses the refresh if an error occurs.
Thus, you can fix the error and resume the operation or abort and revert it.
This option can only be used with a single workshop.
If multiple workshops are listed and an error occurs,
the operation is aborted and reverted for all of them.
Also, if you change the workshop definition while fixing the error,
you must abort the operation and restart from scratch.

Notes:

- The workshop must be "Ready" to be refreshed. Throughout
  the refresh, all affected workshops remain unavailable for other changes.

- Updated and newly added SDKs are installed in the order
  they are listed in the workshop definition.

- To construct a newly defined workshop, use "workshop launch" instead.

`,
		Example: `
Refresh the "nimble" and "jazzy" workshops in the current project directory:
$ workshop refresh nimble jazzy

The name is optional if the project has only one workshop:
$ workshop refresh

Refresh workshop, but pause on any errors (won't accept multiple workshops):
$ workshop refresh --wait-on-error

After refresh paused on error, abort the operation:
$ workshop refresh --abort

After refresh paused on error and the workshop was fixed,
continue the operation:
$ workshop refresh --continue`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopNames([]string{"Ready", "Waiting"}),
	}

	cmd.PersistentFlags().BoolVar(&c.WaitOnError, "wait-on-error",
		false,
		`Pause the operation on error; to resume, use "--continue" or "--abort".`)
	cmd.PersistentFlags().BoolVar(&c.Continue, "continue",
		false,
		"Continue the previously paused operation.")
	cmd.PersistentFlags().BoolVar(&c.Abort, "abort",
		false,
		"Abort the previously paused operation, reverting any changes.")
	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish.")
	cmd.PersistentFlags().BoolVar(&c.verbose, "verbose",
		false,
		"Combine stdout and stderr output from hooks.")
	cmd.MarkFlagsMutuallyExclusive("abort", "continue", "wait-on-error")

	return cmd
}

func workshopName(name string) string {
	// Check if the name contains an SDK reference.
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '/' })
	if len(parts) > 1 {
		return parts[0]
	}
	return name
}

func (c *CmdRefresh) RunRefresh(cli *client.Client, project *client.Project, av []string) (*client.Change, error) {
	mode := "transactional"
	if c.WaitOnError {
		mode = "wait-on-error"
	}
	if c.Continue {
		mode = "continue"
	}
	if c.Abort {
		mode = "abort"
	}

	option := ""
	if mode == "transactional" || mode == "wait-on-error" {
		option = "update"
	}

	changeId, err := cli.Refresh(project.Id, av, mode, option, c.verbose)
	if err != nil {
		return nil, err
	}

	return c.wait(cli, changeId)
}

func (c *CmdRefresh) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	av = strutil.Deduplicate(av)
	if len(av) == 0 {
		name, err := cli.SingleWorkshopName(project)
		if err != nil {
			return err
		}
		av = []string{name}
	}

	chg, err := c.RunRefresh(cli, project, av)

	switch {
	case err == nil:
		fmt.Fprintf(Stdout, "%s refreshed\n", strutil.Quoted(av))
	case errors.Is(err, errNoWait):
	case client.IsNoUpdatesAvailable(err):
		fmt.Fprintf(Stdout, "no updates available for %s\n", strutil.Quoted(av))
	case errors.Is(err, errUndone):
		fmt.Fprintf(Stdout, "%s refresh aborted\n", strutil.Quoted(av))
	case errors.Is(err, errWaitOnError):
		w := workshopName(av[0])
		return fmt.Errorf(`
cannot refresh %[1]q; paused
To view details: "workshop tasks %[2]s"

To abort and undo: "workshop refresh --abort %[1]s"
Otherwise, resolve the error, then run "workshop refresh --continue %[1]s"`[1:],
			w,
			chg.ID,
		)
	default:
		return fmt.Errorf("cannot refresh %s: %w", strutil.Quoted(av), err)
	}

	return nil
}
