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
	"fmt"
	"net"
	"sync"
	"syscall"
)

const (
	ucrednetNobody = uint32((1 << 32) - 1)

	contextUcrednet = contextKeyUcrednet("ucrednet")
)

type contextKeyUcrednet string

type ucrednet struct {
	pid int32
	uid uint32
	// trusted indicates whether the request came directly from the user, or
	// another entity (e.g. a workshop) owned by the user.
	trusted bool
}

type ucrednetConn struct {
	net.Conn
	ucrednet
}

type ucrednetListener struct {
	net.Listener

	trusted bool

	idempotClose sync.Once
	closeErr     error
}

var getUcred = syscall.GetsockoptUcred

func (wl *ucrednetListener) Accept() (net.Conn, error) {
	con, err := wl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	ucon, ok := con.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("internal error: cannot get peer credentials of %T", con)
	}
	rawConn, err := ucon.SyscallConn()
	if err != nil {
		return nil, err
	}
	var ucred *syscall.Ucred
	var ucredErr error
	// Call getUcred inside a Control() block to ensure fd is valid for
	// the duration of the call, avoiding a race condition.
	err = rawConn.Control(func(fd uintptr) {
		ucred, ucredErr = getUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return nil, err
	}
	if ucredErr != nil {
		return nil, ucredErr
	}

	return &ucrednetConn{con, ucrednet{
		pid:     ucred.Pid,
		uid:     ucred.Uid,
		trusted: wl.trusted,
	}}, nil
}

func (wl *ucrednetListener) Close() error {
	wl.idempotClose.Do(func() {
		wl.closeErr = wl.Listener.Close()
	})
	return wl.closeErr
}
