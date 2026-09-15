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
	"io"
	"testing"

	"github.com/godbus/dbus/v5"
	"gopkg.in/check.v1"
)

type fakeBusConnection struct {
	call   func(context.Context, dbus.ObjectPath, string, []any, ...any) error
	closed bool
}

// serviceSuite tests secret retrieval through [DBusService].
type serviceSuite struct{}

var _ = check.Suite(&serviceSuite{})

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

// rejectingService returns a service that fails the test if request
// validation reaches the D-Bus connection boundary.
func rejectingService(c *check.C) DBusService {
	return DBusService{
		connect: func(
			context.Context,
			string,
		) (busConnection, error) {
			c.Error("invalid request must not connect to D-Bus")
			return nil, nil
		},
	}
}

func Test(t *testing.T) {
	check.TestingT(t)
}

// TestConnectUserSessionBusRejectsInvalidUID checks malformed user IDs fail
// before a D-Bus connection is attempted.
func (s *serviceSuite) TestConnectUserSessionBusRejectsInvalidUID(c *check.C) {
	_, err := connectUserSessionBus(context.Background(), "not-a-uid")

	c.Check(err, check.ErrorMatches,
		`parsing user ID "not-a-uid": strconv.ParseUint:.*`)
}

// TestGet checks that the service performs a complete lookup and transfers
// ownership of the returned value to the caller.
func (s *serviceSuite) TestGet(c *check.C) {
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

// TestGetRejectsEmptyAttributeName checks an empty search attribute name fails
// before connecting to the user's session bus.
func (s *serviceSuite) TestGetRejectsEmptyAttributeName(c *check.C) {
	request := Request{
		Attributes: map[string]string{"": "example"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := rejectingService(c).Get(context.Background(), request)

	c.Check(err, check.ErrorMatches,
		"secret request attribute name is empty")
}

// TestGetRejectsMissingAttributes checks missing search attributes fail before
// connecting to the user's session bus.
func (s *serviceSuite) TestGetRejectsMissingAttributes(c *check.C) {
	request := Request{
		Collection: "default",
		UID:        "1000",
	}

	_, err := rejectingService(c).Get(context.Background(), request)

	c.Check(err, check.ErrorMatches, "secret request attributes are missing")
}

// TestGetRejectsMissingCollection checks a missing collection fails before
// connecting to the user's session bus.
func (s *serviceSuite) TestGetRejectsMissingCollection(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example"},
		UID:        "1000",
	}

	_, err := rejectingService(c).Get(context.Background(), request)

	c.Check(err, check.ErrorMatches, "secret request collection is missing")
}

// TestGetRejectsMissingUID checks a missing user ID fails before connecting to
// the user's session bus.
func (s *serviceSuite) TestGetRejectsMissingUID(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example"},
		Collection: "default",
	}

	_, err := rejectingService(c).Get(context.Background(), request)

	c.Check(err, check.ErrorMatches, "secret request user ID is missing")
}
