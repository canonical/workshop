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

package hookstate

import (
	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/state"
)

type managerSuite struct {
	state   *state.State
	manager *HookManager
}

var _ = Suite(&managerSuite{})

func (s *managerSuite) SetUpTest(*C) {
	s.state = state.New(nil)
	s.manager = &HookManager{state: s.state}
}

// TestNewEphemeralContext checks that the manager creates a context with its
// state but without a hook task, which is the contract required by
// cookie-less workshopctl requests after their instance ID is validated.
func (s *managerSuite) TestNewEphemeralContext(c *C) {
	ctx, err := s.manager.NewEphemeralContext()

	c.Assert(err, IsNil)
	c.Check(ctx.IsEphemeral(), Equals, true)
	c.Check(ctx.State(), Equals, s.state)
	c.Check(ctx.ID(), Not(Equals), "")
}
