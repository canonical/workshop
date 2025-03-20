package main

import (
	"fmt"
	"strings"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/internal/progress"
)

type CmdRefresh struct {
	waitMixin
	root        *CmdRoot
	WaitOnError bool
	Continue    bool
	Abort       bool
	Raw         bool
	Verbose     bool
}

func (c *CmdRefresh) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "refresh [--abort|--continue|--wait-on-error] <WORKSHOP>[/<SDK>]...",
		Short: "Update workshops according to their definitions",
		Long: `
This command updates the workshops listed as arguments by going over their
definitions once again. For each workshop, it:

- Saves the working state of the workshop

- Checks the workshop definition and identifies any updates required

- Retrieves the updated components

- Applies and verifies the changes to the workshop

- Restores the working state of the workshop


The '--wait-on-error' option pauses the refresh if an error occurs.
Thus, you can fix the error and resume the operation or abort and revert it.
This option can only be used with a single workshop.
If multiple workshops are listed and an error occurs,
the operation is aborted and reverted for all of them.


The '--verbose' and '--raw' options show the output from SDK hooks run during
the refresh. '--verbose' shows a limited number of lines at a given time, and 
is designed for interactive use. '--raw' shows the same information, however 
renders every line and as such is ideal for a non-interactive environment where 
output is desired.


Notes:

- The workshop must be 'Ready' to be refreshed.

- To construct a newly defined workshop, use 'workshop launch' instead.

- Throughout the refresh, all affected workshops remain 'Pending'.

- If the refresh removes an SDK from the workshop, the SDK state isn't saved.

- Updated and newly added SDKs are installed in the order
  they are listed in the workshop definition.

- For mount interface plugs, mounts the last source
  set by 'workshop remount', if any.

- If the optional <SDK> is supplied,
  the operation is limited to this SDK;
  currently, it can only be 'sketch'.
`,
		Example: `
Refresh the 'nimble' and 'jazzy' workshops in the current project directory:
$ workshop refresh nimble jazzy

The name is optional if the project has only one workshop:
$ workshop refresh

Refresh workshop, but pause on any errors (won’t accept multiple workshops):
$ workshop refresh --wait-on-error

After refresh paused on error, abort the operation:
$ workshop refresh --abort

After refresh paused on error and the workshop was fixed,
continue the operation:
$ workshop refresh --continue

Refresh the sketch SDK in the 'nimble' workshop:
$ workshop refresh nimble/sketch`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopNames([]string{"Ready", "Waiting"}),
	}

	cmd.PersistentFlags().BoolVar(&c.WaitOnError, "wait-on-error",
		false,
		"Pause the operation on error; to resume, use '--continue' or '--abort'.")
	cmd.PersistentFlags().BoolVar(&c.Continue, "continue",
		false,
		"Continue the previously paused operation.")
	cmd.PersistentFlags().BoolVar(&c.Abort, "abort",
		false,
		"Abort the previously paused operation, reverting any changes.")
	cmd.PersistentFlags().BoolVar(&c.Verbose, "verbose",
		false,
		"Show verbose output")
	cmd.PersistentFlags().BoolVar(&c.Raw, "raw",
		false,
		"Show raw output")

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

func (c *CmdRefresh) Run(cmd *cobra.Command, av []string) error {
	av = strutil.Deduplicate(av)

	if c.Abort && c.Continue {
		return fmt.Errorf("cannot refresh: '--abort' incompatible with '--continue'")
	}

	if c.WaitOnError && c.Abort {
		return fmt.Errorf("cannot refresh: '--wait-on-error' incompatible with '--abort'")
	}

	if c.WaitOnError && c.Continue {
		return fmt.Errorf("cannot refresh: '--wait-on-error' incompatible with '--continue'")
	}

	if c.Raw && c.Verbose {
		return fmt.Errorf("cannot refresh: '--raw' incompatible with '--verbose'")
	}

	// We should have no more than one argument (a single workshop) for a
	// wait-on-error operation
	if (c.Abort || c.Continue || c.WaitOnError) && len(av) > 1 {
		return fmt.Errorf("cannot refresh: '--wait-on-error' incompatible with multiple workshops")
	}

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

	displayMode := progress.DisplayModeDefault
	if c.Verbose {
		displayMode = progress.DisplayModeVerbose
	}
	if c.Raw {
		displayMode = progress.DisplayModeRaw
	}

	changeId, err := cli.Refresh(project.Id, av, mode)
	if err != nil {
		return err
	}

	if chg, err := c.wait(cli, changeId, displayMode); err != nil {
		if err == errNoWait {
			return nil
		}

		if err == errWaitOnError {
			return fmt.Errorf(`%s

To proceed, resolve the issue and run "workshop refresh --continue %s"
To cancel and undo: "workshop refresh --abort %s"
To view more information: "workshop tasks %s"`, taskErrorFromChange(chg), workshopName(av[0]), workshopName(av[0]), changeId)
		}

		return fmt.Errorf("%v\n%s refresh aborted", err, strutil.Quoted(av))
	}

	if c.Abort {
		fmt.Fprintf(Stdout, "%q refresh aborted\n", av[0])
		return nil
	}

	for _, i := range av {
		fmt.Fprintf(Stdout, "%q refreshed\n", i)
	}
	return nil
}
