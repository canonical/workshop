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

package sdk_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type ValidateSuite struct {
	testutil.BaseTest
	projectId string
}

var _ = check.Suite(&ValidateSuite{})

func (s *ValidateSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	s.projectId = "prj-4242"
}

func (s *ValidateSuite) TearDownTest(c *check.C) {
	s.BaseTest.TearDownTest(c)
}

func (s *ValidateSuite) TestValidateSlotPlugInterfaceName(c *check.C) {
	valid := []string{
		"a",
		"aaa",
		"a-a",
		"aa-a",
		"a-aa",
		"a-b-c",
		"valid",
		"valid-123",
	}
	for _, name := range valid {
		err := sdk.ValidateSlotName(name)
		c.Assert(err, check.IsNil)
		err = sdk.ValidatePlugName(name)
		c.Assert(err, check.IsNil)
		err = sdk.ValidateInterfaceName(name)
		c.Assert(err, check.IsNil)
	}
	invalid := []string{
		"",
		"a a",
		"a--a",
		"-a",
		"a-",
		"0",
		"123",
		"123abc",
		"日本語",
	}
	for _, name := range invalid {
		err := sdk.ValidateSlotName(name)
		c.Assert(err, check.ErrorMatches, "invalid slot name")
		err = sdk.ValidatePlugName(name)
		c.Assert(err, check.ErrorMatches, "invalid plug name")
		err = sdk.ValidateInterfaceName(name)
		c.Assert(err, check.ErrorMatches, `invalid interface name: ".*"`)
	}
}

func (s *ValidateSuite) TestIllegalSdkName(c *check.C) {
	_, err := sdk.ReadSdkInfo([]byte(`name: foo.something
`), s.projectId, "ws", nil, sdk.Validate)
	c.Check(err, check.ErrorMatches, `invalid SDK name "foo.something"`)
}

func (s *ValidateSuite) TestLongSdkName(c *check.C) {
	_, err := sdk.ReadSdkInfo([]byte(`name: xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40
`), s.projectId, "ws", nil, sdk.Validate)
	c.Assert(err, check.IsNil)

	_, err = sdk.ReadSdkInfo([]byte(`name: xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40x
`), s.projectId, "ws", nil, sdk.Validate)
	c.Check(err, check.ErrorMatches, `SDK name "xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40x" too long`)
}

func (s *ValidateSuite) TestNoSdkBase(c *check.C) {
	_, err := sdk.ReadSdkInfo([]byte(`name: foo
`), s.projectId, "ws", nil, sdk.Validate)
	c.Check(err, check.IsNil)
}

func (s *ValidateSuite) TestIllegalSdkBase(c *check.C) {
	_, err := sdk.ReadSdkInfo([]byte(`name: foo
base: ubuntu@21.04
`), s.projectId, "ws", nil, sdk.Validate)
	c.Check(err, check.ErrorMatches, `invalid SDK base "ubuntu@21.04"; valid bases: ubuntu@20.04, ubuntu@22.04, ubuntu@24.04, ubuntu@26.04`)
}

func (s *ValidateSuite) TestIllegalSdkArch(c *check.C) {
	_, err := sdk.ReadSdkInfo([]byte(`name: foo
architecture: '8086'
`), s.projectId, "ws", nil, sdk.Validate)
	c.Check(err, check.ErrorMatches, `invalid SDK architecture "8086"; supported architectures: amd64, arm64, armhf, i386, powerpc, ppc64, ppc64el, riscv64, s390x`)
}
