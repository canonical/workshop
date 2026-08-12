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

// workshopInstanceIDPath is the current source of the workshop instance ID.
const workshopInstanceIDPath = "/etc/machine-id"

var (
	// ErrorWorkshopInstanceIDNotFound indicates that the workshop instance ID
	// file is absent or empty.
	ErrorWorkshopInstanceIDNotFound = errors.New("workshop instance ID not found")
)

// WorkshopInstanceID returns the unique identifier for a workshop instance.
//
// The following errors may be expected:
//   - [ErrorWorkshopInstanceIDNotFound] if the workshop instance ID file is
//     absent or empty.
func WorkshopInstanceID() (string, error) {
	return workshopInstanceIDFromPath(workshopInstanceIDPath)
}

// workshopInstanceIDFromPath returns the workshop instance identifier from
// path.
//
// The following errors may be expected:
//   - [ErrorWorkshopInstanceIDNotFound] if the workshop instance ID file is
//     absent or empty.
func workshopInstanceIDFromPath(path string) (string, error) {
	instanceIDRawValue, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf(
			"%w: %q does not exist", ErrorWorkshopInstanceIDNotFound, path,
		)
	} else if err != nil {
		return "", err
	}

	// Machine ID files conventionally end with a newline, which is invalid in
	// HTTP header values.
	instanceID := strings.TrimSpace(string(instanceIDRawValue))
	if instanceID == "" {
		return "", fmt.Errorf(
			"%w in %q", ErrorWorkshopInstanceIDNotFound, path,
		)
	}
	return instanceID, nil
}
