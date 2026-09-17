// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2026 Canonical Ltd
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
	"slices"
	"strconv"
	"strings"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const customDeviceSummary = `allows sharing custom devices with SDKs`

const customDeviceBaseDeclarationSlots = `
  custom-device:
    allow-installation:
      slot-sdk-type:
        - system
      slot-names:
        - $INTERFACE
    allow-connection: true
    deny-auto-connection: true
`

const customDeviceBaseDeclarationPlugs = `
  custom-device:
    allow-installation:
      plug-sdk-type:
        - regular
    allow-connection: true
    deny-auto-connection: true
`

var knownCustomDeviceAttributes = []string{"subsystem", "vendorid", "productid"}

type customDeviceInterface struct{}

func (iface *customDeviceInterface) Name() string {
	return "custom-device"
}

func (iface *customDeviceInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              customDeviceSummary,
		BaseDeclarationPlugs: customDeviceBaseDeclarationPlugs,
		BaseDeclarationSlots: customDeviceBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *customDeviceInterface) BeforePreparePlug(plug *sdk.PlugInfo) error {
	for name := range plug.Attrs {
		if !slices.Contains(knownCustomDeviceAttributes, name) {
			return fmt.Errorf("unknown attribute for custom-device interface plug: %q", name)
		}
	}

	// Every attribute is optional, so the map may be nil; make it writable for
	// the parse helpers, which default absent attributes.
	if plug.Attrs == nil {
		plug.Attrs = make(map[string]any)
	}

	subsystem, err := parseString(plug.Attrs, "subsystem")
	if err != nil {
		return err
	}
	vendorID, err := parseDeviceID(plug.Attrs, "vendorid")
	if err != nil {
		return err
	}
	productID, err := parseDeviceID(plug.Attrs, "productid")
	if err != nil {
		return err
	}

	// subsystem, productid, and vendorid are each optional, but the plug must
	// narrow the exposed host devices with at least one of them.
	if subsystem == nil && vendorID == "" && productID == "" {
		return fmt.Errorf(`custom-device plug must contain "subsystem", "vendorid", or "productid"`)
	}
	// A product ID only identifies a device within a vendor's namespace.
	if vendorID == "" && productID != "" {
		return fmt.Errorf(`custom-device plug contains "productid" without "vendorid"`)
	}

	return nil
}

func parseString(attrs map[string]any, key string) (*string, error) {
	object, ok := attrs[key]
	if !ok {
		return nil, nil
	}
	value, ok := object.(string)
	if !ok {
		return nil, fmt.Errorf("custom-device plug %q is not a string (found %T)", key, object)
	}
	return &value, nil
}

func parseDeviceID(attrs map[string]any, key string) (string, error) {
	value, err := parseString(attrs, key)
	if value == nil || err != nil {
		return "", err
	}

	id, err := strconv.ParseUint(strings.TrimPrefix(*value, "0x"), 16, 16)
	if err != nil {
		return "", fmt.Errorf("custom-device plug %q must be a hexadecimal number", key)
	}

	udevForm := fmt.Sprintf("%04x", id)
	attrs[key] = udevForm
	return udevForm, nil
}

func (iface *customDeviceInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	// allow what declarations allowed
	return true
}

func (iface *customDeviceInterface) LxdDeviceSupportsPlug(instance *lxd_device.Instance, plug *sdk.PlugInfo) error {
	if instance.Confinement != workshop.ConfinementContainer {
		return errors.New("custom-device interface only available to containers")
	}
	return nil
}

func (iface *customDeviceInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	device := workshop.CustomDevice{Name: plug.Name()}

	var attrErr *sdk.AttributeNotFoundError
	if err := plug.Attr("subsystem", &device.Subsystem); err != nil && !errors.As(err, &attrErr) {
		return err
	}
	if err := plug.Attr("vendorid", &device.VendorID); err != nil && !errors.As(err, &attrErr) {
		return err
	}
	if err := plug.Attr("productid", &device.ProductID); err != nil && !errors.As(err, &attrErr) {
		return err
	}

	return spec.AddCustomDevice(device)
}

func init() {
	registerIface(&customDeviceInterface{})
}
