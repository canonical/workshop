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

// ErrorNoWaitingChange signals that an abort or continue request could not be
// applied because no change is in progress to resume for the workshop. Match
// it with [errors.Is].
const ErrorNoWaitingChange = ConstError("no waiting change in progress")

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

// Is reports whether the error matches a sentinel for the error's kind.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrorNoWaitingChange:
		return e.Kind == ErrorKindNoWaitingChange
	default:
		return false
	}
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
