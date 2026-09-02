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

type CmdFind struct {
	cmdutil.ColorMixin
	root      *CmdRoot
	noHeaders bool
}

func (c *CmdFind) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "find <QUERY>",
		Short:   "Search the Store for SDKs",
		GroupID: GrpExplore,
		Long: `
Search the Store for SDKs matching the given query.
The query can match the SDK's name, title, summary, description, or publisher.

Notes:

- Only the latest release of the SDK is shown.
- To view more details for one of the SDKs, use "sdk info".
- To list SDKs on the local system, use "sdk list".
`,
		Example: `
Search for SDKs matching a single keyword:
$ sdk find openvino

Combine multiple words into a single query:
$ sdk find jupyter notebooks

Hide the table header in the output:
$ sdk find openvino --no-headers`,
		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

func (c *CmdFind) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	query := strings.Join(av, " ")
	if strings.TrimSpace(query) == "" {
		query = ""
	}

	sdks, err := cli.FindSdks(query)
	if err != nil {
		return err
	}

	if len(sdks) == 0 {
		return fmt.Errorf("no matching SDKs for %q", query)
	}

	slices.SortFunc(sdks, func(a, b client.SdkSummary) int {
		return cmp.Compare(a.Name, b.Name)
	})

	esc := c.GetEscapes()
	w := tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', tabwriter.StripEscape)
	if !c.noHeaders {
		fmt.Fprintln(w, "NAME\tVERSION\tPUBLISHER\tSUMMARY")
	}
	for _, sdk := range sdks {
		version := cmdutil.EmptyDash(sdk.Version)
		publisher := "-"
		if sdk.Publisher != nil {
			publisher = shortPublisher(sdk.Publisher, esc)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sdk.Name, version, publisher, sdk.Summary)
	}

	return w.Flush()
}

func shortPublisher(publisher *client.StoreAccount, esc *cmdutil.Escapes) string {
	var badge string
	switch publisher.Validation {
	case "verified":
		badge = esc.Green + esc.Tick + esc.End
	case "starred":
		badge = esc.BrightYellow + esc.Star + esc.End
	}
	return publisher.DisplayName + badge
}
