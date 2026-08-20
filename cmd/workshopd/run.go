// Copyright (c) 2014-2020 Canonical Ltd
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
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/workshop/cmd/internal/cmdutil"
	"github.com/canonical/workshop/internal/daemon"
	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/syscheck"
	"github.com/canonical/workshop/internal/systemd"
	"github.com/canonical/workshop/internal/version"
)

var shortRunHelp = "Run the workshop daemon"
var longRunHelp = `
The run command workshop and starts accepting clients requests
`

type sharedRunEnterOpts struct {
	CreateDirs bool
	HTTP       string
	Verbose    bool
}

var sharedRunEnterOptsHelp = map[string]string{
	"create-dirs": "Create workshop directory on startup if it doesn't exist",
	"http":        `Start HTTP API listening on this address (e.g., ":4000")`,
	"verbose":     "Log all output from services to stdout",
}

type cmdRun struct {
	sharedRunEnterOpts
}

func (c *cmdRun) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "run",
		Args:  cobra.MaximumNArgs(0),
		Short: shortRunHelp,
		Long:  longRunHelp,
		RunE:  c.Run,
	}

	cmd.Flags().BoolVar(&c.CreateDirs, "create-dirs", false, sharedRunEnterOptsHelp["create-dirs"])
	cmd.Flags().StringVar(&c.HTTP, "http", "", sharedRunEnterOptsHelp["http"])
	cmd.Flags().BoolVar(&c.Verbose, "verbose", false, sharedRunEnterOptsHelp["verbose"])

	_ = cmd.RegisterFlagCompletionFunc("http", cmdutil.CompleteChoices())

	return cmd
}

func (c *cmdRun) Run(cmd *cobra.Command, av []string) error {
	c.run(nil)
	return nil
}

func (rcmd *cmdRun) run(ready chan<- func()) {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if err := runDaemon(rcmd, sigs, ready); err != nil {
		if err == daemon.ErrRestartSocket {
			// No "error: " prefix as this isn't an error.
			fmt.Fprintf(os.Stdout, "%v\n", err)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "cannot run workshop: %v\n", err)
		panic(&exitStatus{1})
	}
}

func runWatchdog(d *daemon.Daemon) (*time.Ticker, error) {
	if os.Getenv("WATCHDOG_USEC") == "" {
		// Not running under systemd.
		return nil, nil
	}
	usec, err := strconv.ParseFloat(os.Getenv("WATCHDOG_USEC"), 32)
	if usec == 0 || err != nil {
		return nil, fmt.Errorf("cannot parse WATCHDOG_USEC: %q", os.Getenv("WATCHDOG_USEC"))
	}
	dur := time.Duration(usec/2) * time.Microsecond
	logger.Debugf("Setting up sd_notify() watchdog timer every %s", dur)
	wt := time.NewTicker(dur)

	go func() {
		for {
			select {
			case <-wt.C:
				// TODO: poke the API here and only report WATCHDOG=1 if it
				//       replies with valid data.
				systemd.SdNotify("WATCHDOG=1")
			case <-d.Dying():
				return
			}
		}
	}()

	return wt, nil
}

var checkRunningConditionsRetryDelay = 5 * time.Second

func runDaemon(rcmd *cmdRun, ch chan os.Signal, ready chan<- func()) error {
	t0 := time.Now().Truncate(time.Millisecond)

	if rcmd.CreateDirs {
		if err := dirs.CreateDirs(); err != nil {
			return err
		}
	}
	dopts := daemon.Options{
		Dir:        dirs.DataDir,
		SocketPath: dirs.SocketPath,
	}

	dopts.HTTPAddress = rcmd.HTTP

	d, err := daemon.New(&dopts)
	if err != nil {
		return err
	}
	if err := d.Init(); err != nil {
		return err
	}

	// Run sanity check now, if anything goes wrong with the
	// check we go into "degraded" mode where we always report
	// the given error to any client.
	//
	// Keep the ticker running even when the initial check passes so the
	// daemon can detect and recover from LXD becoming unavailable after
	// startup (e.g. due to a refresh).
	if err := syscheck.CheckSystem(); err != nil {
		degradedErr := fmt.Errorf("system is not healthy: %w", err)
		logger.Noticef("%s", degradedErr)
		d.SetDegradedMode(degradedErr)
	}
	tic := time.NewTicker(checkRunningConditionsRetryDelay)
	defer tic.Stop()

	d.Version = version.Version
	if err = d.Start(); err != nil {
		return err
	}

	watchdog, err := runWatchdog(d)
	if err != nil {
		return fmt.Errorf("cannot run software watchdog: %v", err)
	}
	if watchdog != nil {
		defer watchdog.Stop()
	}

	logger.Debugf("activation done in %v", time.Now().Truncate(time.Millisecond).Sub(t0))

	var stop chan struct{}
	if ready != nil {
		stop = make(chan struct{}, 1)
		ready <- func() { close(stop) }
		close(ready)
	}

out:
	for {
		select {
		case sig := <-ch:
			logger.Noticef("Exiting on %s signal.\n", sig)
			break out
		case <-d.Dying():
			// something called Stop()
			logger.Noticef("Server exiting!")
			break out
		case <-tic.C:
			if err := syscheck.CheckSystem(); err != nil {
				degradedErr := fmt.Errorf("system is not healthy: %w", err)
				if !d.IsDegraded() {
					logger.Noticef("%s", degradedErr)
				}
				d.SetDegradedMode(degradedErr)
			} else {
				if d.IsDegraded() {
					logger.Noticef("System is healthy.")
				}
				d.SetDegradedMode(nil)
			}
		case <-stop:
			break out
		}
	}

	return d.Stop(ch)
}
