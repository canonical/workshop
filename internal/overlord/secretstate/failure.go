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
	"errors"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/secrets"
)

// failureCode identifies a persisted domain failure, not a command exit code.
type failureCode string

const (
	failureMultipleSecrets  failureCode = "multiple-secrets"
	failurePlugNotConnected failureCode = "plug-not-connected"
	failureProviderLocked   failureCode = "provider-locked"
	failureProviderNotFound failureCode = "provider-not-found"
	failureSecretNotFound   failureCode = "secret-not-found"
	failureUserNotFound     failureCode = "user-not-found"

	secretFailureKey = "secret-failure"
)

// failureCodeForError classifies recognised failures, including wrapped errors.
// The boolean is false for nil and unrecognised errors.
func failureCodeForError(err error) (failureCode, bool) {
	switch {
	case errors.Is(err, secrets.ErrorMultipleSecrets):
		return failureMultipleSecrets, true
	case errors.Is(err, interfaces.ErrorPlugNotConnected):
		return failurePlugNotConnected, true
	case errors.Is(err, secrets.ErrorProviderLocked):
		return failureProviderLocked, true
	case errors.Is(err, secrets.ErrorProviderNotFound):
		return failureProviderNotFound, true
	case errors.Is(err, secrets.ErrorSecretNotFound):
		return failureSecretNotFound, true
	case errors.Is(err, secrets.ErrorUserNotFound):
		return failureUserNotFound, true
	default:
		return "", false
	}
}

// failureError recovers the canonical sentinel for a persisted code.
// Empty and unrecognised codes return nil.
func failureError(code failureCode) error {
	switch code {
	case failureMultipleSecrets:
		return secrets.ErrorMultipleSecrets
	case failurePlugNotConnected:
		return interfaces.ErrorPlugNotConnected
	case failureProviderLocked:
		return secrets.ErrorProviderLocked
	case failureProviderNotFound:
		return secrets.ErrorProviderNotFound
	case failureSecretNotFound:
		return secrets.ErrorSecretNotFound
	case failureUserNotFound:
		return secrets.ErrorUserNotFound
	default:
		return nil
	}
}

// setTaskFailureCode records recognised failures, leaving metadata unchanged
// for nil or unrecognised errors. The caller must not hold the state lock.
func setTaskFailureCode(task *state.Task, err error) {
	code, ok := failureCodeForError(err)
	if !ok {
		return
	}

	st := task.State()
	st.Lock()
	defer st.Unlock()
	task.Set(secretFailureKey, code)
}

// taskFailureCode reads a task's persisted failure code under the state lock.
// The boolean is false when metadata is missing or cannot be decoded.
// A successfully read code may still be unrecognised by [failureError].
func taskFailureCode(task *state.Task) (failureCode, bool) {
	var code failureCode
	err := task.Get(secretFailureKey, &code)
	return code, err == nil
}
