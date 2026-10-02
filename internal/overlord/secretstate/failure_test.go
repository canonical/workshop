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
	"bytes"
	"errors"
	"fmt"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/secrets"
)

// failureSuite checks persisted domain failure codes and their recovery.
type failureSuite struct{}

var _ = Suite(&failureSuite{})

// TestMultipleSecrets checks direct, wrapped and persisted ambiguity errors.
func (s *failureSuite) TestMultipleSecrets(c *C) {
	want := secrets.ErrorMultipleSecrets
	c.Check(failureMultipleSecrets, Equals, failureCode("multiple-secrets"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureMultipleSecrets)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureMultipleSecrets)
	c.Check(failureError(failureMultipleSecrets), Equals, want)
}

// TestNilError checks successful retrieval has no failure code.
func (s *failureSuite) TestNilError(c *C) {
	_, ok := failureCodeForError(nil)
	c.Check(ok, Equals, false)
}

// TestPlugNotConnected checks direct, wrapped and persisted connection errors.
func (s *failureSuite) TestPlugNotConnected(c *C) {
	want := interfaces.ErrorPlugNotConnected
	c.Check(failurePlugNotConnected, Equals,
		failureCode("plug-not-connected"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failurePlugNotConnected)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failurePlugNotConnected)
	c.Check(failureError(failurePlugNotConnected), Equals, want)
}

// TestProviderLocked checks direct, wrapped and persisted locked-store errors.
func (s *failureSuite) TestProviderLocked(c *C) {
	want := secrets.ErrorProviderLocked
	c.Check(failureProviderLocked, Equals, failureCode("provider-locked"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureProviderLocked)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureProviderLocked)
	c.Check(failureError(failureProviderLocked), Equals, want)
}

// TestProviderNotFound checks direct, wrapped and persisted provider errors.
func (s *failureSuite) TestProviderNotFound(c *C) {
	want := secrets.ErrorProviderNotFound
	c.Check(failureProviderNotFound, Equals,
		failureCode("provider-not-found"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureProviderNotFound)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureProviderNotFound)
	c.Check(failureError(failureProviderNotFound), Equals, want)
}

// TestSecretNotFound checks direct, wrapped and persisted missing secrets.
func (s *failureSuite) TestSecretNotFound(c *C) {
	want := secrets.ErrorSecretNotFound
	c.Check(failureSecretNotFound, Equals, failureCode("secret-not-found"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureSecretNotFound)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureSecretNotFound)
	c.Check(failureError(failureSecretNotFound), Equals, want)
}

// TestTaskFailureMalformed checks invalid metadata cannot be read.
func (s *failureSuite) TestTaskFailureMalformed(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	defer st.Unlock()
	task.Set(secretFailureKey, map[string]string{"code": "provider-locked"})
	_, ok := taskFailureCode(task)
	c.Check(ok, Equals, false)
}

// TestTaskFailureMissing checks absent metadata reports no failure code.
func (s *failureSuite) TestTaskFailureMissing(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	defer st.Unlock()
	_, ok := taskFailureCode(task)
	c.Check(ok, Equals, false)
}

// TestTaskFailureSerialised checks codes survive state and task serialisation.
func (s *failureSuite) TestTaskFailureSerialised(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	task.Set(secretFailureKey, failureProviderLocked)
	task.SetStatus(state.ErrorStatus)
	data, err := st.MarshalJSON()
	st.Unlock()
	c.Assert(err, IsNil)
	c.Check(secretFailureKey, Equals, "secret-failure")
	c.Check(bytes.Contains(data,
		[]byte(`"secret-failure":"provider-locked"`)), Equals, true)

	restored, err := state.ReadState(nil, bytes.NewReader(data))
	c.Assert(err, IsNil)
	restored.Lock()
	defer restored.Unlock()
	restoredTask := restored.Task(task.ID())
	c.Assert(restoredTask, NotNil)
	c.Check(restoredTask.Status(), Equals, state.ErrorStatus)
	code, ok := taskFailureCode(restoredTask)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureProviderLocked)
}

// TestTaskFailureUnknown checks unknown codes are returned untranslated.
func (s *failureSuite) TestTaskFailureUnknown(c *C) {
	st := state.New(nil)
	task := newSecretTask(c, st)
	st.Lock()
	defer st.Unlock()
	task.Set(secretFailureKey, "future-failure")
	code, ok := taskFailureCode(task)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureCode("future-failure"))
}

// TestUnknownCode checks empty and unrecognised codes have no domain error.
func (s *failureSuite) TestUnknownCode(c *C) {
	c.Check(failureError(""), IsNil)
	c.Check(failureError("future-failure"), IsNil)
}

// TestUnknownError checks matching text does not impersonate a domain error.
func (s *failureSuite) TestUnknownError(c *C) {
	err := errors.New(secrets.ErrorProviderLocked.Error())
	_, ok := failureCodeForError(err)
	c.Check(ok, Equals, false)
	wrapped := fmt.Errorf("retrieving secret: %w", err)
	_, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, false)
}

// TestUserNotFound checks direct, wrapped and persisted missing-user errors.
func (s *failureSuite) TestUserNotFound(c *C) {
	want := secrets.ErrorUserNotFound
	c.Check(failureUserNotFound, Equals, failureCode("user-not-found"))
	code, ok := failureCodeForError(want)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureUserNotFound)
	wrapped := fmt.Errorf("retrieving secret: %w", want)
	code, ok = failureCodeForError(wrapped)
	c.Check(ok, Equals, true)
	c.Check(code, Equals, failureUserNotFound)
	c.Check(failureError(failureUserNotFound), Equals, want)
}
