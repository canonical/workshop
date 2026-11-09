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

package lxdbackend

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

var (
	// aptProxy is the HTTP proxy, http://<HOST>[:<PORT>], that apt in
	// workshops downloads through, as the user configured it, or empty
	// for none.
	aptProxy = ""

	// snapStoreURL is the URL of the snap store API that snapd in
	// workshops uses instead of the default one, as the user configured
	// it, or empty for the default.
	snapStoreURL = ""

	// goProxy is the GOPROXY list that go in workshops downloads modules
	// through, as the user configured it, or empty for go's default.
	goProxy = ""

	// bridgeAddrs returns the addresses of the host on the workshop
	// network.
	bridgeAddrs = func(ctx context.Context) ([]netip.Addr, error) {
		return NewNetworkManager().InterfaceAddrs(ctx, networkName)
	}
)

// proxyConfig holds the proxies that a workshop downloads packages, snaps
// and Go modules through, as the workshop reaches them. An empty field
// leaves the default of the corresponding tool in place.
type proxyConfig struct {
	// AptProxy is the HTTP proxy of apt, http://<HOST>[:<PORT>].
	AptProxy string

	// SnapStoreURL is the URL of the snap store API that snapd uses.
	SnapStoreURL string

	// GoProxy is the GOPROXY list of go.
	GoProxy string
}

// parseProxyURL parses raw as an absolute URL with one of the given schemes
// and a host, and with no user information, query or fragment. A path is
// accepted only when withPath is true; a trailing slash is dropped.
func parseProxyURL(raw string, withPath bool, schemes ...string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(schemes, u.Scheme) {
		quoted := make([]string, len(schemes))
		for i, scheme := range schemes {
			quoted[i] = strconv.Quote(scheme)
		}
		return nil, fmt.Errorf("scheme must be %s", strings.Join(quoted, " or "))
	}

	if u.Hostname() == "" {
		return nil, fmt.Errorf("no host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("user information not supported")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, fmt.Errorf("query and fragment not supported")
	}
	if !withPath && u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("path not supported")
	}

	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port %q", port)
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("empty port")
	}

	parsed := &url.URL{Scheme: u.Scheme, Host: u.Host}
	if withPath {
		parsed.Path = strings.TrimSuffix(u.Path, "/")
	}
	return parsed, nil
}

// parseAptProxy parses the HTTP proxy of apt, http://<HOST>[:<PORT>]. apt
// uses an HTTP proxy for http:// sources only.
func parseAptProxy(raw string) (*url.URL, error) {
	u, err := parseProxyURL(raw, false, "http")
	if err != nil {
		return nil, fmt.Errorf("invalid apt proxy %q: %w", raw, err)
	}
	return u, nil
}

// parseSnapStoreURL parses the URL of a snap store API,
// http[s]://<HOST>[:<PORT>][/<PATH>], as snapd reads it from
// SNAPPY_FORCE_API_URL.
func parseSnapStoreURL(raw string) (*url.URL, error) {
	u, err := parseProxyURL(raw, true, "http", "https")
	if err != nil {
		return nil, fmt.Errorf("invalid snap store URL %q: %w", raw, err)
	}
	return u, nil
}

// goProxyEntry is one entry of a GOPROXY list.
type goProxyEntry struct {
	// Keyword is "direct" or "off", or empty for a URL.
	Keyword string

	// URL is the URL of a module proxy, or nil for a keyword.
	URL *url.URL

	// Sep is the separator between this entry and the next one, "," or
	// "|", or empty for the last entry.
	Sep string
}

// parseGoProxy parses a GOPROXY list: module proxy URLs,
// http[s]://<HOST>[:<PORT>][/<PATH>], and the keywords "direct" and "off",
// separated by commas or pipes, which tell go when to fall back to the
// next entry.
func parseGoProxy(raw string) ([]goProxyEntry, error) {
	var entries []goProxyEntry
	rest := raw
	for {
		var entry goProxyEntry
		value := rest
		if i := strings.IndexAny(rest, ",|"); i >= 0 {
			value, entry.Sep, rest = rest[:i], rest[i:i+1], rest[i+1:]
		}

		switch value {
		case "":
			return nil, fmt.Errorf("invalid Go proxy %q: empty entry", raw)
		case "direct", "off":
			entry.Keyword = value
		default:
			u, err := parseProxyURL(value, true, "http", "https")
			if err != nil {
				return nil, fmt.Errorf("invalid Go proxy %q: entry %q: %w", raw, value, err)
			}
			entry.URL = u
		}

		entries = append(entries, entry)
		if entry.Sep == "" {
			return entries, nil
		}
	}
}

// formatGoProxy returns the GOPROXY list of entries.
func formatGoProxy(entries []goProxyEntry) string {
	var b strings.Builder
	for _, entry := range entries {
		if entry.URL != nil {
			b.WriteString(entry.URL.String())
		} else {
			b.WriteString(entry.Keyword)
		}
		b.WriteString(entry.Sep)
	}
	return b.String()
}

// isLocalHost reports whether host names whichever machine it is looked up
// on: a workshop would reach itself, not the host, there.
func isLocalHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsLoopback() || addr.IsUnspecified()
}

// urlOnBridge returns u with its host replaced by the host's address on the
// workshop network, one of addrs. IPv4 is preferred: the workshop network
// may have no IPv6 routes.
func urlOnBridge(u *url.URL, addrs []netip.Addr) (*url.URL, error) {
	var addr netip.Addr
	for _, a := range addrs {
		if !a.IsValid() {
			continue
		}
		if a.Is4() {
			addr = a
			break
		}
		if !addr.IsValid() {
			addr = a
		}
	}
	if !addr.IsValid() {
		return nil, fmt.Errorf("cannot use %q in workshops: network %q has no address", u.String(), networkName)
	}

	host := addr.String()
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	} else if addr.Is6() {
		host = "[" + host + "]"
	}

	onBridge := *u
	onBridge.Host = host
	return &onBridge, nil
}

// bridgeResolver replaces the host of URLs that name the host itself
// (localhost, a loopback or an unspecified address) with the host's address
// on the workshop network, which it looks up at most once.
type bridgeResolver struct {
	addrs  []netip.Addr
	err    error
	looked bool
}

// resolve returns u as workshops reach it.
func (r *bridgeResolver) resolve(ctx context.Context, u *url.URL) (*url.URL, error) {
	if !isLocalHost(u.Hostname()) {
		return u, nil
	}

	if !r.looked {
		r.addrs, r.err = bridgeAddrs(ctx)
		r.looked = true
	}
	if r.err != nil {
		return nil, fmt.Errorf("cannot use %q in workshops: %w", u.String(), r.err)
	}
	return urlOnBridge(u, r.addrs)
}

// workshopProxies returns the configured proxies as workshops reach them.
func workshopProxies(ctx context.Context) (proxyConfig, error) {
	var proxies proxyConfig
	var resolver bridgeResolver

	if aptProxy != "" {
		u, err := parseAptProxy(aptProxy)
		if err != nil {
			return proxyConfig{}, err
		}
		u, err = resolver.resolve(ctx, u)
		if err != nil {
			return proxyConfig{}, err
		}
		proxies.AptProxy = u.String()
	}

	if snapStoreURL != "" {
		u, err := parseSnapStoreURL(snapStoreURL)
		if err != nil {
			return proxyConfig{}, err
		}
		u, err = resolver.resolve(ctx, u)
		if err != nil {
			return proxyConfig{}, err
		}
		proxies.SnapStoreURL = u.String()
	}

	if goProxy != "" {
		entries, err := parseGoProxy(goProxy)
		if err != nil {
			return proxyConfig{}, err
		}
		for i, entry := range entries {
			if entry.URL == nil {
				continue
			}
			entries[i].URL, err = resolver.resolve(ctx, entry.URL)
			if err != nil {
				return proxyConfig{}, err
			}
		}
		proxies.GoProxy = formatGoProxy(entries)
	}

	return proxies, nil
}
