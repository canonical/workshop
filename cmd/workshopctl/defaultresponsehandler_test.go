// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"errors"
	"fmt"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

// defaultResponseHandlerSuite checks default response presentation without
// API requests.
type defaultResponseHandlerSuite struct{}

var _ = check.Suite(&defaultResponseHandlerSuite{})

// TestAPIOutput checks that wrapped API errors of any kind can supply output
// and an exit code without duplicating the diagnostic.
func (defaultResponseHandlerSuite) TestAPIOutput(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)
	err := fmt.Errorf("request: %w", &client.Error{
		Kind:    client.ErrorKindSecretNotFound,
		Message: "fallback message",
		Value: map[string]any{
			"exit-code": float64(2),
			"stderr":    "command diagnostic\n",
			"stdout":    "command output\n",
		},
	})

	code := handler(nil, nil, err)

	c.Check(code, check.Equals, 2)
	c.Check(stdout.String(), check.Equals, "command output\n")
	c.Check(stderr.String(), check.Equals, "command diagnostic\n")
}

// TestConfigurableFallback checks that unknown errors use the supplied code.
func (defaultResponseHandlerSuite) TestConfigurableFallback(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 42)

	code := handler(nil, nil, errors.New("daemon unavailable"))

	c.Check(code, check.Equals, 42)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: daemon unavailable\n")
}

// TestEmptyStderr checks that explicitly empty stderr suppresses fallback
// diagnostics, while an absent exit code defaults to failure.
func (defaultResponseHandlerSuite) TestEmptyStderr(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)
	err := &client.Error{
		Message: "fallback message",
		Value:   map[string]any{"stderr": ""},
	}

	code := handler(nil, nil, err)

	c.Check(code, check.Equals, 1)
	c.Check(stderr.String(), check.Equals, "")
}

// TestInvalidExitCode checks that fractional exit codes cannot become
// success through integer truncation.
func (defaultResponseHandlerSuite) TestInvalidExitCode(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)
	err := &client.Error{
		Message: "command failed",
		Value:   map[string]any{"exit-code": 0.5},
	}

	code := handler(nil, nil, err)

	c.Check(code, check.Equals, 1)
	c.Check(stderr.String(), check.Equals, "error: command failed\n")
}

// TestMissingOutput checks that API errors without a payload remain visible
// and return the default failure exit code.
func (defaultResponseHandlerSuite) TestMissingOutput(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)
	err := &client.Error{Message: "command failed"}

	code := handler(nil, nil, err)

	c.Check(code, check.Equals, 1)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: command failed\n")
}

// TestSuccess checks that successful responses preserve both output streams.
func (defaultResponseHandlerSuite) TestSuccess(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 255)

	code := handler([]byte("command output"), []byte("diagnostic"), nil)

	c.Check(code, check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "command output")
	c.Check(stderr.String(), check.Equals, "diagnostic")
}

// TestUnknownError checks that ordinary failures retain exit code 1.
func (defaultResponseHandlerSuite) TestUnknownError(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)

	code := handler(nil, nil, errors.New("daemon unavailable"))

	c.Check(code, check.Equals, 1)
	c.Check(stdout.String(), check.Equals, "")
	c.Check(stderr.String(), check.Equals, "error: daemon unavailable\n")
}

// TestZeroExitCode checks that an API error can explicitly request success.
func (defaultResponseHandlerSuite) TestZeroExitCode(c *check.C) {
	var stdout, stderr bytes.Buffer
	handler := defaultResponseHandler(&stdout, &stderr, 1)
	err := &client.Error{
		Kind:    client.ErrorKindPlugNotConnected,
		Message: "plug not connected",
		Value:   map[string]any{"exit-code": float64(0)},
	}

	code := handler(nil, nil, err)

	c.Check(code, check.Equals, 0)
	c.Check(stderr.String(), check.Equals, "error: plug not connected\n")
}
