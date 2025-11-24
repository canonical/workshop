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

	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(snapInfo *sdk.Info) {}))
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
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.ProjectId, check.Equals, s.projectId)
	c.Assert(info.Base, check.Equals, "ubuntu@20.04")
	c.Assert(info.Name, check.Equals, "sdk")
	c.Assert(info.Workshop, check.Equals, "ws")
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(info.Slots, check.HasLen, 0)
}

func (s *SdkSuite) TestMinimalisticPlug(c *check.C) {
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
plugs:
  training:
    interface: mount
    workshop-target: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Plugs, check.HasLen, 1)
	c.Assert(info.Slots, check.HasLen, 0)
	c.Assert(*info.Plugs["training"], check.DeepEquals, sdk.PlugInfo{
		Sdk:       info,
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-target": "/project"},
	})
}

func (s *SdkSuite) TestMinimalisticSlot(c *check.C) {
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
slots:
  training:
    interface: mount
    workshop-source: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 1)
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(*info.Slots["training"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info,
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-source": "/project"},
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
	c.Assert(info.Plugs["iface"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info,
		Name:      "iface",
		Interface: "complex",
		Attrs: map[string]interface{}{
			"i": int64(3),
			"l": []interface{}{int64(1), int64(2), int64(3)},
			"m": map[string]interface{}{"a": "A", "b": "B"},
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
	c.Assert(info.Plugs["mount"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info,
		Name:      "mount",
		Interface: "mount",
		Attrs:     map[string]interface{}{"ipv6-aware": true},
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

	c.Assert(info.Plugs["bool-file"], check.DeepEquals, &sdk.PlugInfo{
		Sdk:       info,
		Name:      "bool-file",
		Interface: "bool-file",
		Label:     "Disk I/O indicator",
	})
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        interface: 1.0
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `interface name on plug "net" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    bool-file:
        label: 1.0
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `label of plug "bool-file" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithNonStringAttributes(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        1: ok
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `plug "net" has malformed definition \(found map\[interface {}\]interface {}\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithEmptyAttributeKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net:
        "": ok
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `plug "net" has an empty attribute key`)
}

func (s *SdkSuite) TestUnmarshalCorruptedPlugWithUnexpectedType(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    net: 5
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `plug "net" has malformed definition \(found int\)`)
}

func (s *SdkSuite) TestUnmarshalReservedPlugAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        $baud-rate: [9600]
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `plug "serial" uses reserved attribute "\$baud-rate"`)
}

func (s *SdkSuite) TestUnmarshalInvalidPlugAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        foo: null
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `attribute "foo" of plug \"serial\": invalid scalar:.*`)
}

func (s *SdkSuite) TestUnmarshalInvalidAttributeMapKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
plugs:
    serial:
        interface: serial-port
        bar:
          baz:
          - 1: A
`), s.projectId, "ws")
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
	c.Assert(info.Slots["mount"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
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
	c.Assert(info.Slots["net"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
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
	c.Assert(info.Slots["net"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
		Name:      "net",
		Interface: "mount",
		Attrs:     map[string]interface{}{"ipv6-aware": true},
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
	c.Assert(info.Slots["iface"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
		Name:      "iface",
		Interface: "complex",
		Attrs: map[string]interface{}{
			"i": int64(3),
			"l": []interface{}{int64(1), int64(2)},
			"m": map[string]interface{}{"a": "A"},
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
	c.Assert(info.Slots["mount"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
		Name:      "mount",
		Interface: "mount",
		Attrs:     map[string]interface{}{"ipv6-aware": true},
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
	c.Assert(info.Slots["led0"], check.DeepEquals, &sdk.SlotInfo{
		Sdk:       info,
		Name:      "led0",
		Interface: "bool-file",
		Label:     "Front panel LED (red)",
	})
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringInterfaceName(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        interface: 1.0
        ipv6-aware: true
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `interface name on slot "net" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringLabel(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    bool-file:
        label: 1.0
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `label of slot "bool-file" is not a string \(found float64\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithNonStringAttributes(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        1: ok
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `slot \"net\" has malformed definition \(found map\[interface {}\]interface {}\)`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithEmptyAttributeKey(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net:
        "": ok
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `slot "net" has an empty attribute key`)
}

func (s *SdkSuite) TestUnmarshalCorruptedSlotWithUnexpectedType(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    net: 5
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `slot "net" has malformed definition \(found int\)`)
}

func (s *SdkSuite) TestUnmarshalReservedSlotAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    serial:
        interface: serial-port
        $baud-rate: [9600]
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `slot "serial" uses reserved attribute "\$baud-rate"`)
}

func (s *SdkSuite) TestUnmarshalInvalidSlotAttribute(c *check.C) {
	// NOTE: yaml content cannot use tabs, indent the section with spaces.
	_, err := sdk.ReadSdkInfo([]byte(`
name: sdk
slots:
    serial:
        interface: serial-port
        foo: null
`), s.projectId, "ws")
	c.Assert(err, check.ErrorMatches, `attribute "foo" of slot \"serial\": invalid scalar:.*`)
}

func (s *SdkSuite) TestAddingWorkshopSlotOK(c *check.C) {
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
slots:
  training:
    interface: mount
    workshop-source: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 1)
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(*info.Slots["training"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info,
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-source": "/project"},
	})
	slots := map[string]interface{}{
		"cache": map[string]interface{}{
			"interface":       "mount",
			"workshop-source": "/var/cache",
		},
	}
	err = info.SetupWorkshopSlots(slots)
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 2)
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(*info.Slots["cache"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info,
		Name:      "cache",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-source": "/var/cache"},
	})
}

func (s *SdkSuite) TestAddingAlreadyExistingSlotFails(c *check.C) {
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
slots:
  training:
    interface: mount
    workshop-source: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 1)
	c.Assert(info.Plugs, check.HasLen, 0)
	c.Assert(*info.Slots["training"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info,
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-source": "/project"},
	})
	slots := map[string]interface{}{
		"training": map[string]interface{}{
			"workshop-source": "/data",
		},
	}
	err = info.SetupWorkshopSlots(slots)
	c.Assert(err, check.ErrorMatches, `cannot add slot "training" to "sdk" SDK: already exists`)
}

func (s *SdkSuite) TestAddingAlreadyExistingPlugFails(c *check.C) {
	mockYaml := []byte(`name: sdk
base: ubuntu@20.04
plugs:
  training:
    interface: mount
    workshop-target: /project
`)

	info, err := sdk.ReadSdkInfo(mockYaml, s.projectId, "ws")
	c.Assert(err, check.IsNil)
	c.Assert(info.Slots, check.HasLen, 0)
	c.Assert(info.Plugs, check.HasLen, 1)
	c.Assert(*info.Plugs["training"], check.DeepEquals, sdk.PlugInfo{
		Sdk:       info,
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]interface{}{"workshop-target": "/project"},
	})
	plugs := map[string]interface{}{
		"training": map[string]interface{}{
			"workshop-target": "/data",
		},
	}
	err = info.SetupWorkshopPlugs(plugs)
	c.Assert(err, check.ErrorMatches, `cannot add plug "training" to "sdk" SDK: already exists`)
}
