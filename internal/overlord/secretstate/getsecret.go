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
	"github.com/canonical/workshop/internal/workshop"
)

// GetSecret schedules retrieval for a previously authorised secret consumer
// and waits for its result. The caller must not hold the state lock, and the
// state engine must be running. Results are consumed from the in-memory cache.
// Cancellation aborts the change and discards any result.
//
// The following errors may be expected:
//   - [context.Canceled]: the request was cancelled.
//   - [context.DeadlineExceeded]: the request deadline expired.
func GetSecret(
	ctx context.Context,
	st *state.State,
	project workshop.Project,
	ref sdk.PlugRef,
) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok || user == "" {
		return nil, errors.New("secret request has no user")
	}

	err = validateGetSecretArgs(project, ref)
	if err != nil {
		return nil, fmt.Errorf("validating get secret request arguments: %w", err)
	}

	st.Lock()
	task := st.NewTask("get-secret", "Retrieve a workshop secret")
	change := st.NewChange("get-secret", "Retrieve a workshop secret")
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
	// Remove the secret on every return path so results do not outlive their
	// request. This defer runs before the state is unlocked.
	defer st.Cache(resultKey, nil)

	// Prefer cancellation if it raced with task completion. Aborting under
	// this lock prevents the handler from publishing a result after cleanup.
	err = ctx.Err()
	if err != nil {
		change.Abort()
		st.EnsureBefore(0)
		return nil, err
	}

	err = change.Err()
	if err != nil {
		return nil, fmt.Errorf(
			"checking secret retrieval change %s: %w",
			change.ID(),
			err,
		)
	}

	status := task.Status()
	if status != state.DoneStatus {
		return nil, fmt.Errorf(
			"secret task %s in change %s finished with unexpected status %s",
			task.ID(),
			change.ID(),
			status,
		)
	}

	value, ok := st.Cached(resultKey).([]byte)
	if !ok {
		return nil, fmt.Errorf(
			"secret task %s in change %s completed without a result",
			task.ID(),
			change.ID(),
		)
	}

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
