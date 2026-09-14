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
	"errors"
	"strings"

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
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(snapInfo *sdk.Info) {}))
	s.projectId = "prj-4242"
}

func (s *ValidateSuite) TearDownTest(c *check.C) {
	s.BaseTest.TearDownTest(c)
}

// TestParseSketchYaml accepts the sketch SDK YAML shape, including hooks and
// descriptive metadata that are not part of the old command-local shape.
func (s *ValidateSuite) TestParseSketchYaml(c *check.C) {
	parsed, err := sdk.ParseSketchYaml(strings.NewReader(`name: sketch
title: Sketch SDK
summary: Local prototype SDK
description: Used to prototype workshop customisations.
hooks:
  setup-project: |
    echo setup
plugs:
  models:
    interface: mount
slots:
  service:
    interface: tunnel
`))

	c.Assert(err, check.IsNil)
	c.Check(parsed.Name, check.Equals, sdk.Sketch)
	c.Check(parsed.Hooks, check.HasLen, 1)
}

// TestValidateSketchYaml accepts a parsed sketch SDK YAML value with valid
// sketch-specific fields.
func (s *ValidateSuite) TestValidateSketchYaml(c *check.C) {
	err := sdk.ValidateSketchYaml(&sdk.SketchSDKYaml{
		Hooks: map[string]string{"setup-project": "echo setup\n"},
		Name:  sdk.Sketch,
	})

	c.Check(err, check.IsNil)
}

// TestValidateSketchYamlInvalidHookName rejects hook names that are not part
// of the sketch SDK hook lifecycle.
func (s *ValidateSuite) TestValidateSketchYamlInvalidHookName(c *check.C) {
	err := sdk.ValidateSketchYaml(&sdk.SketchSDKYaml{
		Hooks: map[string]string{"setup-prject": "echo typo\n"},
		Name:  sdk.Sketch,
	})

	c.Assert(err, check.NotNil)
	c.Check(
		errors.Is(err, sdk.InvalidSDKHookNameError("setup-prject")),
		check.Equals,
		true,
	)
}

// TestValidateSketchYamlInvalidName requires the sketch SDK YAML to describe
// the reserved sketch SDK name.
func (s *ValidateSuite) TestValidateSketchYamlInvalidName(c *check.C) {
	err := sdk.ValidateSketchYaml(&sdk.SketchSDKYaml{Name: "tools"})

	c.Assert(err, check.NotNil)
	c.Check(errors.Is(err, sdk.ErrorInvalidSDKName), check.Equals, true)
}

// TestParseSketchYamlUnknownFields reports full SDK metadata fields as unknown
// when they are not meaningful for sketch SDK YAML.
func (s *ValidateSuite) TestParseSketchYamlUnknownFields(c *check.C) {
	_, err := sdk.ParseSketchYaml(strings.NewReader(`name: sketch
architecture: amd64
base: ubuntu@24.04
version: 1.0
`))

	c.Assert(err, check.ErrorMatches, `unknown SDK YAML fields: .*`)

	var unknown *sdk.UnknownYamlFieldsError
	ok := errors.As(err, &unknown)
	c.Assert(ok, check.Equals, true)
	c.Check(unknown.Fields, check.HasLen, 3)

	field, ok := unknown.Fields["architecture"]
	c.Assert(ok, check.Equals, true)
	c.Check(field.Line, check.Equals, 2)
	c.Check(field.Column, check.Equals, 15)

	_, ok = unknown.Fields["base"]
	c.Check(ok, check.Equals, true)
	_, ok = unknown.Fields["version"]
	c.Check(ok, check.Equals, true)
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
		c.Assert(err, check.ErrorMatches, `invalid slot name: ".*"`)
		err = sdk.ValidatePlugName(name)
		c.Assert(err, check.ErrorMatches, `invalid plug name: ".*"`)
		err = sdk.ValidateInterfaceName(name)
		c.Assert(err, check.ErrorMatches, `invalid interface name: ".*"`)
	}
}

// TestValidateYaml accepts SDK YAML with known top-level fields and valid
// semantic content.
func (s *ValidateSuite) TestValidateYaml(c *check.C) {
	err := sdk.ValidateYaml(strings.NewReader(`name: valid
base: ubuntu@24.04
architecture: amd64
plugs:
  models:
    interface: mount
slots:
  service:
    interface: tunnel
`))

	c.Check(err, check.IsNil)
}

// TestValidateYamlInvalidContent reports semantic validation failures after
// YAML decoding succeeds.
func (s *ValidateSuite) TestValidateYamlInvalidContent(c *check.C) {
	err := sdk.ValidateYaml(strings.NewReader(`name: invalid.name
`))

	c.Check(err, check.ErrorMatches, `invalid SDK name "invalid.name"`)
}

// TestValidateYamlUnknownFields reports unknown top-level keys as a structured
// error that callers can inspect with [errors.As].
func (s *ValidateSuite) TestValidateYamlUnknownFields(c *check.C) {
	err := sdk.ValidateYaml(strings.NewReader(`name: valid
zzz-field: later
aaa-field: first
`))

	c.Assert(err, check.ErrorMatches, `unknown SDK YAML fields: .*`)

	var unknown *sdk.UnknownYamlFieldsError
	ok := errors.As(err, &unknown)
	c.Assert(ok, check.Equals, true)
	c.Check(unknown.Fields, check.HasLen, 2)

	field, ok := unknown.Fields["zzz-field"]
	c.Assert(ok, check.Equals, true)
	c.Check(field.Line, check.Equals, 2)
	c.Check(field.Column, check.Equals, 12)

	_, ok = unknown.Fields["aaa-field"]
	c.Check(ok, check.Equals, true)
}

func (s *ValidateSuite) TestIllegalSdkName(c *check.C) {
	info, err := sdk.ReadSdkInfo([]byte(`name: foo.something
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.ErrorMatches, `invalid SDK name "foo.something"`)
}

func (s *ValidateSuite) TestLongSdkName(c *check.C) {
	info, err := sdk.ReadSdkInfo([]byte(`name: xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.IsNil)

	info, err = sdk.ReadSdkInfo([]byte(`name: xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40x
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.ErrorMatches, `SDK name "xxx05xxx10xxx15xxx20xxx25xxx30xxx35xxx40x" too long`)
}

func (s *ValidateSuite) TestNoSdkBase(c *check.C) {
	info, err := sdk.ReadSdkInfo([]byte(`name: foo
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.IsNil)
}

func (s *ValidateSuite) TestIllegalSdkBase(c *check.C) {
	info, err := sdk.ReadSdkInfo([]byte(`name: foo
base: ubuntu@21.04
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.ErrorMatches, `invalid SDK base "ubuntu@21.04"; valid bases: ubuntu@20.04, ubuntu@22.04, ubuntu@24.04, ubuntu@26.04`)
}

func (s *ValidateSuite) TestIllegalSdkArch(c *check.C) {
	info, err := sdk.ReadSdkInfo([]byte(`name: foo
architecture: '8086'
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	err = sdk.Validate(info)
	c.Check(err, check.ErrorMatches, `invalid SDK architecture "8086"; supported architectures: amd64, arm64, armhf, i386, powerpc, ppc64, ppc64el, riscv64, s390x`)
}
