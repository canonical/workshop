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

package secret

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// fakeBusConnection records D-Bus calls without a real session bus.
type fakeBusConnection struct {
	call   func(context.Context, dbus.ObjectPath, string, []any, ...any) error
	closed bool
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
