package workshopstate_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"golang.org/x/exp/maps"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type requestSuite struct {
	state   *state.State
	project *workshop.Project
	backend workshop.Backend
	mgr     *workshopstate.WorkshopManager
	ctx     context.Context
}

var _ = check.Suite(&requestSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *requestSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.ctx = context.WithValue(context.Background(), workshop.ContextUser, "testuser")

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.mgr = workshopstate.New(s.state, state.NewTaskRunner(s.state))
	s.project, _, _ = s.backend.CreateOrLoadProject(s.ctx, c.MkDir())
	s.ctx = context.WithValue(s.ctx, workshop.ContextProjectId, s.project.ProjectId)
}

var workshopTemplate = `name: %s
base: ubuntu@20.04
sdks:
  {{ range . }}
  {{- .Name}}:
      channel: {{.Channel}}
  {{ end }} 
`

var sdkTemplate = `name: %s
base: ubuntu@20.04
`

func (s *requestSuite) writeSDKMetaFile(c *check.C, fs workshop.WorkshopFs, name, yaml string) {
	sdkPath := filepath.Join(dirs.WorkshopSdksDir, name, "0", "meta")
	c.Assert(fs.MkdirAll(sdkPath, 0755), check.IsNil)
	metaPath := filepath.Join(sdkPath, "sdk.yaml")
	c.Assert(afero.WriteFile(fs, metaPath, []byte(yaml), 0644), check.IsNil)
}

func (s *requestSuite) launchWorkshopWithSDKs(c *check.C, ws string, sdks workshop.SdkList) *workshop.Workshop {
	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	var workshopFile = bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	path := workshop.Filepath(s.project.Path, ws)

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, workshopFile.Bytes(), 0644)
	c.Assert(err, check.IsNil)

	wf := workshop.File{Name: ws, Base: "ubuntu@20.04", Sdks: sdks}
	err = s.backend.LaunchWorkshop(s.ctx, &wf)
	c.Assert(err, check.IsNil)

	w, err := s.backend.Workshop(s.ctx, ws)
	c.Assert(err, check.IsNil)

	wfs, err := s.backend.WorkshopFs(s.ctx, w.Name)
	c.Assert(err, check.IsNil)

	for _, sd := range sdks {
		s.writeSDKMetaFile(c, wfs, sd.Name, fmt.Sprintf(sdkTemplate, sd.Name))
		c.Assert(w.LinkSdk(s.ctx, sdk.Setup{Name: sd.Name, Channel: sd.Channel}), check.IsNil)
	}

	return w
}

func (s *requestSuite) ensureTaskHasWorkshopAndProjectKeys(c *check.C, w string, ts []*state.Task) {
	for _, i := range ts {
		var prj workshop.Project
		err := i.Get("project", &prj)
		c.Assert(err, check.IsNil)
		c.Assert(&prj, check.DeepEquals, s.project)

		var workshop string
		err = i.Get("workshop", &workshop)
		c.Assert(err, check.IsNil)
		c.Assert(workshop, check.Equals, w)
	}
}

func verifyExpectedTasks(c *check.C, ts []*state.Task, expected []string) {
	actual := make([]string, 0, len(ts))
	for _, i := range ts {
		actual = append(actual, i.Kind())
	}

	c.Assert(actual, testutil.DeepUnsortedMatches, expected)
}

func verifyDisconnectDependencies(c *check.C, ts *state.TaskSet) {
	prev := (*state.Task)(nil)
	for _, t := range ts.Tasks() {
		if t.Kind() == "auto-disconnect" {
			if prev != nil {
				c.Assert(t.WaitTasks(), testutil.Contains, prev)
			}
			prev = t
		}
	}
}

func validateStateHooksTasksSetup(c *check.C, ts []*state.TaskSet, expectedSave, expectedRestore []string) {
	obtainedSave := []string{}
	obtainedRestore := []string{}
	for _, t := range ts[0].Tasks() {
		if t.Kind() == "run-hook" {
			var setup hookstate.HookSetup
			err := t.Get("hook-setup", &setup)
			c.Assert(err, check.IsNil)
			switch setup.HookType {
			case hookstate.SaveState:
				obtainedSave = append(obtainedSave, setup.Sdk)
			case hookstate.RestoreState:
				obtainedRestore = append(obtainedRestore, setup.Sdk)
			}
		}
	}

	// the save state shall be called only for the previously installed SDK
	c.Assert(obtainedSave, testutil.DeepUnsortedMatches, expectedSave)
	// the restore state shall be called for the new previously installed SDK
	c.Assert(obtainedRestore, testutil.DeepUnsortedMatches, expectedRestore)
}

func (s *requestSuite) TestLaunchWorkshopNoSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()
	file := &workshop.File{Name: "test", Base: "ubuntu@22.04"}
	ts := workshopstate.Launch(s.state, file, nil, s.project)

	expected := []string{"download-base", "create-workshop",
		"mount-project",
		"start-workshop", "install-local-sdk", "link-sdk", "auto-connect"}
	tasks := ts.Tasks()

	verifyExpectedTasks(c, tasks, expected)

	var wf workshop.File
	err := tasks[1].Get("workshop-file", &wf)
	c.Assert(err, check.Equals, nil)
	c.Assert(&wf, check.DeepEquals, file)
	s.ensureTaskHasWorkshopAndProjectKeys(c, "test", ts.Tasks())
}

func (s *requestSuite) TestRefreshEmptyWorkshop(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{}}

	ts, err := workshopstate.Refresh(s.state, file, []sdk.Setup{}, []sdk.Setup{}, s.project)
	c.Assert(err, check.IsNil)

	expected := []string{
		"create-state-storage",
		"remove-state-storage",
		"download-base",
		"create-workshop",
		"remove-workshop-stash",
		"stash-workshop",
		"mount-project",
		"start-workshop",
		"install-local-sdk",
		"link-sdk",
		"auto-connect",    // "system" SDK
		"auto-disconnect", // "system" SDK
	}

	tasks := ts.Tasks()

	c.Assert(err, check.Equals, nil)
	verifyExpectedTasks(c, tasks, expected)

	s.ensureTaskHasWorkshopAndProjectKeys(c, "ws", tasks)
}

func (s *requestSuite) TestRefreshWorkshopWithSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/stable"}}}

	installed := []sdk.Setup{
		{Name: "sdk-1", Channel: "latest/stable"},
		{Name: "sdk-2", Channel: "latest/stable"},
	}
	toinst := installed
	ts, err := workshopstate.Refresh(s.state, file, installed, toinst, s.project)
	c.Assert(err, check.IsNil)

	expected := []string{
		"retrieve-sdk",
		"retrieve-sdk",
		"create-state-storage",
		"run-hook",        // save-state sdk-1
		"run-hook",        // save-state sdk-2
		"auto-disconnect", // "system" SDK
		"auto-disconnect",
		"auto-disconnect",
		"stash-workshop",
		"download-base",
		"create-workshop",
		"mount-project",
		"start-workshop",
		"install-local-sdk",
		"link-sdk",
		"install-sdk",
		"link-sdk",
		"install-sdk",
		"link-sdk",
		"auto-connect", // "system" SDK
		"auto-connect",
		"auto-connect",
		"run-hook", // setup-base sdk-1
		"run-hook", // setup-base sdk-2
		"run-hook", // restore-state sdk-1
		"run-hook", // restore-state sdl-2
		"run-hook", // check-health sdk-1
		"run-hook", // check-health sdk-2
		"remove-workshop-stash",
		"remove-state-storage",
	}

	tasks := ts.Tasks()

	c.Assert(err, check.Equals, nil)
	verifyExpectedTasks(c, tasks, expected)
	verifyDisconnectDependencies(c, ts)

	s.ensureTaskHasWorkshopAndProjectKeys(c, "ws", tasks)
}

func (s *requestSuite) TestRefreshSdkRemoved(c *check.C) {
	// Setup
	s.state.Lock()
	defer s.state.Unlock()
	existingSdks := []sdk.Setup{
		{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/stable"},
	}
	toinst := []sdk.Setup{
		{Name: "sdk-1", Channel: "latest/stable"},
	}

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{{Name: "sdk-1", Channel: "latest/stable"}}}

	s.launchWorkshopWithSDKs(c, "ws", []workshop.SdkRecord{
		{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/stable"},
	})

	// Execute
	ts, err := workshopstate.Refresh(s.state, file, existingSdks, toinst, s.project)
	c.Assert(err, check.IsNil)

	// Validate
	// save-state and restore-state will only exist for sdk-1
	validateStateHooksTasksSetup(c, []*state.TaskSet{ts}, []string{"sdk-1"}, []string{"sdk-1"})
	verifyDisconnectDependencies(c, ts)
}

func (s *requestSuite) TestRefreshSdkRemovedMakingWorkshopEmpty(c *check.C) {
	// Setup
	s.state.Lock()
	defer s.state.Unlock()
	existingSdks := workshop.SdkList{
		{Name: "sdk-1", Channel: "latest/stable"},
		{Name: "sdk-2", Channel: "latest/stable"},
	}
	toinst := []sdk.Setup{}

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04"}

	w := s.launchWorkshopWithSDKs(c, "ws", existingSdks)

	// Execute
	ts, err := workshopstate.Refresh(s.state, file, maps.Values(w.Content), toinst, s.project)
	c.Assert(err, check.IsNil)

	// Validate
	// save-state and restore-state will only exist for sdk-1
	validateStateHooksTasksSetup(c, []*state.TaskSet{ts}, []string{}, []string{})
	verifyDisconnectDependencies(c, ts)
}

func (s *requestSuite) TestRefreshSdkReplaced(c *check.C) {
	// Setup
	s.state.Lock()
	defer s.state.Unlock()
	existingSdks := workshop.SdkList{
		{Name: "sdk-1", Channel: "latest/stable"},
		{Name: "sdk-2", Channel: "latest/stable"},
	}
	inst := []sdk.Setup{
		{Name: "test-1", Channel: "latest/stable"}, {Name: "test-2", Channel: "latest/stable"},
	}

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{{Name: "test-1", Channel: "latest/stable"}, {Name: "test-2", Channel: "latest/stable"}}}

	w := s.launchWorkshopWithSDKs(c, "ws", existingSdks)

	// Execute
	ts, err := workshopstate.Refresh(s.state, file, maps.Values(w.Content), inst, s.project)
	c.Assert(err, check.IsNil)

	// Validate
	// save-state and restore-state will only exist for sdk-1
	validateStateHooksTasksSetup(c, []*state.TaskSet{ts}, nil, nil)
	verifyDisconnectDependencies(c, ts)
}

func (s *requestSuite) TestRefreshSdkChannelUpdated(c *check.C) {
	// Setup
	s.state.Lock()
	defer s.state.Unlock()
	existingSdks := workshop.SdkList{
		{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/stable"},
	}

	toninst := []sdk.Setup{
		{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/edge"},
	}

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{{Name: "sdk-1", Channel: "latest/stable"}, {Name: "sdk-2", Channel: "latest/edge"}}}

	w := s.launchWorkshopWithSDKs(c, "ws", existingSdks)

	// Execute
	ts, err := workshopstate.Refresh(s.state, file, maps.Values(w.Content), toninst, s.project)
	c.Assert(err, check.IsNil)

	// Validate
	// save-state and restore-state will only exist for sdk-1
	validateStateHooksTasksSetup(c, []*state.TaskSet{ts}, []string{"sdk-1", "sdk-2"}, []string{"sdk-1", "sdk-2"})
	verifyDisconnectDependencies(c, ts)
}

func (s *requestSuite) TestRefreshManyOneWorkshopHasNoSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	test_sdk := workshop.SdkRecord{Name: "sdk", Channel: "latest/stable"}

	files := []*workshop.File{
		{
			Name: "ws",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{}},
		{
			Name: "ws-1",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{test_sdk}},
	}

	content := [][]sdk.Setup{
		nil,
		{{
			Name:    "sdk",
			Channel: "latest/stable",
		},
		},
	}

	toninst := content

	ts, err := workshopstate.RefreshManyImpl(s.state, files, content, toninst, s.project)
	c.Assert(err, check.IsNil)

	expected_ws := []string{
		"create-state-storage",
		"remove-state-storage",
		"download-base",
		"create-workshop",
		"stash-workshop",
		"mount-project",
		"start-workshop",
		"install-local-sdk",
		"link-sdk",
		"auto-disconnect", // system SDK
		"auto-connect",    // system SDK
		"remove-workshop-stash",
	}

	expected_ws_1 := []string{
		"retrieve-sdk",
		"create-state-storage",
		"run-hook",        // save-state hook
		"auto-disconnect", // system SDK
		"auto-disconnect",
		"stash-workshop",
		"download-base",
		"create-workshop",
		"mount-project",
		"start-workshop",
		"install-local-sdk",
		"link-sdk",
		"install-sdk",
		"link-sdk",
		"auto-connect", // system SDK
		"auto-connect",
		"run-hook", // setup-base hook
		"run-hook", // restore-state hook
		"run-hook", // check-health hook
		"remove-workshop-stash",
		"remove-state-storage",
	}

	verifyExpectedTasks(c, ts[0].Tasks(), expected_ws)
	verifyExpectedTasks(c, ts[1].Tasks(), expected_ws_1)

	c.Assert(ts[0].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible).Kind(), check.Equals, "auto-connect")
	waitFor := ts[0].MaybeEdge(workshopstate.EdgeRefreshCleanup).WaitTasks()
	c.Assert(waitFor, testutil.DeepUnsortedMatches, []*state.Task{
		ts[0].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible),
		ts[1].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible),
	})

	c.Assert(ts[1].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible).Kind(), check.Equals, "run-hook")
	waitFor = ts[1].MaybeEdge(workshopstate.EdgeRefreshCleanup).WaitTasks()
	c.Assert(waitFor, testutil.DeepUnsortedMatches, []*state.Task{
		ts[0].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible),
		ts[1].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible),
	})

	for i, t := range ts {
		s.ensureTaskHasWorkshopAndProjectKeys(c, files[i].Name, t.Tasks())
	}
}

func (s *requestSuite) TestRefreshManyAllWorkshopsHaveSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	testSdk := workshop.SdkRecord{Name: "sdk", Channel: "latest/stable"}

	files := []*workshop.File{
		{
			Name: "ws",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{testSdk},
		},
		{
			Name: "ws1",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{testSdk},
		},
	}

	content := [][]sdk.Setup{
		{{
			Name:    "sdk",
			Channel: "latest/stable",
		},
		},
		{{
			Name:    "sdk",
			Channel: "latest/stable",
		},
		},
	}
	toinst := content

	ts, err := workshopstate.RefreshManyImpl(s.state, files, content, toinst, s.project)
	c.Assert(err, check.IsNil)

	expected := []string{
		"retrieve-sdk",
		"create-state-storage",
		"run-hook",
		"auto-disconnect",
		"stash-workshop",
		"download-base",
		"create-workshop",
		"mount-project",
		"start-workshop",
		"install-local-sdk",
		"link-sdk",
		"install-sdk",
		"link-sdk",
		"auto-disconnect", // system SDK
		"auto-connect",    // system SDK
		"auto-connect",
		"run-hook", // setup-base hook
		"run-hook", // restore state hook
		"run-hook", // check-health hook
		"remove-workshop-stash",
		"remove-state-storage",
	}

	for i, t := range ts {
		c.Assert(err, check.Equals, nil)
		verifyExpectedTasks(c, t.Tasks(), expected)
		s.ensureTaskHasWorkshopAndProjectKeys(c, files[i].Name, t.Tasks())
	}
}

func (s *requestSuite) TestRefreshManyWaitsOnAllSuccessfulBeforeRemovingStash(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	testSdk := workshop.SdkRecord{Name: "sdk", Channel: "latest/stable"}

	files := []*workshop.File{
		{
			Name: "ws",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{testSdk},
		},
		{
			Name: "ws1",
			Base: "ubuntu@22.04",
			Sdks: workshop.SdkList{testSdk},
		},
	}

	content := [][]sdk.Setup{
		{{
			Name:    "sdk",
			Channel: "latest/stable",
		},
		},
		{{
			Name:    "sdk",
			Channel: "latest/stable",
		},
		},
	}
	toinst := content

	ts, err := workshopstate.RefreshManyImpl(s.state, files, content, toinst, s.project)
	c.Assert(err, check.IsNil)

	lastChanceWs := ts[0].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible)
	c.Assert(lastChanceWs, check.NotNil)
	c.Assert(lastChanceWs.Kind(), check.Equals, "run-hook")

	lastChanceWs1 := ts[1].MaybeEdge(workshopstate.EdgeLastTaskBeforeRefreshIrreversible)
	c.Assert(lastChanceWs1, check.NotNil)
	c.Assert(lastChanceWs1.Kind(), check.Equals, "run-hook")

	// Ensure that the remove-stash for ws will wait on the own
	// remove-state-storage and ws1's (i.e. all the other workshops') restore
	// state hooks
	removeWsStStorage := ts[0].MaybeEdge(workshopstate.EdgeRefreshCleanup)
	removeStash := removeWsStStorage.HaltTasks()[0]
	c.Assert(removeWsStStorage.WaitTasks(), testutil.DeepUnsortedMatches, []*state.Task{lastChanceWs, lastChanceWs1})

	// Ensure that the stash and state storage removals belong to a separate
	// lane. In the case of their abortion it must not trigger abortion of the
	// refresh that, by that moment, is already done
	c.Assert(removeWsStStorage.Lanes()[0], check.Not(check.Equals), lastChanceWs.Lanes()[0])
	c.Assert(removeStash.Lanes()[0], check.Not(check.Equals), lastChanceWs.Lanes()[0])
	c.Assert(removeStash.Lanes(), check.HasLen, 1)
	c.Assert(removeWsStStorage.Lanes(), check.HasLen, 1)

	// Ensure that the delete-copy for ws1 will wait on the own
	// remove-state-storage and ws's (i.e. all the other workshops') restore
	// state hooks
	removeWsStStorage = ts[1].MaybeEdge(workshopstate.EdgeRefreshCleanup)
	removeStash = removeWsStStorage.HaltTasks()[0]
	c.Assert(removeWsStStorage.WaitTasks(), testutil.DeepUnsortedMatches, []*state.Task{lastChanceWs1, lastChanceWs})

	// Ensure that the stash and state storage removals belong to a separate
	// lane. In the case of their abortion it must not trigger abortion of the
	// refresh that, by that moment, is already done
	c.Assert(removeWsStStorage.Lanes()[0], check.Not(check.Equals), lastChanceWs1.Lanes()[0])
	c.Assert(removeStash.Lanes()[0], check.Not(check.Equals), lastChanceWs1.Lanes()[0])
	c.Assert(removeStash.Lanes(), check.HasLen, 1)
	c.Assert(removeWsStStorage.Lanes(), check.HasLen, 1)
}

func (s *requestSuite) TestRestoreStateHooks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	oneinst := workshop.SdkRecord{Name: "one", Channel: "latest/stable"}
	twoinst := workshop.SdkRecord{Name: "two", Channel: "latest/stable"}

	// installed SDKs
	one := sdk.Setup{Name: "one", Channel: "latest/stable"}
	two := sdk.Setup{Name: "two", Channel: "latest/stable"}

	ts := workshopstate.RestoreStateHooks(s.state, "ws",
		[]sdk.Setup{one, two}, workshop.SdkList{oneinst, twoinst})
	c.Assert(ts.Tasks(), check.HasLen, 2)

	prev := (*state.Task)(nil)
	for _, t := range ts.Tasks() {
		if prev != nil {
			c.Assert(t.WaitTasks(), check.DeepEquals, []*state.Task{prev})
		}
		prev = t
		var setup hookstate.HookSetup
		err := t.Get("hook-setup", &setup)
		c.Assert(err, check.IsNil)
		c.Assert(setup.HookType, check.Equals, hookstate.RestoreState)
	}
}

func (s *requestSuite) TestCheckHealthHooks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := &workshop.File{
		Name: "ws",
		Base: "ubuntu@22.04",
		Sdks: workshop.SdkList{{Name: "one", Channel: "latest/stable"}, {Name: "two", Channel: "latest/stable"}},
	}

	ts := workshopstate.CheckHealthHooks(s.state, file)
	c.Assert(ts.Tasks(), check.HasLen, 2)

	prev := (*state.Task)(nil)
	for _, t := range ts.Tasks() {
		if prev != nil {
			c.Assert(t.WaitTasks(), check.DeepEquals, []*state.Task{prev})
		}
		prev = t

		var setup hookstate.HookSetup
		err := t.Get("hook-setup", &setup)
		c.Assert(err, check.IsNil)
		c.Assert(setup.HookType, check.Equals, hookstate.CheckHealth)
		c.Assert(setup.Timeout, check.Equals, workshopstate.CheckHealthTimeout)
	}
}

func (s *requestSuite) TestSaveStateHooks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	// the SDK list to be refreshed (i.e. coming from a workshop file)
	installed := workshop.SdkList{workshop.SdkRecord{Name: "one"},
		workshop.SdkRecord{Name: "two"}}

	// installed SDKs
	one := sdk.Setup{Name: "one", Channel: "latest/stable"}
	two := sdk.Setup{Name: "two", Channel: "latest/stable"}

	ts := workshopstate.SaveStateHooks(s.state, "ws", []sdk.Setup{one, two}, installed)
	c.Assert(ts.Tasks(), check.HasLen, 2)

	prev := (*state.Task)(nil)
	for _, t := range ts.Tasks() {
		if prev != nil {
			c.Assert(t.WaitTasks(), check.DeepEquals, []*state.Task{prev})
		}
		prev = t
		var setup hookstate.HookSetup
		err := t.Get("hook-setup", &setup)
		c.Assert(err, check.IsNil)
		c.Assert(setup.HookType, check.Equals, hookstate.SaveState)
	}
}

func (s *requestSuite) TestStartMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts, err := workshopstate.StartManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(err, check.IsNil)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "start-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "start-workshop")
}

func (s *requestSuite) TestStopMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts, err := workshopstate.StopManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(err, check.IsNil)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "stop-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "stop-workshop")

	var force bool
	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)

	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)
}

func (s *requestSuite) TestRemountSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	content := workshop.SdkList{
		{Name: "sdk-1", Channel: "latest/stable"},
	}
	source := c.MkDir()

	s.launchWorkshopWithSDKs(c, "ws-1", content)

	ts, err := s.mgr.Remount(s.ctx, s.state, plug, source, s.project.ProjectId)
	c.Assert(err, check.IsNil)
	c.Assert(ts.Tasks(), check.HasLen, 1)

	var w string
	var p workshop.Project
	task := ts.Tasks()[0]
	c.Assert(task.Get("workshop", &w), check.IsNil)
	c.Assert(task.Get("project", &p), check.IsNil)
	c.Assert(task.Summary(), check.Equals, `Remount ws-1/sdk-1:plug`)
	c.Assert(w, check.Equals, "ws-1")
	c.Assert(p, check.DeepEquals, *s.project)

	var plugRef interfaces.PlugRef
	var src string
	c.Assert(task.Get("plug", &plugRef), check.IsNil)
	c.Assert(plugRef, check.DeepEquals, plug)
	c.Assert(task.Get("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, source)
}

func (s *requestSuite) TestRemountWorkshopNotReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	content := workshop.SdkList{
		{Name: "sdk-1", Channel: "latest/stable"},
	}

	s.launchWorkshopWithSDKs(c, "ws-1", content)

	// pretend there is another change running that would conflict with this one.
	change := s.state.NewChange("refresh", "test")
	task := s.state.NewTask("task", "test")
	task.Set("workshop", "ws-1")
	change.AddTask(task)
	change.Set("project-id", s.project.ProjectId)

	_, err := s.mgr.Remount(s.ctx, s.state, plug, c.MkDir(), s.project.ProjectId)
	c.Assert(err, check.ErrorMatches, remountNotReady)
}
