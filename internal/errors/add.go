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

package errors

import stderrors "errors"

// addedError exposes an additional cause without including its message.
type addedError struct {
	added error
	err   error
}

// Add attaches add to err without changing its displayed message. Both err
// and add participate in [stderrors.Is] and [stderrors.As] matching. The
// added error is searched before unwrapping err. If err is nil, Add returns
// nil. A nil added error does not affect matching.
//
// Unwrapping the returned error with [stderrors.Unwrap] yields only err,
// not add. The added error's chain is searched only during matching with
// [stderrors.Is] or [stderrors.As].
func Add(err, add error) error {
	if err == nil {
		return nil
	}
	return addedError{added: add, err: err}
}

// As searches the added error for the requested type.
func (e addedError) As(target any) bool {
	return stderrors.As(e.added, target)
}

// Error returns only the primary error's message.
func (e addedError) Error() string {
	return e.err.Error()
}

// Is reports whether the added error matches target.
func (e addedError) Is(target error) bool {
	return stderrors.Is(e.added, target)
}

// Unwrap returns the primary error without exposing the added error.
func (e addedError) Unwrap() error {
	return e.err
}
