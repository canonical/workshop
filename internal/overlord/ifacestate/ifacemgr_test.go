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

package ifacestate_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"os/user"
	"path/filepath"
	"slices"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/syscheck"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type interfaceManagerSuite struct {
	testutil.BaseTest
	o          *overlord.Overlord
	state      *state.State
	se         *overlord.StateEngine
	runner     *state.TaskRunner
	ctx        context.Context
	wsbackend  workshop.Backend
	user       *user.User
	prj        workshop.Project
	secBackend *ifacetest.TestSecurityBackend

	restoreProjectId  func()
	restoreUserLookup func()
	restoreUserEnv    func()
}

var _ = check.Suite(&interfaceManagerSuite{})

func (s *interfaceManagerSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	var err error

	s.user = &user.User{Username: "testuser", HomeDir: c.MkDir()}
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser" {
			return nil, user.UnknownUserError("not found")
		}
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

	s.o = overlord.Fake()
	s.state = s.o.State()

	s.se = s.o.StateEngine()
	s.runner = state.NewTaskRunner(s.state)
	s.secBackend = &ifacetest.TestSecurityBackend{}

	s.restoreProjectId = testutil.FakeFunc(func() (string, error) { return "42424242", nil }, &workshop.NewProjectId)

	s.wsbackend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.wsbackend)

	s.ctx = context.WithValue(context.Background(), workshop.ContextUser, "testuser")
	prj, _, err := s.wsbackend.CreateOrLoadProject(s.ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.prj = *prj
	s.ctx = context.WithValue(s.ctx, workshop.ContextProjectId, s.prj.ProjectId)

	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {}))
}

func (s *interfaceManagerSuite) TearDownTest(c *check.C) {
	s.restoreProjectId()
	s.restoreUserEnv()
	s.restoreUserLookup()
	s.BaseTest.TearDownTest(c)
}

var systemYaml = `name: system
base: ubuntu@22.04
type: system
slots:
  mount:
    interface: mount
`

func (s *interfaceManagerSuite) mockSdk(c *check.C, meta sdk.Meta) {
	vfs := c.MkDir()

	path := filepath.Join(vfs, "meta", "sdk.yaml")
	err := os.MkdirAll(filepath.Dir(path), 0755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(path, []byte(meta.SdkYAML), 0644)
	c.Assert(err, check.IsNil)

	s.state.Lock()
	be := s.o.WorkshopBackend()
	s.state.Unlock()
	file, err := os.Open(vfs)
	c.Assert(err, check.IsNil)
	defer file.Close()
	if err := be.ImportSdk(s.ctx, meta, file); err != nil {
		c.Assert(err, testutil.ErrorIs, workshop.ErrVolumeAlreadyExists)
	}
}

func (s *interfaceManagerSuite) launchWorkshop(c *check.C, ws string, sdks []sdk.Meta) *workshop.Workshop {
	ctx := context.WithValue(s.ctx, workshop.ContextProjectId, s.prj.ProjectId)

	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	var workshopFile = bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	var wf workshop.File
	err = yaml.Unmarshal(workshopFile.Bytes(), &wf)
	c.Assert(err, check.IsNil)

	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, "fakeimage123")
	err = s.wsbackend.LaunchOrRebuildWorkshop(ctx, &wf, snapshot)
	c.Assert(err, check.IsNil)

	wsfs, err := s.wsbackend.WorkshopFs(ctx, ws)
	c.Assert(err, check.IsNil)
	defer wsfs.Close()

	systemSetup := sdk.Setup{
		Name:     sdk.System.String(),
		Source:   sdk.SystemSource,
		Revision: sdk.R(1),
		Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
	}
	allsdks := slices.Insert(sdks, 0, sdk.Meta{Setup: systemSetup, SdkYAML: systemYaml})

	for _, meta := range allsdks {
		s.mockSdk(c, meta)
	}

	w, err := s.wsbackend.Workshop(ctx, ws)
	c.Assert(err, check.IsNil)

	s.state.Lock()
	be := s.o.WorkshopBackend()
	s.state.Unlock()

	for _, sk := range allsdks {
		err = be.InstallSdk(ctx, ws, sk.Setup)
		c.Assert(err, check.IsNil)
	}

	return w
}

func (s *interfaceManagerSuite) TestManagerReloadsConnections(c *check.C) {
	var consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr: plug-value
`

	s.launchWorkshop(c, "ws", []sdk.Meta{
		{Setup: consumer.Setup, SdkYAML: consumerYaml},
	})

	s.state.Lock()
	key := fmt.Sprintf("%s/ws/consumer:plug %s/ws/system:mount", s.prj.ProjectId, s.prj.ProjectId)
	s.state.Set("conns", map[string]any{
		key: map[string]any{
			"interface": "mount",
			"plug-static": map[string]any{
				"mount": "foo",
				"attr":  "stored-value",
			},
			"slot-static": map[string]any{
				"mount": "foo",
				"attr":  "stored-value",
			},
		},
	})
	s.state.Unlock()

	defer syscheck.MockChecks()()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	repo := mgr.Repository()

	ifaces := repo.Interfaces()
	cref := &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: sdk.System.String(), Name: "mount"}}
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{cref})

	conn, err := repo.Connection(cref)
	c.Assert(err, check.IsNil)
	c.Assert(conn.Plug.Name(), check.Equals, "plug")
	c.Assert(conn.Plug.StaticAttrs(), check.DeepEquals, map[string]any{
		"mount": "foo",
		"attr":  "stored-value",
	})
	c.Assert(conn.Slot.Name(), check.Equals, "mount")
	c.Assert(conn.Slot.StaticAttrs(), check.DeepEquals, map[string]any{
		"mount": "foo",
		"attr":  "stored-value",
	})

	// Check the InterfaceManager continues to report as ready after StartUp.
	err = syscheck.CheckSystem()
	c.Assert(err, check.IsNil)

	ifaces = repo.Interfaces()
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{cref})
}

func (s *interfaceManagerSuite) TestManagerReloadsConnectionsAfterError(c *check.C) {
	var consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr: plug-value
`

	s.launchWorkshop(c, "ws", []sdk.Meta{
		{Setup: consumer.Setup, SdkYAML: consumerYaml},
	})

	s.state.Lock()
	s.state.Set("conns", "bad-type")
	s.state.Unlock()

	defer syscheck.MockChecks()()

	logs, restore := logger.MockLogger()
	defer restore()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)
	c.Check(logs.String(), check.Matches, "(?s).*: cannot decode data about existing connections: .*")

	repo := mgr.Repository()

	ifaces := repo.Interfaces()
	c.Check(ifaces.Plugs, check.HasLen, 0)
	c.Check(ifaces.Slots, check.HasLen, 0)
	c.Check(ifaces.Connections, check.HasLen, 0)

	// Check we remain in degraded mode, with an empty repo.
	err = syscheck.CheckSystem()
	c.Assert(err, check.ErrorMatches, "cannot decode data about existing connections: .*")

	ifaces = repo.Interfaces()
	c.Check(ifaces.Plugs, check.HasLen, 0)
	c.Check(ifaces.Slots, check.HasLen, 0)
	c.Check(ifaces.Connections, check.HasLen, 0)

	s.state.Lock()
	key := fmt.Sprintf("%s/ws/consumer:plug %s/ws/system:mount", s.prj.ProjectId, s.prj.ProjectId)
	s.state.Set("conns", map[string]any{
		key: map[string]any{"interface": "mount"},
	})
	s.state.Unlock()

	// Check the connections are reloaded once the issue is resolved.
	err = syscheck.CheckSystem()
	c.Assert(err, check.IsNil)

	ifaces = repo.Interfaces()
	cref := &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: sdk.System.String(), Name: "mount"}}
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{cref})
}

func (s *interfaceManagerSuite) TestManagerDoesntReloadUndesiredAutoconnections(c *check.C) {
	var consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr1: value1
 otherplug:
  interface: mount
`

	var producerYaml = `
name: producer
base: ubuntu@22.04
slots:
 slot:
  interface: mount
  attr2: value2
`
	s.launchWorkshop(c, "ws", []sdk.Meta{
		{Setup: consumer.Setup, SdkYAML: consumerYaml},
		{Setup: producer.Setup, SdkYAML: producerYaml},
	})

	s.state.Lock()
	key := fmt.Sprintf("%s/ws/consumer:plug %s/core/producer:slot", s.prj.ProjectId, s.prj.ProjectId)
	s.state.Set("conns", map[string]any{
		key: map[string]any{
			"interface": "test",
			"auto":      true,
			"undesired": true,
		},
	})
	s.state.Unlock()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	c.Assert(mgr.Repository().Interfaces().Connections, check.HasLen, 0)
}

func (s *interfaceManagerSuite) TestManagerRemovesNonexistingWorkshopConnections(c *check.C) {
	var consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr1: value1
 otherplug:
  interface: mount
`

	var producerYaml = `
name: producer
base: ubuntu@22.04
slots:
 slot:
  interface: mount
  attr2: value2
`
	s.launchWorkshop(c, "ws", []sdk.Meta{
		{Setup: consumer.Setup, SdkYAML: consumerYaml},
		{Setup: producer.Setup, SdkYAML: producerYaml},
	})

	s.state.Lock()
	missingPlug := fmt.Sprintf("%s/ws-missing/consumer:plug %s/ws/producer:slot", s.prj.ProjectId, s.prj.ProjectId)
	missingSlot := fmt.Sprintf("%s/ws/consumer:plug %s/ws-missing/producer:slot", s.prj.ProjectId, s.prj.ProjectId)
	stalePlugSlot := fmt.Sprintf("%s/ws/consumer:plug-1 %s/ws/producer:slot-1", s.prj.ProjectId, s.prj.ProjectId)

	s.state.Set("conns", map[string]any{
		missingPlug:   map[string]any{"interface": "test", "auto": true},
		missingSlot:   map[string]any{"interface": "test"},
		stalePlugSlot: map[string]any{"interface": "test", "auto": true},
	})
	s.state.Unlock()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	c.Assert(mgr.Repository().Interfaces().Connections, check.HasLen, 0)

	s.state.Lock()
	var conns map[string]any
	c.Assert(s.state.Get("conns", &conns), check.IsNil)
	s.state.Unlock()
	_, ok := conns[missingPlug]
	c.Check(ok, check.Equals, false)
	_, ok = conns[missingSlot]
	c.Check(ok, check.Equals, false)
	_, ok = conns[stalePlugSlot]
	c.Check(ok, check.Equals, true)
}

func (s *interfaceManagerSuite) TestConnectionStatesAutoManual(c *check.C) {
	isAuto, isUndesired := true, false
	s.testConnectionStates(c, isAuto, isUndesired, map[string]ifacestate.ConnectionState{
		"pid/ws/consumer:plug pid/ws/producer:slot": {
			Interface: "test",
			Auto:      true,
			StaticPlugAttrs: map[string]any{
				"attr1": "value1",
			},
			DynamicPlugAttrs: map[string]any{
				"dynamic-number": int64(7),
			},
			StaticSlotAttrs: map[string]any{
				"attr2": "value2",
			},
			DynamicSlotAttrs: map[string]any{
				"other-number": int64(9),
			},
		}})
}

func (s *interfaceManagerSuite) TestConnectionStatesUndesired(c *check.C) {
	isAuto, isUndesired := true, true
	s.testConnectionStates(c, isAuto, isUndesired, map[string]ifacestate.ConnectionState{
		"pid/ws/consumer:plug pid/ws/producer:slot": {
			Interface: "test",
			Auto:      true,
			Undesired: true,
			StaticPlugAttrs: map[string]any{
				"attr1": "value1",
			},
			DynamicPlugAttrs: map[string]any{
				"dynamic-number": int64(7),
			},
			StaticSlotAttrs: map[string]any{
				"attr2": "value2",
			},
			DynamicSlotAttrs: map[string]any{
				"other-number": int64(9),
			},
		}})
}

func (s *interfaceManagerSuite) testConnectionStates(c *check.C, auto, undesired bool, expected map[string]ifacestate.ConnectionState) {
	consumer := sdk.MockInfo(c, `
name: consumer
base: ubuntu@22.04
plugs:
    plug:
        interface: test
        attr1: value1
`, "pid", "ws")

	producer := sdk.MockInfo(c, `
name: producer
base: ubuntu@22.04
slots:
    slot:
        interface: test
        attr2: value2
`, "pid", "ws")
	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	conns, err := mgr.ConnectionStates()
	c.Assert(err, check.IsNil)
	c.Check(conns, check.HasLen, 0)

	st := s.state
	st.Lock()
	sc, err := ifacestate.GetConns(st)
	c.Assert(err, check.IsNil)

	slot := producer.Slots["slot"]
	c.Assert(slot, check.NotNil)
	plug := consumer.Plugs["plug"]
	c.Assert(plug, check.NotNil)
	dynamicPlugAttrs := map[string]any{"dynamic-number": 7}
	dynamicSlotAttrs := map[string]any{"other-number": 9}
	// create connection in conns state
	conn := &interfaces.Connection{
		Plug: interfaces.NewConnectedPlug(plug, nil, dynamicPlugAttrs),
		Slot: interfaces.NewConnectedSlot(slot, nil, dynamicSlotAttrs),
	}
	ifacestate.UpdateConnectionInConnState(sc, conn, auto, undesired)
	ifacestate.SetConns(st, sc)
	st.Unlock()

	conns, err = mgr.ConnectionStates()
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Check(conns, check.DeepEquals, expected)
}
