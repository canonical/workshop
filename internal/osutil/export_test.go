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

package osutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/canonical/x-go/strutil"

	"github.com/canonical/workshop/internal/osutil/sys"
)

var (
	ParseRawEnvironment       = parseRawEnvironment
	ParseSystemctlEnvironment = parseSystemctlEnvironment
	DoCopyFile                = doCopyFile
)

// ParseRawExpandableEnv returns a new expandable environment parsed from key=value strings.
func ParseRawExpandableEnv(entries []string) (ExpandableEnv, error) {
	om := strutil.NewOrderedMap()
	for _, entry := range entries {
		key, value, err := parseEnvEntry(entry)
		if err != nil {
			return ExpandableEnv{}, err
		}
		if om.Get(key) != "" {
			return ExpandableEnv{}, fmt.Errorf("cannot overwrite earlier value of %q", key)
		}
		om.Set(key, value)
	}
	return ExpandableEnv{OrderedMap: om}, nil
}

func FakeChown(f func(*os.File, sys.UserID, sys.GroupID) error) (restore func()) {
	oldChown := chown
	chown = f
	return func() {
		chown = oldChown
	}
}

// FakeMountInfo fakes content of /proc/self/mountinfo.
func FakeMountInfo(text string) (restore func()) {
	old := procSelfMountInfo
	f, err := os.CreateTemp("", "mountinfo")
	if err != nil {
		panic(fmt.Errorf("cannot open temporary file: %w", err))
	}
	if err := os.WriteFile(f.Name(), []byte(text), 0o644); err != nil {
		panic(fmt.Errorf("cannot write mock mountinfo file: %w", err))
	}
	procSelfMountInfo = f.Name()
	return func() {
		os.Remove(f.Name())
		procSelfMountInfo = old
	}
}

func SetUnsafeIO(b bool) (restore func()) {
	old := unsafeIO
	unsafeIO = b
	return func() {
		unsafeIO = old
	}
}

func SetAtomicFileRenamed(aw *AtomicFile, renamed bool) {
	aw.renamed = renamed
}

func WaitingReaderGuts(r io.Reader) (io.Reader, *exec.Cmd) {
	wr := r.(*waitingReader)
	return wr.reader, wr.cmd
}

func FakeCmdWaitTimeout(timeout time.Duration) (restore func()) {
	oldCmdWaitTimeout := cmdWaitTimeout
	cmdWaitTimeout = timeout
	return func() {
		cmdWaitTimeout = oldCmdWaitTimeout
	}
}

func FakeSyscallKill(f func(int, syscall.Signal) error) (restore func()) {
	oldSyscallKill := syscallKill
	syscallKill = f
	return func() {
		syscallKill = oldSyscallKill
	}
}

func FakeSyscallGetpgid(f func(int) (int, error)) (restore func()) {
	oldSyscallGetpgid := syscallGetpgid
	syscallGetpgid = f
	return func() {
		syscallGetpgid = oldSyscallGetpgid
	}
}

func MockCopyFile(new func(fileish, fileish, os.FileInfo) error) (restore func()) {
	old := copyfile
	copyfile = new
	return func() {
		copyfile = old
	}
}

func MockOpenFile(new func(string, int, os.FileMode) (fileish, error)) (restore func()) {
	old := openfile
	openfile = new
	return func() {
		openfile = old
	}
}

type Fileish = fileish

func MockMaxCp(new int64) (restore func()) {
	old := maxcp
	maxcp = new
	return func() {
		maxcp = old
	}
}

// MockUname mocks syscall.Uname as used by MachineName and KernelVersion
func MockUname(f func(*syscall.Utsname) error) (restore func()) {
	old := syscallUname
	syscallUname = f
	return func() {
		syscallUname = old
	}
}

func FakeRandomString(f func(int) string) (restore func()) {
	old := randomString
	randomString = f
	return func() {
		randomString = old
	}
}
