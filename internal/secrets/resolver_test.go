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
	"errors"
	"io"
	"testing"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
)

// resolverSuite checks provider selection, returned values and errors.
type resolverSuite struct{}

var _ = check.Suite(&resolverSuite{})

// TestSecrets runs the secret resolver suite.
func TestSecrets(t *testing.T) {
	check.TestingT(t)
}

// TestNewResolverCopiesProviders checks that the resolver retains its initial
// provider registrations independently of the caller's map. Replacing an entry
// in that map after construction must not change which provider resolves a
// request.
func (s *resolverSuite) TestNewResolverCopiesProviders(c *check.C) {
	original := NewSecret([]byte("original-secret"))
	defer original.Close()
	provider := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return original, nil
	})

	providers := map[string]Provider{"system": provider}
	resolver := NewResolver(providers)
	replacementSecret := NewSecret([]byte("replacement-secret"))
	defer replacementSecret.Close()

	replacement := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return replacementSecret, nil
	})
	providers["system"] = replacement
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	secret, err := resolver.Resolve(context.Background(), slot)
	c.Assert(err, check.IsNil)
	defer secret.Close()

	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("original-secret"))

}

// TestResolve checks that the resolver selects the requested SDK's provider
// and returns the provider's secret value.
func (s *resolverSuite) TestResolve(c *check.C) {
	resolved := NewSecret([]byte("resolved-secret"))
	defer resolved.Close()
	provider := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return resolved, nil
	})

	otherSecret := NewSecret([]byte("other-secret"))
	defer otherSecret.Close()
	other := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return otherSecret, nil
	})

	resolver := NewResolver(map[string]Provider{
		"other":  other,
		"system": provider,
	})
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	secret, err := resolver.Resolve(context.Background(), slot)
	c.Assert(err, check.IsNil)
	defer secret.Close()

	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("resolved-secret"))

}

// TestResolveCancelled checks cancelled requests return a cancellation error.
func (s *resolverSuite) TestResolveCancelled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resolved := NewSecret([]byte("resolved-secret"))
	defer resolved.Close()
	provider := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return resolved, nil
	})
	resolver := NewResolver(map[string]Provider{"system": provider})
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := resolver.Resolve(ctx, slot)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestResolveExpired checks expired requests return a deadline-exceeded error.
func (s *resolverSuite) TestResolveExpired(c *check.C) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()

	resolved := NewSecret([]byte("resolved-secret"))
	defer resolved.Close()

	provider := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return resolved, nil
	})
	resolver := NewResolver(map[string]Provider{"system": provider})
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := resolver.Resolve(ctx, slot)

	c.Check(errors.Is(err, context.DeadlineExceeded), check.Equals, true)
}

// TestResolveNilProvider checks a nil registration returns a
// provider-not-found error.
func (s *resolverSuite) TestResolveNilProvider(c *check.C) {
	resolver := NewResolver(map[string]Provider{"system": nil})
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := resolver.Resolve(context.Background(), slot)
	c.Check(errors.Is(err, ErrorProviderNotFound), check.Equals, true)
}

// TestResolveProviderError checks provider errors retain their identity.
func (s *resolverSuite) TestResolveProviderError(c *check.C) {
	providerErr := errors.New("secret service unavailable")
	provider := stubProvider(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (Secret, error) {
		return Secret{}, providerErr
	})
	resolver := NewResolver(map[string]Provider{"system": provider})
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := resolver.Resolve(context.Background(), slot)

	c.Check(errors.Is(err, providerErr), check.Equals, true)

}

// TestResolveUnknownSDK checks an unregistered SDK returns a
// provider-not-found error.
func (s *resolverSuite) TestResolveUnknownSDK(c *check.C) {
	resolver := NewResolver(nil)
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := resolver.Resolve(context.Background(), slot)
	c.Check(errors.Is(err, ErrorProviderNotFound), check.Equals, true)
}
