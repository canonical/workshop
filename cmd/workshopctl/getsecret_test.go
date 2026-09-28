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
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

// getSecretSuite tests decoding of systemd LoadCredential secret requests.
type getSecretSuite struct{}

var _ = check.Suite(&getSecretSuite{})

// TestHandleSecretResponse checks that ordinary requests preserve stdout
// and stderr on success.
func (s *getSecretSuite) TestHandleSecretResponse(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := handleSecretResponse(&stdout, &stderr)

	exitCode := handler([]byte("secret-value"), []byte("diagnostic"), nil)

	c.Check(exitCode, check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "secret-value")
	c.Check(stderr.String(), check.Equals, "diagnostic")
}

// TestHandleSecretResponseError checks that other failures retain the
// default diagnostic and exit code 1.
func (s *getSecretSuite) TestHandleSecretResponseError(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := handleSecretResponse(&stdout, &stderr)

	exitCode := handler(nil, nil, errors.New("daemon unavailable"))

	c.Check(exitCode, check.Equals, 1)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: daemon unavailable\n")
}

// TestHandleSecretResponsePlugNotConnected checks that an unconnected plug
// returns exit code 3 without emitting a secret.
func (s *getSecretSuite) TestHandleSecretResponsePlugNotConnected(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := handleSecretResponse(&stdout, &stderr)
	err := &client.Error{
		Kind:    client.ErrorKindPlugNotConnected,
		Message: "plug not connected",
	}

	exitCode := handler(nil, nil, err)

	c.Check(exitCode, check.Equals, 3)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: plug not connected\n")
}

// TestHandleSecretResponseSecretNotFound checks that a missing secret
// returns exit code 2, including when the API error is wrapped.
func (s *getSecretSuite) TestHandleSecretResponseSecretNotFound(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := handleSecretResponse(&stdout, &stderr)
	err := fmt.Errorf("lookup: %w", &client.Error{
		Kind:    client.ErrorKindSecretNotFound,
		Message: "secret not found",
	})

	exitCode := handler(nil, nil, err)

	c.Check(exitCode, check.Equals, 2)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: lookup: secret not found\n")
}

// TestInterceptGetSecretInstallsHandler checks that ordinary get-secret
// requests use the secret-specific exit codes rather than the default ones.
func (s *getSecretSuite) TestInterceptGetSecretInstallsHandler(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)

	exitCode := req.responseHandler(nil, nil, client.ErrorSecretNotFound)

	c.Check(exitCode, check.Equals, 2)
	c.Check(req.Args, check.DeepEquals,
		[]string{"get-secret", "ollama.ollama-api-key"})
}

// TestHandleSystemdSecretResponse checks that the response handler delivers
// the secret value on stdout.
func (s *getSecretSuite) TestHandleSystemdSecretResponse(c *check.C) {
	secretReq := systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	}

	var stdout, stderr bytes.Buffer
	handler := handleSystemdSecretResponse(secretReq, &stdout, &stderr)

	exitCode := handler([]byte("secret-value"), nil, nil)
	c.Check(exitCode, check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "secret-value")
	c.Check(stderr.String(), check.Equals, "")
}

// TestHandleSystemdSecretResponseMultipleMatches checks that an ambiguous
// lookup returns exit code 1 and a diagnostic without emitting a credential.
func (s *getSecretSuite) TestHandleSystemdSecretResponseMultipleMatches(
	c *check.C,
) {
	secretReq := systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	}
	var stdout, stderr bytes.Buffer
	handler := handleSystemdSecretResponse(secretReq, &stdout, &stderr)
	err := &client.Error{
		Kind:    client.ErrorKindSecretMultipleMatches,
		Message: "multiple secrets match the request",
	}

	exitCode := handler(nil, nil, err)

	c.Check(exitCode, check.Equals, 1)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals,
		"error: multiple secrets match credential \"ollama.ollama-api-key\"\n")
}

// TestHandleSystemdSecretResponsePlugNotConnected checks that an unconnected
// plug yields a zero-byte credential and exit code 0.
func (s *getSecretSuite) TestHandleSystemdSecretResponsePlugNotConnected(c *check.C) {
	secretReq := systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	}

	var stdout, stderr bytes.Buffer
	handler := handleSystemdSecretResponse(secretReq, &stdout, &stderr)

	exitCode := handler(nil, nil, client.ErrorPlugNotConnected)
	c.Check(exitCode, check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "")
}

// TestHandleSystemdSecretResponseError checks that the response handler maps
// daemon errors to the exit codes defined by the secrets spec.
func (s *getSecretSuite) TestHandleSystemdSecretResponseError(c *check.C) {
	secretReq := systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	}

	table := []struct {
		err      error
		exitCode int
		stderr   string
	}{
		{
			err:      client.ErrorSecretNotFound,
			exitCode: 1,
			stderr:   "error: credential \"ollama.ollama-api-key\" not found\n",
		}, {
			err:      client.ErrorSecretProviderLocked,
			exitCode: 2,
			stderr:   "error: cannot get credential \"ollama.ollama-api-key\": unlock the secret provider and try again\n",
		}, {
			err:      errors.New("daemon unavailable"),
			exitCode: 255,
			stderr:   "error: cannot get credential \"ollama.ollama-api-key\": daemon unavailable\n",
		},
	}

	for _, t := range table {
		var stdout, stderr bytes.Buffer
		handler := handleSystemdSecretResponse(secretReq, &stdout, &stderr)

		exitCode := handler(nil, nil, t.err)
		c.Check(exitCode, check.Equals, t.exitCode, check.Commentf("err %v", t.err))
		c.Check(stdout.String(), check.Equals, "", check.Commentf("err %v", t.err))
		c.Check(stderr.String(), check.Equals, t.stderr, check.Commentf("err %v", t.err))
	}
}

// TestParseSystemdPeerAddressName checks that a valid LoadCredential peer
// address name is decoded into the requesting unit, sdk and secret.
func (s *getSecretSuite) TestParseSystemdPeerAddressName(c *check.C) {
	req, err := parseSystemdPeerAddressName(
		"\x00DEADBEEF/unit/ollama.service/ollama.ollama-api-key")
	c.Assert(err, check.IsNil)
	c.Check(req, check.DeepEquals, systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	})
}

// TestParseSystemdPeerAddressNameInvalid checks that malformed LoadCredential
// peer address names are rejected with a descriptive error and no request.
func (s *getSecretSuite) TestParseSystemdPeerAddressNameInvalid(c *check.C) {
	tests := []struct {
		addr string
		err  string
	}{
		{
			addr: "ollama.service/ollama.ollama-api-key",
			err:  `malformed peer address missing "/unit/" delimiter`,
		}, {
			addr: "\x00DEADBEEF/unit/ollama.service",
			err:  `unable to identify requesting unit and systemd credential name from peer address`,
		}, {
			addr: "\x00DEADBEEF/unit//ollama.ollama-api-key",
			err:  `unit name in systemd peer address cannot be empty`,
		}, {
			addr: "\x00DEADBEEF/unit/ollama.service/",
			err:  `credential name in systemd peer address cannot be empty`,
		}, {
			addr: "\x00DEADBEEF/unit/ollama.service/ollama-api-key",
			err:  `unable to identify workshop SDK and secret name from systemd credential name "ollama-api-key"`,
		}, {
			addr: "\x00DEADBEEF/unit/ollama.service/.ollama-api-key",
			err:  `workshop SDK in systemd credential name cannot be empty`,
		}, {
			addr: "\x00DEADBEEF/unit/ollama.service/ollama.",
			err:  `workshop secret name in systemd credential name cannot be empty`,
		},
	}

	for _, t := range tests {
		req, err := parseSystemdPeerAddressName(t.addr)
		c.Check(err, check.ErrorMatches, t.err, check.Commentf("addr %q", t.addr))
		c.Check(req, check.DeepEquals, systemdSecretRequest{}, check.Commentf("addr %q", t.addr))
	}
}

// TestDecodeSystemdSecretRequest checks that the requesting unit and secret
// are decoded from the peer address of a real socket connection, mirroring
// how systemd connects for LoadCredential.
func (s *getSecretSuite) TestDecodeSystemdSecretRequest(c *check.C) {
	addr := "\x00" + fmt.Sprintf("%d", os.Getpid()) + "/unit/ollama.service/ollama.ollama-api-key"
	name := filepath.Join(c.MkDir(), "conn")

	server, err := net.ListenUnix("unix", &net.UnixAddr{Name: name})
	c.Assert(err, check.IsNil)
	defer server.Close()

	type acceptResult struct {
		conn *net.UnixConn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := server.AcceptUnix()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	client, err := net.DialUnix("unix",
		&net.UnixAddr{Name: addr},
		&net.UnixAddr{Name: name},
	)
	c.Assert(err, check.IsNil)
	defer client.Close()

	result := <-accepted
	c.Assert(result.err, check.IsNil)
	defer result.conn.Close()

	f, err := result.conn.File()
	c.Assert(err, check.IsNil)
	defer f.Close()

	req, err := decodeSystemdSecretRequest(f.Fd())
	c.Check(err, check.IsNil)
	c.Check(req, check.DeepEquals, systemdSecretRequest{
		Unit:   "ollama.service",
		SDK:    "ollama",
		Secret: "ollama-api-key",
	})
}
