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
	"strconv"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/systemd"
)

// Timeout bounds how long the boot may make no observable progress, not how
// long it may take in total. A slow but advancing boot is never interrupted.
// The host overrides this via WORKSHOP_WAITREADY_TIMEOUT_NS; it only applies
// when waitready is run by hand.
var Timeout = 10 * time.Minute

// IsWaitreadyInvocation reports whether the process was invoked via a symlink
// named waitready. This allows multiple logically unrelated commands to be
// embedded in a single multi-call binary (even in tests).
func IsWaitreadyInvocation() bool {
	return len(os.Args) > 0 && filepath.Base(os.Args[0]) == "waitready"
}

// WaitReady waits for the system to finish booting and then sets the instance
// to Ready via the DevLXD socket.
func WaitReady() error {
	timeout := Timeout
	if timeoutSetting := os.Getenv("WORKSHOP_WAITREADY_TIMEOUT_NS"); timeoutSetting != "" {
		timeoutNS, err := strconv.ParseInt(timeoutSetting, 10, 64)
		if err != nil {
			return err
		}
		timeout = time.Duration(timeoutNS) * time.Nanosecond
	}

	// The deadline is enforced per stall rather than over the whole boot, so
	// it cannot be applied to the context. A hung connect is instead bounded
	// by systemd, which kills the unit if it never reports READY=1.
	ctx, cancel := context.WithCancel(context.Background())
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

	if err := server.UpdateState(api.DevLXDPut{State: api.Started.String()}); err != nil {
		return err
	}

	if err := waitReady(ctx, timeout); err != nil {
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

func waitReady(ctx context.Context, timeout time.Duration) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	// JobRemoved arrives once per unit that finishes starting, so it is a
	// steady stream while the boot advances and stops when it stalls. The
	// buffer is sized for that stream: godbus delivers overflow from a new
	// goroutine per signal rather than dropping it.
	signal := make(chan *dbus.Signal, 256)
	conn.Signal(signal)

	for _, member := range []string{"StartupFinished", "JobRemoved"} {
		options := []dbus.MatchOption{
			dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
			dbus.WithMatchMember(member),
		}
		if err := conn.AddMatchSignalContext(ctx, options...); err != nil {
			return err
		}
	}

	manager := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
	if err := manager.CallWithContext(ctx, "org.freedesktop.systemd1.Manager.Subscribe", 0).Err; err != nil {
		return err
	}

	ready, err := isReady(manager)
	if err != nil {
		return err
	}

	if err := maybeSdNotifyReady(); err != nil {
		return err
	}

	if ready {
		return nil
	}

	stall := time.NewTimer(timeout)
	defer stall.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signal:
			if sig == nil {
				return errors.New("bus connection closed unexpectedly")
			}
			switch sig.Name {
			case "org.freedesktop.systemd1.Manager.StartupFinished":
				return nil
			case "org.freedesktop.systemd1.Manager.JobRemoved":
				stall.Reset(timeout)
			}
		case <-stall.C:
			// Signals are queued, so systemd may have finished booting while
			// this one was still behind a backlog of JobRemoved signals.
			ready, err := isReady(manager)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
			return fmt.Errorf("boot made no progress for %s", timeout)
		}
	}
}

func isReady(manager dbus.BusObject) (bool, error) {
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
