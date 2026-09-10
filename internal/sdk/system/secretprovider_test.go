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
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/secrets"
)

// secretProviderSuite tests placeholder retrieval by [system.SecretProvider].
type secretProviderSuite struct{}

var _ = check.Suite(&secretProviderSuite{})

// TestResolve checks the provider returns the placeholder secret.
func (s *secretProviderSuite) TestResolve(c *check.C) {
	provider := system.NewSecretProvider()

	source := secrets.Source{
		Attributes: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Slot: sdk.SlotRef{Name: "api-key", Sdk: "system"},
	}

	value, err := provider.Resolve(context.Background(), source)

	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("workshop-placeholder-secret"))
}

// TestResolveCancelled checks an already-cancelled request returns no secret.
func (s *secretProviderSuite) TestResolveCancelled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := system.NewSecretProvider()

	source := secrets.Source{
		Attributes: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Slot: sdk.SlotRef{Name: "api-key", Sdk: "system"},
	}

	value, err := provider.Resolve(ctx, source)

	c.Check(value, check.IsNil)
	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestResolveExpired checks an elapsed deadline returns no secret.
func (s *secretProviderSuite) TestResolveExpired(c *check.C) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	provider := system.NewSecretProvider()

	source := secrets.Source{
		Attributes: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Slot: sdk.SlotRef{Name: "api-key", Sdk: "system"},
	}

	value, err := provider.Resolve(ctx, source)

	c.Check(value, check.IsNil)
	c.Check(errors.Is(err, context.DeadlineExceeded), check.Equals, true)
}
