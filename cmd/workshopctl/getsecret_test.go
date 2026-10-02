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

// getSecretSuite tests secret request interception and systemd decoding.
type getSecretSuite struct{}

var _ = check.Suite(&getSecretSuite{})

// TestInterceptGetSecretAPIOutput checks that API metadata, not the domain
// kind, determines the exit code and output even for wrapped errors.
func (s *getSecretSuite) TestInterceptGetSecretAPIOutput(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)
	err = fmt.Errorf("lookup: %w", &client.Error{
		Kind:    client.ErrorKindSecretNotFound,
		Message: "fallback message",
		Value: map[string]any{
			"exit-code": float64(7),
			"stderr":    "API diagnostic\n",
			"stdout":    "API output\n",
		},
	})

	exitCode := req.responseHandler(nil, nil, err)

	c.Check(exitCode, check.Equals, 7)
	c.Check(stdout.String(), check.Equals, "API output\n")
	c.Check(stderr.String(), check.Equals, "API diagnostic\n")
}

// TestInterceptGetSecretDecodeFailure checks that failed local decoding
// retains the request and its secret fallback handler.
func (s *getSecretSuite) TestInterceptGetSecretDecodeFailure(c *check.C) {
	stdin, err := os.CreateTemp(c.MkDir(), "stdin")
	c.Assert(err, check.IsNil)
	defer stdin.Close()
	args := []string{"get-secret", "--systemd"}
	var stdout, stderr bytes.Buffer

	req, err := interceptArgs(args, stdin, &stdout, &stderr)

	c.Assert(err, check.ErrorMatches, "cannot decode secret request: .*")
	c.Check(req.Args, check.DeepEquals, args)
	c.Assert(req.responseHandler, check.NotNil)
	c.Check(req.responseHandler(nil, nil, err), check.Equals, 255)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: "+err.Error()+"\n")
}

// TestInterceptGetSecretInvalidExitCode checks that invalid API metadata
// uses the secret fallback instead of mapping the domain kind.
func (s *getSecretSuite) TestInterceptGetSecretInvalidExitCode(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)
	err = &client.Error{
		Kind:    client.ErrorKindSecretProviderLocked,
		Message: "provider locked",
		Value:   map[string]any{"exit-code": 0.5},
	}

	exitCode := req.responseHandler(nil, nil, err)

	c.Check(exitCode, check.Equals, 255)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: provider locked\n")
}

// TestInterceptGetSecretMissingExitCode checks that a domain error without
// API exit metadata uses the secret fallback rather than a local mapping.
func (s *getSecretSuite) TestInterceptGetSecretMissingExitCode(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)

	exitCode := req.responseHandler(nil, nil, client.ErrorPlugNotConnected)

	c.Check(exitCode, check.Equals, 255)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: plug not connected\n")
}

// TestInterceptGetSecretSuccess checks that ordinary requests retain their
// arguments and preserve both response streams on success.
func (s *getSecretSuite) TestInterceptGetSecretSuccess(c *check.C) {
	var stdout, stderr bytes.Buffer
	args := []string{"get-secret", "ollama.ollama-api-key"}
	req, err := interceptArgs(args, nil, &stdout, &stderr)
	c.Assert(err, check.IsNil)

	exitCode := req.responseHandler(
		[]byte("secret-value"),
		[]byte("diagnostic"),
		nil,
	)

	c.Check(exitCode, check.Equals, 0)
	c.Check(req.Args, check.DeepEquals, args)
	c.Check(stdout.String(), check.Equals, "secret-value")
	c.Check(stderr.String(), check.Equals, "diagnostic")
}

// TestInterceptGetSecretUnknownError checks that unknown failures use the
// secret fallback and retain their diagnostic.
func (s *getSecretSuite) TestInterceptGetSecretUnknownError(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)

	exitCode := req.responseHandler(nil, nil, errors.New("daemon unavailable"))

	c.Check(exitCode, check.Equals, 255)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: daemon unavailable\n")
}

// TestInterceptGetSecretZeroExitCode checks that explicit API success takes
// precedence over both the domain kind and the secret fallback.
func (s *getSecretSuite) TestInterceptGetSecretZeroExitCode(c *check.C) {
	var stdout, stderr bytes.Buffer
	req, err := interceptArgs(
		[]string{"get-secret", "ollama.ollama-api-key"},
		nil,
		&stdout,
		&stderr,
	)
	c.Assert(err, check.IsNil)
	err = &client.Error{
		Kind:    client.ErrorKindPlugNotConnected,
		Message: "plug not connected",
		Value: map[string]any{
			"exit-code": float64(0),
			"stderr":    "no credential available\n",
			"stdout":    "",
		},
	}

	exitCode := req.responseHandler(nil, nil, err)

	c.Check(exitCode, check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "no credential available\n")
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
