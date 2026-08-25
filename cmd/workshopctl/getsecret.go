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
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/canonical/workshop/client"
)

// systemdSecretRequest describes a decoded systemd LoadCredential request:
// which unit asked for which secret.
type systemdSecretRequest struct {
	// Unit is the name of the systemd unit requesting the secret
	// (e.g. "ollama.service").
	Unit string

	// SDK is the name of the SDK providing the secret (e.g. "ollama").
	SDK string

	// Secret is the name of the requested secret plug within the SDK
	// (e.g. "ollama-api-key").
	Secret string
}

const (
	// exitCodeProviderLocked indicates the secret provider is locked and
	// the secret cannot be accessed until it is unlocked.
	exitCodeProviderLocked = 2

	// exitCodeSecretError indicates the requested secret does not exist or
	// matches no entries in the secret provider.
	exitCodeSecretError = 1

	// exitCodeSecretSystemError indicates an internal infrastructure error,
	// such as the secret provider being unavailable.
	exitCodeSecretSystemError = 255

	// getSecretCommandName is the workshopctl subcommand name for
	// retrieving secrets.
	getSecretCommandName = "get-secret"
)

// decodeSystemdSecretRequest reads the peer address of a LoadCredential
// connection and decodes which unit requested which secret. Systemd connects
// with an abstract address of the form "\0<random>/unit/<unit>/<secret>".
func decodeSystemdSecretRequest(fd uintptr) (systemdSecretRequest, error) {
	sa, err := unix.Getpeername(int(fd))
	if err != nil {
		return systemdSecretRequest{}, fmt.Errorf("cannot decode secret request: %w", err)
	}

	addr, ok := sa.(*unix.SockaddrUnix)
	if !ok {
		return systemdSecretRequest{}, errors.New("peer is not a unix socket")
	}

	return parseSystemdPeerAddressName(addr.Name)
}

// interceptGetSecret handles local interception of the get-secret
// subcommand.
//
// With --systemd, workshopctl is invoked by the workshop-secret socket unit
// with the accepted connection on stdin. The request is decoded locally,
// then forwarded to the daemon as a regular get-secret invocation.
func interceptGetSecret(
	req workshopctlRequest,
	stdin fdReader,
	stdout, stderr io.Writer,
) (workshopctlRequest, error) {
	args := req.Args[1:]
	if len(args) != 1 || args[0] != "--systemd" {
		return req, nil
	}

	systemdSecretReq, err := decodeSystemdSecretRequest(stdin.Fd())
	if err != nil {
		return workshopctlRequest{}, err
	}

	fmt.Fprintf(
		stderr,
		"processed systemd load credential request for unit %q, %q SDK and secret %q",
		systemdSecretReq.Unit,
		systemdSecretReq.SDK,
		systemdSecretReq.Secret,
	)

	req.Args = []string{
		getSecretCommandName,
		systemdSecretReq.SDK + "." + systemdSecretReq.Secret,
	}
	// The connection was consumed locally to decode the request; it must
	// not be forwarded to the daemon as the invocation's stdin.
	req.Stdin = nil
	req.responseHandler = handleSystemdSecretResponse(systemdSecretReq, stdout, stderr)
	return req, nil
}

// handleSystemdSecretResponse returns the response handler for a get-secret
// request made on behalf of a systemd LoadCredential connection. The
// returned handler produces the process exit code, using the requested
// secret's identity in error messages.
func handleSystemdSecretResponse(
	secretReq systemdSecretRequest,
	stdout, stderr io.Writer,
) func([]byte, []byte, error) int {
	credential := secretReq.SDK + "." + secretReq.Secret

	return func(responseStdout, responseStderr []byte, err error) int {
		switch {
		case errors.Is(err, client.ErrorPlugNotConnected):
			// An unconnected plug is not an error: systemd receives a
			// zero-byte credential and the unit handles the missing value.
			return 0
		case errors.Is(err, client.ErrorSecretNotFound):
			fmt.Fprintf(
				stderr, "error: credential %q not found\n", credential)
			return exitCodeSecretError
		case errors.Is(err, client.ErrorSecretProviderLocked):
			fmt.Fprintf(
				stderr,
				"error: cannot get credential %q: unlock the secret provider and try again\n",
				credential,
			)
			return exitCodeProviderLocked
		case err != nil:
			fmt.Fprintf(
				stderr,
				"error: cannot get credential %q: %s\n",
				credential,
				err,
			)
			return exitCodeSecretSystemError
		}

		stdout.Write(responseStdout)
		return 0
	}
}

// parseSystemdPeerAddressName decodes a LoadCredential peer address name of
// the form "\0<random>/unit/<unit>/<sdk>.<secret>" into a
// systemdSecretRequest.
func parseSystemdPeerAddressName(addr string) (systemdSecretRequest, error) {
	// Strip the leading NUL of the abstract namespace and split off the
	// random component systemd prepends to the address.
	addr = strings.TrimPrefix(addr, "\x00")
	parts := strings.SplitN(addr, "/unit/", 2)
	if len(parts) != 2 {
		return systemdSecretRequest{}, errors.New(
			"malformed peer address missing \"/unit/\" delimiter")
	}

	unit, credentialName, ok := strings.Cut(parts[1], "/")
	if !ok {
		return systemdSecretRequest{}, errors.New(
			"unable to identify requesting unit and systemd credential name from peer address")
	} else if unit == "" {
		return systemdSecretRequest{}, errors.New(
			"unit name in systemd peer address cannot be empty")
	} else if credentialName == "" {
		return systemdSecretRequest{}, errors.New(
			"credential name in systemd peer address cannot be empty")
	}

	sdk, secret, ok := strings.Cut(credentialName, ".")
	if !ok {
		return systemdSecretRequest{}, fmt.Errorf(
			"unable to identify workshop SDK and secret name from systemd credential name %q",
			credentialName,
		)
	} else if sdk == "" {
		return systemdSecretRequest{}, errors.New(
			"workshop SDK in systemd credential name cannot be empty")
	} else if secret == "" {
		return systemdSecretRequest{}, errors.New(
			"workshop secret name in systemd credential name cannot be empty")
	}

	req := systemdSecretRequest{
		Unit:   unit,
		SDK:    sdk,
		Secret: secret,
	}
	return req, nil
}
