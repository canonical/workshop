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

package waitready

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/systemd"
)

var Timeout = 5 * time.Minute

// IsWaitreadyInvocation reports whether the process was invoked via a symlink
// named waitready. This allows multiple logically unrelated commands to be
// embedded in a single multi-call binary (even in tests).
func IsWaitreadyInvocation() bool {
	return len(os.Args) > 0 && filepath.Base(os.Args[0]) == "waitready"
}

// WaitReady waits for the system to finish booting and then sets the instance
// to Ready via the DevLXD socket.
func WaitReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	server, err := lxd.ConnectDevLXDWithContext(ctx, "/dev/lxd/sock", nil)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "nothing to do: %v\n", err)
		return maybeSdNotifyReady()
	}
	if err != nil {
		return err
	}
	defer server.Disconnect()

	if err := waitReady(ctx); err != nil {
		return err
	}

	return server.UpdateState(api.DevLXDPut{State: api.Ready.String()})
}

func maybeSdNotifyReady() error {
	if !systemd.SocketAvailable() {
		return nil
	}
	return systemd.SdNotify("READY=1")
}

func waitReady(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	signal := make(chan *dbus.Signal, 8)
	conn.Signal(signal)

	options := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
		dbus.WithMatchMember("StartupFinished"),
	}
	if err := conn.AddMatchSignalContext(ctx, options...); err != nil {
		return err
	}

	ready, err := isReady(conn)
	if err != nil {
		return err
	}

	if err := maybeSdNotifyReady(); err != nil {
		return err
	}

	if ready {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signal:
			if sig == nil {
				return errors.New("bus connection closed unexpectedly")
			}
			if sig.Name != "org.freedesktop.systemd1.Manager.StartupFinished" {
				continue
			}
			return nil
		}
	}
}

func isReady(conn *dbus.Conn) (bool, error) {
	manager := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
	variant, err := manager.GetProperty("org.freedesktop.systemd1.Manager.SystemState")
	if err != nil {
		return false, err
	}

	var state string
	if err := variant.Store(&state); err != nil {
		return false, err
	}

	// Based on `systemctl is-system-running --wait`.
	return state != "initializing" && state != "starting", nil
}
