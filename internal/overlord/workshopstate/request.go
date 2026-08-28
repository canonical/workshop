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
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/canonical/workshop/internal/overlord/cmdstate"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	checkHealthTimeout = 5 * time.Second

	// mark the last task in a taskset after which refresh becomes irreversible (i.e. the following tasks
	// will not be possible to undo, e.g. removing an old workshop copy)
	EdgeRefreshLastTaskBeforeCleanup = state.TaskSetEdge("last-before-irreversible")

	// mark the tasks that denote irreversible clean up logic for refresh (e.g.
	// removing state storage and the old workshop copy)
	EdgeRefreshFirstCleanupTask = state.TaskSetEdge("refresh-cleanup")
)

func (w *WorkshopManager) LaunchMany(ctx context.Context, project workshop.Project, manifests []Manifest) ([]*state.TaskSet, error) {
	tasksets := make([]*state.TaskSet, 0, len(manifests))
	for _, manifest := range manifests {
		snapshot, err := w.bestSnapshot(ctx, manifest)
		if err != nil {
			return nil, err
		}

		tasks := launch(w.state, project, manifest, len(snapshot.Sdks))
		tasksets = append(tasksets, tasks)
	}
	return tasksets, nil
}

func retrieveBase(st *state.State, image workshop.BaseImage) *state.Task {
	return st.NewTask("download-base", fmt.Sprintf("Download %q base image", image.Name))
}

func retrieveSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	retrieve := state.NewTaskSet()
	for _, s := range sdks {
		if s.Source.NeedsRetrieve() {
			r := sdkstate.Retrieve(st, s)
			retrieve.AddTask(r)
		}
	}
	return retrieve
}

func installSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	all := state.NewTaskSet()

	var prev *state.Task
	addTask := func(t *state.Task) {
		all.AddTask(t)
		if prev != nil {
			t.WaitFor(prev)
		}
		prev = t
	}

	// Run setup-base after installing each SDK, rather than all at once.
	// This means each SDK snapshot only contains the relevant SDKs.
	for _, setup := range sdks {
		// The install task sets must not run concurrently as exec ops are not
		// allowed by LXD to be run concurrently and in general case we cannot
		// guarantee safety of concurrent installations.
		install := sdkstate.Install(st, setup.Name)
		addTask(install)

		hook := hookstate.Hook(st, setup.Name, 0, hookstate.SetupBase)
		addTask(hook)

		snapshot := sdkstate.Snapshot(st, setup.Name)
		addTask(snapshot)
	}
	return all
}

// Like installSdks but we skip setup-base and snapshot-sdk after restoring
// from a snapshot which already contains the setup-base modifications.
func reinstallSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	all := state.NewTaskSet()

	var prev *state.Task
	addTask := func(t *state.Task) {
		all.AddTask(t)
		if prev != nil {
			t.WaitFor(prev)
		}
		prev = t
	}

	for _, setup := range sdks {
		install := sdkstate.Install(st, setup.Name)
		addTask(install)
	}
	return all
}

func launchWorkshop(st *state.State, file *workshop.File, intact []sdk.Setup) *state.TaskSet {
	var lastIntact string
	var summary string
	if len(intact) == 0 {
		summary = fmt.Sprintf("Create new %q workshop", file.Name)
	} else {
		lastIntact = intact[len(intact)-1].Name
		summary = fmt.Sprintf("Create new %q workshop from %q snapshot", file.Name, lastIntact)
	}

	create := st.NewTask("create-workshop", summary)
	handlersetup.SetWorkshopFile(create, file)
	if len(intact) > 0 {
		create.Set("last-intact-sdk", lastIntact)
	}
	return state.NewTaskSet(create)
}

func rebuildWorkshop(st *state.State, file *workshop.File, intact []sdk.Setup) *state.TaskSet {
	var lastIntact string
	var summary string
	if len(intact) == 0 {
		summary = fmt.Sprintf("Rebuild %q workshop", file.Name)
	} else {
		lastIntact = intact[len(intact)-1].Name
		summary = fmt.Sprintf("Restore %q workshop from %q snapshot", file.Name, lastIntact)
	}

	create := st.NewTask("rebuild-workshop", summary)
	handlersetup.SetWorkshopFile(create, file)
	if len(intact) > 0 {
		create.Set("last-intact-sdk", lastIntact)
	}
	return state.NewTaskSet(create)
}

func startWorkshop(st *state.State, name string) *state.TaskSet {
	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
	return state.NewTaskSet(start)
}

func launch(st *state.State, project workshop.Project, manifest Manifest, intact int) *state.TaskSet {
	var prevInstall *state.TaskSet
	all := state.NewTaskSet()

	addTaskSet := func(ts *state.TaskSet) {
		if len(ts.Tasks()) == 0 {
			return
		}

		if prevInstall != nil {
			ts.WaitAll(prevInstall)
		}
		prevInstall = ts

		all.AddAll(ts)
	}

	intactSdks := manifest.Sdks[:intact]
	newSdks := manifest.Sdks[intact:]

	var base *state.Task
	if intact == 0 {
		// Create download-base first so the task IDs are in a nice order.
		base = retrieveBase(st, manifest.Image)
	}
	retrieve := retrieveSdks(st, manifest.Sdks)
	if base != nil {
		retrieve.AddTask(base)
	}
	addTaskSet(retrieve)

	createDirs := st.NewTask("create-workshop-storage", fmt.Sprintf("Create %q storage directories", manifest.File.Name))
	addTaskSet(state.NewTaskSet(createDirs))

	create := launchWorkshop(st, manifest.File, intactSdks)
	addTaskSet(create)

	// SDKs need to be mounted after restoring a snapshot.
	reinstall := reinstallSdks(st, intactSdks)
	addTaskSet(reinstall)

	start := startWorkshop(st, manifest.File.Name)
	addTaskSet(start)

	install := installSdks(st, newSdks)
	addTaskSet(install)

	configureTimezone := st.NewTask("configure-timezone", fmt.Sprintf("Configure %q workshop timezone", manifest.File.Name))
	addTaskSet(state.NewTaskSet(configureTimezone))

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, manifest.File.Name, manifest.Sdks)
	addTaskSet(connect)

	setupProject := runHooks(st, manifest.Sdks, 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	checkHealth := runHooks(st, manifest.Sdks, checkHealthTimeout, hookstate.CheckHealth)
	addTaskSet(checkHealth)

	for _, task := range all.Tasks() {
		task.Set("workshop", manifest.File.Name)
		task.Set("project", project)
	}

	return all
}

func (w *WorkshopManager) RefreshMany(ctx context.Context, project workshop.Project, current, latest []Manifest, option conflict.RefreshOption) ([]*state.TaskSet, error) {
	tasksets := make([]*state.TaskSet, 0, len(latest))

	for i := range latest {
		snapshot, err := w.bestSnapshot(ctx, latest[i])
		if err != nil {
			return nil, err
		}

		if option == conflict.RefreshRestore || hasUpdates(current[i], latest[i]) {
			tasks := refresh(w.state, project, current[i], latest[i], len(snapshot.Sdks), option)
			tasksets = append(tasksets, tasks)
		}
	}

	for _, ts := range tasksets {
		cleanup := ts.MaybeEdge(EdgeRefreshFirstCleanupTask)
		if cleanup == nil {
			continue
		}

		// We will iterate over other refreshes and make sure that the cleanup
		// task of our refresh will wait until all the other refresh operations
		// finished. This will ensure that we start to remove the workshops'
		// previous copies once all the refresh operations were successful (at
		// this stage, we only need to remove a stashed copy, the newly refreshed
		// workshop is already up and running). Thus, every CleanupEdge will
		// wait for ALL the LastBeforeRefreshIrreversibleEdge tasks of all the
		// other changes before execution.
		for _, otherts := range tasksets {
			if ts != otherts {
				last, err := otherts.Edge(EdgeRefreshLastTaskBeforeCleanup)
				if err != nil {
					return nil, err
				}
				cleanup.WaitFor(last)
			}
		}
	}

	return tasksets, nil
}

func (w *WorkshopManager) bestSnapshot(ctx context.Context, manifest Manifest) (*workshop.Snapshot, error) {
	snapshot := workshop.SdkSnapshot(manifest.Format, manifest.Image, manifest.Sdks)
	for range manifest.Sdks {
		if _, err := w.backend.Snapshot(ctx, snapshot); errors.Is(err, workshop.ErrSnapshotNotFound) {
			snapshot.Sdks = snapshot.Sdks[:len(snapshot.Sdks)-1]
		} else if err != nil {
			return nil, err
		} else {
			break
		}
	}
	return &snapshot, nil
}

func hasUpdates(current, latest Manifest) bool {
	currentSnapshot := workshop.SdkSnapshot(current.Format, current.Image, current.Sdks)
	finalSnapshot := workshop.SdkSnapshot(latest.Format, latest.Image, latest.Sdks)
	if !currentSnapshot.Equal(finalSnapshot) {
		return true
	}

	for _, sk := range finalSnapshot.Sdks {
		a := sdkAdditions(current.File.Sdks, sk.Name)
		b := sdkAdditions(latest.File.Sdks, sk.Name)
		if !reflect.DeepEqual(a, b) {
			return true
		}
	}

	if len(current.File.Connections) != len(latest.File.Connections) {
		return true
	}
	seen := make([]bool, len(latest.File.Connections))
out:
	for _, conn := range current.File.Connections {
		for i, other := range latest.File.Connections {
			if !seen[i] && conn == other {
				seen[i] = true
				continue out
			}
		}
		return true
	}

	return false
}

// Determine workshop-specific adjustments for the given SDK. Currently this is
// determined by plugs and slots.
func sdkAdditions(sdks []workshop.SdkRecord, name string) workshop.SdkRecord {
	idx := slices.IndexFunc(sdks, func(s workshop.SdkRecord) bool {
		return s.Name == name
	})
	if idx < 0 {
		// Likely system or sketch SDK.
		return workshop.SdkRecord{}
	}

	// It's better to refresh too often than to ignore the user's changes,
	// so return a partial record instead of extracting plugs and slots.
	sk := sdks[idx]
	sk.Name = ""
	sk.Channel = ""
	sk.Source = sdk.StoreSource
	return sk
}

func refresh(st *state.State, project workshop.Project, current, latest Manifest, intact int, option conflict.RefreshOption) *state.TaskSet {
	refresh := state.NewTaskSet()
	prev := (*state.TaskSet)(nil)
	addTaskSet := func(ts *state.TaskSet) {
		if len(ts.Tasks()) == 0 {
			return
		}

		if prev != nil {
			ts.WaitAll(prev)
		}
		refresh.AddAll(ts)
		prev = ts
	}

	saveSdks, restoreSdks := saveRestore(current.Sdks, latest.Sdks)

	currentSdks := slices.Clone(current.Sdks)
	slices.Reverse(currentSdks)

	intactSdks := latest.Sdks[:intact]
	newSdks := latest.Sdks[intact:]

	var base *state.Task
	if intact == 0 {
		// Create download-base first so the task IDs are in a nice order.
		base = retrieveBase(st, latest.Image)
	}
	// Retrieve all SDKs just in case the snapshot outlived some of them.
	retrieve := retrieveSdks(st, latest.Sdks)
	if base != nil {
		retrieve.AddTask(base)
	}
	addTaskSet(retrieve)

	if len(saveSdks) > 0 {
		stateStorage := st.NewTask("create-state-storage", "Create SDK state storage")
		addTaskSet(state.NewTaskSet(stateStorage))
	}

	// Call save-state hooks for the SDKs that are installed and will not be
	// removed after this refresh.
	saveState := runHooks(st, saveSdks, 0, hookstate.SaveState)
	addTaskSet(saveState)

	disconnect := disconnectSdks(st, currentSdks, option == conflict.RefreshRestore)
	addTaskSet(disconnect)

	discard := st.NewTask("discard-conns", fmt.Sprintf("Discard %q undesired connections", latest.File.Name))
	addTaskSet(state.NewTaskSet(discard))

	stop := st.NewTask("stop-workshop", fmt.Sprintf("Stop %q workshop", latest.File.Name))
	// Using force is fine for containers, but for VMs it can lead to (usually
	// repairable) filesystem integrity issues, which are copied to the stash.
	stop.Set("force", current.File.Confinement == workshop.ConfinementContainer)
	addTaskSet(state.NewTaskSet(stop))

	// Unmount SDKs and remove plugs and slots from interfaces repository.
	// In normal circumstances we only need to update the repository. The
	// SDKs will be implicitly unmounted when rebuilding the workshop. But
	// the repository is lost when the daemon restarts. It's easier to
	// reconstruct if we keep it in sync with the installed SDKs.
	uninstall := uninstallSdks(st, currentSdks)
	addTaskSet(uninstall)

	stash := st.NewTask("stash-workshop", fmt.Sprintf("Stash previous %q workshop", latest.File.Name))
	addTaskSet(state.NewTaskSet(stash))

	rebuild := rebuildWorkshop(st, latest.File, intactSdks)
	addTaskSet(rebuild)

	// Reinstall intact SDKs. The workshop definition can change plugs and
	// slots, and SDKs need to be mounted after restoring a snapshot.
	reinstall := reinstallSdks(st, intactSdks)
	addTaskSet(reinstall)

	start := startWorkshop(st, latest.File.Name)
	addTaskSet(start)

	// Install updated SDKs to the rebuilt workshop.
	install := installSdks(st, newSdks)
	addTaskSet(install)

	if option == conflict.RefreshUpdate {
		restoreConns := st.NewTask("restore-conns", fmt.Sprintf("Restore %q undesired connections", latest.File.Name))
		restoreConns.Set("discard-conns-task", discard.ID())
		addTaskSet(state.NewTaskSet(restoreConns))
	}

	configureTimezone := st.NewTask("configure-timezone", fmt.Sprintf("Configure %q workshop timezone", latest.File.Name))
	addTaskSet(state.NewTaskSet(configureTimezone))

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, latest.File.Name, latest.Sdks)
	addTaskSet(connect)

	setupProject := runHooks(st, latest.Sdks, 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	restoreState := runHooks(st, restoreSdks, 0, hookstate.RestoreState)
	addTaskSet(restoreState)

	checkHealth := runHooks(st, latest.Sdks, 0, hookstate.CheckHealth)
	addTaskSet(checkHealth)

	length := len(refresh.Tasks())
	last := refresh.Tasks()[length-1]
	refresh.MarkEdge(last, EdgeRefreshLastTaskBeforeCleanup)

	cleanupLane := st.NewLane()

	if len(restoreSdks) > 0 {
		removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
		addTaskSet(state.NewTaskSet(removeStateStorage))
		removeStateStorage.JoinLane(cleanupLane)
		refresh.MarkEdge(removeStateStorage, EdgeRefreshFirstCleanupTask)
	}

	// remove the workshop from stash after the state storage was detached
	removeStash := st.NewTask("remove-workshop-stash", fmt.Sprintf("Remove %q workshop from stash", latest.File.Name))
	// if the change was aborted during the cleanup stage execution,
	// there is a chance that some of the workshop copies that had
	// been created during the refresh were already deleted. If we
	// start to Undo those workshops' refresh progress we will
	// endup deleting the workshops that finished their refresh.
	// Given that they have no copy already, the undo logic
	// (stash-workshop) will delete the existing workshop
	// and fail to restore from the copy. We don't want that. Hence,
	// all the cleanup tasks are extracted into a separate lane. If
	// any problem happens, the workshops that had finished their
	// refresh will not be affected.
	addTaskSet(state.NewTaskSet(removeStash))
	removeStash.JoinLane(cleanupLane)
	if refresh.MaybeEdge(EdgeRefreshFirstCleanupTask) == nil {
		refresh.MarkEdge(removeStash, EdgeRefreshFirstCleanupTask)
	}

	for _, task := range refresh.Tasks() {
		task.Set("workshop", latest.File.Name)
		task.Set("project", project)
	}

	return refresh
}

// List SDKs which belong to both slices (regardless of version), sorted by
// installation order in both cases.
func saveRestore(current, latest []sdk.Setup) (save, restore []sdk.Setup) {
	var saveIdxs []int
	var saveSdks, restoreSdks []sdk.Setup
	for _, sk := range latest {
		idx := slices.IndexFunc(current, func(s sdk.Setup) bool {
			return s.Name == sk.Name
		})
		if idx >= 0 {
			saveIdxs = append(saveIdxs, idx)
			restoreSdks = append(restoreSdks, sk)
		}
	}
	slices.Sort(saveIdxs)
	for _, i := range saveIdxs {
		saveSdks = append(saveSdks, current[i])
	}
	return saveSdks, restoreSdks
}

func autoconnectSdks(st *state.State, w string, sdks []sdk.Setup) *state.TaskSet {
	autoconnectSet := state.NewTaskSet()

	validate := st.NewTask("resolve-interfaces", fmt.Sprintf("Resolve relations between interfaces of %q workshop", w))
	autoconnectSet.AddTask(validate)

	prev := validate
	for _, setup := range sdks {
		autoconnect := st.NewTask("auto-connect", fmt.Sprintf("Auto-connect interfaces of %q SDK", setup.Name))
		autoconnect.Set("sdk", setup.Name)
		autoconnectSet.AddTask(autoconnect)
		autoconnect.WaitFor(prev)
		prev = autoconnect
	}
	return autoconnectSet
}

func uninstallSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	prev := (*state.Task)(nil)
	uninstallSet := state.NewTaskSet()
	for _, s := range sdks {
		uninstall := sdkstate.Uninstall(st, s)
		uninstallSet.AddTask(uninstall)

		if prev != nil {
			uninstall.WaitFor(prev)
		}
		prev = uninstall
	}
	return uninstallSet
}

func disconnectSdks(st *state.State, sdks []sdk.Setup, forget bool) *state.TaskSet {
	prev := (*state.Task)(nil)
	disconnSet := state.NewTaskSet()
	for _, s := range sdks {
		disconn := st.NewTask("auto-disconnect", fmt.Sprintf("Disconnect interfaces of %q SDK", s.Name))
		disconn.Set("sdk", s.Name)
		disconn.Set("forget", forget)
		disconnSet.AddTask(disconn)

		if prev != nil {
			disconn.WaitFor(prev)
		}
		prev = disconn
	}
	return disconnSet
}

func runHooks(st *state.State, installed []sdk.Setup, timeout time.Duration, hooktype hookstate.WorkshopHookType) *state.TaskSet {
	hooks := state.NewTaskSet()
	prev := (*state.Task)(nil)
	for _, sk := range installed {
		hook := hookstate.Hook(st, sk.Name, timeout, hooktype)
		hooks.AddTask(hook)
		if prev != nil {
			hook.WaitFor(prev)
		}
		prev = hook
	}
	return hooks
}

func (w *WorkshopManager) StartMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	var allowedHealthStatus = []healthstate.Status{
		healthstate.StoppedStatus,
	}

	for _, name := range names {
		wp, err := w.Workshop(ctx, name, projectId)
		if err != nil {
			return nil, fmt.Errorf("cannot start %q: %w", name, err)
		}

		if err = healthstate.CheckWorkshopHealth(w.state, wp, allowedHealthStatus); err != nil {
			return nil, fmt.Errorf("cannot start %q: %w", name, err)
		}
	}

	project, err := w.Project(ctx, projectId)
	if err != nil {
		return nil, err
	}

	return startMany(w.state, names, project), nil
}

func startMany(st *state.State, names []string, project workshop.Project) []*state.TaskSet {
	taskset := []*state.TaskSet{}

	for _, name := range names {
		start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
		start.Set("workshop", name)
		start.Set("project", project)

		taskset = append(taskset, state.NewTaskSet(start))
	}

	return taskset
}

func (w *WorkshopManager) StopMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	var allowedHealthStatus = []healthstate.Status{
		healthstate.ReadyStatus,
		healthstate.StoppedStatus,
	}

	for _, name := range names {
		wp, err := w.Workshop(ctx, name, projectId)
		if err != nil {
			return nil, fmt.Errorf("cannot stop %q: %w", name, err)
		}

		if err = healthstate.CheckWorkshopHealth(w.state, wp, allowedHealthStatus); err != nil {
			return nil, fmt.Errorf("cannot stop %q: %w", name, err)
		}
	}

	project, err := w.Project(ctx, projectId)
	if err != nil {
		return nil, err
	}

	return stopMany(w.state, names, project), nil
}

func stopMany(st *state.State, names []string, project workshop.Project) []*state.TaskSet {
	taskset := []*state.TaskSet{}

	for _, name := range names {
		stop := st.NewTask("stop-workshop", fmt.Sprintf("Stop %q workshop", name))
		stop.Set("force", false)
		stop.Set("workshop", name)
		stop.Set("project", project)

		taskset = append(taskset, state.NewTaskSet(stop))
	}

	return taskset
}

func (w *WorkshopManager) Exec(ctx context.Context, name, projectId string, args *workshop.ExecArgs, action bool) (*state.TaskSet, error) {
	project, err := w.Project(ctx, projectId)
	if err != nil {
		return nil, err
	}

	wp, err := w.Workshop(ctx, name, projectId)
	if err != nil {
		return nil, err
	}
	allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.WaitingStatus}
	if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)
	wrkspc, err := w.backend.WorkshopFs(ctx, name)
	if err != nil {
		return nil, err
	}
	defer wrkspc.Close()

	info, err := wrkspc.Stat(args.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("working directory %q not found", args.WorkDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", args.WorkDir)
	}

	var execSet *state.TaskSet
	if action {
		name := args.Command[0]
		cp := w.state.NewTask("install-action", fmt.Sprintf("Install action %q", name))
		exec := w.state.NewTask("exec", fmt.Sprintf("Exec action %q", name))

		// install-action will modify args and pass it to exec.
		w.state.Cache(cmdstate.ExecArgsKey(cp.ID()), args)
		cp.Set("exec-task", exec.ID())

		exec.WaitFor(cp)
		execSet = state.NewTaskSet(cp, exec)
	} else {
		exec := w.state.NewTask("exec", fmt.Sprintf("Exec command %q", args.Command[0]))

		w.state.Cache(cmdstate.ExecArgsKey(exec.ID()), args)

		execSet = state.NewTaskSet(exec)
	}

	for _, task := range execSet.Tasks() {
		task.Set("workshop", name)
		task.Set("project", project)
	}
	return execSet, nil
}

func (w *WorkshopManager) RemoveMany(ctx context.Context, project workshop.Project, stashed, current []Manifest, running map[string]bool) ([]*state.TaskSet, error) {
	hasStash := make(map[string]bool, len(stashed))
	for _, s := range stashed {
		hasStash[s.File.Name] = true
	}

	taskset := []*state.TaskSet{}
	for _, m := range current {
		remove := remove(w.state, m, hasStash[m.File.Name], running[m.File.Name], project)
		taskset = append(taskset, remove)
	}
	return taskset, nil
}

func remove(st *state.State, manifest Manifest, hasStash, running bool, project workshop.Project) *state.TaskSet {
	removeSet := state.NewTaskSet()
	var prevRemove *state.TaskSet
	addTaskSet := func(ts *state.TaskSet) {
		if len(ts.Tasks()) == 0 {
			return
		}
		if prevRemove != nil {
			ts.WaitAll(prevRemove)
		}
		prevRemove = ts
		removeSet.AddAll(ts)
	}

	sdks := slices.Clone(manifest.Sdks)
	slices.Reverse(sdks)

	disconnectSet := disconnectSdks(st, sdks, true)
	addTaskSet(disconnectSet)

	discard := st.NewTask("discard-conns", fmt.Sprintf("Discard %q undesired connections", manifest.File.Name))
	addTaskSet(state.NewTaskSet(discard))

	if running {
		// It's safe to stop if the workshop isn't running, but we
		// don't want to start it if the Change is undone.
		stop := st.NewTask("stop-workshop", fmt.Sprintf("Stop %q workshop", manifest.File.Name))
		stop.Set("force", true)
		addTaskSet(state.NewTaskSet(stop))
	}

	uninstall := uninstallSdks(st, sdks)
	addTaskSet(uninstall)

	remove := st.NewTask("remove-workshop", fmt.Sprintf("Remove %q workshop", manifest.File.Name))
	addTaskSet(state.NewTaskSet(remove))

	// The point of no return starts after the workshop is removed. If any of the tasks
	// after this fails, we can only report the error, but cannot undo the removal.
	removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
	addTaskSet(state.NewTaskSet(removeStateStorage))

	var removeStash *state.Task
	if hasStash {
		removeStash = st.NewTask("remove-workshop-stash", fmt.Sprintf("Remove %q workshop from stash", manifest.File.Name))
		addTaskSet(state.NewTaskSet(removeStash))
	}

	removeDirs := st.NewTask("remove-workshop-storage", fmt.Sprintf("Remove %q storage directories", manifest.File.Name))
	addTaskSet(state.NewTaskSet(removeDirs))

	// Directories should exist from before create-workshop until after remove-workshop.
	// Since there is no way to undo remove-workshop, we run remove-workshop-storage in a separate lane.
	// If an error occurs when removing the directories, it will not affect the other tasks.
	cleanupLane := st.NewLane()
	removeDirs.JoinLane(cleanupLane)
	if removeStash != nil {
		removeStash.JoinLane(cleanupLane)
	}
	removeStateStorage.JoinLane(cleanupLane)

	for _, task := range removeSet.Tasks() {
		task.Set("workshop", manifest.File.Name)
		task.Set("project", project)
	}
	return removeSet
}

func (w *WorkshopManager) Remount(ctx context.Context, st *state.State, plug sdk.PlugRef, source string) (*state.TaskSet, error) {
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("cannot remount: the `source` path must be absolute")
	}

	source = filepath.Clean(source)

	project, err := w.Project(ctx, plug.ProjectId)
	if err != nil {
		return nil, err
	}

	wp, err := w.Workshop(ctx, plug.Workshop, plug.ProjectId)
	if err != nil {
		return nil, fmt.Errorf("cannot load workshop %q: %w", plug.Workshop, err)
	}

	allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.StoppedStatus}
	if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
		return nil, fmt.Errorf("cannot remount %q: %w", plug.ShortRef(), err)
	}

	master, _ := ifacestate.MaybeBound(wp, plug)

	remount := st.NewTask("remount", fmt.Sprintf(`Remount %q`, plug.ShortRef()))
	remount.Set("workshop", plug.Workshop)
	remount.Set("project", project)
	remount.Set("plug", master)
	remount.Set("host-source", source)

	return state.NewTaskSet(remount), nil
}
