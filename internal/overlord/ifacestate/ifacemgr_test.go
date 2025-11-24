package ifacestate_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
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
	prj        workshop.Project
	secBackend *ifacetest.TestSecurityBackend

	restoreProjectId func()
}

var _ = check.Suite(&interfaceManagerSuite{})

func (s *interfaceManagerSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	var err error

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
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(path, []byte(meta.SdkYAML), 0o644)
	c.Assert(err, check.IsNil)

	s.state.Lock()
	be := s.o.WorkshopBackend()
	s.state.Unlock()
	volume := workshop.VolumeSetup{
		Name:     sdk.VolumeName(meta.Name, meta.Revision),
		Kind:     "sdk",
		Sdk:      meta.Name,
		Revision: meta.Revision,
		Metadata: meta.SdkYAML,
	}
	file, err := os.Open(vfs)
	c.Assert(err, check.IsNil)
	defer file.Close()
	if err := be.ImportVolume(s.ctx, volume, file); err != nil {
		c.Assert(err, testutil.ErrorIs, workshop.ErrVolumeAlreadyExists)
	}
}

func (s *interfaceManagerSuite) launchWorkshop(c *check.C, ws string, sdks []sdk.Meta) *workshop.Workshop {
	ctx := context.WithValue(s.ctx, workshop.ContextProjectId, s.prj.ProjectId)

	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	workshopFile := bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	var wf workshop.File
	err = yaml.Unmarshal(workshopFile.Bytes(), &wf)
	c.Assert(err, check.IsNil)

	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}
	err = s.wsbackend.LaunchOrRebuildWorkshop(ctx, &wf, image)
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
		err = be.AttachVolume(ctx, ws, sdk.VolumeName(sk.Name, sk.Revision), sdk.SdkDir(sk.Name), true)
		c.Assert(err, check.IsNil)
		err = w.AddSdk(ctx, sk.Setup)
		c.Assert(err, check.IsNil)
	}

	return w
}

func (s *interfaceManagerSuite) TestManagerReloadsConnections(c *check.C) {
	consumerYaml := `
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
	s.state.Set("conns", map[string]interface{}{
		key: map[string]interface{}{
			"interface": "mount",
			"plug-static": map[string]interface{}{
				"mount": "foo",
				"attr":  "stored-value",
			},
			"slot-static": map[string]interface{}{
				"mount": "foo",
				"attr":  "stored-value",
			},
		},
	})
	s.state.Unlock()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	repo := mgr.Repository()

	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 1)
	cref := &interfaces.ConnRef{
		PlugRef: sdk.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"},
		SlotRef: sdk.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: sdk.System.String(), Name: "mount"},
	}
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{cref})

	conn, err := repo.Connection(cref)
	c.Assert(err, check.IsNil)
	c.Assert(conn.Plug.Name(), check.Equals, "plug")
	c.Assert(conn.Plug.StaticAttrs(), check.DeepEquals, map[string]interface{}{
		"mount": "foo",
		"attr":  "stored-value",
	})
	c.Assert(conn.Slot.Name(), check.Equals, "mount")
	c.Assert(conn.Slot.StaticAttrs(), check.DeepEquals, map[string]interface{}{
		"mount": "foo",
		"attr":  "stored-value",
	})
}

func (s *interfaceManagerSuite) TestManagerDoesntReloadUndesiredAutoconnections(c *check.C) {
	consumerYaml := `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr1: value1
 otherplug:
  interface: mount
`

	producerYaml := `
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
	s.state.Set("conns", map[string]interface{}{
		key: map[string]interface{}{
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

func (s *interfaceManagerSuite) TestManagerRemovesNonexistingAutoConnectionss(c *check.C) {
	consumerYaml := `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: mount
  attr1: value1
 otherplug:
  interface: mount
`

	producerYaml := `
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
	key := fmt.Sprintf("%s/ws/consumer:plug-1 %s/core/producer:slot-1", s.prj.ProjectId, s.prj.ProjectId)

	s.state.Set("conns", map[string]interface{}{
		key: map[string]interface{}{
			"interface": "test",
			"auto":      true,
		},
	})
	s.state.Unlock()

	mgr := ifacestate.New(s.state, s.o.TaskRunner())
	err := mgr.StartUp()
	c.Assert(err, check.IsNil)

	c.Assert(mgr.Repository().Interfaces().Connections, check.HasLen, 0)
}

func (s *interfaceManagerSuite) TestConnectionStatesAutoManual(c *check.C) {
	isAuto, isUndesired := true, false
	s.testConnectionStates(c, isAuto, isUndesired, map[string]ifacestate.ConnectionState{
		"pid/ws/consumer:plug pid/ws/producer:slot": {
			Interface: "test",
			Auto:      true,
			StaticPlugAttrs: map[string]interface{}{
				"attr1": "value1",
			},
			DynamicPlugAttrs: map[string]interface{}{
				"dynamic-number": int64(7),
			},
			StaticSlotAttrs: map[string]interface{}{
				"attr2": "value2",
			},
			DynamicSlotAttrs: map[string]interface{}{
				"other-number": int64(9),
			},
		},
	})
}

func (s *interfaceManagerSuite) TestConnectionStatesUndesired(c *check.C) {
	isAuto, isUndesired := true, true
	s.testConnectionStates(c, isAuto, isUndesired, map[string]ifacestate.ConnectionState{
		"pid/ws/consumer:plug pid/ws/producer:slot": {
			Interface: "test",
			Auto:      true,
			Undesired: true,
			StaticPlugAttrs: map[string]interface{}{
				"attr1": "value1",
			},
			DynamicPlugAttrs: map[string]interface{}{
				"dynamic-number": int64(7),
			},
			StaticSlotAttrs: map[string]interface{}{
				"attr2": "value2",
			},
			DynamicSlotAttrs: map[string]interface{}{
				"other-number": int64(9),
			},
		},
	})
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
	dynamicPlugAttrs := map[string]interface{}{"dynamic-number": 7}
	dynamicSlotAttrs := map[string]interface{}{"other-number": 9}
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
