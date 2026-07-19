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

package handlersetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/tomb.v2"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/ifacestate/schema"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// OnUndo helps to skip the undo handler if the change is an abort-background refresh
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
// 2. The error needs to be reported but safely ignored (ContextCancelled can
// happen if a user cancels or something gets interrupted during the execution
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
					return fmt.Errorf("internal error: unknown change mode: %s", setup.Mode)
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

func MaybeLastIntactSdk(task *state.Task) (string, error) {
	var s string
	if err := task.Get("last-intact-sdk", &s); errors.Is(err, state.ErrNoState) {
		s = ""
	} else if err != nil {
		return "", fmt.Errorf("internal error: last intact SDK invalid (task ID: %s): %w", task.ID(), err)
	}
	return s, nil
}

// SetWorkshopFile stores a workshop file in a Task as a YAML string, to avoid
// converting ints -> JSON numbers -> floats. This can happen on unmarshalling
// plug and slot attributes which are weakly typed.
func SetWorkshopFile(task *state.Task, file *workshop.File) {
	task.Set("workshop-file", (*fileText)(file))
}

// WorkshopFile reads a workshop file set by SetWorkshopFile.
func WorkshopFile(task *state.Task, w string) (*workshop.File, error) {
	var file workshop.File
	if err := task.Get("workshop-file", (*fileText)(&file)); err != nil {
		return nil, fmt.Errorf("internal error: %q workshop definition not found (task ID: %s)", w, task.ID())
	}
	return &file, nil
}

// fileText is a shim which (un)marshals a workshop file as a YAML string. It
// folds YAML marshalling errors into JSON marshalling errors, to avoid having
// to handle them in SetWorkshopFile.
type fileText workshop.File

func (f *fileText) MarshalJSON() ([]byte, error) {
	text, err := yaml.Marshal(f)
	if err != nil {
		return nil, err
	}

	return json.Marshal(string(text))
}

func (f *fileText) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}

	return yaml.Unmarshal([]byte(text), f)
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

type Age string

const (
	// OldWorkshop indicates the state of the workshop before a Change.
	OldWorkshop = Age("old")
	// NewWorkshop indicates the planned state of the workshop after a Change.
	NewWorkshop = Age("new")
	// OldStash indicates the state of the stash before a Change.
	OldStash = Age("old-stash")
)

func WorkshopFormat(change *state.Change, w string, age Age) (sdk.Revision, error) {
	var format sdk.Revision
	if err := change.Get(WorkshopFormatKey(w, age), &format); err != nil {
		return sdk.Revision{}, fmt.Errorf("internal error: %q workshop %s format not found (change ID: %s)", w, age, change.ID())
	}
	return format, nil
}

func WorkshopFormatKey(w string, age Age) string {
	return strings.Join([]string{w, string(age), "format"}, "_")
}

func WorkshopBase(change *state.Change, w string, age Age) (workshop.BaseImage, error) {
	var image workshop.BaseImage
	if err := change.Get(WorkshopBaseKey(w, age), &image); err != nil {
		return workshop.BaseImage{}, fmt.Errorf("internal error: %q workshop %s base image not found (change ID: %s)", w, age, change.ID())
	}
	return image, nil
}

func WorkshopBaseKey(w string, age Age) string {
	return strings.Join([]string{w, string(age), "base"}, "_")
}

func WorkshopSdks(change *state.Change, w string, age Age) ([]sdk.Setup, error) {
	var sdks []sdk.Setup
	if err := change.Get(WorkshopSdksKey(w, age), &sdks); err != nil {
		return nil, fmt.Errorf("internal error: %q workshop %s SDKs not found (change ID: %s)", w, age, change.ID())
	}
	return sdks, nil
}

func WorkshopSdksKey(w string, age Age) string {
	return strings.Join([]string{w, string(age), "sdks"}, "_")
}

func WorkshopSdkSnapshot(change *state.Change, w, lastIntact string) (*workshop.Snapshot, error) {
	if lastIntact == "" {
		return workshopBaseOnly(change, w, NewWorkshop)
	}

	snapshot, err := workshopSnapshot(change, w, NewWorkshop)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(snapshot.Sdks, func(s sdk.ContentID) bool {
		return s.Name == lastIntact
	})
	if idx < 0 {
		return nil, fmt.Errorf("internal error: %q workshop has no %q SDK", w, lastIntact)
	}
	snapshot.Sdks = snapshot.Sdks[:idx+1]

	return snapshot, nil
}

func workshopSnapshot(change *state.Change, w string, age Age) (*workshop.Snapshot, error) {
	base, err := workshopBaseOnly(change, w, age)
	if err != nil {
		return nil, err
	}

	sdks, err := WorkshopSdks(change, w, age)
	if err != nil {
		return nil, err
	}

	snapshot := workshop.SdkSnapshot(base.Format, base.Image, sdks)
	return &snapshot, nil
}

func workshopBaseOnly(change *state.Change, w string, age Age) (*workshop.Snapshot, error) {
	format, err := WorkshopFormat(change, w, age)
	if err != nil {
		return nil, err
	}

	image, err := WorkshopBase(change, w, age)
	if err != nil {
		return nil, err
	}

	snapshot := workshop.SdkSnapshot(format, image, nil)
	return &snapshot, nil
}

func WorkshopPreservedConns(change *state.Change, w string) (map[string]schema.PreservedConn, error) {
	var prev map[string]schema.PreservedConn
	if err := change.Get(WorkshopPreservedConnsKey(w), &prev); errors.Is(err, state.ErrNoState) {
		prev = nil
	} else if err != nil {
		return nil, fmt.Errorf("internal error: %q workshop previous connections not found (change ID: %s)", w, change.ID())
	}
	return prev, nil
}

func WorkshopPreservedConnsKey(w string) string {
	return w + "_conns"
}

// SdkVolumeLastUsed returns the most recent Task of the given Change that
// stopped using the given SDK volume.
func SdkVolumeLastUsed(change *state.Change, setup sdk.Setup) (string, time.Time, error) {
	var lastUsed taskTime
	if err := change.Get(SdkVolumeLastUsedKey(setup.Name, setup.Revision), &lastUsed); err != nil {
		return "", time.Time{}, err
	}
	return lastUsed.Task, lastUsed.Time, nil
}

// SetSdkVolumeLastUsed records the most recent Task of the given Change that
// stopped using the given SDK volume, and a timestamp set to shortly before
// that happened.
func SetSdkVolumeLastUsed(change *state.Change, setup sdk.Setup, task string, time time.Time) {
	lastUsed := taskTime{Task: task, Time: time}
	change.Set(SdkVolumeLastUsedKey(setup.Name, setup.Revision), lastUsed)
}

func SdkVolumeLastUsedKey(sk string, revision sdk.Revision) string {
	return sdk.VolumeName(sk, revision) + "_last-used"
}

type taskTime struct {
	Task string    `json:"task"`
	Time time.Time `json:"time"`
}

// SnapshotLastUsed returns the most recent Task of the given Change that
// stopped using the given snapshot or a descendant thereof.
func SnapshotLastUsed(change *state.Change, snapshot workshop.Snapshot) (string, time.Time, error) {
	var tasks []taskTimeWorkshop
	if err := change.Get("snapshots-last-used", &tasks); err != nil {
		return "", time.Time{}, err
	}

	var task string
	var latest time.Time
	for _, t := range tasks {
		s, err := workshopSnapshot(change, t.Workshop, t.Age)
		if err != nil {
			return "", time.Time{}, err
		}
		if !s.IsBasedOn(snapshot) {
			continue
		}

		if task == "" || t.Time.After(latest) || (t.Time.Equal(latest) && t.Task > task) {
			task = t.Task
			latest = t.Time
		}
	}

	if task == "" {
		return "", time.Time{}, state.ErrNoState
	}
	return task, latest, nil
}

// SnapshotLastUsedByTask returns the snapshot that the given Task stopped
// using, if any.
func SnapshotLastUsedByTask(task *state.Task) (*workshop.Snapshot, time.Time, error) {
	change := task.Change()

	var workshops []taskTimeWorkshop
	if err := change.Get("snapshots-last-used", &workshops); err != nil {
		return nil, time.Time{}, err
	}

	for _, w := range workshops {
		if w.Task != task.ID() {
			continue
		}

		snapshot, err := workshopSnapshot(change, w.Workshop, w.Age)
		if err != nil {
			return nil, time.Time{}, err
		}

		// Each Task deals with at most one snapshot, so we return early.
		return snapshot, w.Time, nil
	}
	return nil, time.Time{}, state.ErrNoState
}

// SetSnapshotLastUsed records the most recent Task of the given Change that
// stopped using a snapshot. The given workshop name and age are used to
// identify the specific snapshot (which is already stored in the Change; see
// e.g. WorkshopSdks).
func SetSnapshotLastUsed(change *state.Change, w string, age Age, task string, time time.Time) error {
	var tasks []taskTimeWorkshop
	if err := change.Get("snapshots-last-used", &tasks); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	tasks = append(tasks, taskTimeWorkshop{
		taskTime: taskTime{Time: time, Task: task},
		Workshop: w,
		Age:      age,
	})

	change.Set("snapshots-last-used", tasks)
	return nil
}

type taskTimeWorkshop struct {
	taskTime
	Workshop string `json:"workshop"`
	Age      Age    `json:"age"`
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

	// Update status of the injected tasks in case the main task was aborted
	// already. Lets consider what status the main task can have at the time
	// of the call:
	// - Do (request processing stage, change is not in a Doing state)
	// - Doing (Do handler is executed for the main task)
	// - Abort (the task was aborted *before* the InjectTasks was called AND the
	// state was locked but *after* the Do handler for the task was started)
	// - Undoing (Undo handler is executed for the task)
	status := mainTask.Status()
	if status == state.AbortStatus {
		for _, t := range extraTasks.Tasks() {
			t.SetStatus(state.HoldStatus)
		}
	}
}
