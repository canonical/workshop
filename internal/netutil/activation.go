// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2015-2019 Canonical Ltd
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

package netutil

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/coreos/go-systemd/activation"
	"github.com/mdlayher/vsock"
	"golang.org/x/sys/unix"

	"github.com/canonical/workshop/internal/logger"
)

var activationFiles = activation.Files

// Addr uniquely identifies a listener address.
type Addr struct {
	Network string
	Address string
}

func newAddr(addr net.Addr) Addr {
	return Addr{Network: addr.Network(), Address: addr.String()}
}

// GetListener tries to get a listener for the given socket path from
// the listener map, and if it fails it tries to set it up directly.
func GetListener(socketPath string, listenerMap map[Addr]net.Listener) (net.Listener, error) {
	address, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, err
	}

	if listener, ok := listenerMap[newAddr(address)]; ok {
		return listener, nil
	}

	c, err := net.DialUnix("unix", nil, address)
	if err == nil {
		c.Close()
		return nil, fmt.Errorf("socket %q already in use", socketPath)
	}

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	runtime.LockOSThread()
	oldmask := unix.Umask(0111)
	listener, err := net.ListenUnix("unix", address)
	unix.Umask(oldmask)
	runtime.UnlockOSThread()
	if err != nil {
		return nil, err
	}

	logger.Debugf("socket %q was not activated; listening", socketPath)

	return listener, nil
}

// GetVsockListener tries to get a listener for the given vsock address from
// the listener map, and if it fails it tries to set it up directly.
func GetVsockListener(contextID, port uint32, listenerMap map[Addr]net.Listener) (net.Listener, error) {
	addr := &vsock.Addr{ContextID: contextID, Port: port}
	if listener, ok := listenerMap[newAddr(addr)]; ok {
		return listener, nil
	}

	listener, err := vsock.ListenContextID(contextID, port, nil)
	if err != nil {
		return nil, err
	}

	logger.Debugf("socket %q was not activated; listening", addr)

	return listener, nil
}

// GetVsockGuestListener attempts to listen to VM guests on the given port. If
// the vhost_vsock kernel module isn't loaded, loads it first. If that fails,
// and limitToGuests is false, it binds to the wildcard address.
func GetVsockGuestListener(port uint32, limitToGuests bool, listenerMap map[Addr]net.Listener) (net.Listener, error) {
	listener, err := GetVsockListener(unix.VMADDR_CID_HOST, port, listenerMap)
	if !errors.Is(err, unix.EADDRNOTAVAIL) {
		return listener, err
	}

	f, err1 := os.OpenFile("/dev/vhost-vsock", os.O_RDWR, 0)
	if err1 == nil {
		f.Close()
		return GetVsockListener(unix.VMADDR_CID_HOST, port, nil)
	}
	if !limitToGuests {
		logger.Noticef("Cannot listen on vsock host context ID, listening on any: %v", err1)
		return GetVsockListener(unix.VMADDR_CID_ANY, port, nil)
	}

	return listener, err
}

// ActivationListeners builds a map of addresses to listeners that were passed
// during systemd activation
func ActivationListeners() (lns map[Addr]net.Listener, err error) {
	// pass false to keep LISTEN_* environment variables passed by systemd
	files := activationFiles(false)
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()

	lns = make(map[Addr]net.Listener, len(files))
	for _, f := range files {
		ln, err := fileListener(f)
		if err != nil {
			return nil, err
		}
		addr := newAddr(ln.Addr())
		lns[addr] = ln
	}
	return lns, nil
}

// fileListener wraps net.FileListener with support for vsock.
func fileListener(f *os.File) (net.Listener, error) {
	ln, err := net.FileListener(f)
	if !errors.Is(err, unix.EAFNOSUPPORT) {
		return ln, err
	}

	return vsock.FileListener(f)
}
