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
	"sort"

	"github.com/canonical/workshop/internal/sdk"
)

type byConnRef []*ConnRef

func (c byConnRef) Len() int      { return len(c) }
func (c byConnRef) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byConnRef) Less(i, j int) bool {
	return c[i].SortsBefore(c[j])
}

type byPlugWorkshopSdkAndName []*sdk.PlugInfo

func (c byPlugWorkshopSdkAndName) Len() int      { return len(c) }
func (c byPlugWorkshopSdkAndName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byPlugWorkshopSdkAndName) Less(i, j int) bool {
	if c[i].Sdk.Workshop != c[j].Sdk.Workshop {
		return c[i].Sdk.Workshop < c[j].Sdk.Workshop
	}
	if c[i].Sdk.Sdk != c[j].Sdk.Sdk {
		return c[i].Sdk.Sdk < c[j].Sdk.Sdk
	}
	return c[i].Name < c[j].Name
}

type bySlotWorkshopSdkAndName []*sdk.SlotInfo

func (c bySlotWorkshopSdkAndName) Len() int      { return len(c) }
func (c bySlotWorkshopSdkAndName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c bySlotWorkshopSdkAndName) Less(i, j int) bool {
	if c[i].Sdk.Workshop != c[j].Sdk.Workshop {
		return c[i].Sdk.Workshop < c[j].Sdk.Workshop
	}
	if c[i].Sdk.Sdk != c[j].Sdk.Sdk {
		return c[i].Sdk.Sdk < c[j].Sdk.Sdk
	}
	return c[i].Name < c[j].Name
}

func sortedSdkNamesWithPlugs(m map[string]map[string]*sdk.PlugInfo) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedPlugNames(m map[string]*sdk.PlugInfo) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSdkNamesWithSlots(m map[string]map[string]*sdk.SlotInfo) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSlotNames(m map[string]*sdk.SlotInfo) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type byInterfaceName []Interface

func (c byInterfaceName) Len() int      { return len(c) }
func (c byInterfaceName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byInterfaceName) Less(i, j int) bool {
	return c[i].Name() < c[j].Name()
}
