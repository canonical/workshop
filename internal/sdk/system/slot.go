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

package system

import (
	"errors"

	"github.com/canonical/workshop/internal/sdk"
)

// BeforePrepareSlot validates and normalizes a slot provided by the system
// SDK.
func BeforePrepareSlot(slot *sdk.SlotInfo) error {
	if slot.Sdk.Type != sdk.System {
		return errors.New("system SDK slot preparation requires a system SDK slot")
	}

	switch slot.Interface {
	case "secret":
		return BeforePrepareSecretSlot(slot)
	default:
		return nil
	}
}
