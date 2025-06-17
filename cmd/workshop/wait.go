// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
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

package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/canonical/x-go/i18n"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/progress"
)

var (
	maxGoneTime = 5 * time.Second
	pollTime    = 100 * time.Millisecond
)

type waitMixin struct {
	NoWait    bool
	skipAbort bool

	verbose bool
	colorMixin
}

var errNoWait = errors.New("no wait for op")
var errWaitOnError = errors.New("wait-on-error")

//nolint:unparam // Copied from snapd.
func (wmx waitMixin) wait(cli *client.Client, id string) (*client.Change, error) {
	if wmx.NoWait {
		fmt.Fprintf(Stdout, "%s\n", id)
		return nil, errNoWait
	}
	wmx.Color = "auto"

	// Intercept sigint.
	c := make(chan os.Signal, 2)

	signal.Notify(c, os.Interrupt)
	go func() {

		sig := <-c
		if sig != nil && wmx.skipAbort {
			fmt.Fprintln(Stdout, "cannot interrupt: it may break the workshop, please wait until the operation is finished")
		}
		// sig is nil if c was closed
		if sig == nil || wmx.skipAbort {
			return
		}
		_, err := cli.Abort(id)
		if err != nil {
			fmt.Fprintf(Stderr, "%v\n", err)
		}
	}()

	var pb progress.Meter
	if wmx.verbose {
		pb = progress.MakeProgressBar(progress.NotifierRaw)
	} else {
		pb = progress.MakeProgressBar(progress.NotifierQuiet)
	}
	defer func() {
		pb.Finished()
		// next two not strictly needed for CLI, but without
		// them the tests will leak goroutines.
		signal.Stop(c)
		close(c)
	}()

	tMax := time.Time{}

	var lastID string
	for {
		var rebootingErr error
		chg, err := cli.Change(id, wmx.verbose)
		if err != nil {
			// a client.Error means we were able to communicate with
			// the server (got an answer)
			if e, ok := err.(*client.Error); ok {
				return nil, e
			}

			// an non-client error here means the server most
			// likely went away
			// XXX: it actually can be a bunch of other things; fix client to expose it better
			now := time.Now()
			if tMax.IsZero() {
				tMax = now.Add(maxGoneTime)
			}
			if now.After(tMax) {
				return nil, err
			}
			pb.Spin(i18n.G("Waiting for server to restart"))
			time.Sleep(pollTime)
			continue
		}
		if maintErr, ok := cli.Maintenance().(*client.Error); ok && maintErr.Kind == client.ErrorKindSystemRestart {
			rebootingErr = maintErr
		}
		if !tMax.IsZero() {
			pb.Finished()
			tMax = time.Time{}
		}

		// Tasks in "wait" state communicate the wait reason
		// via the log mechanism. So make sure the log is
		// visible even if the normal progress reporting
		// has tasks in "Doing" state (like "check-refresh")
		// that would suppress displaying the log. This will
		// ensure on a classic+modes system the user sees
		// the messages: "Task set to wait until a manual system restart allows to continue"
		for _, t := range chg.Tasks {
			if t.Status == "Wait" {
				return nil, errWaitOnError
			}
		}

		wmx.maybeShowLogs(pb, chg)

		// Report progress.
		for _, t := range chg.Tasks {
			switch {
			case t.Status != "Doing" && t.Status != "Undoing":
				continue
			case t.Progress.Total == 1:
				summary := wmx.fmtTaskSummary(chg, t)
				pb.Spin(summary)
			case t.ID == lastID:
				pb.Set(float64(t.Progress.Done))
			default:
				summary := wmx.fmtTaskSummary(chg, t)
				pb.Start(summary, float64(t.Progress.Total))
				lastID = t.ID
			}
			break
		}

		if chg.Ready {
			if chg.Status == "Error" {
				if chg.Err != "" {
					return chg, errors.New(chg.Err)
				}
				return chg, errors.New(i18n.G(`change finished in status "Error" with no error message`))
			}
			return chg, nil
		}

		if rebootingErr != nil {
			return nil, rebootingErr
		}

		// note this very purposely is not a ticker; we want
		// to sleep 100ms between calls, not call once every
		// 100ms.
		time.Sleep(pollTime)
	}
}

var (
	seenLines     = map[string]int{}
	summarisedIds = map[string]bool{}
)

var sortByReady = func(a, b *client.Task) int {
	if a.ReadyTime.IsZero() {
		return 1
	}
	if b.ReadyTime.IsZero() {
		return -1
	}
	return a.ReadyTime.Compare(b.ReadyTime)
}

func (wmx waitMixin) maybeShowLogs(pb progress.Meter, chg *client.Change) {
	if !wmx.verbose {
		return
	}

	tasks := slices.Clone(chg.Tasks)
	slices.SortFunc(tasks, sortByReady)

	esc := wmx.getEscapes()
	for _, t := range tasks {
		if t.Status == "Doing" || t.Status == "Done" || t.Status == "Error" {
			cur := seenLines[t.ID]

			for ; cur < len(t.Log); cur++ {
				pb.Notify(t.Log[cur])
			}

			seenLines[t.ID] = cur

			_, summarised := summarisedIds[t.ID]
			// We have shown all the task's logs and since it's in Done,
			// there'll be now new lines.
			if !summarised && t.Status == "Done" {
				pb.Notify(esc.green + esc.tick + esc.end + " " + t.Summary)
				summarisedIds[t.ID] = true
			}
		}
	}
}

func (wmx waitMixin) fmtTaskSummary(chg *client.Change, t *client.Task) string {
	countPrefix := ""
	if wmx.verbose {
		countPrefix = fmtFinishedCount(chg)
	}
	return countPrefix + t.Summary
}

func fmtFinishedCount(chg *client.Change) string {
	finished, total := 0, len(chg.Tasks)

	if chg.Status == "Doing" {
		for _, t := range chg.Tasks {
			if t.Status == "Done" {
				finished++
			}
		}
	}

	if chg.Status == "Undoing" {
		for _, t := range chg.Tasks {
			// Reverse counting back to 0 if undoing.
			if t.Status == "Undo" || t.Status == "Undoing" {
				finished++
			}
		}
	}

	return fmt.Sprintf("(%d/%d) ", finished, total)
}
