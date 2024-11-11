package ifacestate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/exp/maps"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/ifacestate/schema"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

func (m *InterfaceManager) doAutoConnect(task *state.Task, tomb *tomb.Tomb) (err error) {
	st := task.State()
	user, project, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := handlersetup.BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	wp, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	st.Lock()
	s, err := handlersetup.Sdk(task)
	st.Unlock()
	if err != nil {
		return err
	}

	info, err := wp.SdkInfo(ctx, s)
	if err != nil {
		return err
	}

	st.Lock()
	defer st.Unlock()

	chg := task.Change()
	// If auto-connect is executed during refresh, chances are, that there are
	// SDKs that are going to be reinstalled without any changes to their
	// content interface plugs. In this case, their 'source' directories must be
	// set to how it was in the previous workshop instance as some of those
	// plugs may have been remounted with `workshop remount`. To preserve that,
	// we store the content interface connection's 'source' directories when the
	// old workshop/SDK is removed (see the 'disconnect' task handler) and check
	// if any of those are still relevant for the new workshop.
	var remounts map[string]string
	if err := chg.Get("remounts", &remounts); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	// Ensure that all plugs that the workshop bindings bind to actually exist
	// in the repository.
	if err = m.resolveWorkshopBindings(wp); err != nil {
		return err
	}

	// Ensure that connections requested via the workshop file use existing and
	// compatible plugs and slots.
	if err = m.resolveWorkshopConnections(wp); err != nil {
		return err
	}

	// Ensure that if the SDK is connected there will be no conflicting bind
	// mount targets in the workshop, i.e. the situation when a two or more
	// sources are bind mount at the same target in the workshop.
	if err = m.checkConflictingTargets(info); err != nil {
		return err
	}

	return m.connectAuto(task, wp, info, remounts)
}

// Returns content interface connection IDs of the SDK and their corresponding
// plug's dynamic attributes.
func (m *InterfaceManager) remountSources(projectId, w, s string) map[string]string {
	// [ref.ID]source
	var candidates = make(map[string]string)
	refs, err := m.repo.Connections(projectId, w, s)
	if err != nil {
		return nil
	}

	for _, cref := range refs {
		conn, err := m.repo.Connection(cref)
		if err != nil {
			continue
		}
		if conn.Interface() == "mount" {
			attrs := conn.Slot.DynamicAttrs()
			if attrs != nil && attrs["host-source"] != nil {
				candidates[cref.ID()] = attrs["host-source"].(string)
			}
		}
	}
	return candidates
}

func (m *InterfaceManager) batchAutoConnectTasks(wp *workshop.Workshop, info *sdk.Info, refs []*interfaces.ConnRef, plugDynamic, slotDynamic map[string]map[string]interface{}) (*state.TaskSet, error) {

	connectTs := state.NewTaskSet()
	var affected = map[sdk.Ref]bool{}
	for _, ref := range refs {
		connect := m.state.NewTask("connect", fmt.Sprintf("Connect %s to %s", ref.PlugRef.ShortRef(), ref.SlotRef.ShortRef()))

		connect.Set("plug", ref.PlugRef)
		connect.Set("slot", ref.SlotRef)
		connect.Set("auto", true)
		connect.Set("delayed-setup-profile", true)

		if plugDynamic != nil {
			connect.Set("plug-dynamic", plugDynamic[ref.ID()])
		}
		if slotDynamic != nil {
			connect.Set("slot-dynamic", slotDynamic[ref.ID()])
		}
		connectTs.AddTask(connect)

		plugSdk := sdk.Ref{ProjectId: ref.PlugRef.ProjectId, Workshop: ref.PlugRef.Workshop, Sdk: ref.PlugRef.Sdk}
		affected[plugSdk] = true

		slotSdk := sdk.Ref{ProjectId: ref.SlotRef.ProjectId, Workshop: ref.SlotRef.Workshop, Sdk: ref.SlotRef.Sdk}
		affected[slotSdk] = true
	}

	setup := m.state.NewTask("setup-profiles", fmt.Sprintf("Setup %q SDK profile", info.Name))
	setup.Set("sdks", maps.Keys(affected))
	setup.WaitAll(connectTs)

	if len(connectTs.Tasks()) > 0 {
		connectTs.AddTask(setup)
	}

	for _, tsk := range connectTs.Tasks() {
		tsk.Set("workshop", info.Workshop)
		tsk.Set("sdk", info.Name)
		tsk.Set("project", wp.Project)
	}

	return connectTs, nil
}

func workshopConns(wp *workshop.Workshop) []interfaces.ConnRef {
	conns := []interfaces.ConnRef{}
	for _, wconn := range wp.File.Connections {
		conns = append(conns, interfaces.ConnRef{
			PlugRef: interfaces.PlugRef{ProjectId: wp.Project.ProjectId, Workshop: wp.Name, Sdk: wconn.PlugRef.Sdk, Name: wconn.PlugRef.Name},
			SlotRef: interfaces.SlotRef{ProjectId: wp.Project.ProjectId, Workshop: wp.Name, Sdk: wconn.SlotRef.Sdk, Name: wconn.SlotRef.Name},
		})
	}
	return conns
}

func (m *InterfaceManager) connectAuto(task *state.Task, wp *workshop.Workshop, info *sdk.Info, remounts map[string]string) error {
	conns, err := getConns(m.state)
	if err != nil {
		return err
	}

	var connectRefs = []*interfaces.ConnRef{}
	var wconns = workshopConns(wp)
	var plugDynamic = make(map[string]map[string]interface{})
	var slotDynamic = make(map[string]map[string]interface{})

	for _, plug := range info.Plugs {
		ref := interfaces.NewPlugRef(plug)
		master, slaves := MaybeBound(wp, ref)
		if master != ref {
			// the plug is bound to, its connection will be setup together with
			// its master.
			continue
		}
		candidates := m.repo.AutoConnectCandidateSlots(info.ProjectId, info.Workshop,
			info.Name, plug.Name, autoConnectChecker(wconns))

		for _, slot := range candidates {
			connRef := interfaces.NewConnRef(plug, slot)
			if slotDynamic[connRef.ID()] == nil {
				slotDynamic[connRef.ID()] = make(map[string]interface{})
			}

			if src, ok := remounts[connRef.ID()]; ok {
				slotDynamic[connRef.ID()]["host-source"] = src
			}

			slotRef := interfaces.NewSlotRef(slot)
			// save associated binds as a dynamic attribute
			for _, slave := range slaves {
				slref := &interfaces.ConnRef{PlugRef: slave, SlotRef: slotRef}
				if _, ok := conns[slref.ID()]; !ok {
					connectRefs = append(connectRefs, slref)
					plugDynamic[slref.ID()] = make(map[string]interface{})
					plugDynamic[slref.ID()]["bind"] = connRef.ID()
				}
			}

			if _, ok := conns[connRef.ID()]; ok {
				// Suggested connection already exist (or has Undesired flag
				// set) so don't clobber it. NOTE: we don't log anything here as
				// this is a normal and common condition.
				continue
			}
			connectRefs = append(connectRefs, connRef)
		}
	}

	for _, slot := range info.Slots {
		candidates := m.repo.AutoConnectCandidatePlugs(info.ProjectId, info.Workshop,
			info.Name, slot.Name, autoConnectChecker(wconns))
		for _, plug := range candidates {
			ref := interfaces.NewPlugRef(plug)
			master, slaves := MaybeBound(wp, ref)
			if master != ref {
				// the plug is bound to, its connection will be setup together with
				// its master.
				continue
			}
			connRef := interfaces.NewConnRef(plug, slot)

			if slotDynamic[connRef.ID()] == nil {
				slotDynamic[connRef.ID()] = make(map[string]interface{})
			}

			if src, ok := remounts[connRef.ID()]; ok {
				slotDynamic[connRef.ID()]["host-source"] = src
			}

			slotRef := interfaces.NewSlotRef(slot)
			for _, slave := range slaves {
				slref := &interfaces.ConnRef{PlugRef: slave, SlotRef: slotRef}

				slaveInfo := m.repo.Plug(slave.ProjectId, slave.Workshop, slave.Sdk, slave.Name)
				if slaveInfo == nil {
					return fmt.Errorf("SDK %s/%s has no %q plug", slave.Workshop, slave.Sdk, slave.Name)
				}

				if slaveInfo.Interface != plug.Interface {
					return fmt.Errorf("cannot bind %s/%s:%s (%q interface) to %s/%s:%s (%q interface)",
						slave.Workshop, slave.Sdk, slave.Name, slaveInfo.Interface, master.Workshop, master.Sdk, master.Name, plug.Interface)
				}

				if _, ok := conns[slref.ID()]; !ok {
					connectRefs = append(connectRefs, slref)
					plugDynamic[slref.ID()] = make(map[string]interface{})
					plugDynamic[slref.ID()]["bind"] = connRef.ID()
				}
			}

			if _, ok := conns[connRef.ID()]; ok {
				continue
			}

			connectRefs = append(connectRefs, connRef)
		}
	}

	// remounts may be not nil when a previously existing content
	// interface connection (e.g. pre-refresh) needs to be recreated
	// without changes to its 'source' attribute in the new workshop
	// (given the new workshop also has an SDK with exactly the same
	// plug; the target directory may change in the new workshop).
	ts, err := m.batchAutoConnectTasks(wp, info, connectRefs, plugDynamic, slotDynamic)
	if err != nil {
		return err
	}

	handlersetup.InjectTasks(task, ts)
	m.state.EnsureBefore(0)
	task.SetStatus(state.DoneStatus)

	return nil
}

func (m *InterfaceManager) batchDisconnectTasks(p *workshop.Project, workshop, sdkName string,
	conns map[string]*schema.ConnState, refs []*interfaces.ConnRef) *state.TaskSet {
	ts := state.NewTaskSet()

	var prev *state.Task
	for _, ref := range refs {
		task := m.state.NewTask("disconnect",
			fmt.Sprintf("Disconnnect %s from %s", ref.PlugRef.ShortRef(), ref.SlotRef.ShortRef()))
		task.Set("plug", ref.PlugRef)
		task.Set("slot", ref.SlotRef)
		if conn := conns[ref.ID()]; conn != nil && conn.Undesired {
			continue
		}
		task.Set("forget", true)

		if prev != nil {
			task.WaitFor(prev)
		}
		prev = task
		ts.AddTask(task)
	}

	setup := m.state.NewTask("remove-profiles", fmt.Sprintf("Remove %q SDK profile", sdkName))
	setup.WaitAll(ts)

	ts.AddTask(setup)

	for _, tsk := range ts.Tasks() {
		tsk.Set("workshop", workshop)
		tsk.Set("sdk", sdkName)
		tsk.Set("project", p)
	}
	return ts
}

// Disconnects SDK's interface connections and removes the SDK from the
// repository / connections. The SDK must have its manual connections stored and
// reconnected on undo (the auto connections will be reconnected automatically).
func (m *InterfaceManager) doDisconnectInterfaces(task *state.Task, tomb *tomb.Tomb) (err error) {
	st := task.State()
	_, project, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st.Lock()
	defer st.Unlock()
	s, err := handlersetup.Sdk(task)
	if err != nil {
		return err
	}

	// Save 'source' attributes for the content interface connections as some of
	// them may have been remounted to a non-default locations, so these can be
	// set after refresh for this SDK.
	cattrs := m.remountSources(project.ProjectId, w, s)

	if len(cattrs) > 0 {
		chg := task.Change()
		var remounts map[string]string
		if err = chg.Get("remounts", &remounts); errors.Is(err, state.ErrNoState) {
			remounts = make(map[string]string)
		} else if err != nil {
			return err
		}
		maps.Copy(remounts, cattrs)
		chg.Set("remounts", remounts)
	}

	conns, err := getConns(m.state)
	if err != nil {
		return err
	}

	connections, err := m.repo.Connections(project.ProjectId, w, s)
	if err != nil {
		return err
	}

	ts := m.batchDisconnectTasks(project, w, s, conns, connections)

	handlersetup.InjectTasks(task, ts)
	m.state.EnsureBefore(0)
	task.SetStatus(state.DoneStatus)

	return nil
}

func getPlugAndSlotRefs(task *state.Task) (interfaces.PlugRef, interfaces.SlotRef, error) {
	var plugRef interfaces.PlugRef
	var slotRef interfaces.SlotRef
	if err := task.Get("plug", &plugRef); err != nil {
		return plugRef, slotRef, err
	}
	if err := task.Get("slot", &slotRef); err != nil {
		return plugRef, slotRef, err
	}
	return plugRef, slotRef, nil
}

func MaybeBound(w *workshop.Workshop, ref interfaces.PlugRef) (interfaces.PlugRef, []interfaces.PlugRef) {
	var masters = make(map[interfaces.PlugRef][]interfaces.PlugRef)
	var slaves = make(map[interfaces.PlugRef]interfaces.PlugRef)

	for _, s := range w.File.Sdks {
		for name, pl := range s.Plugs {
			if pl.Bind == nil {
				continue
			}
			sdk, plug := pl.Bind.Sdk, pl.Bind.Name
			mkey := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: sdk, Name: plug}
			skey := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: s.Name, Name: name}
			masters[mkey] = append(masters[mkey], skey)
			slaves[skey] = mkey
		}
	}

	srefs, mok := masters[ref]
	mref, sok := slaves[ref]

	if !mok && !sok {
		// not a bound plug
		return ref, nil
	}

	if mok {
		// the ref is a master plug
		return ref, srefs
	}

	if sok {
		// the ref is bound to another plug
		return mref, masters[mref]
	}

	return ref, nil
}

func (m *InterfaceManager) doConnect(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var user string
	err := task.Change().Get("user", &user)
	if err != nil {
		return err
	}

	plugRef, slotRef, err := getPlugAndSlotRefs(task)
	if err != nil {
		return err
	}

	conns, err := getConns(st)
	if err != nil {
		return err
	}

	plug := m.repo.Plug(plugRef.ProjectId, plugRef.Workshop, plugRef.Sdk, plugRef.Name)
	if plug == nil {
		return fmt.Errorf("SDK %q has no %q plug", plugRef.Sdk, plugRef.Name)
	}

	slot := m.repo.Slot(slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name)
	if slot == nil {
		return fmt.Errorf("SDK %q has no %q slot", slotRef.Sdk, slotRef.Name)
	}

	var plugDynamicAttrs, slotDynamicAttrs map[string]interface{}
	if err = task.Get("plug-dynamic", &plugDynamicAttrs); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}
	if err = task.Get("slot-dynamic", &slotDynamicAttrs); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	var autoConnect bool
	if err := task.Get("auto", &autoConnect); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	var delayedSetupProfile bool
	if err := task.Get("delayed-setup-profile", &delayedSetupProfile); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	rev := revert.New()
	defer rev.Fail()

	cref := &interfaces.ConnRef{PlugRef: plugRef, SlotRef: slotRef}
	conn, err := m.repo.Connect(cref, plug.Attrs, plugDynamicAttrs,
		slot.Attrs, slotDynamicAttrs, connectCheck)
	if err != nil || conn == nil {
		return err
	}

	rev.Add(func() {
		err := m.repo.Disconnect(cref.PlugRef.ProjectId, cref.PlugRef.Workshop, cref.PlugRef.Sdk, cref.PlugRef.Name,
			cref.SlotRef.ProjectId, cref.PlugRef.Workshop, cref.SlotRef.Sdk, cref.SlotRef.Name)
		if err != nil {
			logger.Noticef("On doConnect: Cannot revert connection %q", cref.ID())
		}
	})

	if old, ok := conns[cref.ID()]; ok && old.Undesired {
		task.Set("old-conn", old)
	}

	// To setup a profile immediately it needs to be a master plug (i.e. bound
	// to or a completely unbound plug) AND the task must request the setup on
	// the spot and not as part of another task which usually happens with
	// auto-connections.
	if !delayedSetupProfile {
		for _, ref := range []sdk.Ref{conn.Plug.Sdk().Ref(), conn.Slot.Sdk().Ref()} {
			ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
			defer cancel()
			for _, backend := range m.repo.Backends() {
				if err := backend.Setup(ctx, ref, m.repo); err != nil {
					return err
				}
			}
		}
	}

	conns[cref.ID()] = &schema.ConnState{
		Interface:        conn.Interface(),
		StaticPlugAttrs:  conn.Plug.StaticAttrs(),
		DynamicPlugAttrs: conn.Plug.DynamicAttrs(),
		StaticSlotAttrs:  conn.Slot.StaticAttrs(),
		DynamicSlotAttrs: conn.Slot.DynamicAttrs(),
		Auto:             autoConnect,
	}
	setConns(st, conns)

	rev.Success()

	return nil
}

func (m *InterfaceManager) undoConnect(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var user string
	err := task.Change().Get("user", &user)
	if err != nil {
		return err
	}

	plugRef, slotRef, err := getPlugAndSlotRefs(task)
	if err != nil {
		return err
	}

	connRef := interfaces.ConnRef{PlugRef: plugRef, SlotRef: slotRef}
	conns, err := getConns(st)
	if err != nil {
		return err
	}
	var old schema.ConnState
	err = task.Get("old-conn", &old)
	if err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}
	if err == nil {
		conns[connRef.ID()] = &old
	} else {
		delete(conns, connRef.ID())
	}
	setConns(st, conns)

	plug := m.repo.Plug(plugRef.ProjectId, plugRef.Workshop, plugRef.Sdk, plugRef.Name)
	if plug == nil {
		return fmt.Errorf("SDK %q has no %q plug", plugRef.Sdk, plugRef.Name)
	}

	slot := m.repo.Slot(slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name)
	if slot == nil {
		return fmt.Errorf("SDK %q has no %q slot", slotRef.Sdk, slotRef.Name)
	}

	if err = m.repo.Disconnect(plugRef.ProjectId, plugRef.Workshop,
		plugRef.Sdk, plugRef.Name, slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name); err != nil {
		return err
	}

	var delayedSetupProfile bool
	if err := task.Get("delayed-setup-profile", &delayedSetupProfile); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}
	if delayedSetupProfile {
		logger.Debugf("Connect undo handler: skipping setupSdkSecurity for SDKs %q and %q", connRef.PlugRef.Sdk, connRef.SlotRef.Sdk)
		return nil
	}

	for _, ref := range []sdk.Ref{plug.Sdk.Ref(), slot.Sdk.Ref()} {
		ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
		defer cancel()
		for _, backend := range m.repo.Backends() {
			if err := backend.Setup(ctx, ref, m.repo); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *InterfaceManager) doDisconnect(task *state.Task, tomb *tomb.Tomb) (err error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var user string
	err = task.Change().Get("user", &user)

	plugRef, slotRef, err := getPlugAndSlotRefs(task)
	if err != nil {
		return err
	}

	cref := interfaces.ConnRef{PlugRef: plugRef, SlotRef: slotRef}

	conns, err := getConns(st)
	if err != nil {
		return err
	}

	var forget bool
	if err := task.Get("forget", &forget); err != nil && !errors.Is(err, state.ErrNoState) {
		return fmt.Errorf("internal error: cannot read 'forget' flag: %s", err)
	}

	conn, ok := conns[cref.ID()]
	if !ok {
		return fmt.Errorf("internal error: connection %q not found in state", cref.ID())
	}

	// store old connection for undo
	task.Set("old-conn", conn)

	err = m.repo.Disconnect(plugRef.ProjectId, plugRef.Workshop,
		plugRef.Sdk, plugRef.Name, slotRef.ProjectId, slotRef.Workshop, slotRef.Sdk, slotRef.Name)
	if err != nil {
		_, notConnected := err.(*interfaces.NotConnectedError)
		_, noPlugOrSlot := err.(*interfaces.NoPlugOrSlotError)
		// not connected, just forget it.
		if forget && (notConnected || noPlugOrSlot) {
			delete(conns, cref.ID())
			setConns(st, conns)
			// NB: If undo is executed in this scenario, it would restore a
			// previously undesired connection, but will not run a profile
			// setup.
			return nil
		}
		return fmt.Errorf("workshop changed, please retry the operation: %v", err)
	}

	rev := revert.New()
	defer rev.Fail()

	rev.Add(func() {
		_, err1 := m.repo.Connect(&cref, conn.StaticPlugAttrs, conn.DynamicPlugAttrs, conn.StaticSlotAttrs,
			conn.DynamicSlotAttrs, nil)
		if err1 != nil {
			logger.Noticef("On doDisconnect: Cannot recover disconnected %q", cref.ID())
		}
	})

	switch {
	case forget:
		delete(conns, cref.ID())
	case conn.Auto:
		conn.Undesired = true
		conn.DynamicPlugAttrs = nil
		conn.DynamicSlotAttrs = nil
		conn.StaticPlugAttrs = nil
		conn.StaticSlotAttrs = nil
		conns[cref.ID()] = conn
	default:
		delete(conns, cref.ID())
	}
	setConns(st, conns)

	plugSdk, slotSdk := sdk.Ref{ProjectId: plugRef.ProjectId, Workshop: plugRef.Workshop, Sdk: plugRef.Sdk},
		sdk.Ref{ProjectId: slotRef.ProjectId, Workshop: slotRef.Workshop, Sdk: slotRef.Sdk}

	for _, ref := range []sdk.Ref{plugSdk, slotSdk} {
		ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
		defer cancel()
		for _, backend := range m.repo.Backends() {
			if err := backend.Setup(ctx, ref, m.repo); err != nil {
				return err
			}
		}
	}

	rev.Success()
	return nil
}

func (m *InterfaceManager) undoDisconnect(task *state.Task, tomb *tomb.Tomb) (err error) {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var user string
	err = task.Change().Get("user", &user)

	plugRef, slotRef, err := getPlugAndSlotRefs(task)
	if err != nil {
		return err
	}

	var forget bool
	if err := task.Get("forget", &forget); err != nil && !errors.Is(err, state.ErrNoState) {
		return fmt.Errorf("internal error: cannot read 'forget' flag: %s", err)
	}
	var oldconn schema.ConnState
	if err = task.Get("old-conn", &oldconn); err != nil {
		return err
	}

	cref := interfaces.ConnRef{PlugRef: plugRef, SlotRef: slotRef}
	conns, err := getConns(st)
	if err != nil {
		return err
	}
	plug := m.repo.Plug(cref.PlugRef.ProjectId, cref.PlugRef.Workshop, cref.PlugRef.Sdk, cref.PlugRef.Name)
	slot := m.repo.Slot(cref.SlotRef.ProjectId, cref.SlotRef.Workshop, cref.SlotRef.Sdk, cref.SlotRef.Name)

	if forget && (plug == nil || slot == nil || oldconn.Undesired) {
		// we were trying to forget an inactive connection that was
		// referring to a non-existing plug or slot; just restore it
		// in the conns state but do not reconnect via repository.
		conns[cref.ID()] = &oldconn
		setConns(st, conns)
		return nil
	}
	if plug == nil {
		return fmt.Errorf("SDK %q has no %q plug", cref.PlugRef.Sdk, cref.PlugRef.Name)
	}
	if slot == nil {
		return fmt.Errorf("SDK %q has no %q slot", cref.SlotRef.Sdk, cref.SlotRef.Name)
	}

	c, err := m.repo.Connect(&cref, oldconn.StaticPlugAttrs, oldconn.DynamicPlugAttrs,
		oldconn.StaticSlotAttrs, oldconn.DynamicSlotAttrs, nil)
	if err != nil {
		return err
	}

	for _, ref := range []sdk.Ref{c.Plug.Sdk().Ref(), c.Slot.Sdk().Ref()} {
		ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
		defer cancel()
		for _, backend := range m.repo.Backends() {
			if err := backend.Setup(ctx, ref, m.repo); err != nil {
				return err
			}
		}
	}

	conns[cref.ID()] = &oldconn
	setConns(st, conns)
	return nil
}

func (m *InterfaceManager) doDiscard(task *state.Task, tomb *tomb.Tomb) error {
	_, project, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st := task.State()
	st.Lock()
	defer st.Unlock()

	conns, err := getConns(st)
	if err != nil {
		return err
	}
	removed := make(map[string]*schema.ConnState)
	for id := range conns {
		connRef, err := interfaces.ParseConnRef(id)
		if err != nil {
			return err
		}
		if (connRef.PlugRef.ProjectId == project.ProjectId && connRef.PlugRef.Workshop == w) ||
			(connRef.SlotRef.ProjectId == project.ProjectId && connRef.SlotRef.Workshop == w) {
			removed[id] = conns[id]
			delete(conns, id)
		}
	}
	task.Set("removed", removed)
	setConns(st, conns)

	return nil
}

func (m *InterfaceManager) undoDiscard(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	st.Lock()
	defer st.Unlock()

	var removed map[string]*schema.ConnState
	err := task.Get("removed", &removed)
	if err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}

	conns, err := getConns(st)
	if err != nil {
		return err
	}

	for id, connState := range removed {
		conns[id] = connState
	}
	setConns(st, conns)
	task.Set("removed", nil)
	return nil
}

func (m *InterfaceManager) doSetupProfiles(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	st.Lock()
	user, err := handlersetup.User(task.Change())
	st.Unlock()
	if err != nil {
		return err
	}

	var sdks []sdk.Ref
	st.Lock()
	err = task.Get("sdks", &sdks)
	st.Unlock()
	if err != nil {
		return err
	}

	rev := revert.New()
	defer rev.Fail()

	for _, ref := range sdks {
		ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
		defer cancel()
		for _, backend := range m.repo.Backends() {
			if err := backend.Setup(ctx, ref, m.repo); err != nil {
				return err
			}

			ref := ref
			backend := backend
			rev.Add(func() {
				if err1 := backend.Remove(ctx, ref.Workshop, ref.Sdk); err1 != nil {
					logger.Noticef(`On doSetupProfiles: Failed to clean up "%s/%s" SDK backend setup`, ref.Workshop, ref.Sdk)
				}
			})
		}
	}
	rev.Success()
	return nil
}

func (m *InterfaceManager) undoSetupProfiles(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	user, p, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st.Lock()
	s, err := handlersetup.Sdk(task)
	st.Unlock()
	if err != nil {
		return err
	}

	sdkRef := sdk.Ref{ProjectId: p.ProjectId, Workshop: w, Sdk: s}

	var sdks []sdk.Ref
	st.Lock()
	err = task.Get("sdks", &sdks)
	st.Unlock()
	if err != nil {
		return err
	}

	for _, ref := range sdks {
		ctx, cancel := handlersetup.BackendContext(tomb, user, ref.ProjectId)
		defer cancel()
		for _, backend := range m.repo.Backends() {
			if ref != sdkRef {
				if err := backend.Setup(ctx, ref, m.repo); err != nil {
					return err
				}
			} else {
				if err := backend.Remove(ctx, w, sdkRef.Sdk); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *InterfaceManager) doRemoveProfiles(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	user, p, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	st.Lock()
	s, err := handlersetup.Sdk(task)
	st.Unlock()
	if err != nil {
		return err
	}

	ctx, cancel := handlersetup.BackendContext(tomb, user, p.ProjectId)
	defer cancel()

	_, err = m.repo.DisconnectSdk(p.ProjectId, w, s)
	if err != nil {
		return err
	}

	if err = m.repo.RemoveSdk(p.ProjectId, w, s); err != nil {
		return err
	}

	for _, backend := range m.repo.Backends() {
		// If there are not plugs or slots declared by the SDK the profile does
		// not neccessarily exist for the SDK.
		if err := backend.Remove(ctx, w, s); err != nil && !errors.Is(err, workshop.ErrSdkProfileNotFound) {
			return err
		}
	}
	return nil
}

func (m *InterfaceManager) undoRemoveProfiles(task *state.Task, tomb *tomb.Tomb) error {
	st := task.State()
	user, project, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := handlersetup.BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	st.Lock()
	sdkName, err := handlersetup.Sdk(task)
	st.Unlock()
	if err != nil {
		return err
	}

	inst, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	sdkInfo, err := inst.SdkInfo(ctx, sdkName)
	if err != nil {
		return err
	}

	if err := m.repo.AddSdk(sdkInfo); err != nil {
		return err
	}
	return nil
}

func (m *InterfaceManager) doRemount(task *state.Task, tomb *tomb.Tomb) error {
	user, project, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := handlersetup.BackendContext(tomb, user, project.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	defer st.Unlock()

	var plug interfaces.PlugRef
	if err := task.Get("plug", &plug); err != nil {
		return err
	}

	var source string
	if err := task.Get("host-source", &source); err != nil {
		return err
	}

	inst, err := m.backend.Workshop(ctx, w)
	if err != nil {
		return err
	}

	return m.remount(ctx, task, &plug, source, inst.Running)
}

func (m *InterfaceManager) remount(ctx context.Context, task *state.Task, plug *interfaces.PlugRef, source string, workshopRunning bool) error {
	revert := revert.New()
	defer revert.Fail()

	conns, err := getConns(m.state)
	if err != nil {
		return err
	}

	plugConns, err := m.repo.Connected(plug.ProjectId, plug.Workshop, plug.Sdk, plug.Name)
	if err != nil {
		return err
	}
	if len(plugConns) != 1 {
		return fmt.Errorf("plug %q must have exactly one connection to be remounted", plug.ShortRef())
	}
	connRef := plugConns[0]
	// get the connected plug-slot pair to get its existing attributes (source)
	connection, err := m.repo.Connection(connRef)
	if err != nil {
		return err
	}

	var oldSource string
	if err := connection.Slot.Attr("host-source", &oldSource); err != nil {
		return err
	}

	if err := connection.Slot.SetAttr("host-source", source); err != nil {
		return err
	}

	// the connection exists already; this connect is required to update the
	// plug's source attribute
	newConnection, err := m.repo.Connect(connRef, connection.Plug.StaticAttrs(),
		connection.Plug.DynamicAttrs(), connection.Slot.StaticAttrs(), connection.Slot.DynamicAttrs(), nil)
	if err != nil {
		return err
	}

	revert.Add(func() {
		_ = connection.Slot.SetAttr("host-source", oldSource)
		if _, err := m.repo.Connect(connRef, connection.Plug.StaticAttrs(),
			connection.Plug.DynamicAttrs(), connection.Slot.StaticAttrs(), connection.Slot.DynamicAttrs(), nil); err != nil {
			logger.Debugf("On doRemount: cannot reconnect %q plug on a failed remount", plug.ShortRef())
		}
	})

	_, err = os.Stat(oldSource)
	if osutil.IsDirNotExist(err) {
		task.State().Warnf("%s/%s:%s's source at %q does not exist; will attempt to recreate", plug.Workshop, plug.Sdk, plug.Name, oldSource)
	} else if err != nil {
		return err
	} else {
		if err := osutil.Rename(oldSource, source); err != nil {
			if errno, ok := err.(syscall.Errno); ok {
				if workshopRunning {
					if errno == syscall.ENOTEMPTY {
						return fmt.Errorf("new source is not empty; workshop must be stopped to remount safely")
					}
					if errno == syscall.EXDEV {
						return fmt.Errorf("current and new sources are not on the same mounted filesystem; workshop must be stopped to remount safely")
					}
					return err
				} else {
					// if the workshop is stopped, we can perform a remount safely
					// (other fs or non-empty dir), otherwise, return the error
					if errno != syscall.ENOTEMPTY && errno != syscall.EXDEV {
						return err
					}
				}
			} else {
				return err
			}
		} else {
			revert.Add(func() {
				if err := os.Rename(source, oldSource); err != nil {
					logger.Debugf("On doRemount: Cannot rename %s to %s on a failed remount", source, oldSource)
				}
			})
		}
	}

	for _, backend := range m.repo.Backends() {
		if err := backend.Setup(ctx, connection.Plug.Sdk().Ref(), m.repo); err != nil {
			return err
		}
	}

	var auto bool
	if old, ok := conns[connRef.ID()]; ok {
		auto = old.Auto
	}

	conns[connRef.ID()] = &schema.ConnState{
		Interface:        newConnection.Interface(),
		StaticPlugAttrs:  newConnection.Plug.StaticAttrs(),
		DynamicPlugAttrs: newConnection.Plug.DynamicAttrs(),
		StaticSlotAttrs:  newConnection.Slot.StaticAttrs(),
		DynamicSlotAttrs: newConnection.Slot.DynamicAttrs(),
		Auto:             auto,
	}

	setConns(m.state, conns)

	revert.Success()
	return nil
}
