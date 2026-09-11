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
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	. "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// managerSuite checks secret task registration and execution.
type managerSuite struct{}

var _ = Suite(&managerSuite{})

// newSecretTask creates a retrieval task with complete lookup metadata.
func newSecretTask(c *C, st *state.State) *state.Task {
	st.Lock()
	defer st.Unlock()

	task := st.NewTask("get-secret", "Retrieve a workshop secret")
	change := st.NewChange("get-secret", "Retrieve a workshop secret")
	change.AddTask(task)
	change.Set("user", "test-user")
	task.Set("project", workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	})
	task.Set("workshop", "test-workshop")
	task.Set("sdk", "ollama")
	task.Set("plug", "api-key")

	return task
}

// TestSecretState runs the secret manager suite.
func TestSecretState(t *testing.T) {
	TestingT(t)
}

// TestGetSecretCachesResult checks that successful retrieval stores its result
// in memory without including the secret in serialised state.
func (s *managerSuite) TestGetSecretCachesResult(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)

	st.Lock()
	task := st.NewTask("get-secret", "Retrieve a workshop secret")
	change := st.NewChange("get-secret", "Retrieve a workshop secret")
	change.AddTask(task)
	change.Set("user", "test-user")
	task.Set("project", workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	})
	task.Set("workshop", "test-workshop")
	task.Set("sdk", "ollama")
	task.Set("plug", "api-key")
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.DoneStatus)
	c.Check(change.Err(), IsNil)
	value, ok := st.Cached(secretResultKey(task.ID())).([]byte)
	c.Assert(ok, Equals, true)
	c.Check(string(value), Equals, "workshop-placeholder-secret")

	data, err := st.MarshalJSON()
	c.Assert(err, IsNil)
	c.Check(strings.Contains(string(data), string(value)), Equals, false)
	encoded := base64.StdEncoding.EncodeToString(value)
	c.Check(strings.Contains(string(data), encoded), Equals, false)
}

// TestGetSecretMissingPlug checks that a missing plug fails without caching
// a secret result.
func (s *managerSuite) TestGetSecretMissingPlug(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("plug", nil)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*cannot read get secret task parameters for sdk and plug:.*")
}

// TestGetSecretMissingProject checks that a missing project fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingProject(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("project", nil)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*cannot resolve secret task identity.*")
}

// TestGetSecretMissingSDK checks that a missing SDK fails without caching
// a secret result.
func (s *managerSuite) TestGetSecretMissingSDK(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("sdk", nil)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*cannot read get secret task parameters for sdk and plug:.*")
}

// TestGetSecretMissingUser checks that a missing change user fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingUser(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)

	st.Lock()
	task.Change().Set("user", nil)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*cannot resolve secret task identity.*")
}

// TestGetSecretMissingWorkshop checks that a missing workshop fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingWorkshop(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("workshop", nil)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.ErrorStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*cannot resolve secret task identity.*")
}

// TestGetSecretCancelledBeforeTomb checks successful retrieval publishes a
// result and returns nil even when the task has already been aborted.
func (s *managerSuite) TestGetSecretCancelledBeforeTomb(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	task.SetStatus(state.DoingStatus)
	task.Change().Abort()
	st.Unlock()
	var taskTomb tomb.Tomb
	manager := SecretManager{}

	err := manager.doGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.AbortStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), DeepEquals,
		[]byte("workshop-placeholder-secret"))
}

// TestGetSecretCancelledContext checks retrieval returns the context error
// without a value when cancellation precedes retrieval.
func (s *managerSuite) TestGetSecretCancelledContext(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}
	manager := SecretManager{}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(errors.Is(err, context.Canceled), Equals, true)
}

// TestGetSecretCancelledTomb checks the handler propagates retrieval
// cancellation without caching a result, even while its task is Doing.
func (s *managerSuite) TestGetSecretCancelledTomb(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	task.SetStatus(state.DoingStatus)
	st.Unlock()
	var taskTomb tomb.Tomb
	taskTomb.Kill(nil)
	manager := SecretManager{}

	err := manager.doGetSecret(task, &taskTomb)

	c.Check(errors.Is(err, context.Canceled), Equals, true)
	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestGetSecretExpiredContext checks retrieval returns the deadline error
// without a value when its deadline has already elapsed.
func (s *managerSuite) TestGetSecretExpiredContext(c *C) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}
	manager := SecretManager{}

	value, err := manager.getSecret(ctx, ref)

	c.Check(value, IsNil)
	c.Check(errors.Is(err, context.DeadlineExceeded), Equals, true)
}

// TestGetSecretUndoneAfterFailure checks the runner invokes the registered
// undo handler when a subsequent task fails in the same change.
func (s *managerSuite) TestGetSecretUndoneAfterFailure(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	New(runner)
	task := newSecretTask(c, st)
	runner.AddHandler("fail", func(*state.Task, *tomb.Tomb) error {
		return errors.New("subsequent task failed")
	}, nil)
	st.Lock()
	failure := st.NewTask("fail", "Fail after secret retrieval")
	failure.WaitFor(task)
	task.Change().AddTask(failure)
	st.Unlock()

	err := runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	c.Check(task.Status(), Equals, state.DoneStatus)
	c.Check(st.Cached(secretResultKey(task.ID())), DeepEquals,
		[]byte("workshop-placeholder-secret"))
	st.Unlock()

	err = runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	c.Check(failure.Status(), Equals, state.ErrorStatus)
	c.Check(task.Status(), Equals, state.UndoStatus)
	st.Unlock()

	err = runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.UndoneStatus)
	c.Check(task.Change().Err(), ErrorMatches,
		"(?s).*subsequent task failed.*")
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestUndoGetSecretRemovesResult checks undo discards a cached secret.
func (s *managerSuite) TestUndoGetSecretRemovesResult(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	st.Cache(secretResultKey(task.ID()), []byte("workshop-placeholder-secret"))
	st.Unlock()
	var taskTomb tomb.Tomb
	manager := SecretManager{}

	err := manager.undoGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestUndoGetSecretWithoutResult checks undo succeeds when the caller has
// already removed the cached result, even if the undo tomb is cancelled.
func (s *managerSuite) TestUndoGetSecretWithoutResult(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	st.Cache(secretResultKey(task.ID()), []byte("workshop-placeholder-secret"))
	st.Cache(secretResultKey(task.ID()), nil)
	st.Unlock()
	var taskTomb tomb.Tomb
	taskTomb.Kill(nil)
	manager := SecretManager{}

	err := manager.undoGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestNewRegistersGetSecret checks that construction registers the task kind.
func (s *managerSuite) TestNewRegistersGetSecret(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()

	manager := New(runner)

	c.Check(manager.Ensure(), IsNil)
	c.Check(runner.KnownTaskKinds(), DeepEquals, []string{"get-secret"})
}
