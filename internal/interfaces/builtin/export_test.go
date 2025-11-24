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

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
)

var RegisterIface = registerIface

// MockInterfaces replaces the set of known interfaces and returns a restore function.
func MockInterfaces(ifaces map[string]interfaces.Interface) (restore func()) {
	old := allInterfaces
	allInterfaces = ifaces
	return func() { allInterfaces = old }
}

// Interface returns the interface with the given name (or nil).
func Interface(name string) interfaces.Interface {
	return allInterfaces[name]
}

// MustInterface returns the interface with the given name or panics.
func MustInterface(name string) interfaces.Interface {
	if iface, ok := allInterfaces[name]; ok {
		return iface
	}
	panic(fmt.Errorf("cannot find interface with name %q", name))
}

func MockPlug(c *check.C, yaml string, projectId, workshop, sdkName string, plugName string) *sdk.PlugInfo {
	info := sdk.MockInfo(c, yaml, projectId, workshop)
	if plugInfo, ok := info.Plugs[plugName]; ok {
		return plugInfo
	}
	panic(fmt.Sprintf("cannot find plug %q in sdk %q", plugName, sdkName))
}

func MockSlot(c *check.C, yaml string, projectId, workshop, sdkName string, slotName string) *sdk.SlotInfo {
	info := sdk.MockInfo(c, yaml, projectId, workshop)
	if slotInfo, ok := info.Slots[slotName]; ok {
		return slotInfo
	}
	panic(fmt.Sprintf("cannot find slot %q in sdk %q", slotName, sdkName))
}
