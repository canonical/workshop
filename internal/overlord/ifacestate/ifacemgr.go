package ifacestate

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	backend "github.com/canonical/workshop/internal/interfaces/backends"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/logger"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type InterfaceManager struct {
	state   *state.State
	backend workshop.Backend
	repo    *interfaces.Repository
}

func New(s *state.State, r *state.TaskRunner) *InterfaceManager {
	m := &InterfaceManager{
		state: s,
		repo:  interfaces.NewRepository(),
	}

	s.Lock()
	m.backend = workshop.WorkshopBackend(s)
	s.Unlock()

	r.AddHandler("auto-connect", OnDo(m.doAutoConnect), nil)
	r.AddHandler("auto-disconnect", OnDo(m.doDisconnectInterfaces), nil)

	r.AddHandler("connect", OnDo(m.doConnect), m.undoConnect)
	r.AddHandler("disconnect", OnDo(m.doDisconnect), m.undoDisconnect)

	r.AddHandler("discard-conns", m.doDiscard, m.undoDiscard)

	r.AddHandler("setup-profiles", OnDo(m.doSetupProfiles), m.undoSetupProfiles)
	r.AddHandler("remove-profiles", OnDo(m.doRemoveProfiles), m.undoRemoveProfiles)

	// TODO: there is no use for the undo logic as remount is a single task
	// change that will either finish successfully or fail (in which case it
	// would revert all the partial progress). Shall remount be used as part of
	// a larger change the undo logic must be implemented.
	r.AddHandler("remount", m.doRemount, nil)

	return m
}

func (m *InterfaceManager) Repository() *interfaces.Repository {
	return m.repo
}

type ConnectionState struct {
	// Auto indicates whether the connection was established automatically
	Auto bool
	// ByGadget indicates whether the connection was trigged by the gadget
	ByGadget bool
	// Interface name of the connection
	Interface string
	// Undesired indicates whether the connection, otherwise established
	// automatically, was explicitly disconnected
	Undesired        bool
	StaticPlugAttrs  map[string]interface{}
	DynamicPlugAttrs map[string]interface{}
	StaticSlotAttrs  map[string]interface{}
	DynamicSlotAttrs map[string]interface{}
}

// Active returns true if connection is not undesired and not removed by
// hotplug.
func (c ConnectionState) Active() bool {
	return !c.Undesired
}

// ConnectionStates return the state of connections stored in the state.
// Note that this includes inactive connections (i.e. referring to non-
// existing plug/slots), so this map must be cross-referenced with current
// snap info if needed.
// The state must be locked by the caller.
func ConnectionStates(st *state.State) (connStateByRef map[string]ConnectionState, err error) {
	states, err := getConns(st)
	if err != nil {
		return nil, err
	}

	connStateByRef = make(map[string]ConnectionState, len(states))
	for cref, cstate := range states {
		connStateByRef[cref] = ConnectionState{
			Auto:             cstate.Auto,
			Interface:        cstate.Interface,
			Undesired:        cstate.Undesired,
			StaticPlugAttrs:  cstate.StaticPlugAttrs,
			DynamicPlugAttrs: cstate.DynamicPlugAttrs,
			StaticSlotAttrs:  cstate.StaticSlotAttrs,
			DynamicSlotAttrs: cstate.DynamicSlotAttrs,
		}
	}
	return connStateByRef, nil
}

// ConnectionStates return the state of connections tracked by the manager
func (m *InterfaceManager) ConnectionStates() (connStateByRef map[string]ConnectionState, err error) {
	m.state.Lock()
	defer m.state.Unlock()

	return ConnectionStates(m.state)
}

func (m *InterfaceManager) StartUp() error {
	m.state.Lock()
	defer m.state.Unlock()
	for _, backend := range allSecurityBackends() {
		if err := backend.Initialize(); err != nil {
			return err
		}
		if err := m.repo.AddBackend(backend); err != nil {
			return err
		}
	}

	for _, iface := range builtin.Interfaces() {
		if err := m.repo.AddInterface(iface); err != nil {
			return err
		}
	}

	allprojects, err := m.backend.Projects(context.Background())
	if err != nil {
		return err
	}

	for user, projects := range allprojects {
		ctx := context.WithValue(context.Background(), workshop.ContextUser, user)
		for _, project := range projects {
			pctx := context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)
			_, workshops, err := m.backend.ProjectWorkshops(pctx)
			if err != nil {
				logger.Noticef("Cannot load workshops from %s: %v", project.Path, err)
				continue
			}
			for _, workshop := range workshops {
				// recreate the socket device for every workshop to ensure
				// workshopctl can function (if the daemon was stopped the
				// socket will render /deleted)
				if err := m.recreateInternalMounts(pctx, workshop.Name); err != nil {
					logger.Noticef("Cannot create internal mounts for %q workshop: %v", workshop.Name, err)
				}

				system, err := workshop.SdkInfo(pctx, sdk.System.String())
				if err != nil {
					continue
				}
				if err = m.repo.AddSdk(system); err != nil {
					continue
				}

				infos, err := workshop.ContentInfo(pctx)
				if err != nil {
					logger.Noticef("Cannot obtain the list of installed SDKs for %q workshop: %v", workshop.Name, err)
					continue
				}

				for _, info := range infos {
					if err = m.repo.AddSdk(info); err != nil {
						logger.Noticef("Cannot register %q SDK interfaces:%v", info.Name, err)
						continue
					}
				}
			}
		}
	}

	if _, err := m.reloadConnections("", "", ""); err != nil {
		return err
	}

	return nil
}

// ResolveDisconnect resolves potentially missing plug or slot names and
// returns a list of fully populated connection references that can be
// disconnected.
func (m *InterfaceManager) ResolveDisconnect(
	plugProject, plugWorkshop, plugSdk, plugName string, slotProject, slotWorkshop, slotSdk, slotName string, forget bool) ([]*interfaces.ConnRef, error) {

	var connected func(plugPrj, plugWs, plugSdk, plug, slotPrj, slotWs, slotSdk, slot string) (bool, error)
	var connectedPlugOrSlot func(projectId, workshop, sdkName, plugOrSlotName string) ([]*interfaces.ConnRef, error)

	if forget {
		conns, err := getConns(m.state)
		if err != nil {
			return nil, err
		}
		connected = func(plugPrj, plugWs, plugSdk, plug, slotPrj, slotWs, slotSdk, slot string) (bool, error) {
			cref := interfaces.ConnRef{
				PlugRef: interfaces.PlugRef{ProjectId: plugPrj, Workshop: plugWs, Sdk: plugSdk, Name: plug},
				SlotRef: interfaces.SlotRef{ProjectId: slotPrj, Workshop: slotWs, Sdk: slotSdk, Name: slot},
			}
			_, ok := conns[cref.ID()]
			return ok, nil
		}

		connectedPlugOrSlot = func(projectId, workshop, sdkName, plugOrSlotName string) ([]*interfaces.ConnRef, error) {
			var refs []*interfaces.ConnRef
			for connID := range conns {
				cref, err := interfaces.ParseConnRef(connID)
				if err != nil {
					return nil, err
				}
				if cref.PlugRef.ProjectId == projectId && cref.PlugRef.Workshop == workshop && cref.PlugRef.Sdk == sdkName && cref.PlugRef.Name == plugOrSlotName {
					refs = append(refs, cref)
				}
				if cref.SlotRef.ProjectId == projectId && cref.SlotRef.Workshop == workshop && cref.SlotRef.Sdk == sdkName && cref.SlotRef.Name == plugOrSlotName {
					refs = append(refs, cref)
				}
			}
			return refs, nil
		}
	} else {
		connected = func(plugPrj, plugWs, plugSdk, plug, slotPrj, slotWs, slotSdk, slot string) (bool, error) {
			_, err := m.repo.Connection(&interfaces.ConnRef{
				PlugRef: interfaces.PlugRef{ProjectId: plugPrj, Workshop: plugWs, Sdk: plugSdk, Name: plug},
				SlotRef: interfaces.SlotRef{ProjectId: slotPrj, Workshop: slotWs, Sdk: slotSdk, Name: slot},
			})
			if _, notConnected := err.(*interfaces.NotConnectedError); notConnected {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, nil
		}

		connectedPlugOrSlot = func(projectId, workshop, sdkName, plugOrSlotName string) ([]*interfaces.ConnRef, error) {
			return m.repo.Connected(projectId, workshop, sdkName, plugOrSlotName)
		}
	}
	// There are two allowed forms (see workshop disconnect --help)
	switch {
	// 1: <workshop>/<sdk>:<plug> <workshop>/<sdk>:<slot>
	// Return exactly one plug/slot or an error if it doesn't exist.
	case plugName != "" && slotName != "":
		// The SDK name can be omitted to implicitly refer to the system SDK.
		if plugSdk == "" {
			plugSdk = sdk.System.String()
		}
		// The SDK name can be omitted to implicitly refer to the system SDK.
		if slotSdk == "" {
			slotSdk = sdk.System.String()
		}
		// Ensure that slot and plug are connected
		isConnected, err := connected(plugProject, plugWorkshop, plugSdk, plugName, slotProject, slotWorkshop, slotSdk, slotName)
		if err != nil {
			return nil, err
		}
		if !isConnected {
			if forget {
				return nil, fmt.Errorf("cannot forget connection %s/%s:%s from %s/%s:%s, it was not connected",
					plugWorkshop, plugSdk, plugName, slotWorkshop, slotSdk, slotName)
			}
			return nil, fmt.Errorf("cannot disconnect %s/%s:%s from %s/%s:%s, it is not connected",
				plugWorkshop, plugSdk, plugName, slotWorkshop, slotSdk, slotName)
		}
		return []*interfaces.ConnRef{
			{
				PlugRef: interfaces.PlugRef{ProjectId: plugProject, Workshop: plugWorkshop, Sdk: plugSdk, Name: plugName},
				SlotRef: interfaces.SlotRef{ProjectId: slotProject, Workshop: slotWorkshop, Sdk: slotSdk, Name: slotName},
			}}, nil
	// 2: <workshop>/<sdk>:<plug or slot> (through 1st pair)
	// Return a list of connections involving specified plug or slot.
	case plugWorkshop != "" && plugName != "" && slotWorkshop == "" && slotName == "":
		if plugSdk == "" {
			plugSdk = sdk.System.String()
		}
		return connectedPlugOrSlot(plugProject, plugWorkshop, plugSdk, plugName)
	// 2: <workshop>/<sdk>:<plug or slot> (through 2nd pair)
	// Return a list of connections involving specified plug or slot.
	case plugWorkshop == "" && plugName == "" && slotWorkshop != "" && slotName != "":
		if slotSdk == "" {
			slotSdk = sdk.System.String()
		}
		return connectedPlugOrSlot(slotProject, slotWorkshop, slotSdk, slotName)
	default:
		return nil, fmt.Errorf("allowed forms are <workshop>/<sdk>:<plug> <workshop>/<sdk>:<slot> or <workshop>/<sdk>:<plug or slot>")
	}
}

// Ensure the mounts required by a workshop to function properly were created:
// workshopctl, socket. These mounts are created at the time of launch but can
// become invalid on the daemon restart / update. Thus, recreating them upon
// every daemon restart makes sure they still point to the correct files.
func (m *InterfaceManager) recreateInternalMounts(pctx context.Context, w string) error {
	hostpath := dirs.SocketPath + ".untrusted"
	sname := filepath.Base(hostpath)
	wspath := filepath.Join(dirs.WorkshopRunDir, sname)
	socket := workshop.Mount{Name: "workshop.socket", What: hostpath, Where: wspath}

	_ = m.backend.RemoveWorkshopMount(pctx, w, socket.Name)
	if err := m.backend.AddWorkshopMount(pctx, w, socket); err != nil {
		return err
	}

	// Recreate workshopctl bind mount, this has to be done if, for example,
	// workshopctl was updated to a new version and is shown as /deleted in a
	// workshop.
	workshopctl := workshop.Mount{Name: "workshop.workshopctl", What: filepath.Join(dirs.ExecDir, "workshopctl"),
		Where: "/usr/bin/workshopctl"}

	_ = m.backend.RemoveWorkshopMount(pctx, w, workshopctl.Name)

	if err := m.backend.AddWorkshopMount(pctx, w, workshopctl); err != nil {
		return err
	}

	return nil
}

func (m *InterfaceManager) Ensure() error {
	return nil
}

// reloadConnections reloads connections stored in the state in the repository.
// Using non-empty sdkName the operation can be scoped to connections
// affecting a given sdk.
//
// The return value is the list of affected sdk names.
func (m *InterfaceManager) reloadConnections(projectId, workshop, sdkName string) (map[sdk.Ref]bool, error) {
	conns, err := getConns(m.state)
	if err != nil {
		return nil, err
	}
	connStateChanged := false
	affected := make(map[sdk.Ref]bool)
	for connId, connState := range conns {
		// Skip entries that just mark a connection as undesired. Those don't
		// carry attributes that can go stale.
		if connState.Undesired {
			continue
		}
		connRef, err := interfaces.ParseConnRef(connId)
		if err != nil {
			return nil, err
		}
		// Apply filtering, this allows us to reload only a subset of
		// connections (and similarly, refresh the static attributes of only a
		// subset of connections).
		if projectId != "" && workshop != "" && sdkName != "" {
			if connRef.PlugRef.ProjectId != projectId && connRef.SlotRef.ProjectId != projectId {
				continue
			}
			if connRef.PlugRef.Workshop != workshop && connRef.SlotRef.Workshop != workshop {
				continue
			}
			if connRef.PlugRef.Sdk != sdkName && connRef.SlotRef.Sdk != sdkName {
				continue
			}
		}

		plugInfo := m.repo.Plug(connRef.PlugRef.ProjectId, connRef.PlugRef.Workshop, connRef.PlugRef.Sdk, connRef.PlugRef.Name)
		slotInfo := m.repo.Slot(connRef.SlotRef.ProjectId, connRef.SlotRef.Workshop, connRef.SlotRef.Sdk, connRef.SlotRef.Name)

		// The connection refers to a plug or slot that doesn't exist anymore, e.g. because of a refresh
		// to a new sdk revision that doesn't have the given plug/slot.
		if plugInfo == nil || slotInfo == nil {
			// automatic connection can simply be removed (it will be re-created automatically if needed)
			// as long as it wasn't disconnected manually; note that undesired flag is taken care of at
			// the beginning of the loop.
			if connState.Auto {
				delete(conns, connId)
				connStateChanged = true
			}
			// otherwise keep it and silently ignore, e.g. in case of a revert.
			continue
		}

		staticPlugAttrs := connState.StaticPlugAttrs
		staticSlotAttrs := connState.StaticSlotAttrs

		// Note: reloaded connections are not checked against policy again, and also we don't call BeforeConnect* methods on them.
		if _, err := m.repo.Connect(connRef, staticPlugAttrs, connState.DynamicPlugAttrs, staticSlotAttrs, connState.DynamicSlotAttrs, nil); err != nil {
			logger.Noticef("%s", err)
		} else {
			// If the connection succeeded update the connection state and keep
			// track of the sdks that were affected.

			affected[plugInfo.Sdk.Ref()] = true
			affected[slotInfo.Sdk.Ref()] = true
		}
	}
	if connStateChanged {
		setConns(m.state, conns)
	}
	return affected, nil
}

func (m *InterfaceManager) resolveWorkshopBindings(w *workshop.Workshop) error {
	for _, s := range w.File.Sdks {
		for _, plug := range s.Plugs {
			if plug.Bind != nil {
				master := m.repo.Plug(w.Project.ProjectId, w.Name, plug.Bind.Sdk, plug.Bind.Name)
				if master == nil {
					sdkRef := fmt.Sprintf("%s/%s", w.Name, plug.Bind.Sdk)
					return fmt.Errorf("SDK %q has no plug named %q", sdkRef, plug.Bind.Name)
				}
			}
		}
	}
	return nil
}

func (m *InterfaceManager) resolveWorkshopConnections(w *workshop.Workshop) error {
	for _, conn := range w.File.Connections {
		_, err := m.repo.ResolveConnect(w.Project.ProjectId, w.Name, conn.PlugRef.Sdk, conn.PlugRef.Name,
			w.Project.ProjectId, w.Name, conn.SlotRef.Sdk, conn.SlotRef.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *InterfaceManager) checkConflictingTargets(sdkInfo *sdk.Info) error {
	allPlugs := m.repo.AllPlugs("mount")

	for _, plug := range sdkInfo.Plugs {
		if plug.Interface != "mount" {
			continue
		}
		candidateTarget, _ := plug.Lookup("workshop-target")

		idx := slices.IndexFunc(allPlugs, func(pi *sdk.PlugInfo) bool {
			// only plugs from the same workshop will be considered
			if pi.Sdk.ProjectId != plug.Sdk.ProjectId || pi.Sdk.Workshop != plug.Sdk.Workshop {
				return false
			}
			// exclude oneself
			if pi.Sdk.Ref() == plug.Sdk.Ref() && pi.Name == plug.Name {
				return false
			}
			target, _ := pi.Lookup("workshop-target")
			return target == candidateTarget
		})
		if idx != -1 {
			return fmt.Errorf(`cannot connect "%s/%s:%s": target %s is also mounted by %s/%s:%s`, plug.Sdk.Workshop, plug.Sdk.Name, plug.Name, candidateTarget,
				allPlugs[idx].Sdk.Workshop, allPlugs[idx].Sdk.Name, allPlugs[idx].Name)
		}
	}
	return nil
}

var securityBackendsOverride []interfaces.SecurityBackend

// allSecurityBackends returns a set of the available security backends or the mocked ones, ready to be initialized.
func allSecurityBackends() []interfaces.SecurityBackend {
	if securityBackendsOverride != nil {
		return securityBackendsOverride
	}
	return backend.All()
}

// MockSecurityBackends mocks the list of security backends that are used for setting up security.
//
// This function is public because it is referenced in the daemon
func MockSecurityBackends(be []interfaces.SecurityBackend) func() {
	if be == nil {
		// nil is a marker, use an empty slice instead
		be = []interfaces.SecurityBackend{}
	}
	old := securityBackendsOverride
	securityBackendsOverride = be
	return func() { securityBackendsOverride = old }
}
