// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package sdk

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/canonical/workshop/internal/arch"
)

const MAX_SDK_NAME_LENGTH = 40

var (
	AllowedBases = []string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04", "ubuntu@26.04"}

	// AllowedHooks lists hook names accepted in sketch SDK YAML.
	AllowedHooks = []string{"setup-base", "setup-project", "save-state", "restore-state", "check-health"}

	sdkName = regexp.MustCompile(`^(?:[a-z0-9]-?)*[a-z](?:-?[a-z0-9])*$`)
	// Regular expression describing correct plug, slot and interface names.
	validPlugSlotIface = regexp.MustCompile("^[a-z](?:-?[a-z0-9])*$")
)

// Validate checks whether sdk contains a valid SDK definition.
func Validate(sdk *Info) error {
	if err := ValidateName(sdk.Name); err != nil {
		return err
	}

	if sdk.Base != "" && !slices.Contains(AllowedBases, sdk.Base) {
		return fmt.Errorf("invalid SDK base %q; valid bases: %s", sdk.Base, strings.Join(AllowedBases, ", "))
	}
	if !slices.Contains([]string{"", "all"}, sdk.Arch) && !slices.Contains(arch.AllowedArchitectures, sdk.Arch) {
		arches := strings.Join(arch.AllowedArchitectures, ", ")
		return fmt.Errorf("invalid SDK architecture %q; supported architectures: %s", sdk.Arch, arches)
	}

	for plugName, plug := range sdk.Plugs {
		if err := ValidatePlugName(plugName); err != nil {
			return fmt.Errorf("%w: %q", err, plugName)
		}
		if err := ValidateInterfaceName(plug.Interface); err != nil {
			return fmt.Errorf("invalid interface name %q for plug %q", plug.Interface, plugName)
		}
	}
	for slotName, slot := range sdk.Slots {
		if err := ValidateSlotName(slotName); err != nil {
			return fmt.Errorf("%w: %q", err, slotName)
		}
		if err := ValidateInterfaceName(slot.Interface); err != nil {
			return fmt.Errorf("invalid interface name %q for slot %q", slot.Interface, slotName)
		}
	}
	return nil
}

// ValidateName checks if a string can be used as an SDK name.
func ValidateName(name string) error {
	if name == "agent" || strings.HasPrefix(name, "try-") || strings.HasPrefix(name, "project-") {
		return fmt.Errorf("%q is a reserved SDK name", name)
	}
	if !sdkName.MatchString(name) {
		return fmt.Errorf("invalid SDK name %q", name)
	}
	if len(name) > MAX_SDK_NAME_LENGTH {
		return fmt.Errorf("SDK name %q too long", name)
	}
	return nil
}

// ValidatePlug checks if a string can be used as a slot name.
//
// Slot names and plug names within one sdk must have unique names.
func ValidatePlugName(name string) error {
	if !validPlugSlotIface.MatchString(name) {
		return errors.New("invalid plug name")
	}
	return nil
}

// ValidateSlot checks if a string can be used as a slot name.
//
// Slot names and plug names within one sdk must have unique names.
func ValidateSlotName(name string) error {
	if !validPlugSlotIface.MatchString(name) {
		return errors.New("invalid slot name")
	}
	return nil
}

// ValidateInterface checks if a string can be used as an interface name.
func ValidateInterfaceName(name string) error {
	if !validPlugSlotIface.MatchString(name) {
		return fmt.Errorf("invalid interface name: %q", name)
	}
	return nil
}
