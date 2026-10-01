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
	"github.com/canonical/workshop/internal/workshop"
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

const testConsumerInvalidInterfaceNameYaml = `
name: consumer
plugs:
 name:
  interface: IFACE
slots:
 name:
  interface: IFACE
`

func (s *AllSuite) TestSanitizeErrorsOnInvalidSlotNames(c *C) {
	restore := builtin.MockInterfaces(map[string]interfaces.Interface{
		"iface": &ifacetest.TestInterface{InterfaceName: "iface"},
	})
	defer restore()

	sdkInfo := sdk.MockInvalidInfo(c, testConsumerInvalidSlotNameYaml)
	c.Check(builtin.Sanitize(sdkInfo, workshop.Runtime(0)), ErrorMatches, `"consumer" SDK has bad slots: ttyS5 \(invalid slot name\)`)
}

func (s *AllSuite) TestSanitizeErrorsOnInvalidPlugNames(c *C) {
	restore := builtin.MockInterfaces(map[string]interfaces.Interface{
		"iface": &ifacetest.TestInterface{InterfaceName: "iface"},
	})
	defer restore()

	sdkInfo := sdk.MockInvalidInfo(c, testConsumerInvalidPlugNameYaml)
	c.Check(builtin.Sanitize(sdkInfo, workshop.Runtime(0)), ErrorMatches, `"consumer" SDK has bad plugs: ttyS3 \(invalid plug name\)`)
}

func (s *AllSuite) TestSanitizeErrorsOnInvalidPlugsAndSlots(c *C) {
	restore := builtin.MockInterfaces(map[string]interfaces.Interface{
		"iface": &ifacetest.TestInterface{InterfaceName: "iface"},
	})
	defer restore()

	sdkInfo := sdk.MockInvalidInfo(c, testConsumerInvalidInterfaceNameYaml)
	c.Check(builtin.Sanitize(sdkInfo, workshop.Runtime(0)), ErrorMatches, `"consumer" SDK has bad plugs: name \(unknown interface "IFACE"\); and slots: name \(unknown interface "IFACE"\)`)
}
