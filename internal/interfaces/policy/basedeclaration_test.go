// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2018 Canonical Ltd
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

package policy_test

import (
	"fmt"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/asserts"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/policy"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type baseDeclSuite struct {
	baseDecl        *asserts.BaseDeclaration
	restoreSanitize func()
}

var _ = check.Suite(&baseDeclSuite{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *baseDeclSuite) SetUpSuite(c *check.C) {
	s.restoreSanitize = sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})
	s.baseDecl = asserts.BuiltinBaseDeclaration()
}

func (s *baseDeclSuite) TearDownSuite(c *check.C) {
	s.restoreSanitize()
}

func (s *baseDeclSuite) connectCand(c *check.C, iface, slotYaml, plugYaml string, slotprj, plugprj string, slotws, plugws string) *policy.ConnectCandidate {
	if slotYaml == "" {
		slotYaml = fmt.Sprintf(`name: slot-sdk
base: ubuntu@22.04
slots:
  %s:
`, iface)
	}
	if plugYaml == "" {
		plugYaml = fmt.Sprintf(`name: plug-sdk
base: ubuntu@22.04
plugs:
  %s:
`, iface)
	}
	slotSdk := sdk.MockInfo(c, slotYaml, slotprj, slotws)
	plugSdk := sdk.MockInfo(c, plugYaml, plugprj, plugws)
	return &policy.ConnectCandidate{
		Plug:            interfaces.NewConnectedPlug(plugSdk.Plugs[iface], nil, nil),
		Slot:            interfaces.NewConnectedSlot(slotSdk.Slots[iface], nil, nil),
		BaseDeclaration: s.baseDecl,
	}
}

func (s *baseDeclSuite) installSlotCand(c *check.C, iface string, sdkType sdk.Type, yaml string) *policy.InstallCandidate {
	if yaml == "" {
		name := "install-slot-sdk"
		if sdkType == sdk.System {
			name = "system"
		}
		yaml = fmt.Sprintf(`name: %s
base: ubuntu@22.04
type: %s
slots:
  %s:
`, name, sdkType, iface)
	}
	sdk := sdk.MockInfo(c, yaml, "mock424242", "ws")
	return &policy.InstallCandidate{
		Sdk:             sdk,
		BaseDeclaration: s.baseDecl,
	}
}

func (s *baseDeclSuite) TestAutoConnection(c *check.C) {
	all := builtin.Interfaces()

	// these have more complex or in flux policies and have their
	// own separate tests
	snowflakes := map[string]bool{
		"mount":     true,
		"ssh-agent": true,
		"tunnel":    true,
	}

	// these simply auto-connect, anything else doesn't
	autoconnect := map[string]bool{
		"gpu": true,
	}

	for _, iface := range all {
		if snowflakes[iface.Name()] {
			continue
		}
		expected := autoconnect[iface.Name()]
		comm := check.Commentf(iface.Name())

		// check base declaration
		cand := s.connectCand(c, iface.Name(), "", "", "", "", "", "")
		arity, err := cand.CheckAutoConnect()
		if expected {
			c.Check(err, check.IsNil, comm)
			c.Check(arity.SlotsPerPlugAny(), check.Equals, false)
		} else {
			c.Check(err, check.NotNil, comm)
		}
	}
}

func (s *baseDeclSuite) TestManualConnection(c *check.C) {
	all := builtin.Interfaces()

	for _, iface := range all {
		comm := check.Commentf(iface.Name())

		// check base declaration
		cand := s.connectCand(c, iface.Name(), "", "", "", "", "", "")
		arity, err := cand.Check()
		c.Check(err, check.IsNil, comm)
		// not currently used outside of tests
		c.Check(arity.SlotsPerPlugAny(), check.Equals, true)
	}
}

func (s *baseDeclSuite) TestMountAutoConnection(c *check.C) {
	slotYaml := `name: system
base: ubuntu@22.04
type: system
slots:
    mount:
`

	cand := s.connectCand(c, "mount", slotYaml, "", "mock424242", "mock424242", "ws", "ws")
	arity, err := cand.CheckAutoConnect()
	c.Check(err, check.IsNil)
	c.Check(arity.SlotsPerPlugAny(), check.Equals, false)
}

func (s *baseDeclSuite) TestAutoConnectPlugSlot(c *check.C) {
	all := builtin.Interfaces()

	// these have more complex or in flux policies and have their
	// own separate tests
	snowflakes := map[string]bool{
		"tunnel": true,
	}

	for _, iface := range all {
		if snowflakes[iface.Name()] {
			continue
		}
		c.Check(iface.AutoConnect(nil, nil), check.Equals, true)
	}
}

func (s *baseDeclSuite) TestMountSlotInstallation(c *check.C) {
	// test mount specially
	ic := s.installSlotCand(c, "mount", sdk.Regular, ``)
	err := ic.Check()
	c.Assert(err, check.IsNil)

	ic = s.installSlotCand(c, "mount", sdk.System, ``)
	err = ic.Check()
	c.Assert(err, check.IsNil)
}

func (s *baseDeclSuite) TestComposeBaseDeclaration(c *check.C) {
	decl, err := policy.ComposeBaseDeclaration(nil)
	c.Assert(err, check.IsNil)
	c.Assert(string(decl), testutil.Contains, `
type: base-declaration
authority-id: canonical
series: 1
revision: 0
`)
}

func (s *baseDeclSuite) TestDoesNotPanic(c *check.C) {
	// In case there are any issues in the actual interfaces we'd get a panic
	// on startup. This test prevents this from happening unnoticed.
	_, err := policy.ComposeBaseDeclaration(builtin.Interfaces())
	c.Assert(err, check.IsNil)
}

func (s *baseDeclSuite) TestSlotPlugFromSameWorkshop(c *check.C) {
	slotYaml := `name: system
base: ubuntu@22.04
type: system
slots:
    mount:
`
	cand := s.connectCand(c, "mount", slotYaml, "", "mock424242", "mock424242", "ws", "ws")
	_, err := cand.Check()
	c.Check(err, check.IsNil)
	_, err = cand.CheckAutoConnect()
	c.Check(err, check.IsNil)
}

func (s *baseDeclSuite) TestSlotPlugDifferentProjects(c *check.C) {
	slotYaml := `name: system
base: ubuntu@22.04
type: system
slots:
    mount:
`
	plugYaml := `name: plug-sdk
base: ubuntu@22.04
type: regular
plugs:
    mount:      
`
	cand := s.connectCand(c, "mount", slotYaml, plugYaml, "slot-project", "mock424242", "ws", "ws")
	_, err := cand.Check()
	c.Check(err, check.ErrorMatches, `connection not allowed by plug rule of interface "mount"`)
	_, err = cand.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "mount"`)
}

func (s *baseDeclSuite) TestSlotPlugDifferentWorkshop(c *check.C) {
	slotYaml := `name: system
base: ubuntu@22.04
type: system
slots:
    mount:
`
	plugYaml := `name: plug-sdk
base: ubuntu@22.04
type: regular
plugs:
    mount:
`
	cand := s.connectCand(c, "mount", slotYaml, plugYaml, "mock424242", "mock424242", "ws", "ws-1")
	_, err := cand.Check()
	c.Check(err, check.ErrorMatches, `connection not allowed by plug rule of interface "mount"`)
	_, err = cand.CheckAutoConnect()
	c.Check(err, check.ErrorMatches, `auto-connection not allowed by plug rule of interface "mount"`)
}
