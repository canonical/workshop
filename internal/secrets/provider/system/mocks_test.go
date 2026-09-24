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

	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
)

// fakeBusConnection records D-Bus calls without a real session bus.
type fakeBusConnection struct {
	call   func(context.Context, dbus.ObjectPath, string, []any, ...any) error
	closed bool
}

// secretService delegates secret retrieval to a test-defined function.
type secretService func(
	context.Context,
	Request,
) (secrets.Secret, error)

// secretSlotLookup delegates slot lookup to a test-defined function.
type secretSlotLookup func(
	context.Context,
	sdk.SlotRef,
) (SecretSlotConfig, error)

// slotRepository records repository requests and returns a configured slot.
type slotRepository struct {
	refs []sdk.SlotRef
	slot *sdk.SlotInfo
}

func (c *fakeBusConnection) Call(
	ctx context.Context,
	path dbus.ObjectPath,
	method string,
	args []any,
	results ...any,
) error {
	return c.call(ctx, path, method, args, results...)
}

func (c *fakeBusConnection) Close() error {
	c.closed = true
	return nil
}

// Get calls the test-defined secret retrieval function.
func (s secretService) Get(
	ctx context.Context,
	request Request,
) (secrets.Secret, error) {
	return s(ctx, request)
}

// Lookup calls the test-defined lookup function.
func (l secretSlotLookup) Lookup(
	ctx context.Context,
	slot sdk.SlotRef,
) (SecretSlotConfig, error) {
	return l(ctx, slot)
}

// Slot records all four reference components without accessing a repository.
func (r *slotRepository) Slot(
	project string,
	workshop string,
	sdkName string,
	slot string,
) *sdk.SlotInfo {
	r.refs = append(r.refs, sdk.SlotRef{
		ProjectId: project,
		Workshop:  workshop,
		Sdk:       sdkName,
		Name:      slot,
	})
	return r.slot
}
