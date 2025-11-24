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
	"net"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const tunnelSummary = `allows SDKs and the host to share network services`

const tunnelBaseDeclarationSlots = `
  tunnel:
    allow-installation: true
    allow-connection: true
    allow-auto-connection: true
`

const tunnelBaseDeclarationPlugs = `
  tunnel:
    allow-installation: true
    allow-connection: true
    allow-auto-connection:
      -
        plug-sdk-type:
          - system
        slot-sdk-type:
          - regular
        slot-names:
          - $PLUG
      -
        plug-sdk-type:
          - system
        slot-sdk-type:
          - regular
        plug-attributes:
          auto-explicit: true
`

var knownTunnelAttributes = []string{"endpoint"}

type tunnelInterface struct{}

func (iface *tunnelInterface) Name() string {
	return "tunnel"
}

func (iface *tunnelInterface) StaticInfo() interfaces.StaticInfo {
	return interfaces.StaticInfo{
		Summary:              tunnelSummary,
		BaseDeclarationPlugs: tunnelBaseDeclarationPlugs,
		BaseDeclarationSlots: tunnelBaseDeclarationSlots,
		AffectsPlugOnRefresh: true,
	}
}

func (iface *tunnelInterface) BeforePreparePlug(plug *sdk.PlugInfo) error {
	for name := range plug.Attrs {
		if !slices.Contains(knownTunnelAttributes, name) {
			return fmt.Errorf(`unknown attribute for tunnel interface plug: %q`, name)
		}
	}

	address, err := normalizeEndpoint(plug)
	if err != nil {
		return err
	}
	if plug.Attrs == nil {
		plug.Attrs = make(map[string]interface{})
	}
	plug.Attrs["endpoint"] = address

	return nil
}

func (iface *tunnelInterface) BeforePrepareSlot(slot *sdk.SlotInfo) error {
	for name := range slot.Attrs {
		if !slices.Contains(knownTunnelAttributes, name) {
			return fmt.Errorf(`unknown attribute for tunnel interface slot: %q`, name)
		}
	}

	address, err := normalizeEndpoint(slot)
	if err != nil {
		return err
	}
	if slot.Attrs == nil {
		slot.Attrs = make(map[string]interface{})
	}
	slot.Attrs["endpoint"] = address

	return nil
}

func normalizeEndpoint(attrs interfaces.Attrer) (string, error) {
	var port int64
	var endpoint string
	var attrErr *sdk.AttributeNotFoundError
	if err := attrs.Attr("endpoint", &port); err == nil {
		endpoint = strconv.FormatInt(port, 10)
	} else if err = attrs.Attr("endpoint", &endpoint); err != nil && !errors.As(err, &attrErr) {
		return "", err
	}

	target := parseEndpoint(endpoint)
	if isIP(target.Protocol) {
		address, err := normalizeAddress(target.Address)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s", address, target.Protocol), nil
	}
	return endpoint, nil
}

func normalizeAddress(address string) (string, error) {
	host, port := parseAddress(address)
	if strings.HasPrefix(address, "[") && !strings.ContainsRune(host, ':') {
		return "", fmt.Errorf("invalid bracketed address %q", address)
	}

	switch host {
	case "localhost":
		host = "127.0.0.1"
	case "ip6-localhost", "ip6-loopback":
		host = "::1"
	}

	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("invalid IP address %q", host)
	}

	if port == "" {
		return host, nil
	}

	_, err := strconv.ParseUint(port, 10, 16)
	var numErr *strconv.NumError
	if errors.As(err, &numErr) {
		return "", fmt.Errorf("invalid port %q: %w", numErr.Num, numErr.Err)
	} else if err != nil {
		return "", err
	}

	return net.JoinHostPort(host, port), nil
}

func parseEndpoint(endpoint string) workshop.ProxyTarget {
	// Leave unix sockets untouched.
	if filepath.IsAbs(endpoint) || strings.HasPrefix(endpoint, "@") || strings.HasPrefix(endpoint, "$") {
		return workshop.ProxyTarget{Address: endpoint, Protocol: "unix"}
	}

	address, found := strings.CutSuffix(endpoint, "/udp")
	if found {
		return workshop.ProxyTarget{Address: address, Protocol: "udp"}
	}

	if endpoint == "udp" || endpoint == "tcp" {
		return workshop.ProxyTarget{Address: "", Protocol: endpoint}
	}

	address = strings.TrimSuffix(endpoint, "/tcp")
	return workshop.ProxyTarget{Address: address, Protocol: "tcp"}
}

func isIP(protocol string) bool {
	return protocol == "tcp" || protocol == "udp"
}

func parseAddress(address string) (string, string) {
	if !strings.ContainsFunc(address, func(c rune) bool { return c < '0' || '9' < c }) {
		return "localhost", address
	}

	host, port, err := net.SplitHostPort(address)
	if err == nil && host != "" && port != "" {
		return host, port
	}

	// Remove square brackets from standalone address.
	host, port, err = net.SplitHostPort(address + ":")
	if err == nil && host != "" && port == "" {
		return host, port
	}

	return address, ""
}

func (iface *tunnelInterface) AutoConnect(plug *sdk.PlugInfo, slot *sdk.SlotInfo) bool {
	var endpoint string
	if err := plug.Attr("endpoint", &endpoint); err != nil {
		logger.Noticef("Cannot auto-connect: %v", err)
		return false
	}

	target := parseEndpoint(endpoint)
	if !isIP(target.Protocol) {
		// Allow what declarations allowed.
		return true
	}

	host, _ := parseAddress(target.Address)
	if ip := net.ParseIP(host); ip != nil {
		// Avoid automatically exposing workshop services to other hosts.
		return ip.IsLoopback()
	}

	logger.Noticef("Cannot auto-connect plug %q: invalid IP address %q", plug.Ref().ShortRef(), host)
	return false
}

func (iface *tunnelInterface) MountConnectedPlug(spec *lxd_device.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	entry := workshop.ProxyEntry{Name: plug.Name()}

	if plug.Sdk().Type == sdk.System {
		if slot.Sdk().Type == sdk.System {
			return errors.New("cannot connect system SDK to itself")
		}
		entry.Direction = workshop.HostToWorkshop
	} else {
		if slot.Sdk().Type == sdk.Regular {
			return errors.New("cannot connect regular SDKs from the same workshop")
		}
		entry.Direction = workshop.WorkshopToHost
	}

	var endpoint string
	if err := plug.Attr("endpoint", &endpoint); err != nil {
		return err
	}
	entry.Listen = parseEndpoint(endpoint)
	if err := slot.Attr("endpoint", &endpoint); err != nil {
		return err
	}
	entry.Connect = parseEndpoint(endpoint)

	if err := checkProtocols(entry.Listen.Protocol, entry.Connect.Protocol); err != nil {
		return err
	}
	if err := inferPorts(&entry.Listen, &entry.Connect); err != nil {
		return err
	}
	if err := checkListenPort(entry.Listen, entry.Direction); err != nil {
		return err
	}
	switch entry.Direction {
	case workshop.HostToWorkshop:
		if err := expandProxyPath(&entry.Listen, spec.User); err != nil {
			return err
		}
		if err := authorizePath(entry.Listen, spec.User); err != nil {
			return err
		}
		if err := expandProxyPath(&entry.Connect, &workshop.User); err != nil {
			return err
		}
	case workshop.WorkshopToHost:
		if err := expandProxyPath(&entry.Listen, &workshop.User); err != nil {
			return err
		}
		if err := expandProxyPath(&entry.Connect, spec.User); err != nil {
			return err
		}
	}

	return spec.AddTunnelEntry(workshop.Tunnel{ProxyEntry: entry})
}

func checkProtocols(listen, connect string) error {
	switch {
	case listen == "udp" && connect != "udp":
	case listen != "udp" && connect == "udp":
	default:
		return nil
	}
	return fmt.Errorf("%s and %s are incompatible", listen, connect)
}

func inferPorts(listen, connect *workshop.ProxyTarget) error {
	var connectHost, connectPort string
	needConnectPort := false
	if isIP(connect.Protocol) {
		connectHost, connectPort = parseAddress(connect.Address)
		needConnectPort = connectPort == ""
	}

	if !isIP(listen.Protocol) {
		if needConnectPort {
			return errors.New("both ports unspecified")
		}
		return nil
	}

	listenHost, listenPort := parseAddress(listen.Address)

	// Infer missing port from peer.
	if listenPort == "" {
		if connectPort == "" {
			return errors.New("both ports unspecified")
		}
		listen.Address = net.JoinHostPort(listenHost, connectPort)
	} else if needConnectPort {
		connect.Address = net.JoinHostPort(connectHost, listenPort)
	}

	return nil
}

func checkListenPort(listen workshop.ProxyTarget, direction workshop.ProxyDirection) error {
	if !isIP(listen.Protocol) {
		return nil
	}

	_, listenPort := parseAddress(listen.Address)

	port, err := strconv.ParseUint(listenPort, 10, 16)
	if err != nil {
		return err
	}

	if port == 0 {
		// TODO: Add support for binding port 0
		// if LXD adds a way to query the assigned port.
		return fmt.Errorf("port %v not currently supported", port)
	}

	if direction == workshop.HostToWorkshop && port < 1024 {
		// Avoid bypassing CAP_NET_BIND_SERVICE (for simplicity,
		// we assume the end user doesn't have this capability).
		return fmt.Errorf("port %v is privileged", port)
	}

	return nil
}

func expandProxyPath(target *workshop.ProxyTarget, user *user.User) error {
	if target.Protocol != "unix" {
		return nil
	}

	var err error
	target.Address = os.Expand(target.Address, func(s string) string {
		switch s {
		case "HOME":
			return user.HomeDir
		case "XDG_RUNTIME_DIR":
			return filepath.Join(dirs.XdgRuntimeDirBase, user.Uid)
		case "$":
			// Unescape $$ -> $.
			return "$"
		default:
			err = fmt.Errorf("unexpected variable %q", s)
			return ""
		}
	})

	return err
}

// authorizePath attempts to ensure that the user is allowed to create the given socket.
// The parent directory has to exist for LXD to create the socket,
// so we can evaluate symlinks to ensure it belongs to $HOME or $XDG_RUNTIME_DIR.
// FIXME: this doesn't take bind mounts into account.
func authorizePath(listen workshop.ProxyTarget, user *user.User) error {
	if listen.Protocol != "unix" || strings.HasPrefix(listen.Address, "@") {
		return nil
	}

	realDir, err := filepath.EvalSymlinks(filepath.Dir(listen.Address))
	if err != nil {
		return fmt.Errorf("cannot create socket %q: %w", listen.Address, err)
	}

	if realDir == user.HomeDir || strings.HasPrefix(realDir, user.HomeDir+"/") {
		return nil
	}
	runtimeDir := filepath.Join(dirs.XdgRuntimeDirBase, user.Uid)
	if realDir == runtimeDir || strings.HasPrefix(realDir, runtimeDir+"/") {
		return nil
	}

	return fmt.Errorf("user %q cannot create socket %q for security reasons", user.Username, listen.Address)
}

func init() {
	registerIface(&tunnelInterface{})
}
