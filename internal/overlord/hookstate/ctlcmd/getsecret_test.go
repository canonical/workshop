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
	"errors"
	"strings"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
	"github.com/canonical/workshop/internal/overlord/secretstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// getSecretResult carries command output back from the request goroutine.
type getSecretResult struct {
	err    error
	stderr []byte
	stdout []byte
}

// getSecretSuite tests secret retrieval through the state task runner.
type getSecretSuite struct {
	backend *secretStateBackend
	hookCtx *hookstate.Context
	runner  *state.TaskRunner
	st      *state.State
}

var _ = check.Suite(&getSecretSuite{})

// checkSuccess verifies the scheduled identity and returned secret value.
func (s *getSecretSuite) checkSuccess(
	c *check.C,
	results <-chan getSecretResult,
	sdkName string,
	plugName string,
) {
	delay := <-s.backend.ensureBefore
	c.Check(delay, check.Equals, time.Duration(0))
	err := s.runner.Ensure()
	c.Assert(err, check.IsNil)
	s.runner.Wait()
	result := <-results
	c.Assert(result.err, check.IsNil)
	c.Check(string(result.stdout), check.Equals, "workshop-placeholder-secret")
	c.Check(string(result.stderr), check.Equals, "")

	s.st.Lock()
	defer s.st.Unlock()
	changes := s.st.Changes()
	c.Assert(changes, check.HasLen, 1)
	change := changes[0]
	c.Check(change.Kind(), check.Equals, "get-secret")
	var user, projectID string
	c.Assert(change.Get("user", &user), check.IsNil)
	c.Check(user, check.Equals, "test-user")
	c.Assert(change.Get("project-id", &projectID), check.IsNil)
	c.Check(projectID, check.Equals, "placeholder-project")
	tasks := change.Tasks()
	c.Assert(tasks, check.HasLen, 1)
	task := tasks[0]
	c.Check(task.Kind(), check.Equals, "get-secret")
	c.Check(task.Status(), check.Equals, state.DoneStatus)
	var project workshop.Project
	c.Assert(task.Get("project", &project), check.IsNil)
	c.Check(project, check.DeepEquals, workshop.Project{
		Path:      "/project",
		ProjectId: "placeholder-project",
	})
	var actualSDK, actualPlug, workshopName string
	c.Assert(task.Get("sdk", &actualSDK), check.IsNil)
	c.Check(actualSDK, check.Equals, sdkName)
	c.Assert(task.Get("plug", &actualPlug), check.IsNil)
	c.Check(actualPlug, check.Equals, plugName)
	c.Assert(task.Get("workshop", &workshopName), check.IsNil)
	c.Check(workshopName, check.Equals, "placeholder-workshop")

}

// SetUpTest wires a real hook context and secret task handler.
func (s *getSecretSuite) SetUpTest(c *check.C) {
	s.backend = &secretStateBackend{
		ensureBefore: make(chan time.Duration, 1),
	}
	s.st = state.New(s.backend)
	s.runner = state.NewTaskRunner(s.st)
	secretstate.New(s.runner)
	var err error
	s.hookCtx, err = hookstate.NewContext(
		nil,
		s.st,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
}

// start runs the command without blocking the test's task runner.
func (s *getSecretSuite) start(
	ctx context.Context,
	identifier string,
	uid uint32,
) <-chan getSecretResult {
	results := make(chan getSecretResult, 1)
	go func() {
		stdout, stderr, err := ctlcmd.Run(
			ctx,
			s.hookCtx,
			[]string{"get-secret", identifier},
			uid,
		)
		results <- getSecretResult{err: err, stderr: stderr, stdout: stdout}
	}()
	return results
}

// TearDownTest stops any task handlers before the next case.
func (s *getSecretSuite) TearDownTest(c *check.C) {
	s.runner.Stop()
}

// TestGetSecret checks root requests retrieve and consume the task result.
func (s *getSecretSuite) TestGetSecret(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	results := s.start(ctx, "ollama.ollama-api-key", 0)
	s.checkSuccess(c, results, "ollama", "ollama-api-key")
}

// TestGetSecretCancelled checks request cancellation reaches secretstate
// without scheduling a task or writing a secret.
func (s *getSecretSuite) TestGetSecretCancelled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	cancel()

	result := <-s.start(ctx, "ollama.ollama-api-key", 0)
	c.Check(errors.Is(result.err, context.Canceled), check.Equals, true)
	c.Check(string(result.stdout), check.Equals, "")
	c.Check(string(result.stderr), check.Equals, "")
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), check.HasLen, 0)
	c.Check(s.backend.ensureBefore, check.HasLen, 0)
}

// TestGetSecretInvalidFormat checks that a missing separator reports the
// expected identifier structure without echoing the supplied value.
func (s *getSecretSuite) TestGetSecretInvalidFormat(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identifier := "private-identifier"

	_, _, err := ctlcmd.Run(ctx, nil, []string{"get-secret", identifier}, 0)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		`invalid secret identifier: expected "<SDK>.<secret>" `+
			"with both names present")
	c.Check(strings.Contains(err.Error(), identifier), check.Equals, false)
}

// TestGetSecretInvalidSDK checks that SDK validation reports naming rules
// without exposing the rejected SDK or qualified identifier.
func (s *getSecretSuite) TestGetSecretInvalidSDK(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identifier := "Private_SDK.api-key"

	_, _, err := ctlcmd.Run(ctx, nil, []string{"get-secret", identifier}, 0)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"invalid SDK name: expected at most 40 characters using "+
			"lowercase letters, digits and single internal hyphens, "+
			"with at least one letter; the name agent and prefixes "+
			"try- and project- are reserved")
	c.Check(strings.Contains(err.Error(), "Private_SDK"), check.Equals, false)
}

// TestGetSecretInvalidPlug checks that plug validation reports naming rules
// without exposing the rejected plug name.
func (s *getSecretSuite) TestGetSecretInvalidPlug(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identifier := "ollama.Private_Key"

	_, _, err := ctlcmd.Run(ctx, nil, []string{"get-secret", identifier}, 0)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"invalid secret plug name: expected a lowercase letter "+
			"followed by lowercase letters or digits, optionally "+
			"separated by single hyphens")
	c.Check(strings.Contains(err.Error(), "Private_Key"), check.Equals, false)
}

// TestGetSecretMissingArg checks that get-secret requires a secret
// identifier argument.
func (s *getSecretSuite) TestGetSecretMissingArg(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	_, _, err := ctlcmd.Run(ctx, nil, []string{"get-secret"}, 0)
	c.Check(err, check.ErrorMatches,
		".*the required argument `<SDK>.<secret>` was not provided.*")
}

// TestGetSecretMissingContext checks retrieval requires a hook context.
func (s *getSecretSuite) TestGetSecretMissingContext(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	stdout, stderr, err := ctlcmd.Run(
		ctx,
		nil,
		[]string{"get-secret", "ollama.ollama-api-key"},
		0,
	)
	c.Check(err, check.FitsTypeOf, &ctlcmd.MissingContextError{})
	c.Check(string(stdout), check.Equals, "")
	c.Check(string(stderr), check.Equals, "")
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), check.HasLen, 0)
}

// TestGetSecretNonRoot checks that get-secret is allowed without root, as
// both the socket-activated systemd path and SDK wrapper scripts invoke it
// as the workshop user.
func (s *getSecretSuite) TestGetSecretNonRoot(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	results := s.start(ctx, "my-sdk.api-key", 1000)
	s.checkSuccess(c, results, "my-sdk", "api-key")
}
