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

package ctlcmd_test

import "time"

// secretStateBackend signals task scheduling without persistence or timers.
type secretStateBackend struct {
	ensureBefore chan time.Duration
}

// Checkpoint discards state snapshots for command tests.
func (b *secretStateBackend) Checkpoint([]byte) error {
	return nil
}

// EnsureBefore records a notification without blocking the state lock.
func (b *secretStateBackend) EnsureBefore(delay time.Duration) {
	select {
	case b.ensureBefore <- delay:
	default:
	}
}
