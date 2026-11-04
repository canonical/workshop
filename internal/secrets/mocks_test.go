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
	"context"

	"github.com/canonical/workshop/internal/sdk"
)

// stubProvider delegates secret resolution to a test-defined function.
type stubProvider func(context.Context, sdk.SlotRef) (Secret, error)

// Resolve calls the test-defined function.
func (p stubProvider) Resolve(
	ctx context.Context,
	slot sdk.SlotRef,
) (Secret, error) {
	return p(ctx, slot)
}
