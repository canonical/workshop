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

package secrets

import (
	"fmt"
	"io"
)

// Secret holds bytes that can each be read once. Reads clear consumed bytes;
// [Secret.Close] clears any remainder. The zero value is an empty secret.
// Copies share consumption and clearing state. Concurrent use is not supported.
// Clearing its buffer does not erase copies held elsewhere, including bytes
// returned to readers.
type Secret struct {
	state *secretState
}

// secretState shares the unread buffer across copies of a secret.
type secretState struct {
	value []byte
}

// Close clears unread bytes and releases the buffer. Repeated calls are safe.
// Reads after Close return [io.EOF], except for zero-length reads.
func (s Secret) Close() error {
	if s.state == nil {
		return nil
	}
	clear(s.state.value)
	s.state.value = nil
	return nil
}

// Format suppresses secret output for fmt verbs that use [fmt.Formatter].
// It does not consume the secret. The %T and %p verbs bypass this method.
func (Secret) Format(fmt.State, rune) {}

// NewSecret takes ownership of value without copying it. The caller must not
// subsequently access value or any aliases of its backing array.
// Callers should close the secret if they may not read it to completion.
func NewSecret(value []byte) Secret {
	return Secret{state: &secretState{value: value}}
}

// Read copies unread bytes into p and immediately clears the consumed bytes
// from the internal buffer. A zero-length read consumes nothing and returns nil.
// The destination must not alias the buffer supplied to [NewSecret].
//
// The following errors may be expected:
//   - [io.EOF]: no unread bytes remain and p is non-empty.
func (s Secret) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.state == nil {
		return 0, io.EOF
	}
	if len(s.state.value) == 0 {
		return 0, io.EOF
	}

	n := copy(p, s.state.value)
	clear(s.state.value[:n])
	s.state.value = s.state.value[n:]
	if len(s.state.value) == 0 {
		s.state.value = nil
	}
	return n, nil
}
