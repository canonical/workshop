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

package system_test

import (
	"context"
	"errors"
	"io"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
)

// secretProviderSuite tests placeholder retrieval by [system.SecretProvider].
type secretProviderSuite struct{}

var _ = check.Suite(&secretProviderSuite{})

// TestResolve checks the provider looks up the requested slot and returns a
// readable value after a successful configuration lookup.
func (s *secretProviderSuite) TestResolve(c *check.C) {
	slot := sdk.SlotRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "system",
		Workshop:  "backend",
	}

	called := false
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		called = true
		c.Check(ref.ProjectId, check.Equals, "test-project")
		c.Check(ref.Workshop, check.Equals, "backend")
		c.Check(ref.Sdk, check.Equals, "system")
		c.Check(ref.Name, check.Equals, "api-key")
		return system.SecretSlotConfig{
			Attributes: map[string]string{
				"service": "ollama",
			},
			Collection: "default",
		}, nil
	})
	provider := system.NewSecretProvider(slots)

	secret, err := provider.Resolve(context.Background(), slot)
	c.Assert(err, check.IsNil)
	defer secret.Close()
	c.Check(called, check.Equals, true)

	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("workshop-placeholder-secret"))

}

// TestResolveCancelled checks an already-cancelled request returns an empty,
// usable secret without looking up slot configuration.
func (s *secretProviderSuite) TestResolveCancelled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Error("cancelled request must not look up slot configuration")
		return system.SecretSlotConfig{}, nil
	})
	provider := system.NewSecretProvider(slots)

	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	value, err := provider.Resolve(ctx, slot)

	data, readErr := io.ReadAll(value)
	c.Check(readErr, check.IsNil)
	c.Check(len(data), check.Equals, 0)
	c.Check(value.Close(), check.IsNil)
	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestResolveExpired checks an elapsed deadline returns an empty, usable
// secret without looking up slot configuration.
func (s *secretProviderSuite) TestResolveExpired(c *check.C) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Error("expired request must not look up slot configuration")
		return system.SecretSlotConfig{}, nil
	})
	provider := system.NewSecretProvider(slots)

	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	value, err := provider.Resolve(ctx, slot)

	data, readErr := io.ReadAll(value)
	c.Check(readErr, check.IsNil)
	c.Check(len(data), check.Equals, 0)
	c.Check(value.Close(), check.IsNil)
	c.Check(errors.Is(err, context.DeadlineExceeded), check.Equals, true)
}

// TestResolveLookupFailure checks lookup errors retain their identity and
// return an empty, usable secret instead of a placeholder.
func (s *secretProviderSuite) TestResolveLookupFailure(c *check.C) {
	lookupErr := errors.New("slot not found")
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{}, lookupErr
	})
	provider := system.NewSecretProvider(slots)
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	value, err := provider.Resolve(context.Background(), slot)

	data, readErr := io.ReadAll(value)
	c.Check(readErr, check.IsNil)
	c.Check(len(data), check.Equals, 0)
	c.Check(value.Close(), check.IsNil)

	c.Check(errors.Is(err, lookupErr), check.Equals, true)
}

// TestResolveLookupCancelled checks cancellation reported by the lookup is
// preserved through error wrapping and returns an empty, usable secret.
func (s *secretProviderSuite) TestResolveLookupCancelled(c *check.C) {
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{}, context.Canceled
	})
	provider := system.NewSecretProvider(slots)
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	value, err := provider.Resolve(context.Background(), slot)

	data, readErr := io.ReadAll(value)
	c.Check(readErr, check.IsNil)
	c.Check(len(data), check.Equals, 0)
	c.Check(value.Close(), check.IsNil)
	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}
