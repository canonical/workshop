// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2015 Canonical Ltd
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
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/waitready"
)

var clientConfig = client.Config{
	// we need the less privileged workshop socket in workshopctl, proxied to a
	// fixed path inside the workshop by the daemon (see dirs.WorkshopSocketPath)
	Socket: dirs.WorkshopSocketPath + ".untrusted",
}

func main() {
	if waitready.IsWaitreadyInvocation() {
		if err := waitready.WaitReady(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	// Set the user and group IDs to the workshop user
	uid := uint32(1000) // Change this to the workshop UID

	// Change the user IDs for this process
	if err := syscall.Setuid(int(uid)); err != nil {
		fmt.Println("Error setting UID:", err)
		return
	}

	stdout, stderr, err := run(nil)
	if err != nil {
		if e, ok := err.(*client.Error); ok {
			switch e.Kind {
			case client.ErrorKindUnsuccessful:
				if errRes, ok := e.Value.(map[string]any); ok {
					if stdout, ok := errRes["stdout"].(string); ok {
						os.Stdout.Write([]byte(stdout))
					}
					if stderr, ok := errRes["stderr"].(string); ok {
						os.Stderr.Write([]byte(stderr))
					}
					if errCode, ok := errRes["exit-code"].(float64); ok {
						os.Exit(int(errCode))
					}
				}
			}
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if stdout != nil {
		os.Stdout.Write(stdout)
	}

	if stderr != nil {
		os.Stderr.Write(stderr)
	}
}

func run(stdin io.Reader) (stdout, stderr []byte, err error) {
	config := clientConfig
	config.RoundTripperWrapper = client.NewWorkshopInstanceIDRoundTripper(os.Stderr)

	cli, err := client.New(&config)
	if err != nil {
		return nil, nil, err
	}

	cookie := os.Getenv("WORKSHOP_COOKIE")
	return cli.RunWorkshopctl(&client.WorkshopCtlOptions{
		ContextID: cookie,
		Args:      os.Args[1:],
	}, stdin)
}
