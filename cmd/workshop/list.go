package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
)

type CmdList struct {
	root   *CmdRoot
	global bool
}

func (c *CmdList) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.ExactArgs(0),
		Short: "List project workshops",
		Long: `
This command enumerates all workshops in the project, printing a compact list:

- Project:  Absolute pathname of the project where this workshop belongs

- Workshop: Workshop name, as set by its definition

- Status:   Workshop status, such as 'Off', 'Ready', 'Pending' and so on

- Notes:    Internal remarks on the overall state of the workshop


The '--global' option lists all workshops from all projects in the system;
however, it doesn't include any that are 'Off'.


Notes:

- For details of a single workshop, use 'workshop info' instead.
`,
		Example: `
List the workshops in the current project directory:
$ workshop list

List the globally registered workshops:
$ workshop list --global`,
		RunE: c.Run,
	}

	cmd.Flags().BoolVar(&c.global, "global", false, "List workshops from all projects in the system.")

	return cmd
}

func (c *CmdList) Run(cmd *cobra.Command, _ []string) error {
	// check if both --project and --global were provided
	if cmd.Parent().Flag("project").Changed && cmd.Flag("global").Changed {
		return fmt.Errorf("cannot list: '--project' incompatible with '--global'")
	}
	return c.runList()
}

func (c *CmdList) runList() error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	w := tabWriter()
	var header sync.Once
	printHeader := func() {
		fmt.Fprintf(w, "Project\tWorkshop\tStatus\tNotes\n")
	}

	if !c.global {
		project, err := cli.Project(c.root.project())
		if err != nil {
			return err
		}

		workshops, files, err := cli.List(&client.ListOptions{ProjectId: project.Id})
		if err != nil {
			return err
		}

		/* List all workshops for the current project */
		if len(workshops) != 0 || len(files) != 0 {
			header.Do(printHeader)
			print(w, workshops, files, *project)
		}
	} else {
		projects, err := cli.Projects()
		if err != nil {
			return err
		}

		for _, p := range projects {
			workshops, _, err := cli.List(&client.ListOptions{ProjectId: p.Id})
			if err != nil {
				return err
			}
			if len(workshops) == 0 {
				continue
			}
			header.Do(printHeader)
			// --global flag does not list files for consistency. We may not be
			// aware of all the project directories on the system and, thus,
			// will not know all the available "Off" workshops (contrary to the
			// workshops that are in any known state, i.e. running instances,
			// which we always know about from the workshop backend).
			print(w, workshops, nil, p)
		}
	}

	w.Flush()

	return nil
}

func sorter[T *client.WorkshopInfo | *client.WorkshopFile](extract func(T) string) func(a, b T) int {
	return func(a, b T) int {
		return cmp.Compare(extract(a), extract(b))
	}
}

func print(w *tabwriter.Writer, workshops []*client.WorkshopInfo, files []*client.WorkshopFile, prj client.Project) {
	slices.SortFunc(workshops, sorter(func(w *client.WorkshopInfo) string { return w.Name }))
	for _, wp := range workshops {
		line := workshopEntry(wp, prj)
		fmt.Fprintln(w, strings.Join(line, "\t"))
	}

	slices.SortFunc(files, sorter(func(f *client.WorkshopFile) string { return f.Name }))
	for _, wf := range files {
		_, found := slices.BinarySearchFunc(workshops, wf, func(w *client.WorkshopInfo, wf *client.WorkshopFile) int {
			return cmp.Compare(w.Name, wf.Name)
		})
		if !found {
			line := fileEntry(wf, prj)
			fmt.Fprintln(w, strings.Join(line, "\t"))
		}
	}
}

func fileEntry(w *client.WorkshopFile, p client.Project) []string {
	line := []string{
		cmdutil.ContractHome(p.Path),
		w.Name,
		"Off",
		"-",
	}
	return line
}

func workshopEntry(w *client.WorkshopInfo, p client.Project) []string {
	comment := cmdutil.EmptyDash(strings.Join(w.Notes, ","))
	line := []string{
		cmdutil.ContractHome(p.Path),
		w.Name,
		w.Status,
		comment,
	}
	return line
}

func tabWriter() *tabwriter.Writer {
	/* Tab writer uses the same formatting as snap list */
	return tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', 0)
}
