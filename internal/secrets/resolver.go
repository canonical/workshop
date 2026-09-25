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

import (
	"context"
	"fmt"
	"maps"

	"github.com/canonical/workshop/internal/sdk"
)

// Resolver dispatches secret retrieval to providers keyed by SDK name.
type Resolver struct {
	providers map[string]Provider
}

// NewResolver creates a resolver with the supplied providers keyed by SDK name.
// It copies the map while retaining the supplied provider instances.
func NewResolver(providers map[string]Provider) Resolver {
	return Resolver{
		providers: maps.Clone(providers),
	}
}

// Resolve retrieves a secret using the provider registered for the slot's SDK.
// The caller must supply an already-resolved and authorised slot reference.
// On success, the caller must consume or close the returned secret. On error,
// the returned value can be discarded without reading or closing it.
//
// The following errors may be expected:
//   - [ErrorProviderNotFound]: no non-nil provider is registered for the
//     slot's SDK.
func (r Resolver) Resolve(
	ctx context.Context,
	slot sdk.SlotRef,
) (Secret, error) {
	err := ctx.Err()
	if err != nil {
		return Secret{}, err
	}

	provider, ok := r.providers[slot.Sdk]
	if !ok || provider == nil {
		return Secret{}, fmt.Errorf(
			"getting secret provider for sdk %q slot %q: %w",
			slot.Sdk, slot.Name, ErrorProviderNotFound,
		)
	}

	value, err := provider.Resolve(ctx, slot)
	if err != nil {

		return Secret{}, fmt.Errorf(
			"resolving secret through sdk %q: %w",
			slot.Sdk,
			err,
		)
	}
	return value, nil
}
