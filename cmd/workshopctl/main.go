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

// fdReader is an [io.Reader] that also exposes its underlying file
// descriptor, such as [*os.File].
type fdReader interface {
	io.Reader

	// Fd returns the file descriptor of the reader.
	Fd() uintptr
}

// workshopctlRequest is a request to the daemon's workshopctl endpoint,
// potentially altered by local interception of the invocation.
type workshopctlRequest struct {
	client.WorkshopCtlOptions

	// responseHandler, when set by an interceptor, is invoked with the
	// daemon's response to the request and returns the process exit code.
	responseHandler func([]byte, []byte, error) int
}

var clientConfig = client.Config{
	// we need the less privileged workshop socket in workshopctl, proxied to a
	// fixed path inside the workshop by the daemon (see dirs.WorkshopSocketPath)
	Socket: dirs.WorkshopSocketPath + ".untrusted",
}

// defaultResponseHandler returns a response handler implementing the
// standard workshopctl response behaviour: the response stdout and stderr
// are written to the given writers, daemon-reported exit codes are
// honoured, and any other error is reported with exit code 1.
func defaultResponseHandler(stdout, stderr io.Writer) func([]byte, []byte, error) int {
	return func(responseStdout, responseStderr []byte, err error) int {
		if err == nil {
			stdout.Write(responseStdout)
			stderr.Write(responseStderr)
			return 0
		}

		if e, ok := err.(*client.Error); ok && e.Kind == client.ErrorKindUnsuccessful {
			if errRes, ok := e.Value.(map[string]any); ok {
				if out, ok := errRes["stdout"].(string); ok {
					stdout.Write([]byte(out))
				}
				if errOut, ok := errRes["stderr"].(string); ok {
					stderr.Write([]byte(errOut))
				}
				if errCode, ok := errRes["exit-code"].(float64); ok {
					return int(errCode)
				}
			}
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
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

	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin fdReader, stdout, stderr io.Writer) int {
	req, err := interceptArgs(args, stdin, stdout, stderr)
	if err != nil {
		// The request may not have a handler if interception failed.
		return defaultResponseHandler(stdout, stderr)(nil, nil, err)
	}

	config := clientConfig
	config.RoundTripperWrapper = client.NewWorkshopInstanceIDRoundTripper(stderr)

	cli, err := client.New(&config)
	if err != nil {
		return req.responseHandler(nil, nil, err)
	}

	req.ContextID = os.Getenv("WORKSHOP_COOKIE")
	responseStdout, responseStderr, err := cli.RunWorkshopctl(
		&req.WorkshopCtlOptions,
	)
	return req.responseHandler(responseStdout, responseStderr, err)
}

// interceptArgs inspects the workshopctl invocation for subcommands that
// need local interception before being forwarded to the daemon, returning
// the request to forward. Subcommands with no local handling are forwarded
// unchanged.
func interceptArgs(
	args []string,
	stdin fdReader,
	stdout, stderr io.Writer,
) (workshopctlRequest, error) {
	req := workshopctlRequest{
		WorkshopCtlOptions: client.WorkshopCtlOptions{Args: args},
		responseHandler:    defaultResponseHandler(stdout, stderr),
	}

	if len(args) == 0 {
		return req, nil
	}

	switch args[0] {
	case getSecretCommandName:
		return interceptGetSecret(req, stdin, stdout, stderr)
	}

	return req, nil
}
