/*
 * Copyright (C) 2019 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package ctlcmd_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
	"github.com/canonical/workshop/internal/overlord/hookstate/hooktest"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/testutil"
)

type healthSuite struct {
	testutil.BaseTest
	state       *state.State
	mockContext *hookstate.Context
	mockHandler *hooktest.MockHandler
}

var _ = check.Suite(&healthSuite{})

func (s *healthSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)

	s.mockHandler = hooktest.NewMockHandler()

	s.state = state.New(nil)
	s.state.Lock()
	defer s.state.Unlock()
	task := s.state.NewTask("test-task", "my test task")
	setup := &hookstate.HookSetup{Sdk: "test-sdk", HookType: hookstate.CheckHealth}

	ctx, err := hookstate.NewContext(task, s.state, setup, s.mockHandler, "")
	c.Assert(err, check.IsNil)
	s.mockContext = ctx
}

func (s *healthSuite) TestBadArgs(c *check.C) {
	type tableT struct {
		args []string
		err  string
	}
	table := []tableT{
		{
			[]string{"set-health"},
			"the required argument `<status>` was not provided",
		}, {
			[]string{"set-health", "bananas", "message"},
			`invalid status "bananas".*`,
		}, {
			[]string{"set-health", "unknown", "message"},
			`status cannot be manually set to "unknown"`,
		}, {
			[]string{"set-health", "okay", "message"},
			`when status is "okay", message and code must be empty`,
		}, {
			[]string{"set-health", "okay", "--code=what"},
			`when status is "okay", message and code must be empty`,
		}, {
			[]string{"set-health", "waiting"},
			`when status is not "okay", message is required`,
		}, {
			[]string{"set-health", "waiting", "message", "--code=xx"},
			`code must have between 3 and 30 characters, got 2`,
		}, {
			[]string{"set-health", "waiting", "message", "--code=abcdefghijklmnopqrstuvwxyz12345"},
			`code must have between 3 and 30 characters, got 31`,
		}, {
			[]string{"set-health", "waiting", "message", "--code=☠☢☣💣💢🐍✴👿‼"},
			`code must have between 3 and 30 characters, got 31`,
		}, {
			[]string{"set-health", "waiting", "message", "--code=123"},
			`invalid code "123".*`,
		}, {
			[]string{"set-health", "waiting", "what"},
			`message must be at least 7 characters long \(got 4\)`,
		}, {
			[]string{"set-health", "waiting", "áéíóú"},
			`message must be at least 7 characters long \(got 5\)`,
		}, {
			[]string{"set-health", "waiting", "message"},
			`cannot invoke workshopctl operation commands \(here "set-health"\) from outside of a workshop`,
		},
	}

	for i, t := range table {
		_, _, err := ctlcmd.Run(nil, t.args, 0)
		c.Check(err, check.ErrorMatches, t.err, check.Commentf("%d", i))
	}
}

func (s *healthSuite) TestRegularRun(c *check.C) {
	_, _, err := ctlcmd.Run(s.mockContext, []string{"set-health", "waiting", "message", "--code=some-code"}, 0)
	c.Assert(err, check.IsNil)

	s.mockContext.Lock()
	defer s.mockContext.Unlock()

	var health healthstate.HealthCheck
	c.Assert(s.mockContext.Get("health", &health), check.IsNil)
	c.Check(health.CheckResult, check.Equals, healthstate.CheckWaiting)
	c.Check(health.Message, check.Equals, "message")
	c.Check(health.Code, check.Equals, "some-code")
}

func (s *healthSuite) TestMessageTruncation(c *check.C) {
	_, _, err := ctlcmd.Run(s.mockContext, []string{"set-health", "error", "Sometimes messages will get a little bit too verbose and this can lead to some rather nasty UX (as well as potential memory problems in extreme cases) so we kinda have to deal with that", "--code=some-code"}, 0)
	c.Assert(err, check.IsNil)

	s.mockContext.Lock()
	defer s.mockContext.Unlock()

	var health healthstate.HealthCheck
	c.Assert(s.mockContext.Get("health", &health), check.IsNil)
	c.Check(health.CheckResult, check.Equals, healthstate.CheckError)
	c.Check(health.Message, check.Equals, "Sometimes messages will get a little bit too verbose and this can lea…")
	c.Check(health.Code, check.Equals, "some-code")
}

func (s *healthSuite) TestRegularRunIncorrectHook(c *check.C) {
	setup := &hookstate.HookSetup{Sdk: "test-sdk", HookType: hookstate.SetupBase}
	task, _ := s.mockContext.Task()
	ctx, err := hookstate.NewContext(task, s.state, setup, s.mockHandler, "")
	c.Assert(err, check.IsNil)

	_, _, err = ctlcmd.Run(ctx, []string{"set-health", "waiting", "message", "--code=some-code"}, 0)
	c.Assert(err, check.ErrorMatches, `"set-health" is only allowed from a "check-health" hook`)

	s.mockContext.Lock()
	defer s.mockContext.Unlock()

	var health healthstate.HealthState
	c.Assert(s.mockContext.Get("health", &health), testutil.ErrorIs, state.ErrNoState)
}
