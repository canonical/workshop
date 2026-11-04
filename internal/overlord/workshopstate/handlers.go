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

package workshopstate

import (
	"context"
	"errors"
	"os"
	"slices"
	"syscall"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/workshop"
)

var StopLogInterval = 30 * time.Second

var StopWorkshop = (workshop.Backend).StopWorkshop

func (m *WorkshopManager) doDownloadBase(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()
	st.Lock()
	image, err := WorkshopBase(task.Change(), w, NewWorkshop)
	st.Unlock()
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	reporter := &progress.Reporter{
		Name: task.ID(),
		Report: func(label string, done, total int64) {
			st.Lock()
			task.SetProgress(label, done, total)
			st.Unlock()
		},
	}

	return m.backend.DownloadBase(ctx, image, reporter)
}

func (m *WorkshopManager) doConstructWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	st.Lock()
	wf, err := WorkshopFile(task.Change(), w, NewWorkshop)
	st.Unlock()
	if err != nil {
		return err
	}

	st.Lock()
	lastIntact, err := MaybeLastIntactSdk(task)
	st.Unlock()
	if err != nil {
		return err
	}

	st.Lock()
	snapshot, err := WorkshopSdkSnapshot(task.Change(), w, lastIntact)
	st.Unlock()
	if err != nil {
		return err
	}

	rev := revert.New()
	defer rev.Fail()

	if task.Kind() == "create-workshop" {
		cleanupCtx := context.WithoutCancel(ctx)
		rev.Add(func() {
			cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
			defer cancel()

			// This may fail if the first workshop launch has failed for some
			// reason; it is safe to ignore the error in that case.
			if reverr := m.backend.RemoveWorkshop(cleanupCtx, w); reverr != nil {
				logger.Noticef("On doConstructWorkshop: cannot remove %q workshop on cleanup: %v", w, reverr)
			}
		})
	}

	if err := m.backend.LaunchOrRebuildWorkshop(ctx, wf, *snapshot); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *WorkshopManager) undoRebuildWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	// The undo handler for stash-workshop rolls back both stash-workshop
	// and rebuild-workshop.
	return nil
}

func (m *WorkshopManager) doCreateWorkshopStorage(task *state.Task, tomb *tomb.Tomb) error {
	username, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	// The workshop's root user needs write access to the apt cache.
	// We use the client user (mapped to the workshop user) for simplicity.
	// Another approach would be to add shift=true to the mount device,
	// but it's risky in terms of security (e.g. setuid binaries).
	user, err := osutil.UserLookup(username)
	if err != nil {
		return err
	}
	uid, gid, err := osutil.UidGid(user)
	if err != nil {
		return err
	}

	aptCache := workshop.AptCacheDir(prj.ProjectId, w)
	if err := os.MkdirAll(aptCache, 0755); err != nil {
		return err
	}
	if err = sys.ChownPath(aptCache, uid, gid); err != nil {
		return &os.PathError{Op: "chown", Path: aptCache, Err: err}
	}

	return nil
}

func (m *WorkshopManager) doRemoveWorkshopStorage(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	if err := m.cleanupWorkshopUserData(user, prj.ProjectId, w); err != nil {
		task.State().Lock()
		task.Logf("cannot remove %q workshop user data: %v", w, err)
		task.State().Unlock()
	}

	if err := m.cleanupWorkshopCache(prj.ProjectId, w); err != nil {
		task.State().Lock()
		task.Logf("cannot remove %q workshop cache: %v", w, err)
		task.State().Unlock()
	}

	if err := m.cleanupWorkshopData(prj.ProjectId, w); err != nil {
		task.State().Lock()
		task.Logf("cannot remove %q workshop data: %v", w, err)
		task.State().Unlock()
	}

	return nil
}

func (m *WorkshopManager) cleanupWorkshopUserData(user, projectId, w string) error {
	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return err
	}

	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)
	workshopUserData := workshop.UserData(userDataDir, projectId, w)
	if err := os.RemoveAll(workshopUserData); err != nil {
		return err
	}

	return removeIfEmpty(workshop.ProjectUserData(userDataDir, projectId))
}

func (m *WorkshopManager) cleanupWorkshopCache(projectId, w string) error {
	cache := workshop.CacheDir(projectId, w)
	if err := os.RemoveAll(cache); err != nil {
		return err
	}

	return removeIfEmpty(workshop.ProjectCacheDir(projectId))
}

func (m *WorkshopManager) cleanupWorkshopData(projectId, w string) error {
	workshopData := workshop.DataDir(projectId, w)
	if err := os.RemoveAll(workshopData); err != nil {
		return err
	}

	return removeIfEmpty(workshop.ProjectDataDir(projectId))
}

func removeIfEmpty(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	return err
}

func (m *WorkshopManager) doConfigureTimezone(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	timezone, err := osutil.Timezone()
	if err != nil {
		st := task.State()
		st.Lock()
		defer st.Unlock()
		st.Warnf("cannot determine system time zone: %v", err)
		return nil
	}

	// Set /etc/localtime in workshop. Ubuntu 22.04 and below use a patched
	// systemd that will also write to /etc/timezone.
	args := workshop.Execution{
		ExecArgs: workshop.ExecArgs{
			Command: []string{
				"timedatectl",
				"set-timezone",
				timezone,
			},
			WorkDir: "/",
			Timeout: time.Minute,
		},
	}
	exectx, err := m.backend.Exec(ctx, w, &args)
	if err != nil {
		return err
	}
	if err := exectx.WaitExecution(ctx); err != nil {
		return err
	}

	// Ensure /etc/timezone is updated (for Ubuntu 24.04 and above). It's
	// looking like Ubuntu 26.04 will remove /etc/timezone, so this command
	// only affects Ubuntu 24.04. It can be removed when we drop support.
	args = workshop.Execution{
		ExecArgs: workshop.ExecArgs{
			Command: []string{
				"dpkg-reconfigure",
				"--frontend=noninteractive",
				"tzdata",
			},
			WorkDir: "/",
			Timeout: time.Minute,
		},
	}
	exectx, err = m.backend.Exec(ctx, w, &args)
	if err != nil {
		return err
	}
	err = exectx.WaitExecution(ctx)
	var errExec *workshop.ErrExec
	if errors.As(err, &errExec) && errExec.Status == osutil.CommandNotFound {
		// If dpkg-reconfigure doesn't exist, /etc/timezone probably
		// doesn't either.
		err = nil
	}
	return err
}

func (m *WorkshopManager) doMountProject(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	// Configure workshop core properties: project directory
	var prjMount = workshop.Mount{
		Name:  workshop.ConfigProjectPathDevice,
		Type:  workshop.HostWorkshop,
		What:  prj.Path,
		Where: workshop.WorkshopProjectPath,
	}
	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.AddWorkshopMount(ctx, w, prjMount)
}

func (m *WorkshopManager) undoMountProject(task *state.Task, tomb *tomb.Tomb) error {
	// No need to undo because the mount will be removed with the workshop
	return nil
}

func (m *WorkshopManager) doStart(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	task.Set("force", true)
	st.Unlock()

	return m.backend.StartWorkshop(ctx, w)
}

func (m *WorkshopManager) doStop(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	var force bool
	st := task.State()
	st.Lock()
	// false is by default
	_ = task.Get("force", &force)
	st.Unlock()

	var stopped = make(chan error)
	stopctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		// LXD has an internal timeout (30 seconds) for the operation,
		// if exceeded, the deadline error will be returned
		stopped <- StopWorkshop(m.backend, stopctx, w, force)
	}()

	for {
		select {
		case err = <-stopped:
			return err
		case <-time.After(StopLogInterval):
			st.Lock()
			task.Logf("Still waiting for %q to stop; no change in the last 30 seconds...", w)
			st.Unlock()
		}
	}
}

func (m *WorkshopManager) doRemoveWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	age := OldWorkshop
	if task.Kind() == "create-workshop" {
		age = NewWorkshop
	}
	st := task.State()
	st.Lock()
	err = SetSnapshotLastUsed(task.Change(), w, age, task.ID(), time.Now())
	st.Unlock()
	if err != nil {
		return err
	}

	return m.backend.RemoveWorkshop(ctx, w)
}

func (m *WorkshopManager) doRemoveWorkshopStash(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	age := OldWorkshop
	if task.Change().Kind() == "remove" {
		age = OldStash
	}
	err = SetSnapshotLastUsed(task.Change(), w, age, task.ID(), time.Now())
	st.Unlock()
	if err != nil {
		return err
	}

	if err := m.backend.RemoveWorkshopStash(ctx, w); err != nil {
		task.State().Lock()
		task.Logf("cannot remove %q workshop stash: %v", w, err)
		task.State().Unlock()
	}

	return nil
}

func (m *WorkshopManager) doStashWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	if err = m.backend.StashWorkshop(ctx, w); err != nil {
		return err
	}
	return nil
}

func (m *WorkshopManager) undoStashWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	err = SetSnapshotLastUsed(task.Change(), w, NewWorkshop, task.ID(), time.Now())
	st.Unlock()
	if err != nil {
		return err
	}

	if err = m.backend.UnstashWorkshop(ctx, w); err != nil {
		return err
	}

	return m.backend.RemoveWorkshopStash(ctx, w)
}

func (m *WorkshopManager) doCreateStateStorage(task *state.Task, tomb *tomb.Tomb) error {
	username, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	// The workshop's root user needs write access to the state storage.
	// We use the client user (mapped to the workshop user) for simplicity.
	// Another approach would be to add shift=true to the mount device,
	// but it's risky in terms of security (e.g. setuid binaries).
	user, err := osutil.UserLookup(username)
	if err != nil {
		return err
	}
	uid, gid, err := osutil.UidGid(user)
	if err != nil {
		return err
	}

	storage := workshop.StateStorageDir(prj.ProjectId, w)
	if err := os.MkdirAll(storage, 0755); err != nil {
		return err
	}
	if err = sys.ChownPath(storage, uid, gid); err != nil {
		return &os.PathError{Op: "chown", Path: storage, Err: err}
	}

	return nil
}

func (m *WorkshopManager) doRemoveStateStorage(task *state.Task, tomb *tomb.Tomb) error {
	_, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	storage := workshop.StateStorageDir(prj.ProjectId, w)
	if err := os.RemoveAll(storage); err != nil {
		task.State().Lock()
		task.Logf("cannot remove %q workshop state storage: %v", w, err)
		task.State().Unlock()
	}

	return nil
}

// doDeleteUnusedSnapshots is a cleanup handler that deletes unused snapshots.
// It is called periodically to ensure that snapshots that are no longer
// needed are removed from the system. The cleanup is done only if there are no
// tasks in progress that might use the snapshot. The cleanup is also
// throttled to avoid deleting snapshots too frequently. The cooldown time is
// defined by the snapshotCooldownTime constant.
//
// There are multiple scenarios when this cleanup is triggered:
//   - A workshop was removed (remove-workshop or, if launch failed, create-workshop task)
//   - A stash was removed (after a successful refresh, or if a workshop is
//     removed while a refresh is paused)
//   - A workshop was replaced (undo stash-workshop task)
func (m *WorkshopManager) doDeleteUnusedSnapshots(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, _, err := UserProjectWorkshop(task)
	if err != nil {
		// Log as an internal error, no need to retry again.
		logger.Noticef("On WorkshopManager.doDeleteUnusedSnapshots: internal error: %v", err)
		return nil
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	defer st.Unlock()

	snapshot, lastUsed, err := SnapshotLastUsedByTask(task)
	if err != nil {
		if !errors.Is(err, state.ErrNoState) {
			logger.Noticef("On WorkshopManager.doDeleteUnusedSnapshots: internal error: %v", err)
		}
		return nil
	}

	for range snapshot.Sdks {
		removed, err := m.deleteUnusedSnapshot(ctx, task, *snapshot, lastUsed)
		if err != nil {
			return err
		}
		if !removed {
			return nil
		}
		snapshot.Sdks = snapshot.Sdks[:len(snapshot.Sdks)-1]
	}
	return nil
}

func (m *WorkshopManager) deleteUnusedSnapshot(ctx context.Context, task *state.Task, snapshot workshop.Snapshot, lastUsed time.Time) (bool, error) {
	// Check for changes that might use the same snapshot. If one is in
	// progress, removing the snapshot will break the launch or rebuild. If
	// another task stopped using the snapshot after us, we can let it handle
	// the cleanup.
	st := task.State()
	changes := st.Changes()
	blocking := []string{"launch", "refresh", "remove"}
	for _, change := range changes {
		// Other changes like "exec" do not affect snapshots, so we can skip them.
		if !slices.Contains(blocking, change.Kind()) {
			continue
		}
		if !change.IsReady() {
			return false, &state.Retry{}
		}

		taskID, time, err := SnapshotLastUsed(change, snapshot)
		if errors.Is(err, state.ErrNoState) {
			continue
		}
		if err != nil {
			logger.Noticef("On WorkshopManager.doDeleteUnusedSnapshots: internal error: %v", err)
			return false, nil
		}

		if taskID == task.ID() {
			continue
		}
		// Just compare task IDs lexicographically; it doesn't really matter
		// which one handles the cleanup as long as there's consensus.
		if time.After(lastUsed) || (time.Equal(lastUsed) && taskID > task.ID()) {
			return false, nil
		}

		// Earlier tasks may be responsible for cleaning up descendant
		// snapshots. We wait for them to be clean to avoid deleting snapshots
		// in the wrong order.
		if t := st.Task(taskID); t == nil {
			logger.Noticef("On WorkshopManager.doDeleteUnusedSnapshots: internal error: task %s not found", taskID)
			return false, nil
		} else if !t.IsClean() {
			return false, &state.Retry{}
		}
	}

	if time.Since(lastUsed) < snapshotCooldownTime {
		return false, &state.Retry{}
	}

	info, err := m.backend.Snapshot(ctx, snapshot)
	if errors.Is(err, workshop.ErrSnapshotNotFound) || (err != nil && info != nil) {
		// Snapshot not found, or a hash collision.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if len(info.Workshops) > 0 {
		return false, nil
	}

	if err := m.backend.RemoveSnapshot(ctx, snapshot); err != nil {
		return false, err
	}
	logger.Debugf("On WorkshopManager.doDeleteUnusedSnapshots: snapshot %v for task %s was deleted", len(snapshot.Sdks), task.ID())
	return true, nil
}
