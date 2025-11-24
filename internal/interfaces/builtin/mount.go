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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil/sys"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const mountSummary = `allows sharing host code and data with SDKs`

const mountBaseDeclarationSlots = `
  mount:
    allow-installation:
      -
        slot-sdk-type:
          - system
        slot-names:
          - $INTERFACE
      -
        slot-sdk-type:
          - regular
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
        slot-sdk-type:
          - system
      -
        plug-attributes:
          auto-explicit: true
`

var (
	knownPlugAttributes = []string{"workshop-target", "mode", "uid", "gid", "read-only"}
	knownSlotAttributes = []string{"workshop-source"}
)

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

func (iface *mountInterface) BeforePreparePlug(plug *sdk.PlugInfo) error {
	for name := range plug.Attrs {
		if !slices.Contains(knownPlugAttributes, name) {
			return fmt.Errorf(`unknown attribute for mount interface plug: %q`, name)
		}
	}

	path, err := parseMountPath(plug.Attrs, "plug", "workshop-target", plug.Sdk.Name)
	if err != nil {
		return err
	}

	if _, err := parseBool(plug.Attrs, "read-only", false); err != nil {
		return err
	}

	var fallbackUid, fallbackGid int64
	for _, prefix := range []string{"/home/workshop", "/project", "/run/user/1000"} {
		if path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator)) {
			fallbackUid = workshop.Uid
			fallbackGid = workshop.Gid
			break
		}
	}

	uid, err := parseInt(plug.Attrs, "uid", fallbackUid)
	if err != nil {
		return err
	}
	if uid < 0 || uid >= sys.FlagID {
		return fmt.Errorf(`invalid value %v in key "uid" for mount interface plug: must be between 0 and %#x`, uid, sys.FlagID)
	}

	gid, err := parseInt(plug.Attrs, "gid", fallbackGid)
	if err != nil {
		return err
	}
	if gid < 0 || gid >= sys.FlagID {
		return fmt.Errorf(`invalid value %v in key "gid" for mount interface plug: must be between 0 and %#x`, gid, sys.FlagID)
	}

	fallbackMode := os.ModePerm &^ workshop.NormalUmask
	if uid == 0 {
		fallbackMode = os.ModePerm &^ workshop.RootUmask
	}
	mode, err := parseInt(plug.Attrs, "mode", int64(fallbackMode))
	if err != nil {
		return err
	}
	if mode < 0 || uint64(mode)&^uint64(os.ModePerm) != 0 {
		return fmt.Errorf(`invalid value %#o in key "mode" for mount interface plug: permissions limited to %#o`, mode, os.ModePerm)
	}

	return nil
}

func parseBool(attrs map[string]interface{}, key string, fallback bool) (bool, error) {
	object, ok := attrs[key]
	if !ok {
		attrs[key] = fallback
		return fallback, nil
	}

	switch value := object.(type) {
	case bool:
		return value, nil
	case string:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf(`unknown value %q in key %q for mount interface plug. Accepted values are 'true' or 'false'. String representations (e.g., '"true"') are also permitted`, value, key)
		}
		attrs[key] = parsed
		return parsed, nil
	default:
		return false, fmt.Errorf(`unknown value type %T in key %q for mount interface plug. Accepted types are 'bool' or 'string'`, object, key)
	}
}

func parseInt(attrs map[string]interface{}, key string, fallback int64) (int64, error) {
	object, ok := attrs[key]
	if !ok {
		attrs[key] = fallback
		return fallback, nil
	}

	switch value := object.(type) {
	case int64:
		return value, nil
	case string:
		parsed, err := strconv.ParseInt(value, 0, 64)
		if err != nil {
			return 0, fmt.Errorf(`unknown value %q in key %q for mount interface plug. Accepted types are 'int64'`, value, key)
		}
		attrs[key] = parsed
		return parsed, nil
	default:
		return 0, fmt.Errorf(`unknown value type %T in key %q for mount interface plug. Accepted types are 'int64' or 'string'`, object, key)
	}
}

func (iface *mountInterface) BeforePrepareSlot(slot *sdk.SlotInfo) error {
	if slot.Sdk.Type == sdk.System {
		for name := range slot.Attrs {
			return fmt.Errorf(`unknown attribute for system mount interface slot: %q`, name)
		}
		return nil
	}

	for name := range slot.Attrs {
		if !slices.Contains(knownSlotAttributes, name) {
			return fmt.Errorf(`unknown attribute for mount interface slot: %q`, name)
		}
	}

	_, err := parseMountPath(slot.Attrs, "slot", "workshop-source", slot.Sdk.Name)
	return err
}

func parseMountPath(attrs map[string]interface{}, kind, key string, sk string) (string, error) {
	attr, exist := attrs[key]
	if !exist {
		return "", fmt.Errorf("mount %s must contain %q", kind, key)
	}
	template, ok := attr.(string)
	if !ok {
		return "", fmt.Errorf(`mount %s %q is not a string (found %T)`, kind, key, attr)
	}

	path, err := expandMountPath(template, sk)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf(`mount %s %q must be absolute: %q`, kind, key, path)
	}
	if filepath.Clean(path) != path {
		return "", fmt.Errorf(`mount %s %q is not clean: %q`, kind, key, path)
	}

	attrs[key] = path
	return path, nil
}

func expandMountPath(template string, sk string) (string, error) {
	var err error
	path := os.Expand(template, func(s string) string {
		switch s {
		case "SDK":
			return sdk.SdkDir(sk)
		case "$":
			// Unescape $$ -> $.
			return "$"
		default:
			err = fmt.Errorf("unexpected variable %q", s)
			return ""
		}
	})

	if err != nil {
		return "", err
	}
	return path, nil
}

func (iface *mountInterface) setPlugAttrs(mount *workshop.Mount, plug *interfaces.ConnectedPlug) error {
	if err := plug.Attr("workshop-target", &mount.Where); err != nil {
		return err
	}
	mount.MakeWhere = true

	var value int64
	if err := plug.Attr("mode", &value); err != nil {
		return err
	}
	mount.Mode = os.FileMode(value)

	if err := plug.Attr("uid", &value); err != nil {
		return err
	}
	mount.Owner = sys.UserID(value)

	if err := plug.Attr("gid", &value); err != nil {
		return err
	}
	mount.Group = sys.GroupID(value)

	if err := plug.Attr("read-only", &mount.ReadOnly); err != nil {
		return err
	}

	return nil
}

func (iface *mountInterface) setRegularSlotAttrs(mount *workshop.Mount, slot *interfaces.ConnectedSlot) error {
	mount.Type = workshop.WorkshopWorkshop

	return slot.Attr("workshop-source", &mount.What)
}

func (iface *mountInterface) setSystemSlotAttrs(mount *workshop.Mount, spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	mount.Type = workshop.HostWorkshop

	var attrErr *sdk.AttributeNotFoundError
	if err := slot.Attr("host-source", &mount.What); err == nil {
		return nil
	} else if !errors.As(err, &attrErr) {
		return err
	}

	// default dir: <sdk>/<plug>
	userDataDir := workshop.UserDataRootDir(spec.User.HomeDir, spec.Environment)
	mount.What = workshop.SdkMountHostSource(userDataDir, slot.Sdk().ProjectId, slot.Sdk().Workshop, plug.Sdk().Name, plug.Name())
	mount.MakeWhat = true
	return nil
}

func (iface *mountInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

// Interactions with the mount backend.
func (iface *mountInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	mount := workshop.Mount{Name: plug.Name()}

	if err := iface.setPlugAttrs(&mount, plug); err != nil {
		return err
	}

	if slot.Sdk().Type == sdk.System {
		if err := iface.setSystemSlotAttrs(&mount, spec, plug, slot); err != nil {
			return err
		}
	} else {
		if err := iface.setRegularSlotAttrs(&mount, slot); err != nil {
			return err
		}
	}

	return spec.AddMountEntry(mount)
}

func init() {
	registerIface(&mountInterface{})
}
