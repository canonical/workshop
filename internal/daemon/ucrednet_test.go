// Copyright (c) 2014-2020 Canonical Ltd
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

package daemon

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	sys "syscall"

	"gopkg.in/check.v1"
)

type ucrednetSuite struct {
	ucred *sys.Ucred
	err   error
}

var _ = check.Suite(&ucrednetSuite{})

func (s *ucrednetSuite) getUcred(fd, level, opt int) (*sys.Ucred, error) {
	return s.ucred, s.err
}

func (s *ucrednetSuite) SetUpSuite(c *check.C) {
	getUcred = s.getUcred
}

func (s *ucrednetSuite) TearDownTest(c *check.C) {
	s.ucred = nil
	s.err = nil
}
func (s *ucrednetSuite) TearDownSuite(c *check.C) {
	getUcred = sys.GetsockoptUcred
}

func (s *ucrednetSuite) TestAcceptConnContext(c *check.C) {
	s.ucred = &sys.Ucred{Pid: 100, Uid: 42}
	d := c.MkDir()
	sock := filepath.Join(d, "sock")

	l, err := net.Listen("unix", sock)
	c.Assert(err, check.IsNil)
	wl := &ucrednetListener{Listener: l, trusted: true}
	defer wl.Close()

	go func() {
		cli, err := net.Dial("unix", sock)
		c.Assert(err, check.IsNil)
		cli.Close()
	}()

	conn, err := wl.Accept()
	c.Assert(err, check.IsNil)

	ctx := connContext(context.Background(), conn)
	cred, ok := ctx.Value(contextUcrednet).(ucrednet)
	c.Assert(ok, check.Equals, true)
	c.Check(cred.pid, check.Equals, int32(100))
	c.Check(cred.uid, check.Equals, uint32(42))
	c.Check(cred.trusted, check.Equals, true)
}

func (s *ucrednetSuite) TestAcceptConnContextUntrusted(c *check.C) {
	s.ucred = &sys.Ucred{Pid: 100, Uid: 42}
	d := c.MkDir()
	sock := filepath.Join(d, "sock")

	l, err := net.Listen("unix", sock)
	c.Assert(err, check.IsNil)
	wl := &ucrednetListener{Listener: l, trusted: false}
	defer wl.Close()

	go func() {
		cli, err := net.Dial("unix", sock)
		c.Assert(err, check.IsNil)
		cli.Close()
	}()

	conn, err := wl.Accept()
	c.Assert(err, check.IsNil)

	ctx := connContext(context.Background(), conn)
	cred, ok := ctx.Value(contextUcrednet).(ucrednet)
	c.Assert(ok, check.Equals, true)
	c.Check(cred.pid, check.Equals, int32(100))
	c.Check(cred.uid, check.Equals, uint32(42))
	c.Check(cred.trusted, check.Equals, false)
}

func (s *ucrednetSuite) TestNonUnix(c *check.C) {
	l, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, check.IsNil)

	wl := &ucrednetListener{Listener: l, trusted: true}
	defer wl.Close()

	addr := l.Addr().String()

	go func() {
		cli, err := net.Dial("tcp", addr)
		c.Assert(err, check.IsNil)
		cli.Close()
	}()

	_, err = wl.Accept()
	c.Assert(err, check.ErrorMatches, "internal error: cannot get peer credentials of \\*net.TCPConn")
}

func (s *ucrednetSuite) TestNonUcrednetListener(c *check.C) {
	conn, peer := net.Pipe()
	peer.Close()
	defer conn.Close()

	_, ok := connContext(context.Background(), conn).Value(contextUcrednet).(ucrednet)
	c.Check(ok, check.Equals, false)
}

func (s *ucrednetSuite) TestAcceptErrors(c *check.C) {
	s.ucred = &sys.Ucred{Pid: 100, Uid: 42}
	d := c.MkDir()
	sock := filepath.Join(d, "sock")

	l, err := net.Listen("unix", sock)
	c.Assert(err, check.IsNil)
	c.Assert(l.Close(), check.IsNil)

	wl := &ucrednetListener{Listener: l, trusted: true}

	_, err = wl.Accept()
	c.Assert(err, check.NotNil)
}

func (s *ucrednetSuite) TestUcredErrors(c *check.C) {
	s.err = errors.New("oopsie")
	d := c.MkDir()
	sock := filepath.Join(d, "sock")

	l, err := net.Listen("unix", sock)
	c.Assert(err, check.IsNil)

	wl := &ucrednetListener{Listener: l, trusted: true}
	defer wl.Close()

	go func() {
		cli, err := net.Dial("unix", sock)
		c.Assert(err, check.IsNil)
		cli.Close()
	}()

	_, err = wl.Accept()
	c.Assert(err, check.Equals, s.err)
}

func (s *ucrednetSuite) TestIdempotentClose(c *check.C) {
	s.ucred = &sys.Ucred{Pid: 100, Uid: 42}
	d := c.MkDir()
	sock := filepath.Join(d, "sock")

	l, err := net.Listen("unix", sock)
	c.Assert(err, check.IsNil)
	wl := &ucrednetListener{Listener: l, trusted: true}

	c.Assert(wl.Close(), check.IsNil)
	c.Assert(wl.Close(), check.IsNil)
}
