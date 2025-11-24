package main

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
)

type CmdRemount struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdRemount) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remount <WORKSHOP>/<SDK>:<PLUG> <SOURCE>",
		Args:  cobra.ExactArgs(2),
		Short: "Mount a new source location to the mount interface plug's target",
		Long: `
This command mounts a new source location on the host to the target directory
of the specified mount interface plug, qualified by the SDK name.
Specifically, it does the following:

- Attempts the mount operation atomically;
  this normally succeeds if the new source is either a non-existing directory
  or an empty directory on the same file system as the current source.

- Otherwise, performs the mount operation only if the workshop is 'Stopped'
  to prevent data corruption.


Notes:

- To stop the workshop, use 'workshop stop'.

- 'workshop info' lists any connected mount interface plugs for the workshop.

- 'workshop refresh' mounts the last source set by 'workshop remount', if any.

- During 'workshop remove',
  non-default sources set by 'workshop remount' aren't removed.
`,
		Example: `
Remount the 'mod-cache' mount interface plug of the 'go' SDK
under the 'nimble' workshop in the current project directory
to '~/new-cache-mount/' on the host:
$ workshop remount nimble/go:mod-cache ~/new-cache-mount`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.PersistentFlags().BoolVar(&c.NoWait, "no-wait",
		false,
		"Return the change ID, don't wait for the operation to finish.")

	return cmd
}

func (c *CmdRemount) Run(cmd *cobra.Command, av []string) error {
	var err error

	plugRef, err := client.ParseShortPlugRef(av[0])
	if err != nil {
		return err
	}

	source, err := filepath.Abs(av[1])
	if err != nil {
		return err
	}

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	plugRef.ProjectId = project.Id

	changeId, err := cli.Remount(plugRef, source)
	if err != nil {
		return err
	}

	if _, err := c.wait(cli, changeId); err != nil {
		if err == errNoWait {
			return nil
		}
		return err
	}

	return nil
}

func (c *CmdRemount) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	switch len(args) {
	case 0:
		// continue below
	case 1:
		return completions, cobra.ShellCompDirectiveFilterDirs
	default:
		return completions, cobra.ShellCompDirectiveNoFileComp
	}

	cli, err := c.root.client()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	connections, err := cli.Connections(&client.ConnectionOptions{ProjectId: project.Id, Interface: "mount"})
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}

	// A mount must be connected for remount to work, only show currently
	// connected mounts
	for _, conn := range connections.Established {
		completions = append(completions, endpoint(conn.Plug.Workshop, conn.Plug.Sdk, conn.Plug.Name))
	}
	// We don't want file comp if there was no workshop name provided and
	// no completion was generated
	return completions, cobra.ShellCompDirectiveNoFileComp
}
