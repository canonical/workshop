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

package ctlcmd_test

import (
	"context"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
)

// getSecretSuite tests the get-secret workshopctl command, which currently
// returns a hard-coded placeholder secret value.
type getSecretSuite struct{}

var _ = check.Suite(&getSecretSuite{})

func (s *getSecretSuite) TestGetSecret(c *check.C) {
	stdout, stderr, err := ctlcmd.Run(context.TODO(), nil, []string{"get-secret", "ollama.ollama-api-key"}, 0)
	c.Assert(err, check.IsNil)
	c.Check(string(stdout), check.Equals, "workshop-placeholder-secret")
	c.Check(string(stderr), check.Equals, "")
}

// TestGetSecretMissingArg checks that get-secret requires a secret
// identifier argument.
func (s *getSecretSuite) TestGetSecretMissingArg(c *check.C) {
	_, _, err := ctlcmd.Run(context.TODO(), nil, []string{"get-secret"}, 0)
	c.Check(err, check.ErrorMatches, ".*the required argument `<SDK>.<secret>` was not provided.*")
}

// TestGetSecretNonRoot checks that get-secret is allowed without root, as
// both the socket-activated systemd path and SDK wrapper scripts invoke it
// as the workshop user.
func (s *getSecretSuite) TestGetSecretNonRoot(c *check.C) {
	stdout, _, err := ctlcmd.Run(context.TODO(), nil, []string{"get-secret", "ollama.ollama-api-key"}, 1000)
	c.Assert(err, check.IsNil)
	c.Check(string(stdout), check.Equals, "workshop-placeholder-secret")
}
