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
	"fmt"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
)

// SecretProvider returns a placeholder for system SDK secret slots until
// retrieval from the user's Secret Service over D-Bus is implemented.
type SecretProvider struct {
	slots SecretSlotLookup
}

// NewSecretProvider creates a system SDK secret provider using slots to read
// typed slot configuration.
func NewSecretProvider(slots SecretSlotLookup) SecretProvider {
	return SecretProvider{slots: slots}
}

// Resolve looks up slot configuration before returning a placeholder secret.
// The caller must consume or close the returned secret.
func (p SecretProvider) Resolve(
	ctx context.Context,
	slot sdk.SlotRef,
) (secrets.Secret, error) {
	err := ctx.Err()
	if err != nil {
		return secrets.Secret{}, err
	}

	// Require valid slot configuration even while retrieval is a placeholder.
	_, err = p.slots.Lookup(ctx, slot)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"looking up secret slot configuration: %w", err,
		)
	}
	return secrets.NewSecret([]byte("workshop-placeholder-secret")), nil
}
