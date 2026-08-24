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

package dns

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/juju/clock"
	"github.com/juju/retry"
)

type systemdResolved struct{}

type resolvedAddress struct {
	Family  int32
	Address []byte
}

type resolvedDomain struct {
	Domain    string
	RouteOnly bool
}

func NewSystemdResolved() Resolver {
	return &systemdResolved{}
}

// ConfigureDNS points the given interface's DNS servers at addrs and registers
// domain as a routing-only search domain, replacing any previously configured
// values. It mirrors the following commands:
// - resolvectl dns <iface> <addr>...
// - resolvectl domain <iface> ~<domain>
// - resolvectl dnssec <iface> no
// - resolvectl dnsovertls <iface> no
// The interface's kernel index is resolved once for all calls.
func (r *systemdResolved) ConfigureDNS(ctx context.Context, iface string, addrs []netip.Addr, domain string) error {
	index, err := r.interfaceIndex(iface)
	if err != nil {
		return err
	}
	return r.configureDNS(ctx, index, addrs, domain)
}

func (r *systemdResolved) configureDNS(ctx context.Context, index int32, addrs []netip.Addr, domain string) error {
	addresses := r.marshalAddrs(addrs)
	domains := []resolvedDomain{{Domain: domain, RouteOnly: true}}

	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("cannot connect to system bus: %w", err)
	}
	defer conn.Close()

	// Retry a few times if the interface isn't found. In practice this is
	// very unlikely, because systemd-resolved uses a single-threaded event
	// loop. The D-Bus calls would have to arrive ahead of the netlink event.
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			return r.dispatch(ctx, conn, index, addresses, domains)
		},
		IsFatalError: func(err error) bool {
			dbusErr, ok := errors.AsType[dbus.Error](err)
			return !ok || dbusErr.Name != "org.freedesktop.resolve1.NoSuchLink"
		},
		Attempts:    5,
		Delay:       100 * time.Millisecond,
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})
	if retry.IsRetryStopped(err) {
		err = ctx.Err()
	}
	return err
}

func (*systemdResolved) interfaceIndex(name string) (int32, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, fmt.Errorf("cannot find interface %q", name)
	}
	if iface.Index < math.MinInt32 || iface.Index > math.MaxInt32 {
		return 0, fmt.Errorf("invalid interface index %v", iface.Index)
	}
	return int32(iface.Index), nil
}

func (*systemdResolved) marshalAddrs(addrs []netip.Addr) []resolvedAddress {
	addresses := make([]resolvedAddress, 0, len(addrs))
	for _, a := range addrs {
		family := int32(syscall.AF_INET)
		if a.Is6() {
			family = syscall.AF_INET6
		}
		addresses = append(addresses, resolvedAddress{Family: family, Address: a.AsSlice()})
	}
	return addresses
}

func (*systemdResolved) dispatch(ctx context.Context, conn *dbus.Conn, index int32, addresses []resolvedAddress, domains []resolvedDomain) error {
	manager := conn.Object("org.freedesktop.resolve1", "/org/freedesktop/resolve1")
	if call := manager.CallWithContext(ctx, "org.freedesktop.resolve1.Manager.SetLinkDNS", 0, index, addresses); call.Err != nil {
		return call.Err
	}
	if call := manager.CallWithContext(ctx, "org.freedesktop.resolve1.Manager.SetLinkDomains", 0, index, domains); call.Err != nil {
		return call.Err
	}
	if call := manager.CallWithContext(ctx, "org.freedesktop.resolve1.Manager.SetLinkDNSOverTLS", 0, index, "no"); call.Err != nil {
		return call.Err
	}
	if call := manager.CallWithContext(ctx, "org.freedesktop.resolve1.Manager.SetLinkDNSSEC", 0, index, "no"); call.Err != nil {
		return call.Err
	}
	return nil
}
