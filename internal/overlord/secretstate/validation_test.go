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
	"errors"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// TestGetSecretCancelledAfterValidation checks cancellation during lookup
// prevents returning even a valid, connected secret's placeholder.
func (s *managerSuite) TestGetSecretCancelledAfterValidation(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "api-key",
		Sdk:       &sdk.Info{Name: "system", Type: sdk.System},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		cancel()
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(errors.Is(err, context.Canceled), Equals, true)
}

// TestGetSecretDisconnectedPlug checks a declared secret plug needs a
// connection before the placeholder can be returned.
func (s *managerSuite) TestGetSecretDisconnectedPlug(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	c.Assert(repo.AddPlug(&sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}), IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(err, ErrorMatches,
		`secret plug is not connected`)
}

// TestGetSecretPlugNotDeclared checks installed SDKs cannot request an
// undeclared plug, even if another plug exists in that SDK.
func (s *managerSuite) TestGetSecretPlugNotDeclared(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	c.Assert(repo.AddPlug(&sdk.PlugInfo{
		Interface: "secret",
		Name:      "other-key",
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}), IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(err, ErrorMatches, `requested plug is not declared by sdk`)
}

// TestGetSecretSDKNotInstalled checks lookup requires the requested SDK,
// rather than merely a workshop containing some installed SDK.
func (s *managerSuite) TestGetSecretSDKNotInstalled(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				"other-sdk": {Setup: sdk.Setup{Name: "other-sdk"}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(err, ErrorMatches,
		`requested sdk is not installed in workshop`)
}

// TestGetSecretScopedSuccess checks the requested project and workshop
// select the connected plug despite identically named disconnected plugs.
func (s *managerSuite) TestGetSecretScopedSuccess(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextProjectId, "real-project")
	ctx = context.WithValue(ctx, workshop.ContextUser, "real-user")
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "real-project",
		Sdk: "ollama", Workshop: "real-workshop",
	}
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "api-key",
		Sdk:       &sdk.Info{Name: "system", Type: sdk.System},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	c.Assert(repo.AddPlug(&sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: "other-project",
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}), IsNil)
	c.Assert(repo.AddPlug(&sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: "other-workshop",
		},
	}), IsNil)
	calls := 0
	backend := workshopBackendFunc(func(
		actualContext context.Context,
		name string,
	) (*workshop.Workshop, error) {
		calls++
		c.Check(actualContext, Equals, ctx)
		c.Check(name, Equals, ref.Workshop)
		return &workshop.Workshop{
			Name: ref.Workshop,
			Project: workshop.Project{
				Path: c.MkDir(), ProjectId: ref.ProjectId,
			},
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	value, err := manager.getSecret(ctx, ref)

	c.Check(err, IsNil)
	c.Check(calls, Equals, 1)
	c.Check(value, DeepEquals, []byte("workshop-placeholder-secret"))
}

// TestGetSecretWorkshopResolutionFailure checks backend errors are wrapped
// without losing their identity or returning a secret value.
func (s *managerSuite) TestGetSecretWorkshopResolutionFailure(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	lookupErr := errors.New("workshop unavailable")
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return nil, lookupErr
	})
	manager := SecretManager{backend: backend}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(err, ErrorMatches,
		"resolving workshop: workshop unavailable")
	c.Check(errors.Is(err, lookupErr), Equals, true)
}

// TestGetSecretWrongInterface checks a declared non-secret plug cannot be
// used to retrieve secrets, before looking for any connections.
func (s *managerSuite) TestGetSecretWrongInterface(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project",
		Sdk: "ollama", Workshop: "test-workshop",
	}
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("camera")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	c.Assert(repo.AddPlug(&sdk.PlugInfo{
		Interface: "camera",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: ref.Workshop,
		},
	}), IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(err, ErrorMatches,
		`requested plug does not use the secret interface`)
}
