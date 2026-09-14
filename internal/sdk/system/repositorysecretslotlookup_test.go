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

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
)

// repositorySecretSlotLookupSuite tests repository-backed secret slot lookup.

type repositorySecretSlotLookupSuite struct{}

var _ = check.Suite(&repositorySecretSlotLookupSuite{})

// Lookup rejects an empty attributes map.
func (s *repositorySecretSlotLookupSuite) TestEmptyAttributes(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches, "secret slot attributes must not be empty")
}

// Lookup rejects an empty collection instead of supplying a default.
func (s *repositorySecretSlotLookupSuite) TestEmptyCollection(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": "",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches, "secret slot collection must not be empty")
}

// Lookup rejects a collection whose value is not a string.
func (s *repositorySecretSlotLookupSuite) TestInvalidCollectionType(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": 42,
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches, "secret slot collection must be a string")
}

// Lookup rejects slot metadata without an attributes map.
func (s *repositorySecretSlotLookupSuite) TestMissingAttributes(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(
		err,
		check.ErrorMatches,
		"secret slot attributes must be a map of strings",
	)
}

// Lookup uses the default collection when none is specified.
func (s *repositorySecretSlotLookupSuite) TestMissingCollection(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	config, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Assert(err, check.IsNil)
	c.Check(config, check.DeepEquals, system.SecretSlotConfig{
		Attributes: map[string]string{
			"service": "github",
		},
		Collection: "default",
	})
}

// Lookup forwards all reference components and reports a missing slot.
func (s *repositorySecretSlotLookupSuite) TestMissingSlot(c *check.C) {
	repo := &slotRepository{}
	lookup := system.NewRepositorySecretSlotLookup(repo)
	ref := sdk.SlotRef{
		ProjectId: "test-project",
		Workshop:  "backend",
		Sdk:       "system",
		Name:      "github-token",
	}

	_, err := lookup.Lookup(context.Background(), ref)
	c.Check(err, check.ErrorMatches, "secret slot not found")
	c.Check(repo.refs, check.DeepEquals, []sdk.SlotRef{ref})
}

// Lookup rejects nil attribute values before conversion can panic.
func (s *repositorySecretSlotLookupSuite) TestNilAttributeValue(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
				"account": nil,
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(
		err,
		check.ErrorMatches,
		"secret slot attributes must contain string values",
	)
}

// Lookup defensively rejects a stored slot with no SDK before reading attrs.
func (s *repositorySecretSlotLookupSuite) TestNilSDK(c *check.C) {
	slot := &sdk.SlotInfo{
		Name:      "github-token",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)
	ref := sdk.SlotRef{
		ProjectId: "test-project",
		Workshop:  "backend",
		Sdk:       "system",
		Name:      "github-token",
	}

	_, err := lookup.Lookup(context.Background(), ref)
	c.Check(err, check.ErrorMatches,
		"secret slot is not provided by the system SDK")
}

// Lookup rejects attributes that are not a map.
func (s *repositorySecretSlotLookupSuite) TestNonMapAttributes(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": "service=github",
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(
		err,
		check.ErrorMatches,
		"secret slot attributes must be a map of strings",
	)
}

// Lookup rejects non-string attribute values rather than coercing them.
func (s *repositorySecretSlotLookupSuite) TestNonStringAttributeValue(
	c *check.C,
) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
				"account": 42,
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(
		err,
		check.ErrorMatches,
		"secret slot attributes must contain string values",
	)
}

// Lookup checks SDK type, not merely the SDK's name.
func (s *repositorySecretSlotLookupSuite) TestNonSystemSDK(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.Regular,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "github-token",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches,
		"secret slot is not provided by the system SDK")
}

// Lookup decodes an untyped attributes map into an independent string map.
func (s *repositorySecretSlotLookupSuite) TestAnyMapIsCopied(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "github-token",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
				"account": "workshop-developer",
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)
	expected := system.SecretSlotConfig{
		Attributes: map[string]string{
			"service": "github",
			"account": "workshop-developer",
		},
		Collection: "login",
	}

	config, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Assert(err, check.IsNil)
	c.Assert(config, check.DeepEquals, expected)
	config.Attributes["service"] = "gitlab"
	delete(config.Attributes, "account")
	config.Attributes["extra"] = "unexpected"

	again, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Assert(err, check.IsNil)
	c.Check(again, check.DeepEquals, expected)
	c.Check(slot.Attrs["attributes"], check.DeepEquals, map[string]any{
		"service": "github",
		"account": "workshop-developer",
	})
}

// Lookup clones even typed maps that attribute decoding may return directly.
func (s *repositorySecretSlotLookupSuite) TestStringMapIsCopied(c *check.C) {
	repo := &slotRepository{}
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "github-token",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]string{
				"service": "github",
				"account": "workshop-developer",
			},
			"collection": "login",
		},
	}
	repo.slot = slot
	lookup := system.NewRepositorySecretSlotLookup(repo)
	expected := system.SecretSlotConfig{
		Attributes: map[string]string{
			"service": "github",
			"account": "workshop-developer",
		},
		Collection: "login",
	}

	config, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Assert(err, check.IsNil)
	c.Assert(config, check.DeepEquals, expected)
	config.Attributes["service"] = "gitlab"
	delete(config.Attributes, "account")
	config.Attributes["extra"] = "unexpected"

	again, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Assert(err, check.IsNil)
	c.Check(again, check.DeepEquals, expected)
	c.Check(slot.Attrs["attributes"], check.DeepEquals, expected.Attributes)
}

// Lookup rejects a collection containing only whitespace.
func (s *repositorySecretSlotLookupSuite) TestWhitespaceCollection(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "api-key",
		Interface: "secret",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": " \t\n",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches, "secret slot collection must not be empty")
}

// Lookup rejects other interfaces even when secret-like attributes are present.
func (s *repositorySecretSlotLookupSuite) TestWrongInterface(c *check.C) {
	slot := &sdk.SlotInfo{
		Sdk: &sdk.Info{
			Base:      "ubuntu@24.04",
			Name:      "system",
			Type:      sdk.System,
			ProjectId: "test-project",
			Workshop:  "backend",
		},
		Name:      "github-token",
		Interface: "mount",
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "github",
			},
			"collection": "login",
		},
	}
	repo := &slotRepository{slot: slot}
	lookup := system.NewRepositorySecretSlotLookup(repo)

	_, err := lookup.Lookup(context.Background(), slot.Ref())
	c.Check(err, check.ErrorMatches, "slot does not use the secret interface")
}
