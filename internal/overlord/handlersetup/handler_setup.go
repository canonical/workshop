package handlersetup

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// OnUndo helps to skip the undo handler if the change is an abort-backgroud refresh
func OnUndo(handler state.HandlerFunc) state.HandlerFunc {
	return func(task *state.Task, tomb *tomb.Tomb) error {
		st := task.State()
		st.Lock()
		change := task.Change()
		var discardBackground bool
		err := change.Get("discard-background", &discardBackground)
		st.Unlock()
		if err != nil && !errors.Is(err, state.ErrNoState) {
			return err
		}
		if discardBackground {
			return nil
		}
		return handler(task, tomb)
	}
}

// OnDo helps to decide whether:
// 1. The task needs to be put on Wait (wait-on-error for refresh).
//
// 2. The error needs to be reported but safely ingored (ContextCancelled can
// happen if a user cancells or something gets interrupted during the execution
// due to abortion, e.g. a running hook is called off because their change was
// aborted.
//
// 3. The error needs to be reported as is which will abort the change (or the
// affected lanes).
func OnDo(handler state.HandlerFunc) state.HandlerFunc {
	return func(task *state.Task, tomb *tomb.Tomb) error {
		err := handler(task, tomb)
		if _, ok := err.(*state.Retry); ok {
			return err
		}

		st := task.State()
		st.Lock()
		defer st.Unlock()

		if err != nil {
			change := task.Change()
			if change.Kind() == "refresh" || change.Kind() == "launch" {
				var setup conflict.ChangeSetup
				if errKey := change.Get("wait-setup", &setup); errKey != nil {
					return errKey
				}

				mode, moderr := conflict.ParseMode(setup.Mode)
				if moderr != nil {
					return fmt.Errorf("internal error: unkown change mode: %s", setup.Mode)
				}

				if mode == conflict.ChangeWaitOnError {
					task.Errorf("%v", err)
					return &state.Wait{
						WaitedStatus: state.DoingStatus,
						Reason:       fmt.Sprintf("wait on error: %v", err),
					}
				}

			}
			return err
		}

		return nil
	}
}

func User(change *state.Change) (string, error) {
	var user string
	err := change.Get("user", &user)
	return user, err
}

func Workshop(task *state.Task) (string, error) {
	var w string
	err := task.Get("workshop", &w)
	return w, err
}

func Sdk(task *state.Task) (string, error) {
	var s string
	err := task.Get("sdk", &s)
	return s, err
}

func UserProjectWorkshop(task *state.Task) (string, *workshop.Project, string, error) {
	st := task.State()
	var prj workshop.Project
	var name string
	var user string

	st.Lock()
	id := task.ID()
	err := task.Get("project", &prj)
	st.Unlock()

	if err != nil {
		return "", nil, "", fmt.Errorf("cannot get project, task %q: %v", id, err)
	}

	st.Lock()
	err = task.Get("workshop", &name)
	st.Unlock()

	if err != nil {
		return "", nil, "", fmt.Errorf("cannot get workshop, task %q: %v", id, err)
	}

	st.Lock()
	changeId := task.Change().ID()
	err = task.Change().Get("user", &user)
	st.Unlock()

	if err != nil {
		return "", nil, "", fmt.Errorf("cannot get user name, change %q: %v", changeId, err)
	}

	return user, &prj, name, nil
}

func BackendContext(tomb *tomb.Tomb, user string, projectId string) (context.Context, context.CancelFunc) {
	ctx := tomb.Context(context.Background())
	ctxProject := context.WithValue(ctx, workshop.ContextProjectId, projectId)
	ctxUser := context.WithValue(ctxProject, workshop.ContextUser, user)
	ctxCancel, cancel := context.WithCancel(ctxUser)
	return ctxCancel, cancel
}

// InjectTasks makes all the halt tasks of the mainTask wait for extraTasks;
// extraTasks join the same lane and change as the mainTask.
func InjectTasks(mainTask *state.Task, extraTasks *state.TaskSet) {
	lanes := mainTask.Lanes()
	if len(lanes) == 1 && lanes[0] == 0 {
		lanes = nil
	}
	for _, l := range lanes {
		extraTasks.JoinLane(l)
	}

	chg := mainTask.Change()
	// Change shouldn't normally be nil, except for cases where
	// this helper is used before tasks are added to a change.
	if chg != nil {
		chg.AddAll(extraTasks)
	}

	// make all halt tasks of the mainTask wait on extraTasks
	ht := mainTask.HaltTasks()
	for _, t := range ht {
		t.WaitAll(extraTasks)
	}

	// make the extra tasks wait for main task
	extraTasks.WaitFor(mainTask)
}
