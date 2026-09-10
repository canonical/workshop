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

import (
	"context"

	"github.com/canonical/workshop/internal/secrets"
)

// SecretProvider returns a placeholder for system SDK secret slots until
// retrieval from the user's Secret Service over D-Bus is implemented.
type SecretProvider struct{}

// NewSecretProvider creates a system SDK secret provider.
func NewSecretProvider() SecretProvider {
	return SecretProvider{}
}

// Resolve implements [secrets.Provider] with a placeholder secret value.
// The source configuration is not used until Secret Service lookup is added.
//
// The following errors may be expected:
//   - [context.Canceled]: the request was cancelled.
//   - [context.DeadlineExceeded]: the request deadline expired.
func (SecretProvider) Resolve(
	ctx context.Context,
	_ secrets.Source,
) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	return []byte("workshop-placeholder-secret"), nil
}
