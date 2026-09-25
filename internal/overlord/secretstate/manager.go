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
	"slices"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// SecretManager registers and handles secret operation tasks.
type SecretManager struct {
	backend  WorkshopBackend
	repo     *interfaces.Repository
	resolver SecretResolver
}

// secretResultKey identifies a task's [secrets.Secret] in the non-persisted
// state cache. Removing the entry transfers ownership to the consumer;
// abandoned results must be closed.
type secretResultKey string

// TaskHandlerRegistrar registers handlers for secret operation tasks.
type TaskHandlerRegistrar interface {
	// AddHandler registers the do and undo handlers for a task kind.
	// A nil undo handler indicates that the task has no undo operation.
	AddHandler(string, state.HandlerFunc, state.HandlerFunc)
}

// WorkshopBackend resolves workshops using the identity in the supplied context.
type WorkshopBackend interface {
	// Workshop resolves the named workshop within the user and project
	// identified by [workshop.ContextUser] and [workshop.ContextProjectId]
	// in the supplied context.
	Workshop(context.Context, string) (*workshop.Workshop, error)
}

// doGetSecret extracts the caller and secret consumer from task metadata
// before attempting retrieval outside the state lock.
func (m SecretManager) doGetSecret(
	task *state.Task,
	tomb *tomb.Tomb,
) error {
	user, project, workshopName, err :=
		handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return fmt.Errorf("cannot resolve secret task identity: %w", err)
	}

	var sdkName, plugName string
	st := task.State()

	st.Lock()
	err = task.Get("sdk", &sdkName)
	if err == nil {
		err = task.Get("plug", &plugName)
	}
	st.Unlock()

	if err != nil {
		return fmt.Errorf(
			"cannot read get secret task parameters for sdk and plug: %w",
			err,
		)
	}

	ctx, cancel := handlersetup.BackendContext(
		tomb,
		user,
		project.ProjectId,
	)
	defer cancel()

	ref := sdk.PlugRef{
		Name:      plugName,
		ProjectId: project.ProjectId,
		Sdk:       sdkName,
		Workshop:  workshopName,
	}

	value, err := m.getSecret(ctx, ref)
	if err != nil {
		return fmt.Errorf(
			"getting secret value for sdk %q and plug %q in workshop %q: %w",
			ref.Sdk, ref.Name, ref.Workshop, err,
		)
	}

	st.Lock()
	defer st.Unlock()

	st.Cache(secretResultKey(task.ID()), value)
	// Do not return an error after publication: the runner only schedules
	// undo for a successful aborted handler. An error would skip undo and
	// could leave a late result cached after the caller has returned.
	return nil
}

// Ensure implements the state manager lifecycle. There is currently no
// periodic secret maintenance to perform.
func (m SecretManager) Ensure() error {
	return nil
}

// getSecret validates the consumer and resolves its connected slot's secret.
//
// The following errors may be expected:
//   - [context.Canceled]: the retrieval was cancelled.
//   - [context.DeadlineExceeded]: the retrieval deadline expired.
func (m SecretManager) getSecret(
	ctx context.Context,
	ref sdk.PlugRef,
) (secrets.Secret, error) {
	err := ctx.Err()
	if err != nil {
		return secrets.Secret{}, err
	}

	wp, err := m.backend.Workshop(ctx, ref.Workshop)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf("resolving workshop: %w", err)
	}
	if _, ok := wp.Sdks[ref.Sdk]; !ok {
		return secrets.Secret{}, errors.New(
			"requested sdk is not installed in workshop",
		)
	}

	plug := m.repo.Plug(ref.ProjectId, ref.Workshop, ref.Sdk, ref.Name)
	if plug == nil {
		return secrets.Secret{}, errors.New("requested plug is not declared by sdk")
	}

	if plug.Interface != "secret" {
		return secrets.Secret{}, errors.New(
			"requested plug does not use the secret interface",
		)
	}

	connections, err := m.repo.Connected(
		ref.ProjectId,
		ref.Workshop,
		ref.Sdk,
		ref.Name,
	)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"resolving secret plug connections: %w", err,
		)
	}

	connections = slices.DeleteFunc(
		connections,
		func(connection *interfaces.ConnRef) bool {
			return !connection.ConnectedToPlug(ref)
		},
	)

	if len(connections) == 0 {
		return secrets.Secret{}, errors.New("secret plug is not connected")
	}

	if len(connections) > 1 {
		return secrets.Secret{}, errors.New(
			"secret plug is connected to multiple slots",
		)
	}

	secret, err := m.resolver.Resolve(ctx, connections[0].SlotRef)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf("resolving secret: %w", err)
	}
	return secret, nil
}

// New creates a secret manager and registers its task handlers.
func New(
	runner TaskHandlerRegistrar,
	backend WorkshopBackend,
	repo *interfaces.Repository,
	resolver SecretResolver,
) SecretManager {
	manager := SecretManager{
		backend:  backend,
		repo:     repo,
		resolver: resolver,
	}
	runner.AddHandler("get-secret", manager.doGetSecret, manager.undoGetSecret)
	return manager
}

// undoGetSecret closes and removes a result published by an aborted retrieval.
// Results already transferred to the caller are absent and left untouched.
func (m SecretManager) undoGetSecret(
	task *state.Task,
	_ *tomb.Tomb,
) error {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	key := secretResultKey(task.ID())
	value, ok := st.Cached(key).(secrets.Secret)
	if ok {
		_ = value.Close()
	}
	st.Cache(key, nil)
	return nil
}
