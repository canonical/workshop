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
	"io"
	"strings"
	"testing"
	"time"

	. "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/interfaces"
	_ "github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
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
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name:      "ollama",
			ProjectId: "test-project",
			Type:      sdk.Regular,
			Workshop:  "test-workshop",
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	var lookupUser, lookupProject, lookupWorkshop any
	backend := workshopBackendFunc(func(
		ctx context.Context,
		name string,
	) (*workshop.Workshop, error) {
		lookupUser = ctx.Value(workshop.ContextUser)
		lookupProject = ctx.Value(workshop.ContextProjectId)
		lookupWorkshop = name
		return &workshop.Workshop{
			Name: name,
			Sdks: map[string]workshop.SdkInstallation{
				"ollama": {Setup: sdk.Setup{Name: "ollama"}},
			},
		}, nil
	})
	resolved := secrets.NewSecret([]byte("provider-api-token"))
	defer resolved.Close()
	resolver := secretResolver(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (secrets.Secret, error) {
		c.Check(ref.Name, Equals, "api-key")
		c.Check(ref.ProjectId, Equals, "")
		c.Check(ref.Sdk, Equals, "system")
		c.Check(ref.Workshop, Equals, "")
		return resolved, nil
	})
	New(runner, backend, repo, resolver)

	st.Lock()
	task := st.NewTask("get-secret", "Retrieve a workshop secret")
	change := st.NewChange("get-secret", "Retrieve a workshop secret")
	change.AddTask(task)
	change.Set("user", "test-user")
	// The task project, not stale change metadata, scopes backend lookup.
	change.Set("project-id", "stale-project")
	task.Set("project", workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "test-project",
	})
	task.Set("workshop", "test-workshop")
	task.Set("sdk", "ollama")
	task.Set("plug", "api-key")
	st.Unlock()

	err = runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()
	c.Check(lookupUser, Equals, "test-user")
	c.Check(lookupProject, Equals, "test-project")
	c.Check(lookupWorkshop, Equals, "test-workshop")

	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.DoneStatus)
	c.Check(change.Err(), IsNil)
	cached, ok := st.Cached(secretResultKey(task.ID())).(secrets.Secret)
	c.Assert(ok, Equals, true)
	value, err := io.ReadAll(cached)
	c.Assert(err, IsNil)
	c.Check(string(value), Equals, "provider-api-token")

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
	runner := &taskHandlerRegistrar{}
	New(runner, nil, nil, nil)
	c.Assert(runner.do, NotNil)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("plug", nil)
	st.Unlock()

	var taskTomb tomb.Tomb
	err := runner.do(task, &taskTomb)

	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(err, ErrorMatches,
		"(?s)cannot read get secret task parameters for sdk and plug:.*")
}

// TestGetSecretMissingProject checks that a missing project fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingProject(c *C) {
	st := state.New(nil)
	runner := &taskHandlerRegistrar{}
	New(runner, nil, nil, nil)
	c.Assert(runner.do, NotNil)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("project", nil)
	st.Unlock()

	var taskTomb tomb.Tomb
	err := runner.do(task, &taskTomb)

	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(err, ErrorMatches,
		"(?s)cannot resolve secret task identity.*")
}

// TestGetSecretMissingSDK checks that a missing SDK fails without caching
// a secret result.
func (s *managerSuite) TestGetSecretMissingSDK(c *C) {
	st := state.New(nil)
	runner := &taskHandlerRegistrar{}
	New(runner, nil, nil, nil)
	c.Assert(runner.do, NotNil)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("sdk", nil)
	st.Unlock()

	var taskTomb tomb.Tomb
	err := runner.do(task, &taskTomb)

	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(err, ErrorMatches,
		"(?s)cannot read get secret task parameters for sdk and plug:.*")
}

// TestGetSecretMissingUser checks that a missing change user fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingUser(c *C) {
	st := state.New(nil)
	runner := &taskHandlerRegistrar{}
	New(runner, nil, nil, nil)
	c.Assert(runner.do, NotNil)
	task := newSecretTask(c, st)

	st.Lock()
	task.Change().Set("user", nil)
	st.Unlock()

	var taskTomb tomb.Tomb
	err := runner.do(task, &taskTomb)

	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(err, ErrorMatches,
		"(?s)cannot resolve secret task identity.*")
}

// TestGetSecretMissingWorkshop checks that a missing workshop fails without
// caching a secret result.
func (s *managerSuite) TestGetSecretMissingWorkshop(c *C) {
	st := state.New(nil)
	runner := &taskHandlerRegistrar{}
	New(runner, nil, nil, nil)
	c.Assert(runner.do, NotNil)
	task := newSecretTask(c, st)

	st.Lock()
	task.Set("workshop", nil)
	st.Unlock()

	var taskTomb tomb.Tomb
	err := runner.do(task, &taskTomb)

	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
	c.Check(err, ErrorMatches,
		"(?s)cannot resolve secret task identity.*")
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
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name:      "ollama",
			ProjectId: "test-project",
			Type:      sdk.Regular,
			Workshop:  "test-workshop",
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: "test-workshop",
			Sdks: map[string]workshop.SdkInstallation{
				"ollama": {Setup: sdk.Setup{Name: "ollama"}},
			},
		}, nil
	})
	resolved := secrets.NewSecret([]byte("provider-api-token"))
	defer resolved.Close()
	resolver := secretResolver(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (secrets.Secret, error) {
		c.Check(ref.Name, Equals, "api-key")
		c.Check(ref.ProjectId, Equals, "")
		c.Check(ref.Sdk, Equals, "system")
		c.Check(ref.Workshop, Equals, "")
		return resolved, nil
	})
	manager := SecretManager{
		backend:  backend,
		repo:     repo,
		resolver: resolver,
	}
	err = manager.doGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	c.Check(taskTomb.Alive(), Equals, true)
	st.Lock()
	defer st.Unlock()
	c.Check(task.Status(), Equals, state.AbortStatus)
	cached, ok := st.Cached(secretResultKey(task.ID())).(secrets.Secret)
	c.Assert(ok, Equals, true)
	value, err := io.ReadAll(cached)
	c.Assert(err, IsNil)
	c.Check(string(value), Equals, "provider-api-token")
}

// TestGetSecretCancelledContext checks retrieval returns the context error
// without a value when cancellation precedes retrieval.
func (s *managerSuite) TestGetSecretCancelledContext(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}
	manager := SecretManager{}

	_, err := manager.getSecret(ctx, ref)

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

	_, err := manager.getSecret(ctx, ref)

	c.Check(errors.Is(err, context.DeadlineExceeded), Equals, true)
}

// TestGetSecretUndoneAfterFailure checks the runner invokes the registered
// undo handler to close the secret when a subsequent task fails.
func (s *managerSuite) TestGetSecretUndoneAfterFailure(c *C) {
	st := state.New(nil)
	runner := state.NewTaskRunner(st)
	defer runner.Stop()
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name:      "ollama",
			ProjectId: "test-project",
			Type:      sdk.Regular,
			Workshop:  "test-workshop",
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{"service": "ollama"},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: "test-workshop",
			Sdks: map[string]workshop.SdkInstallation{
				"ollama": {Setup: sdk.Setup{Name: "ollama"}},
			},
		}, nil
	})
	resolved := secrets.NewSecret([]byte("provider-api-token"))
	defer resolved.Close()
	resolver := secretResolver(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (secrets.Secret, error) {
		c.Check(ref.Name, Equals, "api-key")
		c.Check(ref.ProjectId, Equals, "")
		c.Check(ref.Sdk, Equals, "system")
		c.Check(ref.Workshop, Equals, "")
		return resolved, nil
	})
	New(runner, backend, repo, resolver)
	task := newSecretTask(c, st)
	runner.AddHandler("fail", func(*state.Task, *tomb.Tomb) error {
		return errors.New("subsequent task failed")
	}, nil)
	st.Lock()
	failure := st.NewTask("fail", "Fail after secret retrieval")
	failure.WaitFor(task)
	task.Change().AddTask(failure)
	st.Unlock()

	err = runner.Ensure()
	c.Assert(err, IsNil)
	runner.Wait()

	st.Lock()
	c.Check(task.Status(), Equals, state.DoneStatus)
	cached, ok := st.Cached(secretResultKey(task.ID())).(secrets.Secret)
	c.Check(ok, Equals, true)
	st.Unlock()
	// Leave unread bytes so the final read distinguishes undo from consumption.
	value := make([]byte, 1)
	n, err := cached.Read(value)
	c.Assert(err, IsNil)
	c.Check(n, Equals, 1)
	c.Check(string(value), Equals, "p")

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
	_, readErr := resolved.Read(make([]byte, 1))
	c.Check(readErr, Equals, io.EOF)
}

// TestUndoGetSecretRemovesResult checks undo closes and removes a cached secret.
func (s *managerSuite) TestUndoGetSecretRemovesResult(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	cached := secrets.NewSecret([]byte("provider-api-token"))
	defer cached.Close()
	st.Lock()
	st.Cache(secretResultKey(task.ID()), cached)
	st.Unlock()
	var taskTomb tomb.Tomb
	manager := SecretManager{}

	err := manager.undoGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	_, readErr := cached.Read(make([]byte, 1))
	c.Check(readErr, Equals, io.EOF)
	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestUndoGetSecretWithoutResult checks undo leaves a caller-owned secret open
// after cache removal, even if the undo tomb is cancelled.
func (s *managerSuite) TestUndoGetSecretWithoutResult(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	owned := secrets.NewSecret([]byte("provider-api-token"))
	defer owned.Close()
	st.Lock()
	st.Cache(secretResultKey(task.ID()), owned)
	st.Cache(secretResultKey(task.ID()), nil)
	st.Unlock()
	var taskTomb tomb.Tomb
	taskTomb.Kill(nil)
	manager := SecretManager{}

	err := manager.undoGetSecret(task, &taskTomb)

	c.Check(err, IsNil)
	value, err := io.ReadAll(owned)
	c.Assert(err, IsNil)
	c.Check(string(value), Equals, "provider-api-token")
	st.Lock()
	defer st.Unlock()
	c.Check(st.Cached(secretResultKey(task.ID())), IsNil)
}

// TestGetSecretMultipleSlots rejects ambiguous connections before resolution.
func (s *managerSuite) TestGetSecretMultipleSlots(c *C) {
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name:      "ollama",
			ProjectId: "test-project",
			Type:      sdk.Regular,
			Workshop:  "test-workshop",
		},
	}
	firstSlot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "ollama",
			},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "first-api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	secondSlot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "other-ollama",
			},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "second-api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(firstSlot), IsNil)
	c.Assert(repo.AddSlot(secondSlot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, firstSlot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, secondSlot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: "test-workshop",
			Sdks: map[string]workshop.SdkInstallation{
				"ollama": {
					Setup: sdk.Setup{
						Name: "ollama",
					},
				},
			},
		}, nil
	})
	resolver := secretResolver(func(
		context.Context,
		sdk.SlotRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, errors.New("unexpected resolution")
	})
	manager := SecretManager{
		backend:  backend,
		repo:     repo,
		resolver: resolver,
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	_, err = manager.getSecret(context.Background(), ref)

	c.Check(err, ErrorMatches, "secret plug is connected to multiple slots")
}

// TestGetSecretResolverError checks wrapped resolver errors retain their
// identity.
func (s *managerSuite) TestGetSecretResolverError(c *C) {
	repo := interfaces.NewRepository()
	iface, err := interfaces.ByName("secret")
	c.Assert(err, IsNil)
	c.Assert(repo.AddInterface(iface), IsNil)
	plug := &sdk.PlugInfo{
		Interface: "secret",
		Name:      "api-key",
		Sdk: &sdk.Info{
			Name:      "ollama",
			ProjectId: "test-project",
			Type:      sdk.Regular,
			Workshop:  "test-workshop",
		},
	}
	slot := &sdk.SlotInfo{
		Attrs: map[string]any{
			"attributes": map[string]any{
				"service": "ollama",
			},
			"collection": "default",
		},
		Interface: "secret",
		Name:      "host-api-key",
		Sdk: &sdk.Info{
			Name: "system",
			Type: sdk.System,
		},
	}
	c.Assert(repo.AddPlug(plug), IsNil)
	c.Assert(repo.AddSlot(slot), IsNil)
	_, err = repo.Connect(
		interfaces.NewConnRef(plug, slot),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, IsNil)
	backend := workshopBackendFunc(func(
		context.Context,
		string,
	) (*workshop.Workshop, error) {
		return &workshop.Workshop{
			Name: "test-workshop",
			Sdks: map[string]workshop.SdkInstallation{
				"ollama": {
					Setup: sdk.Setup{
						Name: "ollama",
					},
				},
			},
		}, nil
	})
	resolverErr := errors.New("provider unavailable")
	resolver := secretResolver(func(
		_ context.Context,
		ref sdk.SlotRef,
	) (secrets.Secret, error) {
		c.Check(ref.Name, Equals, "host-api-key")
		c.Check(ref.ProjectId, Equals, "")
		c.Check(ref.Sdk, Equals, "system")
		c.Check(ref.Workshop, Equals, "")
		return secrets.Secret{}, resolverErr
	})
	manager := SecretManager{
		backend:  backend,
		repo:     repo,
		resolver: resolver,
	}
	ref := sdk.PlugRef{
		Name:      "api-key",
		ProjectId: "test-project",
		Sdk:       "ollama",
		Workshop:  "test-workshop",
	}

	_, err = manager.getSecret(context.Background(), ref)

	c.Check(errors.Is(err, resolverErr), Equals, true)
}

// TestNewRegistersGetSecret checks construction registers the get-secret
// handler and its undo operation exactly once.
func (s *managerSuite) TestNewRegistersGetSecret(c *C) {
	runner := &taskHandlerRegistrar{}

	manager := New(runner, nil, nil, nil)

	c.Check(manager.Ensure(), IsNil)
	c.Check(runner.calls, Equals, 1)
	c.Check(runner.kind, Equals, "get-secret")
	c.Check(runner.do, NotNil)
	c.Check(runner.undo, NotNil)
}
