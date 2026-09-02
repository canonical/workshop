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
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/canonical/lxd/shared/units"
	"github.com/juju/ansiterm/tabwriter"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/workshop"
)

type CmdInfo struct {
	cmdutil.ColorMixin
	root *CmdRoot

	Base string
	Arch string
}

func (c *CmdInfo) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <SDK>",
		Short:   "Show SDK info",
		GroupID: GrpExplore,
		Long: `
Prints the SDK's metadata,
shows the revisions currently available in the SDK Store,
and lists workshops where the SDK is installed.

Notes:

- The output shows the SDK's build date.
- For an overview of SDK volumes, use "sdk list".
- For per-workshop information, use "workshop info".
`,
		Example: `
Show metadata, Store channels, and local installations for the "openvino" SDK:
$ sdk info openvino

Restrict the Store channels to a specific base:
$ sdk info openvino --base ubuntu@24.04

Show the channels for every supported architecture:
$ sdk info openvino --arch all`,
		Args: cobra.ExactArgs(1),
		RunE: c.Run,
	}

	cmd.PersistentFlags().StringVar(&c.Base, "base", "",
		`Show SDKs compatible with a specific base.`)
	cmd.PersistentFlags().StringVar(&c.Arch, "arch", "",
		`Show SDKs compatible with a different architecture (or "all").`)

	_ = cmd.RegisterFlagCompletionFunc("base", cmdutil.CompleteChoices(workshop.SupportedBases...))
	arches := append([]string{"all"}, arch.AllowedArchitectures...)
	_ = cmd.RegisterFlagCompletionFunc("arch", cmdutil.CompleteChoices(arches...))

	return cmd
}

var channelRisks = []string{"stable", "candidate", "beta", "edge"}

func (c *CmdInfo) Run(cmd *cobra.Command, av []string) error {
	if c.Arch == "" {
		c.Arch = arch.DpkgArchitecture()
	}

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	info, err := cli.SdkInfo(av[0])
	if err != nil {
		return err
	}

	tracks, revsByChannel := sortChannels(filterChannels(info.Channels, c.Base, c.Arch))

	installed := filterInstalled(info.Installed, c.Base, c.Arch)
	slices.SortFunc(installed, func(a, b client.SdkInstalled) int {
		if a.ProjectPath != b.ProjectPath {
			return cmp.Compare(a.ProjectPath, b.ProjectPath)
		}
		return cmp.Compare(a.Workshop, b.Workshop)
	})

	esc := c.GetEscapes()
	w := tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', tabwriter.StripEscape)
	fmt.Fprintf(w, "name:\t%s\n", info.Name)
	if info.Publisher != nil {
		fmt.Fprintf(w, "publisher:\t%s\n", longPublisher(info.Publisher, esc))
	}
	if info.License != "" {
		fmt.Fprintf(w, "license:\t%s\n", info.License)
	}
	if info.Website != "" {
		website := info.Website
		if u, err := url.Parse(info.Website); err == nil {
			website = esc.MakeLink(info.Website, u, info.Website)
		}
		fmt.Fprintf(w, "website:\t%s\n", website)
	}

	if info.Description != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.TrimSuffix(info.Description, "\n"))
	}

	maxRev := len("REV")
	maxSize := len("SIZE")
	for _, revs := range revsByChannel {
		for _, rev := range revs {
			maxRev = max(maxRev, len(rev.Revision))
			maxSize = max(maxSize, len(units.GetByteSizeString(rev.DownloadSize, 2)))
		}
	}
	baseTpl := "%s\t"
	if c.Base != "" {
		baseTpl = "%.0s"
	}
	archTpl := "%s\t"
	if c.Arch != "all" {
		archTpl = "%.0s"
	}
	tpl := "  %s\t%s\t%s\t" + baseTpl + archTpl + "%*s\t%*s\n"
	if len(tracks) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s%s%s\n", esc.Bold, "CHANNELS", esc.End)
		fmt.Fprintf(w, tpl, "CHANNEL", "VERSION", "BUILD", "BASE", "ARCH", maxRev, "REV", maxSize, "SIZE")
	}
	for _, track := range tracks {
		closedSign := "-"
		for _, risk := range channelRisks {
			channel := track + "/" + risk
			revs := revsByChannel[channel]
			if len(revs) == 0 {
				fmt.Fprintf(w, tpl, channel, closedSign, "", "", "", 0, "", 0, "")
				continue
			}
			closedSign = esc.UpArrow

			var prev *client.SdkRevision
			for _, rev := range revs {
				channel, version, build, base := optionalColumns(prev, rev)
				prev = rev

				size := units.GetByteSizeString(rev.DownloadSize, 2)
				fmt.Fprintf(w, tpl, channel, version, build, base, rev.Arch, maxRev, rev.Revision, maxSize, size)
			}
		}
	}

	maxRev = len("REV")
	for _, it := range installed {
		maxRev = max(maxRev, len(it.Revision))
	}
	tpl = "  %s\t%s\t%s\t%s\t" + baseTpl + archTpl + "%*s\n"
	if len(installed) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s%s%s\n", esc.Bold, "INSTALLED", esc.End)
		fmt.Fprintf(w, tpl, "PROJECT", "WORKSHOP", "CHANNEL", "VERSION", "BASE", "ARCH", maxRev, "REV")
	}
	for _, it := range installed {
		project := cmdutil.ContractHome(it.ProjectPath)
		channel := cmdutil.EmptyDash(it.Channel)
		base := it.Base
		if base == "" {
			base = "all"
		}
		fmt.Fprintf(w, tpl, project, it.Workshop, channel, it.Version, base, it.Arch, maxRev, it.Revision)
	}
	w.Flush()

	return nil
}

func filterChannels(channels []*client.SdkRevision, base, arch string) []*client.SdkRevision {
	result := slices.Clone(channels)
	return slices.DeleteFunc(result, func(rev *client.SdkRevision) bool {
		return ((base != "" && rev.Base != "" && base != rev.Base) ||
			(arch != "all" && rev.Arch != "all" && arch != rev.Arch))
	})
}

// sortChannels lists tracks (e.g. "latest") in whatever order the Store
// decides, and groups revisions by channel (e.g. "latest/stable"). Within a
// channel, revisions are sorted first by base OS (e.g. "ubuntu"), then by
// descending base series (e.g. "24.04"), and finally by architecture. In the
// unlikely event there's more than one revision with the same channel, base
// and architecture, they are sorted by descending revision number.
func sortChannels(channels []*client.SdkRevision) ([]string, map[string][]*client.SdkRevision) {
	tracks := make([]string, 0, len(channels))
	seen := make(map[string]bool, len(channels))
	revsByChannel := make(map[string][]*client.SdkRevision, len(channels))

	for _, rev := range channels {
		if !seen[rev.Track] {
			tracks = append(tracks, rev.Track)
			seen[rev.Track] = true
		}

		revsByChannel[rev.Channel] = append(revsByChannel[rev.Channel], rev)
	}

	for _, revisions := range revsByChannel {
		slices.SortFunc(revisions, func(a, b *client.SdkRevision) int {
			os1, series1, _ := strings.Cut(a.Base, "@")
			os2, series2, _ := strings.Cut(b.Base, "@")
			if os1 != os2 {
				return cmp.Compare(os1, os2)
			}
			if series1 != series2 {
				// Newest first.
				return cmp.Compare(series2, series1)
			}
			if a.Arch != b.Arch {
				return cmp.Compare(a.Arch, b.Arch)
			}
			// Should be unreachable.
			return cmp.Compare(b.Revision, a.Revision)
		})
	}

	return tracks, revsByChannel
}

func filterInstalled(installed []client.SdkInstalled, base, arch string) []client.SdkInstalled {
	result := slices.Clone(installed)
	return slices.DeleteFunc(result, func(it client.SdkInstalled) bool {
		return ((base != "" && it.Base != "" && base != it.Base) ||
			(arch != "all" && it.Arch != "all" && arch != it.Arch))
	})
}

func longPublisher(publisher *client.StoreAccount, esc *cmdutil.Escapes) string {
	var badge string
	switch publisher.Validation {
	case "verified":
		badge = esc.Green + esc.Tick + esc.End
	case "starred":
		badge = esc.BrightYellow + esc.Star + esc.End
	}
	// Following snap, we only show the username if it significantly
	// differs from the display name.
	niceUsername := strings.ReplaceAll(publisher.Username, "-", " ")
	if strings.EqualFold(niceUsername, publisher.DisplayName) {
		return publisher.DisplayName + badge
	}
	return fmt.Sprintf("%s (%s%s)", publisher.DisplayName, publisher.Username, badge)
}

func optionalColumns(prev, rev *client.SdkRevision) (string, string, string, string) {
	var channel, version, build, base string
	if prev == nil {
		channel = rev.Channel
	}
	if prev == nil || prev.Version != rev.Version {
		version = cmdutil.EmptyDash(rev.Version)
		// Print everything after this, to avoid holes in the middle.
		prev = nil
	}
	if prev == nil || formatDate(prev.BuiltAt) != formatDate(rev.BuiltAt) {
		build = formatDate(rev.BuiltAt)
		prev = nil
	}
	if prev == nil || prev.Base != rev.Base {
		base = rev.Base
		if base == "" {
			base = "all"
		}
		prev = nil
	}
	return channel, version, build, base
}

func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}
