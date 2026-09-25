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

package main

import (
	"bytes"
	"context"

	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/secrets/provider/system"
)

// failingWriter records up to limit bytes before an output pipe failure.
type failingWriter struct {
	err    error
	limit  int
	output bytes.Buffer
}

// stubService delegates lookups to a test without accessing D-Bus.
type stubService func(
	context.Context,
	system.Request,
) (secrets.Secret, error)

func (s stubService) Get(
	ctx context.Context,
	request system.Request,
) (secrets.Secret, error) {
	return s(ctx, request)
}

func (w *failingWriter) Write(data []byte) (int, error) {
	n := min(len(data), w.limit-w.output.Len())
	_, _ = w.output.Write(data[:n])
	return n, w.err
}
