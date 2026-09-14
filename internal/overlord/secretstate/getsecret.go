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

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// GetSecret schedules retrieval for a previously authorised secret consumer
// and waits for its result. The caller must not hold the state lock, and the
// state engine must be running. On success, ownership transfers from the
// in-memory cache to the caller, which must consume or close the secret.
// On error, the returned value can be discarded without further cleanup.
// Cancellation aborts unfinished changes and closes any cached result.
// Results published after cancellation are closed and removed by task undo.
//
// The following errors may be expected:
//   - [context.Canceled]: the request was cancelled.
//   - [context.DeadlineExceeded]: the request deadline expired.
func GetSecret(
	ctx context.Context,
	st *state.State,
	project workshop.Project,
	ref sdk.PlugRef,
) (secrets.Secret, error) {
	err := ctx.Err()
	if err != nil {
		return secrets.Secret{}, err
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok || user == "" {
		return secrets.Secret{}, errors.New("secret request has no user")
	}

	err = validateGetSecretArgs(project, ref)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"validating get secret request arguments: %w", err,
		)
	}

	summary := fmt.Sprintf("Retrieve secret %q", ref.ShortRef())

	st.Lock()
	task := st.NewTask("get-secret", summary)
	change := st.NewChange("get-secret", summary)
	change.AddTask(task)
	change.Set("user", user)
	change.Set("project-id", project.ProjectId)
	task.Set("project", project)
	task.Set("workshop", ref.Workshop)
	task.Set("sdk", ref.Sdk)
	task.Set("plug", ref.Name)
	st.EnsureBefore(0)
	st.Unlock()

	select {
	case <-change.Ready():
	case <-ctx.Done():
	}

	resultKey := secretResultKey(task.ID())

	st.Lock()
	defer st.Unlock()
	// Successful retrieval removes the entry to transfer ownership. Any
	// result still cached on return is abandoned and closed under the lock.
	// If an aborted handler publishes later, its undo handler closes it.
	defer func() {
		value, ok := st.Cached(resultKey).(secrets.Secret)
		if ok {
			_ = value.Close()
		}
		st.Cache(resultKey, nil)
	}()

	// Prefer cancellation if it raced with task completion. The runner
	// handles undo if retrieval succeeds despite the abort.
	err = ctx.Err()
	if err != nil {
		// A ready change must not become unready through an abort. Check
		// under the same lock so completion cannot race with this decision.
		if !change.IsReady() {
			change.Abort()
			st.EnsureBefore(0)
		}
		return secrets.Secret{}, err
	}

	err = change.Err()
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"checking secret retrieval change %s: %w",
			change.ID(),
			err,
		)
	}

	status := task.Status()
	if status != state.DoneStatus {
		return secrets.Secret{}, fmt.Errorf(
			"secret task %s in change %s finished with unexpected status %s",
			task.ID(),
			change.ID(),
			status,
		)
	}

	value, ok := st.Cached(resultKey).(secrets.Secret)
	if !ok {
		return secrets.Secret{}, fmt.Errorf(
			"secret task %s in change %s completed without a result",
			task.ID(),
			change.ID(),
		)
	}

	st.Cache(resultKey, nil)
	return value, nil
}

// validateGetSecretArgs checks that the project and secret consumer identity
// are complete and refer to the same project.
func validateGetSecretArgs(project workshop.Project, ref sdk.PlugRef) error {
	if project.ProjectId == "" {
		return errors.New("project ID is missing")
	}
	if project.Path == "" {
		return errors.New("project path is missing")
	}
	if ref.ProjectId == "" {
		return errors.New("plug reference project ID is missing")
	}
	if ref.ProjectId != project.ProjectId {
		return errors.New(
			"plug reference project ID does not match project ID",
		)
	}
	if ref.Workshop == "" {
		return errors.New("workshop name is missing")
	}
	if ref.Sdk == "" {
		return errors.New("sdk name is missing")
	}
	if ref.Name == "" {
		return errors.New("plug name is missing")
	}
	return nil
}
