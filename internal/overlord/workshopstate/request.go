package workshopstate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/arch"
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
	finder := localSdkFinder{
		backend:     w.backend,
		user:        usr,
		userDataDir: userDataDir,
		project:     project,
	}

	reqs, err := w.findRemoteArtifacts(ctx, project, names, "launch")
	if err != nil {
		return nil, err
	}

	taskset := make([]*state.TaskSet, 0, len(names))
	for _, req := range reqs {
		name := req.file.Name
		// Make sure the workshop doesn't exist.
		// Has to happen after calling findRemoteArtifacts (because it unlocks the state).
		_, err := w.Workshop(ctx, name, projectId)
		if err == nil {
			return nil, fmt.Errorf("cannot launch %q: workshop exists", name)
		} else if !errors.Is(err, workshop.ErrWorkshopNotLaunched) {
			return nil, fmt.Errorf("cannot launch %q, failed to check whether the workshop exists: %w", name, err)
		}
		if err := conflict.CheckChangeConflict(w.state, projectId, name, nil); err != nil {
			return nil, fmt.Errorf("cannot launch %q, other changes in progress: %w", name, err)
		}

		localSdks, err := finder.findLocalSdks(ctx, nil, req.file)
		if err != nil {
			return nil, fmt.Errorf("cannot launch %q: %w", name, err)
		}
		localSetups, err := validateSdkMeta(projectId, req.file, localSdks)
		if err != nil {
			return nil, fmt.Errorf("cannot launch %q: %w", name, err)
		}
		sdks := ordered(req.installOrder, req.storeSdks, localSetups)

		tasks := launch(w.state, req.file, req.fileText, req.image, sdks, project)
		taskset = append(taskset, tasks)
	}
	return taskset, nil
}

type workshopReq struct {
	// Up to date workshop definitions from the project directory.
	file *workshop.File
	// Marshalled file (to prevent data loss when passing through the state).
	fileText string
	// Up to date base image.
	image workshop.BaseImage
	// All possible SDKs (including sketch) in installation order.
	installOrder []string
	// Up to date SDK setups from the store.
	storeSdks []sdk.Setup
}

func (w *WorkshopManager) findRemoteArtifacts(ctx context.Context, project workshop.Project, names []string, action string) ([]workshopReq, error) {
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

		fileBlob, err := yaml.Marshal(file)
		if err != nil {
			return nil, fmt.Errorf("cannot %s %q: invalid workshop file: %w", action, name, err)
		}

		image, err := w.backend.GetBase(ctx, file.Base)
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

		sdks, err := findStoreSdks(sto, ctx, project.ProjectId, file)
		if err != nil {
			return nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		setups, err := validateSdkMeta(project.ProjectId, file, sdks)
		if err != nil {
			return nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		req := workshopReq{
			file:         file,
			fileText:     string(fileBlob),
			image:        image,
			installOrder: installOrder,
			storeSdks:    setups,
		}
		reqs = append(reqs, req)
	}

	return reqs, nil
}

func findStoreSdks(sto sdk.Store, ctx context.Context, projectid string, file *workshop.File) ([]sdk.Meta, error) {
	systemMeta, err := system.SystemSdkMeta()
	if err != nil {
		return nil, err
	}

	acts := []sdk.SdkAction{}
	for _, sd := range file.Sdks {
		if sd.Source != sdk.StoreSource {
			continue
		}
		act := sdk.SdkAction{ProjectId: projectid, Workshop: file.Name, Name: sd.Name, Base: file.Base, Channel: sd.Channel, Action: sdk.Install}
		acts = append(acts, act)
	}

	sdks, err := sto.SdkAction(ctx, acts)
	if err != nil {
		return nil, err
	}

	sdks = slices.Insert(sdks, 0, *systemMeta)
	return sdks, nil
}

type localSdkFinder struct {
	backend     workshop.Backend
	user        *user.User
	userDataDir string
	project     workshop.Project
	sdkVolumes  []workshop.VolumeInfo
}

func (s *localSdkFinder) findLocalSdks(ctx context.Context, wp *workshop.Workshop, file *workshop.File) ([]sdk.Meta, error) {
	localSdks := []sdk.Meta{}

	for _, sd := range file.Sdks {
		meta, err := s.maybeFindLocalSdk(ctx, wp, file, sd)
		if err != nil {
			return nil, err
		}
		if meta != nil {
			localSdks = append(localSdks, *meta)
		}
	}

	meta, err := s.maybeFindSketchSdk(wp, file.Name)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		localSdks = append(localSdks, *meta)
	}

	return localSdks, nil
}

func (s *localSdkFinder) maybeFindLocalSdk(ctx context.Context, wp *workshop.Workshop, file *workshop.File, sd workshop.SdkRecord) (*sdk.Meta, error) {
	switch sd.Source {
	case sdk.TrySource:
		return s.findTrySdk(ctx, file.Base, sd.Name)
	case sdk.ProjectSource:
		return s.findInProjectSdk(wp, file.Name, sd.Name)
	default:
		return nil, nil
	}
}

func (s *localSdkFinder) findTrySdk(ctx context.Context, base, sk string) (*sdk.Meta, error) {
	trydir := workshop.TrySdkDir(s.userDataDir, sk)
	root, err := os.OpenRoot(trydir)
	if err != nil {
		return nil, fmt.Errorf("SDK %q not found: %w", "try-"+sk, err)
	}
	defer root.Close()

	file, filename, err := findTrySdkFile(root, sk, arch.DpkgArchitecture(), base)
	if err != nil {
		return nil, fmt.Errorf("SDK %q not found: %w", "try-"+sk, err)
	}
	defer file.Close()

	digest, meta, err := readTrySdkMetadata(root, filename)
	if err != nil {
		return nil, fmt.Errorf("invalid SDK %q: %w", "try-"+sk, err)
	}

	volumes, err := s.volumes(ctx)
	if err != nil {
		return nil, err
	}

	minRevision := sdk.Revision{}
	for _, volume := range volumes {
		if volume.Sdk != sk || !volume.Revision.Local() {
			continue
		}
		if volume.Revision.N < minRevision.N {
			minRevision = volume.Revision
		}

		if volume.Sha3_384 == digest {
			setup := sdk.Setup{
				Name:     volume.Sdk,
				Source:   sdk.TrySource,
				Revision: volume.Revision,
				Sha3_384: volume.Sha3_384,
			}
			return &sdk.Meta{Setup: setup, SdkYAML: volume.Metadata}, nil
		}
	}

	revision := sdk.Revision{N: minRevision.N - 1}
	volume := workshop.VolumeSetup{
		Name:     sdk.VolumeName(sk, revision),
		Kind:     "sdk",
		Sha3_384: digest,
		Sdk:      sk,
		Revision: revision,
		Metadata: meta,
	}
	if err := s.importVolume(ctx, volume, file); err != nil {
		return nil, err
	}
	setup := sdk.Setup{
		Name:     volume.Sdk,
		Source:   sdk.TrySource,
		Revision: volume.Revision,
		Sha3_384: volume.Sha3_384,
	}
	return &sdk.Meta{Setup: setup, SdkYAML: volume.Metadata}, nil
}

func (s *localSdkFinder) volumes(ctx context.Context) ([]workshop.VolumeInfo, error) {
	if s.sdkVolumes != nil {
		// The state is locked, preventing other launches and refreshes
		// from calling ImportVolume, so it's safe to reuse sdkVolumes.
		return s.sdkVolumes, nil
	}

	vols, err := s.backend.Volumes(ctx, "sdk")
	if err != nil {
		return nil, err
	}
	if vols == nil {
		vols = []workshop.VolumeInfo{}
	}
	s.sdkVolumes = vols
	return vols, nil
}

func (s *localSdkFinder) importVolume(ctx context.Context, setup workshop.VolumeSetup, tarball *os.File) error {
	if err := s.backend.ImportVolume(ctx, setup, tarball); err != nil {
		return err
	}
	s.sdkVolumes = append(s.sdkVolumes, workshop.VolumeInfo{VolumeSetup: setup, Workshops: make(map[string][]string)})
	return nil
}

func findTrySdkFile(root *os.Root, sk, arch, base string) (*os.File, string, error) {
	filenames := []string{
		fmt.Sprintf("%s_%s_%s.sdk", sk, arch, base),
		fmt.Sprintf("%s_%s.sdk", sk, arch),
		fmt.Sprintf("%s_all_%s.sdk", sk, base),
		fmt.Sprintf("%s_all.sdk", sk),
	}
	var firstErr error
	for _, filename := range filenames {
		file, err := root.Open(filename)
		if err == nil {
			return file, filename, nil
		}
		firstErr = cmp.Or(firstErr, err)
	}
	return nil, "", prependRootPath(root, firstErr)
}

func readTrySdkMetadata(root *os.Root, filename string) (string, string, error) {
	content, err := root.ReadFile(filename + ".sha3-384")
	if err != nil {
		return "", "", prependRootPath(root, err)
	}
	digest := strings.TrimSpace(string(content))

	content, err = root.ReadFile(filename + ".yaml")
	if err != nil {
		return "", "", prependRootPath(root, err)
	}
	meta := string(content)

	return digest, meta, nil
}

func prependRootPath(root *os.Root, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		pathErr.Path = filepath.Join(root.Name(), pathErr.Path)
	}
	return err
}

func (s *localSdkFinder) findInProjectSdk(wp *workshop.Workshop, w, sk string) (*sdk.Meta, error) {
	sdkdir := workshop.ProjectSdkPath(s.project.Path, sk)
	return s.commitRevision(wp, w, sk, sdk.ProjectSource, sdkdir)
}

func (s *localSdkFinder) maybeFindSketchSdk(wp *workshop.Workshop, w string) (*sdk.Meta, error) {
	sketchdir := workshop.SketchSdkCurrent(s.userDataDir, s.project.ProjectId, w)

	recs, err := os.ReadDir(sketchdir)
	// no Sketch SDK exists for the workshop and it is not an error.
	if (err == nil && len(recs) == 0) || osutil.IsDirNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if wp == nil {
		// Old sketches might exist because the snap remove hook doesn't remove them.
		// No workshop exists currently; including the sketch SDK would be unexpected.
		// We remove it (but keep the stash) to prevent future refreshes from including it.
		if err = os.RemoveAll(sketchdir); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return s.commitRevision(wp, w, sdk.Sketch, sdk.SketchSource, sketchdir)
}

func (s *localSdkFinder) commitRevision(wp *workshop.Workshop, w, sk string, source sdk.Source, path string) (*sdk.Meta, error) {
	var installed sdk.Revision
	if wp != nil {
		installed = wp.Sdks[sk].Revision
	}

	target := workshop.LocalSdkDir(s.userDataDir, s.project.ProjectId, w, sk)
	revision, digest, err := sdk.CommitRevision(s.user, path, target, installed)
	if err != nil {
		return nil, err
	}

	sdkYaml, err := os.ReadFile(filepath.Join(target, digest, "meta", "sdk.yaml"))
	if err != nil {
		return nil, fmt.Errorf("invalid %q SDK: %w", sk, err)
	}

	setup := sdk.Setup{
		Name:     sk,
		Source:   source,
		Revision: revision,
		Sha3_384: digest,
	}
	return &sdk.Meta{Setup: setup, SdkYAML: string(sdkYaml)}, nil
}

func validateSdkMeta(projectId string, file *workshop.File, sdks []sdk.Meta) ([]sdk.Setup, error) {
	setups := make([]sdk.Setup, 0, len(sdks))
	for _, s := range sdks {
		if s.Sha3_384 == "" {
			return nil, fmt.Errorf("internal error: hash not found for %q SDK", s.Name)
		}
		if err := workshop.ValidateSdkInfo(projectId, file, s.Name, s.SdkYAML); err != nil {
			return nil, err
		}
		setups = append(setups, s.Setup)
	}
	return setups, nil
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

func retrieveBase(st *state.State, image workshop.BaseImage) *state.Task {
	download := st.NewTask("download-base", fmt.Sprintf("Download %q base image", image.Name))
	download.Set("workshop-base", image)
	return download
}

func retrieveSdks(st *state.State, sdks []sdk.Setup) (*state.TaskSet, map[string]string) {
	retrieve := state.NewTaskSet()
	retrieveMap := map[string]string{}
	for _, s := range sdks {
		if s.Source.NeedsRetrieve() {
			r := sdkstate.Retrieve(st, s)
			retrieve.AddTask(r)
			retrieveMap[s.Name] = r.ID()
		}
	}
	return retrieve, retrieveMap
}

func installSdks(st *state.State, sdks []sdk.Setup, retrieveTasks map[string]string) *state.TaskSet {
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
		if setup.Source.NeedsRetrieve() {
			retrieveTask = retrieveTasks[setup.Name]
			install = sdkstate.Install(st, setup.Name, retrieveTask)
		} else {
			install = sdkstate.InstallLocalSdk(st, setup)
			retrieveTask = install.ID()
		}
		addTask(install)

		register := sdkstate.Register(st, setup.Name, retrieveTask)
		addTask(register)

		hook := hookstate.Hook(st, setup.Name, 0, hookstate.SetupBase)
		addTask(hook)

		snapshot := sdkstate.Snapshot(st, setup.Name)
		addTask(snapshot)
	}
	return all
}

func launchWorkshop(st *state.State, name string, fileText string, image workshop.BaseImage) *state.TaskSet {
	construct := state.NewTaskSet()

	var prev *state.Task
	addTask := func(t *state.Task) {
		construct.AddTask(t)
		if prev != nil {
			t.WaitFor(prev)
		}
		prev = t
	}

	create := st.NewTask("create-workshop", fmt.Sprintf("Create new %q workshop", name))
	addTask(create)
	create.Set("workshop-file", fileText)
	create.Set("workshop-base", image)

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
	addTask(start)

	return construct
}

func rebuildWorkshop(st *state.State, name string, fileText string, sdkSnapshot string, image workshop.BaseImage) *state.TaskSet {
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
		summary = fmt.Sprintf("Rebuild %q workshop", name)
	} else {
		summary = fmt.Sprintf("Restore %q workshop from %q snapshot", name, sdkSnapshot)
	}

	create := st.NewTask("rebuild-workshop", summary)
	addTask(create)
	create.Set("workshop-file", fileText)

	if sdkSnapshot != "" {
		create.Set("sdk-snapshot", sdkSnapshot)
	} else {
		create.Set("workshop-base", image)
	}

	start := st.NewTask("start-workshop", fmt.Sprintf("Start %q workshop", name))
	addTask(start)

	return construct
}

func launch(st *state.State, file *workshop.File, fileText string, image workshop.BaseImage, sdks []sdk.Setup, project workshop.Project) *state.TaskSet {
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

	base := retrieveBase(st, image)
	retrieve, rmap := retrieveSdks(st, sdks)
	retrieve.AddTask(base)
	addTaskSet(retrieve)

	createDirs := st.NewTask("create-workshop-storage", fmt.Sprintf("Create %q storage directories", file.Name))
	addTaskSet(state.NewTaskSet(createDirs))

	create := launchWorkshop(st, file.Name, fileText, image)
	addTaskSet(create)

	install := installSdks(st, sdks, rmap)
	addTaskSet(install)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, file.Name, sdks)
	addTaskSet(connect)

	setupProject := runHooks(st, sdks, 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	checkHealth := runHooks(st, sdks, checkHealthTimeout, hookstate.CheckHealth)
	addTaskSet(checkHealth)

	for _, task := range all.Tasks() {
		task.Set("workshop", file.Name)
		task.Set("project", project)
	}

	return all
}

func (w *WorkshopManager) RefreshMany(ctx context.Context, projectId string, names []string, option conflict.RefreshOption) ([]*state.TaskSet, error) {
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

	taskset := make([]*state.TaskSet, 0, len(names))
	allowed := []healthstate.Status{healthstate.ReadyStatus}

	switch option {
	case conflict.RefreshUpdate:
		reqs, err := w.findRemoteArtifacts(ctx, project, names, "refresh")
		if err != nil {
			return nil, err
		}

		finder := localSdkFinder{
			backend:     w.backend,
			user:        usr,
			userDataDir: userDataDir,
			project:     project,
		}

		for _, req := range reqs {
			name := req.file.Name
			wp, err := w.Workshop(ctx, name, projectId)
			if err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}

			// Check for conflicting changes. Has to happen after calling
			// findRemoteArtifacts (because it unlocks the state).
			if err := healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}

			localSdks, err := finder.findLocalSdks(ctx, wp, req.file)
			if err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}
			localSetups, err := validateSdkMeta(projectId, req.file, localSdks)
			if err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}
			sdks := ordered(req.installOrder, req.storeSdks, localSetups)

			plan := resolveRefresh(wp, req.file, req.image, sdks)
			if plan.HasUpdates() {
				tasks := refresh(w.state, plan, wp, req.file, req.fileText)
				taskset = append(taskset, tasks)
			}
		}
	case conflict.RefreshRestore:
		for _, name := range names {
			wp, err := w.Workshop(ctx, name, projectId)
			if err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}

			if err = healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
				return nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}

			fileBlob, err := yaml.Marshal(wp.File)
			if err != nil {
				return nil, fmt.Errorf("cannot refresh %q: invalid workshop file: %w", name, err)
			}

			installed := wp.SdksByInstallOrder()
			sdks := make([]sdk.Setup, 0, len(installed))
			for _, sk := range installed {
				sdks = append(sdks, sk.Setup)
			}

			plan := resolveRefresh(wp, wp.File, wp.Image, sdks)
			tasks := refresh(w.state, plan, wp, wp.File, string(fileBlob))
			taskset = append(taskset, tasks)
		}
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
	return installed.Source != candidate.Source || installed.Channel != candidate.Channel || installed.Revision != candidate.Revision
}

type refreshPlan struct {
	image workshop.BaseImage

	install []sdk.Setup
	intact  []sdk.Setup
	refresh []sdk.Setup
	remove  []sdk.Setup

	sdkSnapshot    string
	installOrder   []string
	installedOrder []string

	// Indicates if the Workshop definition was updated, i.e. if any new plugs,
	// slots or connections were added.
	workshopDefinitionUpdated bool
}

func (p refreshPlan) InstallOrRefresh() []sdk.Setup {
	return ordered(p.installOrder, p.install, p.refresh)
}

func (p refreshPlan) Intact() []sdk.Setup {
	return ordered(p.installOrder, p.intact)
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

func (p refreshPlan) HasUpdates() bool {
	return len(p.InstallOrRefresh()) > 0 || len(p.remove) > 0 || p.workshopDefinitionUpdated
}

func resolveRefresh(w *workshop.Workshop, newfile *workshop.File, image workshop.BaseImage, candidates []sdk.Setup) *refreshPlan {
	plan := &refreshPlan{
		image:          image,
		install:        make([]sdk.Setup, 0),
		intact:         make([]sdk.Setup, 0),
		refresh:        make([]sdk.Setup, 0),
		remove:         make([]sdk.Setup, 0),
		installOrder:   make([]string, 0),
		installedOrder: make([]string, 0),
	}

	// Restore the order of SDKs installed in the running workshop.
	installed := w.SdksByInstallOrder()

	if w.Image == image {
		for ci, s := range candidates {
			// Do we have this SDK in the same order as in the running workshop?
			if ci >= len(installed) || installed[ci].Name != s.Name {
				break
			}
			// Has this SDK had any updates?
			// If so, break the loop as the rest require to be reinstalled.
			if maybeRefresh(w.Sdks[s.Name].Setup, s) {
				break
			}

			plan.intact = append(plan.intact, s)
			// No updates to the SDK - reuse its snapshot and keep looking.
			plan.sdkSnapshot = s.Name
		}
	}

	for _, s := range candidates[len(plan.intact):] {
		installed, exist := w.Sdks[s.Name]
		if exist {
			plan.refresh = append(plan.refresh, s)
			plan.remove = append(plan.remove, installed.Setup)
		} else {
			plan.install = append(plan.install, s)
		}
	}

	// SDKs that only exist in the previous workshop are to be removed.
	for _, s := range w.Sdks {
		if !slices.ContainsFunc(candidates, func(c sdk.Setup) bool {
			return c.Name == s.Name
		}) {
			plan.remove = append(plan.remove, s.Setup)
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

	plan.workshopDefinitionUpdated = !reflect.DeepEqual(w.File.Sdks, newfile.Sdks) ||
		!reflect.DeepEqual(w.File.Connections, newfile.Connections)

	return plan
}

func refresh(st *state.State, plan *refreshPlan, w *workshop.Workshop, file *workshop.File, fileText string) *state.TaskSet {
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
		base = retrieveBase(st, plan.image)
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
	saveState := runHooks(st, plan.IntactOrRefresh(), 0, hookstate.SaveState)
	addTaskSet(saveState)

	disconnect := disconnectSdks(st, plan.IntactOrRemove())
	addTaskSet(disconnect)

	// Remove SDKs from interfaces repository. If refresh fails, the SDKs will be returned
	// to the repository after restoring the stashed workshop (with the SDKs installed).
	unregister, umap := unregisterSdks(st, plan.IntactOrRemove())
	addTaskSet(unregister)

	stash := st.NewTask("stash-workshop", fmt.Sprintf("Stash previous %q workshop", file.Name))
	addTaskSet(state.NewTaskSet(stash))

	rebuild := rebuildWorkshop(st, file.Name, fileText, plan.sdkSnapshot, plan.image)
	addTaskSet(rebuild)

	// Re-register intact SDKs (the workshop definition can change plugs and slots).
	register := registerSdks(st, plan.Intact(), umap)
	addTaskSet(register)

	// Install updated SDKs to the rebuilt workshop.
	install := installSdks(st, plan.InstallOrRefresh(), rmap)
	addTaskSet(install)

	mountProject := st.NewTask("mount-project", fmt.Sprintf("Mount project directory %q", w.Project.Path))
	addTaskSet(state.NewTaskSet(mountProject))

	connect := autoconnectSdks(st, file.Name, plan.InstallIntactOrRefresh())
	addTaskSet(connect)

	setupProject := runHooks(st, plan.InstallIntactOrRefresh(), 0, hookstate.SetupProject)
	addTaskSet(setupProject)

	restoreState := runHooks(st, plan.IntactOrRefresh(), 0, hookstate.RestoreState)
	addTaskSet(restoreState)

	checkHealth := runHooks(st, plan.InstallIntactOrRefresh(), 0, hookstate.CheckHealth)
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

func registerSdks(st *state.State, sdks []sdk.Setup, retrieveTasks map[string]string) *state.TaskSet {
	prev := (*state.Task)(nil)
	registerSet := state.NewTaskSet()
	for _, s := range sdks {
		register := sdkstate.Register(st, s.Name, retrieveTasks[s.Name])
		registerSet.AddTask(register)

		if prev != nil {
			register.WaitFor(prev)
		}
		prev = register
	}
	return registerSet
}

func unregisterSdks(st *state.State, sdks []sdk.Setup) (*state.TaskSet, map[string]string) {
	prev := (*state.Task)(nil)
	unregisterSet := state.NewTaskSet()
	unregisterMap := map[string]string{}
	for _, s := range sdks {
		unregister := sdkstate.Unregister(st, s)
		unregisterSet.AddTask(unregister)
		unregisterMap[s.Name] = unregister.ID()

		if prev != nil {
			unregister.WaitFor(prev)
		}
		prev = unregister
	}
	return unregisterSet, unregisterMap
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

func (w *WorkshopManager) Exec(ctx context.Context, name, projectId string, args *workshop.ExecArgs, action bool) (*state.TaskSet, error) {
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

func (w *WorkshopManager) RemoveMany(ctx context.Context, names []string, projectId string) ([]*state.TaskSet, error) {
	project, err := w.loadProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)

	workshops := make([]*workshop.Workshop, 0, len(names))
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

	sdks := make([]sdk.Setup, 0, len(w.Sdks))
	for _, sk := range w.Sdks {
		sdks = append(sdks, sk.Setup)
	}

	disconnectSet := disconnectSdks(st, sdks)
	addTaskSet(disconnectSet)

	discard := st.NewTask("discard-conns", fmt.Sprintf("Discard %q undesired connections", w.Name))
	addTaskSet(state.NewTaskSet(discard))

	unregister, _ := unregisterSdks(st, sdks)
	addTaskSet(unregister)

	remove := st.NewTask("remove-workshop", fmt.Sprintf("Remove %q workshop", w.Name))
	addTaskSet(state.NewTaskSet(remove))

	// The point of no return starts after the workshop is removed. If any of the tasks
	// after this fails, we can only report the error, but cannot undo the removal.
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
	removeStash.JoinLane(cleanupLane)
	removeStateStorage.JoinLane(cleanupLane)

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
