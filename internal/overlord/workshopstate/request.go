package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/cmdstate"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	// mark the last task in a taskset after which refresh becomes irreversible (i.e. the following tasks
	// will not be possible to undo, e.g. removing an old workshop copy)
	EdgeLastTaskBeforeRefreshIrreversible = state.TaskSetEdge("last-before-irreversible")

	// mark the tasks that denote irreversible clean up logic for refresh (e.g.
	// removing state storage and the old workshop copy)
	EdgeRefreshCleanup = state.TaskSetEdge("refresh-cleanup")
)

var checkHealthTimeout = 5 * time.Second

func (w *WorkshopManager) loadProject(ctx context.Context, id string) (*workshop.Project, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key user not found")
	}

	projects, err := w.backend.Projects(ctx)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(projects[username], func(p workshop.Project) bool { return p.ProjectId == id })
	if idx == -1 {
		return nil, fmt.Errorf("no project found with \"id\" %v", id)
	}
	return &projects[username][idx], nil
}

func maybeSketch(ctx context.Context, pid, wp string) (bool, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return false, fmt.Errorf("context key user not found")
	}

	usr, err := workshop.LookupUsername(username)
	if err != nil {
		return false, err
	}
	sketchdir := sdk.WorkshopSketchSdkCurrent(usr.HomeDir, pid, wp)

	recs, err := os.ReadDir(sketchdir)
	// no Sketch SDK exists for the workshop and it is not an error.
	if len(recs) == 0 || osutil.IsDirNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (w *WorkshopManager) LaunchMany(ctx context.Context, names []string, projectId string, opChangeId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	taskset := make([]*state.TaskSet, 0, len(names))
	var sdks []sdk.SdkResult
	for _, name := range names {
		// Make sure the workshop doesn't exist
		_, err := w.Workshop(ctx, name, projectId)
		if err == nil {
			return nil, fmt.Errorf("cannot launch %q: workshop exists", name)
		} else if !errors.Is(err, workshop.ErrWorkshopNotLaunched) {
			return nil, fmt.Errorf("cannot launch %q, failed to check whether the workshop exists: %w", name, err)
		}

		file, err := project.Workshop(name)
		if err != nil {
			return nil, fmt.Errorf("cannot launch %q: %w", name, err)
		}

		sdks, err = launchStoreInfo(w.state, ctx, projectId, file)
		if err != nil {
			return nil, err
		}

		sets := []sdk.Setup{}
		for _, s := range sdks {
			sets = append(sets, sdk.Setup{Name: s.Name, Channel: s.Channel, Revision: s.Revision})
		}

		tasks := launch(w.state, file, sets, *project)
		taskset = append(taskset, tasks)
	}
	return taskset, nil
}

func launchStoreInfo(st *state.State, ctx context.Context, projectid string, file *workshop.File) ([]sdk.SdkResult, error) {
	sto := sdk.StoreService(st)
	acts := []sdk.SdkAction{}
	for _, sd := range file.Sdks {
		// "system" SDK is bootstrapped and installed by Workshop locally in a
		// separate task.
		if sd.Name == sdk.System.String() {
			continue
		}
		act := sdk.SdkAction{ProjectId: projectid, Workshop: file.Name, Name: sd.Name, Channel: sd.Channel, Action: sdk.Install}
		acts = append(acts, act)
	}
	res, err := sto.SdkAction(ctx, acts)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func retrieveSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	retrieve := state.NewTaskSet()
	for _, s := range sdks {
		if s.Channel != "" {
			r := sdkstate.Retrieve(st, s)
			retrieve.AddTask(r)
		}
	}
	return retrieve
}

func installSdks(st *state.State, w string, sdks []sdk.Setup, retrieveSet *state.TaskSet) *state.TaskSet {
	var prevInstall = sdkstate.InstallSystemSdk(st)
	install := state.NewTaskSet(prevInstall.Tasks()...)

	var prevSetup *state.Task
	setupHook := state.NewTaskSet()

	var prevAuto = st.NewTask("auto-connect", fmt.Sprintf(`Auto-connect interfaces of %q SDK`, sdk.System.String()))
	prevAuto.Set("sdk", sdk.System.String())
	autoConnect := state.NewTaskSet(prevAuto)

	for idx, setup := range sdks {
		// The install task sets must not run concurrently as exec ops are not
		// allowed by LXD to be run concurrently and in general case we cannot
		// guarantee safety of concurrent installations.
		var installTs *state.TaskSet
		if setup.Channel != "" {
			installTs = sdkstate.Install(st, setup.Name, retrieveSet.Tasks()[idx].ID())
		} else {
			installTs = sdkstate.InstallLocalSdk(st, setup)
		}

		if prevInstall != nil {
			installTs.WaitAll(prevInstall)
		}
		prevInstall = installTs
		install.AddAll(installTs)

		// Make sure that the hook tasks are not concurrent
		setupHookTask := hookstate.Hook(st, w, setup.Name, hookstate.SetupBase)
		if prevSetup != nil {
			setupHookTask.WaitFor(prevSetup)
		}
		prevSetup = setupHookTask
		setupHook.AddTask(setupHookTask)

		autoconnect := st.NewTask("auto-connect", fmt.Sprintf("Auto-connect interfaces of %q SDK", setup.Name))
		autoconnect.Set("sdk", setup.Name)
		autoConnect.AddTask(autoconnect)
		if prevAuto != nil {
			autoconnect.WaitFor(prevAuto)
		}
		prevAuto = autoconnect
	}
	setupHook.WaitAll(install)
	autoConnect.WaitAll(setupHook)
	autoConnect.WaitAll(install)

	all := state.NewTaskSet(install.Tasks()...)
	all.AddAll(setupHook)
	all.AddAll(autoConnect)
	return all
}

func checkHealthHooks(st *state.State, file *workshop.File) *state.TaskSet {
	var prevCheck *state.Task
	checkHealth := state.NewTaskSet()
	for _, sdk := range file.Sdks {
		checkHealthTask := hookstate.HookWithTimeout(st, file.Name, sdk.Name, hookstate.CheckHealth, checkHealthTimeout)
		if prevCheck != nil {
			checkHealthTask.WaitFor(prevCheck)
		}
		prevCheck = checkHealthTask
		checkHealth.AddTask(checkHealthTask)
	}
	return checkHealth
}

func constructWorkshop(st *state.State, file *workshop.File, project workshop.Project) *state.TaskSet {
	base := st.NewTask("download-base", fmt.Sprintf("Download %q base image", file.Base))
	base.Set("workshop-base", file.Base)

	create := st.NewTask("create-workshop", fmt.Sprintf("Create new %q workshop", file.Name))
	create.Set("workshop-file", file)
	create.WaitFor(base)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	mountProject.WaitFor(create)

	mountAptCache := st.NewTask("mount-apt-cache", fmt.Sprintf("Mount apt cache directory %q", dirs.AptCachePath))
	mountAptCache.WaitFor(mountProject)

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", file.Name))
	start.WaitFor(mountAptCache)
	return state.NewTaskSet(base, create, mountProject, mountAptCache, start)
}

func launch(st *state.State, file *workshop.File, sdks []sdk.Setup, project workshop.Project) *state.TaskSet {
	// check and download all the required SDKs
	retrieve := retrieveSdks(st, sdks)

	// create volume to store deb packages
	createAptCache := st.NewTask("create-apt-cache", fmt.Sprintf("Create apt cache for %q", file.Name))

	// create a basic workshop
	create := constructWorkshop(st, file, project)
	create.WaitAll(retrieve)
	create.WaitFor(createAptCache)

	// install the downloaded sdks
	launch := installSdks(st, file.Name, sdks, retrieve)
	launch.WaitAll(create)

	// run a quick check health script (for every SDK, if present)
	checkHealth := checkHealthHooks(st, file)
	checkHealth.WaitAll(launch)
	launch.AddAll(checkHealth)

	all := state.NewTaskSet(retrieve.Tasks()...)
	all.AddAll(retrieve)
	all.AddTask(createAptCache)
	all.AddAll(create)
	all.AddAll(launch)

	for _, task := range all.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", project)
	}

	return all
}

func (w *WorkshopManager) RefreshMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		err = w.CheckStatus(
			ctx,
			name,
			projectId,
			[]healthstate.Status{healthstate.ReadyStatus})
		if err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
		}
	}

	workshops, err := w.Workshops(ctx, projectId)
	if err != nil {
		return nil, err
	}

	var files []*workshop.File
	var installed, toinstall [][]sdk.Setup
	for _, ws := range names {
		idx := slices.IndexFunc(workshops, func(w *workshop.Workshop) bool { return w.Name == ws })
		if idx == -1 {
			return nil, fmt.Errorf("cannot refresh %q: %w", ws, workshop.ErrWorkshopNotLaunched)
		}
		file, err := project.Workshop(ws)
		if err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", ws, err)
		}
		files = append(files, file)

		res, err := launchStoreInfo(w.state, ctx, projectId, file)
		if err != nil {
			return nil, err
		}
		var newSdks []sdk.Setup
		for _, s := range res {
			newSdks = append(newSdks, sdk.Setup{Name: s.Name, Channel: s.Channel, Revision: s.Revision})
		}

		found, err := maybeSketch(ctx, projectId, ws)
		if err != nil {
			return nil, err
		}
		if found {
			newSdks = append(newSdks, sdk.Setup{Name: sdk.Sketch, Revision: sdk.Revision{N: -1}})
		}

		toinstall = append(toinstall, newSdks)
		installed = append(installed, maps.Values(workshops[idx].Sdks))
	}

	fullrefresh, err := refreshMany(w.state, files, installed, toinstall, *project)
	if err != nil {
		return nil, err
	}
	return fullrefresh, nil
}

func (w *WorkshopManager) RefreshLocalSdk(ctx context.Context, pid string, wpn string, sdkn string) ([]*state.TaskSet, error) {
	err := w.CheckStatus(
		ctx,
		wpn,
		pid,
		[]healthstate.Status{healthstate.ReadyStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot refresh %q: %w", wpn, err)
	}

	var taskset []*state.TaskSet
	wp, err := w.Workshop(ctx, wpn, pid)
	if err != nil {
		return nil, err
	}
	ts, err := w.refreshLocalSdk(wp, sdkn)
	if err != nil {
		return nil, err
	}
	taskset = append(taskset, ts)
	return taskset, nil
}

func (w *WorkshopManager) refreshLocalSdk(wp *workshop.Workshop, sdkn string) (*state.TaskSet, error) {
	cur, installed := wp.Sdks[sdkn]
	var setup sdk.Setup
	if installed {
		setup = sdk.Setup{Name: sdkn, Revision: sdk.Revision{N: cur.Revision.N - 1}}
	} else {
		setup = sdk.Setup{Name: sdkn, Revision: sdk.Revision{N: -1}}
	}

	st := w.state
	ts := state.NewTaskSet()

	if installed {
		createStateStorage := st.NewTask("create-state-storage", "Create SDK state storage")
		ts.AddTask(createStateStorage)

		saveStateHook := hookstate.Hook(st, wp.Name, setup.Name, hookstate.SaveState)
		saveStateHook.WaitFor(createStateStorage)

		disconnect := st.NewTask("auto-disconnect", fmt.Sprintf(`Disconnect interfaces of %q SDK`, setup.Name))
		disconnect.Set("sdk", setup.Name)
		disconnect.WaitFor(saveStateHook)
		ts.AddTask(saveStateHook)
		ts.AddTask(disconnect)
	}

	install := sdkstate.InstallLocalSdk(st, setup)
	install.WaitAll(ts)
	ts.AddAll(install)

	setupHook := hookstate.Hook(st, wp.Name, setup.Name, hookstate.SetupBase)
	setupHook.WaitAll(install)
	ts.AddTask(setupHook)

	autoconnect := st.NewTask("auto-connect", fmt.Sprintf("Auto-connect interfaces of %q SDK", setup.Name))
	autoconnect.Set("sdk", setup.Name)
	autoconnect.WaitFor(setupHook)
	ts.AddTask(autoconnect)

	if installed {
		restoreState := hookstate.Hook(st, wp.Name, setup.Name, hookstate.RestoreState)
		restoreState.WaitFor(autoconnect)
		ts.AddTask(restoreState)
	}

	checkHealth := hookstate.Hook(st, wp.Name, setup.Name, hookstate.CheckHealth)
	checkHealth.WaitAll(ts)
	ts.AddTask(checkHealth)

	if installed {
		removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
		removeStateStorage.WaitFor(checkHealth)
		ts.AddTask(removeStateStorage)
	}

	for _, task := range ts.Tasks() {
		task.Set("workshop", wp.Name)
		task.Set("project", wp.Project)
	}

	return ts, nil
}

func refreshMany(st *state.State, files []*workshop.File, installed [][]sdk.Setup,
	toInstall [][]sdk.Setup, project workshop.Project) ([]*state.TaskSet, error) {
	taskset := make([]*state.TaskSet, 0, len(files))

	for i, file := range files {
		tasks, err := refresh(st, file, installed[i], toInstall[i], project)
		if err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", file.Name, err)
		}
		taskset = append(taskset, tasks)
	}

	for _, ts := range taskset {
		cleanup, err := ts.Edge(EdgeRefreshCleanup)
		if err != nil {
			return nil, err
		}

		// We will iterate over other refreshes and make sure that the cleanup
		// task of our refresh will wait until all the other refresh operations
		// finished. This will ensure that we start to remove the workshops'
		// previous copies once all the refresh operations were successful (at
		// this stage, we only need to remove a stashed copy, the newly refreshed
		// workshop is already up and running). Thus, every CleanupEdge will
		// wait for ALL the LastBeforeRefreshIrreversibleEdge tasks of all the
		// other changes before execution.
		for _, otherts := range taskset {
			if ts != otherts {
				last, err := otherts.Edge(EdgeLastTaskBeforeRefreshIrreversible)
				if err != nil {
					return nil, err
				}
				cleanup.WaitFor(last)
			}
		}
	}

	return taskset, nil
}

func refresh(st *state.State, file *workshop.File, installed []sdk.Setup, toInstall []sdk.Setup, p workshop.Project) (*state.TaskSet, error) {
	// 1. Save previous state
	// 2. Stop previous workshop
	// 3. Put to stash
	// 4. Launch the new workshop
	// 5. Run restore state
	// 6. Delete the old workshop
	retrieve := retrieveSdks(st, toInstall)

	createStateStorage := st.NewTask("create-state-storage", "Create SDK state storage")
	createStateStorage.WaitAll(retrieve)

	// the saveStateHooks can be empty if the old SDKs were all removed in
	// the new version of the workshop
	saveStateHooks := saveStateHooks(st, file.Name, installed, file.Sdks)
	saveStateHooks.WaitFor(createStateStorage)

	// disconnect and remove SDKs plugs and slots
	disconnect := disconnectSdks(installed, st)
	disconnect.WaitAll(saveStateHooks)
	disconnect.WaitFor(createStateStorage)

	// put the workshop (old) away and disconnect its interfaces
	putToStash := st.NewTask("stash-workshop", fmt.Sprintf("Stash previous %q workshop", file.Name))
	putToStash.WaitAll(disconnect)
	putToStash.WaitAll(saveStateHooks)
	putToStash.WaitFor(createStateStorage)

	// launch a workshop (new)
	launch := constructWorkshop(st, file, p)
	launch.WaitFor(putToStash)

	// install SDKs and run restore-state scripts. The restoreStateHooks can be
	// empty if the old SDKs were all removed in the new version of the workshop
	install := installSdks(st, file.Name, toInstall, retrieve)
	install.WaitAll(launch)
	launch.AddAll(install)

	// note: restore-state list may be empty if there are no SDKs or
	// old SDKs are all gone
	restoreState := restoreStateHooks(st, file.Name, installed, file.Sdks)
	restoreState.WaitAll(install)
	launch.AddAll(restoreState)

	checkHealth := checkHealthHooks(st, file)
	checkHealth.WaitAll(install)
	checkHealth.WaitAll(restoreState)
	launch.AddAll(checkHealth)

	// remove the state storage after running restore-state scripts
	removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
	createLen := len(launch.Tasks())
	lastLaunchTask := launch.Tasks()[createLen-1]
	removeStateStorage.WaitFor(lastLaunchTask)
	launch.MarkEdge(lastLaunchTask, EdgeLastTaskBeforeRefreshIrreversible)

	// remove the workshop from stash after the state storage was detached
	removeFromStash := st.NewTask("remove-workshop-stash", fmt.Sprintf("Remove %q workshop from stash", file.Name))
	removeFromStash.WaitFor(removeStateStorage)

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
	cleanupLane := st.NewLane()
	removeStateStorage.JoinLane(cleanupLane)
	removeFromStash.JoinLane(cleanupLane)

	refresh := state.NewTaskSet([]*state.Task{}...)
	refresh.AddAll(retrieve)
	refresh.AddTask(createStateStorage)
	refresh.AddAll(saveStateHooks)
	refresh.AddAll(disconnect)
	refresh.AddTask(putToStash)
	refresh.AddAllWithEdges(launch)
	refresh.AddTask(removeStateStorage)
	refresh.AddTask(removeFromStash)

	refresh.MarkEdge(removeStateStorage, EdgeRefreshCleanup)

	for _, task := range refresh.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", p)
	}

	return refresh, nil
}

func disconnectSdks(sdks []sdk.Setup, st *state.State) *state.TaskSet {
	prev := st.NewTask("auto-disconnect", fmt.Sprintf(`Disconnect interfaces of %q SDK`, sdk.System.String()))
	prev.Set("sdk", sdk.System.String())
	disconnectSet := state.NewTaskSet(prev)
	for _, s := range sdks {
		disc := st.NewTask("auto-disconnect", fmt.Sprintf("Disconnect interfaces of %q SDK", s.Name))
		disc.Set("sdk", s.Name)
		disc.WaitFor(prev)
		disconnectSet.AddTask(disc)
		prev = disc
	}
	return disconnectSet
}

func saveStateHooks(st *state.State, w string, sdks []sdk.Setup, newSdks []workshop.SdkRecord,
) *state.TaskSet {
	return createStateHooks(st, w, sdks, newSdks, hookstate.SaveState)
}

func restoreStateHooks(st *state.State, w string, sdks []sdk.Setup, newSdks []workshop.SdkRecord) *state.TaskSet {
	return createStateHooks(st, w, sdks, newSdks, hookstate.RestoreState)
}

func createStateHooks(st *state.State, w string, sdks []sdk.Setup, newSdks []workshop.SdkRecord, hooktype hookstate.WorkshopHookType) *state.TaskSet {
	stateHooks := state.NewTaskSet([]*state.Task{}...)
	prevRestore := (*state.Task)(nil)
	for _, newsdk := range newSdks {
		// the state hooks will only be set for the SDKs that were installed AND
		// were not removed from the workshop file at the time of refresh
		if slices.IndexFunc(sdks, func(s sdk.Setup) bool { return s.Name == newsdk.Name }) == -1 {
			continue
		}
		stateHook := hookstate.Hook(st, w, newsdk.Name, hooktype)
		stateHooks.AddTask(stateHook)
		if prevRestore != nil {
			stateHook.WaitFor(prevRestore)
		}
		prevRestore = stateHook
	}
	return stateHooks
}

func (w *WorkshopManager) StartMany(ctx context.Context, names []string, projectId string, opChangeId string) ([]*state.TaskSet, error) {
	// check if all the workshops are stopped
	for _, name := range names {
		err := w.CheckStatus(
			ctx,
			name,
			projectId,
			[]healthstate.Status{healthstate.StoppedStatus})
		if err != nil {
			return nil, fmt.Errorf("cannot start %q: %w", name, err)
		}
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := startMany(w.state, names, *project)
	if err != nil {
		return nil, err
	}
	return taskset, nil
}

func startMany(st *state.State, names []string, project workshop.Project) ([]*state.TaskSet, error) {
	taskset := []*state.TaskSet{}

	for _, name := range names {
		start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
		start.Set("workshop", name)
		start.Set("project", project)

		taskset = append(taskset, state.NewTaskSet(start))
	}

	return taskset, nil
}

func (w *WorkshopManager) StopMany(ctx context.Context, names []string, projectId string, opChangeId string) ([]*state.TaskSet, error) {
	for _, name := range names {
		err := w.CheckStatus(
			ctx,
			name,
			projectId,
			[]healthstate.Status{healthstate.ReadyStatus, healthstate.StoppedStatus})
		if err != nil {
			return nil, fmt.Errorf("cannot stop %q: %w", name, err)
		}
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := stopMany(w.state, names, *project)
	if err != nil {
		return nil, err
	}
	return taskset, nil
}

func stopMany(st *state.State, names []string, project workshop.Project) ([]*state.TaskSet, error) {
	taskset := []*state.TaskSet{}

	for _, name := range names {
		stop := st.NewTask("stop-workshop", fmt.Sprintf("Stop %q workshop", name))
		stop.Set("force", false)
		stop.Set("workshop", name)
		stop.Set("project", project)

		taskset = append(taskset, state.NewTaskSet(stop))
	}

	return taskset, nil
}

func (w *WorkshopManager) Exec(ctx context.Context, name, projectId string, args *workshop.ExecArgs, script bool) (*state.TaskSet, error) {
	err := w.CheckStatus(
		ctx,
		name,
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus, healthstate.PendingStatus})
	if err != nil {
		return nil, err
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
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
	if script {
		name := args.Command[0]
		cp := w.state.NewTask("install-script", fmt.Sprintf("Install script %q", name))
		exec := w.state.NewTask("exec", fmt.Sprintf("Exec script %q", name))

		// install-script will modify args and pass it to exec.
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

func (w *WorkshopManager) RemoveMany(ctx context.Context, names []string, projectId string, opChangeId string) ([]*state.TaskSet, error) {
	for _, name := range names {
		err := w.CheckStatus(
			ctx,
			name,
			projectId,
			[]healthstate.Status{healthstate.ReadyStatus, healthstate.ErrorStatus, healthstate.StoppedStatus})
		if err != nil {
			return nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)

	var workshops = make([]*workshop.Workshop, 0, len(names))
	for _, name := range names {
		if w, err := w.backend.Workshop(ctx, name); err != nil {
			return nil, err
		} else {
			workshops = append(workshops, w)
		}
	}

	taskset, err := removeMany(w.state, workshops, *project)
	if err != nil {
		return nil, err
	}
	return taskset, nil
}

func removeMany(st *state.State, workshops []*workshop.Workshop, project workshop.Project) ([]*state.TaskSet, error) {
	taskset := []*state.TaskSet{}
	for _, name := range workshops {
		remove, err := remove(st, name, project)
		if err != nil {
			return nil, err
		}
		taskset = append(taskset, remove)
	}
	return taskset, nil
}

func remove(st *state.State, w *workshop.Workshop, project workshop.Project) (*state.TaskSet, error) {
	removeSet := state.NewTaskSet()
	disconnectSet := disconnectSdks(maps.Values(w.Sdks), st)

	discard := st.NewTask("discard-conns", fmt.Sprintf("Discard %q undesired connections", w.Name))
	discard.WaitAll(disconnectSet)

	remove := st.NewTask("remove-workshop", fmt.Sprintf("Remove %q workshop", w.Name))
	remove.WaitAll(disconnectSet)
	remove.WaitFor(discard)

	removeAptCache := st.NewTask("remove-apt-cache", fmt.Sprintf("Remove apt cache for %q", w.Name))
	removeAptCache.WaitFor(remove)

	// The apt cache cannot be removed until the workshop has stopped, which
	// currently happens as part of remove-workshop. Since there is no way to
	// undo remove-workshop, we run remove-apt-cache in a separate lane. If an
	// error occurs when removing the cache, it will not affect the other tasks.
	cleanupLane := st.NewLane()
	removeAptCache.JoinLane(cleanupLane)

	removeSet.AddAll(disconnectSet)
	removeSet.AddTask(discard)
	removeSet.AddTask(remove)
	removeSet.AddTask(removeAptCache)

	for _, task := range removeSet.Tasks() {
		task.Set("workshop", w.Name)
		task.Set("project", project)
	}
	return removeSet, nil
}

func (w *WorkshopManager) Remount(ctx context.Context, st *state.State, plug interfaces.PlugRef, source string, projectId string) (*state.TaskSet, error) {
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("cannot remount: the `source` path must be absolute")
	}

	source = filepath.Clean(source)

	err := w.CheckStatus(
		ctx,
		plug.Workshop,
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus, healthstate.StoppedStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot remount %q: %w", plug.ShortRef(), err)
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	wp, err := w.Workshop(ctx, plug.Workshop, plug.ProjectId)
	if err != nil {
		return nil, fmt.Errorf("cannot load workshop %q: %w", plug.Workshop, err)
	}

	master, _ := ifacestate.MaybeBound(wp, plug)

	remount := st.NewTask("remount", fmt.Sprintf(`Remount %q`, plug.ShortRef()))
	remount.Set("workshop", plug.Workshop)
	remount.Set("project", project)
	remount.Set("plug", master)
	remount.Set("host-source", source)

	return state.NewTaskSet(remount), nil
}
