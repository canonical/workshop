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

	"github.com/canonical/lxd/shared/units"
	"github.com/juju/ansiterm/tabwriter"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
	"github.com/canonical/workshop/internal/sdk"
)

type CmdList struct {
	root      *CmdRoot
	noHeaders bool
}

func (c *CmdList) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List SDK volumes available on this machine",
		GroupID: GrpExplore,
		Long: `
This command lists all local SDK volumes.

Use it to enumerate the SDK revisions currently stored on the system.
Only volumes are reported, not the workshops that use them.

Notes:

- For per-workshop information, use "workshop info".
- Multiple entries may appear for a single SDK
  if several revisions are present simultaneously.
`,
		Example: `
List all local SDK volumes:
$ sdk list

Hide the table header in the output:
$ sdk list --no-headers`,
		Args: cobra.NoArgs,
		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

func (c *CmdList) Run(cmd *cobra.Command, _ []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	sdks, err := cli.Sdks()
	if err != nil {
		return err
	}

	if len(sdks) == 0 {
		return nil
	}

	slices.SortFunc(sdks, func(a, b client.SdkVolume) int {
		if a.Name != b.Name {
			return cmp.Compare(a.Name, b.Name)
		}
		rev1, err1 := sdk.ParseRevision(a.Revision)
		rev2, err2 := sdk.ParseRevision(b.Revision)
		if err1 == nil && err2 == nil {
			// Newest first.
			return cmp.Compare(rev2.N, rev1.N)
		}
		// Should be unreachable.
		return cmp.Compare(b.Revision, a.Revision)
	})

	w := tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', tabwriter.StripEscape)
	var maxRev, maxSize int
	if !c.noHeaders {
		maxRev = len("REV")
		maxSize = len("SIZE")
	}
	for _, sdk := range sdks {
		maxRev = max(maxRev, len(sdk.Revision))
		maxSize = max(maxSize, len(units.GetByteSizeString(sdk.Size, 2)))
	}
	if !c.noHeaders {
		fmt.Fprintf(w, "NAME\tVERSION\t%*s\t%*s\n", maxRev, "REV", maxSize, "SIZE")
	}
	for _, sdk := range sdks {
		version := cmdutil.EmptyDash(sdk.Version)
		size := units.GetByteSizeString(sdk.Size, 2)
		fmt.Fprintf(w, "%s\t%s\t%*s\t%*s\n", sdk.Name, version, maxRev, sdk.Revision, maxSize, size)
	}

	return w.Flush()
}
