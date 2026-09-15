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
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/dirs"
)

// busConnection provides the D-Bus operations required by [DBusService].
type busConnection interface {
	// Call invokes a method and stores its reply body in the supplied results.
	Call(
		context.Context,
		dbus.ObjectPath,
		string,
		[]any,
		...any,
	) error

	// Close closes the connection to the user's D-Bus session bus.
	Close() error
}

// busConnector connects to the D-Bus session bus for the supplied user ID.
type busConnector func(context.Context, string) (busConnection, error)

// dbusConnection adapts a [dbus.Conn] to [busConnection].
type dbusConnection struct {
	conn *dbus.Conn
}

const (
	// dbusFlagNone indicates that a call uses no D-Bus message flags.
	dbusFlagNone dbus.Flags = 0

	// secretServiceBusName is the well-known D-Bus name of the freedesktop
	// Secret Service.
	secretServiceBusName = "org.freedesktop.secrets"
)

// Call invokes method on the Secret Service object at path and stores the
// reply body in results.
func (c dbusConnection) Call(
	ctx context.Context,
	path dbus.ObjectPath,
	method string,
	args []any,
	results ...any,
) error {
	object := c.conn.Object(secretServiceBusName, path)
	call := object.CallWithContext(ctx, method, dbusFlagNone, args...)
	if call.Err != nil {
		return call.Err
	}
	return call.Store(results...)
}

// Close closes the underlying connection to the user's D-Bus session bus.
func (c dbusConnection) Close() error {
	return c.conn.Close()
}

// connectUserSessionBus connects to the existing D-Bus session bus belonging
// to uid. The connection authenticates using the supplied numeric user ID.
func connectUserSessionBus(
	ctx context.Context,
	uid string,
) (busConnection, error) {
	_, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parsing user ID %q: %w", uid, err)
	}

	address := "unix:path=" + filepath.Join(
		dirs.XdgRuntimeDirBase,
		uid,
		"bus",
	)
	conn, err := dbus.Connect(
		address,
		dbus.WithAuth(dbus.AuthExternal(uid)),
		dbus.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"connecting to session bus for user ID %q: %w",
			uid,
			err,
		)
	}
	return dbusConnection{conn: conn}, nil
}
