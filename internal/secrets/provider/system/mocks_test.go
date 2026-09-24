// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package system

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
)

// fakeBusConnection records D-Bus calls without a real session bus.
type fakeBusConnection struct {
	call   func(context.Context, dbus.ObjectPath, string, []any, ...any) error
	closed bool
}

// fakeExecCommand runs only the test binary, or fails before process start.
// Arguments carry responses because ExecService deliberately clears Env.
type fakeExecCommand struct {
	args       []string
	cmd        *exec.Cmd
	executable string
	exitCode   int
	path       string
	startErr   error
	stderr     string
	stdout     string
}

// secretService delegates secret retrieval to a test-defined function.
type secretService func(
	context.Context,
	Request,
) (secrets.Secret, error)

// secretSlotLookup delegates slot lookup to a test-defined function.
type secretSlotLookup func(
	context.Context,
	sdk.SlotRef,
) (SecretSlotConfig, error)

// slotRepository records repository requests and returns a configured slot.
type slotRepository struct {
	refs []sdk.SlotRef
	slot *sdk.SlotInfo
}

func (c *fakeBusConnection) Call(
	ctx context.Context,
	path dbus.ObjectPath,
	method string,
	args []any,
	results ...any,
) error {
	return c.call(ctx, path, method, args, results...)
}

func (c *fakeBusConnection) Close() error {
	c.closed = true
	return nil
}

// Command records command selection and supplies a controlled subprocess.
func (f *fakeExecCommand) Command(
	ctx context.Context,
	name string,
	args ...string,
) *exec.Cmd {
	f.executable = name
	f.args = args
	f.cmd = exec.CommandContext(
		ctx,
		f.path,
		"-test.run=^TestExecCommandHelper$",
		"--",
		"--exec-service-helper",
		f.stdout,
		f.stderr,
		strconv.Itoa(f.exitCode),
	)
	f.cmd.Err = f.startErr
	return f.cmd
}

// TestExecCommandHelper supplies protocol output without running command main.
// Ordinary test runs do nothing; the parent selects helper mode explicitly.
func TestExecCommandHelper(t *testing.T) {
	if len(os.Args) != 7 || os.Args[3] != "--exec-service-helper" {
		return
	}
	_, err := io.Copy(io.Discard, os.Stdin)
	if err != nil {
		os.Exit(120)
	}
	code, err := strconv.Atoi(os.Args[6])
	if err != nil {
		os.Exit(121)
	}
	_, err = io.WriteString(os.Stdout, os.Args[4])
	if err != nil {
		os.Exit(122)
	}
	_, err = io.WriteString(os.Stderr, os.Args[5])
	if err != nil {
		os.Exit(123)
	}
	// Do not let the testing runner append PASS to the protocol response.
	os.Exit(code)
}

// Get calls the test-defined secret retrieval function.
func (s secretService) Get(
	ctx context.Context,
	request Request,
) (secrets.Secret, error) {
	return s(ctx, request)
}

// Lookup calls the test-defined lookup function.
func (l secretSlotLookup) Lookup(
	ctx context.Context,
	slot sdk.SlotRef,
) (SecretSlotConfig, error) {
	return l(ctx, slot)
}

// Slot records all four reference components without accessing a repository.
func (r *slotRepository) Slot(
	project string,
	workshop string,
	sdkName string,
	slot string,
) *sdk.SlotInfo {
	r.refs = append(r.refs, sdk.SlotRef{
		ProjectId: project,
		Workshop:  workshop,
		Sdk:       sdkName,
		Name:      slot,
	})
	return r.slot
}
