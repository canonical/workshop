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

package workshop

import (
	"fmt"
	"strings"
)

// ApparmorDenyWrite returns the normalized apparmor deny-write paths
// declared on the file's "system" SDK entry, if any. ValidateFile guarantees
// that only the "system" entry may carry an ApparmorPolicy and that its
// paths are already absolute, expanded, and deduplicated.
func (f *File) ApparmorDenyWrite() []string {
	for _, s := range f.Sdks {
		if s.Apparmor != nil {
			return s.Apparmor.DenyWrite
		}
	}
	return nil
}

// RenderAppArmorDenyWrite renders a raw.apparmor profile fragment that
// denies write access to each of the given absolute in-workshop paths and
// everything beneath them. It returns an empty string when paths is empty,
// so callers can omit the raw.apparmor config key entirely rather than
// setting it to "".
//
// paths must already be validated (ValidateFile does this): absolute,
// clean, and free of AppArmor glob metacharacters.
func RenderAppArmorDenyWrite(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	rules := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		rules = append(rules,
			fmt.Sprintf("deny %s/ w,", path),
			fmt.Sprintf("deny %s/** w,", path),
		)
	}
	return strings.Join(rules, "\n")
}
