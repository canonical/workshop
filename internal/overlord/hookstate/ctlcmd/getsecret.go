// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package ctlcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	internalerrors "github.com/canonical/workshop/internal/errors"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/overlord/secretstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

type getSecretCommand struct {
	baseCommand
	getSecretPositional `positional-args:"yes"`

	// GetSecret retrieves a secret for the resolved workshop and plug.
	GetSecret func(
		context.Context,
		*state.State,
		workshop.Project,
		sdk.PlugRef,
	) (secrets.Secret, error)

	// Systemd requests credential delivery semantics for systemd.
	Systemd bool `long:"systemd" description:"retrieve a systemd credential"`
}

type getSecretPositional struct {
	Secret string `positional-arg-name:"<SDK>.<secret>" required:"yes" description:"the secret to retrieve, in the form <SDK>.<secret>"`
}

const (
	// secretExitCodeMultipleMatches indicates an ambiguous secret lookup.
	secretExitCodeMultipleMatches = 4

	// secretExitCodeNotFound indicates that the configured secret is missing.
	secretExitCodeNotFound = 1

	// secretExitCodePlugNotConnected identifies an unconnected plug for
	// ordinary callers. Systemd receives a successful empty credential.
	secretExitCodePlugNotConnected = 3

	// secretExitCodeProviderLocked indicates that the store must be unlocked.
	secretExitCodeProviderLocked = 2

	// secretExitCodeSystemError indicates an unexpected retrieval failure.
	secretExitCodeSystemError = 255

	longGetSecretHelp = `
The get-secret command retrieves the value of a secret connected to the
workshop, identified as "<SDK>.<secret>" (e.g. "my-sdk.api-key").
`

	shortGetSecretHelp = "Get the value of a secret"
)

func init() {
	addCommand(
		"get-secret",
		shortGetSecretHelp,
		longGetSecretHelp,
		func() command {
			return &getSecretCommand{GetSecret: secretstate.GetSecret}
		},
	)
}

// parseSecretIdentifier splits an SDK-qualified secret plug identifier into
// its SDK and plug names.
func parseSecretIdentifier(identifier string) (sdkName, plugName string, err error) {
	sdkName, plugName, found := strings.Cut(identifier, ".")

	if !found || sdkName == "" || plugName == "" {
		return "", "", errors.New(
			`invalid secret identifier: expected "<SDK>.<secret>" with both names present`,
		)
	}
	err = sdk.ValidateName(sdkName)
	if err != nil {
		return "", "", fmt.Errorf(
			"invalid SDK name: expected at most %d characters using "+
				"lowercase letters, digits and single internal hyphens, "+
				"with at least one letter; the name agent and prefixes "+
				"try- and project- are reserved",
			sdk.MAX_SDK_NAME_LENGTH,
		)
	}
	err = sdk.ValidatePlugName(plugName)
	if err != nil {
		return "", "", errors.New(
			"invalid secret plug name: expected a lowercase letter " +
				"followed by lowercase letters or digits, optionally " +
				"separated by single hyphens",
		)
	}
	return sdkName, plugName, nil
}

// Execute runs the get-secret command, writing the secret value to stdout.
// With --systemd, an unconnected plug succeeds without a value and writes
// a diagnostic to stderr.
func (c *getSecretCommand) Execute(ctx context.Context, _ []string) error {
	sdkName, plugName, err := parseSecretIdentifier(c.Secret)
	if err != nil {
		return err
	}

	// Log the requested identifier only; never the resolved value.
	logger.Debugf("get-secret request for SDK %q plug %q", sdkName, plugName)

	hookContext, err := c.ensureContext()
	if err != nil {
		return err
	}

	identity, err := hookContext.WorkshopIdentity()
	if err != nil {
		return fmt.Errorf("resolving secret request identity: %w", err)
	}

	ref := sdk.PlugRef{
		Name:      plugName,
		ProjectId: identity.Project.ProjectId,
		Sdk:       sdkName,
		Workshop:  identity.Workshop,
	}

	value, err := c.GetSecret(
		ctx,
		hookContext.State(),
		identity.Project,
		ref,
	)

	if err != nil && c.Systemd {
		return c.systemdSecretRequestError(err)
	} else if err != nil {
		return secretRequestError(c.Secret, err)
	}
	defer value.Close()

	if c.stdout == nil {
		return nil
	}
	_, err = io.Copy(c.stdout, value)
	return err
}

// secretRequestError replaces recognised failures with an ordinary request's
// diagnostic and [CommandExitCodeError], discarding the domain error chain.
// Unknown failures retain their cause with exit code 255. A nil error stays nil.
func secretRequestError(identifier string, err error) error {
	switch {
	case errors.Is(err, secretstate.ErrorPlugNotConnected):
		return fmt.Errorf(
			"secret plug %q is not connected%w",
			identifier,
			CommandExitCodeError{ExitCode: secretExitCodePlugNotConnected},
		)
	case errors.Is(err, secrets.ErrorMultipleSecrets):
		return fmt.Errorf(
			"multiple secrets match plug %q; refine the secret slot%w",
			identifier,
			CommandExitCodeError{ExitCode: secretExitCodeMultipleMatches},
		)
	case errors.Is(err, secrets.ErrorProviderLocked):
		return fmt.Errorf(
			"cannot retrieve secret for plug %q: "+
				"unlock the secret provider and try again%w",
			identifier,
			CommandExitCodeError{ExitCode: secretExitCodeProviderLocked},
		)
	case errors.Is(err, secrets.ErrorSecretNotFound):
		return fmt.Errorf(
			"no secret found for plug %q%w",
			identifier,
			CommandExitCodeError{ExitCode: secretExitCodeNotFound},
		)
	default:
		return internalerrors.Add(
			err,
			CommandExitCodeError{ExitCode: secretExitCodeSystemError},
		)
	}
}

// systemdSecretRequestError treats an unconnected plug as a successful empty
// credential, reporting its diagnostic on stderr. Other failures use the
// same messages and exit codes as ordinary requests.
func (c *getSecretCommand) systemdSecretRequestError(err error) error {
	if errors.Is(err, secretstate.ErrorPlugNotConnected) {
		return c.errorf("secret plug %q is not connected\n", c.Secret)
	}
	return secretRequestError(c.Secret, err)
}
