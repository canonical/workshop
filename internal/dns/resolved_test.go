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

//go:build integration

package dns

import (
	"context"
	"fmt"
	"math"
	"net/netip"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type resolvedSuite struct {
	iface  string
	addrs  []netip.Addr
	domain string
}

var _ = check.Suite(&resolvedSuite{})

func (s *resolvedSuite) SetUpTest(c *check.C) {
	s.iface = "testbr0"
	s.addrs = []netip.Addr{netip.MustParseAddr("10.42.42.1")}
	s.domain = "test"

	_, err := exec.Command("ip", "link", "add", "name", s.iface, "type", "bridge").Output()
	c.Assert(err, check.IsNil)
}

func (s *resolvedSuite) TearDownTest(c *check.C) {
	_, err := exec.Command("ip", "link", "delete", "dev", s.iface).Output()
	c.Check(err, check.IsNil)
}

func (s *resolvedSuite) TestConfigureOK(c *check.C) {
	out, err := exec.Command("resolvectl", "dnssec", s.iface, "yes").Output()
	c.Assert(err, check.IsNil, check.Commentf("%s", out))

	out, err = exec.Command("resolvectl", "dnsovertls", s.iface, "yes").Output()
	c.Assert(err, check.IsNil, check.Commentf("%s", out))

	err = NewSystemdResolved().ConfigureDNS(context.Background(), s.iface, s.addrs, s.domain)
	c.Assert(err, check.IsNil)

	// Support for resolvectl status --json was added in systemd v259. For now
	// we rely on textual output.
	out, err = exec.Command("resolvectl", "dns", s.iface).Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Matches, `Link \d+ \(testbr0\): 10\.42\.42\.1\n`)

	out, err = exec.Command("resolvectl", "domain", s.iface).Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Matches, `Link \d+ \(testbr0\): ~test\n`)

	out, err = exec.Command("resolvectl", "dnssec", s.iface).Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Matches, `Link \d+ \(testbr0\): no\n`)

	out, err = exec.Command("resolvectl", "dnsovertls", s.iface).Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Matches, `Link \d+ \(testbr0\): no\n`)
}

func (s *resolvedSuite) TestInterfaceNotFound(c *check.C) {
	err := NewSystemdResolved().ConfigureDNS(context.Background(), "testbr1", s.addrs, s.domain)
	c.Assert(err, check.ErrorMatches, `cannot find interface "testbr1"`)
}

func (s *resolvedSuite) TestDBusNotAvailable(c *check.C) {
	bus, ok := os.LookupEnv("DBUS_SYSTEM_BUS_ADDRESS")
	if ok {
		defer os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", bus)
	} else {
		defer os.Unsetenv("DBUS_SYSTEM_BUS_ADDRESS")
	}
	os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/dev/null")

	err := NewSystemdResolved().ConfigureDNS(context.Background(), s.iface, s.addrs, s.domain)
	c.Assert(err, check.ErrorMatches, "cannot connect to system bus: dial unix /dev/null: connect: connection refused")
}

func (s *resolvedSuite) TestResolvedNotAvailable(c *check.C) {
	_, err := exec.Command("systemctl", "stop", "systemd-resolved.service").Output()
	c.Assert(err, check.IsNil)
	defer func() {
		_, err1 := exec.Command("systemctl", "start", "systemd-resolved.service").Output()
		c.Check(err1, check.IsNil)
	}()

	_, err = exec.Command("systemctl", "mask", "systemd-resolved.service").Output()
	c.Assert(err, check.IsNil)
	defer func() {
		_, err1 := exec.Command("systemctl", "unmask", "systemd-resolved.service").Output()
		c.Check(err1, check.IsNil)
	}()

	err = NewSystemdResolved().ConfigureDNS(context.Background(), s.iface, s.addrs, s.domain)
	c.Assert(err, check.NotNil)
}

// Simulate a scenario in which systemd-resolved receives the D-Bus calls
// before the RTM_NEWLINK event.
func (s *resolvedSuite) TestConfigureWaitsForInterfaceRegistration(c *check.C) {
	var wg sync.WaitGroup
	added := false
	defer func() {
		if added {
			_, err1 := exec.Command("ip", "link", "delete", "dev", "testbr2").Output()
			c.Check(err1, check.IsNil)
		}
	}()

	wg.Go(func() {
		time.Sleep(50 * time.Millisecond)
		_, err := exec.Command("ip", "link", "add", "name", "testbr2", "index", fmt.Sprint(math.MaxInt32), "type", "bridge").Output()
		c.Assert(err, check.IsNil)
		added = true
	})

	err := (&systemdResolved{}).configureDNS(context.Background(), math.MaxInt32, s.addrs, s.domain)
	c.Assert(err, check.IsNil)
	wg.Wait()

	out, err := exec.Command("resolvectl", "domain", "testbr2").Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Matches, `Link \d+ \(testbr2\): ~test\n`)
}
