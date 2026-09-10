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
	"time"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// secretStateBackend signals ensure requests without blocking the state lock.
type secretStateBackend struct {
	ensureBefore chan time.Duration
}

// taskHandlerRegistrar records task registration without running tasks.
type taskHandlerRegistrar struct {
	calls int
	do    state.HandlerFunc
	kind  string
	undo  state.HandlerFunc
}

// workshopBackendFunc supplies workshop lookup behaviour without a backend.
type workshopBackendFunc func(
	context.Context,
	string,
) (*workshop.Workshop, error)

// AddHandler records the supplied task kind and handlers.
func (r *taskHandlerRegistrar) AddHandler(
	kind string,
	do state.HandlerFunc,
	undo state.HandlerFunc,
) {
	r.calls++
	r.do = do
	r.kind = kind
	r.undo = undo
}

// Checkpoint discards state snapshots in retrieval tests.
func (b *secretStateBackend) Checkpoint([]byte) error {
	return nil
}

// EnsureBefore records a request unless an unread notification is pending.
func (b *secretStateBackend) EnsureBefore(delay time.Duration) {
	select {
	case b.ensureBefore <- delay:
	default:
	}
}

// Workshop delegates lookup to the test's callback.
func (f workshopBackendFunc) Workshop(
	ctx context.Context,
	name string,
) (*workshop.Workshop, error) {
	return f(ctx, name)
}
