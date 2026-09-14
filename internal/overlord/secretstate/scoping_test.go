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

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// TestGetSecretConnectionResolutionFailure checks repository lookup errors
// receive context and never produce a value. An empty workshop deliberately
// exercises this internal path; the public entry point rejects it earlier.
func (s *managerSuite) TestGetSecretConnectionResolutionFailure(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := sdk.PlugRef{
		Name: "api-key", ProjectId: "test-project", Sdk: "ollama",
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
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	_, err = manager.getSecret(ctx, ref)

	c.Check(err, ErrorMatches,
		"resolving secret plug connections: internal error: "+
			"cannot obtain workshop name while computing connections")
}

// TestGetSecretConnectionScopedToProject checks a connection belonging to
// another project cannot authorise the requesting project's identical plug.
func (s *managerSuite) TestGetSecretConnectionScopedToProject(c *C) {
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
	otherPlug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: "other-project",
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
	c.Assert(repo.AddPlug(otherPlug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(otherPlug, slot),
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
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	_, err = manager.getSecret(ctx, ref)

	c.Check(err, ErrorMatches,
		`secret plug is not connected`)
}

// TestGetSecretConnectionScopedToWorkshop checks a connection belonging to
// another workshop cannot authorise the requesting workshop's identical plug.
func (s *managerSuite) TestGetSecretConnectionScopedToWorkshop(c *C) {
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
	otherPlug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      ref.Name,
		Sdk: &sdk.Info{
			Name: ref.Sdk, ProjectId: ref.ProjectId,
			Type: sdk.Regular, Workshop: "other-workshop",
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
	c.Assert(repo.AddPlug(otherPlug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(otherPlug, slot),
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
		return &workshop.Workshop{
			Name: ref.Workshop,
			Sdks: map[string]workshop.SdkInstallation{
				ref.Sdk: {Setup: sdk.Setup{Name: ref.Sdk}},
			},
		}, nil
	})
	manager := SecretManager{backend: backend, repo: repo}

	_, err = manager.getSecret(ctx, ref)

	c.Check(err, ErrorMatches,
		`secret plug is not connected`)
}
