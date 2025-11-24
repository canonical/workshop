package daemon

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha3"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
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
  - name: project-test-sdk-2
`

	actions = `name: actions
base: ubuntu@22.04
actions:
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
	manysdks_broken = `name: manysdks
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
	manysdks_minusone = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
`
	manysdks_newchan = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/edge
  - name: test-sdk-2
    channel: latest/stable
`
	manysdks_connsadded = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
connections:
  - plug: test-sdk:data
    slot: test-sdk-2:data-slot
`

	manysdks_plugadded = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      new-plug:
        interface: mount
        workshop-target: /opt
  - name: test-sdk-2
    channel: latest/stable
`

	manysdks_try = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: try-test-sdk
  - name: try-test-sdk-2
`

	manysdks_project = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: project-test-sdk
  - name: project-test-sdk-2
`

	manysdks_newbase = `name: manysdks
base: ubuntu@24.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
`

	manysdks_system = `name: manysdks
base: ubuntu@24.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: system
`

	manysdks_allremoved = `name: manysdks
base: ubuntu@22.04
`

	manysdks_extended = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk-3
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
  - name: test-sdk
    channel: latest/stable
`

	manysdks_system_extended = `name: manysdks
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: system
    slots:
      tunnel:
        interface: tunnel
        endpoint: 8080
`

	wrongbase = `name: wrongbase
base: ubuntu@24.04
sdks:
  - name: test-sdk-3
    channel: latest/stable
`

	workshoptunnels = `name: tunnels
base: ubuntu@22.04
sdks:
  - name: system
    plugs:
      api:
        interface: tunnel
        endpoint: 0.0.0.0:8888/tcp
    slots:
      dns:
        interface: tunnel
        endpoint: 127.0.0.53/udp
  - name: test-sdk
    channel: latest/stable
    plugs:
      dns:
        interface: tunnel
        endpoint: 127.0.0.1:5353/udp
  - name: test-sdk-2
    channel: latest/stable
    slots:
      api:
        interface: tunnel
        endpoint: '@testapi'
`

	somebound = `name: somebound
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      data:
        bind: mount-conflict:photos
  - name: mount-conflict
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
  - name: test-sdk
    channel: latest/stable
    plugs:
      training-plug:
        interface: mount
        workshop-target: /opt
  - name: test-sdk-2
    channel: latest/stable
`

	workshopplugbound = `name: workshopplugbound
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    plugs:
      training-plug:
        interface: mount
        workshop-target: /opt/data
      data:
        bind: test-sdk:training-plug
  - name: test-sdk-2
    channel: latest/stable
`

	workshopslot = `name: workshopslot
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
    slots:
      training:
        interface: mount
        workshop-source: /project/training
  - name: test-sdk-2
    channel: latest/stable
`

	workshopconns = `name: workshopconns
base: ubuntu@22.04
sdks:
  - name: test-sdk
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
    slots:
      training:
        interface: mount
        workshop-source: /project
connections:
  - plug: test-sdk:data
    slot: test-sdk-2:training
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
  - name: test-sdk
    channel: latest/stable
    slots:
      photos:
        interface: mount
        workshop-source: /project/photos
  - name: mount-conflict
    channel: latest/stable
    plugs:
      photos:
        bind: test-sdk:data
    slots:
      training:
        interface: mount
connections:
  - plug: test-sdk:data
    slot: mount-conflict:training
`

	testsdk = `
name: test-sdk
title: title
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

	testsdk_r2 = `
name: test-sdk
title: title
version: '0.1.3'
summary: summary
description: SDK
sdkcraft-started-at: '2020-04-22T19:12:07.903032Z'
plugs:
  ssh-agent:
    interface: ssh-agent
  desktop:
    interface: desktop
`

	testsdk2 = `
name: test-sdk-2
title: title
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
slots:
  data-slot:
    workshop-source: /mnt
    interface: mount
`

	testsdk2_invalid = `name: sdk-2
`

	mount_conflict = `
name: mount-conflict
title: title
base: ubuntu@22.04
version: '20200401.3f3a63f'
summary: summary
description: SDK
sdkcraft-started-at: '2020-05-03T22:05:35.811829Z'
plugs:
  photos:
    interface: mount
    workshop-target: /opt/data
  gpu:
    interface: gpu
slots:
  data-slot:
    workshop-source: /mnt
    interface: mount
`

	testsdk3 = `
name: test-sdk-3
title: title
base: ubuntu@22.04
version: '20200401.3f3a63f'
summary: summary
description: SDK
sdkcraft-started-at: '2020-05-03T22:05:35.811829Z'
`
)

var apiSuiteSdks = map[string]sdk.Meta{
	"test-sdk": {
		Setup: sdk.Setup{
			Name:     "test-sdk",
			Revision: sdk.R(1),
			Sha3_384: "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
		},
		SdkYAML: testsdk,
	},
	"test-sdk-2": {
		Setup: sdk.Setup{
			Name:     "test-sdk-2",
			Revision: sdk.R(1),
			Sha3_384: "d4089378c26310627268153caa216240311f2a3193c778e96ed6dd895dc10c82db50f4f39676b29d23d9813b21e14b9b",
		},
		SdkYAML: testsdk2,
	},
	"mount-conflict": {
		Setup: sdk.Setup{
			Name:     "mount-conflict",
			Revision: sdk.R(1),
			Sha3_384: "b2bd882ceec6746f00ea2b6cbb6c2073d46d5844b2a3a92a573dd6ea02847a98085b7a7b192d7e24c3f87ba01bbf554e",
		},
		SdkYAML: mount_conflict,
	},
	"test-sdk-3": {
		Setup: sdk.Setup{
			Name:     "test-sdk-3",
			Revision: sdk.R(1),
			Sha3_384: "0b4b94c4685db0970a15d294ce8e0683b5c6957b08a94ab8a3b14d3aac90f06a4d2cc1240dad04e5be0fe358276e64fc",
		},
		SdkYAML: testsdk3,
	},
}

func (s *apiSuite) launchWorkshop(c *check.C, name, yaml string) {
	s.createWFile(c, name, yaml)

	defer s.store.SetActionCallback(storeAction)()
	defer s.store.SetDownloadCallback(storeDownload)()

	reqbuf := bytes.NewBufferString(fmt.Sprintf(`{"names":["%s"],"action":"launch"}`, name))
	s.vars = map[string]string{"id": s.project.ProjectId}
	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", reqbuf)
	c.Assert(err, check.IsNil)

	rsp := v1PostProjectWorkshop(apiCmd("/v1/projects/{id}/workshops"), req, nil).(*resp)

	st := s.d.state
	st.Lock()
	change := st.Change(rsp.Change)
	st.Unlock()
	c.Assert(change, check.NotNil)
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

	s.launchWorkshop(c, "manysdks", manysdks_system)
	s.launchWorkshop(c, "basic", basic)

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

	info := rsp.Result.(Workshops)

	c.Check(info.Workshops, testutil.DeepUnsortedMatches, []*WorkshopInfo{{
		Name:      "manysdks",
		Base:      "ubuntu@24.04",
		ProjectId: s.project.ProjectId,
		Status:    "Ready",
		Sdks: []*SdkInfo{
			{
				Name:        "system",
				Revision:    system.SystemSdkRevision.String(),
				InstallTime: time.Date(2023, 4, 25, 1, 2, 3, 0, time.UTC),
			},
			{
				Name:        "test-sdk",
				Channel:     "latest/stable",
				Revision:    "1",
				InstallTime: time.Date(2023, 4, 25, 1, 2, 3, 0, time.UTC),
			},
		},
	}, {
		Name:      "basic",
		Base:      "ubuntu@22.04",
		ProjectId: s.project.ProjectId,
		Status:    "Ready",
		Sdks: []*SdkInfo{{
			Name:        "system",
			Revision:    system.SystemSdkRevision.String(),
			InstallTime: time.Date(2023, 4, 25, 1, 2, 3, 0, time.UTC),
		}},
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
	// Setup (create a running workshop with a few mounts and tunnels)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "tunnels", workshoptunnels)

	w, ok := s.b.Workshops[s.project.ProjectId]["tunnels"]
	c.Assert(ok, check.Equals, true)

	testSDKSource := workshop.SdkMountHostSource(s.user.HomeDir, s.project.ProjectId, "tunnels", "test-sdk-2", "data")

	p := workshop.NewSdkProfile("system")
	p.Tunnels = []workshop.Tunnel{{ProxyEntry: workshop.ProxyEntry{
		Name:      "api",
		Connect:   workshop.ProxyTarget{Protocol: "unix", Address: "@testapi"},
		Listen:    workshop.ProxyTarget{Protocol: "tcp", Address: "0.0.0.0:8888"},
		Direction: workshop.HostToWorkshop,
	}}}
	w.Profiles["system"] = p

	p = workshop.NewSdkProfile("test-sdk")
	p.Mounts["data"] = workshop.Mount{
		Name:      "data",
		Type:      workshop.HostWorkshop,
		What:      testSDKSource,
		MakeWhat:  true,
		Where:     "/opt/data",
		MakeWhere: true,
		Mode:      0o755,
	}
	p.Tunnels = []workshop.Tunnel{{ProxyEntry: workshop.ProxyEntry{
		Name:      "dns",
		Connect:   workshop.ProxyTarget{Protocol: "udp", Address: "127.0.0.53:5353"},
		Listen:    workshop.ProxyTarget{Protocol: "udp", Address: "127.0.0.1:5353"},
		Direction: workshop.WorkshopToHost,
	}}}
	w.Profiles["test-sdk"] = p

	testSDKSource2 := workshop.SdkMountHostSource(s.user.HomeDir, s.project.ProjectId, "tunnels", "test-sdk-2", "photos")

	p = workshop.NewSdkProfile("test-sdk-2")
	p.Mounts["photos"] = workshop.Mount{
		Name:      "photos",
		Type:      workshop.HostWorkshop,
		What:      testSDKSource2,
		MakeWhat:  true,
		Where:     "/opt/data2",
		MakeWhere: true,
		Mode:      0o755,
	}
	p.Mounts["photos2"] = workshop.Mount{
		Name:  "photos2",
		Type:  workshop.WorkshopWorkshop,
		What:  "/photos",
		Where: "/opt/data3",
	}
	w.Profiles["test-sdk-2"] = p

	// Get Workshop info
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}")
	s.vars = map[string]string{"id": s.project.ProjectId, "name": "tunnels"}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops/tunnels", nil)
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
	result := rsp.Result.(Workshop)
	for _, c := range result.Sdks {
		slices.SortFunc(c.Mounts, func(a, b *Mount) int { return cmp.Compare(a.Plug.Name, b.Plug.Name) })
	}
	c.Assert(err, check.IsNil)
	c.Check(result, check.DeepEquals, Workshop{
		WorkshopInfo: WorkshopInfo{
			Name:      "tunnels",
			Base:      "ubuntu@22.04",
			ProjectId: s.project.ProjectId,
			Status:    "Ready",
			Notes:     nil,
			Sdks: []*SdkInfo{
				{
					Name:        "system",
					Revision:    system.SystemSdkRevision.String(),
					InstallTime: s.installTime,
					Tunnels: []*Tunnel{
						{
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "tunnels",
								Sdk:       "system",
								Name:      "api",
							},
							From: Endpoint{
								Protocol: "tcp",
								Host:     "0.0.0.0",
								Port:     8888,
							},
							To: Endpoint{
								Protocol: "unix",
								Path:     "@testapi",
							},
						},
					},
				},
				{
					Name:        "test-sdk",
					Version:     "0.1.2",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build1,
					InstallTime: s.installTime,
					Mounts: []*Mount{
						{
							HostSource:     testSDKSource,
							WorkshopTarget: "/opt/data",
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "tunnels",
								Sdk:       "test-sdk",
								Name:      "data",
							},
						},
					},
					Tunnels: []*Tunnel{
						{
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "tunnels",
								Sdk:       "test-sdk",
								Name:      "dns",
							},
							From: Endpoint{
								Protocol: "udp",
								Host:     "127.0.0.1",
								Port:     5353,
							},
							To: Endpoint{
								Protocol: "udp",
								Host:     "127.0.0.53",
								Port:     5353,
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
					InstallTime: s.installTime,
					Mounts: []*Mount{
						{
							HostSource:     testSDKSource2,
							WorkshopTarget: "/opt/data2",
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "tunnels",
								Sdk:       "test-sdk-2",
								Name:      "photos",
							},
						},
						{
							WorkshopSource: "/photos",
							WorkshopTarget: "/opt/data3",
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "tunnels",
								Sdk:       "test-sdk-2",
								Name:      "photos2",
							},
						},
					},
				},
			},
		},
		Path: workshop.Filepath(s.project.Path, "tunnels"),
	})
}

func (s *apiSuite) TestGetWorkshopInfoSomePlugsBound(c *check.C) {
	// Setup (create a running workshop with a mount)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "somebound", somebound)

	w, ok := s.b.Workshops[s.project.ProjectId]["somebound"]
	c.Assert(ok, check.Equals, true)

	testSDKSource := workshop.SdkMountHostSource(s.user.HomeDir, s.project.ProjectId, "somebound", "mount-conflict", "photos")
	p := workshop.NewSdkProfile("mount-conflict")
	p.Mounts["photos"] = workshop.Mount{
		Name:      "photos",
		Type:      workshop.HostWorkshop,
		What:      testSDKSource,
		MakeWhat:  true,
		Where:     "/opt/data",
		MakeWhere: true,
		Mode:      0o755,
	}
	w.Profiles["mount-conflict"] = p

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
					Name:        "mount-conflict",
					Version:     "20200401.3f3a63f",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build2,
					InstallTime: s.installTime,
					Mounts: []*Mount{
						{
							HostSource:     testSDKSource,
							WorkshopTarget: "/opt/data",
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "somebound",
								Sdk:       "mount-conflict",
								Name:      "photos",
							},
						},
					},
				},
				{
					Name:        "system",
					Revision:    system.SystemSdkRevision.String(),
					InstallTime: s.installTime,
				},
				{
					Name:        "test-sdk",
					Version:     "0.1.2",
					Channel:     "latest/stable",
					Revision:    "1",
					BuildTime:   &build1,
					InstallTime: s.installTime,
					Mounts: []*Mount{
						{
							HostSource:     testSDKSource,
							WorkshopTarget: "/opt/data",
							Plug: sdk.PlugRef{
								ProjectId: s.project.ProjectId,
								Workshop:  "somebound",
								Sdk:       "test-sdk",
								Name:      "data",
							},
						},
					},
				},
			},
		},
		Path: workshop.Filepath(s.project.Path, "somebound"),
	})
}

func (s *apiSuite) TestGetWorkshopActions(c *check.C) {
	// Setup (create a running workshop with a mount)
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "actions", actions)

	// Get Workshop actions
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}/actions")
	s.vars = map[string]string{"id": s.project.ProjectId, "name": "actions"}
	req, err := s.createProjectsRequest("GET", "/v1/projects/"+s.project.ProjectId+"/workshops/actions/actions", nil)
	c.Assert(err, check.IsNil)

	rsp := v1GetProjectWorkshopActions(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)

	c.Check(rsp.Result, check.DeepEquals, map[string]Action{
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
		c.Check(rsp.Type, check.Equals, expected[num].Type, check.Commentf("case: %v", num))
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
			ticker.Stop()

			st.Lock()
			c.Check(change.Kind(), check.Equals, expected[num].Kind)
			c.Check(change.Summary(), check.Equals, expected[num].Summary)

			if err := change.Err(); err != nil {
				c.Check(err, check.ErrorMatches, expected[num].ChangeErr, check.Commentf("case: %v", num))
			} else if err := change.WaitErr(); err != nil {
				c.Check(err, check.ErrorMatches, expected[num].ChangeErr, check.Commentf("case: %v", num))
			} else {
				c.Check(expected[num].ChangeErr, check.Equals, "")
			}
			st.Unlock()
		}
	}
}

func (s *apiSuite) createWFile(c *check.C, name, yaml string) {
	path := workshop.Filepath(s.project.Path, name)

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(yaml), 0o644)
	c.Assert(err, check.IsNil)
}

func storeDownloadWithSaveRestore(ctx context.Context, setup sdk.Setup, report *progress.Reporter) error {
	if err := storeDownload(ctx, setup, report); err != nil || setup.Source == sdk.SystemSource {
		return err
	}
	hooksdir := filepath.Join(setup.Filepath(), "sdk", "hooks")
	if err := os.WriteFile(filepath.Join(hooksdir, "save-state"), nil, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hooksdir, "restore-state"), nil, 0o644)
}

func storeDownload(ctx context.Context, setup sdk.Setup, report *progress.Reporter) error {
	// Emulate store action behaviour which would reuse the existing SDK if
	// present.
	sdkdir := setup.Filepath()
	_, isDir, err := osutil.ExistsIsDir(sdkdir)
	if isDir {
		return nil
	}
	if err != nil {
		return err
	}

	if setup.Source == sdk.SystemSource {
		return os.CopyFS(sdkdir, system.SystemSdkFs)
	}
	metadir := filepath.Join(sdkdir, "meta")
	hooksdir := filepath.Join(sdkdir, "sdk", "hooks")
	return mockSdk(metadir, hooksdir, apiSuiteSdks[setup.Name].SdkYAML)
}

func storeAction(ctx context.Context, actions []sdk.SdkAction) ([]sdk.Meta, error) {
	sdks := make([]sdk.Meta, 0, len(actions))
	for _, act := range actions {
		setup := apiSuiteSdks[act.Name].Setup
		setup.Channel = act.Channel
		sdkYaml := apiSuiteSdks[act.Name].SdkYAML
		sdks = append(sdks, sdk.Meta{Setup: setup, SdkYAML: sdkYaml})
	}
	return sdks, nil
}

func mockSdk(metadir, hooksdir string, meta string) error {
	if err := os.MkdirAll(metadir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(metadir, "sdk.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Write([]byte(meta)); err != nil {
		return err
	}

	return os.MkdirAll(hooksdir, 0o755)
}

func (s *apiSuite) mockTrySdk(c *check.C, name, filename string, meta string) {
	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sdkdir := filepath.Join(workshop.TrySdkDir(userDataDir, name), filename)
	metadir := filepath.Join(sdkdir, "meta")
	hooksdir := filepath.Join(sdkdir, "sdk", "hooks")
	c.Assert(mockSdk(metadir, hooksdir, meta), check.IsNil)

	c.Assert(os.WriteFile(sdkdir+".yaml", []byte(meta), 0o666), check.IsNil)

	hash := sha3.New384()
	_, _ = hash.Write([]byte(meta))
	digest := fmt.Appendf(nil, "%x", hash.Sum(nil))
	c.Assert(os.WriteFile(sdkdir+".sha3-384", digest, 0o666), check.IsNil)
}

func (s *apiSuite) mockProjectSdk(c *check.C, name string, meta string) {
	sdkdir := workshop.ProjectSdkPath(s.project.Path, name)
	hooksdir := filepath.Join(sdkdir, "hooks")
	c.Assert(mockSdk(sdkdir, hooksdir, meta), check.IsNil)
}

func (s *apiSuite) mockSketchSdk(c *check.C, ws string, meta string) {
	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sdkdir := workshop.SketchSdkCurrent(userDataDir, s.project.ProjectId, ws)
	hooksdir := filepath.Join(sdkdir, "hooks")
	c.Assert(mockSdk(sdkdir, hooksdir, meta), check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopBasic(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "basic", basic)
	s.createWFile(c, "basic-invalid", basic_invalid)
	defer s.store.SetDownloadCallback(storeDownload)()

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
			Message: "cannot launch: no workshop names provided",
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

	c.Check(workshop.AptCacheDir(wp.Project.ProjectId, "basic"), testutil.DirEquals, []string{})

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	c.Assert(repo.Slot(s.project.ProjectId, "workshopslot", "test-sdk", "training"), check.Not(check.IsNil))
}

func (s *apiSuite) TestLaunchWorkshopFailed(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

	s.secBackend.SetupCallback = func(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
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

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1"})
}

func (s *apiSuite) TestLaunchWorkshopNoTrySdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks_try)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	trydir := workshop.TrySdkDir(userDataDir, "test-sdk")
	message := fmt.Sprintf(`cannot launch "manysdks": SDK "try-test-sdk" not found: open %s: no such file or directory`, trydir)
	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: message,
		},
	}
	s.runActionTest(c, requests, expected)

	c.Assert(os.MkdirAll(trydir, os.ModePerm), check.IsNil)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	filename := fmt.Sprintf("test-sdk_%s_ubuntu@22.04.sdk", arch.DpkgArchitecture())
	tryfile := filepath.Join(trydir, filename)
	message = fmt.Sprintf(`cannot launch "manysdks": SDK "try-test-sdk" not found: openat %s: no such file or directory`, tryfile)
	expected = []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: message,
		},
	}
	s.runActionTest(c, requests, expected)

	c.Assert(os.WriteFile(tryfile, nil, 0o666), check.IsNil)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	trydigest := tryfile + ".sha3-384"
	message = fmt.Sprintf(`cannot launch "manysdks": invalid SDK "try-test-sdk": openat %s: no such file or directory`, trydigest)
	expected = []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: message,
		},
	}
	s.runActionTest(c, requests, expected)

	c.Assert(os.WriteFile(trydigest, nil, 0o666), check.IsNil)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	trymeta := tryfile + ".yaml"
	message = fmt.Sprintf(`cannot launch "manysdks": invalid SDK "try-test-sdk": openat %s: no such file or directory`, trymeta)
	expected = []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: message,
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

func (s *apiSuite) TestLaunchWorkshopNoProjectSdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "manysdks", manysdks_project)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}

	message := fmt.Sprintf(`cannot launch "manysdks": stat %s/.workshop/test-sdk: no such file or directory`, s.project.Path)
	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: message,
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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	_, bound := connection.Plug.CheckBound()
	c.Assert(bound, check.Equals, true)
}

func (s *apiSuite) TestLaunchWorkshopBindPlugNoMasterPlug(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "masterunknown", masterunknown)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplug/test-sdk:training-plug %s/workshopplug/system:mount`, s.project.ProjectId, s.project.ProjectId))

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "test-sdk-2-1"})
}

func (s *apiSuite) TestLaunchWorkshopPlugAddedAndBound(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "workshopplugbound", workshopplugbound)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplugbound/test-sdk:training-plug %s/workshopplugbound/system:mount`, s.project.ProjectId, s.project.ProjectId))

	conns, err = repo.Connected(s.project.ProjectId, "workshopplugbound", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].ID(), check.Equals, fmt.Sprintf(`%s/workshopplugbound/test-sdk:data %s/workshopplugbound/system:mount`, s.project.ProjectId, s.project.ProjectId))
}

func (s *apiSuite) TestWorkshopConnectionsOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "workshopconns", workshopconns)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	c.Assert(repo.Slot(s.project.ProjectId, "workshopconns", "test-sdk-2", "training"), check.Not(check.IsNil))

	conns, err := repo.Connections(s.project.ProjectId, "workshopconns", "test-sdk")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk", Name: "data"},
			SlotRef: sdk.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "training"},
		},
	})

	conns, err = repo.Connections(s.project.ProjectId, "workshopconns", "test-sdk-2")
	c.Assert(err, check.IsNil)
	c.Assert(conns, testutil.DeepUnsortedMatches, []*interfaces.ConnRef{
		{
			PlugRef: sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "photos"},
			SlotRef: sdk.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: sdk.System.String(), Name: "mount"},
		},
		{
			PlugRef: sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "gpu"},
			SlotRef: sdk.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: sdk.System.String(), Name: "gpu"},
		},
		{
			PlugRef: sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk", Name: "data"},
			SlotRef: sdk.SlotRef{ProjectId: s.project.ProjectId, Workshop: "workshopconns", Sdk: "test-sdk-2", Name: "training"},
		},
	})
}

func (s *apiSuite) TestWorkshopConnectionsUnknownPlug(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "workshopbrokenconn", workshopbrokenconn)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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

	conns, err = repo.Connected(s.project.ProjectId, "connsplugbound", "mount-conflict", "photos")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].SlotRef.Name, check.Equals, "training")

	connection, err := repo.Connection(conns[0])
	c.Assert(err, check.IsNil)
	_, bound := connection.Plug.CheckBound()
	c.Assert(bound, check.Equals, true)
}

type expectedWorkshop struct {
	name        string
	base        string
	sdks        []sdk.Setup
	connections []string
	plugs       []string
	slots       []string
}

func (s *apiSuite) ensureWorkshops(c *check.C, want []expectedWorkshop) {
	got, err := s.b.ProjectWorkshops(s.ctx)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.HasLen, len(want), check.Commentf("expected %d workshops, got %d", len(want), len(got)))

	for _, w := range got {
		idx := slices.IndexFunc(want, func(ws expectedWorkshop) bool { return ws.name == w.Name })
		c.Assert(idx >= 0, check.Equals, true)

		wantws := want[idx]
		c.Assert(w.Name, check.Equals, wantws.name)
		c.Assert(w.File.Base, check.Equals, wantws.base)

		c.Assert(w.Sdks, check.HasLen, len(wantws.sdks))
		for _, sk := range wantws.sdks {
			sk.Sha3_384 = w.Sdks[sk.Name].Sha3_384
			c.Check(sk.Sha3_384, check.Not(check.Equals), "")
			c.Assert(w.Sdks[sk.Name].Setup, check.DeepEquals, sk)
			c.Check(w.Sdks[sk.Name].InstallTime, check.Equals, s.installTime)
		}

		repo := s.d.overlord.InterfaceManager().Repository()
		for _, conn := range wantws.connections {
			connref, err := interfaces.ParseConnRef(conn)
			c.Assert(err, check.IsNil)

			_, err = repo.Connection(connref)
			c.Assert(err, check.IsNil)
		}

		for _, plug := range wantws.plugs {
			ref := strings.Split(plug, ":")
			c.Assert(ref, check.HasLen, 2)

			p := repo.Plug(s.project.ProjectId, wantws.name, ref[0], ref[1])
			c.Assert(p, check.NotNil, check.Commentf("plug %q is not found in the repository", plug))
		}

		for _, slot := range wantws.slots {
			ref := strings.Split(slot, ":")
			c.Assert(ref, check.HasLen, 2)

			s := repo.Slot(s.project.ProjectId, wantws.name, ref[0], ref[1])
			c.Assert(s, check.NotNil, check.Commentf("slot %q is not found in the repository", slot))
		}

		var allconns []*interfaces.ConnRef
		var allplugs []*sdk.PlugInfo
		var allslots []*sdk.SlotInfo
		// Ensure there are no unexpected plugs, slots or conns in the
		// repository for the workshop.
		for _, sk := range wantws.sdks {
			conns, err := repo.Connections(s.project.ProjectId, wantws.name, sk.Name)
			c.Assert(err, check.IsNil)
			for _, conn := range conns {
				if !slices.ContainsFunc(allconns, func(a *interfaces.ConnRef) bool {
					return *a == *conn
				}) {
					allconns = append(allconns, conn)
				}
			}

			plugs := repo.Plugs(s.project.ProjectId, wantws.name, sk.Name)
			allplugs = append(allplugs, plugs...)

			slots := repo.Slots(s.project.ProjectId, wantws.name, sk.Name)
			allslots = append(allslots, slots...)
		}
		c.Assert(allconns, check.HasLen, len(wantws.connections))
		c.Assert(allplugs, check.HasLen, len(wantws.plugs))
		c.Assert(allslots, check.HasLen, len(wantws.slots))
	}
}

// ensureSdkVolumesAfterCooldown ensures that the SDK volumes are removed if unused by
// setting the cooldown time to 0 and running the state engine multiple times to
// trigger the garbage collection.
func (s *apiSuite) ensureSdkVolumesAfterCooldown(c *check.C, want []string) {
	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.d.overlord.StateEngine().Ensure(), check.IsNil)
		s.d.overlord.StateEngine().Wait()
	}

	vols, err := s.b.Volumes(s.ctx, "sdk")
	c.Assert(err, check.IsNil)
	c.Assert(vols, check.HasLen, len(want))

	for _, wv := range want {
		v := slices.IndexFunc(vols, func(v workshop.VolumeInfo) bool {
			return v.Name == wv
		})
		c.Assert(v, check.Not(check.Equals), -1)
		vol := vols[v]
		c.Check(vol.Kind, check.Equals, "sdk")
		c.Check(vol.Sha3_384, check.Not(check.Equals), "")
		c.Check(sdk.VolumeName(vol.Sdk, vol.Revision), check.Equals, vol.Name)
		c.Check(vol.Metadata, check.Not(check.Equals), "")
	}
}

func (s *apiSuite) checkSnapshotCalls(c *check.C, name string, sdks []string) {
	wpCalls := []fakebackend.SnapshotCall{}
	for _, sc := range s.b.SnapshotCalls {
		if sc.Workshop == name {
			wpCalls = append(wpCalls, sc)
		}
	}

	c.Assert(wpCalls, check.HasLen, len(sdks))

	for i, sk := range sdks {
		c.Assert(wpCalls[i].Sdk, check.Equals, sk)
	}
}

func (s *apiSuite) checkRestoreCalls(c *check.C, name string, sdks []string, files []string) {
	wpCalls := []fakebackend.RestoreCall{}
	for _, sc := range s.b.RestoreCalls {
		if sc.Workshop == name {
			wpCalls = append(wpCalls, sc)
		}
	}

	c.Assert(wpCalls, check.HasLen, len(sdks))

	for i, sk := range sdks {
		c.Assert(wpCalls[i].Sdk, check.Equals, sk)

		var f workshop.File
		err := yaml.Unmarshal([]byte(files[i]), &f)
		c.Assert(err, check.IsNil)
		c.Assert(wpCalls[i].File, check.DeepEquals, &f)
	}
}

func (s *apiSuite) checkHookCalls(c *check.C, name string, sdks []string, hooks []hookstate.WorkshopHookType) {
	wpCalls := []*fakebackend.ExecCall{}
	for _, ec := range s.b.ExecCalls {
		if ec.Name == name {
			wpCalls = append(wpCalls, ec)
		}
	}

	c.Assert(wpCalls, check.HasLen, len(sdks))

	for i, sk := range sdks {
		c.Check(wpCalls[i].Args.WorkDir, check.Equals, sdk.SdkHooksDir(sk))
		command := wpCalls[i].Args.Command
		c.Check(command[len(command)-1], check.Equals, sdk.SdkHookPath(sk, hooks[i].String()))
	}
}

func (s *apiSuite) TestRefreshMany(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	s.createWFile(c, "manysdks", manysdks)
	s.createWFile(c, "somebound", somebound)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic", "manysdks", "somebound"],"action":"launch"}`),
	}
	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic", "manysdks", "somebound" workshops`,
		},
	}
	s.runActionTest(c, requests, expected)

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "test-sdk-2-1", "mount-conflict-1"})

	s.createWFile(c, "manysdks", manysdks_allremoved)
	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic", "manysdks"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic", "manysdks" workshops`,
		},
	}

	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "somebound",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "mount-conflict", Channel: "latest/stable", Revision: sdk.R(1)},
		}, connections: []string{
			"b8639dea/somebound/test-sdk:data b8639dea/somebound/system:mount",
			"b8639dea/somebound/mount-conflict:photos b8639dea/somebound/system:mount",
			"b8639dea/somebound/mount-conflict:gpu b8639dea/somebound/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"mount-conflict:photos",
			"mount-conflict:gpu",
		},
		slots: []string{
			"mount-conflict:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}, {
		name: "basic",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}, {
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "basic", []string{
		"system",
	})

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkSnapshotCalls(c, "somebound", []string{
		"system",
		"test-sdk",
		"mount-conflict",
	})

	s.checkRestoreCalls(c, "basic", nil, nil)
	s.checkRestoreCalls(c, "somebound", nil, nil)
	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks_allremoved})

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "mount-conflict-1"})
}

func (s *apiSuite) TestRefreshAddSdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.checkSnapshotCalls(c, "basic", []string{
		"system",
	})

	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "basic", basic_refreshed)

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

	want := []expectedWorkshop{{
		name: "basic",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Source: sdk.ProjectSource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/basic/test-sdk:data b8639dea/basic/system:mount",
			"b8639dea/basic/test-sdk-2:photos b8639dea/basic/system:mount",
			"b8639dea/basic/test-sdk-2:gpu b8639dea/basic/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "basic", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "basic", []string{"system"}, []string{basic_refreshed})
}

func (s *apiSuite) TestRefreshInsertNewSdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_extended)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-3", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"test-sdk-3",
		"test-sdk-2",
		"test-sdk",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks_extended})

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "test-sdk-2-1", "test-sdk-3-1"})
}

func (s *apiSuite) TestRefreshRemoveSdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_minusone)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
		},
		plugs: []string{
			"test-sdk:data",
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk"}, []string{manysdks_minusone})

	s.ensureSdkVolumesAfterCooldown(c, []string{"test-sdk-1", "system-1"})
}

func (s *apiSuite) TestRefreshNewSdkChannel(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_newchan)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/edge", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks_newchan})
}

func updateSdkStoreRev(name string, rev int, meta string) func() {
	oldrev := apiSuiteSdks[name]

	newrev := oldrev
	newrev.Revision = sdk.R(rev)
	newrev.SdkYAML = meta
	apiSuiteSdks[name] = newrev

	return func() {
		apiSuiteSdks[name] = oldrev
	}
}

func (s *apiSuite) TestRefreshSdkNewRevision(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	defer updateSdkStoreRev("test-sdk", 2, testsdk_r2)()

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(2)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:ssh-agent",
			"test-sdk:desktop",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks})

	s.ensureSdkVolumesAfterCooldown(c, []string{
		"system-1",
		"test-sdk-2",
		"test-sdk-2-1",
	})
}

func (s *apiSuite) TestRefreshSaveAndRestoreState(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownloadWithSaveRestore)()

	// Launch
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

	s.checkHookCalls(c, "manysdks", nil, nil)

	// Refresh saves and restores state for intact SDKs.
	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options":{"mode":"transactional","refresh-option":"restore"}}`),
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

	s.checkHookCalls(c, "manysdks", []string{
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	}, []hookstate.WorkshopHookType{
		hookstate.SaveState,
		hookstate.SaveState,
		hookstate.RestoreState,
		hookstate.RestoreState,
	})
	s.b.ExecCalls = nil

	// Refresh saves and restores state for updated SDKs.
	defer updateSdkStoreRev("test-sdk", 2, testsdk_r2)()

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options":{"mode":"transactional","refresh-option":"update"}}`),
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

	s.checkHookCalls(c, "manysdks", []string{
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	}, []hookstate.WorkshopHookType{
		hookstate.SaveState,
		hookstate.SaveState,
		hookstate.RestoreState,
		hookstate.RestoreState,
	})
	s.b.ExecCalls = nil

	// Refresh doesn't save or restore state for removed SDKs.
	s.createWFile(c, "manysdks", manysdks_minusone)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options":{"mode":"transactional","refresh-option":"update"}}`),
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

	s.checkHookCalls(c, "manysdks", []string{
		"test-sdk",
		"test-sdk",
	}, []hookstate.WorkshopHookType{
		hookstate.SaveState,
		hookstate.RestoreState,
	})
	s.b.ExecCalls = nil

	// Refresh doesn't save or restore state for newly installed SDKs.
	s.createWFile(c, "manysdks", manysdks)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options":{"mode":"transactional","refresh-option":"update"}}`),
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

	wp, err := s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)
	_, err = wp.SdkInfo(s.ctx, "test-sdk-2")
	c.Assert(err, check.IsNil)

	s.checkHookCalls(c, "manysdks", []string{
		"test-sdk",
		"test-sdk",
	}, []hookstate.WorkshopHookType{
		hookstate.SaveState,
		hookstate.RestoreState,
	})
}

func (s *apiSuite) TestRefreshTrySdk(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.mockTrySdk(c, "test-sdk", "test-sdk_all.sdk", testsdk)
	s.mockTrySdk(c, "test-sdk-2", "test-sdk-2_all_ubuntu@22.04.sdk", testsdk2)
	s.createWFile(c, "manysdks", manysdks_try)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Source: sdk.TrySource, Revision: sdk.R(-1)},
			{Name: "test-sdk-2", Source: sdk.TrySource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	c.Assert(os.RemoveAll(workshop.TrySdkDir(userDataDir, "test-sdk")), check.IsNil)
	s.mockTrySdk(c, "test-sdk", "test-sdk_all.sdk", testsdk_r2)

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

	want = []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Source: sdk.TrySource, Revision: sdk.R(-2)},
			{Name: "test-sdk-2", Source: sdk.TrySource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:ssh-agent",
			"test-sdk:desktop",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks_try})
}

func (s *apiSuite) TestRefreshSdkNewProjectFiles(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.mockProjectSdk(c, "test-sdk", testsdk)
	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "manysdks", manysdks_project)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Source: sdk.ProjectSource, Revision: sdk.R(-1)},
			{Name: "test-sdk-2", Source: sdk.ProjectSource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	c.Assert(os.RemoveAll(workshop.ProjectSdkPath(s.project.Path, "test-sdk")), check.IsNil)
	s.mockProjectSdk(c, "test-sdk", testsdk_r2)

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

	want = []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Source: sdk.ProjectSource, Revision: sdk.R(-2)},
			{Name: "test-sdk-2", Source: sdk.ProjectSource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:ssh-agent",
			"test-sdk:desktop",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}
	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"system"}, []string{manysdks_project})
}

func (s *apiSuite) TestRefreshConnectionsChanged(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_connsadded)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/test-sdk-2:data-slot",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk-2"}, []string{manysdks_connsadded})
}

func (s *apiSuite) TestRefreshSdkRecordPlugChanged(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_plugadded)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk:new-plug b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk:new-plug",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk-2"}, []string{manysdks_plugadded})
}

func (s *apiSuite) TestRefreshSystemDefinitionExtended(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_system_extended)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
		},
		plugs: []string{
			"test-sdk:data",
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
			"system:tunnel",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk"}, []string{manysdks_system_extended})
}

func (s *apiSuite) TestRefreshSdkRecordPlugRemoved(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks_plugadded)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk-2"}, []string{manysdks})
}

func (s *apiSuite) TestRefreshNoChanges(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: "no updates available",
			Kind:    string(errorKindNoUpdatesAvailable),
		},
	}

	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", nil, nil)

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "test-sdk-2-1"})
}

func (s *apiSuite) TestRefreshRestore(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options":{"mode":"transactional","refresh-option":"restore"}}`),
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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk-2"}, []string{manysdks})

	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1", "test-sdk-1", "test-sdk-2-1"})
}

func (s *apiSuite) TestRefreshBaseChange(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_newbase)

	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@24.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{}, []string{})
}

func (s *apiSuite) TestRefreshBaseUpdate(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

	oldGetBase := s.b.GetBaseCallback
	s.b.GetBaseCallback = func(ctx context.Context, base string) (workshop.BaseImage, error) {
		return workshop.BaseImage{Name: base, Fingerprint: "oldimage123"}, nil
	}
	defer func() { s.b.GetBaseCallback = oldGetBase }()

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

	wp, err := s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)
	c.Check(wp.Image, check.Equals, workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "oldimage123"})

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

	s.b.GetBaseCallback = func(ctx context.Context, base string) (workshop.BaseImage, error) {
		return workshop.BaseImage{Name: base, Fingerprint: "newimage321"}, nil
	}

	s.runActionTest(c, requests, expected)

	wp, err = s.b.Workshop(s.ctx, "manysdks")
	c.Assert(err, check.IsNil)
	c.Check(wp.Image, check.Equals, workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "newimage321"})

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{}, []string{})
}

func (s *apiSuite) TestRefreshSystemSdkInstalledFirst(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.createWFile(c, "manysdks", manysdks_system)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_minusone)

	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
		},
		plugs: []string{
			"test-sdk:data",
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"system",
		"test-sdk",
	})

	s.checkRestoreCalls(c, "manysdks", []string{}, []string{})
}

func (s *apiSuite) TestRefreshAllSdksRemoved(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer func() { _ = s.d.Overlord().Stop() }()
	// Setup
	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "basic", basic_refreshed)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "basic", basic)

	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "basic",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "basic", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "basic", []string{"system"}, []string{basic})
}

func (s *apiSuite) TestRefreshRestoreFromStash(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	// Setup "refresh" with a new workshop that will trigger an error
	s.createWFile(c, "manysdks", manysdks_broken)
	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh"}`),
	}
	expected = []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "refresh",
			Summary:   `Refresh "manysdks" workshop`,
			ChangeErr: `(?s).*SDK "manysdks/test-sdk" has no plug named "data-non-existent".*`,
		},
	}
	s.runActionTest(c, requests, expected)

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "test-sdk-2", Channel: "latest/stable", Revision: sdk.R(1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:photos b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk-2:gpu b8639dea/manysdks/system:gpu",
		},
		plugs: []string{
			"test-sdk:data",
			"test-sdk-2:photos",
			"test-sdk-2:gpu",
		},
		slots: []string{
			"test-sdk-2:data-slot",
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	s.checkSnapshotCalls(c, "manysdks", []string{
		"system",
		"test-sdk",
		"test-sdk-2",
	})

	s.checkRestoreCalls(c, "manysdks", []string{"test-sdk-2"}, []string{manysdks_broken})
}

func (s *apiSuite) TestRefreshNoRefreshInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

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

func (s *apiSuite) TestRefreshContinue(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.RemoveCallback = func(sdkName string) error {
		var err error
		errOnce.Do(func() { err = errors.New("cannot remove profile") })
		return err
	}

	// Setup
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

	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "basic", basic_refreshed)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"continue"}}`),
	}
	expected = []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "refresh",
			Summary:   `Refresh "basic" workshop`,
			ChangeErr: `(?s).*\(cannot remove profile\)`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	// This will be called twice: first, on the refresh attempt, second, on the
	// refresh --continue, which will be successfull and allow the refresh to
	// finish.
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 2)

	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no refresh in progress after continue was successful
	c.Assert(conflict.CheckChangeConflict(st, s.project.ProjectId, "basic", nil), check.IsNil)
}

func (s *apiSuite) TestRefreshAbort(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.RemoveCallback = func(sdkName string) error {
		var err error
		errOnce.Do(func() { err = errors.New("cannot remove profile") })
		return err
	}

	// Setup
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

	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "basic", basic_refreshed)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"abort"}}`),
	}
	expected = []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "refresh",
			Summary:   `Refresh "basic" workshop`,
			ChangeErr: `(?s).*\(cannot remove profile\)`,
		},
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "refresh",
			Summary: `Refresh "basic" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
	st := s.d.state
	st.Lock()
	defer st.Unlock()
	// no refresh in progress after continue was successful
	c.Assert(conflict.CheckChangeConflict(st, s.project.ProjectId, "basic", nil), check.IsNil)
}

// Tests the input validation logic of v1PostProjectWorkshop. Excludes any
// dispatch validation, these are covered by their own tests.
func (s *apiSuite) TestValidateIncorrectActionModeInputs(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	type table struct {
		cmd    string
		result map[string]string
	}

	s.vars = map[string]string{"id": s.project.ProjectId}

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
				"":              `cannot refresh "basic": workshop definition .*`,
				"transactional": `cannot refresh "basic": workshop definition .*`,
				"wait-on-error": `cannot refresh "basic": workshop definition .*`,
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
		for mode, experr := range cmd.result {
			// Construct request
			body := strings.NewReader(fmt.Sprintf(`{"names":["basic"],"action":"%s", "options": {"mode":"%s"}}`, cmd.cmd, mode))
			req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", body)
			c.Assert(err, check.IsNil)

			// Construct response
			exp := expectedResp{
				Type:    ResponseTypeError,
				Status:  http.StatusBadRequest,
				Message: experr,
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

func (s *apiSuite) TestValidateIncorrectRefreshOption(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.vars = map[string]string{"id": s.project.ProjectId}

	type table struct {
		action string
		mode   string
		option string
		result string
	}

	cmds := []table{
		{
			action: "refresh",
			mode:   "continue",
			option: "update",
			result: `cannot refresh: "refresh-option" is only applicable to "transactional" and "wait-on-error" modes; given: "continue"`,
		},
		{
			action: "refresh",
			mode:   "abort",
			option: "update",
			result: `cannot refresh: "refresh-option" is only applicable to "transactional" and "wait-on-error" modes; given: "abort"`,
		},
		{
			action: "launch",
			mode:   "abort",
			option: "update",
			result: `cannot launch: "refresh-option" is only valid for refresh actions`,
		},
	}

	for _, cmd := range cmds {
		// Construct request
		reqBody := strings.NewReader(fmt.Sprintf(`{"names":["basic"],"action":"%s", "options": {"mode":"%s","refresh-option":"%s"}}`, cmd.action, cmd.mode, cmd.option))
		req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops", reqBody)
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

func (s *apiSuite) TestValidateActionInputs(c *check.C) {
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

// ValidateSdkInfo is already covered by unit tests. This test ensures it's
// applied to every type of SDK on both launch and refresh.
func (s *apiSuite) TestValidateSdkInfo(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.mockProjectSdk(c, "test-sdk-2", testsdk2_invalid)
	s.createWFile(c, "basic", basic_refreshed)
	s.mockTrySdk(c, "test-sdk", "test-sdk_all.sdk", testsdk)
	s.mockTrySdk(c, "test-sdk-2", "test-sdk-2_all.sdk", testsdk2_invalid)
	s.createWFile(c, "manysdks", manysdks_try)
	s.createWFile(c, "wrongbase", wrongbase)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["wrongbase"],"action":"launch"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot launch "basic": SDK must be named "test-sdk-2" (now: "sdk-2")`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot launch "manysdks": SDK must be named "test-sdk-2" (now: "sdk-2")`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot launch "wrongbase": "test-sdk-3" SDK has "ubuntu@22.04" base; required: "ubuntu@24.04"`,
		},
	}

	s.runActionTest(c, requests, expected)

	s.createWFile(c, "basic", basic)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"launch"}`),
	}

	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "basic" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	s.mockSketchSdk(c, "basic", `name: illegal-sketch-name
`)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh"}`),
	}

	expected = []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot refresh "basic": SDK must be named "sketch" (now: "illegal-sketch-name")`,
		},
	}

	s.runActionTest(c, requests, expected)
}

func (s *apiSuite) TestLaunchWorkshopRefreshLaunchInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
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
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "manysdks" workshop`,
			ChangeErr: `(?s).*\(setup failed\)`,
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
	err := conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", nil)
	c.Check(err, check.ErrorMatches, `*. has "launch" change in progress`)
}

func (s *apiSuite) TestLaunchWorkshopContinueSuccess(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
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
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "manysdks" workshop`,
			ChangeErr: `(?s).*\(setup failed\)`,
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
	c.Assert(conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", nil), check.IsNil)
}

func (s *apiSuite) TestLaunchWorkshopNoRefreshInProgress(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.SetupCallback = func(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
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
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "launch",
			Summary:   `Launch "manysdks" workshop`,
			ChangeErr: `(?s).*\(setup failed\)`,
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
	c.Assert(conflict.CheckChangeConflict(st, s.project.ProjectId, "manysdks", nil), check.IsNil)
}

func (s *apiSuite) TestRefreshPartialOK(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "manysdks", manysdks)
	defer s.store.SetDownloadCallback(storeDownload)()

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

	s.createWFile(c, "manysdks", manysdks_minusone)
	s.mockSketchSdk(c, "manysdks", `name: sketch
base: ubuntu@22.04
`)

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

	want := []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "sketch", Source: sdk.SketchSource, Revision: sdk.R(-1)},
		},
		connections: []string{
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
		},
		plugs: []string{
			"test-sdk:data",
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sketchdir := workshop.SketchSdkCurrent(userDataDir, s.project.ProjectId, "manysdks")
	c.Assert(os.RemoveAll(sketchdir), check.IsNil)
	s.mockSketchSdk(c, "manysdks", `name: sketch
base: ubuntu@22.04
plugs:
  sketch-plug:
    interface: mount
    workshop-target: /etc
`)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
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

	want = []expectedWorkshop{{
		name: "manysdks",
		base: "ubuntu@22.04",
		sdks: []sdk.Setup{
			{Name: sdk.System.String(), Source: sdk.SystemSource, Revision: system.SystemSdkRevision},
			{Name: "test-sdk", Channel: "latest/stable", Revision: sdk.R(1)},
			{Name: "sketch", Source: sdk.SketchSource, Revision: sdk.R(-2)},
		},
		connections: []string{
			"b8639dea/manysdks/sketch:sketch-plug b8639dea/manysdks/system:mount",
			"b8639dea/manysdks/test-sdk:data b8639dea/manysdks/system:mount",
		},
		plugs: []string{
			"sketch:sketch-plug",
			"test-sdk:data",
		},
		slots: []string{
			"system:camera",
			"system:desktop",
			"system:gpu",
			"system:mount",
			"system:ssh-agent",
		},
	}}

	s.ensureWorkshops(c, want)
}

func (s *apiSuite) TestRefreshConflictChange(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

	var errOnce sync.Once
	s.secBackend.RemoveCallback = func(sdkName string) error {
		var err error
		errOnce.Do(func() { err = errors.New("cannot remove profile") })
		return err
	}

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

	s.mockProjectSdk(c, "test-sdk-2", testsdk2)
	s.createWFile(c, "basic", basic_refreshed)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh","options": {"mode":"wait-on-error"}}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh"}`),
		bytes.NewBufferString(`{"names":["basic"],"action":"refresh", "options": {"mode":"transactional", "refresh-option":"restore"}}`),
	}
	expected = []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "refresh",
			Summary:   `Refresh "basic" workshop`,
			ChangeErr: `(?s).*\(cannot remove profile\)`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot refresh "basic": waiting on error`,
			Summary: `Refresh "basic" workshop`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot refresh "basic": waiting on error`,
			Summary: `Refresh "basic" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	// This will be called by the first refresh, the others fail earlier.
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *apiSuite) TestSDKInstallationOrder(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Install test-sdk-2 first.
	s.createWFile(c, "manysdks", manysdks_reversed)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
		bytes.NewBufferString(`{"names":["manysdks"],"action":"remove"}`),
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
			Kind:    "remove",
			Summary: `Remove "manysdks" workshop`,
		},
	}
	s.runActionTest(c, requests, expected)

	s1 := apiSuiteSdks["test-sdk"].Setup
	s2 := apiSuiteSdks["test-sdk-2"].Setup

	c.Assert(s.b.AttachVolumeCalls, check.DeepEquals, []fakebackend.AttachVolumeCall{
		{Workshop: "manysdks", Name: sdk.VolumeName(sdk.System.String(), system.SystemSdkRevision)},
		{Workshop: "manysdks", Name: sdk.VolumeName(s2.Name, s2.Revision)},
		{Workshop: "manysdks", Name: sdk.VolumeName(s1.Name, s1.Revision)},
	})
	s.b.AttachVolumeCalls = s.b.AttachVolumeCalls[:0]

	// Install test-sdk first this time.
	s.createWFile(c, "manysdks", manysdks)

	requests = []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["manysdks"],"action":"launch"}`),
	}
	expected = []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "launch",
			Summary: `Launch "manysdks" workshop`,
		},
	}

	s.runActionTest(c, requests, expected)

	c.Assert(s.b.AttachVolumeCalls, check.DeepEquals, []fakebackend.AttachVolumeCall{
		{Workshop: "manysdks", Name: sdk.VolumeName(sdk.System.String(), system.SystemSdkRevision)},
		{Workshop: "manysdks", Name: sdk.VolumeName(s1.Name, s1.Revision)},
		{Workshop: "manysdks", Name: sdk.VolumeName(s2.Name, s2.Revision)},
	})
}

func (s *apiSuite) TestStartWorkshop(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()
	// Setup
	s.createWFile(c, "basic", basic)
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	defer s.store.SetDownloadCallback(storeDownload)()

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
	s.ensureSdkVolumesAfterCooldown(c, []string{"system-1"})
}

func (s *apiSuite) TestRemoveWorkshopNotFound(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	// Setup
	s.createWFile(c, "workshopconns", workshopconns)
	defer s.store.SetDownloadCallback(storeDownload)()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"names":["workshopconns"],"action":"remove"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot remove "workshopconns": workshop not launched`,
		},
	}
	s.runActionTest(c, requests, expected)

	s.d.state.Lock()
	defer s.d.state.Unlock()
	changes := s.d.state.Changes()
	removeChange := changes[len(changes)-1]
	c.Check(removeChange.Kind(), check.Equals, "remove")
	c.Assert(removeChange.IsReady(), check.Equals, true)
}
