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

package overlord

import (
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/secrets"
)

// makeSecretResolver builds the resolver with the built-in secret providers.
func makeSecretResolver(repo system.SlotRepository) secrets.Resolver {
	slots := system.NewRepositorySecretSlotLookup(repo)
	provider := system.NewSecretProvider(slots)

	return secrets.NewResolver(map[string]secrets.Provider{
		sdk.System.String(): provider,
	})
}
