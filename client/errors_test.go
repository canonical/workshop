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

package client_test

import (
	"errors"
	"fmt"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

// errorSuite checks structured API error conversion behaviour.
type errorSuite struct{}

var _ = check.Suite(&errorSuite{})

// TestChangeConflictErrorAsFullValue checks that a complete API error value
// maps to [client.ChangeConflictError].
func (errorSuite) TestChangeConflictErrorAsFullValue(c *check.C) {
	err := &client.Error{
		Kind: client.ErrorKindChangeConflict,
		Value: map[string]any{
			"change-id":   "29",
			"change-kind": "refresh",
			"project-id":  "project-1",
			"workshop":    "dev",
		},
	}

	var conflictErr client.ChangeConflictError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, client.ChangeConflictError{
		ChangeID:   "29",
		ChangeKind: "refresh",
		ProjectID:  "project-1",
		Workshop:   "dev",
	})
}

// TestChangeConflictErrorAsNonMapValue checks that unexpected API error value
// shapes do not map to [client.ChangeConflictError].
func (errorSuite) TestChangeConflictErrorAsNonMapValue(c *check.C) {
	err := &client.Error{
		Kind:  client.ErrorKindChangeConflict,
		Value: "not a map",
	}

	var conflictErr client.ChangeConflictError
	c.Check(errors.As(err, &conflictErr), check.Equals, false)
}

// TestChangeConflictErrorAsPartialValue checks that incomplete API error values
// still map to a partial [client.ChangeConflictError].
func (errorSuite) TestChangeConflictErrorAsPartialValue(c *check.C) {
	err := &client.Error{
		Kind: client.ErrorKindChangeConflict,
		Value: map[string]any{
			"change-id": "29",
			"workshop":  "dev",
		},
	}

	var conflictErr client.ChangeConflictError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, client.ChangeConflictError{
		ChangeID: "29",
		Workshop: "dev",
	})
}

// TestChangeConflictErrorAsWrongKind checks that unrelated API error kinds do
// not map to [client.ChangeConflictError].
func (errorSuite) TestChangeConflictErrorAsWrongKind(c *check.C) {
	err := &client.Error{
		Kind: client.ErrorKindNoUpdatesAvailable,
		Value: map[string]any{
			"change-id": "29",
		},
	}

	var conflictErr client.ChangeConflictError
	c.Check(errors.As(err, &conflictErr), check.Equals, false)
}

// TestErrorExitCode checks that a JSON-decoded exit code is returned as an
// integer, including the largest supported process exit code.
func (errorSuite) TestErrorExitCode(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": float64(255)}}

	code, ok := err.ExitCode()

	c.Check(code, check.Equals, 255)
	c.Check(ok, check.Equals, true)
}

// TestErrorExitCodeFractional checks that fractional codes are rejected
// rather than truncated into a different exit status.
func (errorSuite) TestErrorExitCodeFractional(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": 0.5}}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeMissing checks that an absent code is not success.
func (errorSuite) TestErrorExitCodeMissing(c *check.C) {
	err := &client.Error{Value: map[string]any{"stderr": "command failed"}}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeNegative checks that negative process codes are rejected.
func (errorSuite) TestErrorExitCodeNegative(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": float64(-1)}}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeNonObject checks that a non-object value has no exit code.
func (errorSuite) TestErrorExitCodeNonObject(c *check.C) {
	err := &client.Error{Value: "not an object"}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeTooLarge checks that a code cannot overflow the process
// exit status range.
func (errorSuite) TestErrorExitCodeTooLarge(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": float64(256)}}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeWrongType checks that a string is not parsed as a code.
func (errorSuite) TestErrorExitCodeWrongType(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": "1"}}

	_, ok := err.ExitCode()

	c.Check(ok, check.Equals, false)
}

// TestErrorExitCodeZero checks that explicit success is a present exit code.
func (errorSuite) TestErrorExitCodeZero(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": float64(0)}}

	code, ok := err.ExitCode()

	c.Check(code, check.Equals, 0)
	c.Check(ok, check.Equals, true)
}

// TestErrorOutput checks that the accessors return their respective output
// fields without changing their contents.
func (errorSuite) TestErrorOutput(c *check.C) {
	err := &client.Error{Value: map[string]any{
		"stderr": "command diagnostic\n",
		"stdout": "command output\n",
	}}

	stderr, hasStderr := err.Stderr()
	stdout, hasStdout := err.Stdout()

	c.Check(stderr, check.Equals, "command diagnostic\n")
	c.Check(hasStderr, check.Equals, true)
	c.Check(stdout, check.Equals, "command output\n")
	c.Check(hasStdout, check.Equals, true)
}

// TestErrorOutputEmpty checks that explicitly empty output is still present.
func (errorSuite) TestErrorOutputEmpty(c *check.C) {
	err := &client.Error{Value: map[string]any{"stderr": "", "stdout": ""}}

	stderr, hasStderr := err.Stderr()
	stdout, hasStdout := err.Stdout()

	c.Check(stderr, check.Equals, "")
	c.Check(hasStderr, check.Equals, true)
	c.Check(stdout, check.Equals, "")
	c.Check(hasStdout, check.Equals, true)
}

// TestErrorOutputMissing checks that absent output fields are not reported
// as explicitly empty output.
func (errorSuite) TestErrorOutputMissing(c *check.C) {
	err := &client.Error{Value: map[string]any{"exit-code": float64(1)}}

	stderr, hasStderr := err.Stderr()
	stdout, hasStdout := err.Stdout()

	c.Check(stderr, check.Equals, "")
	c.Check(hasStderr, check.Equals, false)
	c.Check(stdout, check.Equals, "")
	c.Check(hasStdout, check.Equals, false)
}

// TestErrorOutputNonObject checks that a non-object value has no output.
func (errorSuite) TestErrorOutputNonObject(c *check.C) {
	err := &client.Error{Value: "not an object"}

	stderr, hasStderr := err.Stderr()
	stdout, hasStdout := err.Stdout()

	c.Check(stderr, check.Equals, "")
	c.Check(hasStderr, check.Equals, false)
	c.Check(stdout, check.Equals, "")
	c.Check(hasStdout, check.Equals, false)
}

// TestErrorOutputWrongType checks that non-string fields are not treated as
// output or converted to strings.
func (errorSuite) TestErrorOutputWrongType(c *check.C) {
	err := &client.Error{Value: map[string]any{
		"stderr": float64(1),
		"stdout": true,
	}}

	stderr, hasStderr := err.Stderr()
	stdout, hasStdout := err.Stdout()

	c.Check(stderr, check.Equals, "")
	c.Check(hasStderr, check.Equals, false)
	c.Check(stdout, check.Equals, "")
	c.Check(hasStdout, check.Equals, false)
}

// TestPlugNotConnectedErrorIs checks that an unconnected-plug API error
// matches its sentinel even when wrapped with diagnostic context.
func (errorSuite) TestPlugNotConnectedErrorIs(c *check.C) {
	err := &client.Error{Kind: client.ErrorKindPlugNotConnected}

	c.Check(errors.Is(err, client.ErrorPlugNotConnected), check.Equals, true)
	wrapped := fmt.Errorf("request failed: %w", err)
	c.Check(errors.Is(wrapped, client.ErrorPlugNotConnected), check.Equals, true)
}

// TestSecretMultipleMatchesErrorIs checks that an ambiguous lookup API
// error matches its sentinel, including through wrapping.
func (errorSuite) TestSecretMultipleMatchesErrorIs(c *check.C) {
	err := &client.Error{Kind: client.ErrorKindSecretMultipleMatches}

	c.Check(errors.Is(err, client.ErrorSecretMultipleMatches), check.Equals, true)
	wrapped := fmt.Errorf("request failed: %w", err)
	c.Check(errors.Is(wrapped, client.ErrorSecretMultipleMatches),
		check.Equals, true)
}

// TestSecretNotFoundErrorIs checks that a missing-secret API error matches
// its sentinel, including through wrapping.
func (errorSuite) TestSecretNotFoundErrorIs(c *check.C) {
	err := &client.Error{Kind: client.ErrorKindSecretNotFound}

	c.Check(errors.Is(err, client.ErrorSecretNotFound), check.Equals, true)
	wrapped := fmt.Errorf("request failed: %w", err)
	c.Check(errors.Is(wrapped, client.ErrorSecretNotFound), check.Equals, true)
}

// TestSecretProviderLockedErrorIs checks that a locked-provider API error
// matches its sentinel, including through wrapping.
func (errorSuite) TestSecretProviderLockedErrorIs(c *check.C) {
	err := &client.Error{Kind: client.ErrorKindSecretProviderLocked}

	c.Check(errors.Is(err, client.ErrorSecretProviderLocked), check.Equals, true)
	wrapped := fmt.Errorf("request failed: %w", err)
	c.Check(errors.Is(wrapped, client.ErrorSecretProviderLocked),
		check.Equals, true)
}

// TestWaitingChangeErrorIs checks that a no-waiting-change API error matches
// the [client.ErrorNoWaitingChange] sentinel via errors.Is.
func (errorSuite) TestWaitingChangeErrorIs(c *check.C) {
	err := &client.Error{
		Kind: client.ErrorKindNoWaitingChange,
	}

	c.Check(errors.Is(err, client.ErrorNoWaitingChange), check.Equals, true)
}

// TestWaitingChangeErrorIsWrongKind checks that unrelated API error kinds do
// not match the [client.ErrorNoWaitingChange] sentinel.
func (errorSuite) TestWaitingChangeErrorIsWrongKind(c *check.C) {
	err := &client.Error{
		Kind: client.ErrorKindChangeConflict,
	}

	c.Check(errors.Is(err, client.ErrorNoWaitingChange), check.Equals, false)
}
