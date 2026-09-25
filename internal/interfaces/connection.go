// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017 Canonical Ltd
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

	"github.com/canonical/workshop/internal/interfaces/utils"
	"github.com/canonical/workshop/internal/metautil"
	"github.com/canonical/workshop/internal/sdk"
)

// Connection represents a connection between a particular plug and slot.
type Connection struct {
	Plug *ConnectedPlug
	Slot *ConnectedSlot
}

// ConnectedPlug represents a plug that is connected to a slot.
type ConnectedPlug struct {
	plugInfo     *sdk.PlugInfo
	staticAttrs  map[string]any
	dynamicAttrs map[string]any
}

func (p *ConnectedPlug) CheckBound() (*ConnRef, bool) {
	attr, _ := p.Lookup("bind")
	if attr == nil {
		return nil, false
	}
	id, ok := attr.(string)
	if !ok {
		return nil, false
	}

	cref, err := ParseConnRef(id)
	if err != nil {
		ok = false
	}
	return cref, ok
}

// ConnectedSlot represents a slot that is connected to a plug.
type ConnectedSlot struct {
	slotInfo     *sdk.SlotInfo
	staticAttrs  map[string]any
	dynamicAttrs map[string]any
}

// Attrer is an interface with Attr getter method common
// to ConnectedSlot, ConnectedPlug, PlugInfo and SlotInfo types.
type Attrer interface {
	// Attr returns attribute value for given path, or an error. Dotted paths are supported.
	Attr(path string, value any) error
	// Lookup returns attribute value for given path, or false. Dotted paths are supported.
	Lookup(path string) (value any, ok bool)
}

// NewConnectedSlot creates an object representing a connected slot.
func NewConnectedSlot(slot *sdk.SlotInfo, staticAttrs, dynamicAttrs map[string]any) *ConnectedSlot {
	var static map[string]any
	if staticAttrs != nil {
		static = staticAttrs
	} else {
		static = slot.Attrs
	}
	return &ConnectedSlot{
		slotInfo:     slot,
		staticAttrs:  utils.CopyAttributes(static),
		dynamicAttrs: utils.NormalizeInterfaceAttributes(dynamicAttrs).(map[string]any),
	}
}

// NewConnectedPlug creates an object representing a connected plug.
func NewConnectedPlug(plug *sdk.PlugInfo, staticAttrs, dynamicAttrs map[string]any) *ConnectedPlug {
	var static map[string]any
	if staticAttrs != nil {
		static = staticAttrs
	} else {
		static = plug.Attrs
	}
	return &ConnectedPlug{
		plugInfo:     plug,
		staticAttrs:  utils.CopyAttributes(static),
		dynamicAttrs: utils.NormalizeInterfaceAttributes(dynamicAttrs).(map[string]any),
	}
}

// Interface returns the name of the interface for this plug.
func (plug *ConnectedPlug) Interface() string {
	return plug.plugInfo.Interface
}

// Name returns the name of this plug.
func (plug *ConnectedPlug) Name() string {
	return plug.plugInfo.Name
}

// sdk returns the sdk Ref of this plug.
func (plug *ConnectedPlug) Sdk() sdk.Ref {
	return plug.plugInfo.Sdk
}

// StaticAttr returns a static attribute with the given key, or error if attribute doesn't exist.
func (plug *ConnectedPlug) StaticAttr(key string, val any) error {
	return plug.getAttribute(nil, key, val)
}

// StaticAttrs returns all static attributes.
func (plug *ConnectedPlug) StaticAttrs() map[string]any {
	return utils.CopyAttributes(plug.staticAttrs)
}

// DynamicAttrs returns all dynamic attributes.
func (plug *ConnectedPlug) DynamicAttrs() map[string]any {
	return utils.CopyAttributes(plug.dynamicAttrs)
}

// Attr returns a dynamic attribute with the given name. It falls back to returning static
// attribute if dynamic one doesn't exist. Error is returned if neither dynamic nor static
// attribute exist.
func (plug *ConnectedPlug) Attr(key string, val any) error {
	return plug.getAttribute(plug.dynamicAttrs, key, val)
}

func (plug *ConnectedPlug) getAttribute(dynamicAttrs map[string]any, key string, val any) error {
	v, ok := metautil.LookupAttr(plug.staticAttrs, dynamicAttrs, key)
	if !ok {
		ref := plug.Ref()
		return &sdk.AttributeNotFoundError{Attribute: key, Plug: &ref}
	}

	if err := metautil.SetValueFromAttribute(v, val); err != nil {
		return fmt.Errorf("invalid attribute %q for plug %q: %w", key, plug.Ref().ShortRef(), err)
	}
	return nil
}

func (plug *ConnectedPlug) Lookup(path string) (any, bool) {
	return metautil.LookupAttr(plug.staticAttrs, plug.dynamicAttrs, path)
}

// SetAttr sets the given dynamic attribute. Error is returned if the key is already used by a static attribute.
func (plug *ConnectedPlug) SetAttr(key string, value any) error {
	if _, ok := plug.staticAttrs[key]; ok {
		return fmt.Errorf("cannot change attribute %q as it was statically specified in the %q SDK details", key, plug.plugInfo.Sdk.Sdk)
	}
	if plug.dynamicAttrs == nil {
		plug.dynamicAttrs = make(map[string]any)
	}
	plug.dynamicAttrs[key] = utils.NormalizeInterfaceAttributes(value)
	return nil
}

// Ref returns the PlugRef for this plug.
func (plug *ConnectedPlug) Ref() sdk.PlugRef {
	return plug.plugInfo.Ref()
}

// Interface returns the name of the interface for this slot.
func (slot *ConnectedSlot) Interface() string {
	return slot.slotInfo.Interface
}

// Name returns the name of this slot.
func (slot *ConnectedSlot) Name() string {
	return slot.slotInfo.Name
}

// sdk returns the sdk Ref of this slot.
func (slot *ConnectedSlot) Sdk() sdk.Ref {
	return slot.slotInfo.Sdk
}

// StaticAttr returns a static attribute with the given key, or error if attribute doesn't exist.
func (slot *ConnectedSlot) StaticAttr(key string, val any) error {
	return slot.getAttribute(nil, key, val)
}

// StaticAttrs returns all static attributes.
func (slot *ConnectedSlot) StaticAttrs() map[string]any {
	return utils.CopyAttributes(slot.staticAttrs)
}

// DynamicAttrs returns all dynamic attributes.
func (slot *ConnectedSlot) DynamicAttrs() map[string]any {
	return utils.CopyAttributes(slot.dynamicAttrs)
}

// Attr returns a dynamic attribute with the given name. It falls back to returning static
// attribute if dynamic one doesn't exist. Error is returned if neither dynamic nor static
// attribute exist.
func (slot *ConnectedSlot) Attr(key string, val any) error {
	return slot.getAttribute(slot.dynamicAttrs, key, val)
}

func (slot *ConnectedSlot) getAttribute(dynamicAttrs map[string]any, key string, val any) error {
	v, ok := metautil.LookupAttr(slot.staticAttrs, dynamicAttrs, key)
	if !ok {
		ref := slot.Ref()
		return &sdk.AttributeNotFoundError{Attribute: key, Slot: &ref}
	}

	if err := metautil.SetValueFromAttribute(v, val); err != nil {
		return fmt.Errorf("invalid attribute %q for slot %q: %w", key, slot.Ref().ShortRef(), err)
	}
	return nil
}

func (slot *ConnectedSlot) Lookup(path string) (any, bool) {
	return metautil.LookupAttr(slot.staticAttrs, slot.dynamicAttrs, path)
}

// SetAttr sets the given dynamic attribute. Error is returned if the key is already used by a static attribute.
func (slot *ConnectedSlot) SetAttr(key string, value any) error {
	if _, ok := slot.staticAttrs[key]; ok {
		return fmt.Errorf("cannot change attribute %q as it was statically specified in the %q SDK details", key, slot.slotInfo.Sdk.Sdk)
	}
	if slot.dynamicAttrs == nil {
		slot.dynamicAttrs = make(map[string]any)
	}
	slot.dynamicAttrs[key] = utils.NormalizeInterfaceAttributes(value)
	return nil
}

// Ref returns the SlotRef for this slot.
func (slot *ConnectedSlot) Ref() sdk.SlotRef {
	return slot.slotInfo.Ref()
}

// Interface returns the name of the interface for this connection.
func (conn *Connection) Interface() string {
	return conn.Plug.plugInfo.Interface
}
