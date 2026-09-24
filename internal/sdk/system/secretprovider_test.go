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
	"fmt"
	"io"
	"os/user"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/sdk/system/secret"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// secretProviderSuite tests secret retrieval by [system.SecretProvider].
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
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	value := secrets.NewSecret([]byte("test-secret"))
	defer value.Close()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return value, nil
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)
	c.Assert(err, check.IsNil)
	defer resolved.Close()
	c.Check(called, check.Equals, true)

	contents, err := io.ReadAll(resolved)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte("test-secret"))

}

// TestResolveCancelled checks an already-cancelled request returns a
// cancellation error without looking up slot configuration.
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
	provider := system.NewSecretProvider(slots, nil)

	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestResolveExpired checks an elapsed deadline returns a deadline-exceeded
// error without looking up slot configuration.
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
	provider := system.NewSecretProvider(slots, nil)

	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, context.DeadlineExceeded), check.Equals, true)
}

// TestResolveServiceCancelled checks service cancellation remains identifiable.
func (s *secretProviderSuite) TestResolveServiceCancelled(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf("service: %w", context.Canceled)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceCollectionAmbiguous checks collection ambiguity remains
// identifiable rather than being mapped to a missing secret.
func (s *secretProviderSuite) TestResolveServiceCollectionAmbiguous(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf(
			"service: %w", secret.ErrorCollectionAmbiguous,
		)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secret.ErrorCollectionAmbiguous), check.Equals, true)
	c.Check(errors.Is(err, secrets.ErrorSecretNotFound), check.Equals, false)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceCollectionLocked checks a wrapped locked collection error
// maps to the provider-locked error.
func (s *secretProviderSuite) TestResolveServiceCollectionLocked(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf(
			"service: %w", secret.ErrorCollectionLocked,
		)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secrets.ErrorProviderLocked), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceCollectionNotFound checks a wrapped missing collection
// error maps to the secret-not-found error.
func (s *secretProviderSuite) TestResolveServiceCollectionNotFound(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf(
			"service: %w", secret.ErrorCollectionNotFound,
		)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secrets.ErrorSecretNotFound), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceMultipleSecrets checks a wrapped multiple-secrets error
// maps to the provider's multiple-secrets error.
func (s *secretProviderSuite) TestResolveServiceMultipleSecrets(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf(
			"service: %w", secret.ErrorMultipleSecrets,
		)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secrets.ErrorMultipleSecrets), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceSecretNotFound checks a wrapped missing secret error maps
// to the provider's secret-not-found error.
func (s *secretProviderSuite) TestResolveServiceSecretNotFound(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf(
			"service: %w", secret.ErrorSecretNotFound,
		)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secrets.ErrorSecretNotFound), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveServiceUnknownError checks an unknown wrapped service error
// retains its identity.
func (s *secretProviderSuite) TestResolveServiceUnknownError(c *check.C) {
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	slots := secretSlotLookup(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		c.Check(ref, check.DeepEquals, slot)
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		c.Check(name, check.Equals, "alice")
		return &user.User{
			Gid: "1002", HomeDir: "/home/alice", Name: "Alice",
			Uid: "1001", Username: "alice",
		}, nil
	})()
	serviceErr := errors.New("session bus unavailable")
	service := secretService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
			UID:        "1001",
		})
		return secrets.Secret{}, fmt.Errorf("service: %w", serviceErr)
	})
	provider := system.NewSecretProvider(slots, service)
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	resolved, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, serviceErr), check.Equals, true)
	c.Check(resolved, check.DeepEquals, secrets.Secret{})
}

// TestResolveUnknownUser checks an unknown account returns the user-not-found
// error, including when the account lookup error is wrapped.
func (s *secretProviderSuite) TestResolveUnknownUser(c *check.C) {
	slots := secretSlotLookup(func(
		context.Context,
		sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	defer osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return nil, fmt.Errorf("lookup: %w", user.UnknownUserError(name))
	})()
	provider := system.NewSecretProvider(slots, nil)
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	_, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, secrets.ErrorUserNotFound), check.Equals, true)
}

// TestResolveUserLookupFailure checks other account lookup failures retain
// their identity rather than being reported as a missing user.
func (s *secretProviderSuite) TestResolveUserLookupFailure(c *check.C) {
	slots := secretSlotLookup(func(
		context.Context,
		sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	lookupErr := errors.New("account database unavailable")
	defer osutil.FakeUserLookup(func(string) (*user.User, error) {
		return nil, lookupErr
	})()
	provider := system.NewSecretProvider(slots, nil)
	slot := sdk.SlotRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "system", Workshop: "backend",
	}
	ctx := context.WithValue(context.Background(), workshop.ContextUser, "alice")

	_, err := provider.Resolve(ctx, slot)

	c.Check(errors.Is(err, lookupErr), check.Equals, true)
	c.Check(errors.Is(err, secrets.ErrorUserNotFound), check.Equals, false)
}

// TestResolveMissingUser checks a missing context user returns the expected
// error without requesting a secret.
func (s *secretProviderSuite) TestResolveMissingUser(c *check.C) {
	slots := secretSlotLookup(func(
		context.Context,
		sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{
			Attributes: map[string]string{"service": "ollama"},
			Collection: "default",
		}, nil
	})
	service := secretService(func(
		context.Context,
		secret.Request,
	) (secrets.Secret, error) {
		c.Fatal("missing user must not request a secret")
		return secrets.Secret{}, nil
	})
	provider := system.NewSecretProvider(slots, service)
	slot := sdk.SlotRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "system",
		Workshop:  "backend",
	}

	_, err := provider.Resolve(context.Background(), slot)

	c.Check(errors.Is(err, secrets.ErrorUserNotFound), check.Equals, true)
}

// TestResolveLookupFailure checks lookup errors retain their identity.
func (s *secretProviderSuite) TestResolveLookupFailure(c *check.C) {
	lookupErr := errors.New("slot not found")
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{}, lookupErr
	})
	provider := system.NewSecretProvider(slots, nil)
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := provider.Resolve(context.Background(), slot)

	c.Check(errors.Is(err, lookupErr), check.Equals, true)
}

// TestResolveLookupCancelled checks cancellation reported by the lookup is
// preserved through error wrapping.
func (s *secretProviderSuite) TestResolveLookupCancelled(c *check.C) {
	slots := secretSlotLookup(func(
		_ context.Context,
		_ sdk.SlotRef,
	) (system.SecretSlotConfig, error) {
		return system.SecretSlotConfig{}, context.Canceled
	})
	provider := system.NewSecretProvider(slots, nil)
	slot := sdk.SlotRef{Name: "api-key", Sdk: "system"}

	_, err := provider.Resolve(context.Background(), slot)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}
