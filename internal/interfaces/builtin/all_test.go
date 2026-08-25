// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package builtin_test

import (
	"os/user"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/sdk"
)

type AllSuite struct{}

var (
	_        = Suite(&AllSuite{})
	testuser = user.User{
		Username: "testuser",
	}
)

func (s *AllSuite) TestRegisterIface(c *C) {
	restore := builtin.MockInterfaces(nil)
	defer restore()

	// Registering an interface works correctly.
	iface := &ifacetest.TestInterface{InterfaceName: "foo"}
	builtin.RegisterIface(iface)
	c.Assert(builtin.Interface("foo"), DeepEquals, iface)

	// Duplicates are detected.
	c.Assert(func() { builtin.RegisterIface(iface) }, PanicMatches, `cannot register duplicate interface "foo"`)
}

const testConsumerInvalidSlotNameYaml = `
name: consumer
slots:
 ttyS5:
  interface: iface
`

const testConsumerInvalidPlugNameYaml = `
name: consumer
plugs:
 ttyS3:
  interface: iface
`

func (s *AllSuite) TestSanitizeErrorsOnInvalidSlotNames(c *C) {
	restore := builtin.MockInterfaces(map[string]interfaces.Interface{
		"iface": &ifacetest.TestInterface{InterfaceName: "iface"},
	})
	defer restore()

	sdkInfo := sdk.MockInvalidInfo(c, testConsumerInvalidSlotNameYaml)
	sdk.SanitizePlugsSlots(sdkInfo)
	c.Assert(sdkInfo.BadInterfaces, HasLen, 1)
	c.Check(sdk.BadInterfacesSummary(sdkInfo), Matches, `"consumer" SDK has bad plugs or slots: ttyS5 \(invalid slot name: "ttyS5"\)`)
}

func (s *AllSuite) TestSanitizeErrorsOnInvalidPlugNames(c *C) {
	restore := builtin.MockInterfaces(map[string]interfaces.Interface{
		"iface": &ifacetest.TestInterface{InterfaceName: "iface"},
	})
	defer restore()

	sdkInfo := sdk.MockInvalidInfo(c, testConsumerInvalidPlugNameYaml)
	sdk.SanitizePlugsSlots(sdkInfo)
	c.Assert(sdkInfo.BadInterfaces, HasLen, 1)
	c.Check(sdk.BadInterfacesSummary(sdkInfo), Matches, `"consumer" SDK has bad plugs or slots: ttyS3 \(invalid plug name: "ttyS3"\)`)
}
