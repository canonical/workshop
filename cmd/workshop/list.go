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
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/juju/ansiterm/tabwriter"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
)

type CmdList struct {
	root      *CmdRoot
	global    bool
	noHeaders bool
}

func (c *CmdList) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "list",
		Args:    cobra.ExactArgs(0),
		Short:   "List project workshops",
		GroupID: GrpExplore,
		Long: `
This command enumerates all workshops in the project, printing a compact list:

- Project:  Absolute pathname of the project where this workshop belongs

- Workshop: Workshop name, as set by its definition

- Status:   Workshop status, such as "Off", "Ready", "Pending" and so on

- Notes:    Internal remarks on the overall state of the workshop


The "--global" option lists all workshops from all projects in the system;
however, it doesn't include any that are "Off".


Notes:

- For details of a single workshop, use "workshop info" instead.
`,
		Example: `
List the workshops in the current project directory:
$ workshop list

List the globally registered workshops:
$ workshop list --global`,
		RunE: c.Run,
	}

	cmd.Flags().BoolVar(&c.global, "global", false, "List workshops from all projects in the system.")
	cmd.Flags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

func (c *CmdList) Run(cmd *cobra.Command, _ []string) error {
	// check if both --project and --global were provided
	if cmd.Parent().Flag("project").Changed && cmd.Flag("global").Changed {
		return fmt.Errorf(`cannot list: "--project" incompatible with "--global"`)
	}
	return c.runList()
}

func (c *CmdList) runList() error {
	if c.global {
		return c.listGlobal()
	}
	return c.listProject()
}

func (c *CmdList) listProject() error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	workshops, files, err := cli.List(&client.ListOptions{ProjectId: project.Id})
	if err != nil {
		return err
	}

	merged := sortWorkshops(workshops, files)
	if len(merged) == 0 {
		return nil
	}

	w := tabWriter()
	if !c.noHeaders {
		fmt.Fprintf(w, "WORKSHOP\tSTATUS\tNOTES\n")
	}
	for _, wp := range merged {
		fmt.Fprintf(w, "%s\t%s\t%s\n", wp.name, wp.status, wp.notes)
	}
	w.Flush()

	return nil
}

func (c *CmdList) listGlobal() error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	projects, err := cli.Projects()
	if err != nil {
		return err
	}

	w := tabWriter()
	printHeaders := !c.noHeaders
	for _, p := range projects {
		workshops, _, err := cli.List(&client.ListOptions{ProjectId: p.Id})
		if err != nil {
			return err
		}
		// --global flag does not list files for consistency. We may not be
		// aware of all the project directories on the system and, thus,
		// will not know all the available "Off" workshops (contrary to the
		// workshops that are in any known state, i.e. running instances,
		// which we always know about from the workshop backend).
		merged := sortWorkshops(workshops, nil)
		if len(merged) == 0 {
			continue
		}

		if printHeaders {
			fmt.Fprintf(w, "PROJECT\tWORKSHOP\tSTATUS\tNOTES\n")
			printHeaders = false
		}
		project := cmdutil.ContractHome(p.Path)
		for _, wp := range merged {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", project, wp.name, wp.status, wp.notes)
		}
	}
	w.Flush()

	return nil
}

type workshopSummary struct {
	name   string
	status string
	notes  string
}

func sorter[T *client.WorkshopInfo | *client.WorkshopFile](extract func(T) string) func(a, b T) int {
	return func(a, b T) int {
		return cmp.Compare(extract(a), extract(b))
	}
}

func sortWorkshops(workshops []*client.WorkshopInfo, files []*client.WorkshopFile) []workshopSummary {
	result := make([]workshopSummary, 0, len(workshops)+len(files))

	slices.SortFunc(workshops, sorter(func(w *client.WorkshopInfo) string { return w.Name }))
	for _, wp := range workshops {
		notes := cmdutil.EmptyDash(strings.Join(wp.Notes, ","))
		result = append(result, workshopSummary{wp.Name, wp.Status, notes})
	}

	slices.SortFunc(files, sorter(func(f *client.WorkshopFile) string { return f.Name }))
	for _, wf := range files {
		_, found := slices.BinarySearchFunc(workshops, wf, func(w *client.WorkshopInfo, wf *client.WorkshopFile) int {
			return cmp.Compare(w.Name, wf.Name)
		})
		if !found {
			result = append(result, workshopSummary{wf.Name, "Off", "-"})
		}
	}

	return result
}

func tabWriter() *tabwriter.Writer {
	/* Tab writer uses the same formatting as snap list */
	return tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', tabwriter.StripEscape)
}
