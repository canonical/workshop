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
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

func init() {
	// setup the ByName function using allInterfaces
	interfaces.ByName = func(name string) (interfaces.Interface, error) {
		iface, ok := allInterfaces[name]
		if !ok {
			return nil, fmt.Errorf("interface %q not found", name)
		}
		return iface, nil
	}
}

var (
	allInterfaces map[string]interfaces.Interface

	sanitizeSdk = (*Sanitizer).sanitize
)

func Sanitize(sdkInfo *sdk.Info, runtime workshop.Runtime) error {
	return new(Sanitizer).Sanitize(sdkInfo, runtime)
}

// Sanitizer sanitizes the plugs and slots of an SDK and prunes those that are
// incompatible with the workshop runtime. Pruned plugs and slots are not an
// error; they are reported in Warning after Sanitize returns.
type Sanitizer struct {
	Warning error
}

func (s *Sanitizer) Sanitize(sdkInfo *sdk.Info, runtime workshop.Runtime) error {
	return sanitizeSdk(s, sdkInfo, runtime)
}

func (s *Sanitizer) sanitize(sdkInfo *sdk.Info, runtime workshop.Runtime) error {
	badPlugs := map[string][]string{}
	badSlots := map[string][]string{}
	skippedPlugs := map[string][]string{}
	skippedSlots := map[string][]string{}
	var prunePlugs []string
	var pruneSlots []string

	for plugName, plugInfo := range sdkInfo.Plugs {
		if err := sanitizePlug(plugInfo, runtime); err != nil {
			reason := err.Error()
			if _, ok := errors.AsType[skipped](err); ok {
				skippedPlugs[reason] = append(skippedPlugs[reason], plugName)
			} else {
				badPlugs[reason] = append(badPlugs[reason], plugName)
			}
			prunePlugs = append(prunePlugs, plugName)
		}
	}

	for slotName, slotInfo := range sdkInfo.Slots {
		if err := sanitizeSlot(slotInfo, runtime); err != nil {
			reason := err.Error()
			if _, ok := errors.AsType[skipped](err); ok {
				skippedSlots[reason] = append(skippedSlots[reason], slotName)
			} else {
				badSlots[reason] = append(badSlots[reason], slotName)
			}
			pruneSlots = append(pruneSlots, slotName)
		}
	}

	// remove any bad plugs and slots
	for _, plugName := range prunePlugs {
		delete(sdkInfo.Plugs, plugName)
	}
	for _, slotName := range pruneSlots {
		delete(sdkInfo.Slots, slotName)
	}

	s.Warning = interfacesSummary(sdkInfo.Name, "incompatible", skippedPlugs, skippedSlots)
	return interfacesSummary(sdkInfo.Name, "bad", badPlugs, badSlots)
}

func sanitizePlug(plugInfo *sdk.PlugInfo, runtime workshop.Runtime) error {
	iface, ok := allInterfaces[plugInfo.Interface]
	if !ok {
		return fmt.Errorf("unknown interface %q", plugInfo.Interface)
	}
	// Reject plug with invalid name
	if err := sdk.ValidatePlugName(plugInfo.Name); err != nil {
		return err
	}
	if err := interfaces.BeforePreparePlug(iface, plugInfo); err != nil {
		return err
	}
	if err := interfaces.CheckCompatiblePlug(iface, plugInfo, runtime); err != nil {
		return skipped{err: err}
	}
	return nil
}

func sanitizeSlot(slotInfo *sdk.SlotInfo, runtime workshop.Runtime) error {
	iface, ok := allInterfaces[slotInfo.Interface]
	if !ok {
		return fmt.Errorf("unknown interface %q", slotInfo.Interface)
	}
	// Reject slot with invalid name
	if err := sdk.ValidateSlotName(slotInfo.Name); err != nil {
		return err
	}
	if err := interfaces.BeforePrepareSlot(iface, slotInfo); err != nil {
		return err
	}
	if err := interfaces.CheckCompatibleSlot(iface, slotInfo, runtime); err != nil {
		return skipped{err: err}
	}
	return nil
}

type skipped struct {
	err error
}

func (s skipped) Error() string {
	return s.err.Error()
}

func (s skipped) Unwrap() error {
	return s.err
}

func interfacesSummary(name, problem string, plugs, slots map[string][]string) error {
	if len(plugs) == 0 && len(slots) == 0 {
		return nil
	}

	var buf strings.Builder
	if len(plugs) > 0 {
		fmt.Fprintf(&buf, "%q SDK has %s plugs: ", name, problem)
		interfaceSummary(&buf, plugs)
	}
	if len(slots) > 0 {
		if buf.Len() == 0 {
			fmt.Fprintf(&buf, "%q SDK has %s slots: ", name, problem)
		} else {
			buf.WriteString("and slots: ")
		}
		interfaceSummary(&buf, slots)
	}
	return errors.New(strings.TrimSuffix(buf.String(), "; "))
}

func interfaceSummary(buf *strings.Builder, bad map[string][]string) {
	reasons := slices.Sorted(maps.Keys(bad))
	for _, reason := range reasons {
		names := bad[reason]
		slices.Sort(names)
		for i, name := range names {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(name)
		}
		fmt.Fprintf(buf, " (%s); ", reason)
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

func MockSanitize(f func(s *Sanitizer, sdkInfo *sdk.Info, runtime workshop.Runtime) error) (restore func()) {
	old := sanitizeSdk
	sanitizeSdk = f
	return func() { sanitizeSdk = old }
}

type byIfaceName []interfaces.Interface

func (c byIfaceName) Len() int      { return len(c) }
func (c byIfaceName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byIfaceName) Less(i, j int) bool {
	return c[i].Name() < c[j].Name()
}
