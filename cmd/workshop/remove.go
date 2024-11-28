package main

import (
	"fmt"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
)

type CmdRemove struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdRemove) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "remove <WORKSHOP>...",
		Args:  cobra.MinimumNArgs(1),
		Short: "Remove one or many workshops",
		Long: `
This command removes the workshops listed as arguments. For each workshop, it:

- Checks that the workshop isn't *Off* or *Pending*
- Stops the workshop if it's not already *Stopped*
- Deletes the workshop but preserves its definition

Notes:

- If any listed workshop is *Off* or *Pending*, none are removed
- To rebuild a removed workshop from scratch, use **workshop launch**
- For content interface plugs, non-default sources set by **workshop remount**
  aren't removed
`,
		Example: `
# Remove the 'nimble' and 'jazzy' workshops in the current project directory
workshop remove nimble jazzy`,
		RunE: c.Run,
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

	project, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	changeId, err := cli.Remove(project.Id, av)
	if err != nil {
		return err
	}

	user, err := osutil.UserMaybeSudoUser()
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId, false); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	// Drop all the workshops' hack directories if exist. Hack SDK content is
	// controlled by the client code now, thus, we will not consider it to be a
	// responsibility of workshopd to drop the hack directory on removal (see
	// doRemoveWorkshop). Hack SDK is a type of a local SDK that will continue
	// to exist in a stored directory for some time after the workshop removal
	// so if recreated, it can be summoned back with 'workshop hack --restore
	// <WORKSHOP>.
	// workshopd will, however, be responsible for the final clean up of the
	// hack SDK content (e.g. if Workshop is removed from the system or the hack
	// SDK content was stored for over 90 days).
	for _, wp := range av {
		hackdir := sdk.WorkshopHackSdkCurrent(user.HomeDir, project.Id, wp)
		if exists, dir, _ := osutil.ExistsIsDir(hackdir); exists && dir {
			storedir := sdk.WorkshopHackSdkStored(user.HomeDir, project.Id, wp)
			if _, err := dropHack(hackdir, storedir); err != nil {
				fmt.Fprintf(Stderr, "cannot drop hack SDK for %q: %v\n", wp, err)
			}
		}
	}

	for _, name := range av {
		fmt.Fprintf(Stdout, "%q removed\n", name)
	}

	return nil
}
