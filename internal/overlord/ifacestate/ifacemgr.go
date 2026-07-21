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

package ifacestate

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	backend "github.com/canonical/workshop/internal/interfaces/backends"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/syscheck"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/x11"
)

type InterfaceManager struct {
	state        *state.State
	backend      workshop.Backend
	repo         *interfaces.Repository
	backendReady bool
}

func New(s *state.State, r *state.TaskRunner) *InterfaceManager {
	m := &InterfaceManager{
		state: s,
		repo:  interfaces.NewRepository(),
	}

	s.Lock()
	m.backend = workshop.WorkshopBackend(s)
	s.Unlock()

	r.AddHandler("resolve-interfaces", OnDo(m.doResolveInterfaces), nil)
	r.AddHandler("auto-connect", OnDo(m.doAutoConnect), nil)
	r.AddHandler("auto-disconnect", OnDo(m.doDisconnectInterfaces), nil)

	r.AddHandler("connect", OnDo(m.doConnect), OnUndo(m.undoConnect))
	r.AddHandler("disconnect", OnDo(m.doDisconnect), OnUndo(m.undoDisconnect))

	r.AddHandler("discard-conns", m.doDiscard, OnUndo(m.undoDiscard))

	r.AddHandler("setup-profiles", OnDo(m.doSetupProfiles), OnUndo(m.undoSetupProfiles))
	r.AddHandler("remove-profiles", OnDo(m.doRemoveProfiles), nil)

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
	// Interface name of the connection
	Interface string
	// Undesired indicates whether the connection, otherwise established
	// automatically, was explicitly disconnected
	Undesired        bool
	StaticPlugAttrs  map[string]any
	DynamicPlugAttrs map[string]any
	StaticSlotAttrs  map[string]any
	DynamicSlotAttrs map[string]any
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
	for _, backend := range allSecurityBackends() {
		if err := backend.Initialize(); err != nil {
			m.state.Unlock()
			return err
		}
		if err := m.repo.AddBackend(backend); err != nil {
			m.state.Unlock()
			return err
		}
	}

	for _, iface := range builtin.Interfaces() {
		if err := m.repo.AddInterface(iface); err != nil {
			m.state.Unlock()
			return err
		}
	}
	m.state.Unlock()

	if err := m.ensureBackendInit(); err != nil {
		// LXD may not be available yet; do not propagate the error so the
		// daemon can start. The syscheck registered below keeps the daemon in
		// degraded mode and retries via the recovery ticker until it succeeds.
		logger.Noticef("Interface manager backend init deferred: %v", err)
	}

	// Register the syscheck HERE (after adding backends and interfaces to the
	// repo) so the initial CheckSystem call in runDaemon — which fires before
	// d.Start() — does not trigger ensureBackendInit prematurely.  Running
	// ensureBackendInit before AddBackend/AddInterface are called means
	// repo.Plug returns nil for every connection, causing reloadConnections to
	// skip all of them.  Once backendReady is mistakenly set to true, the
	// subsequent call from StartUp() is a no-op and connections are never loaded.
	syscheck.RegisterCheck(m.ensureBackendInit)
	return nil
}

// ensureBackendInit performs the LXD-dependent part of startup: it loads all
// existing projects and workshops, recreates internal mounts, registers SDK
// interfaces, and reloads connections. It is registered as a syscheck at the
// end of StartUp so the daemon stays in degraded mode until it succeeds and
// can recover if LXD becomes available later.
func (m *InterfaceManager) ensureBackendInit() error {
	if m.backendReady {
		return nil
	}

	m.state.Lock()
	defer m.state.Unlock()

	allprojects, err := m.backend.Projects(context.Background())
	if err != nil {
		return fmt.Errorf("interface manager not ready: %w", err)
	}

	for user, projects := range allprojects {

		ctx := context.WithValue(context.Background(), workshop.ContextUser, user)
		for _, project := range projects {
			pctx := context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)
			workshops, err := m.backend.ProjectWorkshops(pctx)
			if err != nil {
				logger.Noticef("Cannot load workshops from %q: %v", project.Path, err)
				continue
			}
			for _, workshop := range workshops {
				// recreate the socket device for every workshop to ensure
				// workshopctl can function (if the daemon was stopped the
				// socket will render /deleted)
				if err := m.recreateInternalMounts(pctx, workshop.Name); err != nil {
					logger.Noticef("Cannot create internal mounts for %q workshop: %v", workshop.Name, err)
				}

				infos, err := workshop.SdkInfosByInstallOrder(pctx)
				if err != nil {
					logger.Noticef("Cannot obtain the installed SDKs for %q workshop: %v", workshop.Name, err)
					continue
				}

				for _, info := range infos {
					if err = m.repo.AddSdk(info); err != nil {
						logger.Noticef("Cannot register %q SDK interfaces: %v", info.Name, err)
						continue
					}
				}
			}
		}
		// The .Xauthority cookie contains a 128bit key used to authenticate
		// consumers of the X11 socket. It is generated on each boot with a random
		// suffix, because of this we need to ensure there exists a
		// consistently-named copy of the cookie for the LXC profile.
		// this consistency across reboots here.}
		err = updateXauthority(user)
		if err != nil {
			logger.Noticef("cannot copy Xauthority file for user %q, X11 applications may not work: %v", user, err)
		}
	}
	_, err = m.reloadConnections("", "", "")
	if err != nil {
		return err
	}
	m.backendReady = true
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
				PlugRef: sdk.PlugRef{ProjectId: plugPrj, Workshop: plugWs, Sdk: plugSdk, Name: plug},
				SlotRef: sdk.SlotRef{ProjectId: slotPrj, Workshop: slotWs, Sdk: slotSdk, Name: slot},
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
				PlugRef: sdk.PlugRef{ProjectId: plugPrj, Workshop: plugWs, Sdk: plugSdk, Name: plug},
				SlotRef: sdk.SlotRef{ProjectId: slotPrj, Workshop: slotWs, Sdk: slotSdk, Name: slot},
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
		plugRef := sdk.PlugRef{ProjectId: plugProject, Workshop: plugWorkshop, Sdk: plugSdk, Name: plugName}
		slotRef := sdk.SlotRef{ProjectId: slotProject, Workshop: slotWorkshop, Sdk: slotSdk, Name: slotName}
		if !isConnected {
			if forget {
				return nil, fmt.Errorf("cannot forget connection between %q and %q: not connected",
					plugRef.ShortRef(), slotRef.ShortRef())
			}
			return nil, fmt.Errorf("cannot disconnect %q from %q: not connected",
				plugRef.ShortRef(), slotRef.ShortRef())
		}
		return []*interfaces.ConnRef{{PlugRef: plugRef, SlotRef: slotRef}}, nil
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

// Ensure the workshopctl mount (required by a workshop to function properly)
// is created. This mount is created at the time of launch but can become
// invalid on the daemon restart / update. Thus, recreating it upon every
// daemon restart makes sure it still points to the correct files.
func (m *InterfaceManager) recreateInternalMounts(pctx context.Context, w string) error {
	// Recreate the workshopctl bind mount, this has to be done if, for
	// example, the Workshop snap was updated to a new revision.
	mount := workshop.Mount{
		Name:     "workshop.bin",
		Type:     workshop.HostWorkshop,
		What:     filepath.Dir(dirs.WorkshopCtlPath),
		Where:    dirs.WorkshopGuestBinDir,
		ReadOnly: true,
	}

	_ = m.backend.RemoveWorkshopMount(pctx, w, mount.Name)

	if err := m.backend.AddWorkshopMount(pctx, w, mount); err != nil {
		return err
	}

	// TODO: remove this in future. Old workshops may still have a single-file
	// workshopctl mount. At some point the source path will no longer exist,
	// and LXD will complain about it. Currently workshopctl is only used
	// during launch, refresh and restore. The first two will use the new
	// symlink in /usr/local/bin/workshopctl. Restore won't happen because
	// we disallow restoring to an unsupported snapshot revision.
	_ = m.backend.RemoveWorkshopMount(pctx, w, "workshop.workshopctl")

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
		for name, plug := range s.Plugs {
			if plug.Bind == nil {
				continue
			}

			master := m.repo.Plug(w.Project.ProjectId, w.Name, plug.Bind.Sdk, plug.Bind.Name)
			if master == nil {
				sdkRef := sdk.Ref{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: plug.Bind.Sdk}
				return fmt.Errorf("%q SDK has no plug named %q", sdkRef.ShortRef(), plug.Bind.Name)
			}

			slave := m.repo.Plug(w.Project.ProjectId, w.Name, s.Name, name)
			if slave == nil {
				sdkRef := sdk.Ref{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: s.Name}
				return fmt.Errorf("internal error: %q SDK has no plug named %q", sdkRef.ShortRef(), name)
			}

			if slave.Interface != master.Interface {
				return fmt.Errorf("%s plug %q incompatible with %s plug %q", slave.Interface, slave.Ref().ShortRef(), master.Interface, master.Ref().ShortRef())
			}

			if slave.Label != master.Label || !reflect.DeepEqual(slave.Attrs, master.Attrs) {
				return fmt.Errorf("plugs %q and %q have different attributes", slave.Ref().ShortRef(), master.Ref().ShortRef())
			}
		}
	}
	return nil
}

func (m *InterfaceManager) resolveWorkshopConnections(w *workshop.Workshop) error {
	for _, conn := range w.File.Connections {
		if _, ok := w.Sdks[conn.PlugRef.Sdk]; !ok {
			return fmt.Errorf("invalid connection between %q and %q: %q SDK not found", conn.PlugRef, conn.SlotRef, conn.PlugRef.Sdk)
		}
		if _, ok := w.Sdks[conn.SlotRef.Sdk]; !ok {
			return fmt.Errorf("invalid connection between %q and %q: %q SDK not found", conn.PlugRef, conn.SlotRef, conn.SlotRef.Sdk)
		}

		_, err := m.repo.ResolveConnect(w.Project.ProjectId, w.Name, conn.PlugRef.Sdk, conn.PlugRef.Name,
			w.Project.ProjectId, w.Name, conn.SlotRef.Sdk, conn.SlotRef.Name)
		if err != nil {
			return fmt.Errorf("invalid connection between %q and %q: %w", conn.PlugRef, conn.SlotRef, err)
		}
	}
	return nil
}

func (m *InterfaceManager) checkConflictingMounts(w *workshop.Workshop) error {
	var plugs []*sdk.PlugInfo
	for _, sk := range w.Sdks {
		for _, plug := range m.repo.Plugs(w.Project.ProjectId, w.Name, sk.Name) {
			if plug.Interface == "mount" {
				plugs = append(plugs, plug)
			}
		}
	}

	sdks := map[string]workshop.SdkRecord{}
	for _, sk := range w.File.Sdks {
		sdks[sk.Name] = sk
	}

	for _, plug := range plugs {
		if sdks[plug.Sdk.Name].Plugs[plug.Name].Bind != nil {
			continue
		}
		candidateTarget, _ := plug.Lookup("workshop-target")

		idx := slices.IndexFunc(plugs, func(pi *sdk.PlugInfo) bool {
			// exclude oneself
			if pi.Sdk.Name == plug.Sdk.Name && pi.Name == plug.Name {
				return false
			}
			// exclude bound plugs
			if sdks[pi.Sdk.Name].Plugs[pi.Name].Bind != nil {
				return false
			}
			target, _ := pi.Lookup("workshop-target")
			return target == candidateTarget
		})
		if idx >= 0 {
			return fmt.Errorf(`conflicting target %q: plug %q must bind to plug %q`,
				candidateTarget, plug.Ref().ShortRef(), plugs[idx].Ref().ShortRef())
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

// updateXauthority determines user and environment information, then calls
// MigrateXauthority
func updateXauthority(user string) error {
	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return err
	}

	return x11.MigrateXauthority(usr, env["XAUTHORITY"])
}
