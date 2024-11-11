package conflict_test

import (
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	conflict "github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

type conflictSuite struct {
	state   *state.State
	project *workshop.Project
}

var _ = check.Suite(&conflictSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *conflictSuite) newChange(kind string) *state.Change {
	change := s.state.NewChange(kind, "test")
	task := s.state.NewTask("test", "test")
	task.Set("workshop", "ws")
	change.AddTask(task)
	change.Set("project-id", s.project.ProjectId)
	return change
}

func (s *conflictSuite) newChangeDisconnect(kind string) *state.Change {
	change := s.state.NewChange(kind, "test")
	task := s.state.NewTask("disconnect", "test")
	task.Set("slot", interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "ws"})
	task.Set("plug", interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "another-ws"})
	change.AddTask(task)
	change.Set("project-id", s.project.ProjectId)
	return change
}

func (s *conflictSuite) SetUpTest(c *check.C) {
	s.state = state.New(nil)
	s.project = &workshop.Project{Path: c.MkDir(), ProjectId: "42ws42ws"}
}

func (s *conflictSuite) TestCheckChangeConflictNotFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	err := conflict.CheckChangeConflict(s.state, s.project.ProjectId, "ws", "")
	c.Assert(err, check.IsNil)
}

func (s *conflictSuite) TestCheckChangeConflictFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChange("launch")

	err := conflict.CheckChangeConflict(s.state, s.project.ProjectId, "ws", "")
	c.Assert(err, check.NotNil)
	conflictErr, ok := err.(*conflict.ChangeConflictError)
	c.Assert(ok, check.Equals, true)
	c.Assert(conflictErr.ChangeID, check.Equals, change.ID())
	c.Assert(conflictErr.ChangeKind, check.Equals, "launch")
	c.Assert(conflictErr.Workshop, check.Equals, "ws")
	c.Assert(conflictErr.ProjectId, check.Equals, s.project.ProjectId)
}

func (s *conflictSuite) TestCheckChangeDisconnectConflictFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChangeDisconnect("disconnect")

	err := conflict.CheckChangeConflict(s.state, s.project.ProjectId, "ws", "")
	c.Assert(err, check.NotNil)
	conflictErr, ok := err.(*conflict.ChangeConflictError)
	c.Assert(ok, check.Equals, true)
	c.Assert(conflictErr.ChangeID, check.Equals, change.ID())
	c.Assert(conflictErr.ChangeKind, check.Equals, "disconnect")
	c.Assert(conflictErr.Workshop, check.Equals, "ws")
	c.Assert(conflictErr.ProjectId, check.Equals, s.project.ProjectId)

	err = conflict.CheckChangeConflict(s.state, s.project.ProjectId, "another-ws", "")
	c.Assert(err, check.NotNil)
	conflictErr, ok = err.(*conflict.ChangeConflictError)
	c.Assert(ok, check.Equals, true)
	c.Assert(conflictErr.ChangeID, check.Equals, change.ID())
	c.Assert(conflictErr.ChangeKind, check.Equals, "disconnect")
	c.Assert(conflictErr.Workshop, check.Equals, "another-ws")
	c.Assert(conflictErr.ProjectId, check.Equals, s.project.ProjectId)
}

func (s *conflictSuite) TestCheckChangeNoConflictWithReadyChange(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChange("launch")
	change.SetStatus(state.DoneStatus)

	err := conflict.CheckChangeConflict(s.state, s.project.ProjectId, "ws", "")
	c.Assert(err, check.IsNil)
}

func (s *conflictSuite) TestCheckChangeConflictIgnoreChange(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChange("launch")

	err := conflict.CheckChangeConflict(s.state, s.project.ProjectId, "ws", change.ID())
	c.Assert(err, check.IsNil)
}

func (s *conflictSuite) TestResumeRefreshNothingInProgress(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshContinue)
	c.Check(err, check.ErrorMatches, noRefresh)
}

func (s *conflictSuite) TestResumeRefreshIncorrectMode(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshTransactional)
	c.Check(err, check.ErrorMatches, "cannot resume: only abort or continue can be used to resume the refresh operation")
}

func (s *conflictSuite) TestResumeRefreshWrongChangeKindInProgress(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	_ = s.newChange("launch")

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshContinue)
	c.Check(err, check.ErrorMatches, resumeDuringLaunch)
}

func (s *conflictSuite) TestResumeRefreshNoWaitingOnError(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	_ = s.newChange("refresh")

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshContinue)
	c.Check(err, check.ErrorMatches, resumeDuringRefresh)
}

func (s *conflictSuite) TestResumeRefreshContinue(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChange("refresh")
	task := change.Tasks()[0]
	task.SetToWait(state.HoldStatus)

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshContinue)
	c.Assert(err, check.IsNil)
	c.Assert(task.Status(), check.Equals, state.HoldStatus)
}

func (s *conflictSuite) TestResumeRefreshAbort(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	change := s.newChange("refresh")
	task := change.Tasks()[0]
	task.SetToWait(state.DoingStatus)
	task2 := s.state.NewTask("do", "to do")
	// emulate tasks to be done down the lane
	// if abort is called for the change, the task will
	// be reverted to hold.
	task2.SetStatus(state.DoStatus)
	change.AddTask(task2)
	change.SetStatus(state.WaitStatus)

	_, err := conflict.ResumeRefresh(s.state, "ws", s.project.ProjectId, conflict.RefreshAbort)
	c.Assert(err, check.IsNil)
	c.Assert(task.Status(), check.Equals, state.ErrorStatus)
	c.Assert(task2.Status(), check.Equals, state.HoldStatus)
}
