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
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const mountSummary = `allows sharing host code and data with SDKs`

const mountBaseDeclarationSlots = `
  mount:
    allow-installation:
      slot-sdk-type:
        - system
        - regular
    deny-installation:
      slot-attributes:
        host-source: .*
    allow-connection: true
    allow-auto-connection: true
`

const mountBaseDeclarationPlugs = `
  mount:
    allow-installation:
      plug-sdk-type:
        - regular
    allow-connection: true
    allow-auto-connection:
      -
        slot-names:
          - $INTERFACE
      -
        plug-attributes:
          auto-explicit: true
`

var knownPlugAttributes = []string{"workshop-target", "read-only"}
var knownSlotAttributes = []string{"workshop-source", "host-source"}

// mountInterface allows sharing content between sdks
type mountInterface struct{}

func (iface *mountInterface) Name() string {
	return "mount"
}

func (iface *mountInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              mountSummary,
		BaseDeclarationPlugs: mountBaseDeclarationPlugs,
		BaseDeclarationSlots: mountBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func cleanSubPath(path string) bool {
	return filepath.Clean(path) == path && path != ".." && !strings.HasPrefix(path, "../")
}

func validatePath(path string) error {
	if ok := cleanSubPath(path); !ok {
		return fmt.Errorf("mount interface path is not clean: %q", path)
	}
	return nil
}

func (iface *mountInterface) BeforePreparePlug(plug *sdk.PlugInfo) error {
	for name := range plug.Attrs {
		if !slices.Contains(knownPlugAttributes, name) {
			return fmt.Errorf(`unknown attribute for mount interface plug: %q`, name)
		}
	}

	target, ok := plug.Attrs["workshop-target"].(string)
	if !ok || len(target) == 0 {
		return fmt.Errorf("mount plug must contain target path")
	}

	if err := validatePath(target); err != nil {
		return err
	}

	ro, ok := plug.Attrs["read-only"]
	if !ok {
		ro = false
	}

	switch ro := ro.(type) {
	case bool:
		plug.Attrs["read-only"] = ro
	case string:
		roBool, err := strconv.ParseBool(ro)
		if err != nil {
			return fmt.Errorf(`unknown value %q in key "read-only" for mount interface plug. Accepted values are 'true' or 'false'. String representations (ie. '"true"') are also permitted.`, ro)
		}
		plug.Attrs["read-only"] = roBool
	default:
		return fmt.Errorf(`unknown value type %T in key "read-only" for mount interface plug. Accepted types are 'bool' or 'string'.`, ro)
	}
	return nil
}

func (iface *mountInterface) BeforePrepareSlot(slot *sdk.SlotInfo) error {
	for name := range slot.Attrs {
		if !slices.Contains(knownSlotAttributes, name) {
			return fmt.Errorf(`unknown attribute for mount interface slot: %q`, name)
		}
	}
	source, exist := slot.Attrs["workshop-source"]
	if !exist {
		// perfectly fine scenario for the default mount slot
		return nil
	}
	path, ok := source.(string)
	if !ok {
		return fmt.Errorf(`mount slot "workshop-source" is not a string (found %T)`, source)
	}

	if strings.HasPrefix(path, "$SDK") {
		path = strings.Replace(path, "$SDK", sdk.SdkCurrentPath(slot.Sdk.Name), 1)
	}

	if !filepath.IsAbs(path) {
		return fmt.Errorf(`mount slot "workshop-source" must be absolute`)
	}
	return nil
}

func (iface *mountInterface) target(attrs interfaces.Attrer) string {
	var target string

	if err := attrs.Attr("workshop-target", &target); err == nil {
		return target
	}
	return ""
}

func (iface *mountInterface) readOnly(attrs interfaces.Attrer) bool {
	var ro bool
	attrs.Attr("read-only", &ro)
	return ro
}

func (iface *mountInterface) workshopSource(slot *interfaces.ConnectedSlot) (string, error) {
	var source string
	err := slot.Attr("workshop-source", &source)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(source, "$SDK") {
		return strings.Replace(source, "$SDK", sdk.SdkCurrentPath(slot.Ref().Sdk), 1), nil
	}
	return source, nil
}

func (iface *mountInterface) hostSource(baseDir string, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) (string, error) {
	var source string
	err := slot.Attr("host-source", &source)
	if err == nil {
		return source, nil
	}
	// default dir: <workshop>_<sdk>_plug.sdk
	source = sdk.SdkMountHostSource(baseDir, slot.Sdk().ProjectId, slot.Sdk().Workshop, plug.Sdk().Name, plug.Name())
	if err = slot.SetAttr("host-source", source); err != nil {
		return "", err
	}
	return source, nil
}

func (iface *mountInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

func (iface *mountInterface) MountConnectedSlot(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	return nil
}

// Interactions with the mount backend.
func (iface *mountInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	source, err := iface.workshopSource(slot)
	if err != nil && !errors.Is(err, sdk.AttributeNotFoundError{}) {
		return err
	}
	if err == nil {
		return spec.AddMountEntry(workshop.Mount{Name: plug.Name(), What: source, Where: iface.target(plug), Type: workshop.WorkshopWorkshop, ReadOnly: iface.readOnly(plug)})
	}

	source, err = iface.hostSource(spec.User.HomeDir, plug, slot)
	if err == nil {
		return spec.AddMountEntry(workshop.Mount{Name: plug.Name(), What: source, Where: iface.target(plug), Type: workshop.HostWorkshop, ReadOnly: iface.readOnly(plug)})
	}

	return err
}

func init() {
	registerIface(&mountInterface{})
}
