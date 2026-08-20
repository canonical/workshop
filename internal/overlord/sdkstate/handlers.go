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

package sdkstate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/interfaces/policy"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/sdkstore"
	"github.com/canonical/workshop/internal/workshop"
)

func SdkSetup(task *state.Task, w string, age Age) (sdk.Setup, error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var sk string
	if err := task.Get("sdk", &sk); err != nil {
		return sdk.Setup{}, err
	}

	setups, err := WorkshopSdks(task.Change(), w, age)
	if err != nil {
		return sdk.Setup{}, err
	}

	idx := slices.IndexFunc(setups, func(s sdk.Setup) bool {
		return s.Name == sk
	})
	if idx < 0 {
		return sdk.Setup{}, fmt.Errorf("internal error: %q workshop has no %q SDK", w, sk)
	}
	return setups[idx], nil
}

func sdkName(task *state.Task) (string, error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var sk string
	if err := task.Get("sdk", &sk); err != nil {
		return "", err
	}
	return sk, nil
}

func (m *SdkManager) doRetrieveSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	rec, err := SdkSetup(task, w, NewWorkshop)
	if err != nil {
		return err
	}

	st := task.State()
	st.Lock()
	base, err := WorkshopBase(task.Change(), w, NewWorkshop)
	st.Unlock()
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	fl, err := sdk.OpenLock(rec.Name)
	if err != nil {
		return err
	}
	defer fl.Close()
	if err := fl.Lock(); err != nil {
		return err
	}

	if _, err = m.backend.Sdk(ctx, rec); err == nil {
		logger.Debugf("On doRetrieveSdk: reuse existing SDK volume %q", sdk.VolumeName(rec.Name, rec.Revision))
		return nil
	}

	if err := m.retrieveSdk(ctx, task, rec); err != nil {
		return err
	}

	sdkYaml, err := extractSdkYAML(ctx, task, rec)
	if err != nil {
		return err
	}
	if err := workshop.ValidateSdkInfo(project.ProjectId, w, base.Name, rec.Name, sdkYaml); err != nil {
		return err
	}
	meta := sdk.Meta{Setup: rec, SdkYAML: sdkYaml}

	reader, err := os.Open(rec.Filepath())
	if err != nil {
		return err
	}
	defer reader.Close()

	err = m.backend.ImportSdk(ctx, meta, reader)
	if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
		logger.Debugf("On doRetrieveSdk: reuse existing SDK volume %q", sdk.VolumeName(meta.Name, meta.Revision))
		return nil
	}
	if err != nil {
		return err
	}

	// If the SDK was downloaded successfully, remove its previous rev if any.
	if err := cleanupSdk(rec); err != nil {
		logger.Noticef("On doRetrieveSdk: cannot cleanup previous download: %v", err)
	}
	return nil
}

func (m *SdkManager) retrieveSdk(ctx context.Context, task *state.Task, rec sdk.Setup) error {
	path := rec.Filepath()
	if osutil.FileExists(path) {
		logger.Debugf("On doRetrieveSdk: %q SDK found locally: %s", rec.Name, path)
		return nil
	}

	rev := revert.New()
	defer rev.Fail()

	writer, err := osutil.NewAtomicFile(path, 0666, 0, osutil.NoChown, osutil.NoChown)
	if err != nil {
		return err
	}
	rev.Add(func() {
		if reverr := writer.Cancel(); reverr != nil {
			logger.Noticef("On doRetrieveSdk: %v", reverr)
		}
	})

	archive := sdkstore.SdkArchive{
		Name:      rec.Name,
		PackageID: rec.PackageID,
		Revision:  rec.Revision.N,
		Sha3_384:  rec.Sha3_384,
	}
	st := task.State()
	reporter := &progress.Reporter{
		Name: task.ID(),
		Report: func(label string, done, total int64) {
			st.Lock()
			task.SetProgress(label, done, total)
			st.Unlock()
		},
	}

	if rec.Source == sdk.SystemSource {
		err = system.RetrieveSystemSdk(writer.File, rec, reporter)
	} else {
		err = m.store.Download(ctx, writer, archive, sdkstore.WithReporter(reporter))
	}
	if err != nil {
		return err
	}

	if err := writer.Commit(); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func extractSdkYAML(ctx context.Context, task *state.Task, rec sdk.Setup) (string, error) {
	path := rec.Filepath()
	cache := path + ".yaml"

	content, err := os.ReadFile(cache)
	if err == nil {
		return string(content), nil
	}

	cmd := exec.CommandContext(ctx, "tar",
		"--extract",
		"--to-stdout",
		"--force-local",
		"--file="+path,
		"meta/sdk.yaml",
	)
	content, err = cmd.Output()
	if err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if ok && len(exitErr.Stderr) > 0 {
			st := task.State()
			st.Lock()
			task.Logf("%s", exitErr.Stderr)
			st.Unlock()
		}
		return "", err
	}

	if err := osutil.AtomicWriteFile(cache, content, 0666, 0); err != nil {
		return "", err
	}
	return string(content), nil
}

func cleanupSdk(rec sdk.Setup) error {
	target := rec.Filepath()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), rec.Name+"_*.sdk"))
	if err != nil {
		return err
	}

	for _, m := range matches {
		if m == target {
			continue
		}
		if err1 := os.Remove(m + ".yaml"); err1 != nil && !errors.Is(err1, os.ErrNotExist) {
			err = cmp.Or(err, err1)
		}
		if err1 := os.Remove(m); err1 != nil {
			err = cmp.Or(err, err1)
		}
	}

	return err
}

func (m *SdkManager) doInstallSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	age := NewWorkshop
	if task.Kind() == "uninstall-sdk" {
		age = OldWorkshop
	}
	sdkSetup, err := SdkSetup(task, w, age)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	rev := revert.New()
	defer rev.Fail()

	if err := m.backend.InstallSdk(ctx, w, sdkSetup); err != nil {
		return err
	}
	cleanupCtx := context.WithoutCancel(ctx)
	rev.Add(func() {
		cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
		defer cancel()

		if reverr := m.backend.UninstallSdk(cleanupCtx, w, sdkSetup.Name); reverr != nil {
			logger.Noticef("On doInstallSdk: cannot uninstall %q SDK on cleanup: %v", sdkSetup.Name, reverr)
		}
	})

	// add SDK's plugs and slots
	if err := m.registerSdk(ctx, w, sdkSetup.Name); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *SdkManager) doUninstallSdk(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	age := OldWorkshop
	if task.Kind() == "install-sdk" {
		age = NewWorkshop
	}
	sdkSetup, err := SdkSetup(task, w, age)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	rev := revert.New()
	defer rev.Fail()

	if err := m.repo.RemoveSdk(project.ProjectId, w, sdkSetup.Name); err != nil {
		return err
	}
	cleanupCtx := context.WithoutCancel(ctx)
	rev.Add(func() {
		cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
		defer cancel()

		if reverr := m.registerSdk(cleanupCtx, w, sdkSetup.Name); reverr != nil {
			logger.Noticef("On doUninstallSdk: cannot re-register %q SDK on cleanup: %v", sdkSetup.Name, reverr)
		}
	})

	if sdkSetup.IsVolume() && sdkSetup.Source != sdk.SystemSource {
		// Record when the volume was last used in this change. The cleanup logic
		// won't remove an SDK for a fixed window of time after this point.
		st := task.State()
		st.Lock()
		SetSdkVolumeLastUsed(task.Change(), sdkSetup, task.ID(), timeNow())
		st.Unlock()
	}

	if err := m.backend.UninstallSdk(ctx, w, sdkSetup.Name); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (m *SdkManager) registerSdk(ctx context.Context, w, sk string) error {
	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	info, err := wp.SdkInfo(ctx, sk)
	if err != nil {
		return err
	}

	if len(info.BadInterfaces) > 0 {
		return fmt.Errorf("%s", sdk.BadInterfacesSummary(info))
	}

	if err = policy.CheckInterfaces(info); err != nil {
		return err
	}

	return m.repo.AddSdk(info)
}

func (m *SdkManager) doSnapshotSdk(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	st.Lock()
	change := task.Change()
	attempt, err := conflict.ChangeAttempt(change)
	st.Unlock()
	if err != nil {
		return err
	}
	if attempt > 1 {
		st.Lock()
		task.Logf("Skipping snapshot after %s was resumed", change.Kind())
		st.Unlock()
		return nil
	}

	user, project, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	sk, err := sdkName(task)
	if err != nil {
		return err
	}

	st.Lock()
	snapshot, err := WorkshopSdkSnapshot(change, w, sk)
	st.Unlock()
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	if err := m.backend.TakeSnapshot(ctx, w, *snapshot); err != nil && !errors.Is(err, workshop.ErrSnapshotAlreadyExists) {
		return err
	}

	return nil
}

// doDeleteUnusedSdkVolumes is a cleanup handler that deletes unused SDK volumes.
// It is called periodically to ensure that SDK volumes that are no longer
// needed are removed from the system. The cleanup is done only if there are no
// tasks in progress that use the same SDK volume. The cleanup is also
// throttled to avoid deleting volumes too frequently. The cooldown time is
// defined by the sdkVolumeCooldownTime constant.
//
// There are multiple scenarios when this cleanup is triggered:
//   - A workshop was removed and an SDK volume was detached from it (uninstall-sdk task)
//   - An SDK volume was detached during a refresh change,
//     and is no longer used by any other workshop (uninstall-sdk or, if refresh failed, install-sdk task)
//   - A workshop launch failed (install-sdk task)
func (m *SdkManager) doDeleteUnusedSdkVolumes(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		// Log as an internal error, no need to retry again.
		logger.Noticef("On SdkManager.Cleanup: internal error: %v", err)
		return nil
	}

	age := OldWorkshop
	if task.Kind() == "install-sdk" {
		age = NewWorkshop
	}
	sdkSetup, err := SdkSetup(task, w, age)
	if err != nil {
		logger.Noticef("On SdkManager.Cleanup: the %q task is not associated with a SDK setup", task.ID())
		return nil
	}

	// Either no volume to clean up, or it's the current system SDK, which we
	// might as well keep as an optimization.
	if !sdkSetup.IsVolume() || (sdkSetup.Source == sdk.SystemSource && sdkSetup.Revision == system.SystemSdkRevision) {
		return nil
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	defer st.Unlock()

	// Check the last Task in this Change to uninstall the SDK. If there isn't
	// one, there's no need to clean up. If there's a later Task, we let it
	// handle the cleanup instead.
	taskID, lastUsed, err := SdkVolumeLastUsed(task.Change(), sdkSetup)
	if err != nil || taskID != task.ID() {
		if err != nil && !errors.Is(err, state.ErrNoState) {
			logger.Noticef("On SdkManager.Cleanup: internal error: %v", err)
		}
		return nil
	}

	// Check for changes that might use the same SDK volume. If one is in
	// progress and retrieve-sdk is done already, removing the SDK will break
	// the install-sdk task. If another task uninstalled the SDK after us, we
	// can let it handle the cleanup.
	changes := st.Changes()
	blocking := []string{"launch", "refresh", "remove"}
	for _, change := range changes {
		// Other changes like "exec" do not affect SDK volumes, so we can skip them.
		if !slices.Contains(blocking, change.Kind()) {
			continue
		}
		if !change.IsReady() {
			return &state.Retry{}
		}

		taskID, time, err := SdkVolumeLastUsed(change, sdkSetup)
		if errors.Is(err, state.ErrNoState) {
			continue
		}
		if err != nil {
			logger.Noticef("On SdkManager.Cleanup: internal error: %v", err)
			return nil
		}

		// Just compare task IDs lexicographically; it doesn't really matter
		// which one handles the cleanup as long as there's consensus.
		if time.After(lastUsed) || (time.Equal(lastUsed) && taskID > task.ID()) {
			return nil
		}
	}

	if timeNow().Sub(lastUsed) < sdkVolumeCooldownTime {
		return &state.Retry{}
	}

	// Delete volume ignores ErrVolumeNotFound.
	err = m.backend.DeleteSdk(ctx, sdkSetup)
	if err == nil || errors.Is(err, workshop.ErrVolumeInUse) {
		if errors.Is(err, workshop.ErrVolumeInUse) {
			logger.Debugf("On SdkManager.Cleanup: the %q SDK volume is still in use, skip clean up", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
		} else {
			logger.Debugf("On SdkManager.Cleanup: the %q SDK volume was deleted", sdk.VolumeName(sdkSetup.Name, sdkSetup.Revision))
		}
		return nil
	}

	return err
}
