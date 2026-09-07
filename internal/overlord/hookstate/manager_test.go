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

// TestContextReturnsActiveContext checks that cookie lookup returns the
// registered hook context without requiring persisted cookies.
func (s *managerSuite) TestContextReturnsActiveContext(c *C) {
	s.state.Lock()
	task := s.state.NewTask("run-hook", "Run test hook")
	s.state.Unlock()
	active, err := NewContext(task, s.state, &HookSetup{}, nil, "cookie-id")
	c.Assert(err, IsNil)
	s.manager.contexts = map[string]*Context{active.ID(): active}

	ctx, err := s.manager.Context(active.ID())

	c.Assert(err, IsNil)
	c.Check(ctx, Equals, active)
	c.Check(ctx.IsEphemeral(), Equals, false)
}

// TestContextRejectsUnknownCookie checks that an unregistered cookie returns
// an invalid-cookie error rather than a state lookup error.
func (s *managerSuite) TestContextRejectsUnknownCookie(c *C) {
	ctx, err := s.manager.Context("unknown-cookie")

	c.Check(ctx, IsNil)
	c.Check(err, ErrorMatches, "invalid workshop cookie requested")
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
