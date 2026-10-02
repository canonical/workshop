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

package client

import (
	"fmt"
	"math"
)

// ChangeConflictError describes an operation blocked by another change.
type ChangeConflictError struct {
	// ChangeID is the ID of the blocking change.
	ChangeID string

	// ChangeKind is the kind of the blocking change, such as "refresh".
	ChangeKind string

	// ProjectID is the ID of the project containing the blocked workshop.
	ProjectID string

	// Workshop is the name of the blocked workshop.
	Workshop string
}

// ConstError is a string-based error type for declaring sentinel errors as
// constants. Unlike sentinels created with [errors.New], a ConstError
// cannot be reassigned and is matched by [errors.Is] through string
// equality, so each ConstError message must be unique.
type ConstError string

const (
	// ErrorNoWaitingChange signals that an abort or continue request could
	// not be applied because no change is in progress to resume for the
	// workshop. Match it with [errors.Is].
	ErrorNoWaitingChange = ConstError("no waiting change in progress")

	// ErrorPlugNotConnected signals that the requested plug has no
	// connection and therefore has no value to supply. Match it with
	// [errors.Is].
	ErrorPlugNotConnected = ConstError("plug not connected")

	// ErrorSecretMultipleMatches signals that a lookup matches more than one
	// secret and cannot safely select a value. Match it with [errors.Is].
	ErrorSecretMultipleMatches = ConstError("multiple secrets match the request")

	// ErrorSecretNotFound signals that a requested secret does not exist or
	// matches no entries in the secret provider. Match it with [errors.Is].
	ErrorSecretNotFound = ConstError("secret not found")

	// ErrorSecretProviderLocked signals that a secret cannot be accessed
	// because the secret provider is locked. Match it with [errors.Is].
	ErrorSecretProviderLocked = ConstError("secret provider locked")
)

// As maps generic API errors into richer client-side error types.
func (e *Error) As(target any) bool {
	switch e.Kind {
	case ErrorKindChangeConflict:
		conflict, ok := target.(*ChangeConflictError)
		if !ok {
			return false
		}
		return toChangeConflictError(*e, conflict)
	default:
		return false
	}
}

// Error returns the error message, implementing the error interface.
func (c ConstError) Error() string {
	return string(c)
}

// Error returns a human-readable description of the blocking change.
func (e ChangeConflictError) Error() string {
	if e.ChangeKind != "" {
		return fmt.Sprintf(
			"workshop %q has %q change in progress",
			e.Workshop,
			e.ChangeKind,
		)
	}
	return fmt.Sprintf("workshop %q has changes in progress", e.Workshop)
}

// ExitCode returns the exit code from the error's value and whether it is
// valid. The field must be a JSON-decoded number representing an integer
// between 0 and 255 inclusive. Missing fields, other types and invalid
// values return zero and false.
func (e *Error) ExitCode() (int, bool) {
	value, ok := e.Value.(map[string]any)
	if !ok {
		return 0, false
	}
	code, ok := value["exit-code"].(float64)
	if !ok || code < 0 || code > 255 || math.Trunc(code) != code {
		return 0, false
	}
	return int(code), true
}

// Is reports whether the error matches a sentinel for the error's kind.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrorNoWaitingChange:
		return e.Kind == ErrorKindNoWaitingChange
	case ErrorPlugNotConnected:
		return e.Kind == ErrorKindPlugNotConnected
	case ErrorSecretMultipleMatches:
		return e.Kind == ErrorKindSecretMultipleMatches
	case ErrorSecretNotFound:
		return e.Kind == ErrorKindSecretNotFound
	case ErrorSecretProviderLocked:
		return e.Kind == ErrorKindSecretProviderLocked
	default:
		return false
	}
}

// Stderr returns the stderr string from the error's value and whether it
// is present. An empty string is present; a missing or non-string field,
// or a value that is not an object, returns an empty string and false.
func (e *Error) Stderr() (string, bool) {
	value, ok := e.Value.(map[string]any)
	if !ok {
		return "", false
	}
	output, ok := value["stderr"].(string)
	return output, ok
}

// Stdout returns the stdout string from the error's value and whether it
// is present. An empty string is present; a missing or non-string field,
// or a value that is not an object, returns an empty string and false.
func (e *Error) Stdout() (string, bool) {
	value, ok := e.Value.(map[string]any)
	if !ok {
		return "", false
	}
	output, ok := value["stdout"].(string)
	return output, ok
}

// toChangeConflictError extracts change-conflict details from a generic API
// error. It returns true when the error value has the expected object shape,
// even if some individual fields are missing or not strings; in that case the
// corresponding [ChangeConflictError] fields remain empty. It returns false
// when the error value is not an object and therefore cannot represent a
// change conflict payload.
func toChangeConflictError(err Error, conflict *ChangeConflictError) bool {
	value, ok := err.Value.(map[string]any)
	if !ok {
		return false
	}

	changeID, _ := value["change-id"].(string)
	changeKind, _ := value["change-kind"].(string)
	projectID, _ := value["project-id"].(string)
	workshop, _ := value["workshop"].(string)

	*conflict = ChangeConflictError{
		ChangeID:   changeID,
		ChangeKind: changeKind,
		ProjectID:  projectID,
		Workshop:   workshop,
	}
	return true
}
