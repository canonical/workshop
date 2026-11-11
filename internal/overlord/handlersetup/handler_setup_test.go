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

package handlersetup_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type CommonStateFuncs struct {
	state   *state.State
	project workshop.Project
}

var _ = check.Suite(&CommonStateFuncs{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *CommonStateFuncs) setupTask() *state.Task {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("refresh", "...")
	chg.Set("user", "testuser")

	t := s.state.NewTask("task", "...")
	t.Set("workshop", "ws")
	t.Set("project", s.project)
	chg.AddTask(t)
	return t
}

func (s *CommonStateFuncs) SetUpTest(c *check.C) {
	s.state = state.New(nil)
	s.project = workshop.Project{Path: c.MkDir(), ProjectId: "42ws42ws"}
}

func (s *CommonStateFuncs) TestChangeWaitOnError(c *check.C) {
	handler := handlersetup.OnDo(func(task *state.Task, tomb *tomb.Tomb) error {
		return errors.New("task failed")
	})

	task := s.setupTask()
	s.state.Lock()
	chg := task.Change()
	chg.Set("wait-setup", conflict.ChangeSetup{Mode: conflict.ChangeWaitOnError.String()})
	chg.Set("project-id", s.project.ProjectId)
	s.state.Unlock()

	err := handler(task, nil)
	expected := state.Wait{Reason: "wait on error: task failed", WaitedStatus: state.DoingStatus}
	c.Assert(err, check.ErrorMatches, expected.Error())
	s.state.Lock()
	c.Assert(task.Log(), check.HasLen, 1)
	c.Assert(task.Log()[0], check.Matches, ".*task failed")
	s.state.Unlock()

}

func (s *CommonStateFuncs) TestExecutionOnDoRetry(c *check.C) {
	task := s.setupTask()

	handler := handlersetup.OnDo(func(task *state.Task, tomb *tomb.Tomb) error {
		return &state.Retry{Reason: "not enough time"}
	})

	err := handler(task, nil)
	c.Assert(err, check.ErrorMatches, "task should be retried")
	c.Assert(err.(*state.Retry).Reason, check.Equals, "not enough time")
}

func (s *CommonStateFuncs) TestInjectTasks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	lane := s.state.NewLane()

	// setup main task and two tasks waiting for it; all part of same change
	chg := s.state.NewChange("change", "")
	t0 := s.state.NewTask("task1", "")
	chg.AddTask(t0)
	t0.JoinLane(lane)
	t01 := s.state.NewTask("task1-1", "")
	t01.WaitFor(t0)
	chg.AddTask(t01)
	t02 := s.state.NewTask("task1-2", "")
	t02.WaitFor(t0)
	chg.AddTask(t02)

	// setup extra tasks
	t1 := s.state.NewTask("task2", "")
	t2 := s.state.NewTask("task3", "")
	ts := state.NewTaskSet(t1, t2)

	handlersetup.InjectTasks(t0, ts)

	// verify that extra tasks are now part of same change
	c.Assert(t1.Change().ID(), check.Equals, t0.Change().ID())
	c.Assert(t2.Change().ID(), check.Equals, t0.Change().ID())
	c.Assert(t1.Change().ID(), check.Equals, chg.ID())

	c.Assert(t1.Lanes(), check.DeepEquals, []int{lane})

	// verify that halt tasks of the main task now wait for extra tasks
	c.Assert(t1.HaltTasks(), check.HasLen, 2)
	c.Assert(t2.HaltTasks(), check.HasLen, 2)
	c.Assert(t1.HaltTasks(), check.DeepEquals, t2.HaltTasks())

	ids := []string{t1.HaltTasks()[0].Kind(), t2.HaltTasks()[1].Kind()}
	sort.Strings(ids)
	c.Assert(ids, check.DeepEquals, []string{"task1-1", "task1-2"})

	// verify that extra tasks wait for the main task
	c.Assert(t1.WaitTasks(), check.HasLen, 1)
	c.Assert(t1.WaitTasks()[0].Kind(), check.Equals, "task1")
	c.Assert(t2.WaitTasks(), check.HasLen, 1)
	c.Assert(t2.WaitTasks()[0].Kind(), check.Equals, "task1")
}

func (s *CommonStateFuncs) TestInjectTasksWithNullChange(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	// setup main task
	t0 := s.state.NewTask("task1", "")
	t01 := s.state.NewTask("task1-1", "")
	t01.WaitFor(t0)

	// setup extra task
	t1 := s.state.NewTask("task2", "")
	ts := state.NewTaskSet(t1)

	handlersetup.InjectTasks(t0, ts)

	c.Assert(t1.Lanes(), check.DeepEquals, []int{0})

	// verify that halt tasks of the main task now wait for extra tasks
	c.Assert(t1.HaltTasks(), check.HasLen, 1)
	c.Assert(t1.HaltTasks()[0].Kind(), check.Equals, "task1-1")
}

func (s *CommonStateFuncs) TestInjectTasksMainAborted(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	lane := s.state.NewLane()

	chg := s.state.NewChange("change", "")
	t0 := s.state.NewTask("task1", "")
	// Emulate scenario when the task handler is being executed.
	t0.SetStatus(state.DoingStatus)
	chg.AddTask(t0)
	t0.JoinLane(lane)

	t01 := s.state.NewTask("task1-1", "")
	t02 := s.state.NewTask("task1-2", "")
	ts := state.NewTaskSet(t01, t02)

	// This will abort the main task.
	chg.Abort()
	handlersetup.InjectTasks(t0, ts)

	// verify that extra tasks are on hold
	c.Assert(t01.Status(), check.Equals, state.HoldStatus)
	c.Assert(t02.Status(), check.Equals, state.HoldStatus)
}

func (s *CommonStateFuncs) TestRestoreStorageReserve(c *check.C) {
	backend, err := fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, backend)
	cleanups := 0
	var cleanupErr error
	handler := handlersetup.RestoreStorageReserve(func(task *state.Task, tomb *tomb.Tomb) error {
		cleanups++
		return cleanupErr
	})

	task := s.setupTask()
	s.state.Lock()
	task.SetStatus(state.DoneStatus)
	s.state.Unlock()

	// The cleanup fails, so the reserve isn't restored yet.
	cleanupErr = errors.New("cannot clean up")
	err = handler(task, &tomb.Tomb{})
	c.Check(err, check.Equals, cleanupErr)
	c.Check(cleanups, check.Equals, 1)
	c.Check(backend.RestoreStorageReserveCalls, check.Equals, 0)

	// The pool hasn't reclaimed the space yet.
	cleanupErr = nil
	backend.RestoreStorageReserveCallback = func(ctx context.Context) error {
		return errors.New("storage pool is full")
	}
	err = handler(task, &tomb.Tomb{})
	c.Check(err, check.FitsTypeOf, &state.Retry{})
	c.Check(cleanups, check.Equals, 2)
	c.Check(backend.RestoreStorageReserveCalls, check.Equals, 1)

	// The cleanup isn't repeated.
	backend.RestoreStorageReserveCallback = nil
	err = handler(task, &tomb.Tomb{})
	c.Check(err, check.IsNil)
	c.Check(cleanups, check.Equals, 2)
	c.Check(backend.RestoreStorageReserveCalls, check.Equals, 2)
}

func (s *CommonStateFuncs) TestRestoreStorageReserveGivesUp(c *check.C) {
	defer handlersetup.MockStorageReserveRestoreWindow(0)()
	backend, err := fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, backend)
	backend.RestoreStorageReserveCallback = func(ctx context.Context) error {
		return errors.New("storage pool is full")
	}
	handler := handlersetup.RestoreStorageReserve(func(task *state.Task, tomb *tomb.Tomb) error {
		return nil
	})

	task := s.setupTask()
	s.state.Lock()
	task.SetStatus(state.DoneStatus)
	s.state.Unlock()

	err = handler(task, &tomb.Tomb{})
	c.Check(err, check.IsNil)
	c.Check(backend.RestoreStorageReserveCalls, check.Equals, 1)
}
