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
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/canonical/workshop/internal/sdk"
)

// RepositorySecretSlotLookup reads system secret slot configuration from the
// interface repository. Registered slot metadata must remain immutable while
// lookups are in progress; returned configurations are independent copies.
type RepositorySecretSlotLookup struct {
	repo SlotRepository
}

// SecretSlotConfig describes a system secret slot's Secret Service search.
type SecretSlotConfig struct {
	// Attributes contains the string key/value pairs used to find the secret.
	Attributes map[string]string

	// Collection identifies the Secret Service collection to search.
	Collection string
}

// SecretSlotLookup reads typed configuration for system SDK secret slots.
// Consumer authorisation and connection validation are the caller's
// responsibility, not part of slot lookup.
type SecretSlotLookup interface {
	// Lookup returns validated, independently owned configuration for the
	// referenced slot, or an error if the slot is missing or invalid.
	// Implementations must release any
	// repository or state locks before returning.
	Lookup(context.Context, sdk.SlotRef) (SecretSlotConfig, error)
}

// SlotRepository looks up registered SDK slots.
type SlotRepository interface {
	// Slot looks up a slot by project ID, workshop name, SDK name and slot
	// name, returning nil if none exists. Returned metadata must remain
	// immutable while it is being read.
	Slot(string, string, string, string) *sdk.SlotInfo
}

// Lookup returns typed configuration for the referenced system secret slot.
// It does not authorise consumers or validate connections. An omitted
// collection defaults to "default".
func (l RepositorySecretSlotLookup) Lookup(
	_ context.Context,
	ref sdk.SlotRef,
) (SecretSlotConfig, error) {
	slot := l.repo.Slot(ref.ProjectId, ref.Workshop, ref.Sdk, ref.Name)
	if slot == nil {
		return SecretSlotConfig{}, errors.New("secret slot not found")
	}

	if slot.Sdk == nil || slot.Sdk.Type != sdk.System {
		return SecretSlotConfig{}, errors.New(
			"secret slot is not provided by the system SDK",
		)
	}

	if slot.Interface != "secret" {
		return SecretSlotConfig{}, errors.New(
			"slot does not use the secret interface",
		)
	}

	// Attribute conversion assumes non-nil values. Validate the map shape
	// first so malformed slot metadata produces an error rather than a panic.
	switch attributes := slot.Attrs["attributes"].(type) {
	case map[string]any:
		for _, value := range attributes {
			if _, ok := value.(string); !ok {
				return SecretSlotConfig{}, errors.New(
					"secret slot attributes must contain string values",
				)
			}
		}
	case map[string]string:
	default:
		return SecretSlotConfig{}, errors.New(
			"secret slot attributes must be a map of strings",
		)
	}

	config := SecretSlotConfig{Collection: "default"}
	err := slot.Attr("attributes", &config.Attributes)
	if err != nil {
		return SecretSlotConfig{}, fmt.Errorf(
			"reading secret slot attributes: %w", err,
		)
	}
	if len(config.Attributes) == 0 {
		return SecretSlotConfig{}, errors.New(
			"secret slot attributes must not be empty",
		)
	}
	if collection, exists := slot.Attrs["collection"]; exists {
		if _, ok := collection.(string); !ok {
			return SecretSlotConfig{}, errors.New(
				"secret slot collection must be a string",
			)
		}
		err = slot.Attr("collection", &config.Collection)
		if err != nil {
			return SecretSlotConfig{}, fmt.Errorf(
				"reading secret slot collection: %w", err,
			)
		}
	}
	if strings.TrimSpace(config.Collection) == "" {
		return SecretSlotConfig{}, errors.New(
			"secret slot collection must not be empty",
		)
	}

	// Attr may return the original map when its type already matches.
	config.Attributes = maps.Clone(config.Attributes)
	return config, nil
}

// NewRepositorySecretSlotLookup creates a lookup backed by repo.
// The repository must not be nil.
func NewRepositorySecretSlotLookup(
	repo SlotRepository,
) RepositorySecretSlotLookup {
	return RepositorySecretSlotLookup{repo: repo}
}
