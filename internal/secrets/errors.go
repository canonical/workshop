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

package secrets

// ConstError is a string-backed error suitable for constant declarations.
type ConstError string

const (
	// ErrorMultipleSecrets indicates that more than one secret matches the
	// request and the provider cannot safely choose one.
	ErrorMultipleSecrets = ConstError("multiple secrets match the request")

	// ErrorProviderLocked indicates that the backing secret store is locked
	// and must be unlocked before the secret can be retrieved.
	ErrorProviderLocked = ConstError("secret provider is locked")

	// ErrorProviderNotFound indicates that no non-nil secret provider is
	// registered for the requested SDK.
	ErrorProviderNotFound = ConstError("secret provider not found")

	// ErrorSecretNotFound indicates that the requested secret does not exist
	// or no entries match the request.
	ErrorSecretNotFound = ConstError("secret not found")

	// ErrorUserNotFound indicates that the user requesting a secret does not
	// exist.
	ErrorUserNotFound = ConstError("secret request user not found")
)

// Error implements the [error] interface and returns the error message.
func (c ConstError) Error() string {
	return string(c)
}
