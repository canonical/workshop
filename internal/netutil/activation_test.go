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

package netutil

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/mdlayher/vsock"
	"golang.org/x/sys/unix"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/testutil"
)

func Test(t *testing.T) { check.TestingT(t) }

type activationSuite struct{}

var _ = check.Suite(&activationSuite{})

func listenVsock(c *check.C) *vsock.Listener {
	l, err := vsock.ListenContextID(unix.VMADDR_CID_ANY, unix.VMADDR_PORT_ANY, nil)
	if err != nil {
		c.Skip("vsock not available: " + err.Error())
	}
	return l
}

func (s *activationSuite) TestGetListenerSocketActivated(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	defer l.Close()

	ln, err := GetListener(sock, map[Addr]net.Listener{newAddr(l.Addr()): l})
	c.Assert(err, check.IsNil)
	c.Check(ln, check.Equals, l)
}

func (s *activationSuite) TestGetListenerNew(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")

	ln, err := GetListener(sock, nil)
	c.Assert(err, check.IsNil)
	defer ln.Close()

	c.Check(newAddr(ln.Addr()), check.Equals, Addr{Network: "unix", Address: sock})

	info, err := os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)
	c.Check(info.Mode().Perm(), check.Equals, os.FileMode(0666))
}

func (s *activationSuite) TestGetListenerStale(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	l.SetUnlinkOnClose(false)
	l.Close()

	info, err := os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)

	ln, err := GetListener(sock, nil)
	c.Assert(err, check.IsNil)
	defer ln.Close()

	c.Check(newAddr(ln.Addr()), check.Equals, Addr{Network: "unix", Address: sock})

	info, err = os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)
	c.Check(info.Mode().Perm(), check.Equals, os.FileMode(0666))
}

func (s *activationSuite) TestGetListenerConflict(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	defer l.Close()

	_, err = GetListener(sock, nil)
	c.Check(err, check.ErrorMatches, `socket ".*/sock" already in use`)
}

func (s *activationSuite) TestGetVsockListenerSocketActivated(c *check.C) {
	l := listenVsock(c)
	defer l.Close()
	addr := l.Addr().(*vsock.Addr)

	ln, err := GetVsockListener(addr.ContextID, addr.Port, map[Addr]net.Listener{newAddr(addr): l})
	c.Assert(err, check.IsNil)
	c.Check(ln, check.Equals, l)
}

func (s *activationSuite) TestGetVsockListenerNew(c *check.C) {
	listenVsock(c).Close()

	ln, err := GetVsockListener(unix.VMADDR_CID_ANY, unix.VMADDR_PORT_ANY, nil)
	c.Assert(err, check.IsNil)
	defer ln.Close()
	c.Check(ln, check.FitsTypeOf, &vsock.Listener{})
}

func (s *activationSuite) TestActivationListenersUnix(c *check.C) {
	rev := revert.New()
	defer rev.Fail()

	socks := []string{filepath.Join(c.MkDir(), "trusted"), filepath.Join(c.MkDir(), "untrusted")}
	files := make([]*os.File, 0, len(socks))
	rev.Add(func() {
		for _, f := range files {
			f.Close()
		}
	})
	for _, sock := range socks {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
		c.Assert(err, check.IsNil)
		defer l.Close()

		f, err := l.File()
		c.Assert(err, check.IsNil)
		files = append(files, f)
	}

	defer testutil.FakeFunc(func(bool) []*os.File { return files }, &activationFiles)()
	rev.Success()

	lns, err := ActivationListeners()
	c.Assert(err, check.IsNil)
	for _, ln := range lns {
		defer ln.Close()
	}
	c.Assert(lns, check.HasLen, 2)
	for _, f := range files {
		c.Check(f.Close(), testutil.ErrorIs, os.ErrClosed)
	}

	for _, sock := range socks {
		addr := Addr{Network: "unix", Address: sock}
		c.Assert(lns[addr], check.NotNil)
		c.Check(newAddr(lns[addr].Addr()), check.Equals, addr)
	}
}

func (s *activationSuite) TestActivationListenersVsock(c *check.C) {
	rev := revert.New()
	defer rev.Fail()

	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		c.Skip("vsock not available: " + err.Error())
	}
	f := os.NewFile(uintptr(fd), "vsock")
	rev.Add(func() {
		f.Close()
	})

	err = unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: unix.VMADDR_PORT_ANY})
	c.Assert(err, check.IsNil)
	err = unix.Listen(fd, 1)
	c.Assert(err, check.IsNil)

	defer testutil.FakeFunc(func(bool) []*os.File { return []*os.File{f} }, &activationFiles)()
	rev.Success()

	lns, err := ActivationListeners()
	c.Assert(err, check.IsNil)
	for _, ln := range lns {
		defer ln.Close()
	}
	c.Assert(lns, check.HasLen, 1)
	c.Check(f.Close(), testutil.ErrorIs, os.ErrClosed)

	for _, ln := range lns {
		addr := newAddr(ln.Addr())
		c.Check(addr.Network, check.Equals, "vsock")
		c.Check(addr.Address, check.Matches, `vm\(\d+\):\d+`)
	}
}

func (s *activationSuite) TestActivationListenersNotSocket(c *check.C) {
	f, err := os.Open(os.DevNull)
	c.Assert(err, check.IsNil)

	defer testutil.FakeFunc(func(bool) []*os.File { return []*os.File{f} }, &activationFiles)()

	_, err = ActivationListeners()
	c.Check(err, testutil.ErrorIs, unix.ENOTSOCK)
	c.Check(f.Close(), testutil.ErrorIs, os.ErrClosed)
}
