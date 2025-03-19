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
}

var errNoWait = errors.New("no wait for op")
var errWaitOnError = errors.New("wait-on-error")

func (wmx waitMixin) wait(cli *client.Client, id string, mode progress.DisplayMode) (*client.Change, error) {
	if wmx.NoWait {
		fmt.Fprintf(Stdout, "%s\n", id)
		return nil, errNoWait
	}
	// Intercept sigint
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

	tMax := time.Time{}

	display := progress.NewDisplay(mode)
	defer display.Close()

	for {
		var rebootingErr error
		chg, err := cli.Change(id)
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
			display.Errorf("Waiting for server to restart")
			time.Sleep(pollTime)
			continue
		}
		if maintErr, ok := cli.Maintenance().(*client.Error); ok && maintErr.Kind == client.ErrorKindSystemRestart {
			rebootingErr = maintErr
		}
		if !tMax.IsZero() {
			display.Close()
			tMax = time.Time{}
		}

		_ = chg.Get("log", display.Buffer())

		// progress reporting
		for _, t := range chg.Tasks {
			switch {
			case t.Status == "Wait":
				display.Render(*t)
				return chg, errWaitOnError
			case t.Status != "Doing":
				continue
			default:
				display.Render(*t)
			}
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

		// ensure we write out all logs, and spin if not already
		display.Flush()

		if rebootingErr != nil {
			return nil, rebootingErr
		}

		// note this very purposely is not a ticker; we want
		// to sleep 100ms between calls, not call once every
		// 100ms.
		time.Sleep(pollTime)
	}
}
