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

package secretstate

import (
	"context"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
)

// SecretResolver retrieves secrets from resolved and authorised provider slots.
type SecretResolver interface {
	// Resolve retrieves the slot's secret using the identity and
	// cancellation in the supplied context. On success, the caller must
	// consume or close the returned secret. On error, the returned value
	// can be discarded without reading or closing it.
	Resolve(context.Context, sdk.SlotRef) (secrets.Secret, error)
}
