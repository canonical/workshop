package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/cmdstate"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	// mark the last task in a taskset after which refresh becomes irreversible (i.e. the following tasks
	// will not be possible to undo, e.g. removing an old workshop copy)
	EdgeRefreshLastTaskBeforeCleanup = state.TaskSetEdge("last-before-irreversible")

	// mark the tasks that denote irreversible clean up logic for refresh (e.g.
	// removing state storage and the old workshop copy)
	EdgeRefreshFirstCleanupTask = state.TaskSetEdge("refresh-cleanup")
)

var checkHealthTimeout = 5 * time.Second

func (w *WorkshopManager) loadProject(ctx context.Context, id string) (workshop.Project, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return workshop.Project{}, fmt.Errorf("context key user not found")
	}

	projects, err := w.backend.Projects(ctx)
	if err != nil {
		return workshop.Project{}, err
	}

	idx := slices.IndexFunc(projects[username], func(p workshop.Project) bool { return p.ProjectId == id })
	if idx == -1 {
		return workshop.Project{}, fmt.Errorf("no project found with \"id\" %v", id)
	}
	return projects[username][idx], nil
}

func (w *WorkshopManager) LaunchMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key user not found")
	}

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return nil, err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	local := localStore{usr, userDataDir, project}

	reqs, err := w.resolveWorkshops(ctx, project, names, "launch")
	if err != nil {
		return nil, err
	}

	taskset := make([]*state.TaskSet, 0, len(names))
	for _, req := range reqs {
		name := req.file.Name
		// Make sure the workshop doesn't exist.
		// Has to happen after calling resolveWorkshops (because it unlocks the state).
		_, err := w.Workshop(ctx, name, projectId)
		if err == nil {
			return nil, fmt.Errorf("cannot launch %q: workshop exists", name)
		} else if !errors.Is(err, workshop.ErrWorkshopNotLaunched) {
			return nil, fmt.Errorf("cannot launch %q, failed to check whether the workshop exists: %w", name, err)
		}
		if err := conflict.CheckChangeConflict(w.state, projectId, name, nil); err != nil {
			return nil, fmt.Errorf("cannot launch %q, other changes in progress: %w", name, err)
		}

		localSdks, err := local.resolveSdks(name, req.file, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot launch %q: %w", name, err)
		}
		sdks := ordered(req.installOrder, req.storeSdks, localSdks)

		tasks := launch(w.state, req.file, sdks, project)
		taskset = append(taskset, tasks)
	}
	return taskset, nil
}

type workshopReq struct {
	// Up to date workshop definitions from the project directory.
	file *workshop.File
	// All possible SDKs (including sketch) in installation order.
	installOrder []string
	// Up to date SDK setups from the store.
	storeSdks []sdk.Setup
}

func (w *WorkshopManager) resolveWorkshops(ctx context.Context, project workshop.Project, names []string, action string) ([]workshopReq, error) {
	sto := sdk.StoreService(w.state)
	reqs := make([]workshopReq, 0, len(names))

	// Not an error, the state is locked; unlock it to let other requests to be
	// processed while we are getting the store info sorted.
	// This code can be concurrent with other changes,
	// so we avoid interacting with local SDKs.
	w.state.Unlock()
	defer w.state.Lock()

	for _, name := range names {
		file, err := project.Workshop(name)
		if err != nil {
			return nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}

		installOrder := make([]string, 1, len(file.Sdks)+2)
		installOrder[0] = sdk.System.String()
		for _, sk := range file.Sdks {
			if !workshop.IsImplicitSdk(sk.Name) {
				installOrder = append(installOrder, sk.Name)
			}
		}
		installOrder = append(installOrder, sdk.Sketch)

		storeSdks, err := resolveStoreSdks(sto, ctx, project.ProjectId, file)
		if err != nil {
			return nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		reqs = append(reqs, workshopReq{file: file, installOrder: installOrder, storeSdks: storeSdks})
	}

	return reqs, nil
}

func resolveStoreSdks(sto sdk.Store, ctx context.Context, projectid string, file *workshop.File) ([]sdk.Setup, error) {
	acts := []sdk.SdkAction{}
	for _, sd := range file.Sdks {
		if workshop.IsImplicitSdk(sd.Name) || workshop.IsProjectSdk(sd.Name) {
			continue
		}
		act := sdk.SdkAction{ProjectId: projectid, Workshop: file.Name, Name: sd.Name, Base: file.Base, Channel: sd.Channel, Action: sdk.Install}
		acts = append(acts, act)
	}

	infos, err := sto.SdkAction(ctx, acts)
	if err != nil {
		return nil, err
	}

	setups := make([]sdk.Setup, 1, len(infos)+1)
	setups[0] = sdk.Setup{Name: "system", Revision: system.SystemSdkRevision}
	for _, s := range infos {
		setups = append(setups, sdk.Setup{Name: s.Name, Channel: s.Channel, Revision: s.Revision})
	}

	return setups, nil
}

type localStore struct {
	user        *user.User
	userDataDir string
	project     workshop.Project
}

func (s *localStore) resolveSdks(name string, file *workshop.File, wp *workshop.Workshop) ([]sdk.Setup, error) {
	localSdks := []sdk.Setup{}

	for _, sd := range file.Sdks {
		if !workshop.IsProjectSdk(sd.Name) {
			continue
		}
		source := workshop.ProjectSdkPath("$PROJECT", strings.TrimPrefix(sd.Name, "project-"))
		localSdks = append(localSdks, sdk.Setup{Name: sd.Name, Source: source})
	}

	sketch, err := maybeSketch(s.userDataDir, s.project.ProjectId, name, wp == nil)
	if err != nil {
		return nil, err
	}
	if sketch != nil {
		localSdks = append(localSdks, *sketch)
	}

	for i, sk := range localSdks {
		var installed sdk.Revision
		if wp != nil {
			installed = wp.Sdks[sk.Name].Revision
		}

		source := workshop.ExpandSdkSource(sk.Source, s.project.Path)
		target := workshop.LocalSdkDir(s.userDataDir, s.project.ProjectId, name, sk.Name)

		localSdks[i].Revision, err = sdk.CommitRevision(s.user, source, target, installed)
		if err != nil {
			return nil, err
		}
	}

	return localSdks, nil
}

func maybeSketch(userDataDir, pid, name string, launch bool) (*sdk.Setup, error) {
	sketchdir := workshop.SketchSdkCurrent(userDataDir, pid, name)

	recs, err := os.ReadDir(sketchdir)
	// no Sketch SDK exists for the workshop and it is not an error.
	if (err == nil && len(recs) == 0) || osutil.IsDirNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if launch {
		// Old sketches might exist because the snap remove hook doesn't remove them.
		// No workshop exists currently; including the sketch SDK would be unexpected.
		// We remove it (but keep the stash) to prevent future refreshes from including it.
		if err = os.RemoveAll(sketchdir); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return &sdk.Setup{Name: sdk.Sketch, Source: sketchdir}, nil
}

func ordered(order []string, setups ...[]sdk.Setup) []sdk.Setup {
	ordered := make([]sdk.Setup, 0, len(order))

	for _, sk := range order {
		for _, setup := range setups {
			contains := func(sp sdk.Setup) bool { return sk == sp.Name }

			idx := slices.IndexFunc(setup, contains)
			if idx != -1 {
				ordered = append(ordered, setup[idx])
				break
			}
		}
	}
	return ordered
}

func retrieveBase(st *state.State, file *workshop.File) *state.Task {
	base := st.NewTask("download-base", fmt.Sprintf("Download %q base image", file.Base))
	base.Set("workshop-base", file.Base)
	return base
}

func retrieveSdks(st *state.State, sdks []sdk.Setup) (*state.TaskSet, map[string]string) {
	retrieve := state.NewTaskSet()
	retrieveMap := map[string]string{}
	for _, s := range sdks {
		if s.Revision.Store() {
			r := sdkstate.Retrieve(st, s)
			retrieve.AddTask(r)
			retrieveMap[s.Name] = r.ID()
		}
	}
	return retrieve, retrieveMap
}

func installSdks(st *state.State, pid, w string, sdks []sdk.Setup, retrieveTasks map[string]string) *state.TaskSet {
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
		var install *state.Task
		var retrieveTask string
		if setup.Revision.Local() {
			install = sdkstate.InstallLocalSdk(st, setup)
			retrieveTask = install.ID()
		} else {
			retrieveTask = retrieveTasks[setup.Name]
			install = sdkstate.Install(st, setup.Name, retrieveTask)
		}
		addTask(install)

		register := sdkstate.Register(st, setup.Name, retrieveTask)
		addTask(register)

		hook := hookstate.Hook(st, pid, w, setup.Name, 0, hookstate.SetupBase)
		addTask(hook)
	}
	return all
}

func launchWorkshop(st *state.State, file *workshop.File) *state.TaskSet {
	construct := state.NewTaskSet()

	var prev *state.Task
	addTask := func(t *state.Task) {
		construct.AddTask(t)
		if prev != nil {
			t.WaitFor(prev)
		}
		prev = t
	}

	create := st.NewTask("create-workshop", fmt.Sprintf("Create new %q workshop", file.Name))
	addTask(create)
	create.Set("workshop-file", file)

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", file.Name))
	addTask(start)

	return construct
}

func rebuildWorkshop(st *state.State, file *workshop.File, sdkSnapshot string) *state.TaskSet {
	construct := state.NewTaskSet()

	var prev *state.Task
	addTask := func(t *state.Task) {
		construct.AddTask(t)
		if prev != nil {
			t.WaitFor(prev)
		}
		prev = t
	}

	var summary string
	if sdkSnapshot == "" {
		summary = fmt.Sprintf("Rebuild %q workshop", file.Name)
	} else {
		summary = fmt.Sprintf("Restore %q workshop from %q snapshot", file.Name, sdkSnapshot)
	}

	create := st.NewTask("create-workshop", summary)
	addTask(create)
	create.Set("workshop-file", file)

	if sdkSnapshot != "" {
		create.Set("sdk-snapshot", sdkSnapshot)
	}

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", file.Name))
	addTask(start)

	return construct
}

func launch(st *state.State, file *workshop.File, sdks []sdk.Setup, project workshop.Project) *state.TaskSet {
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

	base := retrieveBase(st, file)
	retrieve, rmap := retrieveSdks(st, sdks)
	retrieve.AddTask(base)
	addTaskSet(retrieve)

	createDirs := st.NewTask("create-workshop-storage", fmt.Sprintf("Create %q storage directories", file.Name))
	addTaskSet(state.NewTaskSet(createDirs))

	create := launchWorkshop(st, file)
	addTaskSet(create)

	install := installSdks(st, project.ProjectId, file.Name, sdks, rmap)
	addTaskSet(install)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, file.Name, sdks)
	addTaskSet(connect)

	setupProject := runHooks(st, project.ProjectId, file.Name, sdks, 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	checkHealth := runHooks(st, project.ProjectId, file.Name, sdks, checkHealthTimeout, hookstate.CheckHealth)
	addTaskSet(checkHealth)

	for _, task := range all.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", project)
	}

	return all
}

func (w *WorkshopManager) RefreshMany(ctx context.Context, projectId string, names []string) ([]*state.TaskSet, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key user not found")
	}

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return nil, err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	local := localStore{usr, userDataDir, project}

	reqs, err := w.resolveWorkshops(ctx, project, names, "refresh")
	if err != nil {
		return nil, err
	}

	taskset := make([]*state.TaskSet, 0, len(reqs))
	allowed := []healthstate.Status{healthstate.ReadyStatus}
	for _, req := range reqs {
		name := req.file.Name
		wp, err := w.Workshop(ctx, name, projectId)
		if err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
		}

		// Check for conflicting changes.
		// Has to happen after calling resolveWorkshops (because it unlocks the state).
		if err := healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
		}

		localSdks, err := local.resolveSdks(name, req.file, wp)
		if err != nil {
			return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
		}
		sdks := ordered(req.installOrder, req.storeSdks, localSdks)

		plan := resolveRefresh(wp, req.file, sdks)
		tasks := refresh(w.state, plan, wp, req.file)
		if len(tasks.Tasks()) == 0 {
			continue
		}
		taskset = append(taskset, tasks)
	}

	for _, ts := range taskset {
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
		for _, otherts := range taskset {
			if ts != otherts {
				last, err := otherts.Edge(EdgeRefreshLastTaskBeforeCleanup)
				if err != nil {
					return nil, err
				}
				cleanup.WaitFor(last)
			}
		}
	}

	return taskset, nil
}

func maybeRefresh(installed, candidate sdk.Setup) bool {
	return installed.Channel != candidate.Channel || installed.Revision != candidate.Revision
}

func definitionChanged(old, new *workshop.File, sdkName string) bool {
	byName := func(s workshop.SdkRecord) bool { return s.Name == sdkName }
	oldidx := slices.IndexFunc(old.Sdks, byName)
	newidx := slices.IndexFunc(new.Sdks, byName)

	if oldidx == -1 || newidx == -1 {
		// Check if the SDK definition was added or removed.
		return oldidx != newidx
	}

	oldrec := old.Sdks[oldidx]
	newrec := new.Sdks[newidx]

	return !reflect.DeepEqual(oldrec.Plugs, newrec.Plugs) || !reflect.DeepEqual(oldrec.Slots, newrec.Slots)
}

type refreshPlan struct {
	install []sdk.Setup
	intact  []sdk.Setup
	refresh []sdk.Setup
	remove  []sdk.Setup

	sdkSnapshot    string
	installOrder   []string
	installedOrder []string
}

func (p refreshPlan) InstallOrRefresh() []sdk.Setup {
	return ordered(p.installOrder, p.install, p.refresh)
}

func (p refreshPlan) IntactOrRefresh() []sdk.Setup {
	return ordered(p.installOrder, p.intact, p.refresh)
}

func (p refreshPlan) IntactOrRemove() []sdk.Setup {
	revOrder := slices.Clone(p.installedOrder)
	slices.Reverse(revOrder)
	ordered := ordered(revOrder, p.intact, p.remove)
	return ordered
}

func (p refreshPlan) InstallIntactOrRefresh() []sdk.Setup {
	return ordered(p.installOrder, p.install, p.refresh, p.intact)
}

func (p refreshPlan) Remove() []sdk.Setup {
	revOrder := slices.Clone(p.installedOrder)
	slices.Reverse(revOrder)
	return ordered(revOrder, p.remove)
}

func resolveRefresh(w *workshop.Workshop, newfile *workshop.File, candidates []sdk.Setup) *refreshPlan {
	plan := &refreshPlan{
		install:        make([]sdk.Setup, 0),
		intact:         make([]sdk.Setup, 0),
		refresh:        make([]sdk.Setup, 0),
		remove:         make([]sdk.Setup, 0),
		installOrder:   make([]string, 0),
		installedOrder: make([]string, 0),
	}

	// Restore the order of SDKs installed in the running workshop.
	installed := w.SdksByInstallOrder()

	// Determine if a workshop can be partially updated.
	lastIntactIdx := -1

	if w.Base == newfile.Base {
		for ci, s := range candidates {
			// Do we have this SDK in the same order as in the running workshop?
			if ci < len(installed) && installed[ci].Name == s.Name {
				// Has this SDK had any updates?
				if maybeRefresh(w.Sdks[s.Name], s) {
					break
				}
				// Has this SDK had any changes in the workshop definition?
				if definitionChanged(w.File, newfile, s.Name) {
					break
				}

				plan.intact = append(plan.intact, s)
				// No updates to the SDK - reuse its snapshot and keep looking.
				// Otherwise, break the loop as the rest require to be reinstalled.
				plan.sdkSnapshot = s.Name
				lastIntactIdx = ci
			} else {
				break
			}
		}
	}

	for _, s := range candidates[lastIntactIdx+1:] {
		if installed, exist := w.Sdks[s.Name]; exist {
			plan.refresh = append(plan.refresh, s)
			plan.remove = append(plan.remove, installed)
		} else {
			plan.install = append(plan.install, s)
		}
	}

	// SDKs that only exist in the previous workshop are to be removed.
	for _, rec := range w.Sdks {
		if !slices.ContainsFunc(candidates, func(r sdk.Setup) bool {
			return r.Name == rec.Name
		}) {
			plan.remove = append(plan.remove, rec)
		}
	}

	// Establish SDK order for the installed SDKs.
	for _, s := range installed {
		plan.installedOrder = append(plan.installedOrder, s.Name)
	}

	// Establish SDK installation order.
	for _, s := range candidates {
		plan.installOrder = append(plan.installOrder, s.Name)
	}

	return plan
}

func refresh(st *state.State, plan *refreshPlan, w *workshop.Workshop, file *workshop.File) *state.TaskSet {
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

	var base *state.Task
	if plan.sdkSnapshot == "" {
		// Create download-base first so the task IDs are in a nice order.
		base = retrieveBase(st, file)
	}
	retrieve, rmap := retrieveSdks(st, plan.InstallOrRefresh())
	if base != nil {
		retrieve.AddTask(base)
	}
	addTaskSet(retrieve)

	if len(plan.IntactOrRefresh()) > 0 {
		stateStorage := st.NewTask("create-state-storage", "Create SDK state storage")
		addTaskSet(state.NewTaskSet(stateStorage))
	}

	// Call save-state hooks for the SDKs that are installed and will not be
	// removed after this refresh.
	saveState := runHooks(st, w.Project.ProjectId, file.Name, plan.IntactOrRefresh(), 0, hookstate.SaveState)
	addTaskSet(saveState)

	disconnect := disconnectSdks(st, plan.IntactOrRemove())
	addTaskSet(disconnect)

	// Remove SDKs from interfaces repository. If refresh fails, the SDKs will be returned
	// to the repository after restoring the stashed workshop (with the SDKs installed).
	unregister := unregisterSdks(st, plan.Remove())
	addTaskSet(unregister)

	stash := st.NewTask("stash-workshop", fmt.Sprintf("Stash previous %q workshop", file.Name))
	addTaskSet(state.NewTaskSet(stash))

	rebuild := rebuildWorkshop(st, file, plan.sdkSnapshot)
	addTaskSet(rebuild)

	// Install updated SDKs to the rebuilt workshop.
	install := installSdks(st, w.Project.ProjectId, file.Name, plan.InstallOrRefresh(), rmap)
	addTaskSet(install)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", w.Project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, file.Name, plan.InstallIntactOrRefresh())
	addTaskSet(connect)

	setupProject := runHooks(st, w.Project.ProjectId, file.Name, plan.InstallIntactOrRefresh(), 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	restoreState := runHooks(st, w.Project.ProjectId, file.Name, plan.IntactOrRefresh(), 0, hookstate.RestoreState)
	addTaskSet(restoreState)

	checkHealth := runHooks(st, w.Project.ProjectId, file.Name, plan.InstallIntactOrRefresh(), 0, hookstate.CheckHealth)
	addTaskSet(checkHealth)

	length := len(refresh.Tasks())
	last := refresh.Tasks()[length-1]
	refresh.MarkEdge(last, EdgeRefreshLastTaskBeforeCleanup)

	cleanupLane := st.NewLane()

	if len(plan.IntactOrRefresh()) > 0 {
		removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
		removeStateStorage.WaitFor(last)
		removeStateStorage.JoinLane(cleanupLane)
		refresh.MarkEdge(removeStateStorage, EdgeRefreshFirstCleanupTask)

		refresh.AddTask(removeStateStorage)
	}

	// remove the workshop from stash after the state storage was detached
	removeStash := st.NewTask("remove-workshop-stash", fmt.Sprintf("Remove %q workshop from stash", file.Name))
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
	removeStash.JoinLane(cleanupLane)
	removeStash.WaitFor(last)

	if refresh.MaybeEdge(EdgeRefreshFirstCleanupTask) == nil {
		refresh.MarkEdge(removeStash, EdgeRefreshFirstCleanupTask)
	}

	refresh.AddTask(removeStash)

	for _, task := range refresh.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", w.Project)
	}

	return refresh
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

func unregisterSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	prev := (*state.Task)(nil)
	unregisterSet := state.NewTaskSet()
	for _, s := range sdks {
		unregister := sdkstate.Unregister(st, s)
		unregisterSet.AddTask(unregister)

		if prev != nil {
			unregister.WaitFor(prev)
		}
		prev = unregister
	}
	return unregisterSet
}

func disconnectSdks(st *state.State, sdks []sdk.Setup) *state.TaskSet {
	prev := (*state.Task)(nil)
	disconnSet := state.NewTaskSet()
	for _, s := range sdks {
		disconn := st.NewTask("auto-disconnect", fmt.Sprintf("Disconnect interfaces of %q SDK", s.Name))
		disconn.Set("sdk", s.Name)
		disconnSet.AddTask(disconn)

		if prev != nil {
			disconn.WaitFor(prev)
		}
		prev = disconn
	}
	return disconnSet
}

func runHooks(st *state.State, pid, w string, installed []sdk.Setup, timeout time.Duration, hooktype hookstate.WorkshopHookType) *state.TaskSet {
	hooks := state.NewTaskSet()
	prev := (*state.Task)(nil)
	for _, sk := range installed {
		hook := hookstate.Hook(st, pid, w, sk.Name, timeout, hooktype)
		hooks.AddTask(hook)
		if prev != nil {
			hook.WaitFor(prev)
		}
		prev = hook
	}
	return hooks
}

func (w *WorkshopManager) StartMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	// check if all the workshops are stopped
	for _, name := range names {
		wp, err := w.Workshop(ctx, name, projectId)
		if err != nil {
			return nil, fmt.Errorf("cannot start %q: %w", name, err)
		}
		allowed := []healthstate.Status{healthstate.StoppedStatus}
		if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
			return nil, fmt.Errorf("cannot start %q: %w", name, err)
		}
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := startMany(w.state, names, project)
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

func (w *WorkshopManager) StopMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	for _, name := range names {
		wp, err := w.Workshop(ctx, name, projectId)
		if err != nil {
			return nil, fmt.Errorf("cannot stop %q: %w", name, err)
		}
		allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.StoppedStatus}
		if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
			return nil, fmt.Errorf("cannot stop %q: %w", name, err)
		}
	}

	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}
	taskset, err := stopMany(w.state, names, project)
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
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)
	wp, err := w.backend.Workshop(ctx, name)
	if err != nil {
		return nil, err
	}
	allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.WaitingStatus}
	if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
		return nil, err
	}

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

func (w *WorkshopManager) RemoveMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)

	var workshops = make([]*workshop.Workshop, 0, len(names))
	for _, name := range names {
		wp, err := w.backend.Workshop(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}
		workshops = append(workshops, wp)
		if err = conflict.BackgroundDiscardWaitingRefresh(w.state, name, projectId); err != nil {
			return nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}

		allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.ErrorStatus, healthstate.StoppedStatus}
		if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
			return nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}
	}

	taskset := []*state.TaskSet{}
	for _, name := range workshops {
		remove := remove(w.state, name, project)
		taskset = append(taskset, remove)
	}
	return taskset, nil
}

func remove(st *state.State, w *workshop.Workshop, project workshop.Project) *state.TaskSet {
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

	disconnectSet := disconnectSdks(st, slices.Collect(maps.Values(w.Sdks)))
	addTaskSet(disconnectSet)

	discard := st.NewTask("discard-conns", fmt.Sprintf("Discard %q undesired connections", w.Name))
	addTaskSet(state.NewTaskSet(discard))

	unregister := unregisterSdks(st, slices.Collect(maps.Values(w.Sdks)))
	addTaskSet(unregister)

	remove := st.NewTask("remove-workshop", fmt.Sprintf("Remove %q workshop", w.Name))
	addTaskSet(state.NewTaskSet(remove))

	removeStateStorage := st.NewTask("remove-state-storage", "Remove SDK state storage")
	addTaskSet(state.NewTaskSet(removeStateStorage))

	removeStash := st.NewTask("remove-workshop-stash", fmt.Sprintf("Remove %q workshop from stash", w.Name))
	addTaskSet(state.NewTaskSet(removeStash))

	removeDirs := st.NewTask("remove-workshop-storage", fmt.Sprintf("Remove %q storage directories", w.Name))
	addTaskSet(state.NewTaskSet(removeDirs))

	// Directories should exist from before create-workshop until after remove-workshop.
	// Since there is no way to undo remove-workshop, we run remove-workshop-storage in a separate lane.
	// If an error occurs when removing the directories, it will not affect the other tasks.
	cleanupLane := st.NewLane()
	removeDirs.JoinLane(cleanupLane)

	for _, task := range removeSet.Tasks() {
		task.Set("workshop", w.Name)
		task.Set("project", project)
	}
	return removeSet
}

func (w *WorkshopManager) Remount(ctx context.Context, st *state.State, plug sdk.PlugRef, source string) (*state.TaskSet, error) {
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("cannot remount: the `source` path must be absolute")
	}

	source = filepath.Clean(source)

	project, err := w.loadProject(ctx, plug.ProjectId)
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
