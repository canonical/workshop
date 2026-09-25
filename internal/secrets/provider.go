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

	"github.com/canonical/workshop/internal/sdk"
)

// Provider retrieves secrets from resolved provider slots.
type Provider interface {
	// Resolve retrieves the referenced slot's secret using the user
	// identity in [github.com/canonical/workshop/internal/workshop.ContextUser]
	// in the supplied context. It must honour context cancellation.
	// On success, ownership of the returned secret transfers to the caller,
	// which must consume or close it. On error, the returned value can be
	// discarded without reading or closing it; the provider is responsible
	// for clearing any secret material acquired before failure.
	//
	// The following errors may be expected:
	//   - [ErrorMultipleSecrets] when multiple secrets match the request and
	//     the provider cannot safely choose one.
	//   - [ErrorProviderLocked] when the backing secret store must be unlocked
	//     before the secret can be retrieved.
	//   - [ErrorSecretNotFound] when the requested secret does not exist or no
	//     entries match the request.
	//   - [ErrorUserNotFound] when the user requesting the secret does not
	//     exist.
	Resolve(context.Context, sdk.SlotRef) (Secret, error)
}
