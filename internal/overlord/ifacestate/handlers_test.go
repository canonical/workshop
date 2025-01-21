package ifacestate_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/ifacestate/schema"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type interfaceHandlersSuite struct {
	interfaceManagerSuite
	mgr                      *ifacestate.InterfaceManager
	restoreSimple            func()
	restoreDeny              func()
	restoreSecurtityBackends func()
	restoreUserLookup        func()
}

var _ = check.Suite(&interfaceHandlersSuite{})

func fakeHandler(task *state.Task, _ *tomb.Tomb) error {
	return nil
}

var producer = `name: producer
base: ubuntu@22.04
slots:
  slot:
    interface: mock-network`
var psetup = sdk.Setup{Name: "producer", Channel: "latest/stable", Revision: sdk.Revision{N: 1}}

var consumer = `name: consumer
base: ubuntu@22.04
plugs:
  plug:
    interface: mock-network
    attribute: one
`

var consumerManyPlugs = `name: consumer
base: ubuntu@22.04
plugs:
  plug:
    interface: mock-network
    attribute: one
  plug2:
    interface: mock-network
    attribute: two
  plug3:
    interface: mock-network
    attribute: three
  plug-ssh:
    interface: mock-ssh-agent
    attribute: four
`

var consumer2 = `name: consumer2
base: ubuntu@22.04
plugs:
  plug2:
    interface: mock-network
    attribute: one
`

var conflictingTarget1 = `name: conflict-1
base: ubuntu@22.04
plugs:
  plug:
    interface: mount
    workshop-target: /home/workshop
`

var conflictingTarget2 = `name: conflict-2
base: ubuntu@22.04
plugs:
  plug:
    interface: mount
    workshop-target: /home/workshop
`

var csetup = sdk.Setup{Name: "consumer", Channel: "latest/stable", Revision: sdk.Revision{N: 1}}
var csetup2 = sdk.Setup{Name: "consumer2", Channel: "latest/stable", Revision: sdk.Revision{N: 1}}

func (s *interfaceHandlersSuite) SetUpTest(c *check.C) {
	s.interfaceManagerSuite.SetUpTest(c)
	s.restoreSimple = builtin.MockInterface(simpleIface{name: "mock-network"})
	s.restoreDeny = builtin.MockInterface(denyAutoIface{name: "mock-ssh-agent"})

	s.mgr = ifacestate.New(s.state, s.runner)
	c.Assert(s.mgr, check.NotNil)

	s.runner.AddHandler("fake-task", fakeHandler, nil)

	// error-provoking task handler
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return errors.New("error-trigger task")
	}
	s.runner.AddHandler("error-trigger", erroringHandler, nil)
	s.restoreSecurtityBackends = ifacestate.MockSecurityBackends([]interfaces.SecurityBackend{s.secBackend})

	s.o.AddManager(s.mgr)
	s.o.AddManager(s.runner)
	err := s.o.StartUp()
	c.Assert(err, check.IsNil)

	s.restoreUserLookup = testutil.FakeFunc(func(name string) (*user.User, error) {
		u := &user.User{
			Name:     name,
			Username: name,
			Uid:      "1000",
			Gid:      "1000",
			HomeDir:  c.MkDir(),
		}
		return u, nil
	}, &workshop.LookupUsername)
}

func (s *interfaceHandlersSuite) TearDownTest(c *check.C) {
	s.restoreSimple()
	s.restoreDeny()
	s.restoreSecurtityBackends()
	s.restoreUserLookup()
}

func (s *interfaceHandlersSuite) settle(c *check.C) {
	err := s.o.Settle(5 * time.Second)
	c.Check(err, check.IsNil)
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, i := range tasks {
		i.Set("workshop", w)
		i.Set("project", p)
	}
}

type simpleIface struct {
	name string
}

func (si simpleIface) Name() string                                            { return si.name }
func (si simpleIface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool { return true }

type denyAutoIface struct {
	name string
}

func (di denyAutoIface) Name() string                                            { return di.name }
func (di denyAutoIface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool { return false }

func (s *interfaceHandlersSuite) newAutoconnectChange() *state.Change {
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("auto-connect", "...")
	t1.Set("sdk", "consumer")
	setWorkshopProject("ws", s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	return chg
}

func (s *interfaceHandlersSuite) TestAutoconnectPlugSlotPairSuccess(c *check.C) {
	// Setup
	// Create an already installed workshop with a candidate SDK/slot
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-producer", []testSdkSetup{{psetup, producer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-producer")), check.IsNil)

	// Launch another workshop with a candidate plug
	s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	s.state.Lock()
	chg := s.newAutoconnectChange()
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	ref, err := repo.Connected(s.prj.ProjectId, "ws", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	ref, err = repo.Connected(s.prj.ProjectId, "ws-producer", "producer", "slot")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	var conns map[string]interface{}
	s.state.Get("conns", &conns)
	c.Assert(conns, check.DeepEquals, map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws-producer/producer:slot": map[string]interface{}{
			"interface":   "mock-network",
			"auto":        true,
			"plug-static": map[string]interface{}{"attribute": "one"},
		},
	})

	// ensure that backend profiles were set for both SDKs
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestAutoconnectBindPlugSuccess(c *check.C) {
	// Setup
	// Create an already installed workshop with a candidate SDK/slot
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-producer", []testSdkSetup{{psetup, producer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-producer")), check.IsNil)

	wp, err := s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumerManyPlugs}})
	c.Check(err, check.IsNil)
	wp.File.Sdks[0].Plugs = make(map[string]workshop.Plug)
	wp.File.Sdks[0].Plugs["plug"] = workshop.Plug{Bind: &workshop.PlugRef{Sdk: "consumer", Name: "plug2"}}
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	s.state.Lock()
	chg := s.newAutoconnectChange()
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	pconns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Check(pconns, check.HasLen, 3)
	c.Check(err, check.IsNil)

	ref, err := repo.Connected(s.prj.ProjectId, "ws-producer", "producer", "slot")
	c.Check(ref, check.HasLen, 3)
	c.Check(err, check.IsNil)

	var conns map[string]interface{}
	s.state.Get("conns", &conns)
	c.Check(conns, check.DeepEquals, map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws-producer/producer:slot": map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{"bind": "42424242/ws/consumer:plug2 42424242/ws-producer/producer:slot"},
		},
		"42424242/ws/consumer:plug2 42424242/ws-producer/producer:slot": map[string]interface{}{
			"interface":   "mock-network",
			"auto":        true,
			"plug-static": map[string]interface{}{"attribute": "two"},
		},
		"42424242/ws/consumer:plug3 42424242/ws-producer/producer:slot": map[string]interface{}{
			"interface":   "mock-network",
			"auto":        true,
			"plug-static": map[string]interface{}{"attribute": "three"},
		},
	})

	// ensure that backend profiles were set for both SDKs
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestAutoconnectBindMasterPlugNotFound(c *check.C) {
	// Setup
	// Create an already installed workshop with a candidate SDK/slot
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-producer", []testSdkSetup{{psetup, producer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-producer")), check.IsNil)

	wp, err := s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumerManyPlugs}})
	c.Check(err, check.IsNil)
	wp.File.Sdks[0].Plugs = make(map[string]workshop.Plug)
	wp.File.Sdks[0].Plugs["plug"] = workshop.Plug{Bind: &workshop.PlugRef{Sdk: "consumer", Name: "no-such-plug2"}}
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	s.state.Lock()
	chg := s.newAutoconnectChange()
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, `(?s).*SDK "ws/consumer" has no plug named "no-such-plug2".*`)

	// Validate
	pconns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Check(pconns, check.HasLen, 0)
	c.Check(err, check.IsNil)

	ref, err := repo.Connected(s.prj.ProjectId, "ws-producer", "producer", "slot")
	c.Check(ref, check.HasLen, 0)
	c.Check(err, check.IsNil)
}

func (s *interfaceHandlersSuite) TestAutoconnectBackendSetupFail(c *check.C) {
	// Setup
	// Create an already launched workshop with a candidate SDK/slot
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-producer", []testSdkSetup{{psetup, producer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-producer")), check.IsNil)

	s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumerManyPlugs}, {csetup2, consumer2}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer2, s.prj.ProjectId, "ws")), check.IsNil)

	n := 0
	// One of the SDKs setup fails, we need to make sure that any partial
	// progress will be aborted (i.e. previously created profiles for other SDKs
	// will be removed).
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		if n > 0 {
			return errors.New("cannot finish backend setup")
		}
		n++
		return nil
	}
	defer func() { s.secBackend.SetupCallback = nil }()

	// Execute
	s.state.Lock()
	chg := s.newAutoconnectChange()
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, "(?s).*cannot finish backend setup.*")

	// Validate
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	ref, err := repo.Connected(s.prj.ProjectId, "ws-producer", "producer", "slot")
	c.Assert(ref, check.HasLen, 0)
	c.Assert(err, check.IsNil)

	var conns map[string]interface{}
	s.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestAutoconnectFailsOnConflictingMountTargets(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{sdk.Setup{Name: "conflict-1", Channel: "latest/stable", Revision: sdk.R(1)}, conflictingTarget1},
		{sdk.Setup{Name: "conflict-2", Channel: "latest/stable", Revision: sdk.R(1)}, conflictingTarget2},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, conflictingTarget1, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, conflictingTarget2, s.prj.ProjectId, "ws")), check.IsNil)

	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("auto-connect", "...")
	t1.Set("sdk", "conflict-1")
	t2 := s.state.NewTask("auto-connect", "...")
	t2.Set("sdk", "conflict-2")
	t2.WaitFor(t1)
	setWorkshopProject("ws", s.prj, t1, t2)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t2)
	s.state.Unlock()

	// Execute
	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()

	// Validate
	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*target "/home/workshop" is also mounted by.*`)
}

func (s *interfaceHandlersSuite) TestAutoconnectNoConnectionCandidates(c *check.C) {
	// Setup
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumer}})

	// Execute
	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("auto-connect", "...")
	t1.Set("sdk", "consumer")
	t2 := s.state.NewTask("error-trigger", "...")
	setWorkshopProject("ws", s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t2)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws", "consumer"), check.HasLen, 0)

	var conns map[string]interface{}
	s.state.Get("conns", &conns)
	c.Assert(conns, check.DeepEquals, map[string]interface{}(nil))

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestAutoconnectRemountedPlugsOnRefresh(c *check.C) {
	// Setup
	// Create an already installed workshop with a candidate SDK/slot
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-producer", []testSdkSetup{{psetup, producer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-producer")), check.IsNil)

	// Launch another workshop with a candidate plug
	s.launchWorkshop(c, "ws", []testSdkSetup{{csetup, consumer}})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	s.state.Lock()
	chg := s.state.NewChange("refresh", "...")
	// existing remounts that should be preserved after refresh
	chg.Set("remounts", map[string]string{
		"42424242/ws/consumer:plug 42424242/ws-producer/producer:slot": "/old/source",
	})
	t1 := s.state.NewTask("auto-connect", "...")
	t1.Set("sdk", "consumer")
	setWorkshopProject("ws", s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(t1.Status(), check.Equals, state.DoneStatus)
	var conns map[string]interface{}
	s.state.Get("conns", &conns)
	c.Assert(conns, check.DeepEquals, map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws-producer/producer:slot": map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"slot-dynamic": map[string]interface{}{"host-source": "/old/source"},
		},
	})

	// ensure that backend profiles were set for both SDKs
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) newRemountChange(newSource string) *state.Change {
	s.state.Lock()
	defer s.state.Unlock()

	t1 := s.state.NewTask("remount", "remount")
	t1.Set("host-source", newSource)
	t1.Set("plug", interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws-consumer", Sdk: "consumer", Name: "plug"})
	setWorkshopProject("ws-consumer", s.prj, t1)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	return chg
}

func (s *interfaceHandlersSuite) launchRemountWorkshop(c *check.C, source string) {
	// Note: we set the source attribute for the plug in these tests, however,
	// it is a dynamic attribute that will be defined when we install an SDK
	// into a workshop as the default path depends on the username
	var sdkYaml = `
name: consumer
base: ubuntu@22.04
plugs:
    plug:
        interface: mount
        workshop-target: /home/workshop
`
	var systemYaml = `
name: system
base: ubuntu@22.04
type: system
slots:
    slot:
        interface: mount
`
	wm, err := s.launchWorkshop(c, "ws-consumer", []testSdkSetup{{csetup, sdkYaml}})
	c.Assert(err, check.IsNil)
	wm.Running = true
	c.Assert(s.mgr.Repository().AddSdk(sdk.MockInfo(c, sdkYaml, s.prj.ProjectId, "ws-consumer")), check.IsNil)
	c.Assert(s.mgr.Repository().AddSdk(sdk.MockInfo(c, systemYaml, s.prj.ProjectId, "ws-consumer")), check.IsNil)

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws-consumer/consumer:plug 42424242/ws-consumer/system:slot": map[string]interface{}{
			"interface":    "mount",
			"auto":         true,
			"plug-static":  map[string]interface{}{"target": "/opt"},
			"slot-dynamic": map[string]interface{}{"host-source": source},
		},
	})
	_, err = ifacestate.ReloadConnections(s.mgr, s.prj.ProjectId, "ws-consumer", "consumer")
	c.Assert(err, check.IsNil)
	s.state.Unlock()
}

func (s *interfaceHandlersSuite) TestRemountSuccessDestExistsAndEmpty(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := c.MkDir()
	_, err := os.Create(filepath.Join(oldSource, "tempfile"))
	c.Check(err, check.IsNil)

	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.IsNil)
	c.Assert(change.Status(), check.Equals, state.DoneStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var remountSource string
	c.Assert(connection.Slot.Attr("host-source", &remountSource), check.IsNil)
	c.Assert(remountSource, check.Equals, newSource)

	c.Assert(oldSource, testutil.FileAbsent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 1)

	// ensure the global conns state was updated correctly
	conns, err := ifacestate.GetConns(s.state)
	c.Assert(err, check.IsNil)
	c.Assert(conns[ref[0].ID()], check.DeepEquals, &schema.ConnState{
		Auto:             true,
		Interface:        "mount",
		Undesired:        false,
		StaticPlugAttrs:  map[string]interface{}{"target": "/opt"},
		DynamicPlugAttrs: map[string]interface{}{},
		StaticSlotAttrs:  map[string]interface{}{},
		DynamicSlotAttrs: map[string]interface{}{"host-source": newSource}})
	c.Assert(conns, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestRemountSuccessIfNewSourceDoesNotExist(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := filepath.Join(c.MkDir(), "new")
	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.IsNil)
	c.Assert(change.Status(), check.Equals, state.DoneStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var remountSource string
	c.Assert(connection.Slot.Attr("host-source", &remountSource), check.IsNil)
	c.Assert(remountSource, check.Equals, newSource)

	c.Assert(oldSource, testutil.FileAbsent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 1)
	c.Assert(s.secBackend.SetupCalls[0].SdkInfo.Sdk, check.Equals, "consumer")
	c.Assert(s.secBackend.SetupCalls[0].SdkInfo.Workshop, check.Equals, "ws-consumer")

	// ensure the global conns state was updated correctly
	conns, err := ifacestate.GetConns(s.state)
	c.Assert(err, check.IsNil)
	c.Assert(conns[ref[0].ID()], check.DeepEquals, &schema.ConnState{
		Auto:             true,
		Interface:        "mount",
		Undesired:        false,
		StaticPlugAttrs:  map[string]interface{}{"target": "/opt"},
		DynamicPlugAttrs: map[string]interface{}{},
		StaticSlotAttrs:  map[string]interface{}{},
		DynamicSlotAttrs: map[string]interface{}{"host-source": newSource}})
	c.Assert(conns, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestRemountRenameNewSourceNotEmptyFails(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := c.MkDir()
	_, err := os.Create(filepath.Join(newSource, "tempfile"))
	c.Check(err, check.IsNil)
	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.ErrorMatches, fmt.Sprintf(`(?s).*\(source %q is not empty; workshop must be stopped to remount safely\)`, newSource))
	c.Assert(change.Status(), check.Equals, state.ErrorStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var src string
	c.Assert(connection.Slot.Attr("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, oldSource)

	c.Assert(oldSource, testutil.FilePresent)
	c.Assert(newSource, testutil.FilePresent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestRemountRenameNewSourceNotEmptySucceeds(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := c.MkDir()
	_, err := os.Create(filepath.Join(newSource, "tempfile"))
	c.Check(err, check.IsNil)
	s.launchRemountWorkshop(c, oldSource)

	// the remount will be performed if the workshop is not running
	err = s.wsbackend.StopWorkshop(s.ctx, "ws-consumer", true)
	c.Check(err, check.IsNil)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.IsNil)
	c.Assert(change.Status(), check.Equals, state.DoneStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var src string
	c.Assert(connection.Slot.Attr("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, newSource)

	c.Assert(oldSource, testutil.FilePresent)
	c.Assert(newSource, testutil.FilePresent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestRemountRenameUnexpectedError(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := filepath.Join(c.MkDir(), "new-source")
	_, err := os.Create(newSource)
	c.Check(err, check.IsNil)
	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.ErrorMatches, `(?s).*\(not a directory\)`)
	c.Assert(change.Status(), check.Equals, state.ErrorStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var src string
	c.Assert(connection.Slot.Attr("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, oldSource)

	c.Assert(oldSource, testutil.FilePresent)
	c.Assert(newSource, testutil.FilePresent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestRemountInterfaceBackendSetupFails(c *check.C) {
	// Setup
	oldSource := c.MkDir()
	newSource := c.MkDir()
	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		return errors.New("cannot setup LXD profile")
	}
	defer func() { s.secBackend.SetupCallback = nil }()

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(change.Err(), check.ErrorMatches, "(?s).*\\(cannot setup LXD profile\\)")
	c.Assert(change.Status(), check.Equals, state.ErrorStatus)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	var src string
	c.Assert(connection.Slot.Attr("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, oldSource)

	c.Assert(oldSource, testutil.FilePresent)
	c.Assert(newSource, testutil.FileAbsent)
	// 2 calls for the autoconnect, no calls for the remount
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestRemountWorksIfOldSourceNotExist(c *check.C) {
	// Setup
	oldSource := "/does/not/exist"
	newSource := c.MkDir()
	s.launchRemountWorkshop(c, oldSource)
	change := s.newRemountChange(newSource)

	// Execute
	s.settle(c)

	// Validate
	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(change.Status(), check.Equals, state.DoneStatus)

	warnings := s.state.AllWarnings()
	c.Check(warnings, check.HasLen, 1)
	c.Check(warnings[0].String(), check.Equals, `cannot find source "/does/not/exist" for "ws-consumer/consumer:plug"; will attempt to recreate`)

	repo := s.mgr.Repository()
	ref, err := repo.Connected(s.prj.ProjectId, "ws-consumer", "consumer", "plug")
	c.Assert(ref, check.HasLen, 1)
	c.Assert(err, check.IsNil)

	connection, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	var src string
	c.Assert(connection.Slot.Attr("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, newSource)

	c.Assert(newSource, testutil.FilePresent)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) newDisconnectInterfacesChange(sdkName string) *state.Change {
	t1 := s.state.NewTask("auto-disconnect", "...")
	t1.Set("plug", interfaces.PlugRef{
		ProjectId: s.prj.ProjectId, Workshop: "ws-consumer", Sdk: "consumer", Name: "plug"})
	t1.Set("slot", interfaces.PlugRef{
		ProjectId: s.prj.ProjectId, Workshop: "ws-consumer", Sdk: "producer", Name: "slot"})
	t1.Set("sdk", sdkName)
	setWorkshopProject("ws-consumer", s.prj, t1)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	return chg
}

func (s *interfaceHandlersSuite) TestAutoDisconnectSuccess(c *check.C) {
	// Setup
	// Create an already installed workshop with a connected plug
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-consumer", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})

	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "producer", Name: "slot"},
	}

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		connRef.ID(): map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{"test-dynamic-attr": "new-dynamic-value"},
		},
	})
	_, err := ifacestate.ReloadConnections(s.mgr, "", "", "")
	c.Assert(err, check.IsNil)
	s.state.Unlock()

	// Execute
	s.state.Lock()
	chg := s.newDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	var stateConns map[string]interface{}
	c.Assert(s.state.Get("conns", &stateConns), check.IsNil)
	c.Assert(stateConns, check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestAutoDisconnectSavesRemounts(c *check.C) {
	// Setup
	// Create an already installed workshop with a connected mount plug
	repo := s.mgr.Repository()
	source := c.MkDir()
	s.launchRemountWorkshop(c, source)

	// Execute
	s.state.Lock()
	chg := s.newDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	var stateConns map[string]interface{}
	c.Assert(s.state.Get("conns", &stateConns), check.IsNil)
	c.Assert(stateConns, check.HasLen, 0)

	var attrs map[string]interface{}
	c.Assert(chg.Get("remounts", &attrs), check.IsNil)
	c.Assert(attrs, check.HasLen, 1)
	c.Assert(attrs["42424242/ws-consumer/consumer:plug 42424242/ws-consumer/system:slot"],
		check.DeepEquals, source)
	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestAutoDisconnectDisconnected(c *check.C) {
	// Setup
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	s.state.Lock()
	chg := s.newDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestAutoDisconnectNoSdkProfile(c *check.C) {
	// Setup
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	s.secBackend.RemoveCallback = func(sdkName string) error { return workshop.ErrSdkProfileNotFound }

	// Execute
	s.state.Lock()
	chg := s.newDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) newUndoDisconnectInterfacesChange(sdkName string) *state.Change {
	chg := s.newDisconnectInterfacesChange(sdkName)
	terr := s.state.NewTask("error-trigger", "...")
	terr.WaitFor(chg.Tasks()[0])
	chg.AddTask(terr)
	return chg
}

func (s *interfaceHandlersSuite) TestUndoDisconnectInterfacesSuccess(c *check.C) {
	// Setup
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-consumer", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws-consumer")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-consumer")), check.IsNil)

	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "producer", Name: "slot"},
	}

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		connRef.ID(): map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{"test-dynamic-attr": "new-dynamic-value"},
		},
	})
	_, err := ifacestate.ReloadConnections(s.mgr, "", "", "")
	c.Assert(err, check.IsNil)
	s.state.Unlock()

	// Execute
	s.state.Lock()
	chg := s.newUndoDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.NotNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 1)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	var stateConns map[string]interface{}
	c.Assert(s.state.Get("conns", &stateConns), check.IsNil)
	c.Assert(stateConns, check.HasLen, 1)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 4)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) TestUndoDisconnectInterfacesManualRestored(c *check.C) {
	// Setup
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws-consumer", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws-consumer")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws-consumer")), check.IsNil)

	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "42424242", Workshop: "ws-consumer", Sdk: "producer", Name: "slot"},
	}

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		connRef.ID(): map[string]interface{}{
			"interface":    "mock-network",
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{"test-dynamic-attr": "new-dynamic-value"},
		},
	})
	_, err := ifacestate.ReloadConnections(s.mgr, "", "", "")
	c.Assert(err, check.IsNil)
	s.state.Unlock()

	// Execute
	s.state.Lock()
	chg := s.newUndoDisconnectInterfacesChange("consumer")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.NotNil)

	// Validate
	c.Assert(repo.Plugs(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 1)
	c.Assert(repo.Slots(s.prj.ProjectId, "ws-consumer", "consumer"), check.HasLen, 0)

	var stateConns map[string]interface{}
	c.Assert(s.state.Get("conns", &stateConns), check.IsNil)
	c.Assert(stateConns, check.HasLen, 1)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 4)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 1)
}

func (s *interfaceHandlersSuite) disconnectChange(c *check.C, workshop string, forget bool) *state.Change {
	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("disconnect", "...")
	plugRef := interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: workshop, Sdk: "consumer", Name: "plug"}
	slotRef := interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: workshop, Sdk: "producer", Name: "slot"}
	t1.Set("plug", plugRef)
	t1.Set("slot", slotRef)
	t1.Set("forget", forget)
	setWorkshopProject(workshop, s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	s.state.Unlock()

	repo := s.mgr.Repository()
	_, err := repo.Connect(&interfaces.ConnRef{PlugRef: plugRef, SlotRef: slotRef}, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)
	return chg
}

func (s *interfaceHandlersSuite) TestDisconnectSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"interface":    "mock-network",
			"auto":         false,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{},
		},
	})
	s.state.Unlock()

	// Execute
	chg := s.disconnectChange(c, "ws", false)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)

	conns, err = repo.Connections(s.prj.ProjectId, "ws", "producer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestDisconnectAuto(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	connRefKey := "42424242/ws/consumer:plug 42424242/ws/producer:slot"
	s.state.Set("conns", map[string]interface{}{
		connRefKey: map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{},
		},
	})
	s.state.Unlock()

	// Execute
	chg := s.disconnectChange(c, "ws", false)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	cs, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.HasLen, 0)

	cs, err = repo.Connections(s.prj.ProjectId, "ws", "producer")
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.HasLen, 0)

	var conns map[string]*schema.ConnState
	s.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[connRefKey].Undesired, check.Equals, true)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestDisconnectForgetAuto(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	connRefKey := "42424242/ws/consumer:plug 42424242/ws/producer:slot"
	s.state.Set("conns", map[string]interface{}{
		connRefKey: map[string]interface{}{
			"interface":    "mock-network",
			"auto":         true,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{},
		},
	})
	s.state.Unlock()

	// Execute
	chg := s.disconnectChange(c, "ws", true)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	cs, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.HasLen, 0)

	cs, err = repo.Connections(s.prj.ProjectId, "ws", "producer")
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.HasLen, 0)

	var conns map[string]*schema.ConnState
	s.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestundoDisconnectSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"interface":    "mock-network",
			"auto":         false,
			"plug-static":  map[string]interface{}{"attribute": "one"},
			"plug-dynamic": map[string]interface{}{"bind": "42424242/ws/consumer:plug2 42424242/ws/producer:slot"},
		},
	})
	s.state.Unlock()

	chg := s.disconnectChange(c, "ws", false)
	s.state.Lock()
	terr := s.state.NewTask("error-trigger", "...")
	terr.WaitFor(chg.Tasks()[0])
	chg.AddTask(terr)
	s.state.Unlock()

	// Execute
	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, "(?s).*error-trigger task.*")

	// Validate
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)

	conns, err = repo.Connections(s.prj.ProjectId, "ws", "producer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)

	conn, err := repo.Connection(conns[0])
	c.Assert(err, check.IsNil)
	_, ok := conn.CheckBound()
	c.Assert(ok, check.Equals, true)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 4)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestundoDisconnectUndesiredSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	conn := map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"interface": "mock-network",
			"auto":      true,
			"undesired": true,
		},
	}
	s.state.Set("conns", conn)
	s.state.Unlock()

	chg := s.disconnectChange(c, "ws", true)
	disc, err := repo.Connections("42424242", "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(disc, check.HasLen, 1)
	// emulate an undesired connection
	repo.DisconnectAll(disc)

	s.state.Lock()
	terr := s.state.NewTask("error-trigger", "...")
	terr.WaitFor(chg.Tasks()[0])
	chg.AddTask(terr)
	s.state.Unlock()

	// Execute
	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, "(?s).*error-trigger task.*")

	// Validate
	cons, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(cons, check.HasLen, 0)

	prods, err := repo.Connections(s.prj.ProjectId, "ws", "producer")
	c.Assert(err, check.IsNil)
	c.Assert(prods, check.HasLen, 0)

	conns := map[string]interface{}{}
	s.state.Get("conns", &conns)
	c.Assert(conns, check.DeepEquals, conn)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) connectChange(workshop string, auto bool, delayedBacked bool) *state.Change {
	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("connect", "...")
	plugRef := interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: workshop, Sdk: "consumer", Name: "plug"}
	slotRef := interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: workshop, Sdk: "producer", Name: "slot"}
	t1.Set("plug", plugRef)
	t1.Set("slot", slotRef)
	t1.Set("auto", auto)
	t1.Set("delayed-setup-profile", delayedBacked)
	setWorkshopProject(workshop, s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	s.state.Unlock()

	return chg
}

func (s *interfaceHandlersSuite) TestConnectSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	chg := s.connectChange("ws", false, true)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].PlugRef, check.DeepEquals, interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"})
	c.Assert(conns[0].SlotRef, check.DeepEquals, interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestConnectSuccessSetupBackend(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	chg := s.connectChange("ws", false, false)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].PlugRef, check.DeepEquals, interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"})
	c.Assert(conns[0].SlotRef, check.DeepEquals, interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 2)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestConnectDisconnectsIfBackedSetupFailed(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		return errors.New("cannot finish backend setup")
	}
	defer func() { s.secBackend.SetupCallback = nil }()

	// Execute
	chg := s.connectChange("ws", false, false)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()

	// Validate that even if a connection was created in the repository, it
	// won't persist if the backend setup fails.
	c.Check(chg.Tasks()[0].Status(), check.Equals, state.ErrorStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestConnectSetsPlugDynamicAttrs(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	chg := s.connectChange("ws", false, true)
	s.state.Lock()
	tsk := chg.Tasks()[0]
	tsk.Set("plug-dynamic", map[string]interface{}{"dynamic": "value"})
	tsk.Set("delayed-setup-profile", false)
	s.state.Unlock()

	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
		c.Check(err, check.IsNil)
		conn, err := repo.Connection(conns[0])
		c.Check(err, check.IsNil)
		conn.Plug.SetAttr("set-from-profile-setup", "value")
		return nil
	}

	// Execute
	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].PlugRef, check.DeepEquals, interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"})
	c.Assert(conns[0].SlotRef, check.DeepEquals, interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})

	conn, err := repo.Connection(conns[0])
	c.Assert(err, check.IsNil)
	v, _ := conn.Plug.Lookup("dynamic")
	c.Assert(v, check.Equals, "value")
	v, _ = conn.Plug.Lookup("set-from-profile-setup")
	c.Assert(v, check.Equals, "value")
}

func (s *interfaceHandlersSuite) TestConnectAuto(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	chg := s.connectChange("ws", true, true)

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns[0].PlugRef, check.DeepEquals, interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug"})
	c.Assert(conns[0].SlotRef, check.DeepEquals, interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})

	stateConns, err := ifacestate.GetConns(s.state)
	c.Assert(err, check.IsNil)
	c.Assert(stateConns, check.DeepEquals, map[string]*schema.ConnState{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:             true,
			Interface:        "mock-network",
			StaticPlugAttrs:  map[string]interface{}{"attribute": "one"},
			DynamicPlugAttrs: map[string]interface{}{},
			StaticSlotAttrs:  map[string]interface{}{},
			DynamicSlotAttrs: map[string]interface{}{},
		},
	})
}

func (s *interfaceHandlersSuite) TestUndoConnectUndesired(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		}})
	s.state.Unlock()

	// Execute
	chg := s.connectChange("ws", false, true)
	s.state.Lock()
	t := chg.Tasks()[0]
	et := s.state.NewTask("error-trigger", "...")
	et.WaitFor(t)
	chg.AddTask(et)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, "(?s).*error-trigger task.*")

	// Validate
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 0)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)

	var afterUndo map[string]*schema.ConnState
	err = s.state.Get("conns", &afterUndo)
	c.Assert(err, check.IsNil)
	c.Assert(afterUndo, check.DeepEquals, map[string]*schema.ConnState{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:      true,
			Interface: "mock-network",
			Undesired: true,
		}})
}

func (s *interfaceHandlersSuite) TestUndoConnectBackendSetup(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	// Execute
	chg := s.connectChange("ws", false, false)
	s.state.Lock()
	t := chg.Tasks()[0]
	et := s.state.NewTask("error-trigger", "...")
	et.WaitFor(t)
	chg.AddTask(et)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.ErrorMatches, "(?s).*error-trigger task.*")

	// Validate
	conns, err := repo.Connections(s.prj.ProjectId, "ws", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(conns, check.HasLen, 0)

	c.Assert(s.secBackend.SetupCalls, check.HasLen, 4)
	c.Assert(s.secBackend.RemoveCalls, check.HasLen, 0)
}

func (s *interfaceHandlersSuite) TestDiscardConnsSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)
	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
		"42424242/ws-1/consumer:plug 42424242/ws-1/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
		"other/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
		"other/ws/consumer:plug other/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
	})
	s.state.Unlock()

	// Execute
	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("discard-conns", "...")
	t1.Set("sdk", "consumer")
	setWorkshopProject("ws", s.prj, t1)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)

	// Validate
	c.Assert(chg.Tasks()[0].Status(), check.Equals, state.DoneStatus)
	var conns map[string]*schema.ConnState
	s.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 2)

	var removed map[string]*schema.ConnState
	err := t1.Get("removed", &removed)
	c.Assert(err, check.IsNil)
	c.Assert(removed, check.DeepEquals, map[string]*schema.ConnState{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:      true,
			Interface: "mock-network",
			Undesired: true,
		},
		"other/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:      true,
			Interface: "mock-network",
			Undesired: true,
		},
	})
}

func (s *interfaceHandlersSuite) TestUndoDiscardConnsSuccess(c *check.C) {
	// Setup
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumer},
		{psetup, producer},
	})
	repo := s.mgr.Repository()
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumer, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, producer, s.prj.ProjectId, "ws")), check.IsNil)

	s.state.Lock()
	s.state.Set("conns", map[string]interface{}{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
		"other/ws/consumer:plug 42424242/ws/producer:slot": map[string]interface{}{
			"auto":      true,
			"interface": "mock-network",
			"undesired": true,
		},
	})
	s.state.Unlock()

	// Execute
	s.state.Lock()
	chg := s.state.NewChange("sample", "...")
	t1 := s.state.NewTask("discard-conns", "...")
	t1.Set("sdk", "consumer")
	t2 := s.state.NewTask("error-trigger", "...")
	t2.WaitFor(t1)
	setWorkshopProject("ws", s.prj, t1, t2)
	chg.Set("user", "testuser")
	chg.AddTask(t1)
	chg.AddTask(t2)
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()

	// Validate
	c.Assert(t1.Status(), check.Equals, state.UndoneStatus)
	var conns map[string]*schema.ConnState
	s.state.Get("conns", &conns)
	c.Assert(conns, check.DeepEquals, map[string]*schema.ConnState{
		"42424242/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:      true,
			Interface: "mock-network",
			Undesired: true,
		},
		"other/ws/consumer:plug 42424242/ws/producer:slot": {
			Auto:      true,
			Interface: "mock-network",
			Undesired: true,
		},
	})
}

func (s *interfaceHandlersSuite) TestDoDisconnectSetupFailure(c *check.C) {
	// Launch
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumerManyPlugs},
		{psetup, strings.Replace(producer, "mock-network", "mock-ssh-agent", 1)},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, strings.Replace(producer, "mock-network", "mock-ssh-agent", 1), s.prj.ProjectId, "ws")), check.IsNil)

	// Connect
	s.state.Lock()
	c1 := s.state.NewChange("sample", "")
	t1 := s.state.NewTask("connect", "")
	t1.Set("slot", interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})
	t1.Set("plug", interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug-ssh"})
	setWorkshopProject("ws", s.prj, t1)
	c1.AddTask(t1)
	c1.Set("project-id", s.prj.ProjectId)
	c1.Set("user", "testuser")
	s.state.Unlock()

	s.settle(c)

	// Store current connections present in state and repo
	s.state.Lock()
	oldConns := map[string]*schema.ConnState{}
	s.state.Get("conns", &oldConns)

	oldPlugRefs, err := repo.Connected(s.prj.ProjectId, "ws", "consumer", "plug-ssh")
	c.Assert(err, check.IsNil)

	oldSlotRefs, err := repo.Connected(s.prj.ProjectId, "ws", "producer", "slot")
	c.Assert(err, check.IsNil)

	// Force the disconnect to fail
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		return fmt.Errorf("setup failed")
	}

	// Disconnect
	c2 := s.state.NewChange("sample", "")
	t2 := s.state.NewTask("disconnect", "")

	t2.Set("slot", interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})
	t2.Set("plug", interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug-ssh"})
	setWorkshopProject("ws", s.prj, t2)
	c2.AddTask(t2)
	c2.Set("project-id", s.prj.ProjectId)
	c2.Set("user", "testuser")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(c2.Err(), check.ErrorMatches, `cannot perform the following tasks:
-  \(setup failed\)`)

	// Ensure that the connection is not removed from state
	newConns := map[string]*schema.ConnState{}
	s.state.Get("conns", &newConns)
	c.Assert(oldConns, check.DeepEquals, newConns)

	// Ensure the connection remains identical in the repo
	refs, err := repo.Connected(s.prj.ProjectId, "ws", "consumer", "plug-ssh")
	c.Assert(refs, check.DeepEquals, oldPlugRefs)
	c.Assert(err, check.IsNil)

	refs, err = repo.Connected(s.prj.ProjectId, "ws", "producer", "slot")
	c.Assert(refs, check.DeepEquals, oldSlotRefs)
	c.Assert(err, check.IsNil)
}

func (s *interfaceHandlersSuite) TestDoDisconnectSetupFailureAuto(c *check.C) {
	// Launch
	repo := s.mgr.Repository()
	s.launchWorkshop(c, "ws", []testSdkSetup{
		{csetup, consumerManyPlugs},
		{psetup, strings.Replace(producer, "mock-network", "mock-ssh-agent", 1)},
	})
	c.Assert(repo.AddSdk(sdk.MockInfo(c, consumerManyPlugs, s.prj.ProjectId, "ws")), check.IsNil)
	c.Assert(repo.AddSdk(sdk.MockInfo(c, strings.Replace(producer, "mock-network", "mock-ssh-agent", 1), s.prj.ProjectId, "ws")), check.IsNil)

	// Connect
	s.state.Lock()
	c1 := s.state.NewChange("sample", "")
	t1 := s.state.NewTask("connect", "")
	t1.Set("slot", interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})
	t1.Set("plug", interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug-ssh"})
	t1.Set("auto", true)
	setWorkshopProject("ws", s.prj, t1)
	c1.AddTask(t1)
	c1.Set("project-id", s.prj.ProjectId)
	c1.Set("user", "testuser")
	s.state.Unlock()

	s.settle(c)

	// Store current connections present in state and repo
	s.state.Lock()
	oldConns := map[string]*schema.ConnState{}
	s.state.Get("conns", &oldConns)

	oldPlugRefs, err := repo.Connected(s.prj.ProjectId, "ws", "consumer", "plug-ssh")
	c.Assert(err, check.IsNil)

	oldSlotRefs, err := repo.Connected(s.prj.ProjectId, "ws", "producer", "slot")
	c.Assert(err, check.IsNil)

	// Force the disconnect to fail
	s.secBackend.SetupCallback = func(context context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
		return fmt.Errorf("setup failed")
	}

	// Disconnect
	c2 := s.state.NewChange("sample", "")
	t2 := s.state.NewTask("disconnect", "")

	t2.Set("slot", interfaces.SlotRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "producer", Name: "slot"})
	t2.Set("plug", interfaces.PlugRef{ProjectId: s.prj.ProjectId, Workshop: "ws", Sdk: "consumer", Name: "plug-ssh"})
	setWorkshopProject("ws", s.prj, t2)
	c2.AddTask(t2)
	c2.Set("project-id", s.prj.ProjectId)
	c2.Set("user", "testuser")
	s.state.Unlock()

	s.settle(c)

	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(c2.Err(), check.ErrorMatches, `cannot perform the following tasks:
-  \(setup failed\)`)

	// Ensure the connection remains identical in state
	newConns := map[string]*schema.ConnState{}
	s.state.Get("conns", &newConns)
	c.Assert(oldConns, check.DeepEquals, newConns)

	// Ensure the connection remains identical in the repo
	refs, err := repo.Connected(s.prj.ProjectId, "ws", "consumer", "plug-ssh")
	c.Assert(refs, check.DeepEquals, oldPlugRefs)
	c.Assert(err, check.IsNil)

	refs, err = repo.Connected(s.prj.ProjectId, "ws", "producer", "slot")
	c.Assert(refs, check.DeepEquals, oldSlotRefs)
	c.Assert(err, check.IsNil)
}
