// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
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

package interfaces

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/exp/maps"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// Repository stores all known plugs and slots and ifaces.
type Repository struct {
	// Protects the internals from concurrent access.
	m      sync.Mutex
	ifaces map[string]Interface
	// Indexed by [project-id-workshop-sdk][plugName]
	plugs map[string]map[string]*sdk.PlugInfo
	// Indexed by [project-id-workshop-sdk][slotName]
	slots map[string]map[string]*sdk.SlotInfo
	// given a slot and a plug, are they connected?
	slotPlugs map[*sdk.SlotInfo]map[*sdk.PlugInfo]*Connection
	// given a plug and a slot, are they connected?
	plugSlots map[*sdk.PlugInfo]map[*sdk.SlotInfo]*Connection
	backends  []SecurityBackend
}

// NewRepository creates an empty plug repository.
func NewRepository() *Repository {
	repo := &Repository{
		ifaces:    make(map[string]Interface),
		plugs:     make(map[string]map[string]*sdk.PlugInfo),
		slots:     make(map[string]map[string]*sdk.SlotInfo),
		slotPlugs: make(map[*sdk.SlotInfo]map[*sdk.PlugInfo]*Connection),
		plugSlots: make(map[*sdk.PlugInfo]map[*sdk.SlotInfo]*Connection),
	}

	return repo
}

func plugOrSlotKey(projectId, workshop, sdkName string) string {
	return strings.Join([]string{projectId, workshop, sdkName}, "-")
}

// Interface returns an interface with a given name.
func (r *Repository) Interface(interfaceName string) Interface {
	r.m.Lock()
	defer r.m.Unlock()

	return r.ifaces[interfaceName]
}

// AddInterface adds the provided interface to the repository.
func (r *Repository) AddInterface(i Interface) error {
	r.m.Lock()
	defer r.m.Unlock()

	interfaceName := i.Name()
	if err := sdk.ValidateInterfaceName(interfaceName); err != nil {
		return err
	}
	if _, ok := r.ifaces[interfaceName]; ok {
		return fmt.Errorf("cannot add interface: %q, interface name is in use", interfaceName)
	}
	r.ifaces[interfaceName] = i

	return nil
}

// AllInterfaces returns all the interfaces added to the repository, ordered by name.
func (r *Repository) AllInterfaces() []Interface {
	r.m.Lock()
	defer r.m.Unlock()

	ifaces := make([]Interface, 0, len(r.ifaces))
	for _, iface := range r.ifaces {
		ifaces = append(ifaces, iface)
	}
	sort.Sort(byInterfaceName(ifaces))
	return ifaces
}

// InfoOptions describes options for Info.
//
// Names: return just this subset if non-empty.
// Doc: return documentation.
// Plugs: return information about plugs.
// Slots: return information about slots.
// Connected: only consider interfaces with at least one connection.
type InfoOptions struct {
	Names     []string
	Doc       bool
	Plugs     bool
	Slots     bool
	Connected bool
}

func (r *Repository) interfaceInfo(iface Interface, opts *InfoOptions) *Info {
	// NOTE: InfoOptions.Connected is handled by Info
	si := StaticInfoOf(iface)
	ifaceName := iface.Name()
	ii := &Info{
		Name:    ifaceName,
		Summary: si.Summary,
	}
	if opts != nil && opts.Doc {
		// Collect documentation URL
		ii.DocURL = si.DocURL
	}
	if opts != nil && opts.Plugs {
		// Collect all plugs of this interface type.
		for _, sdkName := range sortedSdkNamesWithPlugs(r.plugs) {
			for _, plugName := range sortedPlugNames(r.plugs[sdkName]) {
				plugInfo := r.plugs[sdkName][plugName]
				if plugInfo.Interface == ifaceName {
					ii.Plugs = append(ii.Plugs, plugInfo)
				}
			}
		}
	}
	if opts != nil && opts.Slots {
		// Collect all slots of this interface type.
		for _, sdkName := range sortedSdkNamesWithSlots(r.slots) {
			for _, slotName := range sortedSlotNames(r.slots[sdkName]) {
				slotInfo := r.slots[sdkName][slotName]
				if slotInfo.Interface == ifaceName {
					ii.Slots = append(ii.Slots, slotInfo)
				}
			}
		}
	}
	return ii
}

// Info returns information about interfaces in the system.
//
// If names is empty then all interfaces are considered. Query options decide
// which data to return but can also skip interfaces without connections. See
// the documentation of InfoOptions for details.
func (r *Repository) Info(opts *InfoOptions) []*Info {
	r.m.Lock()
	defer r.m.Unlock()

	// If necessary compute the set of interfaces with any connections.
	var connected map[string]bool
	if opts != nil && opts.Connected {
		connected = make(map[string]bool)
		for _, plugMap := range r.slotPlugs {
			for plug, conn := range plugMap {
				if conn != nil {
					connected[plug.Interface] = true
				}
			}
		}
		for _, slotMap := range r.plugSlots {
			for slot, conn := range slotMap {
				if conn != nil {
					connected[slot.Interface] = true
				}
			}
		}
	}

	// If weren't asked about specific interfaces then query every interface.
	var names []string
	if opts == nil || len(opts.Names) == 0 {
		for _, iface := range r.ifaces {
			name := iface.Name()
			if connected == nil || connected[name] {
				// Optionally filter out interfaces without connections.
				names = append(names, name)
			}
		}
	} else {
		names = make([]string, len(opts.Names))
		copy(names, opts.Names)
	}
	sort.Strings(names)

	// Query each interface we are interested in.
	infos := make([]*Info, 0, len(names))
	for _, name := range names {
		if iface, ok := r.ifaces[name]; ok {
			if connected == nil || connected[name] {
				infos = append(infos, r.interfaceInfo(iface, opts))
			}
		}
	}
	return infos
}

// AddBackend adds the provided security backend to the repository.
func (r *Repository) AddBackend(backend SecurityBackend) error {
	r.m.Lock()
	defer r.m.Unlock()

	name := backend.Name()
	for _, other := range r.backends {
		if other.Name() == name {
			return fmt.Errorf("cannot add backend %q, security system name is in use", name)
		}
	}
	r.backends = append(r.backends, backend)
	return nil
}

// AllPlugs returns all plugs of the given interface.
// If interfaceName is the empty string, all plugs are returned.
func (r *Repository) AllPlugs(interfaceName string) []*sdk.PlugInfo {
	r.m.Lock()
	defer r.m.Unlock()

	var result []*sdk.PlugInfo
	for _, plugsForSdk := range r.plugs {
		for _, plug := range plugsForSdk {
			if interfaceName == "" || plug.Interface == interfaceName {
				result = append(result, plug)
			}
		}
	}
	sort.Sort(byPlugWorkshopSdkAndName(result))
	return result
}

// Plugs returns the plugs offered by the named sdk.
func (r *Repository) Plugs(projectId, workshop, sdkName string) []*sdk.PlugInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	var result []*sdk.PlugInfo
	for _, plug := range r.plugs[key] {
		result = append(result, plug)
	}
	sort.Sort(byPlugWorkshopSdkAndName(result))
	return result
}

// Plug returns the specified plug from the named sdk.
func (r *Repository) Plug(projectId, workshop, sdkName, plugName string) *sdk.PlugInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	return r.plugs[key][plugName]
}

// Connection returns the specified Connection object or an error.
func (r *Repository) Connection(connRef *ConnRef) (*Connection, error) {
	plugkey := plugOrSlotKey(connRef.PlugRef.ProjectId, connRef.PlugRef.Workshop, connRef.PlugRef.Sdk)
	slotkey := plugOrSlotKey(connRef.SlotRef.ProjectId, connRef.SlotRef.Workshop, connRef.SlotRef.Sdk)

	// Ensure that such plug exists
	plug := r.plugs[plugkey][connRef.PlugRef.Name]
	if plug == nil {
		sdkRef := fmt.Sprintf("%s/%s", connRef.PlugRef.Workshop, connRef.PlugRef.Sdk)
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no plug named %q",
				sdkRef, connRef.PlugRef.Name)}
	}
	// Ensure that such slot exists
	slot := r.slots[slotkey][connRef.SlotRef.Name]
	if slot == nil {
		sdkRef := fmt.Sprintf("%s/%s", connRef.SlotRef.Workshop, connRef.SlotRef.Sdk)
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no slot named %q",
				sdkRef, connRef.SlotRef.Name)}
	}
	// Ensure that slot and plug are connected
	conn, ok := r.slotPlugs[slot][plug]
	if !ok {
		return nil, &NotConnectedError{
			message: fmt.Sprintf("no connection from %s:%s to %s:%s",
				connRef.PlugRef.Sdk, connRef.PlugRef.Name,
				connRef.SlotRef.Sdk, connRef.SlotRef.Name)}
	}

	return conn, nil
}

// AddPlug adds a plug to the repository.
// Plug names must be valid sdk names, as defined by ValidateName.
// Plug name must be unique within a particular sdk.
func (r *Repository) AddPlug(plug *sdk.PlugInfo) error {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(plug.Sdk.ProjectId, plug.Sdk.Workshop, plug.Sdk.Name)

	// Reject plugs with invalid names
	if err := sdk.ValidatePlugName(plug.Name); err != nil {
		return err
	}
	i := r.ifaces[plug.Interface]
	if i == nil {
		return fmt.Errorf("cannot add plug, interface %q is not known", plug.Interface)
	}
	if _, ok := r.plugs[key][plug.Name]; ok {
		return fmt.Errorf("sdk %q has plugs conflicting on name %q", plug.Sdk.Name, plug.Name)
	}
	if _, ok := r.slots[key][plug.Name]; ok {
		return fmt.Errorf("sdk %q has plug and slot conflicting on name %q", plug.Sdk.Name, plug.Name)
	}
	if r.plugs[key] == nil {
		r.plugs[key] = make(map[string]*sdk.PlugInfo)
	}
	r.plugs[key][plug.Name] = plug
	return nil
}

// RemovePlug removes the named plug provided by a given sdk.
// The removed plug must exist and must not be used anywhere.
func (r *Repository) RemovePlug(projectId, workshop, sdkName, plugName string) error {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	// Ensure that such plug exists
	plug := r.plugs[key][plugName]
	if plug == nil {
		return fmt.Errorf("cannot remove plug %q from sdk %q, no such plug", plugName, sdkName)
	}
	// Ensure that the plug is not used by any slot
	if len(r.plugSlots[plug]) > 0 {
		return fmt.Errorf("cannot remove plug %q from sdk %q, it is still connected", plugName, sdkName)
	}
	delete(r.plugs[key], plugName)
	if len(r.plugs[key]) == 0 {
		delete(r.plugs, sdkName)
	}
	return nil
}

// AllSlots returns all slots of the given interface.
// If interfaceName is the empty string, all slots are returned.
func (r *Repository) AllSlots(interfaceName string) []*sdk.SlotInfo {
	r.m.Lock()
	defer r.m.Unlock()

	var result []*sdk.SlotInfo
	for _, slotsForSdk := range r.slots {
		for _, slot := range slotsForSdk {
			if interfaceName == "" || slot.Interface == interfaceName {
				result = append(result, slot)
			}
		}
	}
	sort.Sort(bySlotWorkshopSdkAndName(result))
	return result
}

// Slots returns the slots offered by the named sdk.
func (r *Repository) Slots(projectId, workshop, sdkName string) []*sdk.SlotInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	var result []*sdk.SlotInfo
	for _, slot := range r.slots[key] {
		result = append(result, slot)
	}
	sort.Sort(bySlotWorkshopSdkAndName(result))
	return result
}

// Slot returns the specified slot from the named sdk.
func (r *Repository) Slot(projectId, workshop, sdkName, slotName string) *sdk.SlotInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	return r.slots[key][slotName]
}

// AddSlot adds a new slot to the repository.
// Adding a slot with invalid name returns an error.
// Adding a slot that has the same name and sdk name as another slot returns an error.
func (r *Repository) AddSlot(slot *sdk.SlotInfo) error {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(slot.Sdk.ProjectId, slot.Sdk.Workshop, slot.Sdk.Name)

	// Reject slots with invalid names
	if err := sdk.ValidateSlotName(slot.Name); err != nil {
		return err
	}
	// TODO: ensure that apps are correct
	i := r.ifaces[slot.Interface]
	if i == nil {
		return fmt.Errorf("cannot add slot, interface %q is not known", slot.Interface)
	}
	if _, ok := r.slots[key][slot.Name]; ok {
		return fmt.Errorf("sdk %q has slots conflicting on name %q", slot.Sdk.Name, slot.Name)
	}
	if _, ok := r.plugs[key][slot.Name]; ok {
		return fmt.Errorf("sdk %q has plug and slot conflicting on name %q", slot.Sdk.Name, slot.Name)
	}
	if r.slots[key] == nil {
		r.slots[key] = make(map[string]*sdk.SlotInfo)
	}
	r.slots[key][slot.Name] = slot
	return nil
}

// RemoveSlot removes a named slot from the given sdk.
// Removing a slot that doesn't exist returns an error.
// Removing a slot that is connected to a plug returns an error.
func (r *Repository) RemoveSlot(projectId, workshop, sdkName, slotName string) error {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	// Ensure that such slot exists
	slot := r.slots[key][slotName]
	if slot == nil {
		return fmt.Errorf("cannot remove slot %q from sdk %q, no such slot", slotName, sdkName)
	}
	// Ensure that the slot is not using any plugs
	if len(r.slotPlugs[slot]) > 0 {
		return fmt.Errorf("cannot remove slot %q from sdk %q, it is still connected", slotName, sdkName)
	}
	delete(r.slots[key], slotName)
	if len(r.slots[key]) == 0 {
		delete(r.slots, sdkName)
	}
	return nil
}

// slotValidator can be implemented by Interfaces that need to validate the slot before the security is lifted.
type slotValidator interface {
	BeforeConnectSlot(slot *ConnectedSlot) error
}

// plugValidator can be implemented by Interfaces that need to validate the plug before the security is lifted.
type plugValidator interface {
	BeforeConnectPlug(plug *ConnectedPlug) error
}

type PolicyFunc func(*ConnectedPlug, *ConnectedSlot) (bool, error)

// Connect establishes a connection between a plug and a slot.
// The plug and the slot must have the same interface.
// When connections are reloaded policyCheck is null (we don't check policy again).
func (r *Repository) Connect(ref *ConnRef, plugStaticAttrs, plugDynamicAttrs, slotStaticAttrs, slotDynamicAttrs map[string]interface{}, policyCheck PolicyFunc) (*Connection, error) {
	r.m.Lock()
	defer r.m.Unlock()

	plugSdkName := ref.PlugRef.Sdk
	plugName := ref.PlugRef.Name
	slotSdkName := ref.SlotRef.Sdk
	slotName := ref.SlotRef.Name

	plugKey := plugOrSlotKey(ref.PlugRef.ProjectId, ref.PlugRef.Workshop, plugSdkName)
	slotKey := plugOrSlotKey(ref.SlotRef.ProjectId, ref.SlotRef.Workshop, slotSdkName)

	// Ensure that such plug exists
	plug := r.plugs[plugKey][plugName]
	if plug == nil {
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("cannot connect plug %q from sdk %q: no such plug",
				plugName, plugSdkName)}
	}
	// Ensure that such slot exists
	slot := r.slots[slotKey][slotName]
	if slot == nil {
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("cannot connect slot %q from sdk %q: no such slot",
				slotName, slotSdkName)}
	}
	// Ensure that plug and slot are compatible
	if slot.Interface != plug.Interface {
		return nil, fmt.Errorf(`cannot connect plug "%s:%s" (interface %q) to "%s:%s" (interface %q)`,
			plugSdkName, plugName, plug.Interface, slotSdkName, slotName, slot.Interface)
	}

	iface, ok := r.ifaces[plug.Interface]
	if !ok {
		return nil, fmt.Errorf("internal error: unknown interface %q", plug.Interface)
	}

	cplug := NewConnectedPlug(plug, plugStaticAttrs, plugDynamicAttrs)
	cslot := NewConnectedSlot(slot, slotStaticAttrs, slotDynamicAttrs)

	// policyCheck is null when reloading connections
	if policyCheck != nil {
		if i, ok := iface.(plugValidator); ok {
			if err := i.BeforeConnectPlug(cplug); err != nil {
				return nil, fmt.Errorf("cannot connect plug %q of sdk %q: %s", plug.Name, plug.Sdk.Name, err)
			}
		}
		if i, ok := iface.(slotValidator); ok {
			if err := i.BeforeConnectSlot(cslot); err != nil {
				return nil, fmt.Errorf("cannot connect slot %q of sdk %q: %s", slot.Name, slot.Sdk.Name, err)
			}
		}

		// autoconnect policy checker returns false to indicate disallowed auto-connection, but it's not an error.
		ok, err := policyCheck(cplug, cslot)
		if !ok || err != nil {
			return nil, err
		}
	}

	// Connect the plug
	if r.slotPlugs[slot] == nil {
		r.slotPlugs[slot] = make(map[*sdk.PlugInfo]*Connection)
	}
	if r.plugSlots[plug] == nil {
		r.plugSlots[plug] = make(map[*sdk.SlotInfo]*Connection)
	}

	conn := &Connection{Plug: cplug, Slot: cslot}
	r.slotPlugs[slot][plug] = conn
	r.plugSlots[plug][slot] = conn
	return conn, nil
}

// NotConnectedError is returned by Disconnect() if the requested connection does
// not exist.
type NotConnectedError struct {
	message string
}

func (e *NotConnectedError) Error() string {
	return e.message
}

// NoPlugOrSlotError is returned by Disconnect() if either the plug or slot does
// no exist.
type NoPlugOrSlotError struct {
	message string
}

func (e *NoPlugOrSlotError) Error() string {
	return e.message
}

// Disconnect disconnects the named plug from the slot of the given sdk.
//
// Disconnect() finds a specific slot and a specific plug and disconnects that
// plug from that slot. It is an error if plug or slot cannot be found or if
// the connect does not exist.
func (r *Repository) Disconnect(plugProjectId, plugWorkshop, plugSdkName, plugName, slotProjectId, slotWorkshop, slotSdkName, slotName string) error {
	r.m.Lock()
	defer r.m.Unlock()

	// Validity check
	if plugSdkName == "" {
		return fmt.Errorf("cannot disconnect, plug sdk name is empty")
	}
	if plugName == "" {
		return fmt.Errorf("cannot disconnect, plug name is empty")
	}
	if slotSdkName == "" {
		return fmt.Errorf("cannot disconnect, slot sdk name is empty")
	}
	if slotName == "" {
		return fmt.Errorf("cannot disconnect, slot name is empty")
	}

	plugKey := plugOrSlotKey(plugProjectId, plugWorkshop, plugSdkName)
	slotKey := plugOrSlotKey(slotProjectId, slotWorkshop, slotSdkName)

	// Ensure that such plug exists
	plug := r.plugs[plugKey][plugName]
	if plug == nil {
		sdkRef := fmt.Sprintf("%s/%s", plugWorkshop, plugSdkName)
		return &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no plug named %q",
				sdkRef, plugName),
		}
	}
	// Ensure that such slot exists
	slot := r.slots[slotKey][slotName]
	if slot == nil {
		sdkRef := fmt.Sprintf("%s/%s", slotWorkshop, slotSdkName)
		return &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no slot named %q",
				sdkRef, slotName),
		}
	}
	// Ensure that slot and plug are connected
	if r.slotPlugs[slot][plug] == nil {
		return &NotConnectedError{
			message: fmt.Sprintf("cannot disconnect %s:%s from %s:%s, it is not connected",
				plugSdkName, plugName, slotSdkName, slotName),
		}
	}
	r.disconnect(plug, slot)
	return nil
}

// DisconnectAll disconnects all provided connection references.
func (r *Repository) DisconnectAll(conns []*ConnRef) {
	r.m.Lock()
	defer r.m.Unlock()

	for _, conn := range conns {
		plugkey := plugOrSlotKey(conn.PlugRef.ProjectId, conn.PlugRef.Workshop, conn.PlugRef.Sdk)
		slotkey := plugOrSlotKey(conn.SlotRef.ProjectId, conn.SlotRef.Workshop, conn.SlotRef.Sdk)

		plug := r.plugs[plugkey][conn.PlugRef.Name]
		slot := r.slots[slotkey][conn.SlotRef.Name]
		if plug != nil && slot != nil {
			r.disconnect(plug, slot)
		}
	}
}

// disconnect disconnects a plug from a slot.
func (r *Repository) disconnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) {
	delete(r.slotPlugs[slot], plug)
	if len(r.slotPlugs[slot]) == 0 {
		delete(r.slotPlugs, slot)
	}
	delete(r.plugSlots[plug], slot)
	if len(r.plugSlots[plug]) == 0 {
		delete(r.plugSlots, plug)
	}
}

// Connected returns references for all connections that are currently
// established with the provided plug or slot.
func (r *Repository) Connected(projectId, workshop, sdkName, plugOrSlotName string) ([]*ConnRef, error) {
	r.m.Lock()
	defer r.m.Unlock()

	return r.connected(projectId, workshop, sdkName, plugOrSlotName)
}

func (r *Repository) connected(projectId, workshop, sdk, plugOrSlotName string) ([]*ConnRef, error) {
	if workshop == "" {
		return nil, fmt.Errorf("internal error: cannot obtain workshop name while computing connections")
	}

	if sdk == "" {
		return nil, fmt.Errorf("internal error: cannot obtain sdk name while computing connections")
	}

	key := plugOrSlotKey(projectId, workshop, sdk)

	var conns []*ConnRef
	if plugOrSlotName == "" {
		return nil, fmt.Errorf("plug or slot name is empty")
	}
	// Check if plugOrSlotName actually maps to anything
	if r.plugs[key][plugOrSlotName] == nil && r.slots[key][plugOrSlotName] == nil {
		sdkRef := fmt.Sprintf("%s/%s", workshop, sdk)
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no plug or slot named %q",
				sdkRef, plugOrSlotName)}
	}
	// Collect all the relevant connections

	if plug, ok := r.plugs[key][plugOrSlotName]; ok {
		for slotInfo := range r.plugSlots[plug] {
			connRef := NewConnRef(plug, slotInfo)
			conns = append(conns, connRef)
		}
	}

	if slot, ok := r.slots[key][plugOrSlotName]; ok {
		for plugInfo := range r.slotPlugs[slot] {
			connRef := NewConnRef(plugInfo, slot)
			conns = append(conns, connRef)
		}
	}

	return conns, nil
}

func (r *Repository) Connections(projectId, workshop, sdk string) ([]*ConnRef, error) {
	r.m.Lock()
	defer r.m.Unlock()
	if workshop == "" {
		return nil, fmt.Errorf("internal error: cannot obtain workshop name while computing connections")
	}

	if sdk == "" {
		return nil, fmt.Errorf("internal error: cannot obtain sdk name while computing connections")
	}

	key := plugOrSlotKey(projectId, workshop, sdk)

	var conns []*ConnRef
	for _, plugInfo := range r.plugs[key] {
		for slotInfo := range r.plugSlots[plugInfo] {
			connRef := NewConnRef(plugInfo, slotInfo)
			conns = append(conns, connRef)
		}
	}
	for _, slotInfo := range r.slots[key] {
		for plugInfo := range r.slotPlugs[slotInfo] {
			// self-connection, ignore here as we got it already in the plugs loop above
			if plugInfo.Sdk == slotInfo.Sdk {
				continue
			}
			connRef := NewConnRef(plugInfo, slotInfo)
			conns = append(conns, connRef)
		}
	}

	return conns, nil
}

// Backends returns all the security backends.
// The order is the same as the order in which they were inserted.
func (r *Repository) Backends() []SecurityBackend {
	r.m.Lock()
	defer r.m.Unlock()

	result := make([]SecurityBackend, len(r.backends))
	copy(result, r.backends)
	return result
}

// Interfaces returns object holding a lists of all the plugs and slots and their connections.
func (r *Repository) Interfaces() *Interfaces {
	r.m.Lock()
	defer r.m.Unlock()

	ifaces := &Interfaces{}

	// Copy and flatten plugs and slots
	for _, plugs := range r.plugs {
		for _, plugInfo := range plugs {
			ifaces.Plugs = append(ifaces.Plugs, plugInfo)
		}
	}
	for _, slots := range r.slots {
		for _, slotInfo := range slots {
			ifaces.Slots = append(ifaces.Slots, slotInfo)
		}
	}

	for plug, slots := range r.plugSlots {
		for slot := range slots {
			ifaces.Connections = append(ifaces.Connections, NewConnRef(plug, slot))
		}
	}

	sort.Sort(byPlugWorkshopSdkAndName(ifaces.Plugs))
	sort.Sort(bySlotWorkshopSdkAndName(ifaces.Slots))
	sort.Sort(byConnRef(ifaces.Connections))
	return ifaces
}

// SdkSpecification returns the specification of a given sdk in a given security system.
func (r *Repository) SdkSpecification(ctx context.Context, securitySystem SecuritySystem, sdkInfo sdk.Ref) (Specification, error) {
	r.m.Lock()
	defer r.m.Unlock()

	var backend SecurityBackend
	for _, b := range r.backends {
		if b.Name() == securitySystem {
			backend = b
			break
		}
	}
	if backend == nil {
		return nil, fmt.Errorf("cannot handle interfaces of %q workshop, security system %q is not known", sdkInfo.Workshop, securitySystem)
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("internal error: context key %s not found", workshop.ContextUser)
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	spec := backend.NewSpecification(user, projectId, sdkInfo.Sdk)

	key := plugOrSlotKey(sdkInfo.ProjectId, sdkInfo.Workshop, sdkInfo.Sdk)

	// XXX: If either of the AddConnected{Plug,Slot} methods for a connection
	// fail resiliently as-in they can never succeed (such as the case where a
	// bit of policy generated is unable to be used on this system), we may be
	// stuck never able to modify the policy without restarting daemon. This is
	// because the (broken) connection is still left inside the in-memory
	// repository so the next time we try to do any modification to this sdk's
	// plugs or slots, we will try to add that connection again and fail. It is
	// resolved by restarting daemon since we just store the repository in-memory
	// and don't persist new connections until after these bits are successful.
	// We may want to consider removing connections which fail when we try to
	// generate/add policy for them. This may just be a transitory failure
	// however, so maybe the right thing to do is try again, but we don't know
	// if the error is transient so we also don't want to infinitely loop trying
	// to add a connected plug that will never work.

	// slot side
	for _, slotInfo := range r.slots[key] {
		iface := r.ifaces[slotInfo.Interface]
		if err := spec.AddPermanentSlot(iface, slotInfo); err != nil {
			return nil, err
		}
		for _, conn := range r.slotPlugs[slotInfo] {
			if _, ok := conn.CheckBound(); ok {
				continue
			}
			if err := spec.AddConnectedSlot(iface, conn.Plug, conn.Slot); err != nil {
				return nil, err
			}
		}
	}
	// plug side
	for _, plugInfo := range r.plugs[key] {
		iface := r.ifaces[plugInfo.Interface]
		if err := spec.AddPermanentPlug(iface, plugInfo); err != nil {
			return nil, err
		}
		for _, conn := range r.plugSlots[plugInfo] {
			if _, ok := conn.CheckBound(); ok {
				continue
			}
			if err := spec.AddConnectedPlug(iface, conn.Plug, conn.Slot); err != nil {
				return nil, err
			}
		}
	}
	return spec, nil
}

// AddSdk adds plugs and slots declared by the given sdk to the repository.
//
// AddSdk doesn't change existing plugs/slots. The caller is responsible for
// ensuring that the sdk is not present in the repository in any way prior to
// calling this function. If this constraint is violated then no changes are
// made and an error is returned.
//
// Each added plug/slot is validated according to the corresponding interface.
// Unknown interfaces and plugs/slots that don't validate are not added.
// Information about those failures are returned to the caller.
func (r *Repository) AddSdk(sdkInfo *sdk.Info) error {
	err := sdk.Validate(sdkInfo)
	if err != nil {
		return err
	}

	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(sdkInfo.ProjectId, sdkInfo.Workshop, sdkInfo.Name)

	if r.plugs[key] != nil || r.slots[key] != nil {
		return fmt.Errorf("cannot register interfaces for %q SDK more than once", key)
	}

	for plugName, plugInfo := range sdkInfo.Plugs {
		if _, ok := r.ifaces[plugInfo.Interface]; !ok {
			continue
		}
		if r.plugs[key] == nil {
			r.plugs[key] = make(map[string]*sdk.PlugInfo)
		}
		r.plugs[key][plugName] = plugInfo
	}

	for slotName, slotInfo := range sdkInfo.Slots {
		if _, ok := r.ifaces[slotInfo.Interface]; !ok {
			continue
		}
		if r.slots[key] == nil {
			r.slots[key] = make(map[string]*sdk.SlotInfo)
		}
		r.slots[key][slotName] = slotInfo
	}
	return nil
}

// RemoveSdk removes all the plugs and slots associated with a given sdk.
//
// This function can be used to implement sdk removal or, when used along with
// AddSdk, sdk upgrade.
//
// RemoveSdk does not remove connections. The caller is responsible for
// ensuring that connections are broken before calling this method. If this
// constraint is violated then no changes are made and an error is returned.
func (r *Repository) RemoveSdk(projectId, workshop, sdkName string) error {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	for plugName, plug := range r.plugs[key] {
		if len(r.plugSlots[plug]) > 0 {
			return fmt.Errorf("cannot remove connected plug %s.%s", sdkName, plugName)
		}
	}
	for slotName, slot := range r.slots[key] {
		if len(r.slotPlugs[slot]) > 0 {
			return fmt.Errorf("cannot remove connected slot %s.%s", sdkName, slotName)
		}
	}

	for _, plug := range r.plugs[key] {
		delete(r.plugSlots, plug)
	}
	delete(r.plugs, key)
	for _, slot := range r.slots[key] {
		delete(r.slotPlugs, slot)
	}
	delete(r.slots, key)

	return nil
}

// DisconnectSdk disconnects all the connections to and from a given sdk.
//
// The return value is a list of names that were affected.
func (r *Repository) DisconnectSdk(projectId, workshop, sdkName string) ([]*sdk.Info, error) {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, sdkName)

	seen := make(map[*sdk.Info]bool)

	for _, plug := range r.plugs[key] {
		for slot := range r.plugSlots[plug] {
			r.disconnect(plug, slot)
			seen[plug.Sdk] = true
			seen[slot.Sdk] = true
		}
	}

	for _, slot := range r.slots[key] {
		for plug := range r.slotPlugs[slot] {
			r.disconnect(plug, slot)
			seen[plug.Sdk] = true
			seen[slot.Sdk] = true
		}
	}

	return maps.Keys(seen), nil
}

// SideArity conveys the arity constraints for an allowed auto-connection.
// ATM only slots-per-plug might have an interesting non-default
// value.
// See: https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
type SideArity interface {
	SlotsPerPlugAny() bool
	// TODO: consider PlugsPerSlot*
}

// AutoConnectCandidateSlots finds and returns viable auto-connection candidates
// for a given plug.
func (r *Repository) AutoConnectCandidateSlots(projectId, workshop, plugSdkName, plugName string, policyCheck func(*ConnectedPlug, *ConnectedSlot) (bool, error)) []*sdk.SlotInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, plugSdkName)

	plugInfo := r.plugs[key][plugName]
	if plugInfo == nil {
		return nil
	}

	var candidates []*sdk.SlotInfo
	for _, slotsForSdk := range r.slots {
		for _, slotInfo := range slotsForSdk {
			if slotInfo.Interface != plugInfo.Interface {
				continue
			}
			iface := slotInfo.Interface

			ok, err := policyCheck(NewConnectedPlug(plugInfo, nil, nil), NewConnectedSlot(slotInfo, nil, nil))
			if !ok || err != nil {
				continue
			}

			if r.ifaces[iface].AutoConnect(plugInfo, slotInfo) {
				candidates = append(candidates, slotInfo)
			}
		}
	}
	return candidates
}

// AutoConnectCandidatePlugs finds and returns viable auto-connection candidates
// for a given slot.
func (r *Repository) AutoConnectCandidatePlugs(projectId, workshop, slotSdkName, slotName string, policyCheck func(*ConnectedPlug, *ConnectedSlot) (bool, error)) []*sdk.PlugInfo {
	r.m.Lock()
	defer r.m.Unlock()

	key := plugOrSlotKey(projectId, workshop, slotSdkName)

	slotInfo := r.slots[key][slotName]
	if slotInfo == nil {
		return nil
	}

	var candidates []*sdk.PlugInfo
	for _, plugsForSdk := range r.plugs {
		for _, plugInfo := range plugsForSdk {
			if slotInfo.Interface != plugInfo.Interface {
				continue
			}
			iface := slotInfo.Interface

			// declaration based checks disallow
			ok, err := policyCheck(NewConnectedPlug(plugInfo, nil, nil), NewConnectedSlot(slotInfo, nil, nil))
			if !ok || err != nil {
				continue
			}

			if r.ifaces[iface].AutoConnect(plugInfo, slotInfo) {
				candidates = append(candidates, plugInfo)
			}
		}
	}
	return candidates
}

// ResolveConnect resolves potentially missing plug or slot names and returns a
// fully populated connection reference.
func (r *Repository) ResolveConnect(plugProjectId, plugWorkshop, plugSdkName, plugName, slotProjectId, slotWorkshop, slotSdkName, slotName string) (*ConnRef, error) {
	r.m.Lock()
	defer r.m.Unlock()

	if plugProjectId == "" {
		return nil, fmt.Errorf("cannot resolve connection, plug project-id name is empty")
	}
	if plugWorkshop == "" {
		return nil, fmt.Errorf("cannot resolve connection, plug Workshop name is empty")
	}
	if plugSdkName == "" {
		return nil, fmt.Errorf("cannot resolve connection, plug SDK name is empty")
	}
	if plugName == "" {
		return nil, fmt.Errorf("cannot resolve connection, plug name is empty")
	}

	plugKey := plugOrSlotKey(plugProjectId, plugWorkshop, plugSdkName)

	// Ensure that such plug exists
	plug := r.plugs[plugKey][plugName]
	if plug == nil {
		sdkRef := fmt.Sprintf("%s/%s", plugWorkshop, plugSdkName)
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf(`SDK %q has no plug named %q`,
				sdkRef, plugName),
		}
	}

	if slotSdkName == "" {
		// Use the host SDK if the slot-side snap name is empty
		slotProjectId = plugProjectId
		slotWorkshop = plugWorkshop
		slotSdkName = sdk.System.String()
	}

	slotKey := plugOrSlotKey(slotProjectId, slotWorkshop, slotSdkName)

	if slotName == "" {
		// Find the unambiguous slot that satisfies plug requirements
		var candidates []string
		for candidateSlotName, candidateSlot := range r.slots[slotKey] {
			// TODO: use some smarter matching (e.g. against $attrs)
			if candidateSlot.Interface == plug.Interface {
				candidates = append(candidates, candidateSlotName)
			}
		}
		switch len(candidates) {
		case 0:
			sdkRef := fmt.Sprintf("%s/%s", slotWorkshop, slotSdkName)
			return nil, fmt.Errorf(`SDK %q has no %q interface slots`, sdkRef, plug.Interface)
		case 1:
			slotName = candidates[0]
		default:
			sort.Strings(candidates)
			sdkRef := fmt.Sprintf("%s/%s", slotWorkshop, slotSdkName)
			return nil, fmt.Errorf("SDK %q has multiple %q interface slots: %s", sdkRef, plug.Interface, strings.Join(candidates, ", "))
		}
	}

	// Ensure that such slot exists
	slot := r.slots[slotKey][slotName]
	if slot == nil {
		sdkRef := fmt.Sprintf("%s/%s", slotWorkshop, slotSdkName)
		return nil, &NoPlugOrSlotError{
			message: fmt.Sprintf("SDK %q has no slot named %q", sdkRef, slotName),
		}
	}
	// Ensure that plug and slot are compatible
	if slot.Interface != plug.Interface {
		return nil, fmt.Errorf("cannot connect %s/%s:%s (%q interface) to %s/%s:%s (%q interface)",
			plugWorkshop, plugSdkName, plugName, plug.Interface, slotWorkshop, slotSdkName, slotName, slot.Interface)
	}
	return NewConnRef(plug, slot), nil
}
