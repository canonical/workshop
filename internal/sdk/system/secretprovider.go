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
	"errors"
	"fmt"
	"os/user"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system/secret"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// SecretProvider resolves system SDK secret slots through a host secret service.
type SecretProvider struct {
	service SecretService
	slots   SecretSlotLookup
}

// SecretService retrieves host secrets for the system SDK secret provider.
type SecretService interface {
	// Get retrieves the secret matching the request for its user ID.
	// The caller must consume or close the returned secret.
	// Implementations must honour context cancellation.
	//
	// The following errors may be expected:
	//   - [secret.ErrorCollectionAmbiguous] when multiple collections have the
	//     requested label.
	//   - [secret.ErrorCollectionLocked] when the requested collection is locked.
	//   - [secret.ErrorCollectionNotFound] when the requested collection does
	//     not exist.
	//   - [secret.ErrorMultipleSecrets] when multiple secrets match the
	//     requested attributes.
	//   - [secret.ErrorSecretNotFound] when no secret matches the requested
	//     attributes.
	Get(context.Context, secret.Request) (secrets.Secret, error)
}

// NewSecretProvider creates a provider using slots for configuration and service
// for secret retrieval.
func NewSecretProvider(
	slots SecretSlotLookup,
	service SecretService,
) SecretProvider {
	return SecretProvider{service: service, slots: slots}
}

// Resolve looks up slot configuration and retrieves its secret for the host
// user identified by [workshop.ContextUser]. The caller must consume or close
// the returned secret.
//
// The following errors may be expected:
//   - [secrets.ErrorMultipleSecrets] when multiple secrets match the request.
//   - [secrets.ErrorProviderLocked] when the requested collection is locked.
//   - [secrets.ErrorSecretNotFound] when the requested collection does not
//     exist or no secret matches the request.
//   - [secrets.ErrorUserNotFound] when the context user is missing, empty or
//     not a string, or the named user does not exist.
func (p SecretProvider) Resolve(
	ctx context.Context,
	slot sdk.SlotRef,
) (secrets.Secret, error) {
	err := ctx.Err()
	if err != nil {
		return secrets.Secret{}, err
	}

	config, err := p.slots.Lookup(ctx, slot)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"looking up secret slot configuration: %w", err,
		)
	}

	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok || username == "" {
		return secrets.Secret{}, secrets.ErrorUserNotFound
	}

	account, err := osutil.UserLookup(username)
	_, unknownUser := errors.AsType[user.UnknownUserError](err)
	if unknownUser {
		return secrets.Secret{}, fmt.Errorf(
			"looking up secret request user %q: %w",
			username,
			secrets.ErrorUserNotFound,
		)
	} else if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"looking up secret request user %q: %w", username, err,
		)
	}

	value, err := p.service.Get(ctx, secret.Request{
		Attributes: config.Attributes,
		Collection: config.Collection,
		UID:        account.Uid,
	})
	switch {
	case errors.Is(err, secret.ErrorCollectionLocked):
		return secrets.Secret{}, fmt.Errorf(
			"retrieving system secret: %w",
			secrets.ErrorProviderLocked,
		)
	case errors.Is(err, secret.ErrorCollectionNotFound):
		return secrets.Secret{}, fmt.Errorf(
			"retrieving system secret from missing collection %q: %w",
			config.Collection,
			secrets.ErrorSecretNotFound,
		)
	case errors.Is(err, secret.ErrorMultipleSecrets):
		return secrets.Secret{}, fmt.Errorf(
			"retrieving system secret: %w",
			secrets.ErrorMultipleSecrets,
		)
	case errors.Is(err, secret.ErrorSecretNotFound):
		return secrets.Secret{}, fmt.Errorf(
			"retrieving system secret: %w",
			secrets.ErrorSecretNotFound,
		)
	case err != nil:
		return secrets.Secret{}, fmt.Errorf("retrieving system secret: %w", err)
	}
	return value, nil
}
