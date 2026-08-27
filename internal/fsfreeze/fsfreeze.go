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

package fsfreeze

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/canonical/workshop/internal/fsfreeze/sys"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
)

var (
	timeout = 2 * time.Minute

	mountinfoPath   = "/proc/self/mountinfo"
	filesystemsPath = "/proc/filesystems"

	fsFreeze = sys.FsFreeze
	fsThaw   = sys.FsThaw
)

type superblock struct {
	major int
	minor int
}

func (d superblock) String() string {
	return fmt.Sprintf("%d:%d", d.major, d.minor)
}

// IsFsfreezeInvocation reports whether the process was invoked via a symlink
// named fsfreeze. This allows multiple logically unrelated commands to be
// embedded in a single multi-call binary (even in tests).
func IsFsfreezeInvocation() bool {
	return len(os.Args) > 0 && filepath.Base(os.Args[0]) == "fsfreeze"
}

// FreezeLocalFilesystems freezes every filesystem associated with a block
// device. It uses the sd_notify protocol to signal readiness on stdout. When
// stdin is closed, the filesystems are thawed. It also thaws them on error,
// which can be a signal (HUP, INT, PIPE, or TERM) or a timeout. In this case,
// stdin should still be closed in order to unblock stdin.Read().
func FreezeLocalFilesystems(stdin io.Reader, stdout io.Writer) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGPIPE, syscall.SIGTERM)
	defer stop()

	mounts, root, err := localMounts(mountinfoPath, filesystemsPath)
	if err != nil {
		return err
	}

	frozen, err := freeze(ctx, mounts, root)
	if err != nil {
		return err
	}
	defer func() {
		err = cmp.Or(err, thaw(frozen))
	}()

	if _, err := fmt.Fprintln(stdout, "READY=1"); err != nil {
		return fmt.Errorf("cannot report readiness: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ctx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)
	go func() {
		defer cancelCause(io.EOF)
		if _, err1 := io.Copy(io.Discard, stdin); err1 != nil {
			cancelCause(err1)
		}
	}()

	<-ctx.Done()
	if err := context.Cause(ctx); errors.Is(err, io.EOF) {
		return nil
	} else {
		return err
	}
}

// localMounts lists mounts involving block devices. The list is in reverse
// mount order, to guarantee that loop mounts are frozen before the filesystem
// holding the backing file. Only one representative of each superblock is
// listed, to avoid freezing a frozen filesystem.
//
// It also returns the device ID of the rootfs superblock.
func localMounts(mountinfo, filesystems string) ([]*osutil.MountInfoEntry, superblock, error) {
	local, err := localFilesystems(filesystems)
	if err != nil {
		return nil, superblock{}, err
	}

	mounts, err := osutil.LoadMountInfo(mountinfo)
	if err != nil {
		return nil, superblock{}, fmt.Errorf("cannot parse %q: %w", mountinfo, err)
	}
	slices.Reverse(mounts)

	idx := slices.IndexFunc(mounts, func(m *osutil.MountInfoEntry) bool {
		return m.MountDir == "/"
	})
	if idx < 0 {
		return nil, superblock{}, errors.New("root filesystem not mounted")
	}
	root := superblock{major: mounts[idx].DevMajor, minor: mounts[idx].DevMinor}

	last := make(map[superblock]*osutil.MountInfoEntry, len(mounts))
	for _, m := range mounts {
		if isLocal(m, local) {
			last[superblock{major: m.DevMajor, minor: m.DevMinor}] = m
		} else if m.MountDir == "/" {
			return nil, superblock{}, fmt.Errorf("root filesystem %s (%s) is not freezable", root, m.FsType)
		}
	}
	mounts = slices.DeleteFunc(mounts, func(m *osutil.MountInfoEntry) bool {
		return last[superblock{major: m.DevMajor, minor: m.DevMinor}] != m
	})

	return mounts, root, nil
}

// localFilesystems returns a map of filesystems that require block devices.
func localFilesystems(filesystems string) (map[string]bool, error) {
	file, err := os.Open(filesystems)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	types := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		flag, name, ok := strings.Cut(scanner.Text(), "\t")
		if !ok {
			return nil, fmt.Errorf("cannot parse %q: too few fields", filesystems)
		}
		// Currently the first column is either "" or "nodev".
		if !strings.Contains(flag, "nodev") {
			types[name] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot parse %q: %w", filesystems, err)
	}
	return types, nil
}

// isLocal attempts to determine if the given mount is backed by a block
// device. This is intended as an optimisation: most network-based filesystems
// don't support FIFREEZE and will return EOPNOTSUPP immediately. However,
// opening the file descriptor that we pass to FIFREEZE can potentially block
// for a long time if the network is unstable.
//
// There are a few caveats: CIFS does support FIFREEZE, and loop mounts may
// appear to be local even if the loop file is remote. The implementation is
// based on the QEMU guest agent, which ignores these edge cases. Typically
// mounts with major number 0 are remote and others are local. QEMU carves out
// an exception for btrfs; we treat any filesystem which doesn't specify
// "nodev" in /proc/filesystems as local, which should be more future-proof.
// For example bcachefs is a similar exception to btrfs.
//
// Another exception is ZFS, which uses major number 0 and specifies "nodev,"
// but doesn't support FIFREEZE. If in future VMs use ZFS for the rootfs, we
// need to at least call fsync, or preferably the native ZFS equivalent of
// FIFREEZE, to handle this case properly.
func isLocal(m *osutil.MountInfoEntry, local map[string]bool) bool {
	// Some filesystems have subtypes, like fuse.sshfs. Only the main type is
	// listed in /proc/filesystems.
	name, _, _ := strings.Cut(m.FsType, ".")

	return m.DevMajor != 0 || local[name]
}

func freeze(ctx context.Context, mounts []*osutil.MountInfoEntry, root superblock) ([]*os.File, error) {
	rev := revert.New()
	defer rev.Fail()

	var frozen []*os.File
	rev.Add(func() {
		for _, f := range slices.Backward(frozen) {
			_ = fsThaw(f)
			f.Close()
		}
	})

	for _, m := range mounts {
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		default:
		}

		file, err := os.Open(m.MountDir)
		if err != nil {
			return nil, err
		}

		if err := fsFreeze(file); err != nil {
			file.Close()
			if (superblock{major: m.DevMajor, minor: m.DevMinor} != root) && errors.Is(err, unix.EOPNOTSUPP) {
				continue
			}
			return nil, err
		}

		frozen = append(frozen, file)
	}

	rev.Success()
	return frozen, nil
}

func thaw(frozen []*os.File) error {
	var errs []error
	for _, file := range slices.Backward(frozen) {
		errs = append(errs, fsThaw(file))
		file.Close()
	}
	return errors.Join(errs...)
}

type ReadyWriter struct {
	once  sync.Once
	ready chan struct{}
	buf   bytes.Buffer
}

func NewReadyWriter() *ReadyWriter {
	return &ReadyWriter{ready: make(chan struct{})}
}

func (w *ReadyWriter) Ready() <-chan struct{} {
	return w.ready
}

func (w *ReadyWriter) Write(data []byte) (int, error) {
	n, err := w.buf.Write(data)
	for {
		line, _, found := bytes.Cut(w.buf.Bytes(), []byte{'\n'})
		if !found {
			break
		}
		if bytes.Equal(line, []byte("READY=1")) {
			w.once.Do(func() {
				close(w.ready)
			})
		}
		w.buf.Next(len(line) + 1)
	}
	return n, err
}
