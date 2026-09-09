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

package secretstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// getSecretResult carries retrieval completion without blocking its goroutine.
type getSecretResult struct {
	err   error
	value []byte
}

// getSecretSuite checks synchronous retrieval through the task runner.
type getSecretSuite struct {
	backend *secretStateBackend
	runner  *state.TaskRunner
	st      *state.State
}

var _ = Suite(&getSecretSuite{})

// awaitTask waits for scheduling before inspecting the task under lock.
func (s *getSecretSuite) awaitTask(c *C) *state.Task {
	s.awaitEnsure(c)
	s.st.Lock()
	defer s.st.Unlock()
	changes := s.st.Changes()
	c.Assert(changes, HasLen, 1)
	tasks := changes[0].Tasks()
	c.Assert(tasks, HasLen, 1)
	c.Check(changes[0].Kind(), Equals, "get-secret")
	c.Check(tasks[0].Kind(), Equals, "get-secret")
	return tasks[0]
}

// awaitEnsure waits for an immediate ensure request without polling.
func (s *getSecretSuite) awaitEnsure(c *C) {
	delay := <-s.backend.ensureBefore
	c.Check(delay, Equals, time.Duration(0))
}

// SetUpTest provides the state backend and task runner.
func (s *getSecretSuite) SetUpTest(c *C) {
	s.backend = &secretStateBackend{
		ensureBefore: make(chan time.Duration, 1),
	}
	s.st = state.New(s.backend)
	s.runner = state.NewTaskRunner(s.st)
	New(s.runner)
}

// start launches retrieval with a buffered completion channel.
func (s *getSecretSuite) start(
	ctx context.Context,
	project workshop.Project,
	ref sdk.PlugRef,
) <-chan getSecretResult {
	results := make(chan getSecretResult, 1)
	go func() {
		value, err := GetSecret(ctx, s.st, project, ref)
		results <- getSecretResult{err: err, value: value}
	}()
	return results
}

// TearDownTest stops the runner.
func (s *getSecretSuite) TearDownTest(c *C) {
	s.runner.Stop()
}

// TestCancelledBeforeScheduling checks cancellation creates no changes.
func (s *getSecretSuite) TestCancelledBeforeScheduling(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	cancel()

	result := <-s.start(ctx, project, ref)
	c.Check(errors.Is(result.err, context.Canceled), Equals, true)
	c.Check(result.value, IsNil)

	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
	c.Check(s.backend.ensureBefore, HasLen, 0)
}

// TestCancelledWhileQueued checks cancellation aborts an unstarted task and
// removes even a cached result before requesting another ensure pass.
func (s *getSecretSuite) TestCancelledWhileQueued(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	results := s.start(ctx, project, ref)
	task := s.awaitTask(c)

	s.st.Lock()
	c.Check(task.Status(), Equals, state.DoStatus)
	s.st.Cache(secretResultKey(task.ID()), []byte("stale"))
	s.st.Unlock()
	cancel()

	result := <-results
	c.Check(errors.Is(result.err, context.Canceled), Equals, true)
	c.Check(result.value, IsNil)
	s.awaitEnsure(c)

	err := s.runner.Ensure()
	c.Assert(err, IsNil)
	s.runner.Wait()

	s.st.Lock()
	defer s.st.Unlock()
	c.Check(task.Status(), Equals, state.HoldStatus)
	c.Check(s.st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestMissingUser checks that unauthenticated requests create no tasks.
func (s *getSecretSuite) TestMissingUser(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := <-s.start(ctx, project, ref)

	c.Check(result.err, ErrorMatches, "secret request has no user")
	c.Check(result.value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMismatchedProject checks that inconsistent lookup identity is rejected
// before a task is scheduled.
func (s *getSecretSuite) TestMismatchedProject(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "another-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	result := <-s.start(ctx, project, ref)

	c.Check(result.err, ErrorMatches,
		"validating get secret request arguments: "+
			"plug reference project ID does not match project ID")
	c.Check(result.value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingPlug checks that an absent plug name is identified before scheduling.
func (s *getSecretSuite) TestMissingPlug(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: plug name is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingPlugProjectID checks that an absent reference project ID is
// distinguished from a project mismatch before scheduling.
func (s *getSecretSuite) TestMissingPlugProjectID(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:     "api-key",
		Sdk:      "ollama",
		Workshop: "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: "+
			"plug reference project ID is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingProjectID checks that an absent project ID prevents scheduling.
func (s *getSecretSuite) TestMissingProjectID(c *C) {
	project := workshop.Project{
		Path: c.MkDir(),
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: project ID is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingProjectPath checks that an absent project path prevents scheduling.
func (s *getSecretSuite) TestMissingProjectPath(c *C) {
	project := workshop.Project{
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: project path is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingSDK checks that an absent SDK name prevents scheduling.
func (s *getSecretSuite) TestMissingSDK(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: sdk name is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestMissingWorkshop checks that an absent workshop name prevents scheduling.
func (s *getSecretSuite) TestMissingWorkshop(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")

	value, err := GetSecret(ctx, s.st, project, ref)

	c.Check(err, ErrorMatches,
		"validating get secret request arguments: workshop name is missing")
	c.Check(value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Check(s.st.Changes(), HasLen, 0)
}

// TestSuccess checks identity metadata, result delivery and cache consumption.
func (s *getSecretSuite) TestSuccess(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	results := s.start(ctx, project, ref)
	task := s.awaitTask(c)

	func() {
		s.st.Lock()
		defer s.st.Unlock()
		var user, projectID, workshopName, sdkName, plugName string
		var actualProject workshop.Project
		c.Assert(task.Change().Get("user", &user), IsNil)
		c.Assert(task.Change().Get("project-id", &projectID), IsNil)
		c.Assert(task.Get("project", &actualProject), IsNil)
		c.Assert(task.Get("workshop", &workshopName), IsNil)
		c.Assert(task.Get("sdk", &sdkName), IsNil)
		c.Assert(task.Get("plug", &plugName), IsNil)
		c.Check(user, Equals, "test-user")
		c.Check(projectID, Equals, project.ProjectId)
		c.Check(actualProject, DeepEquals, project)
		c.Check(workshopName, Equals, ref.Workshop)
		c.Check(sdkName, Equals, ref.Sdk)
		c.Check(plugName, Equals, ref.Name)
	}()

	err := s.runner.Ensure()
	c.Assert(err, IsNil)
	s.runner.Wait()
	result := <-results
	c.Check(result.err, IsNil)
	c.Check(result.value, DeepEquals, []byte("workshop-placeholder-secret"))

	s.st.Lock()
	defer s.st.Unlock()
	c.Check(task.Status(), Equals, state.DoneStatus)
	c.Check(task.Change().Err(), IsNil)
	c.Check(s.st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestSuccessWithoutResult checks a done task must supply a cached secret.
func (s *getSecretSuite) TestSuccessWithoutResult(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	s.runner.AddHandler("get-secret", func(*state.Task, *tomb.Tomb) error {
		return nil
	}, nil)
	results := s.start(ctx, project, ref)
	task := s.awaitTask(c)

	err := s.runner.Ensure()
	c.Assert(err, IsNil)
	s.runner.Wait()
	result := <-results
	c.Check(result.value, IsNil)

	s.st.Lock()
	defer s.st.Unlock()
	c.Assert(result.err, NotNil)
	c.Check(result.err.Error(), Equals, fmt.Sprintf(
		"secret task %s in change %s completed without a result",
		task.ID(),
		task.Change().ID(),
	))
	c.Check(task.Status(), Equals, state.DoneStatus)
	c.Check(s.st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestUnexpectedStatus checks that a held task reports both IDs and its
// status, and discards any cached result rather than returning it.
func (s *getSecretSuite) TestUnexpectedStatus(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	results := s.start(ctx, project, ref)
	task := s.awaitTask(c)

	s.st.Lock()
	s.st.Cache(secretResultKey(task.ID()), []byte("stale"))
	task.Change().Abort()
	s.st.Unlock()
	result := <-results

	c.Check(result.value, IsNil)
	s.st.Lock()
	defer s.st.Unlock()
	c.Assert(task.Status(), Equals, state.HoldStatus)
	c.Assert(result.err, NotNil)
	c.Check(result.err.Error(), Equals, fmt.Sprintf(
		"secret task %s in change %s finished with unexpected status Hold",
		task.ID(),
		task.Change().ID(),
	))
	c.Check(s.st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestTaskFailure checks task errors propagate without returning cached data.
func (s *getSecretSuite) TestTaskFailure(c *C) {
	project := workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, workshop.ContextUser, "test-user")
	s.runner.AddHandler("get-secret", func(
		task *state.Task,
		_ *tomb.Tomb,
	) error {
		s.st.Lock()
		s.st.Cache(secretResultKey(task.ID()), []byte("stale"))
		s.st.Unlock()
		return errors.New("secret provider unavailable")
	}, nil)
	results := s.start(ctx, project, ref)
	task := s.awaitTask(c)

	err := s.runner.Ensure()
	c.Assert(err, IsNil)
	s.runner.Wait()
	result := <-results
	c.Check(result.value, IsNil)

	s.st.Lock()
	defer s.st.Unlock()
	c.Assert(result.err, NotNil)
	changeErr := task.Change().Err()
	c.Assert(changeErr, ErrorMatches, "(?s).*secret provider unavailable.*")
	c.Check(result.err.Error(), Equals, fmt.Sprintf(
		"checking secret retrieval change %s: %s",
		task.Change().ID(),
		changeErr,
	))
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(s.st.Cached(secretResultKey(task.ID())), IsNil)
}
