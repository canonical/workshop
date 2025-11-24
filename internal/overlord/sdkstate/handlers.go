package sdkstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces/policy"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/workshop"
)

func SdkSetup(task *state.Task) (sdk.Setup, error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var retrieveId string
	var sdkSetup sdk.Setup

	if err := task.Get("sdk-retrieve-task", &retrieveId); err != nil {
		return sdk.Setup{}, err
	}

	retrieve := st.Task(retrieveId)
	if retrieve == nil {
		return sdk.Setup{}, fmt.Errorf("internal error: no corresponding retrieve-sdk task found")
	}

	if err := retrieve.Get("sdk-setup", &sdkSetup); err != nil {
		return sdk.Setup{}, err
	}
	return sdkSetup, nil
}

func maybeSdkYaml(task *state.Task) (string, error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var retrieveId string
	var sdkYaml string

	if err := task.Get("sdk-retrieve-task", &retrieveId); err != nil {
		return "", err
	}

	retrieve := st.Task(retrieveId)
	if retrieve == nil {
		return "", fmt.Errorf("internal error: no corresponding retrieve-sdk task found")
	}

	if err := retrieve.Get("sdk-yaml", &sdkYaml); err != nil {
		return "", nil
	}
	return sdkYaml, nil
}

func (m *SdkManager) doRetrieveSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, _, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()
	var rec sdk.Setup

	st.Lock()
	err = task.Get("sdk-setup", &rec)
	st.Unlock()
	if err != nil {
		return err
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

	var meta *sdk.Meta
	if rec.Source == sdk.SystemSource {
		meta, err = system.RetrieveSystemSdk(rec, reporter)
	} else {
		st.Lock()
		store := sdk.StoreService(st)
		st.Unlock()

		meta, err = store.DownloadSdk(ctx, rec, reporter)
	}
	if err != nil {
		return err
	}

	// TODO: We should be downloading a specific revision. Remove this when
	// the Store supports that (it will probably have to be removed anyway
	// since DownloadSdk won't return metadata after that).
	if meta.Revision != rec.Revision {
		st.Lock()
		task.Set("sdk-setup", meta.Setup)
		// Ideally we would validate the YAML here, but we can't check
		// the base without knowing about the workshop. And this task
		// should remain workshop-agnostic if possible. Instead we
		// pass it to doInstallSdk and validate there.
		task.Set("sdk-yaml", meta.SdkYAML)
		st.Unlock()
	}

	volume := workshop.VolumeSetup{
		Name:     sdk.VolumeName(meta.Name, meta.Revision),
		Kind:     "sdk",
		Sha3_384: meta.Sha3_384,
		Sdk:      meta.Name,
		Revision: meta.Revision,
	}
	file, err := os.Open(meta.Filepath())
	if err != nil {
		return err
	}
	defer file.Close()
	err = m.backend.ImportVolume(ctx, volume, file)
	if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
		logger.Debugf("SDK Manager on maybeCreateVolume: reuse existing SDK volume %q", volume.Name)
		return nil
	}

	return err
}

func (m *SdkManager) doInstallSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	sdkSetup, err := SdkSetup(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	// Directory: /var/lib/workshop/sdk/<name>/
	fs, err := m.backend.WorkshopFs(ctx, w)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err = fs.MkdirAll(dirs.WorkshopSdksDir, 0o755); err != nil {
		return err
	}

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	// TODO: Remove this section when we can download a specific SDK
	// revision from the Store (this revision was validated earlier).
	sdkYaml, err := maybeSdkYaml(task)
	if err != nil {
		return err
	}
	if sdkYaml != "" {
		if err := workshop.ValidateSdkInfo(project.ProjectId, wp.File, sdkSetup.Name, sdkYaml); err != nil {
			return err
		}
	}

	rev := revert.New()
	defer rev.Fail()

	if err := m.mountSdk(ctx, user, project, w, sdkSetup); err != nil {
		return err
	}
	st := task.State()
	rev.Add(func() {
		if reverr := m.unmountSdk(ctx, w, sdkSetup); reverr != nil {
			st.Lock()
			task.Logf("Install SDK cleanup: could not unmount %q SDK: %v", sdkSetup.Name, reverr)
			st.Unlock()
		}
	})

	if err = wp.AddSdk(ctx, sdkSetup); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *SdkManager) mountSdk(ctx context.Context, user string, project *workshop.Project, w string, sdkSetup sdk.Setup) error {
	// Mount the SDK content at the workshop location.
	name := sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision)
	sdkPath := sdk.SdkDir(sdkSetup.Name)

	if sdkSetup.IsVolume() {
		return m.backend.AttachVolume(ctx, w, name, sdkPath, true)
	}

	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	sdkDir := workshop.LocalSdkDir(userDataDir, project.ProjectId, w, sdkSetup.Name)
	what := filepath.Join(sdkDir, sdkSetup.Sha3_384)

	mnt := workshop.Mount{
		Name:     name,
		Type:     workshop.HostWorkshop,
		What:     what,
		Where:    sdkPath,
		ReadOnly: true,
	}
	return m.backend.AddWorkshopMount(ctx, w, mnt)
}

func (m *SdkManager) doUninstallSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	sdkSetup, err := SdkSetup(task)
	if err != nil {
		return err
	}

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	if err := wp.RemoveSdk(ctx, sdkSetup.Name); err != nil {
		return err
	}

	return m.unmountSdk(ctx, w, sdkSetup)
}

func (m *SdkManager) unmountSdk(ctx context.Context, w string, sdkSetup sdk.Setup) error {
	name := sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision)
	if sdkSetup.IsVolume() {
		return m.backend.DetachVolume(ctx, w, name)
	}
	return m.backend.RemoveWorkshopMount(ctx, w, name)
}

func (m *SdkManager) doRegisterSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	setup, err := SdkSetup(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	info, err := wp.SdkInfo(ctx, setup.Name)
	if err != nil {
		return err
	}

	if len(info.BadInterfaces) > 0 {
		return fmt.Errorf("%s", sdk.BadInterfacesSummary(info))
	}

	if err = policy.CheckInterfaces(info); err != nil {
		return err
	}

	// add SDK's plugs and slots
	return m.repo.AddSdk(info)
}

func (m *SdkManager) doUnregisterSdk(task *state.Task, tomb *tomb.Tomb) error {
	_, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	setup, err := SdkSetup(task)
	if err != nil {
		return err
	}

	return m.repo.RemoveSdk(project.ProjectId, w, setup.Name)
}

func (m *SdkManager) doSnapshotSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()
	var sdk string

	st.Lock()
	err = task.Get("sdk", &sdk)
	st.Unlock()
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	return m.backend.Snapshot(ctx, w, sdk)
}

type SdkVolumeCooldownTimeKey string

// doDeleteUnusedSdkVolumes is a cleanup handler that deletes unused SDK volumes.
// It is called periodically to ensure that SDK volumes that are no longer
// needed are removed from the system. The cleanup is done only if there are no
// tasks in progress that use the same SDK volume. The cleanup is also
// throttled to avoid deleting volumes too frequently. The cooldown time is
// defined by the sdkVolumeCooldownTime constant.
//
// There are multiple scenarios when this cleanup is triggered:
//   - A workshop was removed and an SDK volume was detached from it (unregister-sdk task)
//   - An SDK volume was detached during a refresh change,
//     and is no longer used by any other workshop (unregister-sdk or, if refresh failed, install-sdk task)
//   - A workshop launch failed (install-sdk task)
func (m *SdkManager) doDeleteUnusedSdkVolumes(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()

	sdkSetup, err := SdkSetup(task)
	if err != nil {
		logger.Debugf("On SdkManager.Cleanup: the %q task is not associated with a SDK setup", task.ID())
		return nil
	}

	if !sdkSetup.IsVolume() || sdkSetup.Source == sdk.SystemSource {
		return nil
	}

	user, prj, _, err := UserProjectWorkshop(task)
	if err != nil {
		// Log as an internal error, no need to retry again.
		logger.Debugf("On SdkManager.Cleanup: internal error: %v", err)
		return nil
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st.Lock()
	defer st.Unlock()

	if task.Kind() == "install-sdk" && task.Status() == state.DoneStatus {
		// If the launch or refresh was successful, no need to clean up the SDK volume.
		return nil
	}

	vk := SdkVolumeCooldownTimeKey(sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
	cooldownStart, ok := st.Cached(vk).(time.Time)
	if !ok {
		// No cooldown start time cached means that clean up has just started or
		// workshopd was restarted.
		st.Cache(vk, task.ReadyTime())
		// We need to return here to give other competing cleanup tasks a chance
		// to find out who was the last one to initiate the cleanup.
		return fmt.Errorf("new cooldown start time for %q SDK volume: %v", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision), task.ReadyTime())
	} else {
		// Imagine the situation when a change with multiple tasks that use this
		// volume is in progress. Every unregister-sdk task will initiate a
		// cleanup after the change is ready. All cleanups are unarranged and
		// can run concurrently. We need to ensure that only one of them will
		// continue to track the volume cooldown time.
		readyTime := task.ReadyTime()
		if readyTime.Before(cooldownStart) {
			// Another more recent task initiated cleanup for this SDK
			// volume, no need to continue.
			return nil
		}
		if readyTime.After(cooldownStart) {
			// Update the cooldown start time to the current task ready time.
			st.Cache(vk, readyTime)
			// We need to return here to give other competing cleanup tasks
			// a chance to find out who was the last one to initiate the
			// cleanup.
			return fmt.Errorf("new cooldown start time for %q SDK volume: %v", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision), readyTime)
		}
	}

	if time.Since(cooldownStart) < sdkVolumeCooldownTime {
		remaining := sdkVolumeCooldownTime - time.Since(cooldownStart)
		return fmt.Errorf("cooldown period for %q SDK volume has not elapsed yet, time remaining: %s",
			sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision), remaining.Round(time.Second))
	}

	// Check if there are any tasks in progress that use the same SDK volume.
	// Remember, cleanup tasks are attempted on every Ensure call, which can be
	// either periodic or triggered by a new API request. If it's the latter,
	// there could be a chance that the volume will be used again.
	changes := st.Changes()
	blocking := []string{"launch", "refresh", "remove"}
	for _, change := range changes {
		// Other changes like "exec" do not affect SDK volumes, so we can skip them.
		if slices.Contains(blocking, change.Kind()) && !change.IsReady() {
			return &state.Retry{}
		}
	}

	// Delete volume ignores ErrVolumeNotFound.
	err = m.backend.DeleteVolume(ctx, sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
	if err == nil || errors.Is(err, workshop.ErrVolumeInUse) {
		if errors.Is(err, workshop.ErrVolumeInUse) {
			logger.Debugf("On SdkManager.Cleanup: the %q SDK volume is still in use, skip clean up", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
		} else {
			logger.Debugf("On SdkManager.Cleanup: the %q SDK volume was deleted", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
		}
		st.Cache(vk, nil)
		return nil
	}

	return err
}
