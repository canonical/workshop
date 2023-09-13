// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/adrg/xdg"
	"github.com/canonical/x-go/i18n"
	"github.com/canonical/x-go/strutil"
	"github.com/canonical/x-go/strutil/quantity"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/osutil"
)

type CmdWarnings struct {
	timeMixin
	unicodeMixin
	root    *CmdRoot
	All     bool
	Verbose bool
}

type CmdOkay struct {
	root *CmdRoot
}

func (c *CmdWarnings) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "warnings [OPTIONS]",
		Args:  cobra.NoArgs,
		Short: "List warnings",
		Long: `
This command lists the warnings that were reported to the system.

All warnings listed by 'workshop warnings'
can be acknowledged with the 'workshop okay' command.
Acknowledged warnings aren't listed by 'workshop warnings'
unless they occur again after their cooldown period has elapsed
or the '--all' option is used.

Also, warnings expire automatically; expired warnings are not listed.
`,
		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.All, "all",
		false,
		"Show all warnings, including the acknowledged ones.")

	cmd.PersistentFlags().BoolVar(&c.All, "verbose",
		false,
		"Show more information per each warning.")

	cmd.PersistentFlags().BoolVar(&c.AbsTime, "abs-time",
		false,
		`Use absolute times in RFC 3339 format.
By default, relative times are used up to 60 days, then YYYY-MM-DD.`)

	cmd.PersistentFlags().StringVar(&c.Unicode, "unicode",
		"auto",
		`Use Unicode characters to improve legibility (auto|never|always).
By default, Unicode is used only if the output supports it.`)

	return cmd
}

func (c *CmdOkay) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "okay",
		Args:  cobra.NoArgs,
		Short: "Acknowledge listed warnings",
		Long: `
This command acknowledges all warnings
listed previously by the 'workshop warnings' command.
`,
		RunE: c.Run,
	}

	return cmd
}

func termSize() (width, height int) {
	if f, ok := Stdout.(*os.File); ok {
		width, height, _ = terminal.GetSize(int(f.Fd()))
	}

	if width <= 0 {
		width = int(osutil.GetenvInt64("COLUMNS"))
	}

	if height <= 0 {
		height = int(osutil.GetenvInt64("LINES"))
	}

	if width < 40 {
		width = 80
	}

	if height < 15 {
		height = 25
	}

	return width, height
}

// printDescr formats a given string (typically a workshop description)
// in a user friendly way.
//
// The rules are (intentionally) very simple:
// - trim trailing whitespace
// - word wrap at "max" chars preserving line indent
// - keep \n intact and break there
func printDescr(w io.Writer, descr string, termWidth int) error {
	var err error
	descr = strings.TrimRightFunc(descr, unicode.IsSpace)
	for _, line := range strings.Split(descr, "\n") {
		err = strutil.WordWrapPadded(w, []rune(line), "  ", termWidth)
		if err != nil {
			break
		}
	}
	return err
}

func (c *CmdWarnings) Run(cmd *cobra.Command, av []string) error {
	now := time.Now()

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	warnings, err := cli.Warnings(client.WarningsOptions{All: c.All})
	if err != nil {
		return err
	}
	if len(warnings) == 0 {
		if t, _ := lastWarningTimestamp(); t.IsZero() {
			fmt.Fprintln(Stdout, "No warnings.")
		} else {
			fmt.Fprintln(Stdout, "No further warnings.")
		}
		return nil
	}

	if err := writeWarningTimestamp(now); err != nil {
		return err
	}

	termWidth, _ := termSize()
	if termWidth > 100 {
		// any wider than this and it gets hard to read
		termWidth = 100
	}

	esc := c.getEscapes()
	w := tabWriter()
	for i, warning := range warnings {
		if i > 0 {
			fmt.Fprintln(w, "---")
		}
		if c.Verbose {
			fmt.Fprintf(w, "first-occurrence:\t%s\n", c.fmtTime(warning.FirstAdded))
		}
		fmt.Fprintf(w, "last-occurrence:\t%s\n", c.fmtTime(warning.LastAdded))
		if c.Verbose {
			lastShown := esc.dash
			if !warning.LastShown.IsZero() {
				lastShown = c.fmtTime(warning.LastShown)
			}
			fmt.Fprintf(w, "acknowledged:\t%s\n", lastShown)
			// TODO: cmd.fmtDuration() using timeutil.HumanDuration
			fmt.Fprintf(w, "repeats-after:\t%s\n", quantity.FormatDuration(warning.RepeatAfter.Seconds()))
			fmt.Fprintf(w, "expires-after:\t%s\n", quantity.FormatDuration(warning.ExpireAfter.Seconds()))
		}
		fmt.Fprintln(w, "warning: |")
		printDescr(w, warning.Message, termWidth)
		w.Flush()
	}

	return nil
}

func (c *CmdOkay) Run(cmd *cobra.Command, av []string) error {
	last, err := lastWarningTimestamp()
	if err != nil {
		return err
	}

	cli, err := c.root.client()
	if err != nil {
		return err
	}

	return cli.Okay(last)
}

const warnFileEnvKey = "WORKSHOPD_LAST_WARNING_TIMESTAMP_FILENAME"

func warnFilename() string {
	if fn := os.Getenv(warnFileEnvKey); fn != "" {
		return fn
	}

	return filepath.Join(xdg.CacheHome, "workshop", "warnings.json")
}

type clientWarningData struct {
	Timestamp time.Time `json:"timestamp"`
}

func writeWarningTimestamp(t time.Time) error {
	user, err := osutil.UserMaybeSudoUser()
	if err != nil {
		return err
	}
	uid, gid, err := osutil.UidGid(user)
	if err != nil {
		return err
	}

	filename := warnFilename()
	if err := osutil.MkdirAllChown(filepath.Dir(filename), 0700, uid, gid); err != nil {
		return err
	}

	aw, err := osutil.NewAtomicFile(filename, 0600, 0, uid, gid)
	if err != nil {
		return err
	}
	// Cancel once Committed is a NOP :-)
	defer aw.Cancel()

	enc := json.NewEncoder(aw)
	if err := enc.Encode(clientWarningData{Timestamp: t}); err != nil {
		return err
	}

	return aw.Commit()
}

func lastWarningTimestamp() (time.Time, error) {
	_, err := osutil.UserMaybeSudoUser()
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot determine real user: %v", err)
	}

	f, err := os.Open(warnFilename())
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, fmt.Errorf("you must have looked at the warnings before acknowledging them. Try 'workshop warnings'.")
		}
		return time.Time{}, fmt.Errorf("cannot open timestamp file: %v", err)

	}
	dec := json.NewDecoder(f)
	var d clientWarningData
	if err := dec.Decode(&d); err != nil {
		return time.Time{}, fmt.Errorf("cannot decode timestamp file: %v", err)
	}
	if dec.More() {
		return time.Time{}, fmt.Errorf("spurious extra data in timestamp file")
	}
	return d.Timestamp, nil
}

func maybePresentWarnings(count int, timestamp time.Time) {
	if count == 0 {
		return
	}

	if last, _ := lastWarningTimestamp(); !timestamp.After(last) {
		return
	}

	fmt.Fprintf(Stderr,
		i18n.NG("WARNING: There is %d new warning. See 'workshop warnings'.\n",
			"WARNING: There are %d new warnings. See 'workshop warnings'.\n",
			count),
		count)
}
