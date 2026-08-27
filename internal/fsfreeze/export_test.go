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
	"os"
	"time"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
)

func MockFilesystems(path string) func() {
	return testutil.FakeFunc(path, &filesystemsPath)
}

func MockMountinfo(path string) func() {
	return testutil.FakeFunc(path, &mountinfoPath)
}

func MockTimeout(t time.Duration) func() {
	return testutil.FakeFunc(t, &timeout)
}

func MockFsFreeze(f func(*os.File) error) func() {
	return testutil.FakeFunc(f, &fsFreeze)
}

func MockFsThaw(f func(*os.File) error) func() {
	return testutil.FakeFunc(f, &fsThaw)
}

func LocalMounts() ([]*osutil.MountInfoEntry, string, error) {
	mounts, rootFS, err := localMounts(mountinfoPath, filesystemsPath)
	if err != nil {
		return nil, "", err
	}
	return mounts, rootFS.String(), nil
}
