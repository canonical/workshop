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
	"fmt"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
)

// SecretManager registers and handles secret operation tasks.
type SecretManager struct{}

// secretResultKey identifies a task's []byte result in the non-persisted state
// cache. The consumer must remove the entry after reading the result.
type secretResultKey string

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
		return err
	}

	st.Lock()
	defer st.Unlock()

	// Check under the publication lock: an abort may precede tomb cancellation.
	switch status := task.Status(); status {
	case state.DoingStatus:
		st.Cache(secretResultKey(task.ID()), value)
		return nil

	case state.AbortStatus:
		return context.Canceled

	default:
		return fmt.Errorf(
			"cannot publish secret result: unexpected task status %s",
			status,
		)
	}
}

// Ensure implements the state manager lifecycle. There is currently no
// periodic secret maintenance to perform.
func (m SecretManager) Ensure() error {
	return nil
}

// getSecret returns a placeholder until provider support is implemented.
//
// The following errors may be expected:
//   - [context.Canceled]: the retrieval was cancelled.
//   - [context.DeadlineExceeded]: the retrieval deadline expired.
func (m SecretManager) getSecret(
	ctx context.Context,
	_ sdk.PlugRef,
) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	return []byte("workshop-placeholder-secret"), nil
}

// New creates a secret manager and registers its task handlers.
func New(runner *state.TaskRunner) SecretManager {
	manager := SecretManager{}
	runner.AddHandler("get-secret", manager.doGetSecret, nil)
	return manager
}
