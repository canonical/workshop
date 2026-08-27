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

//go:generate ./generate.sh

package sys

import (
	"os"

	"golang.org/x/sys/unix"
)

func FsFreeze(file *os.File) error {
	if err := unix.IoctlSetInt(int(file.Fd()), FIFREEZE, 0); err != nil {
		return &os.PathError{Op: "fsfreeze", Path: file.Name(), Err: err}
	}
	return nil
}

func FsThaw(file *os.File) error {
	if err := unix.IoctlSetInt(int(file.Fd()), FITHAW, 0); err != nil {
		return &os.PathError{Op: "fsthaw", Path: file.Name(), Err: err}
	}
	return nil
}
