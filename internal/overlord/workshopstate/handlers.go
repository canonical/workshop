package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

var StopLogInterval = 30 * time.Second

var StopWorkshop = (workshop.Backend).StopWorkshop

func (m *WorkshopManager) undoCreateWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, workshop, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.RemoveWorkshop(ctx, workshop)
}

func (m *WorkshopManager) doDownloadBase(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()
	var base string
	st.Lock()
	err = task.Get("workshop-base", &base)
	st.Unlock()
	if err != nil {
		return fmt.Errorf("internal error: %q workshop configuration not found (task ID: %s)", w, task.ID())
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

	return m.backend.Download(ctx, base, reporter)
}

func (m *WorkshopManager) doCreateWorkshop(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	var wf workshop.File
	st.Lock()
	err = task.Get("workshop-file", &wf)
	st.Unlock()

	if err != nil {
		return fmt.Errorf("internal error: %q workshop configuration not found (task ID: %s)", w, task.ID())
	}

	rev := revert.New()
	defer rev.Fail()

	if err = m.backend.LaunchWorkshop(ctx, &wf); err != nil {
		return err
	}

	cleanupCtx := context.WithoutCancel(ctx)
	rev.Add(func() {
		cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
		defer cancel()
		if err1 := m.backend.RemoveWorkshop(cleanupCtx, w); err1 != nil {
			logger.Noticef("On doCreateWorkshop: cannot remove %q workshop on cleanup: %v", w, err1)
		}
	})

	// Create workshop base and run directories
	fs, err := m.backend.WorkshopFs(ctx, wf.Name)
	if err != nil {
		return err
	}
	defer fs.Close()

	if err = fs.MkdirAll(dirs.WorkshopRunDir, 0755); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *WorkshopManager) doCreateAptCache(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	// TODO: The apt cache directory usually has mode 0755.
	// At present CreateVolume doesn't provide a way to specify this,
	// and the LXD backend will default to mode 0711 for new volumes.
	//
	// It seems possible to override the LXD default by restoring a "backup",
	// which is a tarball containing the volume contents and a YAML metadata file.
	//
	// Currently the difference in modes doesn't seem to cause any issues,
	// so the effort required to remedy this probably isn't worth it.
	volume := workshop.AptCacheVolumeName(w, prj.ProjectId)
	err = m.backend.CreateVolume(ctx, volume)
	if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
		logger.Debugf("reusing existing storage volume %q", volume)
		return nil
	}
	return err
}

func (m *WorkshopManager) doRemoveAptCache(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.DeleteVolume(ctx, workshop.AptCacheVolumeName(w, prj.ProjectId))
}

func (m *WorkshopManager) doMountProject(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	// Configure workshop core properties: project directory
	var prjMount = workshop.Mount{Name: workshop.ConfigProjectPathDevice, What: prj.Path, Where: workshop.WorkshopProjectPath}
	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.AddWorkshopMount(ctx, w, prjMount)
}

func (m *WorkshopManager) undoMountProject(task *state.Task, tomb *tomb.Tomb) error {
	// No need to undo because the mount will be removed with the workshop
	return nil
}

func (m *WorkshopManager) doMountAptCache(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	volume := workshop.AptCacheVolumeName(w, prj.ProjectId)
	return m.backend.AttachVolume(ctx, w, volume, dirs.AptCachePath)
}

func (m *WorkshopManager) undoMountAptCache(task *state.Task, tomb *tomb.Tomb) error {
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

	if err := m.backend.RemoveWorkshop(ctx, w); err != nil {
		return err
	}

	if err = m.cleanupWorkshopData(user, prj.ProjectId, w); err != nil {
		st := task.State()
		st.Lock()
		task.Logf("%v", err)
		st.Unlock()
	}

	return nil
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
	return nil
}

func (m *WorkshopManager) doCreateStateStorage(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	return m.backend.CreateVolume(ctx, workshop.WorkshopStateVolumeName(w, prj.ProjectId))
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

type cleanupError struct {
	errs []error
}

func (e *cleanupError) Error() string {
	return fmt.Sprintf("workshop cleanup errors: %v", e.errs)
}

func (m *WorkshopManager) cleanupWorkshopData(user, projectId, w string) error {
	usr, err := workshop.LookupUsername(user)
	if err != nil {
		return err
	}

	var errors []error
	projectContent := sdk.ProjectContentDir(usr.HomeDir, projectId)

	var contentDirs []fs.DirEntry
	if contentDirs, err = os.ReadDir(projectContent); err != nil {
		errors = append(errors, fmt.Errorf("%q workshop content directory is not available: %v", w, err))
	}

	// Remove all the possible workshop default mount interface 'source'
	// locations that could have existed over the workshop's lifecycle. These
	// are not only the ones that exist by the time we remove the workshop.
	// Imagine the following scenario. An SDK added to a workshop and created a
	// mount interface plug. Then, the SDK was removed from the workshop via
	// refresh. When we call 'workshop remove', the plug does not exist anymore
	// (nor the SDK profile for this plug); however, the content is still stored
	// on the host and must also be removed alongside the workshop.
	for _, dir := range contentDirs {
		// Remove all default content dirs that belong the workshop. These will be
		// named as <workshop>_<sdk>_<plug>.sdk
		base := filepath.Base(dir.Name())
		if dir.IsDir() && strings.HasPrefix(base, w+"_") {
			if err := os.RemoveAll(filepath.Join(projectContent, dir.Name())); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return &cleanupError{errs: errors}
	}

	return nil
}
