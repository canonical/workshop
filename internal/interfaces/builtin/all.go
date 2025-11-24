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

package builtin

import (
	"fmt"
	"sort"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
)

func init() {
	sdk.SanitizePlugsSlots = SanitizePlugsSlots

	// setup the ByName function using allInterfaces
	interfaces.ByName = func(name string) (interfaces.Interface, error) {
		iface, ok := allInterfaces[name]
		if !ok {
			return nil, fmt.Errorf("interface %q not found", name)
		}
		return iface, nil
	}
}

var allInterfaces map[string]interfaces.Interface

func SanitizePlugsSlots(sdkInfo *sdk.Info) {
	var badPlugs []string
	var badSlots []string

	for plugName, plugInfo := range sdkInfo.Plugs {
		iface, ok := allInterfaces[plugInfo.Interface]
		if !ok {
			sdkInfo.BadInterfaces[plugName] = fmt.Sprintf("unknown interface %q", plugInfo.Interface)
			badPlugs = append(badPlugs, plugName)
			continue
		}
		// Reject plug with invalid name
		if err := sdk.ValidatePlugName(plugName); err != nil {
			sdkInfo.BadInterfaces[plugName] = err.Error()
			badPlugs = append(badPlugs, plugName)
			continue
		}
		if err := interfaces.BeforePreparePlug(iface, plugInfo); err != nil {
			sdkInfo.BadInterfaces[plugName] = err.Error()
			badPlugs = append(badPlugs, plugName)
			continue
		}
	}

	for slotName, slotInfo := range sdkInfo.Slots {
		iface, ok := allInterfaces[slotInfo.Interface]
		if !ok {
			sdkInfo.BadInterfaces[slotName] = fmt.Sprintf("unknown interface %q", slotInfo.Interface)
			badSlots = append(badSlots, slotName)
			continue
		}
		// Reject slot with invalid name
		if err := sdk.ValidateSlotName(slotName); err != nil {
			sdkInfo.BadInterfaces[slotName] = err.Error()
			badSlots = append(badSlots, slotName)
			continue
		}
		if err := interfaces.BeforePrepareSlot(iface, slotInfo); err != nil {
			sdkInfo.BadInterfaces[slotName] = err.Error()
			badSlots = append(badSlots, slotName)
			continue
		}
	}

	// remove any bad plugs and slots
	for _, plugName := range badPlugs {
		delete(sdkInfo.Plugs, plugName)
	}
	for _, slotName := range badSlots {
		delete(sdkInfo.Slots, slotName)
	}
}

// Interfaces returns all of the built-in interfaces.
func Interfaces() []interfaces.Interface {
	ifaces := make([]interfaces.Interface, 0, len(allInterfaces))
	for _, iface := range allInterfaces {
		ifaces = append(ifaces, iface)
	}
	sort.Sort(byIfaceName(ifaces))
	return ifaces
}

// registerIface appends the given interface into the list of all known interfaces.
func registerIface(iface interfaces.Interface) {
	if allInterfaces[iface.Name()] != nil {
		panic(fmt.Errorf("cannot register duplicate interface %q", iface.Name()))
	}
	if allInterfaces == nil {
		allInterfaces = make(map[string]interfaces.Interface)
	}
	allInterfaces[iface.Name()] = iface
}

func MockInterface(iface interfaces.Interface) func() {
	name := iface.Name()
	allInterfaces[name] = iface
	return func() {
		delete(allInterfaces, name)
	}
}

type byIfaceName []interfaces.Interface

func (c byIfaceName) Len() int      { return len(c) }
func (c byIfaceName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byIfaceName) Less(i, j int) bool {
	return c[i].Name() < c[j].Name()
}
