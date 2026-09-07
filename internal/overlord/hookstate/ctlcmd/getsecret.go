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

	"github.com/canonical/workshop/internal/logger"
)

type getSecretCommand struct {
	baseCommand
	getSecretPositional `positional-args:"yes"`
}

type getSecretPositional struct {
	Secret string `positional-arg-name:"<SDK>.<secret>" required:"yes" description:"the secret to retrieve, in the form <SDK>.<secret>"`
}

const (
	// hardCodedSecret is a placeholder secret value returned until secret
	// resolution via workshopd is implemented.
	hardCodedSecret = "workshop-placeholder-secret"

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

// Execute runs the get-secret command, writing the secret value to stdout.
func (c *getSecretCommand) Execute(context.Context, []string) error {
	// Log the requested identifier only; never the resolved value.
	logger.Debugf("get-secret request for %q", c.Secret)

	// TODO: resolve the requested secret via workshopd instead of
	// returning a hard-coded value.
	return c.printf("%s", hardCodedSecret)
}
