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

package ctlcmd_test

import (
	"context"
	"errors"
	"time"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// secretResolver delegates secret resolution to a test callback.
type secretResolver func(context.Context, sdk.SlotRef) (secrets.Secret, error)

// secretStateBackend signals task scheduling without persistence or timers.
type secretStateBackend struct {
	ensureBefore chan time.Duration
}

// secretWorkshopBackend resolves one workshop for its owning user and project.
type secretWorkshopBackend struct {
	err      error
	user     string
	workshop *workshop.Workshop
}

// Checkpoint discards state snapshots for command tests.
func (b *secretStateBackend) Checkpoint([]byte) error {
	return nil
}

// EnsureBefore records a notification without blocking the state lock.
func (b *secretStateBackend) EnsureBefore(delay time.Duration) {
	select {
	case b.ensureBefore <- delay:
	default:
	}
}

// Resolve delegates retrieval to the test's callback.
func (f secretResolver) Resolve(
	ctx context.Context,
	ref sdk.SlotRef,
) (secrets.Secret, error) {
	return f(ctx, ref)
}

// Workshop returns the fixture only when the request identity matches.
func (b *secretWorkshopBackend) Workshop(
	ctx context.Context,
	name string,
) (*workshop.Workshop, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	if b.err != nil {
		return nil, b.err
	}
	if ctx.Value(workshop.ContextUser) != b.user {
		return nil, errors.New("user not found")
	}
	if ctx.Value(workshop.ContextProjectId) != b.workshop.Project.ProjectId {
		return nil, errors.New("project not found")
	}
	if name != b.workshop.Name {
		return nil, errors.New("workshop not found")
	}
	return b.workshop, nil
}
