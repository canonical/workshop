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

package osutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// machineIDPath is the path to the machine identifier inside a workshop.
const machineIDPath = "/etc/machine-id"

var (
	// ErrorMachineIDNotFound indicates that the machine ID file is absent or
	// empty.
	ErrorMachineIDNotFound = errors.New("machine ID not found")
)

// MachineID returns the unique machine identifier inside a workshop.
//
// The following errors may be expected:
//   - [ErrorMachineIDNotFound] if the machine ID file is absent or empty.
func MachineID() (string, error) {
	return machineIDFromPath(machineIDPath)
}

// machineIDFromPath returns the unique machine identifier from path.
//
// The following errors may be expected:
//   - [ErrorMachineIDNotFound] if the machine ID file is absent or empty.
func machineIDFromPath(path string) (string, error) {
	machineIDRawVal, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf(
			"%w: %q does not exist", ErrorMachineIDNotFound, path,
		)
	} else if err != nil {
		return "", err
	}

	// Machine ID files conventionally end with a newline, which is invalid in
	// HTTP header values.
	machineID := strings.TrimSpace(string(machineIDRawVal))
	if machineID == "" {
		return "", fmt.Errorf(
			"%w in %q", ErrorMachineIDNotFound, path,
		)
	}
	return machineID, nil
}
