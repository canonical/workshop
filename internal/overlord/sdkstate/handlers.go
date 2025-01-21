package sdkstate

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces/policy"
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

	err := task.Get("sdk-retrieve-task", &retrieveId)

	if err != nil {
		return sdk.Setup{}, err
	}

	retrieve := task.State().Task(retrieveId)
	if retrieve == nil {
		return sdk.Setup{}, fmt.Errorf("internal error: no corresponding retrieve-sdk task found")
	}

	if err = retrieve.Get("sdk-setup", &sdkSetup); err != nil {
		return sdk.Setup{}, err
	}
	return sdkSetup, nil
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

	st.Lock()
	store := sdk.StoreService(st)
	st.Unlock()

	reporter := &progress.Reporter{
		Name: task.ID(),
		Report: func(label string, done, total int) {
			st.Lock()
			task.SetProgress(label, done, total)
			st.Unlock()
		},
	}

	if err = store.DownloadSdk(ctx, rec, reporter); err != nil {
		return err
	}

	if err = m.maybeCreateVolume(ctx, rec); err != nil {
		return err
	}

	return nil
}

func (m *SdkManager) doInstallLocalSdk(task *state.Task, tomb *tomb.Tomb) error {
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

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	switch sdkSetup.Name {
	case sdk.System.String():
		return wp.InstallLocalSdk(ctx, sdkSetup.Name, sdkSetup.Revision.String(), system.SystemSdkFs)
	case sdk.Sketch:
		usr, err := workshop.LookupUsername(user)
		if err != nil {
			return err
		}
		sketchdir := sdk.WorkshopSketchSdkCurrent(usr.HomeDir, project.ProjectId, w)
		return wp.InstallLocalSdk(ctx, sdkSetup.Name, sdkSetup.Revision.String(), os.DirFS(sketchdir))
	default:
		return fmt.Errorf("unknown type of the local SDK")
	}
}

func (m *SdkManager) undoInstallLocalSdk(task *state.Task, tomb *tomb.Tomb) error {
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

	wfs, err := m.backend.WorkshopFs(ctx, w)
	if err != nil {
		return err
	}
	defer wfs.Close()

	return wfs.RemoveAll(sdk.SdkRevPath(sdkSetup.Name, sdkSetup.Revision.String()))
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

	// Directory: /var/lib/workshop/sdk/<name>/<revision>/
	fs, err := m.backend.WorkshopFs(ctx, w)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err = fs.MkdirAll(dirs.WorkshopSdksDir, 0755); err != nil {
		return err
	}

	// Mount the SDK content at the workshop location.
	sdkPath := filepath.Join(dirs.WorkshopSdksDir, sdkSetup.Name, sdkSetup.Revision.String())

	return m.backend.AttachVolume(ctx, w, sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision.String()), sdkPath)
}

func (m *SdkManager) undoInstallSdk(task *state.Task, tomb *tomb.Tomb) error {
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

	return m.backend.DetachVolume(ctx, w, sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision.String()))
}

func (m *SdkManager) doLinkSdk(task *state.Task, tomb *tomb.Tomb) error {
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

	if err = wp.LinkSdk(ctx, setup); err != nil {
		return err
	}

	rev := revert.New()
	defer rev.Fail()

	st := task.State()
	rev.Add(func() {
		if err := wp.UnlinkSdk(ctx, setup.Name); err != nil {
			st.Lock()
			task.Logf("Link SDK cleanup: could not unlink %q SDK: %v", setup.Name, err)
			st.Unlock()
		}
	})

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
	if err := m.repo.AddSdk(info); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *SdkManager) undoLinkSdk(task *state.Task, tomb *tomb.Tomb) error {
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

	if err := m.repo.RemoveSdk(project.ProjectId, w, sdkSetup.Name); err != nil {
		return err
	}

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	return wp.UnlinkSdk(ctx, sdkSetup.Name)
}
