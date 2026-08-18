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

package syscheck

import "sync"

var m sync.Mutex
var checks []func() error

func RegisterCheck(f func() error) {
	m.Lock()
	defer m.Unlock()
	checks = append(checks, f)
}

// CheckSystem ensures that the system is capable of running workshopd.
//
// An error with details is returned if some check fails.
func CheckSystem() error {
	m.Lock()
	defer m.Unlock()

	for _, f := range checks {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}

// MockChecks replaces the registered checks with an empty list and returns a
// function restoring the previous ones. For testing.
func MockChecks() (restore func()) {
	m.Lock()
	defer m.Unlock()

	old := checks
	checks = nil

	return func() {
		m.Lock()
		defer m.Unlock()
		checks = old
	}
}
