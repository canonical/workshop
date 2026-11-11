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
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

// renderedCloudConfig is the part of the rendered cloud-init user-data that
// configures the proxies.
type renderedCloudConfig struct {
	Apt struct {
		HTTPProxy string `yaml:"http_proxy"`
		Conf      string `yaml:"conf"`
	} `yaml:"apt"`
	WriteFiles []struct {
		Path    string `yaml:"path"`
		Content string `yaml:"content"`
		Append  bool   `yaml:"append"`
	} `yaml:"write_files"`
	Runcmd []string `yaml:"runcmd"`
}

// writeFile returns the content and the append flag of the file at path
// that the cloud-config writes, and whether it writes one.
func (cfg *renderedCloudConfig) writeFile(path string) (content string, appended, ok bool) {
	for _, f := range cfg.WriteFiles {
		if f.Path == path {
			return f.Content, f.Append, true
		}
	}
	return "", false, false
}

// runcmdIndex returns the index of the first runcmd entry containing cmd,
// or -1.
func (cfg *renderedCloudConfig) runcmdIndex(cmd string) int {
	for i, c := range cfg.Runcmd {
		if strings.Contains(c, cmd) {
			return i
		}
	}
	return -1
}

type proxySuite struct {
	testutil.BaseTest

	bridgeQueries int
	bridge        []netip.Addr
	bridgeErr     error
}

var _ = check.Suite(&proxySuite{})

func (s *proxySuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)

	s.bridgeQueries = 0
	s.bridge = []netip.Addr{netip.MustParseAddr("fd42:4c81:5a1e:6a2b::1"), netip.MustParseAddr("10.153.172.1")}
	s.bridgeErr = nil

	oldAptProxy, oldSnapStoreURL, oldGoProxy := aptProxy, snapStoreURL, goProxy
	aptProxy, snapStoreURL, goProxy = "", "", ""
	oldBridgeAddrs := bridgeAddrs
	bridgeAddrs = func(ctx context.Context) ([]netip.Addr, error) {
		s.bridgeQueries++
		return s.bridge, s.bridgeErr
	}
	s.AddCleanup(func() {
		aptProxy, snapStoreURL, goProxy = oldAptProxy, oldSnapStoreURL, oldGoProxy
		bridgeAddrs = oldBridgeAddrs
	})
}

func (s *proxySuite) TestParseAptProxy(c *check.C) {
	for _, t := range []struct {
		raw, url string
	}{
		{"http://192.168.0.51:3142", "http://192.168.0.51:3142"},
		{"http://192.168.0.51:3142/", "http://192.168.0.51:3142"},
		{"http://proxy.lan:3142", "http://proxy.lan:3142"},
		{"http://proxy.lan", "http://proxy.lan"},
		{"http://[fd00::51]:3142", "http://[fd00::51]:3142"},
		{"http://127.0.0.1:3142", "http://127.0.0.1:3142"},
	} {
		u, err := parseAptProxy(t.raw)
		c.Assert(err, check.IsNil, check.Commentf("%q", t.raw))
		c.Check(u.String(), check.Equals, t.url, check.Commentf("%q", t.raw))
	}

	for _, t := range []struct {
		raw, err string
	}{
		{"https://proxy.lan:3142", `invalid apt proxy "https://proxy.lan:3142": scheme must be "http"`},
		{"proxy.lan:3142", `invalid apt proxy "proxy.lan:3142": scheme must be "http"`},
		{"192.168.0.51:3142", `invalid apt proxy "192.168.0.51:3142": .*`},
		{"http://:3142", `invalid apt proxy "http://:3142": no host`},
		{"http:///apt", `invalid apt proxy "http:///apt": no host`},
		{"http://user:secret@proxy.lan:3142", `invalid apt proxy "http://user:secret@proxy.lan:3142": user information not supported`},
		{"http://proxy.lan:3142/apt", `invalid apt proxy "http://proxy.lan:3142/apt": path not supported`},
		{"http://proxy.lan:3142?x=1", `invalid apt proxy "http://proxy.lan:3142\?x=1": query and fragment not supported`},
		{"http://proxy.lan:3142#x", `invalid apt proxy "http://proxy.lan:3142#x": query and fragment not supported`},
		{"http://proxy.lan:0", `invalid apt proxy "http://proxy.lan:0": invalid port "0"`},
		{"http://proxy.lan:65536", `invalid apt proxy "http://proxy.lan:65536": invalid port "65536"`},
		{"http://proxy.lan:", `invalid apt proxy "http://proxy.lan:": empty port`},
		{"http://proxy.lan:http", `invalid apt proxy "http://proxy.lan:http": .*`},
		{"http://proxy lan:3142", `invalid apt proxy "http://proxy lan:3142": .*`},
	} {
		_, err := parseAptProxy(t.raw)
		c.Check(err, check.ErrorMatches, t.err, check.Commentf("%q", t.raw))
	}
}

func (s *proxySuite) TestParseSnapStoreURL(c *check.C) {
	for _, t := range []struct {
		raw, url string
	}{
		{"http://192.168.0.51:3142", "http://192.168.0.51:3142"},
		{"http://192.168.0.51:3142/", "http://192.168.0.51:3142"},
		{"https://store.lan", "https://store.lan"},
		{"https://store.lan:8443/api/", "https://store.lan:8443/api"},
		{"http://[fd00::51]:3142/snap", "http://[fd00::51]:3142/snap"},
		{"http://localhost:3142", "http://localhost:3142"},
	} {
		u, err := parseSnapStoreURL(t.raw)
		c.Assert(err, check.IsNil, check.Commentf("%q", t.raw))
		c.Check(u.String(), check.Equals, t.url, check.Commentf("%q", t.raw))
	}

	for _, t := range []struct {
		raw, err string
	}{
		{"ftp://store.lan", `invalid snap store URL "ftp://store.lan": scheme must be "http" or "https"`},
		{"store.lan", `invalid snap store URL "store.lan": scheme must be "http" or "https"`},
		{"https://", `invalid snap store URL "https://": no host`},
		{"https://user@store.lan", `invalid snap store URL "https://user@store.lan": user information not supported`},
		{"https://store.lan/?x=1", `invalid snap store URL "https://store.lan/\?x=1": query and fragment not supported`},
		{"https://store.lan:0", `invalid snap store URL "https://store.lan:0": invalid port "0"`},
		{"https://store.lan:", `invalid snap store URL "https://store.lan:": empty port`},
	} {
		_, err := parseSnapStoreURL(t.raw)
		c.Check(err, check.ErrorMatches, t.err, check.Commentf("%q", t.raw))
	}
}

func (s *proxySuite) TestParseGoProxy(c *check.C) {
	for _, t := range []struct {
		raw, list string
	}{
		{"direct", "direct"},
		{"off", "off"},
		{"http://192.168.0.51:3142/go", "http://192.168.0.51:3142/go"},
		{"http://192.168.0.51:3142/go/,direct", "http://192.168.0.51:3142/go,direct"},
		{"https://proxy.golang.org,direct", "https://proxy.golang.org,direct"},
		{"http://goproxy.lan|https://proxy.golang.org,off", "http://goproxy.lan|https://proxy.golang.org,off"},
		{"http://[::1]:3142/go,direct", "http://[::1]:3142/go,direct"},
	} {
		entries, err := parseGoProxy(t.raw)
		c.Assert(err, check.IsNil, check.Commentf("%q", t.raw))
		c.Check(formatGoProxy(entries), check.Equals, t.list, check.Commentf("%q", t.raw))
	}

	for _, t := range []struct {
		raw, err string
	}{
		{",", `invalid Go proxy ",": empty entry`},
		{"direct,", `invalid Go proxy "direct,": empty entry`},
		{"http://goproxy.lan,,direct", `invalid Go proxy "http://goproxy.lan,,direct": empty entry`},
		{"noproxy", `invalid Go proxy "noproxy": entry "noproxy": scheme must be "http" or "https"`},
		{"goproxy.lan,direct", `invalid Go proxy "goproxy.lan,direct": entry "goproxy.lan": scheme must be "http" or "https"`},
		{"file:///srv/go", `invalid Go proxy "file:///srv/go": entry "file:///srv/go": scheme must be "http" or "https"`},
		{"direct|http://user:pw@goproxy.lan", `invalid Go proxy "direct\|http://user:pw@goproxy.lan": entry "http://user:pw@goproxy.lan": user information not supported`},
		{"http://goproxy.lan/go?x=1", `invalid Go proxy "http://goproxy.lan/go\?x=1": entry "http://goproxy.lan/go\?x=1": query and fragment not supported`},
		{"http://goproxy.lan:99999", `invalid Go proxy "http://goproxy.lan:99999": entry "http://goproxy.lan:99999": invalid port "99999"`},
		{"direct off", `invalid Go proxy "direct off": entry "direct off": .*`},
	} {
		_, err := parseGoProxy(t.raw)
		c.Check(err, check.ErrorMatches, t.err, check.Commentf("%q", t.raw))
	}
}

func (s *proxySuite) TestIsLocalHost(c *check.C) {
	for _, host := range []string{"localhost", "LocalHost", "proxy.localhost", "127.0.0.1", "127.0.1.1", "::1", "0.0.0.0", "::"} {
		c.Check(isLocalHost(host), check.Equals, true, check.Commentf("%q", host))
	}
	for _, host := range []string{"proxy.lan", "192.168.0.51", "10.153.172.1", "fd00::51", "localhost.lan", ""} {
		c.Check(isLocalHost(host), check.Equals, false, check.Commentf("%q", host))
	}
}

func (s *proxySuite) TestURLOnBridge(c *check.C) {
	v4 := netip.MustParseAddr("10.153.172.1")
	v6 := netip.MustParseAddr("fd42:4c81:5a1e:6a2b::1")

	for _, t := range []struct {
		addrs []netip.Addr
		in    string
		url   string
	}{
		// IPv4 is preferred, whatever the order.
		{[]netip.Addr{v6, v4}, "http://127.0.0.1:3142", "http://10.153.172.1:3142"},
		{[]netip.Addr{v4, v6}, "http://127.0.0.1:3142", "http://10.153.172.1:3142"},
		{[]netip.Addr{v6}, "http://127.0.0.1:3142", "http://[fd42:4c81:5a1e:6a2b::1]:3142"},
		{[]netip.Addr{v4}, "http://localhost", "http://10.153.172.1"},
		{[]netip.Addr{v6}, "http://localhost", "http://[fd42:4c81:5a1e:6a2b::1]"},
		{[]netip.Addr{{}, v6}, "http://127.0.0.1:3142", "http://[fd42:4c81:5a1e:6a2b::1]:3142"},
		// The path is kept.
		{[]netip.Addr{v4}, "https://localhost:8443/api", "https://10.153.172.1:8443/api"},
	} {
		in, err := url.Parse(t.in)
		c.Assert(err, check.IsNil)
		u, err := urlOnBridge(in, t.addrs)
		c.Assert(err, check.IsNil, check.Commentf("%v %s", t.addrs, t.in))
		c.Check(u.String(), check.Equals, t.url, check.Commentf("%v %s", t.addrs, t.in))
		c.Check(in.String(), check.Equals, t.in, check.Commentf("%v %s", t.addrs, t.in))
	}

	in, err := url.Parse("http://127.0.0.1:3142")
	c.Assert(err, check.IsNil)
	_, err = urlOnBridge(in, nil)
	c.Check(err, check.ErrorMatches, `cannot use "http://127.0.0.1:3142" in workshops: network "workshopbr0" has no address`)
}

func (s *proxySuite) TestWorkshopProxiesUnset(c *check.C) {
	proxies, err := workshopProxies(context.Background())
	c.Assert(err, check.IsNil)
	c.Check(proxies, check.Equals, proxyConfig{})
	c.Check(s.bridgeQueries, check.Equals, 0)
}

func (s *proxySuite) TestWorkshopProxiesRemote(c *check.C) {
	aptProxy = "http://192.168.0.51:3142/"
	snapStoreURL = "https://store.lan/"
	goProxy = "https://goproxy.lan/go/|direct"

	proxies, err := workshopProxies(context.Background())
	c.Assert(err, check.IsNil)
	c.Check(proxies, check.Equals, proxyConfig{
		AptProxy:     "http://192.168.0.51:3142",
		SnapStoreURL: "https://store.lan",
		GoProxy:      "https://goproxy.lan/go|direct",
	})
	c.Check(s.bridgeQueries, check.Equals, 0)
}

func (s *proxySuite) TestWorkshopProxiesLocal(c *check.C) {
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]", "0.0.0.0", "[::]"} {
		aptProxy = "http://" + host + ":3142"
		snapStoreURL = "http://" + host + ":3142"
		goProxy = "http://" + host + ":3142/go,https://proxy.golang.org|http://" + host + ":3000,direct"

		proxies, err := workshopProxies(context.Background())
		c.Assert(err, check.IsNil, check.Commentf("%q", host))
		c.Check(proxies, check.Equals, proxyConfig{
			AptProxy:     "http://10.153.172.1:3142",
			SnapStoreURL: "http://10.153.172.1:3142",
			GoProxy:      "http://10.153.172.1:3142/go,https://proxy.golang.org|http://10.153.172.1:3000,direct",
		}, check.Commentf("%q", host))
	}
	// The host's address on the workshop network is looked up once per
	// launch.
	c.Check(s.bridgeQueries, check.Equals, 5)
}

func (s *proxySuite) TestWorkshopProxiesLocalEach(c *check.C) {
	for _, t := range []struct {
		apt, snap, goProxy string
		proxies            proxyConfig
	}{
		{apt: "http://localhost:3142", proxies: proxyConfig{AptProxy: "http://10.153.172.1:3142"}},
		{snap: "http://localhost:3142", proxies: proxyConfig{SnapStoreURL: "http://10.153.172.1:3142"}},
		{goProxy: "http://localhost:3142/go,direct", proxies: proxyConfig{GoProxy: "http://10.153.172.1:3142/go,direct"}},
	} {
		aptProxy, snapStoreURL, goProxy = t.apt, t.snap, t.goProxy

		proxies, err := workshopProxies(context.Background())
		c.Assert(err, check.IsNil)
		c.Check(proxies, check.Equals, t.proxies)
	}
	c.Check(s.bridgeQueries, check.Equals, 3)
}

func (s *proxySuite) TestWorkshopProxiesLocalNoBridge(c *check.C) {
	s.bridgeErr = errors.New(`cannot get network "workshopbr0": Network not found`)

	for _, t := range []struct {
		apt, snap, goProxy string
		err                string
	}{
		{apt: "http://localhost:3142", err: `cannot use "http://localhost:3142" in workshops: cannot get network "workshopbr0": Network not found`},
		{snap: "http://localhost:3142/snap", err: `cannot use "http://localhost:3142/snap" in workshops: cannot get network "workshopbr0": Network not found`},
		{goProxy: "direct|http://localhost:3142/go", err: `cannot use "http://localhost:3142/go" in workshops: cannot get network "workshopbr0": Network not found`},
	} {
		aptProxy, snapStoreURL, goProxy = t.apt, t.snap, t.goProxy

		_, err := workshopProxies(context.Background())
		c.Check(err, check.ErrorMatches, t.err)
	}
}

func (s *proxySuite) TestWorkshopProxiesInvalid(c *check.C) {
	for _, t := range []struct {
		apt, snap, goProxy string
		err                string
	}{
		{apt: "https://localhost:3142", err: `invalid apt proxy "https://localhost:3142": scheme must be "http"`},
		{snap: "localhost:3142", err: `invalid snap store URL "localhost:3142": scheme must be "http" or "https"`},
		{goProxy: "http://localhost:3142/go,", err: `invalid Go proxy "http://localhost:3142/go,": empty entry`},
	} {
		aptProxy, snapStoreURL, goProxy = t.apt, t.snap, t.goProxy

		_, err := workshopProxies(context.Background())
		c.Check(err, check.ErrorMatches, t.err)
	}
	c.Check(s.bridgeQueries, check.Equals, 0)
}

func (s *proxySuite) TestLoadEnvConfigProxies(c *check.C) {
	defer FakeImageServer(imageServer)()
	defer FakeImageRegistry(imageRegistry)()

	setenv(&s.BaseTest, "WORKSHOP_APT_PROXY", "http://127.0.0.1:3142")
	setenv(&s.BaseTest, "WORKSHOP_SNAP_STORE_URL", "http://127.0.0.1:3142")
	setenv(&s.BaseTest, "WORKSHOP_GO_PROXY", "http://127.0.0.1:3142/go,direct")
	loadEnvConfig()
	c.Check(aptProxy, check.Equals, "http://127.0.0.1:3142")
	c.Check(snapStoreURL, check.Equals, "http://127.0.0.1:3142")
	c.Check(goProxy, check.Equals, "http://127.0.0.1:3142/go,direct")

	setenv(&s.BaseTest, "WORKSHOP_APT_PROXY", "")
	setenv(&s.BaseTest, "WORKSHOP_SNAP_STORE_URL", "")
	setenv(&s.BaseTest, "WORKSHOP_GO_PROXY", "")
	loadEnvConfig()
	c.Check(aptProxy, check.Equals, "")
	c.Check(snapStoreURL, check.Equals, "")
	c.Check(goProxy, check.Equals, "")
}

// renderCloudConfig renders the cloud-config of a workshop of the given
// runtime that uses proxies.
func renderCloudConfig(c *check.C, runtime workshop.Runtime, proxies proxyConfig) (string, *renderedCloudConfig) {
	b := &Backend{}
	file := &workshop.File{Name: "test", Base: "ubuntu@24.04", Runtime: runtime}

	cfg, err := b.workshopConfig("42ws42ws", "1000", "1000", file, b.FormatRevision(), "fakeimage12345", proxies)
	c.Assert(err, check.IsNil)

	userData := cfg["cloud-init.user-data"]
	var parsed renderedCloudConfig
	c.Assert(yaml.Unmarshal([]byte(userData), &parsed), check.IsNil, check.Commentf("%s", userData))
	return userData, &parsed
}

const (
	testAptProxy     = "http://10.153.172.1:3142"
	testSnapStoreURL = "http://10.153.172.1:3142"
	testGoProxy      = "http://10.153.172.1:3142/go,direct"

	snapdStoreDropIn = "/etc/systemd/system/snapd.service.d/70-workshop-store.conf"
	archivesRewrite  = `s#https://(archive|security|ports|old-releases)\.ubuntu\.com#http://\1.ubuntu.com#g`
)

var proxyRuntimes = []workshop.Runtime{workshop.RuntimeLXDContainer, workshop.RuntimeLXDVM}

func (s *proxySuite) TestCloudConfigWithoutProxies(c *check.C) {
	for _, runtime := range proxyRuntimes {
		comment := check.Commentf("runtime %v", runtime)
		userData, cfg := renderCloudConfig(c, runtime, proxyConfig{})

		c.Check(cfg.Apt.HTTPProxy, check.Equals, "", comment)
		c.Check(cfg.Apt.Conf, testutil.Contains, `APT::Install-Recommends "0";`, comment)
		_, _, ok := cfg.writeFile(snapdStoreDropIn)
		c.Check(ok, check.Equals, false, comment)
		_, _, ok = cfg.writeFile("/etc/environment")
		c.Check(ok, check.Equals, false, comment)
		c.Check(cfg.runcmdIndex(archivesRewrite), check.Equals, -1, comment)
		c.Check(cfg.runcmdIndex("systemctl try-restart snapd.service"), check.Equals, -1, comment)
		c.Check(userData, check.Not(testutil.Contains), "SNAPPY_FORCE_API_URL", comment)
		c.Check(userData, check.Not(testutil.Contains), "GOPROXY", comment)
	}
}

func (s *proxySuite) TestCloudConfigWithAptProxy(c *check.C) {
	for _, runtime := range proxyRuntimes {
		comment := check.Commentf("runtime %v", runtime)
		userData, cfg := renderCloudConfig(c, runtime, proxyConfig{AptProxy: testAptProxy})

		c.Check(cfg.Apt.HTTPProxy, check.Equals, testAptProxy, comment)
		c.Check(cfg.Apt.Conf, testutil.Contains, `APT::Install-Recommends "0";`, comment)

		// Ubuntu's archives are made http://, before the workshop reports
		// that it is ready, which is when Workshop runs the SDKs' hooks.
		rewrite := cfg.runcmdIndex(archivesRewrite)
		ready := cfg.runcmdIndex("systemctl enable --now workshop-waitready.service")
		c.Check(rewrite >= 0, check.Equals, true, comment)
		c.Check(ready > rewrite, check.Equals, true, comment)

		// Nothing else is configured.
		_, _, ok := cfg.writeFile(snapdStoreDropIn)
		c.Check(ok, check.Equals, false, comment)
		_, _, ok = cfg.writeFile("/etc/environment")
		c.Check(ok, check.Equals, false, comment)
		c.Check(cfg.runcmdIndex("systemctl try-restart snapd.service"), check.Equals, -1, comment)
		c.Check(userData, check.Not(testutil.Contains), "SNAPPY_FORCE_API_URL", comment)
		c.Check(userData, check.Not(testutil.Contains), "GOPROXY", comment)
	}
}

func (s *proxySuite) TestCloudConfigWithSnapStoreURL(c *check.C) {
	for _, runtime := range proxyRuntimes {
		comment := check.Commentf("runtime %v", runtime)
		userData, cfg := renderCloudConfig(c, runtime, proxyConfig{SnapStoreURL: testSnapStoreURL})

		content, appended, ok := cfg.writeFile(snapdStoreDropIn)
		c.Assert(ok, check.Equals, true, comment)
		c.Check(appended, check.Equals, false, comment)
		c.Check(content, check.Equals, "[Service]\nEnvironment=SNAPPY_FORCE_API_URL=http://10.153.172.1:3142\n", comment)

		// "snap download", for hooks, commands and SSH sessions
		content, appended, ok = cfg.writeFile("/etc/environment")
		c.Assert(ok, check.Equals, true, comment)
		c.Check(appended, check.Equals, true, comment)
		c.Check(content, check.Equals, "SNAPPY_FORCE_API_URL=http://10.153.172.1:3142\n", comment)

		// The existing snapd drop-in is still there.
		_, _, ok = cfg.writeFile("/etc/systemd/system/snapd.service.d/70-snapd-after-network.conf")
		c.Check(ok, check.Equals, true, comment)

		// snapd is restarted before the workshop reports that it is ready.
		reload := cfg.runcmdIndex("systemctl daemon-reload")
		restart := cfg.runcmdIndex("systemctl try-restart snapd.service")
		ready := cfg.runcmdIndex("systemctl enable --now workshop-waitready.service")
		c.Check(reload >= 0, check.Equals, true, comment)
		c.Check(restart > reload, check.Equals, true, comment)
		c.Check(ready > restart, check.Equals, true, comment)

		// Nothing else is configured.
		c.Check(cfg.Apt.HTTPProxy, check.Equals, "", comment)
		c.Check(cfg.runcmdIndex(archivesRewrite), check.Equals, -1, comment)
		c.Check(userData, check.Not(testutil.Contains), "GOPROXY", comment)
	}
}

func (s *proxySuite) TestCloudConfigWithSnapStoreURLEscaped(c *check.C) {
	_, cfg := renderCloudConfig(c, workshop.RuntimeLXDContainer, proxyConfig{SnapStoreURL: "https://store.lan/a%20b"})

	content, _, ok := cfg.writeFile(snapdStoreDropIn)
	c.Assert(ok, check.Equals, true)
	c.Check(content, check.Equals, "[Service]\nEnvironment=SNAPPY_FORCE_API_URL=https://store.lan/a%%20b\n")
	content, _, ok = cfg.writeFile("/etc/environment")
	c.Assert(ok, check.Equals, true)
	c.Check(content, check.Equals, "SNAPPY_FORCE_API_URL=https://store.lan/a%20b\n")
}

func (s *proxySuite) TestCloudConfigWithGoProxy(c *check.C) {
	for _, runtime := range proxyRuntimes {
		comment := check.Commentf("runtime %v", runtime)
		userData, cfg := renderCloudConfig(c, runtime, proxyConfig{GoProxy: testGoProxy})

		// go, for hooks, commands and SSH sessions
		content, appended, ok := cfg.writeFile("/etc/environment")
		c.Assert(ok, check.Equals, true, comment)
		c.Check(appended, check.Equals, true, comment)
		c.Check(content, check.Equals, "GOPROXY=http://10.153.172.1:3142/go,direct\n", comment)

		// Nothing else is configured.
		c.Check(cfg.Apt.HTTPProxy, check.Equals, "", comment)
		_, _, ok = cfg.writeFile(snapdStoreDropIn)
		c.Check(ok, check.Equals, false, comment)
		c.Check(cfg.runcmdIndex(archivesRewrite), check.Equals, -1, comment)
		c.Check(cfg.runcmdIndex("systemctl try-restart snapd.service"), check.Equals, -1, comment)
		c.Check(userData, check.Not(testutil.Contains), "SNAPPY_FORCE_API_URL", comment)
	}
}

func (s *proxySuite) TestCloudConfigWithAllProxies(c *check.C) {
	for _, runtime := range proxyRuntimes {
		comment := check.Commentf("runtime %v", runtime)
		_, cfg := renderCloudConfig(c, runtime, proxyConfig{
			AptProxy:     testAptProxy,
			SnapStoreURL: testSnapStoreURL,
			GoProxy:      testGoProxy,
		})

		c.Check(cfg.Apt.HTTPProxy, check.Equals, testAptProxy, comment)

		content, _, ok := cfg.writeFile(snapdStoreDropIn)
		c.Assert(ok, check.Equals, true, comment)
		c.Check(content, check.Equals, "[Service]\nEnvironment=SNAPPY_FORCE_API_URL=http://10.153.172.1:3142\n", comment)

		content, appended, ok := cfg.writeFile("/etc/environment")
		c.Assert(ok, check.Equals, true, comment)
		c.Check(appended, check.Equals, true, comment)
		c.Check(content, check.Equals, "SNAPPY_FORCE_API_URL=http://10.153.172.1:3142\nGOPROXY=http://10.153.172.1:3142/go,direct\n", comment)

		rewrite := cfg.runcmdIndex(archivesRewrite)
		reload := cfg.runcmdIndex("systemctl daemon-reload")
		restart := cfg.runcmdIndex("systemctl try-restart snapd.service")
		ready := cfg.runcmdIndex("systemctl enable --now workshop-waitready.service")
		c.Check(rewrite >= 0, check.Equals, true, comment)
		c.Check(reload > rewrite, check.Equals, true, comment)
		c.Check(restart > reload, check.Equals, true, comment)
		c.Check(ready > restart, check.Equals, true, comment)
	}
}

func (s *proxySuite) TestCloudConfigWithIPv6Proxies(c *check.C) {
	const ipv6Proxy = "http://[fd42:4c81:5a1e:6a2b::1]:3142"
	_, cfg := renderCloudConfig(c, workshop.RuntimeLXDContainer, proxyConfig{
		AptProxy:     ipv6Proxy,
		SnapStoreURL: ipv6Proxy,
		GoProxy:      ipv6Proxy + "/go,direct",
	})

	c.Check(cfg.Apt.HTTPProxy, check.Equals, ipv6Proxy)
	content, _, ok := cfg.writeFile("/etc/environment")
	c.Assert(ok, check.Equals, true)
	c.Check(content, check.Equals, "SNAPPY_FORCE_API_URL="+ipv6Proxy+"\nGOPROXY="+ipv6Proxy+"/go,direct\n")
}

func (s *proxySuite) TestHashSnapshotProxies(c *check.C) {
	b := &Backend{}
	snapshot := workshop.Snapshot{
		Format: sdk.R(14),
		Image: workshop.BaseImage{
			Name:        "ubuntu@24.04",
			Runtime:     workshop.RuntimeLXDContainer,
			Fingerprint: testNobleContainerFP,
		},
		Sdks: []sdk.ContentID{{
			Name:     "go",
			Sha3_384: strings.Repeat("ab", 48),
			IsVolume: true,
		}},
	}
	hash := func() string {
		h, err := b.HashSnapshot(snapshot)
		c.Assert(err, check.IsNil)
		return h
	}

	// With no proxy configured, the hash is the one Workshop computed
	// before it supported proxies, so existing snapshots are still used.
	withoutProxies := hash()
	c.Check(withoutProxies, check.Equals, "af5cac1b416bd3e840319051557214123c185f2321ffe72a89eabebcb4431d9e5df3dac9d397e2730739acc03d9a3ca4")

	// Snapshots made with a proxy are never used without it, with another
	// one, or with the same value given to another option.
	seen := map[string]string{withoutProxies: "no proxies"}
	for _, t := range []struct {
		apt, snap, goProxy string
	}{
		{apt: "http://127.0.0.1:3142"},
		{apt: "http://192.168.0.51:3142"},
		{snap: "http://127.0.0.1:3142"},
		{snap: "http://192.168.0.51:3142"},
		{goProxy: "http://127.0.0.1:3142"},
		{goProxy: "http://127.0.0.1:3142/go,direct"},
		{apt: "http://127.0.0.1:3142", snap: "http://127.0.0.1:3142"},
		{apt: "http://127.0.0.1:3142", snap: "http://127.0.0.1:3142", goProxy: "http://127.0.0.1:3142/go,direct"},
	} {
		aptProxy, snapStoreURL, goProxy = t.apt, t.snap, t.goProxy
		h := hash()
		desc := fmt.Sprintf("%+v", t)
		c.Check(seen[h], check.Equals, "", check.Commentf("%s and %s", desc, seen[h]))
		seen[h] = desc
	}

	aptProxy, snapStoreURL, goProxy = "", "", ""
	c.Check(hash(), check.Equals, withoutProxies)
}
