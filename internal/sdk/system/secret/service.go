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
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/secrets"
)

// DBusService retrieves secrets through the freedesktop Secret Service D-Bus
// API.
type DBusService struct {
	connect busConnector
}

// Request describes a lookup against a user's host Secret Service.
type Request struct {
	Attributes map[string]string
	Collection string
	UID        string
}

// Service retrieves secrets from a user's host Secret Service.
type Service interface {
	// Get retrieves the unique secret matching the request. On success,
	// ownership of the returned secret transfers to the caller, which must
	// consume or close it. It must honour context cancellation.
	Get(context.Context, Request) (secrets.Secret, error)
}

// secretValue follows the field order of the Secret Service Secret struct.
type secretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

const (
	collectionInterface = "org.freedesktop.Secret.Collection"
	itemInterface       = "org.freedesktop.Secret.Item"
	propertiesInterface = "org.freedesktop.DBus.Properties"
	serviceInterface    = "org.freedesktop.Secret.Service"
	sessionInterface    = "org.freedesktop.Secret.Session"
	servicePath         = dbus.ObjectPath("/org/freedesktop/secrets")
)

// Get retrieves the unique secret matching request from the host Secret
// Service. Non-default collections are selected by their label. The caller
// must consume or close the returned secret.
//
// The following errors may be expected:
//   - [ErrorCollectionAmbiguous]: several collections have the requested label.
//   - [ErrorCollectionLocked]: the selected collection is locked.
//   - [ErrorCollectionNotFound]: no collection has the requested name.
//   - [ErrorMultipleSecrets]: several secrets match the supplied attributes.
//   - [ErrorSecretNotFound]: no secret matches the supplied attributes.
func (s DBusService) Get(
	ctx context.Context,
	req Request,
) (secrets.Secret, error) {
	err := validateRequest(req)
	if err != nil {
		return secrets.Secret{}, err
	}

	conn, err := s.connect(ctx, req.UID)
	if err != nil {
		return secrets.Secret{}, err
	}
	defer conn.Close()

	var collection dbus.ObjectPath
	if req.Collection == "default" {
		collection, err = resolveDefaultCollection(ctx, conn)
		if err != nil {
			return secrets.Secret{}, fmt.Errorf(
				"resolving default secret collection: %w", err,
			)
		}
	} else {
		collection, err = findCollectionByLabel(
			ctx,
			conn,
			req.Collection,
		)
		if err != nil {
			return secrets.Secret{}, fmt.Errorf(
				"finding collection %q: %w", req.Collection, err,
			)
		}
	}

	locked, err := objectBoolProperty(
		ctx,
		conn,
		collection,
		collectionInterface+".Locked",
	)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"checking secret collection lock state: %w",
			err,
		)
	}
	if locked {
		return secrets.Secret{}, ErrorCollectionLocked
	}

	item, err := findUniqueItemByAttributes(
		ctx,
		conn,
		collection,
		req.Attributes,
	)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"finding secret in collection %q: %w",
			req.Collection,
			err,
		)
	}

	session, err := openSession(ctx, conn)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"opening secret service session: %w",
			err,
		)
	}

	value, getErr := getItem(ctx, conn, item, session)
	closeErr := closeSession(ctx, conn, session)
	if getErr != nil {
		return secrets.Secret{}, getErr
	}

	if closeErr != nil {
		clear(value)
		return secrets.Secret{}, closeErr
	}
	return secrets.NewSecret(value), nil
}

// NewDBusService creates a service that connects to each requesting user's
// session bus.
func NewDBusService() DBusService {
	return DBusService{connect: connectUserSessionBus}
}

func closeSession(
	ctx context.Context,
	conn busConnection,
	session dbus.ObjectPath,
) error {
	err := conn.Call(
		ctx,
		session,
		sessionInterface+".Close",
		nil,
	)
	if err != nil {
		return fmt.Errorf("closing secret service session: %w", err)
	}
	return nil
}

// findCollectionByLabel returns the unique collection whose label matches
// label.
//
// The following errors may be expected:
//   - [ErrorCollectionAmbiguous]: several collections have the requested label.
//   - [ErrorCollectionNotFound]: no collection has the requested label.
func findCollectionByLabel(
	ctx context.Context,
	conn busConnection,
	label string,
) (dbus.ObjectPath, error) {
	collections, err := objectPathsProperty(
		ctx,
		conn,
		servicePath,
		serviceInterface+".Collections",
	)
	if err != nil {
		return "", fmt.Errorf("listing secret collections: %w", err)
	}

	matches := make([]dbus.ObjectPath, 0, 1)
	for _, collection := range collections {
		collectionLabel, err := objectStringProperty(
			ctx,
			conn,
			collection,
			collectionInterface+".Label",
		)
		if err != nil {
			return "", fmt.Errorf(
				"reading label for secret collection %q: %w",
				collection,
				err,
			)
		}
		if collectionLabel == label {
			matches = append(matches, collection)
		}
	}

	switch len(matches) {
	case 0:
		return "", ErrorCollectionNotFound
	case 1:
		return matches[0], nil
	default:
		return "", ErrorCollectionAmbiguous
	}
}

// findUniqueItemByAttributes returns the unique item in collection matching
// attributes. Secret Service searches may return any number of items, so this
// function prevents callers from selecting a secret nondeterministically.
//
// The following errors may be expected:
//   - [ErrorMultipleSecrets]: several items match attributes.
//   - [ErrorSecretNotFound]: no item matches attributes.
func findUniqueItemByAttributes(
	ctx context.Context,
	conn busConnection,
	collection dbus.ObjectPath,
	attributes map[string]string,
) (dbus.ObjectPath, error) {
	var items []dbus.ObjectPath
	err := conn.Call(
		ctx,
		collection,
		collectionInterface+".SearchItems",
		[]any{attributes},
		&items,
	)
	if err != nil {
		return "", err
	}

	switch len(items) {
	case 0:
		return "", ErrorSecretNotFound
	case 1:
		return items[0], nil
	default:
		return "", ErrorMultipleSecrets
	}
}

func getItem(
	ctx context.Context,
	conn busConnection,
	item dbus.ObjectPath,
	session dbus.ObjectPath,
) ([]byte, error) {
	var value secretValue
	err := conn.Call(
		ctx,
		item,
		itemInterface+".GetSecret",
		[]any{session},
		&value,
	)
	if err != nil {
		return nil, fmt.Errorf("getting secret value: %w", err)
	}
	return value.Value, nil
}

func objectBoolProperty(
	ctx context.Context,
	conn busConnection,
	path dbus.ObjectPath,
	property string,
) (bool, error) {
	value, err := objectProperty(ctx, conn, path, property)
	if err != nil {
		return false, err
	}
	result, ok := value.Value().(bool)
	if !ok {
		return false, fmt.Errorf("property %q is not a boolean", property)
	}
	return result, nil
}

func objectPathsProperty(
	ctx context.Context,
	conn busConnection,
	path dbus.ObjectPath,
	property string,
) ([]dbus.ObjectPath, error) {
	value, err := objectProperty(ctx, conn, path, property)
	if err != nil {
		return nil, err
	}
	result, ok := value.Value().([]dbus.ObjectPath)
	if !ok {
		return nil, fmt.Errorf(
			"property %q is not an object path list",
			property,
		)
	}
	return result, nil
}

func objectProperty(
	ctx context.Context,
	conn busConnection,
	path dbus.ObjectPath,
	property string,
) (dbus.Variant, error) {
	separator := strings.LastIndexByte(property, '.')
	if separator < 1 || separator == len(property)-1 {
		return dbus.Variant{}, fmt.Errorf(
			"invalid D-Bus property name %q",
			property,
		)
	}
	iface := property[:separator]
	name := property[separator+1:]
	var value dbus.Variant
	err := conn.Call(
		ctx,
		path,
		propertiesInterface+".Get",
		[]any{iface, name},
		&value,
	)
	if err != nil {
		return dbus.Variant{}, err
	}
	return value, nil
}

func objectStringProperty(
	ctx context.Context,
	conn busConnection,
	path dbus.ObjectPath,
	property string,
) (string, error) {
	value, err := objectProperty(ctx, conn, path, property)
	if err != nil {
		return "", err
	}
	result, ok := value.Value().(string)
	if !ok {
		return "", fmt.Errorf("property %q is not a string", property)
	}
	return result, nil
}

// openSession negotiates a plain Secret Service session and returns its D-Bus
// object path. The plain algorithm adds no encryption beyond the authenticated
// local session bus. The caller must release the returned session with
// [closeSession].
func openSession(
	ctx context.Context,
	conn busConnection,
) (dbus.ObjectPath, error) {
	var output dbus.Variant
	var session dbus.ObjectPath
	err := conn.Call(
		ctx,
		servicePath,
		serviceInterface+".OpenSession",
		[]any{"plain", dbus.MakeVariant("")},
		&output,
		&session,
	)
	if err != nil {
		return "", err
	}
	return session, nil
}

// resolveDefaultCollection resolves the collection assigned to the Secret
// Service default alias.
func resolveDefaultCollection(
	ctx context.Context,
	conn busConnection,
) (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	err := conn.Call(
		ctx,
		servicePath,
		serviceInterface+".ReadAlias",
		[]any{"default"},
		&path,
	)

	if err != nil {
		return "", err
	}

	if path == "/" || path == "" {
		return "", ErrorCollectionNotFound
	}
	return path, nil
}

// validateRequest checks that request identifies a host user, collection and
// at least one named search attribute.
func validateRequest(request Request) error {
	if strings.TrimSpace(request.UID) == "" {
		return errors.New("secret request user ID is missing")
	}
	if strings.TrimSpace(request.Collection) == "" {
		return errors.New("secret request collection is missing")
	}
	if len(request.Attributes) == 0 {
		return errors.New("secret request attributes are missing")
	}
	for name := range request.Attributes {
		if strings.TrimSpace(name) == "" {
			return errors.New("secret request attribute name is empty")
		}
	}
	return nil
}
