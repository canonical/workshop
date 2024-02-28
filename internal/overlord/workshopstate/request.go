package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/operation"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshopbackend"
	"golang.org/x/exp/slices"
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

func (w *WorkshopManager) loadProject(ctx context.Context, id string) (*workshopbackend.Project, error) {
	username, ok := ctx.Value(workshopbackend.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key user not found")
	}

	projects, err := w.backend.Projects(ctx)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(projects[username], func(p *workshopbackend.Project) bool { return p.ProjectId == id })
	if idx == -1 {
		return nil, fmt.Errorf("no project found with \"id\" %v", id)
	}
	return projects[username][idx], nil
}

func (w *WorkshopManager) LaunchMany(ctx context.Context, names []string, projectId string, opChangeId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	taskset := make([]*state.TaskSet, 0, len(names))
	for _, name := range names {
		file, err := project.WorkshopFile(name)
		if err != nil {
			return nil, fmt.Errorf("cannot read %q file: %w", name, err)
		}

		workshop, err := w.Workshop(ctx, name, projectId)
		if workshop != nil {
			return nil, fmt.Errorf("cannot launch: %q already exists", name)
		}
		if !errors.Is(err, workshopbackend.ErrWorkshopNotFound) {
			return nil, err
		}

		tasks := launch(w.state, file, project)
		taskset = append(taskset, tasks)
	}

	if err = w.startOperationMany(names, projectId, operation.Operation{ChangeId: opChangeId, Operation: operation.OperationLaunch}); err != nil {
		return nil, err
	}
	return taskset, nil
}

func retrieveSdks(st *state.State, file *workshopbackend.WorkshopFile) *state.TaskSet {
	retrieve := state.NewTaskSet([]*state.Task{}...)
	for _, sdk := range file.Sdks {
		r := sdkstate.Retrieve(st, &sdk)
		retrieve.AddTask(r)
	}
	return retrieve
}

func installSdks(st *state.State, file *workshopbackend.WorkshopFile, retrieveSet *state.TaskSet) *state.TaskSet {
	var prevInstall *state.TaskSet
	var prevSetup, prevAuto *state.Task
	install := state.NewTaskSet([]*state.Task{}...)
	setupHook := state.NewTaskSet([]*state.Task{}...)
	autoConnectSet := state.NewTaskSet([]*state.Task{}...)
	for idx, sdk := range file.Sdks {
		// The install task sets must not run concurrently as exec ops are not
		// allowed by LXD to be run concurrently and in general case we cannot
		// guarantee safety of concurrent installations
		installTaskSet := sdkstate.Install(st, sdk.Name, retrieveSet.Tasks()[idx].ID())
		if prevInstall != nil {
			installTaskSet.WaitAll(prevInstall)
		}
		prevInstall = installTaskSet
		install.AddAll(installTaskSet)

		// Make sure that the hook tasks are not concurrent
		setupHookTask := hookstate.Hook(st, file.Name, sdk.Name, hookstate.SetupBase)
		if prevSetup != nil {
			setupHookTask.WaitFor(prevSetup)
		}
		prevSetup = setupHookTask
		setupHook.AddTask(setupHookTask)

		autoconnect := st.NewTask("auto-connect", fmt.Sprintf("Auto-connect interfaces of %q SDK", sdk.Name))
		autoconnect.Set("sdk", sdk.Name)
		autoConnectSet.AddTask(autoconnect)
		if prevAuto != nil {
			autoconnect.WaitFor(prevAuto)
		}
		prevAuto = autoconnect
	}
	setupHook.WaitAll(install)
	autoConnectSet.WaitAll(setupHook)

	all := state.NewTaskSet(install.Tasks()...)
	all.AddAll(setupHook)
	all.AddAll(autoConnectSet)
	return all
}

func checkHealthHooks(st *state.State, file *workshopbackend.WorkshopFile) *state.TaskSet {
	var prevCheck *state.Task
	checkHealth := state.NewTaskSet([]*state.Task{}...)
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

func constructWorkshop(st *state.State, file *workshopbackend.WorkshopFile, project *workshopbackend.Project) *state.TaskSet {
	create := st.NewTask("create-workshop", fmt.Sprintf("Create new %q workshop", file.Name))
	create.Set("base", file.Base)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	mountProject.WaitFor(create)

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", file.Name))
	start.WaitFor(mountProject)
	return state.NewTaskSet(create, mountProject, start)
}

func launch(st *state.State, file *workshopbackend.WorkshopFile, project *workshopbackend.Project) *state.TaskSet {
	// check and download all the required SDKs
	retrieve := retrieveSdks(st, file)

	// create a basic workshop
	create := constructWorkshop(st, file, project)
	create.WaitAll(retrieve)

	// launch the downloaded sdks
	launch := installSdks(st, file, retrieve)
	launch.WaitAll(create)

	// run a quick check health script (for every SDK, if present)
	checkHealth := checkHealthHooks(st, file)
	checkHealth.WaitAll(launch)
	launch.AddAll(checkHealth)

	all := state.NewTaskSet(retrieve.Tasks()...)
	all.AddAll(create)
	all.AddAll(launch)

	tasksLen := len(all.Tasks())
	all.Tasks()[0].Set("start-operation", true)
	all.Tasks()[tasksLen-1].Set("stop-operation", true)

	for _, task := range all.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", project)
	}

	return all
}

func (w *WorkshopManager) RefreshMany(ctx context.Context,
	names []string, projectId string, refreshMode operation.RefreshMode, opChangeId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	err = w.CheckStatus(
		ctx,
		names,
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot refresh: %v", err)
	}

	_, workshops, err := w.Workshops(ctx, projectId)
	if err != nil {
		return nil, err
	}

	files := make([]*workshopbackend.WorkshopFile, 0)
	content := make([][]sdk.Setup, 0)
	for _, workshop := range names {
		idx := slices.IndexFunc(workshops, func(w *workshopbackend.Workshop) bool { return w.Name == workshop })
		if idx == -1 {
			return nil, fmt.Errorf("cannot refresh: workshop %q not found", workshop)
		}
		file, err := project.WorkshopFile(workshops[idx].Name)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
		content = append(content, workshops[idx].Content())
	}

	taskset, err := refreshMany(w.state, files, content, project)
	if err != nil {
		return nil, err
	}

	if err = w.startOperationMany(names, projectId, operation.Operation{
		ChangeId:    opChangeId,
		Operation:   operation.OperationRefresh,
		WaitOnError: refreshMode == operation.RefreshWaitOnError,
	}); err != nil {
		return nil, err
	}

	return taskset, nil
}

func refreshMany(st *state.State, w []*workshopbackend.WorkshopFile, content [][]sdk.Setup,
	project *workshopbackend.Project) ([]*state.TaskSet, error) {
	taskset := make([]*state.TaskSet, 0, len(w))

	for i, w := range w {
		tasks, err := refresh(st, w, content[i], project)
		if err != nil {
			return nil, fmt.Errorf("cannot refresh \"%s\" workshop: %w", w, err)
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

func refresh(st *state.State, file *workshopbackend.WorkshopFile, content []sdk.Setup, p *workshopbackend.Project) (*state.TaskSet, error) {
	// 1. Save previous state
	// 2. Stop previous workshop
	// 3. Put to stash
	// 4. Launch the new workshop
	// 5. Run restore state
	// 6. Delete the old workshop
	retrieve := retrieveSdks(st, file)

	createStateStorage := st.NewTask("create-state-storage", "Create SDK state storage")
	createStateStorage.WaitAll(retrieve)

	// the saveStateHooks can be empty if the old SDKs were all removed in
	// the new version of the workshop
	saveStateHooks := saveStateHooks(st, file.Name, content, file.Sdks)
	saveStateHooks.WaitFor(createStateStorage)

	// disconnect and remove SDKs plugs and slots
	disconnect := disconnectSdks(content, st)
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
	install := installSdks(st, file, retrieve)
	install.WaitAll(launch)
	launch.AddAll(install)

	// note: restore-state list may be empty if there are no SDKs or
	// old SDKs are all gone
	restoreState := restoreStateHooks(st, file.Name, content, file.Sdks)
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

	// mark the first task to start the operation so if cancelled or failed the
	// operation will be removed from the list of the active ones
	refresh.Tasks()[0].Set("start-operation", true)

	// mark the last task to stop the operation
	// and make the workshop available for other commands
	removeFromStash.Set("stop-operation", true)
	for _, i := range refresh.Tasks() {
		i.Set("workshop", file.Name)
		i.Set("project", *p)
	}

	return refresh, nil
}

func disconnectSdks(content []sdk.Setup, st *state.State) *state.TaskSet {
	disconnectSet := []*state.Task{}
	prev := (*state.Task)(nil)
	for _, s := range content {
		disc := st.NewTask("disconnect", fmt.Sprintf("Disconnect interfaces of %q SDK", s.Name))
		disc.Set("sdk", s.Name)
		if prev != nil {
			disc.WaitFor(prev)
		}
		disconnectSet = append(disconnectSet, disc)
		prev = disc
	}
	return state.NewTaskSet(disconnectSet...)
}

func saveStateHooks(st *state.State, workshop string, content []sdk.Setup, newContent workshopbackend.SdkList,
) *state.TaskSet {
	return createStateHooks(st, workshop, content, newContent, hookstate.SaveState)
}

func restoreStateHooks(st *state.State, workshop string, content []sdk.Setup, newContent workshopbackend.SdkList) *state.TaskSet {
	return createStateHooks(st, workshop, content, newContent, hookstate.RestoreState)
}

func createStateHooks(st *state.State, workshop string, content []sdk.Setup, newContent workshopbackend.SdkList, hooktype hookstate.WorkshopHookType) *state.TaskSet {
	stateHooks := state.NewTaskSet([]*state.Task{}...)
	prevRestore := (*state.Task)(nil)
	for _, newsdk := range newContent {
		// the state hooks will only be set for the SDKs that were installed AND
		// were not removed from the workshop file at the time of refresh
		if slices.IndexFunc(content, func(s sdk.Setup) bool { return s.Name == newsdk.Name }) == -1 {
			continue
		}
		stateHook := hookstate.Hook(st, workshop, newsdk.Name, hooktype)
		stateHooks.AddTask(stateHook)
		if prevRestore != nil {
			stateHook.WaitFor(prevRestore)
		}
		prevRestore = stateHook
	}
	return stateHooks
}

func (w *WorkshopManager) StartMany(ctx context.Context, names []string, projectId string, opChangeId string) (*state.TaskSet, error) {
	// check if all the workshops are stopped
	err := w.CheckStatus(
		ctx,
		names,
		projectId,
		[]healthstate.Status{healthstate.StoppedStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot start: %w", err)
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := startMany(w.state, names, project)
	if err != nil {
		return nil, err
	}

	if err = w.startOperationMany(names, projectId, operation.Operation{ChangeId: opChangeId, Operation: operation.OperationStart}); err != nil {
		return nil, err
	}
	return taskset, nil
}

func startMany(st *state.State, names []string, project *workshopbackend.Project) (*state.TaskSet, error) {
	taskset := state.NewTaskSet([]*state.Task{}...)

	for _, name := range names {
		start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
		// start is a single task, so it is the beginning and the end of the operation
		start.Set("start-operation", true)
		start.Set("stop-operation", true)
		taskset.AddTask(start)

		start.Set("workshop", name)
		start.Set("project", *project)
	}

	return taskset, nil
}

func (w *WorkshopManager) StopMany(ctx context.Context, names []string, projectId string, opChangeId string) (*state.TaskSet, error) {
	err := w.CheckStatus(
		ctx,
		names,
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus, healthstate.StoppedStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot stop: %w", err)
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := stopMany(w.state, names, project)
	if err != nil {
		return nil, err
	}

	if err = w.startOperationMany(names, projectId, operation.Operation{ChangeId: opChangeId, Operation: operation.OperationStop}); err != nil {
		return nil, err
	}
	return taskset, nil
}

func stopMany(st *state.State, names []string, project *workshopbackend.Project) (*state.TaskSet, error) {
	taskset := state.NewTaskSet([]*state.Task{}...)

	for _, name := range names {
		stop := st.NewTask("stop-workshop", fmt.Sprintf("Stop %q workshop", name))
		// start is a single task, so it is the beginning and the end of the operation
		stop.Set("start-operation", true)
		stop.Set("stop-operation", true)
		stop.Set("force", false)
		taskset.AddTask(stop)

		stop.Set("workshop", name)
		stop.Set("project", *project)
	}

	return taskset, nil
}

type ExecMeta struct {
	Environment map[string]string
	WorkingDir  string
}

func (w *WorkshopManager) Exec(ctx context.Context, name, projectId string, args *workshopbackend.ExecArgs) (*state.Task, error) {
	err := w.CheckStatus(
		ctx,
		[]string{name},
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus, healthstate.PendingStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot exec: %w", err)
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshopbackend.ContextProjectId, project.ProjectId)
	wrkspc, err := w.backend.WorkshopFs(ctx, name)
	if err != nil {
		return nil, err
	}
	defer wrkspc.Close()

	info, err := wrkspc.Stat(args.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("%s does not exist", args.WorkDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", args.WorkDir)
	}

	exec := w.state.NewTask("exec", fmt.Sprintf("Exec command %q", args.Command[0]))

	exec.Set("exec-setup", args)
	exec.Set("project", project)
	exec.Set("workshop", name)

	return exec, nil
}

func (w *WorkshopManager) RemoveMany(ctx context.Context, names []string, projectId string, opChangeId string) (*state.TaskSet, error) {
	err := w.CheckStatus(
		ctx,
		names,
		projectId,
		[]healthstate.Status{healthstate.ReadyStatus, healthstate.ErrorStatus, healthstate.StoppedStatus})
	if err != nil {
		return nil, fmt.Errorf("cannot remove: %w", err)
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshopbackend.ContextProjectId, project.ProjectId)

	var workshops = make([]*workshopbackend.Workshop, 0, len(names))
	for _, name := range names {
		if workshop, err := w.backend.Workshop(ctx, name); err != nil {
			return nil, err
		} else {
			workshops = append(workshops, workshop)
		}
	}

	taskset, err := removeMany(w.state, workshops, project)
	if err != nil {
		return nil, err
	}

	if err := w.startOperationMany(names, projectId, operation.Operation{ChangeId: opChangeId, Operation: operation.OperationRemove}); err != nil {
		return nil, err
	}

	return taskset, nil
}

func (w *WorkshopManager) startOperationMany(names []string, projectId string, op operation.Operation) error {
	rev := revert.New()
	defer rev.Fail()
	for _, name := range names {
		err := operation.StartOperation(w.state, name, projectId, op)
		if err != nil {
			return err
		}
		name := name // go loop var capturing issue
		rev.Add(func() {
			operation.StopOperation(w.state, name, projectId, op.Operation)
		})
	}
	rev.Success()
	return nil
}

func removeMany(st *state.State, workshops []*workshopbackend.Workshop, project *workshopbackend.Project) (*state.TaskSet, error) {
	taskset := state.NewTaskSet([]*state.Task{}...)
	for _, name := range workshops {
		remove, err := remove(st, name, project)
		if err != nil {
			return nil, err
		}
		taskset.AddAll(remove)
	}
	return taskset, nil
}

func remove(st *state.State, workshop *workshopbackend.Workshop, project *workshopbackend.Project) (*state.TaskSet, error) {
	removeSet := state.NewTaskSet()
	disconnectSet := disconnectSdks(workshop.Content(), st)
	remove := st.NewTask("remove-workshop", fmt.Sprintf("Remove %q workshop", workshop.Name))
	if len(disconnectSet.Tasks()) > 0 {
		disconnectSet.Tasks()[0].Set("start-operation", true)
	} else {
		remove.Set("start-operation", true)
	}
	remove.Set("stop-operation", true)
	remove.WaitAll(disconnectSet)
	removeSet.AddAll(disconnectSet)
	removeSet.AddTask(remove)

	for _, i := range removeSet.Tasks() {
		i.Set("workshop", workshop.Name)
		i.Set("project", project)
	}
	return removeSet, nil
}
