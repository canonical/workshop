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
	"io"
	"testing"

	"github.com/godbus/dbus/v5"
	"gopkg.in/check.v1"
)

// dbusServiceSuite tests secret retrieval through [DBusService].
type dbusServiceSuite struct{}

var _ = check.Suite(&dbusServiceSuite{})

func Test(t *testing.T) {
	check.TestingT(t)
}

// TestConnectUserSessionBusRejectsInvalidUID checks malformed user IDs fail
// before a D-Bus connection is attempted.
func (s *dbusServiceSuite) TestConnectUserSessionBusRejectsInvalidUID(c *check.C) {
	_, err := connectUserSessionBus(context.Background(), "not-a-uid")

	c.Check(err, check.ErrorMatches,
		`parsing user ID "not-a-uid": strconv.ParseUint:.*`)
}

// TestGet checks that the service performs a complete lookup and transfers
// ownership of the returned value to the caller.
func (s *dbusServiceSuite) TestGet(c *check.C) {
	calls := make([]string, 0, 6)
	conn := &fakeBusConnection{}
	conn.call = func(
		_ context.Context,
		path dbus.ObjectPath,
		method string,
		args []any,
		results ...any,
	) error {
		calls = append(calls, method)
		switch method {
		case serviceInterface + ".ReadAlias":
			c.Check(path, check.Equals, servicePath)
			c.Check(args, check.DeepEquals, []any{"default"})
			*results[0].(*dbus.ObjectPath) = "/collection/default"
		case propertiesInterface + ".Get":
			c.Check(path, check.Equals,
				dbus.ObjectPath("/collection/default"))
			c.Check(args, check.DeepEquals,
				[]any{collectionInterface, "Locked"})
			*results[0].(*dbus.Variant) = dbus.MakeVariant(false)
		case collectionInterface + ".SearchItems":
			c.Check(path, check.Equals,
				dbus.ObjectPath("/collection/default"))
			*results[0].(*[]dbus.ObjectPath) = []dbus.ObjectPath{
				"/collection/default/item/1",
			}
		case serviceInterface + ".OpenSession":
			*results[0].(*dbus.Variant) = dbus.MakeVariant("")
			*results[1].(*dbus.ObjectPath) = "/session/1"
		case itemInterface + ".GetSecret":
			*results[0].(*secretValue) = secretValue{
				ContentType: "text/plain",
				Session:     "/session/1",
				Value:       []byte("test-secret"),
			}
		case sessionInterface + ".Close":
			c.Check(path, check.Equals, dbus.ObjectPath("/session/1"))
		default:
			c.Fatalf("unexpected D-Bus method %q", method)
		}
		return nil
	}

	service := DBusService{
		connect: func(
			_ context.Context,
			uid string,
		) (busConnection, error) {
			c.Check(uid, check.Equals, "1000")
			return conn, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example"},
		Collection: "default",
		UID:        "1000",
	}

	value, err := service.Get(context.Background(), request)
	c.Assert(err, check.IsNil)
	defer value.Close()

	contents, err := io.ReadAll(value)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte("test-secret"))
	c.Check(conn.closed, check.Equals, true)
	c.Check(calls, check.DeepEquals, []string{
		serviceInterface + ".ReadAlias",
		propertiesInterface + ".Get",
		collectionInterface + ".SearchItems",
		serviceInterface + ".OpenSession",
		itemInterface + ".GetSecret",
		sessionInterface + ".Close",
	})
}

// TestGetRejectsInvalidRequest checks Get validates the request before
// attempting to connect to the user's session bus.
func (s *dbusServiceSuite) TestGetRejectsInvalidRequest(c *check.C) {
	service := DBusService{
		connect: func(
			context.Context,
			string,
		) (busConnection, error) {
			c.Fatal("invalid request must not connect to D-Bus")
			return nil, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example"},
		UID:        "1000",
	}

	_, err := service.Get(context.Background(), request)

	c.Check(err, check.ErrorMatches, "secret request collection is missing")
}
