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

package system

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/canonical/workshop/internal/sdk"
)

const (
	// SecretSlotAttributesKey identifies the host keyring search attributes
	// in a system SDK secret slot.
	SecretSlotAttributesKey = "attributes"

	// SecretSlotCollectionKey identifies the optional host keyring collection
	// in a system SDK secret slot.
	SecretSlotCollectionKey = "collection"

	defaultSecretCollection = "default"
)

// BeforePrepareSecretSlot validates and normalizes a secret slot provided by
// the system SDK.
func BeforePrepareSecretSlot(slot *sdk.SlotInfo) error {
	knownSlotKeys := []string{SecretSlotAttributesKey, SecretSlotCollectionKey}

	attrKeys := slices.Collect(maps.Keys(slot.Attrs))
	unknownKeys := slices.DeleteFunc(attrKeys, func(key string) bool {
		return slices.Contains(knownSlotKeys, key)
	})
	if len(unknownKeys) > 0 {
		return errors.New("unsupported keys found in secret slot")
	}

	rawAttributes, exists := slot.Attrs[SecretSlotAttributesKey]
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

	rawCollection, exists := slot.Attrs[SecretSlotCollectionKey]
	if !exists {
		slot.Attrs[SecretSlotCollectionKey] = defaultSecretCollection
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
