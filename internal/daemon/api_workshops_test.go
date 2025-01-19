package daemon

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

var (
	basic = `name: basic
base: ubuntu@22.04
`

	basic_invalid = `name: [basic]
base: ubuntu@22.04
`

	basic_refreshed = `name: basic
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
`

	scripts = `name: scripts
base: ubuntu@22.04
scripts:
  oneline: echo one line
  multiline: |
    echo two
    echo lines
`

	manysdks = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
`
	manysdks_refreshed = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:data-non-existent
    slot: system:mount
`
	manysdks_reversed = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk-2
    channel: latest/stable
  - name: test-sdk
    channel: latest/stable
`

	somebound = `name: somebound
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      data:
        bind: test-sdk-2:photos
  - name: test-sdk-2
    channel: latest/stable
`

	masterunknown = `name: masterunknown
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      unknown-data:
        bind: test-sdk-2:unknown
  - name: test-sdk-2
    channel: latest/stable
`

	slaveunknown = `name: slaveunknown
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      unknown:
        bind: test-sdk-2:photos
  - name: test-sdk-2
    channel: latest/stable
`

	bindincompatible = `name: bindincompatible
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      data:
        bind: test-sdk-2:gpu
  - name: test-sdk-2
    channel: latest/stable
`

	workshopplug = `name: workshopplug
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      training-slot:
        interface: mount
  - name: test-sdk
    channel: latest/stable
    plugs:
      training-plug:
        interface: mount
        workshop-target: /opt
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:training-plug
    slot: system:training-slot
`

	workshopplugbound = `name: workshopplugbound
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      training-slot:
        interface: mount
  - name: test-sdk
    channel: latest/stable
    plugs:
      training-plug:
        interface: mount
        workshop-target: /opt
      data:
        bind: test-sdk:training-plug
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:training-plug
    slot: system:training-slot
`

	workshopslot = `name: workshopslot
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      training:
        interface: mount
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
`

	workshopconns = `name: workshopconns
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      training:
        interface: mount
        workshop-source: /project
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:data
    slot: system:training
`

	workshopbrokenconn = `name: workshopbrokenconn
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:data-unknown-plug
    slot: system:mount
`

	connsplugbound = `name: connsplugbound
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      training:
        interface: mount
      photos:
        interface: mount
        workshop-source: /project/photos
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
    plugs:
      photos:
        bind: test-sdk:data
connections:
  - plug: test-sdk:data
    slot: system:training
`

	testsdk = `
name: test-sdk
title: title
base: ubuntu@20.04
version: '0.1.2'
summary: summary
description: SDK
sdkcraft-started-at: '2020-04-22T19:12:07.903032Z'
plugs:
  data:
    interface: mount
    workshop-target: /opt/data
  ssh-agent:
    interface: test
`

	testsdk2 = `
name: test-sdk-2
title: title
base: ubuntu@20.04
version: '20200401.3f3a63f'
summary: summary
description: SDK
sdkcraft-started-at: '2020-05-03T22:05:35.811829Z'
plugs:
  photos:
    interface: mount
    workshop-target: /opt/data2
  gpu:
    interface: gpu
`
)

var testsdks = map[string]string{
	"test-sdk":   testsdk,
	"test-sdk-2": testsdk2,
}

func (s *apiSuite) launchWorkshop(c *check.C, name, yaml string, sdks map[string]string) {
	s.createWFile(c, name, yaml)

	defer s.store.SetActionCallback(storeAction)()
	defer s.mockDoInstallSdk(c, name, sdks)()

	reqbuf := bytes.NewBufferString(fmt.Sprintf(`{"names":["%s"],"action":"launch"}`, name))
	s.vars = map[string]string{"id": s.project.ProjectId}
	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", reqbuf)
	c.Assert(err, check.IsNil)

	rsp := v1PostProjectWorkshop(apiCmd("/v1/projects/{id}/workshops"), req, nil).(*resp)

	st := s.d.state
	st.Lock()
	change := st.Change(rsp.Change)
	st.Unlock()
	<-change.Ready()

	st.Lock()
	c.Assert(change.Err(), check.IsNil)
	st.Unlock()
}

func (s *apiSuite) TestGetWorkshops(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "manysdks", manysdks, testsdks)
	s.launchWorkshop(c, "basic", basic, map[string]string{})

	projectsCmd := apiCmd("/v1/projects/{id}/workshops")
	s.vars = map[string]string{"id": s.project.ProjectId}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops", nil)
	c.Assert(err, check.IsNil)

	// Execute
	rsp := v1GetProjectWorkshops(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)
	// for DeepEqual to work correctly
	install1, install2 := s.installTime, s.installTime
	info := rsp.Result.(Workshops)
	for _, w := range info.Workshops {
		slices.SortFunc(w.Sdks, func(a, b *SdkInfo) int { return cmp.Compare(a.Name, b.Name) })
	}
	c.Check(info.Workshops, testutil.DeepUnsortedMatches, []*WorkshopInfo{{
		Name:      "manysdks",
		Base:      "ubuntu@22.04",
		ProjectId: s.project.ProjectId,
		Status:    "Ready",
		Sdks: []*SdkInfo{
			{
				Name:        "test-sdk",
				Channel:     "latest/stable",
				Revision:    "1",
				InstallTime: &install1,
			},
			{
				Name:        "test-sdk-2",
				Channel:     "latest/stable",
				Revision:    "1",
				InstallTime: &install2,
			},
		},
		Notes: nil,
	}, {
		Name:      "basic",
		Base:      "ubuntu@22.04",
		ProjectId: s.project.ProjectId,
		Status:    "Ready",
	}})

	c.Check(info.Files, testutil.DeepUnsortedMatches, []*WorkshopFileInfo{{
		Name:      "manysdks",
		Path:      workshop.Filepath(s.project.Path, "manysdks"),
		ProjectId: s.project.ProjectId,
	}, {
		Name:      "basic",
		Path:      workshop.Filepath(s.project.Path, "basic"),
		ProjectId: s.project.ProjectId,
	}})
}

func (s *apiSuite) TestGetWorkshopInfo(c *check.C) {
	// Setup (create a running workshop with a few mounts)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "manysdks", manysdks, testsdks)

	w, ok := s.b.Workshops[s.project.ProjectId]["manysdks"]
	c.Assert(ok, check.Equals, true)

	p := workshop.NewSdkProfile("test-sdk")
	p.Mounts["data"] = workshop.Mount{Name: "data",
		What:  sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "manysdks", "test-sdk", "data"),
		Where: "/opt/data",
		Type:  workshop.HostWorkshop,
	}
	w.Profiles["test-sdk"] = p

	p = workshop.NewSdkProfile("test-sdk-2")
	p.Mounts["photos"] = workshop.Mount{Name: "photos",
		What:  sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "manysdks", "test-sdk-2", "photos"),
		Where: "/opt/data2",
		Type:  workshop.HostWorkshop,
	}
	p.Mounts["photos2"] = workshop.Mount{Name: "photos2",
		What:  "/photos",
		Where: "/opt/data2",
		Type:  workshop.WorkshopWorkshop,
	}
	w.Profiles["test-sdk-2"] = p

	// Get Workshop info
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}")
	s.vars = map[string]string{"id": s.project.ProjectId, "name": "manysdks"}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops/manysdks", nil)
	c.Assert(err, check.IsNil)

	rsp := v1GetProjectWorkshop(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)
	build1 := time.Date(2020, 4, 22, 19, 12, 7, 903032000, time.UTC)
	build2 := time.Date(2020, 5, 3, 22, 5, 35, 811829000, time.UTC)
	// for DeepEqual to work correctly
	install1, install2 := s.installTime, s.installTime
	result := rsp.Result.(Workshop)
	slices.SortFunc(result.Sdks, func(a, b *SdkInfo) int { return cmp.Compare(a.Name, b.Name) })
	for _, c := range result.Sdks {
		slices.SortFunc(c.Mounts, func(a, b *Mount) int { return cmp.Compare(a.Plug.Name, b.Plug.Name) })
	}
	c.Check(result, check.DeepEquals, Workshop{
		WorkshopInfo: WorkshopInfo{
			Name:      "manysdks",
			Base:      "ubuntu@22.04",
			ProjectId: s.project.ProjectId,
			Status:    "Ready",
			Notes:     nil,
			Sdks: []*SdkInfo{
				{
					Name:        "test-sdk",
					Version:     "0.1.2",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build1,
					InstallTime: &install1,
					Mounts: []*Mount{
						{
							HostSource:     sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "manysdks", "test-sdk", "data"),
							WorkshopTarget: "/opt/data",
							Plug: interfaces.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "manysdks",
								Sdk:       "test-sdk",
								Name:      "data",
							},
						},
					},
				},
				{
					Name:        "test-sdk-2",
					Version:     "20200401.3f3a63f",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build2,
					InstallTime: &install2,
					Mounts: []*Mount{
						{
							HostSource:     sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "manysdks", "test-sdk-2", "photos"),
							WorkshopTarget: "/opt/data2",
							Plug: interfaces.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "manysdks",
								Sdk:       "test-sdk-2",
								Name:      "photos",
							},
						},
						{
							WorkshopSource: "/photos",
							WorkshopTarget: "/opt/data2",
							Plug: interfaces.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "manysdks",
								Sdk:       "test-sdk-2",
								Name:      "photos2",
							},
						},
					},
				},
			},
		},
		Path: workshop.Filepath(s.project.Path, "manysdks")})
}

func (s *apiSuite) TestGetWorkshopInfoSomePlugsBound(c *check.C) {
	// Setup (create a running workshop with a mount)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "somebound", somebound, testsdks)

	w, ok := s.b.Workshops[s.project.ProjectId]["somebound"]
	c.Assert(ok, check.Equals, true)

	p := workshop.NewSdkProfile("test-sdk-2")
	p.Mounts["photos"] = workshop.Mount{Name: "photos",
		What:  sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "somebound", "test-sdk-2", "photos"),
		Where: "/opt/data2",
		Type:  workshop.HostWorkshop,
	}
	w.Profiles["test-sdk-2"] = p

	// Get Workshop info
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}")
	s.vars = map[string]string{"id": s.project.ProjectId, "name": "somebound"}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops/somebound", nil)
	c.Assert(err, check.IsNil)

	rsp := v1GetProjectWorkshop(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)
	build1 := time.Date(2020, 4, 22, 19, 12, 7, 903032000, time.UTC)
	build2 := time.Date(2020, 5, 3, 22, 5, 35, 811829000, time.UTC)
	// for DeepEqual to work correctly
	install1, install2 := s.installTime, s.installTime
	result := rsp.Result.(Workshop)
	slices.SortFunc(result.Sdks, func(a, b *SdkInfo) int { return cmp.Compare(a.Name, b.Name) })
	for _, c := range result.Sdks {
		slices.SortFunc(c.Mounts, func(a, b *Mount) int { return cmp.Compare(a.Plug.Name, b.Plug.Name) })
	}
	c.Check(result, check.DeepEquals, Workshop{
		WorkshopInfo: WorkshopInfo{
			Name:      "somebound",
			Base:      "ubuntu@22.04",
			ProjectId: s.project.ProjectId,
			Status:    "Ready",
			Notes:     nil,
			Sdks: []*SdkInfo{
				{
					Name:        "test-sdk",
					Version:     "0.1.2",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build1,
					InstallTime: &install1,
					Mounts: []*Mount{
						{
							HostSource:     sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "somebound", "test-sdk-2", "photos"),
							WorkshopTarget: "/opt/data2",
							Plug: interfaces.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "somebound",
								Sdk:       "test-sdk",
								Name:      "data",
							},
						},
					},
				},
				{
					Name:        "test-sdk-2",
					Version:     "20200401.3f3a63f",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build2,
					InstallTime: &install2,
					Mounts: []*Mount{
						{
							HostSource:     sdk.SdkMountHostSource(s.userhome, s.project.ProjectId, "somebound", "test-sdk-2", "photos"),
							WorkshopTarget: "/opt/data2",
							Plug: interfaces.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "somebound",
								Sdk:       "test-sdk-2",
								Name:      "photos",
							},
						},
					},
				},
			},
		},
		Path: workshop.Filepath(s.project.Path, "somebound")})
}

func (s *apiSuite) TestGetWorkshopScripts(c *check.C) {
	// Setup (create a running workshop with a mount)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "scripts", scripts, map[string]string{})

	// Get Workshop scripts
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}/scripts")
	s.vars = map[string]string{"id": s.project.ProjectId, "name": "scripts"}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops/scripts/scripts", nil)
	c.Assert(err, check.IsNil)

	rsp := v1GetProjectWorkshopScripts(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)

	c.Check(rsp.Result, check.DeepEquals, map[string]Script{
		"oneline":   {Script: "echo one line"},
		"multiline": {Script: "echo two\necho lines\n"},
	})
}

type expectedResp struct {
	Type      ResponseType
	Status    int
	Message   string
	Kind      string
	Summary   string
	ChangeErr string // an error that happens during the change execution
}

func (s *apiSuite) runActionTest(c *check.C, buffers []*bytes.Buffer, expected []*expectedResp) {
	defer s.store.SetActionCallback(storeAction)()

	s.vars = map[string]string{"id": s.project.ProjectId}
	requests := []*http.Request{}

	for _, i := range buffers {
		req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", i)
		c.Assert(err, check.IsNil)
		requests = append(requests, req)
	}

	for num, req := range requests {
		// Execute
		rsp := v1PostProjectWorkshop(apiCmd("/v1/projects/{id}/workshops"), req, nil).(*resp)

		st := s.d.state
		st.Lock()
		change := st.Change(rsp.Change)
		st.Unlock()

		// Verify
		c.Check(rsp.Type, check.Equals, expected[num].Type)
		c.Check(rsp.Status, check.Equals, expected[num].Status, check.Commentf("case: %v", num))

		if rsp.Type == ResponseTypeError {
			c.Check(string(rsp.Result.(*errorResult).Kind), check.Equals, expected[num].Kind)
			c.Check(rsp.Result.(*errorResult).Message, check.Equals, expected[num].Message)
		}

		if rsp.Type == ResponseTypeAsync {
			ticker := time.NewTicker(100 * time.Millisecond)
		End:
			for {
				select {
				case <-change.Ready():
					break End
				case <-ticker.C:
					st.Lock()
					status := change.Status()
					st.Unlock()
					if status == state.WaitStatus {
						// some tests (refresh continue/abort) leave the change
						// in the wait state and this is expected.
						break End
					}
				}
			}
			c.Check(change.Kind(), check.Equals, expected[num].Kind)
			c.Check(change.Summary(), check.Equals, expected[num].Summary)
			st.Lock()
			if expected[num].ChangeErr != "" {
				c.Check(change.Err(), check.ErrorMatches, expected[num].ChangeErr)
			} else {
				c.Assert(change.Err(), check.IsNil)
			}
			st.Unlock()
		}
	}
}

func (s *apiSuite) createWFile(c *check.C, name, yaml string) {
	path := workshop.Filepath(s.project.Path, name)

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(yaml), 0644)
	c.Assert(err, check.IsNil)
}

func storeAction(ctx context.Context, actions []sdk.SdkAction) ([]sdk.SdkResult, error) {
	var result = []sdk.SdkResult{}
	for _, act := range actions {
		info, err := sdk.ReadSdkInfo([]byte(testsdks[act.Name]), act.ProjectId, act.Workshop)
		if err != nil {
			return nil, err
		}
		info.Channel = act.Channel
		info.Revision = sdk.Revision{N: 1}
		result = append(result, sdk.SdkResult{Info: info})
	}
	return result, nil
}

func (s *apiSuite) mockDoInstallSdk(c *check.C, ws string, sdks map[string]string) func() {
	order := 0
	s.b.ExecCallback = func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
		// check if the command is to install an SDK
		if args.Command[0] != "tar" {
			return workshop.ExecContext{WaitExecution: func(ctx context.Context) error { return nil }}, nil
		}

		fs, err := s.b.WorkshopFs(s.ctx, ws)
		c.Check(err, check.IsNil)
		defer fs.Close()

		// Get the SDK name from the exec command (we don't have a separate
		// method to install an SDK now).
		sdkname, found := strings.CutSuffix(filepath.Base(args.Command[3]), "_1.sdk")
		c.Check(found, check.Equals, true)

		// Install meta.
		metadir := filepath.Join(sdk.SdkRevPath(sdkname, "1"), "meta")
		err = fs.MkdirAll(metadir, 0755)
		c.Check(err, check.IsNil)
		err = fs.Symlink(sdk.SdkRevPath(sdkname, "1"), sdk.SdkCurrentPath(sdkname))
		c.Check(err, check.IsNil)
		file, err := fs.OpenFile(filepath.Join(metadir, "sdk.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		c.Check(err, check.IsNil)
		defer file.Close()
		syaml, exists := sdks[sdkname]
		c.Check(exists, check.Equals, true)
		_, err = file.Write([]byte(syaml))
		c.Check(err, check.IsNil)

		// Record installation order.
		order += 1
		orderPath := filepath.Join(sdk.SdkRevPath(sdkname, "1"), "order")
		err = afero.WriteFile(fs, orderPath, []byte(fmt.Sprintln(order)), 0644)
		c.Check(err, check.IsNil)

		// Install hooks.
		hooksdir := sdk.SdkHooksDir(sdkname)
		err = fs.MkdirAll(hooksdir, 0755)
		c.Check(err, check.IsNil)
		for _, hook := range []string{"setup-base", "save-state", "restore-state"} {
			setuphook, err := fs.OpenFile(sdk.SdkHookPath(sdkname, hook), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0744)
			c.Check(err, check.IsNil)
			defer setuphook.Close()
		}

		return workshop.ExecContext{WaitExecution: func(ctx context.Context) error { return nil }}, nil
	}
	return func() { s.b.ExecCallback = nil }
}

func (s *apiSuite) mockSketchSdk(c *check.C, ws string, meta string) {
	sdkpath := sdk.WorkshopSketchSdkCurrent(s.userhome, s.project.ProjectId, ws)
	metadir := filepath.Join(sdkpath, "meta")
	c.Assert(os.MkdirAll(metadir, 0755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(meta), 0644), check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopBasic(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "basic", basic)
	s.createWFile(c, "basic-invalid", basic_invalid)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic", "basic", "basic"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":[],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["missing"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["basic-invalid"],"action":"launch"}`),
	}

	missingFile := workshop.Filepath(s.project.Path, "missing")
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot launch: at least one workshop name must be provided",
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot launch "basic": workshop exists`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf(`cannot launch "missing": workshop definition %q not found`, missingFile),
		},
		{
			Type:   ResponseTypeError,
			Status: http.StatusBadRequest,
			Message: `cannot launch "basic-invalid": workshop definition YAML:
line 1: cannot unmarshal !!seq into string`,
		},
	}

	s.runActionTest(c, requests, expected)

	wp, err := s.b.Workshop(s.ctx, "basic")
	c.Assert(err, check.IsNil)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Slots(s.project.ProjectId, "basic", sdk.System.String()), check.HasLen, 5)

	c.Assert(s.b.DownloadBaseCalls, check.HasLen, 1)

	fw := s.b.Workshops[wp.Project.ProjectId]["basic"]
	c.Assert(fw.Devices[workshop.ConfigProjectPathDevice]["path"], check.Equals, workshop.WorkshopProjectPath)

	volume := workshop.AptCacheVolumeName("basic", wp.Project.ProjectId)
	c.Assert(s.b.WorkshopVolumes[volume], check.Equals, true)
	c.Assert(s.b.WorkshopVolumeMountPoints[volume], check.Equals, dirs.AptCachePath)

	c.Assert(wp.Running, check.Equals, true)

	sdkInfo, err := wp.SdkInfo(s.ctx, "system")
	c.Assert(err, check.IsNil)
	c.Assert(sdkInfo.Workshop, check.Equals, "basic")
	c.Assert(sdkInfo.Name, check.Equals, sdk.System.String())
	c.Assert(sdkInfo.Version, check.Equals, "")
	c.Assert(sdkInfo.Type, check.Equals, sdk.System)
	c.Assert(sdkInfo.BuildTime, check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopWithSlotOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "workshopslot", workshopslot)
	defer s.mockDoInstallSdk(c, "workshopslot", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopslot"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "workshopslot" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	_, err := s.b.Workshop(s.ctx, "workshopslot")
	c.Assert(err, check.IsNil)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Slot(s.project.ProjectId, "workshopslot", sdk.System.String(), "training"), check.Not(check.IsNil))
}

func (s *apiSuite) TestLaunchWorkshopFailed(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		return fmt.Errorf(`cannot assign profile to "manysdks"`)
	}

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "manysdks" workshop`,
			ChangeErr: `(?s).*cannot assign profile to "manysdks".*`,
		},
	}

	s.runActionTest(c, requests, expected)

	_, err := s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Slots(s.project.ProjectId, "manysdks", sdk.System.String()), check.HasLen, 0)
	c.Assert(repo.Plugs(s.project.ProjectId, "manysdks", sdk.System.String()), check.HasLen, 0)

	c.Assert(repo.Slots(s.project.ProjectId, "manysdks", "test-sdk"), check.HasLen, 0)
	c.Assert(repo.Plugs(s.project.ProjectId, "manysdks", "test-sdk"), check.HasLen, 0)

	c.Assert(repo.Slots(s.project.ProjectId, "manysdks", "test-sdk-2"), check.HasLen, 0)
	c.Assert(repo.Plugs(s.project.ProjectId, "manysdks", "test-sdk-2"), check.HasLen, 0)
}

func (s *apiSuite) TestLaunchWorkshopPlugBindsSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "somebound", somebound)
	defer s.mockDoInstallSdk(c, "somebound", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["somebound"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "somebound" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)
	_, err := s.b.Workshop(s.ctx, "somebound")
	c.Assert(err, check.IsNil)

	repo := s.d.overlord.InterfaceManager().Repository()
	conns, err := repo.Connected(s.project.ProjectId, "somebound", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)

	connection, err := repo.Connection(conns[0])
	c.Assert(err, check.IsNil)
	_, bound := connection.CheckBound()
	c.Assert(bound, check.Equals, true)
}

func (s *apiSuite) TestLaunchWorkshopBindPlugNoMasterPlug(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "masterunknown", masterunknown)
	defer s.mockDoInstallSdk(c, "masterunknown", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["masterunknown"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "masterunknown" workshop`,
			ChangeErr: `(?s).*SDK "masterunknown/test-sdk" has no plug named "unknown-data".*`,
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestLaunchWorkshopBindPlugNoSlavePlug(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "slaveunknown", slaveunknown)
	defer s.mockDoInstallSdk(c, "slaveunknown", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["slaveunknown"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "slaveunknown" workshop`,
			ChangeErr: `(?s).*SDK "slaveunknown/test-sdk" has no plug named "unknown".*`,
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestLaunchWorkshopBindPlugIncompatibleIface(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "bindincompatible", bindincompatible)
	defer s.mockDoInstallSdk(c, "bindincompatible", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["bindincompatible"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "bindincompatible" workshop`,
			ChangeErr: `(?s).*cannot bind "bindincompatible/test-sdk:data" \("mount" interface\) to "bindincompatible/test-sdk-2:gpu" \("gpu" interface\).*`,
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestLaunchWorkshopWithPlugOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "workshopplug", workshopplug)
	defer s.mockDoInstallSdk(c, "workshopplug", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopplug"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "workshopplug" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Plug(s.project.ProjectId, "workshopplug", "test-sdk", "training-plug"), check.NotNil)
	conns, err := repo.Connected(s.project.ProjectId, "workshopplug", "test-sdk", "training-plug")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplug/test-sdk:training-plug %s/workshopplug/system:training-slot`, s.project.ProjectId, s.project.ProjectId))
}

func (s *apiSuite) TestLaunchWorkshopPlugAddedAndBound(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "workshopplugbound", workshopplugbound)
	defer s.mockDoInstallSdk(c, "workshopplugbound", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopplugbound"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "workshopplugbound" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Plug(s.project.ProjectId, "workshopplugbound", "test-sdk", "training-plug"), check.NotNil)
	conns, err := repo.Connected(s.project.ProjectId, "workshopplugbound", "test-sdk", "training-plug")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplugbound/test-sdk:training-plug %s/workshopplugbound/system:training-slot`, s.project.ProjectId, s.project.ProjectId))

	conns, err = repo.Connected(s.project.ProjectId, "workshopplugbound", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplugbound/test-sdk:data %s/workshopplugbound/system:training-slot`, s.project.ProjectId, s.project.ProjectId))
}

func (s *apiSuite) TestWorkshopConnectionsOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "workshopconns", workshopconns)
	defer s.mockDoInstallSdk(c, "workshopconns", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopconns"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "workshopconns" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	_, err := s.b.Workshop(s.ctx, "workshopconns")
	c.Assert(err, check.IsNil)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Slot(s.project.ProjectId, "workshopconns", sdk.System.String(), "training"), check.Not(check.IsNil))

	conns, err := repo.Connections(s.project.ProjectId, "workshopconns", "test-sdk")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk", Name: "data"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: sdk.System.String(), Name: "training"},
		},
	})

	conns, err = repo.Connections(s.project.ProjectId, "workshopconns", "test-sdk-2")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "photos"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: sdk.System.String(), Name: "mount"},
		}, {
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "gpu"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: sdk.System.String(), Name: "gpu"},
		},
	})
}

func (s *apiSuite) TestWorkshopConnectionsUnknownPlug(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "workshopbrokenconn", workshopbrokenconn)
	defer s.mockDoInstallSdk(c, "workshopbrokenconn", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopbrokenconn"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "workshopbrokenconn" workshop`,
			ChangeErr: `(?s).*SDK "workshopbrokenconn/test-sdk" has no plug named "data-unknown-plug".*`,
		},
	}

	s.runActionTest(c, requests, expected)

	repo := s.d.overlord.InterfaceManager().Repository()
	conns, err := repo.Connections(s.project.ProjectId, "workshopbrokenconn", "test-sdk")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)
	conns, err = repo.Connections(s.project.ProjectId, "workshopbrokenconn", "test-sdk-2")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)
}

func (s *apiSuite) TestWorkshopConnectionsPlugIsBoundTo(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "connsplugbound", connsplugbound)
	defer s.mockDoInstallSdk(c, "connsplugbound", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["connsplugbound"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "connsplugbound" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	repo := s.d.overlord.InterfaceManager().Repository()
	conns, err := repo.Connected(s.project.ProjectId, "connsplugbound", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].SlotRef.Name, check.Equals, "training")

	conns, err = repo.Connected(s.project.ProjectId, "connsplugbound", "test-sdk-2", "photos")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].SlotRef.Name, check.Equals, "training")

	connection, err := repo.Connection(conns[0])
	c.Assert(err, check.IsNil)
	_, bound := connection.CheckBound()
	c.Assert(bound, check.Equals, true)
}

func (s *apiSuite) TestRefreshWorkshopSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
	}
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	s.createWFile(c, "basic", basic_refreshed)
	defer s.mockDoInstallSdk(c, "basic", testsdks)()

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	repo := s.d.overlord.InterfaceManager().Repository()

	conns, err := repo.Connections(s.project.ProjectId, "basic", "test-sdk")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: "test-sdk", Name: "data"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: sdk.System.String(), Name: "mount"},
		},
	})

	conns, err = repo.Connections(s.project.ProjectId, "basic", "test-sdk-2")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: "test-sdk-2", Name: "photos"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: sdk.System.String(), Name: "mount"},
		}, {
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: "test-sdk-2", Name: "gpu"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "basic", Sdk: sdk.System.String(), Name: "gpu"},
		},
	})
}

func (s *apiSuite) TestRefreshWorkshopReturnsPreviousWorkshopIfFailed(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`)}

	expected := []*expectedResp{{
		Type:    ResponseTypeAsync,
		Status:  http.StatusAccepted,
		Kind:    "launch",
		Summary: `Launch "manysdks" workshop`,
	}}
	s.runActionTest(c, requests, expected)

	// Setup "refresh" with a new workshop that will trigger an error
	s.createWFile(c, "manysdks", manysdks_refreshed)
	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh"}`)}
	expected = []*expectedResp{{
		Type:      ResponseTypeAsync,
		Status:    http.StatusAccepted,
		Kind:      "refresh",
		Summary:   `Refresh "manysdks" workshop`,
		ChangeErr: `(?s).*SDK "manysdks/test-sdk" has no plug named "data-non-existent".*`,
	}}
	s.runActionTest(c, requests, expected)

	wp, err := s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)

	sdks, err := wp.SdkInfos(s.ctx)
	c.Assert(err, check.IsNil)
	c.Assert(sdks, check.HasLen, 2)

	repo := s.d.overlord.InterfaceManager().Repository()
	conns, err := repo.Connections(s.project.ProjectId, "manysdks", "test-sdk")
	c.Assert(err, check.IsNil)

	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: "test-sdk", Name: "data"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: sdk.System.String(), Name: "mount"},
		},
	})

	conns, err = repo.Connections(s.project.ProjectId, "manysdks", "test-sdk-2")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: "test-sdk-2", Name: "photos"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: sdk.System.String(), Name: "mount"},
		},
		{
			PlugRef: interfaces.PlugRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: "test-sdk-2", Name: "gpu"},
			SlotRef: interfaces.SlotRef{ProjectId: s.project.ProjectId, Workshop: "manysdks", Sdk: sdk.System.String(), Name: "gpu"},
		},
	})
}

// Tests the input validation logic of v1PostProjectWorkshop. Excludes any
// dispatch validation, these are covered by their own tests.
func (s *apiSuite) TestValidatev1PostProjectWorkshopModeInputs(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	type table struct {
		cmd    string
		result map[string]string
	}

	// Note we are explicitly testing the validation up until dispatch here. All
	// error messages are desired. 'mode' errors represent an invalid
	// input, all other errors occur after input validation - these represent a
	// valid input
	cmds := []table{
		{
			cmd: "launch",
			result: map[string]string{
				"":              `cannot launch "basic": workshop definition .*`,
				"transactional": `cannot launch "basic": workshop definition .*`,
				"wait-on-error": `cannot launch "basic": workshop definition .*`,
				"continue":      "cannot continue: no wait in progress",
				"abort":         "cannot abort: no wait in progress",
				"invalid-mode":  `cannot launch: "invalid-mode" is not a valid mode`,
			},
		}, {
			cmd: "refresh",
			result: map[string]string{
				"":              `cannot refresh "basic": workshop not launched`,
				"transactional": `cannot refresh "basic": workshop not launched`,
				"wait-on-error": `cannot refresh "basic": workshop not launched`,
				"continue":      "cannot continue: no wait in progress",
				"abort":         "cannot abort: no wait in progress",
				"invalid-mode":  `cannot refresh: "invalid-mode" is not a valid mode`,
			},
		}, {
			cmd: "start",
			result: map[string]string{
				"":              `cannot start "basic": workshop not launched`,
				"transactional": `cannot start "basic": workshop not launched`,
				"wait-on-error": `cannot start: mode "wait-on-error" is not valid with the "start" command`,
				"continue":      `cannot start: mode "continue" is not valid with the "start" command`,
				"abort":         `cannot start: mode "abort" is not valid with the "start" command`,
				"invalid-mode":  `cannot start: "invalid-mode" is not a valid mode`,
			},
		}, {
			cmd: "stop",
			result: map[string]string{
				"":              `cannot stop "basic": workshop not launched`,
				"transactional": `cannot stop "basic": workshop not launched`,
				"wait-on-error": `cannot stop: mode "wait-on-error" is not valid with the "stop" command`,
				"continue":      `cannot stop: mode "continue" is not valid with the "stop" command`,
				"abort":         `cannot stop: mode "abort" is not valid with the "stop" command`,
				"invalid-mode":  `cannot stop: "invalid-mode" is not a valid mode`,
			},
		}, {
			cmd: "remove",
			result: map[string]string{
				"":              `cannot remove "basic": workshop not launched`,
				"transactional": `cannot remove "basic": workshop not launched`,
				"wait-on-error": `cannot remove: mode "wait-on-error" is not valid with the "remove" command`,
				"continue":      `cannot remove: mode "continue" is not valid with the "remove" command`,
				"abort":         `cannot remove: mode "abort" is not valid with the "remove" command`,
				"invalid-mode":  `cannot remove: "invalid-mode" is not a valid mode`,
			},
		},
	}

	for _, cmd := range cmds {
		for mode, error := range cmd.result {
			// Construct request
			req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", strings.NewReader(fmt.Sprintf(`{"names":["basic"],"action":"%s", "options": {"mode":"%s"}}`, cmd.cmd, mode)))
			c.Assert(err, check.IsNil)

			// Construct response
			exp := expectedResp{
				Type:    ResponseTypeError,
				Status:  http.StatusBadRequest,
				Message: error,
			}

			// Execute
			rsp := v1PostProjectWorkshop(apiCmd("/v1/projects/{id}/workshops"), req, nil).(*resp)

			// Validate
			c.Check(rsp.Type, check.Equals, exp.Type)
			c.Check(rsp.Status, check.Equals, exp.Status)
			c.Check(rsp.Result.(*errorResult).Message, check.Matches, exp.Message)
		}
	}
}

func (s *apiSuite) TestValidatev1PostProjectWorkshopActionInputs(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	type table struct {
		cmd    string
		result string
	}

	cmds := []table{
		{
			cmd:    "invalid-cmd",
			result: "unknown action \"invalid-cmd\"",
		},
		{
			cmd:    "",
			result: "unknown action \"\"",
		},
	}

	for _, cmd := range cmds {
		// Construct request
		req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", strings.NewReader(fmt.Sprintf(`{"names":["basic"],"action":"%s"}`, cmd.cmd)))
		c.Assert(err, check.IsNil)

		// Construct response
		exp := expectedResp{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: cmd.result,
		}

		// Execute
		rsp := v1PostProjectWorkshop(apiCmd("/v1/projects/{id}/workshops"), req, nil).(*resp)

		// Validate
		c.Check(rsp.Type, check.Equals, exp.Type)
		c.Check(rsp.Status, check.Equals, exp.Status)
		c.Check(rsp.Result.(*errorResult).Message, check.Matches, exp.Message)
	}
}

func (s *apiSuite) TestRefreshWorkshopContinueSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	s.createWFile(c, "basic", basic)

	var errOnce sync.Once
	s.secBackend.RemoveCallback = func(sdkName string) error {
		var err error
		errOnce.Do(func() { err = errors.New("cannot remove profile") })
		return err
	}

	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		// start - continue (success)
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"continue"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no refresh in progress after continue was successful
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "basic", "")
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestRefreshWorkshopNoRefreshInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"continue"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"abort"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot continue: no wait in progress",
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot abort: no wait in progress",
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestRefreshWorkshopChangeAbort(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)

	var errOnce sync.Once
	removeProfile := false
	s.secBackend.RemoveCallback = func(sdkName string) error {
		var err error
		errOnce.Do(func() { err = fmt.Errorf("cannot remove profiles") })
		removeProfile = true
		return err
	}

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		// start - abort (both success)
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"abort"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	c.Assert(removeProfile, check.Equals, true)
	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no refresh in progress after continue was successful
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "basic", "")
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopRefreshLaunchInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		var err error
		errOnce.Do(func() { err = errors.New("setup failed") })
		return err
	}

	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options": {"mode":"continue"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot continue: refresh requested but launch is in progress",
		},
	}
	s.runActionTest(c, requests, expected)

	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no wait in progress after continue was successful
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", "")
	c.Assert(err, check.ErrorMatches, `*. has "launch" change in progress`)
}

func (s *apiSuite) TestLaunchWorkshopContinueSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		var err error
		errOnce.Do(func() { err = errors.New("setup failed") })
		return err
	}

	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch","options": {"mode":"continue"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no wait in progress after continue was successful
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", "")
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopNoRefreshInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"launch","options": {"mode":"continue"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"launch","options": {"mode":"abort"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot continue: no wait in progress",
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "cannot abort: no wait in progress",
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestLaunchWorkshopChangeAbort(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		var err error
		errOnce.Do(func() { err = errors.New("setup failed") })
		return err
	}

	requests := []*bytes.Buffer{
		// start - abort (both success)
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch","options": {"mode":"abort"}}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no refresh in progress after continue was successful
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", "")
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestRefreshWorkshopPartialOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()
	s.mockSketchSdk(c, "manysdks", `name: sketch
base: ubuntu@22.04
plugs:
  sketch-plug:
    interface: mount
    workshop-target: /etc
`)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks/sketch"],"action":"refresh"}`),
		bytes.NewBufferString(`{"names":["manysdks/sketch"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "manysdks/sketch" SDK`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "manysdks/sketch" SDK`,
		},
	}

	s.runActionTest(c, requests, expected)

	wp, err := s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)
	c.Assert(wp.Running, check.Equals, true)

	sketchsetup := wp.Sdks["sketch"]
	c.Assert(sketchsetup.RevisionSequence, check.HasLen, 1)
	c.Assert(sketchsetup.RevisionSequence[0].String(), check.Equals, "x1")
	c.Assert(sketchsetup.Revision.String(), check.Equals, "x2")

	fs, err := s.b.WorkshopFs(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)

	_, err = fs.Stat(sdk.SdkRevPath("sketch", "x1"))
	c.Assert(err, check.IsNil)

	_, err = fs.Stat(sdk.SdkRevPath("sketch", "x2"))
	c.Assert(err, check.IsNil)

	path, err := fs.ReadLink(sdk.SdkCurrentPath("sketch"))
	c.Assert(err, check.IsNil)
	c.Assert(strings.HasSuffix(path, sdk.SdkRevPath("sketch", "x2")), check.Equals, true)

	repo := s.d.overlord.InterfaceManager().Repository()
	c.Assert(repo.Plug(s.project.ProjectId, "manysdks", "sketch", "sketch-plug"), check.NotNil)
}
func (s *apiSuite) TestRefreshWorkshopPartialConflictChange(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()
	s.mockSketchSdk(c, "manysdks", `name: illegal%
base: ubuntu@22.04
`)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks/sketch"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "manysdks/sketch" SDK`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot refresh "manysdks": waiting on error`,
			Kind:    "waiting-on-error",
			Summary: `Refresh "manysdks" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestSDKInstallationOrder(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Install test-sdk-2 first.
	s.createWFile(c, "manysdks", manysdks_reversed)
	defer s.mockDoInstallSdk(c, "manysdks", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	fs, err := s.b.WorkshopFs(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)

	order, err := afero.ReadFile(fs, filepath.Join(sdk.SdkCurrentPath("test-sdk-2"), "order"))
	c.Assert(err, check.IsNil)
	c.Check(string(order), check.Equals, "1\n")

	order, err = afero.ReadFile(fs, filepath.Join(sdk.SdkCurrentPath("test-sdk"), "order"))
	c.Assert(err, check.IsNil)
	c.Check(string(order), check.Equals, "2\n")

	// Install test-sdk first this time.
	s.createWFile(c, "manysdks", manysdks)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "manysdks" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	fs, err = s.b.WorkshopFs(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)

	order, err = afero.ReadFile(fs, filepath.Join(sdk.SdkCurrentPath("test-sdk"), "order"))
	c.Assert(err, check.IsNil)
	c.Check(string(order), check.Equals, "3\n")

	order, err = afero.ReadFile(fs, filepath.Join(sdk.SdkCurrentPath("test-sdk-2"), "order"))
	c.Assert(err, check.IsNil)
	c.Check(string(order), check.Equals, "4\n")
}

func (s *apiSuite) TestStartWorkshop(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),

		bytes.NewBufferString(`{"names":["basic"],"action":"stop"}`),
		//
		bytes.NewBufferString(`{"names":["basic"],"action":"start"}`),
		// a second attempt to start the workshop that is already in Started
		bytes.NewBufferString(`{"names":["basic"],"action":"start"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "stop",
			Summary: `Stop "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "start",
			Summary: `Start "basic" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot start "basic": workshop already running`,
		},
	}

	s.runActionTest(c, requests, expected)

	wp, err := s.b.Workshop(s.ctx, "basic")
	c.Assert(err, check.IsNil)
	c.Assert(wp.Running, check.Equals, true)
}

func (s *apiSuite) TestStopWorkshop(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),

		bytes.NewBufferString(`{"names":["basic"],"action":"stop"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "stop",
			Summary: `Stop "basic" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	wp, err := s.b.Workshop(s.ctx, "basic")
	c.Assert(err, check.IsNil)
	c.Assert(wp.Running, check.Equals, false)
}

func (s *apiSuite) TestRemoveWorkshopSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "workshopconns", workshopconns)
	defer s.mockDoInstallSdk(c, "workshopconns", testsdks)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopconns"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["workshopconns"],"action":"remove"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "workshopconns" workshop`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "remove",
			Summary: `Remove "workshopconns" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	_, err := s.b.Workshop(s.ctx, "workshopconns")
	c.Check(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
	c.Check(s.secBackend.RemoveCalls, check.HasLen, 3)
}
