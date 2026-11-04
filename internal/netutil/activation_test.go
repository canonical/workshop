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

	"github.com/canonical/workshop/internal/testutil"
)

func Test(t *testing.T) { check.TestingT(t) }

type activationSuite struct{}

var _ = check.Suite(&activationSuite{})

func (s *activationSuite) TestCreateOrTakeListenerSocketActivated(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	defer l.Close()

	listenerMap := map[Addr]net.Listener{newAddr(l.Addr()): l}
	ln, err := CreateOrTakeListener(sock, listenerMap)
	c.Assert(err, check.IsNil)
	c.Check(ln, check.Equals, l)
	c.Check(listenerMap, check.HasLen, 0)
}

func (s *activationSuite) TestCreateOrTakeListenerNew(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")

	listenerMap := map[Addr]net.Listener{Addr{Network: "GNU", Address: sock}: nil}
	ln, err := CreateOrTakeListener(sock, listenerMap)
	c.Assert(err, check.IsNil)
	defer ln.Close()

	c.Check(newAddr(ln.Addr()), check.Equals, Addr{Network: "unix", Address: sock})
	c.Check(listenerMap, check.HasLen, 1)

	info, err := os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)
	c.Check(info.Mode().Perm(), check.Equals, os.FileMode(0666))
}

func (s *activationSuite) TestCreateOrTakeListenerStale(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	l.SetUnlinkOnClose(false)
	l.Close()

	info, err := os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)

	ln, err := CreateOrTakeListener(sock, nil)
	c.Assert(err, check.IsNil)
	defer ln.Close()

	c.Check(newAddr(ln.Addr()), check.Equals, Addr{Network: "unix", Address: sock})

	info, err = os.Stat(sock)
	c.Assert(err, check.IsNil)
	c.Check(info.Mode()&os.ModeSocket, check.Equals, os.ModeSocket)
	c.Check(info.Mode().Perm(), check.Equals, os.FileMode(0666))
}

func (s *activationSuite) TestCreateOrTakeListenerConflict(c *check.C) {
	sock := filepath.Join(c.MkDir(), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	c.Assert(err, check.IsNil)
	defer l.Close()

	_, err = CreateOrTakeListener(sock, nil)
	c.Check(err, check.ErrorMatches, `socket ".*/sock" already in use`)
}

func (s *activationSuite) TestCreateOrTakeVsockListenerSocketActivated(c *check.C) {
	l, err := vsock.ListenContextID(unix.VMADDR_CID_ANY, unix.VMADDR_PORT_ANY, nil)
	if err != nil {
		c.Skip("vsock not available: " + err.Error())
	}
	defer l.Close()
	addr := l.Addr().(*vsock.Addr)

	listenerMap := map[Addr]net.Listener{newAddr(addr): l}
	ln, err := CreateOrTakeVsockListener(addr.ContextID, addr.Port, listenerMap)
	c.Assert(err, check.IsNil)
	c.Check(ln, check.Equals, l)
	c.Check(listenerMap, check.HasLen, 0)
}

func (s *activationSuite) TestCreateOrTakeVsockListenerNew(c *check.C) {
	l, err := vsock.ListenContextID(unix.VMADDR_CID_ANY, unix.VMADDR_PORT_ANY, nil)
	if err != nil {
		c.Skip("vsock not available: " + err.Error())
	}
	l.Close()

	ln, err := CreateOrTakeVsockListener(unix.VMADDR_CID_ANY, unix.VMADDR_PORT_ANY, nil)
	c.Assert(err, check.IsNil)
	defer ln.Close()
	addr := newAddr(ln.Addr())
	c.Check(addr.Network, check.Equals, "vsock")
	c.Check(addr.Address, check.Matches, `vm\(\d+\):\d+`)
}

func (s *activationSuite) TestActivationListenersUnix(c *check.C) {
	socks := []string{filepath.Join(c.MkDir(), "trusted"), filepath.Join(c.MkDir(), "untrusted")}
	files := make([]*os.File, 0, len(socks))
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, sock := range socks {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
		c.Assert(err, check.IsNil)
		defer l.Close()

		f, err := l.File()
		c.Assert(err, check.IsNil)
		files = append(files, f)
	}

	lns, err := ActivationListeners(files)
	c.Assert(err, check.IsNil)
	for _, ln := range lns {
		defer ln.Close()
	}
	c.Assert(lns, check.HasLen, 2)

	for _, sock := range socks {
		addr := Addr{Network: "unix", Address: sock}
		c.Assert(lns[addr], check.NotNil)
		c.Check(newAddr(lns[addr].Addr()), check.Equals, addr)
	}
}

func (s *activationSuite) TestActivationListenersVsock(c *check.C) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		c.Skip("vsock not available: " + err.Error())
	}
	f := os.NewFile(uintptr(fd), "vsock")
	defer f.Close()

	err = unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: unix.VMADDR_PORT_ANY})
	c.Assert(err, check.IsNil)
	err = unix.Listen(fd, 1)
	c.Assert(err, check.IsNil)

	lns, err := ActivationListeners([]*os.File{f})
	c.Assert(err, check.IsNil)
	for _, ln := range lns {
		defer ln.Close()
	}
	c.Assert(lns, check.HasLen, 1)

	for _, ln := range lns {
		addr := newAddr(ln.Addr())
		c.Check(addr.Network, check.Equals, "vsock")
		c.Check(addr.Address, check.Matches, `vm\(\d+\):\d+`)
	}
}

func (s *activationSuite) TestActivationListenersNotSocket(c *check.C) {
	f, err := os.Open(os.DevNull)
	c.Assert(err, check.IsNil)
	defer f.Close()

	_, err = ActivationListeners([]*os.File{f})
	c.Check(err, testutil.ErrorIs, unix.ENOTSOCK)
}
