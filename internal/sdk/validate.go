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
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/arch"
)

// InvalidSDKHookNameError reports an unsupported SDK hook name.
type InvalidSDKHookNameError string

// UnknownYamlField reports where an unknown top-level SDK YAML field was
// found.
type UnknownYamlField struct {
	Column int
	Line   int
}

// UnknownYamlFieldsError reports unknown top-level fields in an SDK YAML
// definition.
type UnknownYamlFieldsError struct {
	Fields map[string]UnknownYamlField
}

type sdkYamlValidator struct {
	sdkYaml `yaml:",inline"`
	Unknown map[string]UnknownYamlField `yaml:",inline"`
}

type sketchSDKYamlValidator struct {
	SketchSDKYaml `yaml:",inline"`
	Unknown       map[string]UnknownYamlField `yaml:",inline"`
}

const MAX_SDK_NAME_LENGTH = 40

var (
	// ErrorInvalidSDKName reports an invalid SDK name.
	ErrorInvalidSDKName = errors.New("invalid SDK name")
)

var (
	AllowedBases = []string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04", "ubuntu@26.04"}

	// AllowedHooks lists hook names accepted in sketch SDK YAML.
	AllowedHooks = []string{"setup-base", "setup-project", "save-state", "restore-state", "check-health"}

	sdkName = regexp.MustCompile(`^(?:[a-z0-9]-?)*[a-z](?:-?[a-z0-9])*$`)
	// Regular expression describing correct plug, slot and interface names.
	validPlugSlotIface = regexp.MustCompile("^[a-z](?:-?[a-z0-9])*$")
)

// Error returns a human-readable message describing the invalid hook name.
func (e InvalidSDKHookNameError) Error() string {
	return "invalid SDK hook name"
}

// Error returns a human-readable message describing the unknown YAML fields.
func (e UnknownYamlFieldsError) Error() string {
	var builder strings.Builder
	builder.WriteString("unknown SDK YAML fields: ")
	i := 0
	for name, field := range e.Fields {
		if i > 0 {
			builder.WriteString(", ")
		}
		_, _ = fmt.Fprintf(
			&builder,
			"%s (line %d, column %d)",
			name,
			field.Line,
			field.Column,
		)
		i++
	}
	return builder.String()
}

// UnmarshalYAML records the source location of an unknown YAML field value.
func (f *UnknownYamlField) UnmarshalYAML(value *yaml.Node) error {
	f.Column = value.Column
	f.Line = value.Line
	return nil
}

func infoFromYaml(y *sdkYaml) (*Info, error) {
	if y.Type == "" {
		y.Type = Regular.String()
	}
	if y.Type == System.String() && !IsSystem(y.Name) {
		return nil, fmt.Errorf(
			"type %q is reserved for the system SDK",
			y.Type,
		)
	}

	sdkInfo := &Info{
		Arch:          y.Arch,
		BadInterfaces: make(map[string]string),
		Base:          y.Base,
		BuiltAt:       nil,
		Description:   y.Description,
		License:       y.License,
		Name:          y.Name,
		PlugBinds:     make(map[string]PlugRef),
		Plugs:         make(map[string]*PlugInfo),
		Slots:         make(map[string]*SlotInfo),
		Summary:       y.Summary,
		Title:         y.Title,
		Type:          Type(y.Type),
		Version:       y.Version,
	}

	if y.BuiltAt != nil {
		sdkInfo.BuiltAt = (*time.Time)(y.BuiltAt)
	}

	err := setPlugsFromSdkYaml(y, sdkInfo)
	if err != nil {
		return nil, err
	}
	err = setSlotsFromSdkYaml(y, sdkInfo)
	if err != nil {
		return nil, err
	}

	SanitizePlugsSlots(sdkInfo)
	return sdkInfo, nil
}

// newUnknownYamlFieldsError collects unknown YAML fields into an error.
func newUnknownYamlFieldsError(
	unknownFields map[string]UnknownYamlField,
) error {
	return &UnknownYamlFieldsError{Fields: unknownFields}
}

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
			return err
		}
		if err := ValidateInterfaceName(plug.Interface); err != nil {
			return fmt.Errorf("invalid interface name %q for plug %q", plug.Interface, plugName)
		}
	}
	for slotName, slot := range sdk.Slots {
		if err := ValidateSlotName(slotName); err != nil {
			return err
		}
		if err := ValidateInterfaceName(slot.Interface); err != nil {
			return fmt.Errorf("invalid interface name %q for slot %q", slot.Interface, slotName)
		}
	}
	return nil
}

// ParseSketchYaml parses and validates a sketch SDK YAML definition.
func ParseSketchYaml(reader io.Reader) (SketchSDKYaml, error) {
	var validator sketchSDKYamlValidator
	dec := yaml.NewDecoder(reader)
	err := dec.Decode(&validator)

	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return SketchSDKYaml{}, fmt.Errorf(
			"sketch SDK YAML:\n%s",
			strings.Join(typeErr.Errors, "\n"),
		)
	} else if err != nil {
		return SketchSDKYaml{}, err
	}

	if len(validator.Unknown) > 0 {
		return SketchSDKYaml{}, newUnknownYamlFieldsError(validator.Unknown)
	}

	err = ValidateSketchYaml(&validator.SketchSDKYaml)
	if err != nil {
		return SketchSDKYaml{}, err
	}
	return validator.SketchSDKYaml, nil
}

// ValidateSketchYaml checks whether y contains a valid sketch SDK definition.
func ValidateSketchYaml(y *SketchSDKYaml) error {
	if !IsSketch(y.Name) {
		return fmt.Errorf(
			"%w, sketch SDK name can only be %q",
			ErrorInvalidSDKName,
			Sketch,
		)
	}

	for hookName := range y.Hooks {
		if !slices.Contains(AllowedHooks, hookName) {
			return InvalidSDKHookNameError(hookName)
		}
	}

	for plugName, plug := range y.Plugs {
		iface, _, _, err := convertToSlotOrPlugData("plug", plugName, plug)
		if err != nil {
			return err
		}
		err = ValidatePlugName(plugName)
		if err != nil {
			return err
		}
		err = ValidateInterfaceName(iface)
		if err != nil {
			return fmt.Errorf(
				"invalid interface name %q for plug %q",
				iface,
				plugName,
			)
		}
	}

	for slotName, slot := range y.Slots {
		iface, _, _, err := convertToSlotOrPlugData("slot", slotName, slot)
		if err != nil {
			return err
		}
		err = ValidateSlotName(slotName)
		if err != nil {
			return err
		}
		err = ValidateInterfaceName(iface)
		if err != nil {
			return fmt.Errorf(
				"invalid interface name %q for slot %q",
				iface,
				slotName,
			)
		}
	}
	return nil
}

// ValidateYaml checks whether reader contains a valid SDK YAML definition.
func ValidateYaml(reader io.Reader) error {
	var validator sdkYamlValidator
	dec := yaml.NewDecoder(reader)
	err := dec.Decode(&validator)

	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return fmt.Errorf(
			"SDK definition YAML:\n%s",
			strings.Join(typeErr.Errors, "\n"),
		)
	}
	if err != nil {
		return err
	}

	if len(validator.Unknown) > 0 {
		return newUnknownYamlFieldsError(validator.Unknown)
	}

	sdkInfo, err := infoFromYaml(&validator.sdkYaml)
	if err != nil {
		return err
	}

	return Validate(sdkInfo)
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
		return fmt.Errorf("invalid plug name: %q", name)
	}
	return nil
}

// ValidateSlot checks if a string can be used as a slot name.
//
// Slot names and plug names within one sdk must have unique names.
func ValidateSlotName(name string) error {
	if !validPlugSlotIface.MatchString(name) {
		return fmt.Errorf("invalid slot name: %q", name)
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
