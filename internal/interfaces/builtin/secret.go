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

package builtin

import (
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
)

// secretInterface is the built-in interface for routing secrets from
// the user's host keyring to SDKs. Plugs declare an SDK's need for a
// secret; slots on the system SDK describe where the secret is found
// in the host keyring. The interface carries no backend hooks: secret
// values are resolved at request time via the workshop secret socket
// and never configured into the container.
type secretInterface struct{}

// secretBaseDeclarationPlugs extends the base declaration with the plug
// side policy for the secret interface: only regular SDKs may declare
// secret plugs, and while users may connect them freely, they are never
// auto-connected — a secret must always be attached by an explicit
// user decision.
const secretBaseDeclarationPlugs = `
  secret:
    allow-installation:
      plug-sdk-type:
        - regular
    allow-connection: true
    allow-auto-connection: false
`

// secretBaseDeclarationSlots extends the base declaration with the slot
// side policy for the secret interface: only the system SDK may declare
// secret slots, as slots carry the host keyring lookup details for a
// workshop. Like the plug side, connections are user-initiated only and
// never auto-connected.
const secretBaseDeclarationSlots = `
  secret:
    allow-installation:
      slot-sdk-type:
        - system
    allow-connection: true
    allow-auto-connection: false
`

const secretSummary = `allows SDKs to consume secrets from the host keyring`

// AutoConnect implements [interfaces.Interface]. It returns true to
// defer entirely to the base declaration policy, which denies
// auto-connection for secrets: a secret must always be attached by an
// explicit user decision.
func (secretInterface) AutoConnect(_ *sdk.PlugInfo, _ *sdk.SlotInfo) bool {
	return true
}

func init() {
	registerIface(secretInterface{})
}

// Name implements [interfaces.Interface], returning the unique name of
// the secret interface.
func (secretInterface) Name() string {
	return "secret"
}

// StaticInfo implements [interfaces.StaticInfoProvider], supplying the
// summary and base declaration policy for the secret interface.
func (secretInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              secretSummary,
		BaseDeclarationPlugs: secretBaseDeclarationPlugs,
		BaseDeclarationSlots: secretBaseDeclarationSlots,
	}
}
