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
	"fmt"
	"io"

	"github.com/canonical/workshop/internal/overlord/hookstate"
)

// NewGetSecretCommand exposes command construction without running the parser.
func NewGetSecretCommand(
	hookContext *hookstate.Context,
	identifier string,
	stdout io.Writer,
	stderr io.Writer,
) *getSecretCommand {
	command := &getSecretCommand{
		getSecretPositional: getSecretPositional{Secret: identifier},
	}
	command.setName("get-secret")
	command.setContext(hookContext)
	command.setStdout(stdout)
	command.setStderr(stderr)
	return command
}

func AddMockCommand(name string) *MockCommand {
	return addMockCmd(name, false)
}

func addMockCmd(name string, hidden bool) *MockCommand {
	mockCommand := NewMockCommand()
	cmd := addCommand(name, "", "", func() command { return mockCommand })
	cmd.hidden = hidden
	return mockCommand
}

func AddHiddenMockCommand(name string) *MockCommand {
	return addMockCmd(name, true)
}

func RemoveCommand(name string) {
	delete(commands, name)
}

func NewMockCommand() *MockCommand {
	return &MockCommand{
		ExecuteError: false,
	}
}

func (c *MockCommand) Execute(ctx context.Context, args []string) error {
	c.Args = args

	if c.ExecuteFunc != nil {
		return c.ExecuteFunc(ctx, args)
	}

	if c.FakeStdout != "" {
		err := c.printf("%s", c.FakeStdout)
		if err != nil {
			return err
		}
	}

	if c.FakeStderr != "" {
		err := c.errorf("%s", c.FakeStderr)
		if err != nil {
			return err
		}
	}

	if c.ExecuteError {
		return fmt.Errorf("failed at user request")
	}

	return nil
}

type MockCommand struct {
	baseCommand

	ExecuteError bool
	FakeStdout   string
	FakeStderr   string
	ExecuteFunc  func(context.Context, []string) error

	Args []string
}
