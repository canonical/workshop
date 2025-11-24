// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
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

// Package policy implements the declaration based policy checks for
// connecting or permitting installation of sdks based on their slots
// and plugs.
package policy

import (
	"fmt"

	"github.com/canonical/workshop/internal/asserts"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
)

// InstallCandidate represents a candidate SDK for installation.
type InstallCandidate struct {
	Sdk             *sdk.Info
	BaseDeclaration *asserts.BaseDeclaration
}

func (ic *InstallCandidate) checkSlotRule(slot *sdk.SlotInfo, rule *asserts.SlotRule) error {
	if checkSlotInstallationAltConstraints(slot, rule.DenyInstallation) == nil {
		return fmt.Errorf("installation denied by %q slot rule of interface %q", slot.Name, slot.Interface)
	}
	if checkSlotInstallationAltConstraints(slot, rule.AllowInstallation) != nil {
		return fmt.Errorf("installation not allowed by %q slot rule of interface %q", slot.Name, slot.Interface)
	}
	return nil
}

func (ic *InstallCandidate) checkPlugRule(plug *sdk.PlugInfo, rule *asserts.PlugRule) error {
	context := ""
	if checkPlugInstallationAltConstraints(plug, rule.DenyInstallation) == nil {
		return fmt.Errorf("installation denied by %q plug rule of interface %q%s", plug.Name, plug.Interface, context)
	}
	if checkPlugInstallationAltConstraints(plug, rule.AllowInstallation) != nil {
		return fmt.Errorf("installation not allowed by %q plug rule of interface %q%s", plug.Name, plug.Interface, context)
	}
	return nil
}

func (ic *InstallCandidate) checkSlot(slot *sdk.SlotInfo) error {
	iface := slot.Interface
	if rule := ic.BaseDeclaration.SlotRule(iface); rule != nil {
		return ic.checkSlotRule(slot, rule)
	}
	return nil
}

func (ic *InstallCandidate) checkPlug(plug *sdk.PlugInfo) error {
	iface := plug.Interface
	if rule := ic.BaseDeclaration.PlugRule(iface); rule != nil {
		return ic.checkPlugRule(plug, rule)
	}
	return nil
}

// Check checks whether the installation is allowed.
func (ic *InstallCandidate) Check() error {
	if ic.BaseDeclaration == nil {
		return fmt.Errorf("internal error: improperly initialized InstallCandidate")
	}

	for _, slot := range ic.Sdk.Slots {
		err := ic.checkSlot(slot)
		if err != nil {
			return err
		}
	}

	for _, plug := range ic.Sdk.Plugs {
		err := ic.checkPlug(plug)
		if err != nil {
			return err
		}
	}

	return nil
}

// ConnectCandidate represents a candidate connection.
type ConnectCandidate struct {
	Plug            *interfaces.ConnectedPlug
	Slot            *interfaces.ConnectedSlot
	BaseDeclaration *asserts.BaseDeclaration
}

func nestedGet(which string, attrs interfaces.Attrer, path string) (interface{}, error) {
	val, ok := attrs.Lookup(path)
	if !ok {
		return nil, fmt.Errorf("%s attribute %q not found", which, path)
	}
	return val, nil
}

func (connc *ConnectCandidate) PlugAttr(arg string) (interface{}, error) {
	return nestedGet("plug", connc.Plug, arg)
}

func (connc *ConnectCandidate) SlotAttr(arg string) (interface{}, error) {
	return nestedGet("slot", connc.Slot, arg)
}

func (connc *ConnectCandidate) checkPlugRule(kind string, rule *asserts.PlugRule) (interfaces.SideArity, error) {
	context := ""
	denyConst := rule.DenyConnection
	allowConst := rule.AllowConnection
	if kind == "auto-connection" {
		denyConst = rule.DenyAutoConnection
		allowConst = rule.AllowAutoConnection
	}
	if _, err := checkPlugConnectionAltConstraints(connc, denyConst); err == nil {
		return nil, fmt.Errorf("%s denied by plug rule of interface %q%s", kind, connc.Plug.Interface(), context)
	}

	allowedConstraints, err := checkPlugConnectionAltConstraints(connc, allowConst)
	if err != nil {
		return nil, fmt.Errorf("%s not allowed by plug rule of interface %q%s", kind, connc.Plug.Interface(), context)
	}
	return sideArity{allowedConstraints.SlotsPerPlug}, nil
}

func (connc *ConnectCandidate) checkSlotRule(kind string, rule *asserts.SlotRule) (interfaces.SideArity, error) {
	denyConst := rule.DenyConnection
	allowConst := rule.AllowConnection
	if kind == "auto-connection" {
		denyConst = rule.DenyAutoConnection
		allowConst = rule.AllowAutoConnection
	}
	if _, err := checkSlotConnectionAltConstraints(connc, denyConst); err == nil {
		return nil, fmt.Errorf("%s denied by slot rule of interface %q", kind, connc.Plug.Interface())
	}

	allowedConstraints, err := checkSlotConnectionAltConstraints(connc, allowConst)
	if err != nil {
		return nil, fmt.Errorf("%s not allowed by slot rule of interface %q", kind, connc.Plug.Interface())
	}
	return sideArity{allowedConstraints.SlotsPerPlug}, nil
}

func (connc *ConnectCandidate) check(kind string) (interfaces.SideArity, error) {
	baseDecl := connc.BaseDeclaration
	if baseDecl == nil {
		return nil, fmt.Errorf("internal error: improperly initialized ConnectCandidate")
	}

	iface := connc.Plug.Interface()

	if connc.Slot.Interface() != iface {
		return nil, fmt.Errorf("cannot connect mismatched plug interface %q to slot interface %q", iface, connc.Slot.Interface())
	}

	if rule := baseDecl.PlugRule(iface); rule != nil {
		return connc.checkPlugRule(kind, rule)
	}
	if rule := baseDecl.SlotRule(iface); rule != nil {
		return connc.checkSlotRule(kind, rule)
	}
	return nil, nil
}

// Check checks whether the connection is allowed.
func (connc *ConnectCandidate) Check() (interfaces.SideArity, error) {
	arity, err := connc.check("connection")
	return arity, err
}

// CheckAutoConnect checks whether the connection is allowed to auto-connect.
func (connc *ConnectCandidate) CheckAutoConnect() (interfaces.SideArity, error) {
	arity, err := connc.check("auto-connection")
	if err != nil {
		return nil, err
	}
	return arity, nil
}

// sideArity carries relevant arity constraints for successful
// allow-auto-connection rules. It implements policy.SideArity.
// ATM only slots-per-plug might have an interesting non-default
// value.
// See: https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
type sideArity struct {
	slotsPerPlug asserts.SideArityConstraint
}

func (a sideArity) SlotsPerPlugOne() bool {
	return a.slotsPerPlug.N == 1
}

func (a sideArity) SlotsPerPlugAny() bool {
	return a.slotsPerPlug.Any()
}

// CheckInterfaces checks whether plugs and slots of sdk are allowed for installation.
func CheckInterfaces(sdkInfo *sdk.Info) error {
	baseDecl := asserts.BuiltinBaseDeclaration()
	if baseDecl == nil {
		return fmt.Errorf("internal error: cannot find base declaration")
	}

	ic := InstallCandidate{
		Sdk:             sdkInfo,
		BaseDeclaration: baseDecl,
	}

	return ic.Check()
}
