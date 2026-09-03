/*
 * Copyright (C) 2016 Canonical Ltd
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
	"context"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
	"github.com/canonical/workshop/internal/overlord/hookstate/hooktest"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/testutil"
)

func Test(t *testing.T) { TestingT(t) }

type ctlcmdSuite struct {
	mockContext *hookstate.Context
}

var _ = Suite(&ctlcmdSuite{})

func (s *ctlcmdSuite) SetUpTest(c *C) {
	handler := hooktest.NewMockHandler()

	state := state.New(nil)
	state.Lock()
	defer state.Unlock()

	task := state.NewTask("test-task", "my test task")
	setup := &hookstate.HookSetup{Sdk: "test-sdk", HookType: hookstate.SetupBase}

	var err error
	s.mockContext, err = hookstate.NewContext(task, task.State(), setup, handler, "")
	c.Assert(err, IsNil)
}

func (s *ctlcmdSuite) TestNonExistingCommand(c *C) {
	stdout, stderr, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"foo"}, 0)
	c.Check(string(stdout), Equals, "")
	c.Check(string(stderr), Equals, "")
	c.Check(err, ErrorMatches, ".*[Uu]nknown command.*")
}

func (s *ctlcmdSuite) TestCommandOutput(c *C) {
	mockCommand := ctlcmd.AddMockCommand("mock")
	defer ctlcmd.RemoveCommand("mock")

	mockCommand.FakeStdout = "test stdout"
	mockCommand.FakeStderr = "test stderr"

	stdout, stderr, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"mock", "foo"}, 0)
	c.Check(err, IsNil)
	c.Check(string(stdout), Equals, "test stdout")
	c.Check(string(stderr), Equals, "test stderr")
	c.Check(mockCommand.Args, DeepEquals, []string{"foo"})
}

// TestRunPassesContextToCommand verifies that Run forwards its context to the
// selected command. Commands rely on this contract to access request-scoped
// values during Execute.
func (s *ctlcmdSuite) TestRunPassesContextToCommand(c *C) {
	mockCommand := ctlcmd.AddMockCommand("mock")
	defer ctlcmd.RemoveCommand("mock")

	key := struct{}{}
	ctx := context.WithValue(context.Background(), key, "test")
	called := false
	mockCommand.ExecuteFunc = func(ctx context.Context, _ []string) error {
		called = true
		c.Check(ctx.Value(key), Equals, "test")
		return nil
	}

	_, _, err := ctlcmd.Run(ctx, s.mockContext, []string{"mock"}, 0)

	c.Check(err, IsNil)
	c.Check(called, Equals, true)
}

func (s *ctlcmdSuite) TestHiddenCommand(c *C) {
	ctlcmd.AddHiddenMockCommand("mock-hidden")
	ctlcmd.AddMockCommand("mock-shown")
	defer ctlcmd.RemoveCommand("mock-hidden")
	defer ctlcmd.RemoveCommand("mock-shown")

	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"--help"}, 0)
	// help message output is returned as *flags.Error with
	// Type as flags.ErrHelp
	c.Assert(err, FitsTypeOf, &flags.Error{})
	c.Check(err.(*flags.Error).Type, Equals, flags.ErrHelp)
	// workshopctl is mentioned (not workshopd)
	c.Check(err.Error(), testutil.Contains, "workshopctl")
	// mock-shown is in the help message
	c.Check(err.Error(), testutil.Contains, "  mock-shown\n")
	// mock-hidden is not in the help message
	c.Check(err.Error(), Not(testutil.Contains), "  mock-hidden\n")
}

func (s *ctlcmdSuite) TestRootRequiredCommandFailure(c *C) {
	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"start"}, 1000)

	c.Check(err, FitsTypeOf, &ctlcmd.ForbiddenCommandError{})
	c.Check(err.Error(), Equals, `cannot use "start" with uid 1000, try with sudo`)
}

func (s *ctlcmdSuite) TestRunNoArgsFailure(c *C) {
	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{}, 0)
	c.Check(err, NotNil)
}

func (s *ctlcmdSuite) TestRunOnlyHelp(c *C) {
	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"-h"}, 1000)
	c.Check(err, NotNil)
	c.Assert(strings.HasPrefix(err.Error(), "Usage:"), Equals, true)

	_, _, err = ctlcmd.Run(context.TODO(), s.mockContext, []string{"--help"}, 1000)
	c.Check(err, NotNil)
	c.Assert(strings.HasPrefix(err.Error(), "Usage:"), Equals, true)
}

func (s *ctlcmdSuite) TestRunHelpAtAnyPosition(c *C) {
	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"set-health", "a", "-h"}, 1000)
	c.Check(err, NotNil)
	c.Assert(strings.HasPrefix(err.Error(), "Usage:"), Equals, true)

	_, _, err = ctlcmd.Run(context.TODO(), s.mockContext, []string{"set-health", "a", "b", "--help"}, 1000)
	c.Check(err, NotNil)
	c.Assert(strings.HasPrefix(err.Error(), "Usage:"), Equals, true)
}

func (s *ctlcmdSuite) TestRunNonRootAllowedCommandWithAllowedCmdAsArg(c *C) {
	// this test protects us against a future refactor introducing a bug that allows
	// a root-only command to run without root if an arg is in the nonRootAllowed list
	_, _, err := ctlcmd.Run(context.TODO(), s.mockContext, []string{"set", "get", "a"}, 1000)
	c.Check(err, FitsTypeOf, &ctlcmd.ForbiddenCommandError{})
	c.Check(err.Error(), Equals, `cannot use "set" with uid 1000, try with sudo`)
}
