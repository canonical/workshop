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
	"encoding/json"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type SdkSuite struct {
	testutil.BaseTest
	projectId string
}

var _ = check.Suite(&SdkSuite{})

func TestSdkSuite(t *testing.T) { check.TestingT(t) }

func (s *SdkSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	s.projectId = "prj42prj42"

	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil }))
}

func (s *SdkSuite) TearDownTest(c *check.C) {
	s.BaseTest.TearDownTest(c)
}

func (s *SdkSuite) TestSourceMarshalUnmarshal(c *check.C) {
	for i := range sdk.SketchSource + 1 {
		setup := sdk.Setup{Name: "sdk", Source: i}
		data, err := json.Marshal(setup)
		c.Assert(err, check.IsNil)

		var result sdk.Setup
		c.Assert(json.Unmarshal(data, &result), check.IsNil)
		c.Check(result, check.Equals, setup)
	}
}

func (s *SdkSuite) TestSimple(c *check.C) {
	var mockYaml = []byte(`name: sdk
base: ubuntu@24.04
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.ProjectId, check.Equals, s.projectId)
	c.Assert(info.Base, check.Equals, "ubuntu@24.04")
	c.Assert(info.Name, check.Equals, "sdk")
	c.Assert(info.Workshop, check.Equals, "ws")
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(info.Slots, check.HasLen, 0)
}

func (s *SdkSuite) TestMinimalisticPlug(c *check.C) {
	var mockYaml = []byte(`name: sdk
base: ubuntu@24.04
plugs:
  training:
    interface: mount
    workshop-target: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Plugs, check.HasLen, 1)
	c.Assert(info.Slots, check.HasLen, 0)

	plugs, err := sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.IsNil)
	c.Assert(*plugs["training"], check.DeepEquals, sdk.PlugInfo{
		Sdk:       info.Ref(),
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]any{"workshop-target": "/project"},
	})
}

func (s *SdkSuite) TestMinimalisticSlot(c *check.C) {
	var mockYaml = []byte(`name: sdk
base: ubuntu@24.04
slots:
  training:
    interface: mount
    workshop-source: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 1)
	c.Assert(info.Plugs, check.HasLen, 0)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(*slots["training"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]any{"workshop-source": "/project"},
	})
}

func (s *SdkSuite) TestUnmarshalStandalonePlugWithIntAndListAndMap(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    iface:
        interface: complex
        i: 3
        l: [1,2,3]
        m:
          a: A
          b: B
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Plugs, check.HasLen, 1)
	c.Assert(info.Slots, check.HasLen, 0)

	plugs, err := sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.IsNil)
	c.Assert(plugs["iface"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info.Ref(),
		Name:      "iface",
		Interface: "complex",
		Attrs: map[string]any{
			"i": int64(3),
			"l": []any{int64(1), int64(2), int64(3)},
			"m": map[string]any{"a": "A", "b": "B"},
		},
	})
}

func (s *SdkSuite) TestUnmarshalDuplicatePlugFails(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        interface: mount
        attr: 1
    net:
        interface: mount
        attr: 2
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `(?s).*line 7: mapping key \"net\" already defined at line 4`)
}

func (s *SdkSuite) TestUnmarshalPlugWithoutInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    mount:
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 1)
	c.Check(info.Slots, check.HasLen, 0)

	plugs, err := sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.IsNil)
	c.Assert(plugs["mount"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info.Ref(),
		Name:      "mount",
		Interface: "mount",
		Attrs:     map[string]any{"ipv6-aware": true},
	})
}

func (s *SdkSuite) TestUnmarshalPlugWithLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    bool-file:
        label: Disk I/O indicator
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)

	c.Check(info.Plugs, check.HasLen, 1)
	c.Check(info.Slots, check.HasLen, 0)

	plugs, err := sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.IsNil)
	c.Assert(plugs["bool-file"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info.Ref(),
		Name:      "bool-file",
		Interface: "bool-file",
		Label:     "Disk I/O indicator",
	})
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        interface: 1.0
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `interface name on plug "net" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    bool-file:
        label: 1.0
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `label of plug "bool-file" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringAttributes(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        1: ok
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `plug "net" has malformed definition \(found map\[interface {}\]interface {}\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithEmptyAttributeKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        "": ok
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `plug "net" has an empty attribute key`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithUnexpectedType(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net: 5
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `plug "net" has malformed definition \(found int\)`)
}

func (s *SdkSuite) TestUnmarshalReservedPlugAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        $baud-rate: [9600]
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `plug "serial" uses reserved attribute "\$baud-rate"`)
}

func (s *SdkSuite) TestUnmarshalInvalidPlugAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        foo: null
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `attribute "foo" of plug \"serial\": invalid scalar:.*`)
}

func (s *SdkSuite) TestUnmarshalInvalidAttributeMapKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        bar:
          baz:
          - 1: A
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParsePlugs(info.Ref(), info.Plugs)
	c.Assert(err, check.ErrorMatches, `attribute "bar" of plug \"serial\": non-string key: 1`)
}

// Tests focusing on slots

func (s *SdkSuite) TestUnmarshalStandaloneImplicitSlot(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    mount:
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["mount"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "mount",
		Interface: "mount",
	})
}

func (s *SdkSuite) TestUnmarshalStandaloneAbbreviatedSlot(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net: mount
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["net"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "net",
		Interface: "mount",
	})
}

func (s *SdkSuite) TestUnmarshalStandaloneCompleteSlot(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        interface: mount
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["net"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "net",
		Interface: "mount",
		Attrs:     map[string]any{"ipv6-aware": true},
	})
}

func (s *SdkSuite) TestUnmarshalStandaloneSlotWithIntAndListAndMap(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    iface:
        interface: complex
        i: 3
        l: [1,2]
        m:
          a: "A"
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["iface"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "iface",
		Interface: "complex",
		Attrs: map[string]any{
			"i": int64(3),
			"l": []any{int64(1), int64(2)},
			"m": map[string]any{"a": "A"},
		},
	})
}

func (s *SdkSuite) TestUnmarshalDuplicateSlotFail(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        interface: mount
        attr: 1
    net:
        interface: mount
        attr: 2
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `(?s).*line 7: mapping key \"net\" already defined at line 4`)
}

func (s *SdkSuite) TestUnmarshalSlotWithoutInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    mount:
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["mount"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "mount",
		Interface: "mount",
		Attrs:     map[string]any{"ipv6-aware": true},
	})
}

func (s *SdkSuite) TestUnmarshalSlotWithLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    led0:
        interface: bool-file
        label: Front panel LED (red)
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Check(info.Plugs, check.HasLen, 0)
	c.Check(info.Slots, check.HasLen, 1)

	slots, err := sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.IsNil)
	c.Assert(slots["led0"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "led0",
		Interface: "bool-file",
		Label:     "Front panel LED (red)",
	})
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        interface: 1.0
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `interface name on slot "net" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    bool-file:
        label: 1.0
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `label of slot "bool-file" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringAttributes(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        1: ok
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `slot \"net\" has malformed definition \(found map\[interface {}\]interface {}\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithEmptyAttributeKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        "": ok
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `slot "net" has an empty attribute key`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithUnexpectedType(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net: 5
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `slot "net" has malformed definition \(found int\)`)
}

func (s *SdkSuite) TestUnmarshalReservedSlotAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    serial:
        interface: serial-port
        $baud-rate: [9600]
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `slot "serial" uses reserved attribute "\$baud-rate"`)
}

func (s *SdkSuite) TestUnmarshalInvalidSlotAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	info, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    serial:
        interface: serial-port
        foo: null
`), s.projectId, "ws")
	c.Assert(err, check.IsNil)
	_, err = sdk.ParseSlots(info.Ref(), info.Slots)
	c.Assert(err, check.ErrorMatches, `attribute "foo" of slot \"serial\": invalid scalar:.*`)
}

// Note: workshop-file plug/slot merging (formerly Info.SetupWorkshopSlots /
// Info.SetupWorkshopPlugs, including "already exists" conflict checks) now
// lives in Workshop.SdkPlugsAndSlots; see workshop_test.go for
// TestSdkPlugsAndSlotsAddsWorkshopSlot,
// TestSdkPlugsAndSlotsAlreadyExistingSlotFails, and
// TestSdkPlugsAndSlotsAlreadyExistingPlugFails.
