package main

import (
	"cmp"
	"fmt"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
)

type CmdInfo struct {
	waitMixin
	root *CmdRoot
}

func (c *CmdInfo) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "info <WORKSHOP>",
		Args:  cobra.RangeArgs(1, 1),
		Short: "Print the current status and details of a workshop as YAML",
		Long: `
This command outputs the basic settings, current status and individual SDK
details for a workshop, formatting them as YAML. Specifically, it prints:

- Essential workshop attributes, such as name, base and project directory

- Current status (e.g. *Ready*, *Pending*, *Off*) and notes for the workshop

- Individual SDK details, such as name, channel, installation date and revision

- Currently mounted content interface plugs


Notes:

- Avoid assumptions based on SDK channels: 'latest/stable' may be neither
`,

		RunE: c.Run,
	}

	return cmd
}

// The default paths created by workshop are not human friendly. Thus, the
// client checks if that path is a lengthy default and substitutes its common
// prefix with .../. Hence something like:
//
//	/home/dmitry/.local/share/workshop/project/17942561/mount/albert_go_mod-cache.sdk
//
// becomes:
//
//	.../17942561/mount/albert_go_mod-cache.sdk
func shortenDefaulPath(source string) string {
	user, _ := user.Current()
	defaultPathPrefix := filepath.Join(user.HomeDir, ".local", "share", "workshop", "project")
	if after, ok := strings.CutPrefix(source, defaultPathPrefix); ok {
		return "..." + after
	}
	return source
}

func (c *CmdInfo) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	workshop, err := cli.Workshop(project.Id, av[0])
	if err != nil {
		return err
	}

	w := tabWriter()

	fmt.Fprintf(w, "name:\t%s\n", workshop.Name)
	fmt.Fprintf(w, "base:\t%s\n", workshop.Base)
	fmt.Fprintf(w, "project:\t%s\n", project.Path)
	fmt.Fprintf(w, "status:\t%s\n", strings.ToLower(workshop.Status))

	// get the workshop notes
	notes := workshop.Notes

	// get the SDKs notes (if there is an ongoing health check)
	for _, sdk := range workshop.Content {
		if sdk.Health != nil && sdk.Health.Code != "" {
			notes = append(notes, sdk.Health.Code)
		}
	}

	// combine notes from workshop and its SDKs
	notesFormatted := strings.Join(notes, ",")
	if len(workshop.Notes) == 0 {
		notesFormatted = "-"
	}

	fmt.Fprintf(w, "notes:\t%s\n", notesFormatted)

	if len(workshop.Content) > 0 {
		fmt.Fprintf(w, "content:\n")
		for _, sdk := range workshop.Content {
			fmt.Fprintf(w, "  %s:\n", sdk.Name)
			installTime := sdk.InstallTime.Format("2006-01-02")
			if sdk.InstallTime.IsZero() {
				installTime = ""
			}
			if sdk.Channel == "" {
				sdk.Channel = "~"
			}
			fmt.Fprintf(w, "    channel:\t%s\t%s\t(%s)\n", sdk.Channel, installTime, sdk.Revision)
			if sdk.Health != nil {
				fmt.Fprintf(w, "    message:\t%s\n", sdk.Health.Message)
			}

			if len(sdk.Mounts) > 0 {
				fmt.Fprintf(w, "    mounts:\n")
				slices.SortFunc(sdk.Mounts, func(a, b *client.Mount) int { return cmp.Compare(a.Plug.Name, b.Plug.Name) })
				for _, mount := range sdk.Mounts {
					if mount.HostSource != "" {
						fmt.Fprintf(w, "      %s:\n", mount.Plug.Name)
						fmt.Fprintf(w, "        host-source:\t%s\n", shortenDefaulPath(mount.HostSource))
						fmt.Fprintf(w, "        workshop-target:\t%s\n", mount.WorkshopTarget)
						continue
					}
					if mount.WorkshopSource != "" {
						fmt.Fprintf(w, "      %s:\n", mount.Plug.Name)
						fmt.Fprintf(w, "        workshop-source:\t%s\n", mount.WorkshopSource)
						fmt.Fprintf(w, "        workshop-target:\t%s\n", mount.WorkshopTarget)
						continue
					}
				}
			}
		}
	}

	w.Flush()

	return nil
}
