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
	"fmt"
	"strings"

	"github.com/canonical/workshop/internal/sdk"
)

// BeforePreparePlug sanitizes a plug with a given interface.
func BeforePreparePlug(iface Interface, plugInfo *sdk.PlugInfo) error {
	if iface.Name() != plugInfo.Interface {
		return fmt.Errorf("cannot sanitize plug %q (interface %q) using interface %q",
			plugInfo.Ref().ShortRef(), plugInfo.Interface, iface.Name())
	}
	if iface, ok := iface.(PlugSanitizer); ok {
		return iface.BeforePreparePlug(plugInfo)
	}

	// Plugs which accept attributes should define BeforePreparePlug.
	for name := range plugInfo.Attrs {
		return fmt.Errorf("unknown attribute for %s interface plug: %q", iface.Name(), name)
	}
	return nil
}

func BeforeConnectPlug(iface Interface, plug *ConnectedPlug) error {
	if iface.Name() != plug.plugInfo.Interface {
		return fmt.Errorf("cannot sanitize connection for plug %q (interface %q) using interface %q",
			plug.plugInfo.Ref().ShortRef(), plug.plugInfo.Interface, iface.Name())
	}
	var err error
	if iface, ok := iface.(ConnPlugSanitizer); ok {
		err = iface.BeforeConnectPlug(plug)
	}
	return err
}

// ByName returns an Interface for the given interface name. Note that in order for
// this to work properly, the package "interfaces/builtin" must also eventually be
// imported to populate the full list of interfaces.
var ByName = func(name string) (iface Interface, err error) {
	panic("ByName is unset, import interfaces/builtin to initialize this")
}

// Sanitize slot with a given interface.
func BeforePrepareSlot(iface Interface, slotInfo *sdk.SlotInfo) error {
	if iface.Name() != slotInfo.Interface {
		return fmt.Errorf("cannot sanitize slot %q (interface %q) using interface %q",
			slotInfo.Ref(), slotInfo.Interface, iface.Name())
	}
	if iface, ok := iface.(SlotSanitizer); ok {
		return iface.BeforePrepareSlot(slotInfo)
	}
	// Slots which accept attributes should define BeforePrepareSlot.
	for name := range slotInfo.Attrs {
		return fmt.Errorf("unknown attribute for %s interface slot: %q", iface.Name(), name)
	}
	return nil
}

// Interfaces holds information about a list of plugs, slots and their connections.
type Interfaces struct {
	Plugs       []*sdk.PlugInfo
	Slots       []*sdk.SlotInfo
	Connections []*ConnRef
}

// Info holds information about a given interface and its instances.
type Info struct {
	Name    string
	Summary string
	DocURL  string
	Plugs   []*sdk.PlugInfo
	Slots   []*sdk.SlotInfo
}

// ConnRef holds information about plug and slot reference that form a particular connection.
type ConnRef struct {
	PlugRef sdk.PlugRef
	SlotRef sdk.SlotRef
}

// NewConnRef creates a connection reference for given plug and slot
func NewConnRef(plug *sdk.PlugInfo, slot *sdk.SlotInfo) *ConnRef {
	return &ConnRef{PlugRef: plug.Ref(), SlotRef: slot.Ref()}
}

// ID returns a string identifying a given connection.
func (conn *ConnRef) ID() string {
	return fmt.Sprintf("%s/%s/%s:%s %s/%s/%s:%s",
		conn.PlugRef.ProjectId, conn.PlugRef.Workshop, conn.PlugRef.Sdk, conn.PlugRef.Name,
		conn.SlotRef.ProjectId, conn.SlotRef.Workshop, conn.SlotRef.Sdk, conn.SlotRef.Name)
}

// SortsBefore returns true when connection should be sorted before the other
func (conn *ConnRef) SortsBefore(other *ConnRef) bool {
	if conn.PlugRef != other.PlugRef {
		return conn.PlugRef.SortsBefore(other.PlugRef)
	}
	return conn.SlotRef.SortsBefore(other.SlotRef)
}

// ParseConnRef parses an ID string
func ParseConnRef(id string) (*ConnRef, error) {
	var conn ConnRef
	parts := strings.SplitN(id, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed connection identifier: %q", id)
	}
	plugParts := strings.Split(parts[0], ":")
	slotParts := strings.Split(parts[1], ":")
	if len(plugParts) != 2 || len(slotParts) != 2 {
		return nil, fmt.Errorf("malformed connection identifier: %q", id)
	}

	// plug's project / workshop / sdk
	plugPwsParts := strings.Split(plugParts[0], "/")
	slotPwsParts := strings.Split(slotParts[0], "/")
	if len(plugPwsParts) != 3 || len(slotPwsParts) != 3 {
		return nil, fmt.Errorf("malformed connection identifier: %q", id)
	}

	conn.PlugRef.ProjectId = plugPwsParts[0]
	conn.PlugRef.Workshop = plugPwsParts[1]
	conn.PlugRef.Sdk = plugPwsParts[2]
	conn.PlugRef.Name = plugParts[1]

	conn.SlotRef.ProjectId = slotPwsParts[0]
	conn.SlotRef.Workshop = slotPwsParts[1]
	conn.SlotRef.Sdk = slotPwsParts[2]
	conn.SlotRef.Name = slotParts[1]
	return &conn, nil
}

// Interface describes a group of interchangeable capabilities with common features.
// Interfaces act as a contract between system builders, application developers
// and end users.
type Interface interface {
	// Unique and public name of this interface.
	Name() string

	// AutoConnect returns whether plug and slot should be
	// implicitly auto-connected assuming there will be an
	// unambiguous connection candidate and declaration-based checks
	// allow.
	AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool
}

// ConnPlugSanitizer can be implemented by Interfaces that have reasons to sanitize
// their plugs specifically before a connection is performed.
type ConnPlugSanitizer interface {
	BeforeConnectPlug(plug *ConnectedPlug) error
}

// PlugSanitizer can be implemented by Interfaces that have reasons to sanitize their plugs.
type PlugSanitizer interface {
	BeforePreparePlug(plug *sdk.PlugInfo) error
}

// SlotSanitizer can be implemented by Interfaces that have reasons to sanitize their slots.
type SlotSanitizer interface {
	BeforePrepareSlot(slot *sdk.SlotInfo) error
}

// StaticInfo describes various static-info of a given interface.
//
// The Summary must be a one-line string of length suitable for listing views.
// The DocURL can point to website (e.g. a forum thread) that goes into more
// depth and documents the interface in detail.
type StaticInfo struct {
	Summary string `json:"summary,omitempty"`
	DocURL  string `json:"doc-url,omitempty"`

	// AffectsPlugOnRefresh tells if refreshing of a sdk with a slot of this interface
	// is disruptive for the sdk on the plug side (when the interface is connected),
	// meaning that a refresh of the slot-side affects sdk(s) on the plug side
	AffectsPlugOnRefresh bool `json:"affects-plug-on-refresh,omitempty"`

	// BaseDeclarationPlugs defines an optional extension to the base-declaration assertion relevant for this interface.
	BaseDeclarationPlugs string
	// BaseDeclarationSlots defines an optional extension to the base-declaration assertion relevant for this interface.
	BaseDeclarationSlots string
}

// StaticInfoProvider can be implemented by Interfaces that supply static
// metadata about themselves. [StaticInfoOf] discovers the hook by type
// assertion; an interface that does not implement it has empty static
// info.
type StaticInfoProvider interface {
	// StaticInfo returns the static metadata describing the interface,
	// such as its summary and base-declaration policy extensions.
	StaticInfo() StaticInfo
}

// PermanentPlugServiceSnippets will return the set of snippets for the systemd
// service unit that should be generated for a sdk with the specified plug.
// The list returned is not unique, callers must de-duplicate themselves.
// The plug is provided because the snippet may depend on plug attributes for
// example. The plug is sanitized before the snippets are returned.
func PermanentPlugServiceSnippets(iface Interface, plug *sdk.PlugInfo) (snips []string, err error) {
	// sanitize the plug first
	err = BeforePreparePlug(iface, plug)
	if err != nil {
		return nil, err
	}

	type serviceSnippetPlugger interface {
		ServicePermanentPlug(plug *sdk.PlugInfo) []string
	}
	if iface, ok := iface.(serviceSnippetPlugger); ok {
		snips = iface.ServicePermanentPlug(plug)
	}
	return snips, nil
}

// StaticInfoOf returns the static-info of the given interface.
func StaticInfoOf(iface Interface) (si StaticInfo) {
	if iface, ok := iface.(StaticInfoProvider); ok {
		si = iface.StaticInfo()
	}
	return si
}

// Specification describes interactions between backends and interfaces.
type Specification interface {
	// AddPermanentSlot records side-effects of having a slot.
	AddPermanentSlot(iface Interface, slot *sdk.SlotInfo) error
	// AddPermanentPlug records side-effects of having a plug.
	AddPermanentPlug(iface Interface, plug *sdk.PlugInfo) error
	// AddConnectedSlot records side-effects of having a connected slot.
	AddConnectedSlot(iface Interface, plug *ConnectedPlug, slot *ConnectedSlot) error
	// AddConnectedPlug records side-effects of having a connected plug.
	AddConnectedPlug(iface Interface, plug *ConnectedPlug, slot *ConnectedSlot) error
}

// SecuritySystem is a name of a security system.
type SecuritySystem string

const (
	// SecurityLxdDevice creates LXD device configurations (mount, GPU, etc.)
	SecurityLxdDevice SecuritySystem = "lxd-device"
)
