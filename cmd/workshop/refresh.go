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
	Restore     bool
}

func (c *CmdRefresh) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "refresh [--abort|--continue|--restore|--wait-on-error] <WORKSHOP>...",
		Short: "Update workshops according to their definitions",
		Long: `
This command updates the workshops listed as arguments. For each workshop, 
it checks the workshop definition and applies any required updates 
to the base image, SDKs and interface connections.

The "--wait-on-error" option pauses the refresh if an error occurs.
Thus, you can fix the error and resume the operation or abort and revert it.
This option can only be used with a single workshop.
If multiple workshops are listed and an error occurs,
the operation is aborted and reverted for all of them.

The "--restore" option restores the workshop from the snapshot that was 
created after the last successful launch or refresh. Any changes made 
to the workshop will be discarded.

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
		"Pause the operation on error; to resume, use \"--continue\" or \"--abort\".")
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
	cmd.PersistentFlags().BoolVar(&c.Restore, "restore",
		false,
		"Restore the workshop to the state after the most recent launch or refresh.")

	cmd.MarkFlagsMutuallyExclusive("abort", "continue", "wait-on-error")
	cmd.MarkFlagsMutuallyExclusive("restore", "continue", "abort")

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
		if c.Restore {
			option = "restore"
		}
	}

	changeId, err := cli.Refresh(project.Id, av, mode, option)
	if err != nil {
		return nil, err
	}

	return c.wait(cli, changeId)
}

func (c *CmdRefresh) Run(cmd *cobra.Command, av []string) error {
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
		return fmt.Errorf(`cannot complete refresh for %q, execution is paused

To proceed, resolve the issue and run "workshop refresh --continue %s"
To cancel and undo: "workshop refresh --abort %s"
To view more information: "workshop tasks %s"`, w, w, w, chg.ID)
	default:
		return fmt.Errorf("%v\n%s refresh aborted", err, strutil.Quoted(av))
	}

	return nil
}
