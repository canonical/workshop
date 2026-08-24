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

package lxd_device

import (
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/sdk"
)

// MountConnectedPlugDefiner is implemented by interfaces that contribute
// to a workshop's LXD device configuration when a plug of the interface
// is connected. The backend detects the method by type assertion; an
// interface that does not implement it contributes nothing.
type MountConnectedPlugDefiner interface {
	MountConnectedPlug(
		spec *Specification,
		plug *interfaces.ConnectedPlug,
		slot *interfaces.ConnectedSlot,
	) error
}

// MountConnectedSlotDefiner is implemented by interfaces that contribute
// to a workshop's LXD device configuration when a slot of the interface
// is connected. The backend detects the method by type assertion; an
// interface that does not implement it contributes nothing.
type MountConnectedSlotDefiner interface {
	MountConnectedSlot(
		spec *Specification,
		plug *interfaces.ConnectedPlug,
		slot *interfaces.ConnectedSlot,
	) error
}

// MountPermanentPlugDefiner is implemented by interfaces that contribute
// to a workshop's LXD device configuration whenever a plug of the
// interface is present, regardless of any connection. The backend
// detects the method by type assertion; an interface that does not
// implement it contributes nothing.
type MountPermanentPlugDefiner interface {
	MountPermanentPlug(spec *Specification, plug *sdk.PlugInfo) error
}

// MountPermanentSlotDefiner is implemented by interfaces that contribute
// to a workshop's LXD device configuration whenever a slot of the
// interface is present, regardless of any connection. The backend
// detects the method by type assertion; an interface that does not
// implement it contributes nothing.
type MountPermanentSlotDefiner interface {
	MountPermanentSlot(spec *Specification, slot *sdk.SlotInfo) error
}
