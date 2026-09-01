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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

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

const (
	// secretSlotAttributesKey identifies the host keyring search attributes in
	// a secret slot.
	secretSlotAttributesKey = "attributes"

	// secretSlotCollectionKey identifies the optional host keyring collection
	// in a secret slot.
	secretSlotCollectionKey = "collection"

	// defaultSecretCollection is used when a secret slot does not select a host
	// keyring collection.
	defaultSecretCollection = "default"

	// secretSummary describes the purpose of the secret interface.
	secretSummary = `allows SDKs to consume secrets from the host keyring`
)

// AutoConnect implements [interfaces.Interface]. It returns true to
// defer entirely to the base declaration policy, which denies
// auto-connection for secrets: a secret must always be attached by an
// explicit user decision.
func (secretInterface) AutoConnect(_ *sdk.PlugInfo, _ *sdk.SlotInfo) bool {
	return true
}

// BeforePrepareSlot validates and normalizes the host keyring lookup details
// declared by a secret slot.
func (secretInterface) BeforePrepareSlot(slot *sdk.SlotInfo) error {
	if slot.Sdk.Type != sdk.System {
		return errors.New(
			"secret interface slots are only supported by the system SDK")
	}

	knownSlotKeys := []string{
		secretSlotAttributesKey,
		secretSlotCollectionKey,
	}
	attrKeys := slices.Collect(maps.Keys(slot.Attrs))
	unknownKeys := slices.DeleteFunc(attrKeys, func(key string) bool {
		return slices.Contains(knownSlotKeys, key)
	})
	if len(unknownKeys) > 0 {
		return errors.New("unsupported keys found in secret slot")
	}

	rawAttributes, exists := slot.Attrs[secretSlotAttributesKey]
	if !exists {
		return errors.New(`secret interface slot must contain "attributes"`)
	}

	attributes, ok := rawAttributes.(map[string]any)
	if !ok {
		return errors.New(`secret interface slot "attributes" must be a map`)
	}
	if len(attributes) == 0 {
		return errors.New("at least one attribute must be defined for a secret slot")
	}

	for key, value := range attributes {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("attribute %q must be of type string", key)
		}
	}

	rawCollection, exists := slot.Attrs[secretSlotCollectionKey]
	if !exists {
		slot.Attrs[secretSlotCollectionKey] = defaultSecretCollection
		return nil
	}

	collection, ok := rawCollection.(string)
	if !ok {
		return errors.New("secret slot collection key must have a string value")
	}
	if strings.TrimSpace(collection) == "" {
		return errors.New("secret slot collection value must contain a non empty string value")
	}

	return nil
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
