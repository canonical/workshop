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
	"strings"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/overlord/secretstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type getSecretCommand struct {
	baseCommand
	getSecretPositional `positional-args:"yes"`
}

type getSecretPositional struct {
	Secret string `positional-arg-name:"<SDK>.<secret>" required:"yes" description:"the secret to retrieve, in the form <SDK>.<secret>"`
}

const (
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
			return &getSecretCommand{}
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

	// TODO: use the validated workshop identity before enabling real providers.
	project := workshop.Project{
		Path:      "/project",
		ProjectId: "placeholder-project",
	}
	ref := sdk.PlugRef{
		Name:      plugName,
		ProjectId: project.ProjectId,
		Sdk:       sdkName,
		Workshop:  "placeholder-workshop",
	}

	value, err := secretstate.GetSecret(ctx, hookContext.State(), project, ref)

	if err != nil {
		return err
	}
	return c.printf("%s", value)
}
