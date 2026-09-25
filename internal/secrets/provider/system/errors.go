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

package system

// constError is a constant Secret Service error suitable for [errors.Is].
type constError string

const (
	// ErrorCollectionAmbiguous indicates that more than one collection has the
	// requested label.
	ErrorCollectionAmbiguous constError = "secret service collection is ambiguous"

	// ErrorCollectionLocked indicates that the requested collection is locked.
	ErrorCollectionLocked constError = "secret service collection is locked"

	// ErrorCollectionNotFound indicates that the requested collection does not
	// exist.
	ErrorCollectionNotFound constError = "secret service collection not found"

	// ErrorMultipleSecrets indicates that more than one secret matches the
	// requested attributes.
	ErrorMultipleSecrets constError = "multiple secrets match the request"

	// ErrorSecretNotFound indicates that no secret matches the requested
	// attributes.
	ErrorSecretNotFound constError = "secret not found"
)

// Error implements the error interface.
func (e constError) Error() string {
	return string(e)
}
