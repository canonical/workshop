package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"gopkg.in/tomb.v2"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
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
	var image workshop.BaseImage
	st.Lock()
	err = task.Get("workshop-base", &image)
	st.Unlock()
	if err != nil {
		return fmt.Errorf("internal error: %q workshop base image not found (task ID: %s)", w, task.ID())
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	reporter := &progress.Reporter{
		Name: task.ID(),
		Report: func(label string, done, total int) {
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

	var fileText string
	st.Lock()
	err = task.Get("workshop-file", &fileText)
	st.Unlock()
	if err != nil {
		return fmt.Errorf("internal error: %q workshop definition not found (task ID: %s)", w, task.ID())
	}

	var wf workshop.File
	if err := yaml.Unmarshal([]byte(fileText), &wf); err != nil {
		return fmt.Errorf("invalid workshop file: %w", err)
	}

	var sdkSnapshot string
	st.Lock()
	err = task.Get("sdk-snapshot", &sdkSnapshot)
	st.Unlock()
	if err != nil && !errors.Is(err, state.ErrNoState) {
		return fmt.Errorf("internal error: %q workshop snapshot not found (task ID: %s)", w, task.ID())
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

	if sdkSnapshot == "" {
		var image workshop.BaseImage
		st.Lock()
		err = task.Get("workshop-base", &image)
		st.Unlock()
		if err != nil {
			return fmt.Errorf("internal error: %q workshop base image not found (task ID: %s)", w, task.ID())
		}

		if err := m.backend.LaunchOrRebuildWorkshop(ctx, &wf, image); err != nil {
			return err
		}
		// Create workshop base and run directories
		fs, err := m.backend.WorkshopFs(ctx, wf.Name)
		if err != nil {
			return err
		}
		defer fs.Close()

		if err = fs.MkdirAll(dirs.WorkshopRunDir, 0o755); err != nil {
			return err
		}
	} else {
		if err = m.backend.Restore(ctx, w, sdkSnapshot, &wf); err != nil {
			return err
		}
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
	if err := os.MkdirAll(aptCache, 0o755); err != nil {
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

	var errs []error
	if err := m.cleanupWorkshopData(user, prj.ProjectId, w); err != nil {
		errs = append(errs, err)
	}
	if err := m.cleanupWorkshopCache(prj.ProjectId, w); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	st := task.State()
	st.Lock()
	for _, err := range errs[:len(errs)-1] {
		task.Errorf("%v", err)
	}
	st.Unlock()
	return errs[len(errs)-1]
}

func (m *WorkshopManager) cleanupWorkshopData(user, projectId, w string) error {
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

func removeIfEmpty(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	return err
}

func (m *WorkshopManager) doMountProject(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	// Configure workshop core properties: project directory
	prjMount := workshop.Mount{
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

	stopped := make(chan error)
	stopctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		// LXD has an internal timeout (30 seconds) for the operation,
		// if exceeded, the dealine error will be returned
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

	return m.backend.RemoveWorkshop(ctx, w)
}

func (m *WorkshopManager) doRemoveWorkshopStash(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.RemoveWorkshopStash(ctx, w)
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

	if err = m.backend.UnstashWorkshop(ctx, w); err != nil {
		return err
	}

	return m.backend.RemoveWorkshopStash(ctx, w)
}

func (m *WorkshopManager) doCreateStateStorage(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	volume := workshop.VolumeSetup{
		Name: workshop.WorkshopStateVolumeName(w, prj.ProjectId),
		Kind: "state-storage",
	}
	return m.backend.CreateVolume(ctx, volume)
}

func (m *WorkshopManager) doRemoveStateStorage(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.DeleteVolume(ctx, workshop.WorkshopStateVolumeName(w, prj.ProjectId))
}
