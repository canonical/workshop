// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type collectFilter struct {
	projectId string
	workshop  string
	ifaceName string
	connected bool
}

func (c *collectFilter) plugOrConnectedSlotMatches(plug *sdk.PlugRef, connectedSlots []sdk.SlotRef) bool {
	for _, slot := range connectedSlots {
		if c.slotOrConnectedPlugMatches(&slot, nil) {
			return true
		}
	}
	if (c.projectId != plug.ProjectId) || (c.workshop != "" && plug.Workshop != c.workshop) {
		return false
	}
	return true
}

func (c *collectFilter) slotOrConnectedPlugMatches(slot *sdk.SlotRef, connectedPlugs []sdk.PlugRef) bool {
	for _, plug := range connectedPlugs {
		if c.plugOrConnectedSlotMatches(&plug, nil) {
			return true
		}
	}
	if (c.projectId != slot.ProjectId) || (c.workshop != "" && slot.Workshop != c.workshop) {
		return false
	}
	return true
}

func (c *collectFilter) ifaceMatches(ifaceName string) bool {
	if c.ifaceName != "" && c.ifaceName != ifaceName {
		return false
	}
	return true
}

type bySlotRef []sdk.SlotRef

func (b bySlotRef) Len() int      { return len(b) }
func (b bySlotRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b bySlotRef) Less(i, j int) bool {
	return b[i].SortsBefore(b[j])
}

type byPlugRef []sdk.PlugRef

func (b byPlugRef) Len() int      { return len(b) }
func (b byPlugRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byPlugRef) Less(i, j int) bool {
	return b[i].SortsBefore(b[j])
}

// mergeAttrs merges attributes from 2 disjoint sets of static and dynamic slot or
// plug attributes into a single map.
func mergeAttrs(one map[string]any, other map[string]any) map[string]any {
	merged := make(map[string]any, len(one)+len(other))
	for k, v := range one {
		merged[k] = v
	}
	for k, v := range other {
		merged[k] = v
	}
	return merged
}

func collectConnections(ifaceMgr *ifacestate.InterfaceManager, filter collectFilter) (*connectionsJSON, error) {
	repo := ifaceMgr.Repository()
	ifaces := repo.Interfaces()

	var connsjson connectionsJSON
	var connStates map[string]ifacestate.ConnectionState
	plugConns := map[string][]sdk.SlotRef{}
	slotConns := map[string][]sdk.PlugRef{}

	var err error
	connStates, err = ifaceMgr.ConnectionStates()
	if err != nil {
		return nil, err
	}

	connsjson.Established = make([]connectionJSON, 0, len(connStates))
	connsjson.Plugs = make([]*plugJSON, 0, len(ifaces.Plugs))
	connsjson.Slots = make([]*slotJSON, 0, len(ifaces.Slots))

	for crefStr, cstate := range connStates {
		if cstate.Undesired && filter.connected {
			continue
		}

		cref, err := interfaces.ParseConnRef(crefStr)
		if err != nil {
			return nil, err
		}

		if repo.Plug(cref.PlugRef.ProjectId, cref.PlugRef.Workshop, cref.PlugRef.Sdk, cref.PlugRef.Name) == nil ||
			repo.Slot(cref.SlotRef.ProjectId, cref.SlotRef.Workshop, cref.SlotRef.Sdk, cref.SlotRef.Name) == nil {
			continue
		}

		if !filter.plugOrConnectedSlotMatches(&cref.PlugRef, nil) && !filter.slotOrConnectedPlugMatches(&cref.SlotRef, nil) {
			continue
		}
		if !filter.ifaceMatches(cstate.Interface) {
			continue
		}
		plugRef := cref.PlugRef
		slotRef := cref.SlotRef
		plugID := plugRef.String()
		slotID := slotRef.String()

		cj := connectionJSON{
			Slot:      slotRef,
			Plug:      plugRef,
			Manual:    !cstate.Auto,
			Interface: cstate.Interface,
			PlugAttrs: mergeAttrs(cstate.StaticPlugAttrs, cstate.DynamicPlugAttrs),
			SlotAttrs: mergeAttrs(cstate.StaticSlotAttrs, cstate.DynamicSlotAttrs),
		}
		if cstate.Undesired {
			// explicitly disconnected are always manual
			cj.Manual = true
			connsjson.Undesired = append(connsjson.Undesired, cj)
		} else {
			plugConns[plugID] = append(plugConns[plugID], slotRef)
			slotConns[slotID] = append(slotConns[slotID], plugRef)

			connsjson.Established = append(connsjson.Established, cj)
		}
	}

	for _, plug := range ifaces.Plugs {
		plugRef := plug.Ref()
		connectedSlots, connected := plugConns[plugRef.String()]
		if !connected && filter.connected {
			continue
		}
		if !filter.ifaceMatches(plug.Interface) || !filter.plugOrConnectedSlotMatches(&plugRef, connectedSlots) {
			continue
		}
		sort.Sort(bySlotRef(connectedSlots))
		var bind *sdk.PlugRef
		if pb, ok := plug.Sdk.PlugBinds[plug.Name]; ok {
			bind = &pb
		}
		pj := &plugJSON{
			ProjectId:   plugRef.ProjectId,
			Workshop:    plugRef.Workshop,
			Sdk:         plugRef.Sdk,
			Name:        plugRef.Name,
			Interface:   plug.Interface,
			Attrs:       plug.Attrs,
			Label:       plug.Label,
			Bind:        bind,
			Connections: connectedSlots,
		}
		connsjson.Plugs = append(connsjson.Plugs, pj)
	}
	for _, slot := range ifaces.Slots {
		slotRef := slot.Ref()
		connectedPlugs, connected := slotConns[slotRef.String()]
		if !connected && filter.connected {
			continue
		}
		if !filter.ifaceMatches(slot.Interface) || !filter.slotOrConnectedPlugMatches(&slotRef, connectedPlugs) {
			continue
		}
		sort.Sort(byPlugRef(connectedPlugs))
		sj := &slotJSON{
			ProjectId:   slotRef.ProjectId,
			Workshop:    slotRef.Workshop,
			Sdk:         slotRef.Sdk,
			Name:        slotRef.Name,
			Interface:   slot.Interface,
			Attrs:       slot.Attrs,
			Label:       slot.Label,
			Connections: connectedPlugs,
		}
		connsjson.Slots = append(connsjson.Slots, sj)
	}
	return &connsjson, nil
}

type byCrefConnJSON []connectionJSON

func (b byCrefConnJSON) Len() int      { return len(b) }
func (b byCrefConnJSON) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byCrefConnJSON) Less(i, j int) bool {
	icj := b[i]
	jcj := b[j]
	iCref := interfaces.ConnRef{PlugRef: icj.Plug, SlotRef: icj.Slot}
	jCref := interfaces.ConnRef{PlugRef: jcj.Plug, SlotRef: jcj.Slot}
	sortsBefore := iCref.SortsBefore(&jCref)
	return sortsBefore
}

func checkWorkshopExists(ctx context.Context, manager *workshopstate.WorkshopManager, projectId, name string) error {
	_, err := manager.Workshop(ctx, name, projectId)
	return err
}

func v1GetConnections(c *Command, r *http.Request, _ *userState) Response {
	query := r.URL.Query()
	projectId := query.Get("project-id")
	workshop := query.Get("workshop")
	ifaceName := query.Get("interface")
	qselect := query.Get("select")

	if projectId == "" {
		return statusBadRequest("project-id must not be empty")
	}

	if qselect != "all" && qselect != "" {
		return statusBadRequest("unsupported select qualifier")
	}
	onlyConnected := qselect == ""

	if workshop != "" {
		st := c.d.overlord.State()
		st.Lock()
		err := checkWorkshopExists(r.Context(), c.d.overlord.WorkshopManager(), projectId, workshop)
		st.Unlock()
		if err != nil {
			return statusNotFound("cannot access %q workshop: %w", workshop, err)
		}
	}

	connsjson, err := collectConnections(c.d.overlord.InterfaceManager(), collectFilter{
		projectId: projectId,
		workshop:  workshop,
		ifaceName: ifaceName,
		connected: onlyConnected,
	})
	if err != nil {
		return statusInternalError("collecting connection information failed: %w", err)
	}
	sort.Sort(byCrefConnJSON(connsjson.Established))
	sort.Sort(byCrefConnJSON(connsjson.Undesired))

	return SyncResponse(connsjson, http.StatusOK)
}

func newConnectionChange(st *state.State, user string, tasks []*state.TaskSet, reqData *interfaceAction) *state.Change {
	summary := fmt.Sprintf("%s %s", cases.Title(language.BritishEnglish).String(reqData.Action),
		fmt.Sprintf("%s/%s:%s", reqData.Plugs[0].Workshop, reqData.Plugs[0].Sdk, reqData.Plugs[0].Name))

	change := st.NewChange(reqData.Action, summary)
	change.Set("user", user)
	change.Set("project-id", reqData.Plugs[0].ProjectId)
	for _, ts := range tasks {
		change.AddAll(ts)
	}
	return change
}

func v1PostConnections(c *Command, r *http.Request, _ *userState) Response {
	var a interfaceAction
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&a); err != nil {
		return statusBadRequest("cannot decode request body into an interface action: %w", err)
	}
	if a.Action == "" {
		return statusBadRequest("interface action not specified")
	}
	if len(a.Plugs) > 1 || len(a.Slots) > 1 {
		return statusNotImplemented("many-to-many operations are not implemented")
	}
	if a.Action != "connect" && a.Action != "disconnect" {
		return statusBadRequest("unsupported interface action: %q", a.Action)
	}
	if len(a.Plugs) == 0 || len(a.Slots) == 0 {
		return statusBadRequest("at least one plug and slot is required")
	}

	user, ok := r.Context().Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("internal error: no user associated with the request")
	}

	var err error
	var tasksets []*state.TaskSet

	st := c.d.overlord.State()
	st.Lock()
	defer st.Unlock()

	checkInstalled := func(projectId, workshopName string) error {
		if err := checkWorkshopExists(r.Context(), c.d.overlord.WorkshopManager(), projectId, workshopName); err != nil {
			return err
		}
		return nil
	}

	for i := range a.Plugs {
		if a.Plugs[i].Workshop == "" && a.Plugs[i].Name == "" {
			continue
		}
		if err := checkInstalled(a.Plugs[i].ProjectId, a.Plugs[i].Workshop); err != nil {
			return statusNotFound("cannot access %q workshop: %w", a.Plugs[i].Workshop, err)
		}
	}
	for i := range a.Slots {
		if a.Slots[i].Workshop == "" && a.Slots[i].Name == "" {
			continue
		}
		if err := checkInstalled(a.Slots[i].ProjectId, a.Slots[i].Workshop); err != nil {
			return statusNotFound("cannot access %q workshop: %w", a.Slots[i].Workshop, err)
		}
	}

	switch a.Action {
	case "connect":
		var connRef *interfaces.ConnRef
		repo := c.d.overlord.InterfaceManager().Repository()
		connRef, err = repo.ResolveConnect(a.Plugs[0].ProjectId, a.Plugs[0].Workshop, a.Plugs[0].Sdk, a.Plugs[0].Name,
			a.Slots[0].ProjectId, a.Slots[0].Workshop, a.Slots[0].Sdk, a.Slots[0].Name)
		if err != nil {
			err = fmt.Errorf("cannot resolve connection: %w", err)
		} else {
			wsmgr := c.d.overlord.WorkshopManager()
			var plugW, slotW *workshop.Workshop
			plugW, err = wsmgr.Workshop(r.Context(), connRef.PlugRef.Workshop, connRef.PlugRef.ProjectId)
			if err != nil {
				break
			}
			slotW, err = wsmgr.Workshop(r.Context(), connRef.SlotRef.Workshop, connRef.SlotRef.ProjectId)
			if err != nil {
				break
			}
			ts, connErr := ifacestate.Connect(st, plugW, slotW, connRef)
			if connErr != nil {
				if _, ok := connErr.(*ifacestate.ErrAlreadyConnected); !ok {
					return statusBadRequest("%w", connErr)
				}
			} else {
				tasksets = append(tasksets, ts)
			}
		}
	case "disconnect":
		var conns []*interfaces.ConnRef
		conns, err = c.d.overlord.InterfaceManager().ResolveDisconnect(a.Plugs[0].ProjectId, a.Plugs[0].Workshop, a.Plugs[0].Sdk, a.Plugs[0].Name,
			a.Slots[0].ProjectId, a.Slots[0].Workshop, a.Slots[0].Sdk, a.Slots[0].Name, a.Forget)
		if err == nil && len(conns) == 0 {
			return statusBadRequest("nothing to do")
		}
		repo := c.d.overlord.InterfaceManager().Repository()
		seen := map[interfaces.ConnRef]bool{}
		for _, connRef := range conns {
			var ts *state.TaskSet
			wsmgr := c.d.overlord.WorkshopManager()
			var plugW, slotW *workshop.Workshop
			plugW, err = wsmgr.Workshop(r.Context(), connRef.PlugRef.Workshop, connRef.PlugRef.ProjectId)
			if err != nil {
				break
			}
			slotW, err = wsmgr.Workshop(r.Context(), connRef.SlotRef.Workshop, connRef.SlotRef.ProjectId)
			if err != nil {
				break
			}

			if !a.Forget {
				// Ensure the connection exists if it is not going to be
				// forgotten (if forget is true the connection may present only
				// in the state and not in the repository).
				_, err = repo.Connection(connRef)
				if err != nil {
					break
				}
			}
			ts, err = ifacestate.Disconnect(st, plugW, slotW, connRef, a.Forget, seen)
			if err != nil {
				break
			}

			if len(ts.Tasks()) > 0 {
				ts.JoinLane(st.NewLane())
				tasksets = append(tasksets, ts)
			}
		}
	}
	if err != nil {
		return statusBadRequest("%w", err)
	}

	change := newConnectionChange(st, user, tasksets, &a)
	if len(change.Tasks()) == 0 {
		change.SetStatus(state.DoneStatus)
	}
	st.EnsureBefore(0)

	return AsyncResponse(nil, change.ID())
}
