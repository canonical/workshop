package asserts_test

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/asserts"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

var (
	_ = check.Suite(&attrConstraintsSuite{})
	_ = check.Suite(&nameConstraintsSuite{})

	_ = check.Suite(&plugSlotRulesSuite{})
)

var (
	sideArityAny = asserts.SideArityConstraint{N: -1}
	sideArityOne = asserts.SideArityConstraint{N: 1}
)

func checkBoolPlugConnConstraints(c *check.C, subrule string, cstrs []*asserts.PlugConnectionConstraints, always bool) {
	expected := asserts.NeverMatchAttributes
	if always {
		expected = asserts.AlwaysMatchAttributes
	}
	c.Assert(cstrs, check.HasLen, 1)
	cstrs1 := cstrs[0]
	c.Check(cstrs1.PlugAttributes, check.Equals, expected)
	c.Check(cstrs1.SlotAttributes, check.Equals, expected)
	if strings.HasPrefix(subrule, "deny-") {
		undef := asserts.SideArityConstraint{}
		c.Check(cstrs1.SlotsPerPlug, check.Equals, undef)
		c.Check(cstrs1.PlugsPerSlot, check.Equals, undef)
	} else {
		c.Check(cstrs1.PlugsPerSlot, check.Equals, sideArityAny)
		if strings.HasSuffix(subrule, "-auto-connection") {
			c.Check(cstrs1.SlotsPerPlug, check.Equals, sideArityOne)
		} else {
			c.Check(cstrs1.SlotsPerPlug, check.Equals, sideArityAny)
		}
	}
	c.Check(cstrs1.SlotSdkTypes, check.HasLen, 0)
}

type attrConstraintsSuite struct {
	testutil.BaseTest
}

type attrerObject map[string]interface{}

func (o attrerObject) Lookup(path string) (interface{}, bool) {
	v, ok := o[path]
	return v, ok
}

func attrs(yml string) *attrerObject {
	var attrs map[string]interface{}
	err := yaml.Unmarshal([]byte(yml), &attrs)
	if err != nil {
		panic(err)
	}
	sdkYaml, err := yaml.Marshal(map[string]interface{}{
		"name": "sample",
		"plugs": map[string]interface{}{
			"plug": attrs,
		},
	})
	if err != nil {
		panic(err)
	}

	// NOTE: it's important to go through sdk yaml here even though we're really interested in Attrs only,
	// as InfoFromSdkYaml normalizes yaml values.
	info, err := sdk.ReadSdkInfo(sdkYaml, "", "")
	if err != nil {
		panic(err)
	}

	ao := attrerObject(info.Plugs["plug"].Attrs)
	return &ao
}

func (s *attrConstraintsSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {}))
}

func (s *attrConstraintsSuite) TearDownTest(c *check.C) {
	s.BaseTest.TearDownTest(c)
}

func (s *attrConstraintsSuite) TestSimple(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`attrs:
  foo: FOO
  bar: BAR`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	plug := attrerObject(map[string]interface{}{
		"foo": "FOO",
		"bar": "BAR",
		"baz": "BAZ",
	})
	err = cstrs.Check(plug, nil)
	c.Check(err, check.IsNil)

	plug = attrerObject(map[string]interface{}{
		"foo": "FOO",
		"bar": "BAZ",
		"baz": "BAZ",
	})
	err = cstrs.Check(plug, nil)
	c.Check(err, check.ErrorMatches, `attribute "bar" value "BAZ" does not match \^\(BAR\)\$`)

	plug = attrerObject(map[string]interface{}{
		"foo": "FOO",
		"baz": "BAZ",
	})
	err = cstrs.Check(plug, nil)
	c.Check(err, check.ErrorMatches, `attribute "bar" has constraints but is unset`)
}

func (s *attrConstraintsSuite) TestMissingCheck(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`attrs:
  foo: $MISSING`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)
	c.Check(asserts.RuleFeature(cstrs, "dollar-attr-constraints"), check.Equals, true)
}

func (s *attrConstraintsSuite) TestNested(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`attrs:
  foo: FOO
  bar:
    bar1: BAR1
    bar2: BAR2`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	err = cstrs.Check(attrs(`
foo: FOO
bar:
  bar1: BAR1
  bar2: BAR2
  bar3: BAR3
baz: BAZ
`), nil)
	c.Check(err, check.IsNil)

	err = cstrs.Check(attrs(`
foo: FOO
bar: BAZ
baz: BAZ
`), nil)
	c.Check(err, check.ErrorMatches, `attribute "bar" must be a map`)

	err = cstrs.Check(attrs(`
foo: FOO
bar:
  bar1: BAR1
  bar2: BAR22
  bar3: BAR3
baz: BAZ
`), nil)
	c.Check(err, check.ErrorMatches, `attribute "bar\.bar2" value "BAR22" does not match \^\(BAR2\)\$`)

	err = cstrs.Check(attrs(`
foo: FOO
bar:
  bar1: BAR1
  bar2:
    bar22: true
  bar3: BAR3
baz: BAZ
`), nil)
	c.Check(err, check.ErrorMatches, `attribute "bar\.bar2" must be a scalar or list`)
}

func (s *attrConstraintsSuite) TestAlternativeMatchingComplex(c *check.C) {
	toMatch := attrs(`
mnt: [{what: "/dev/x*", where: "/foo/*", options: ["rw", "nodev"]}, {what: "/bar/*", where: "/baz/*", options: ["rw", "bind"]}]
`)

	m, err := asserts.ParseHeaders([]byte(`attrs:
  mnt:
    -
      what: /(bar/|dev/x)\*
      where: /(foo|baz)/\*
      options: rw|bind|nodev`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	err = cstrs.Check(toMatch, nil)
	c.Check(err, check.IsNil)

	m, err = asserts.ParseHeaders([]byte(`attrs:
  mnt:
    -
      what: /dev/x\*
      where: /foo/\*
      options:
        - nodev
        - rw
    -
      what: /bar/\*
      where: /baz/\*
      options:
        - rw
        - bind`))
	c.Assert(err, check.IsNil)

	cstrsExtensive, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	err = cstrsExtensive.Check(toMatch, nil)
	c.Check(err, check.IsNil)

	// not matching case
	m, err = asserts.ParseHeaders([]byte(`attrs:
  mnt:
    -
      what: /dev/x\*
      where: /foo/\*
      options:
        - rw
    -
      what: /bar/\*
      where: /baz/\*
      options:
        - rw
        - bind`))
	c.Assert(err, check.IsNil)

	cstrsExtensiveNoMatch, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	err = cstrsExtensiveNoMatch.Check(toMatch, nil)
	c.Check(err, check.ErrorMatches, `no alternative for attribute "mnt\.0" matches: no alternative for attribute "mnt\.0.options\.1" matches:.*`)
}

func (s *attrConstraintsSuite) TestOtherScalars(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`attrs:
  foo: 1
  bar: true`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	err = cstrs.Check(attrs(`
foo: 1
bar: true
`), nil)
	c.Check(err, check.IsNil)
}

func (s *attrConstraintsSuite) TestCompileErrors(c *check.C) {
	_, err := asserts.CompileAttributeConstraints(map[string]interface{}{
		"foo": "[",
	})
	c.Check(err, check.ErrorMatches, `cannot compile "foo" constraint "\[": error parsing regexp:.*`)

	_, err = asserts.CompileAttributeConstraints("FOO")
	c.Check(err, check.ErrorMatches, `first level of non alternative constraints must be a set of key-value contraints`)

	_, err = asserts.CompileAttributeConstraints([]interface{}{"FOO"})
	c.Check(err, check.ErrorMatches, `first level of non alternative constraints must be a set of key-value contraints`)

	wrongDollarConstraints := []string{
		"$",
		"$FOO(a)",
		"$SLOT",
		"$SLOT()",
	}

	for _, wrong := range wrongDollarConstraints {
		_, err := asserts.CompileAttributeConstraints(map[string]interface{}{
			"foo": wrong,
		})
		c.Check(err, check.ErrorMatches, fmt.Sprintf(`cannot compile "foo" constraint "%s": not a valid \$SLOT\(\)/\$PLUG\(\) constraint`, regexp.QuoteMeta(wrong)))

	}
}

type testEvalAttr struct {
	comp func(side string, arg string) (interface{}, error)
}

func (ca testEvalAttr) SlotAttr(arg string) (interface{}, error) {
	return ca.comp("slot", arg)
}

func (ca testEvalAttr) PlugAttr(arg string) (interface{}, error) {
	return ca.comp("plug", arg)
}

func (s *attrConstraintsSuite) TestEvalCheck(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`attrs:
  foo: $SLOT(foo)
  bar: $PLUG(bar.baz)`))
	c.Assert(err, check.IsNil)

	cstrs, err := asserts.CompileAttributeConstraints(m["attrs"].(map[string]interface{}))
	c.Assert(err, check.IsNil)
	c.Check(asserts.RuleFeature(cstrs, "dollar-attr-constraints"), check.Equals, true)

	err = cstrs.Check(attrs(`
foo: foo
bar: bar
`), nil)
	c.Check(err, check.ErrorMatches, `attribute "(foo|bar)" cannot be matched without context`)

	calls := make(map[[2]string]bool)
	comp1 := func(op string, arg string) (interface{}, error) {
		calls[[2]string{op, arg}] = true
		return arg, nil
	}

	err = cstrs.Check(attrs(`
foo: foo
bar: bar.baz
`), testEvalAttr{comp1})
	c.Check(err, check.IsNil)

	c.Check(calls, check.DeepEquals, map[[2]string]bool{
		{"slot", "foo"}:     true,
		{"plug", "bar.baz"}: true,
	})

	comp2 := func(op string, arg string) (interface{}, error) {
		if op == "plug" {
			return nil, fmt.Errorf("boom")
		}
		return arg, nil
	}

	err = cstrs.Check(attrs(`
foo: foo
bar: bar.baz
`), testEvalAttr{comp2})
	c.Check(err, check.ErrorMatches, `attribute "bar" constraint \$PLUG\(bar\.baz\) cannot be evaluated: boom`)

	comp3 := func(op string, arg string) (interface{}, error) {
		if op == "slot" {
			return "other-value", nil
		}
		return arg, nil
	}

	err = cstrs.Check(attrs(`
foo: foo
bar: bar.baz
`), testEvalAttr{comp3})
	c.Check(err, check.ErrorMatches, `attribute "foo" does not match \$SLOT\(foo\): foo != other-value`)
}

func (s *attrConstraintsSuite) TestNeverMatchAttributeConstraints(c *check.C) {
	c.Check(asserts.NeverMatchAttributes.Check(nil, nil), check.NotNil)
}

type plugSlotRulesSuite struct{}

func checkAttrs(c *check.C, attrs *asserts.AttributeConstraints, witness, expected string) {
	plug := attrerObject(map[string]interface{}{
		witness: "XYZ",
	})
	c.Check(attrs.Check(plug, nil), check.ErrorMatches, fmt.Sprintf(`attribute "%s".*does not match.*`, witness))
	plug = attrerObject(map[string]interface{}{
		witness: expected,
	})
	c.Check(attrs.Check(plug, nil), check.IsNil)
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleAllAllowDenyStanzas(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-installation:
    plug-attributes:
      a1: A1
  deny-installation:
    plug-attributes:
      a2: A2
  allow-connection:
    plug-attributes:
      pa3: PA3
    slot-attributes:
      sa3: SA3
  deny-connection:
    plug-attributes:
      pa4: PA4
    slot-attributes:
      sa4: SA4
  allow-auto-connection:
    plug-attributes:
      pa5: PA5
    slot-attributes:
      sa5: SA5
  deny-auto-connection:
    plug-attributes:
      pa6: PA6
    slot-attributes:
      sa6: SA6`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.Interface, check.Equals, "iface")
	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	checkAttrs(c, rule.AllowInstallation[0].PlugAttributes, "a1", "A1")
	c.Assert(rule.DenyInstallation, check.HasLen, 1)
	checkAttrs(c, rule.DenyInstallation[0].PlugAttributes, "a2", "A2")
	// connection subrules
	c.Assert(rule.AllowConnection, check.HasLen, 1)
	checkAttrs(c, rule.AllowConnection[0].PlugAttributes, "pa3", "PA3")
	checkAttrs(c, rule.AllowConnection[0].SlotAttributes, "sa3", "SA3")
	c.Assert(rule.DenyConnection, check.HasLen, 1)
	checkAttrs(c, rule.DenyConnection[0].PlugAttributes, "pa4", "PA4")
	checkAttrs(c, rule.DenyConnection[0].SlotAttributes, "sa4", "SA4")
	// auto-connection subrules
	c.Assert(rule.AllowAutoConnection, check.HasLen, 1)
	checkAttrs(c, rule.AllowAutoConnection[0].PlugAttributes, "pa5", "PA5")
	checkAttrs(c, rule.AllowAutoConnection[0].SlotAttributes, "sa5", "SA5")
	c.Assert(rule.DenyAutoConnection, check.HasLen, 1)
	checkAttrs(c, rule.DenyAutoConnection[0].PlugAttributes, "pa6", "PA6")
	checkAttrs(c, rule.DenyAutoConnection[0].SlotAttributes, "sa6", "SA6")
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleAllAllowDenyOrStanzas(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-installation:
    -
      plug-attributes:
        a1: A1
    -
      plug-attributes:
        a1: A1alt
  deny-installation:
    -
      plug-attributes:
        a2: A2
    -
      plug-attributes:
        a2: A2alt
  allow-connection:
    -
      plug-attributes:
        pa3: PA3
      slot-attributes:
        sa3: SA3
    -
      plug-attributes:
        pa3: PA3alt
  deny-connection:
    -
      plug-attributes:
        pa4: PA4
      slot-attributes:
        sa4: SA4
    -
      plug-attributes:
        pa4: PA4alt
  allow-auto-connection:
    -
      plug-attributes:
        pa5: PA5
      slot-attributes:
        sa5: SA5
    -
      plug-attributes:
        pa5: PA5alt
  deny-auto-connection:
    -
      plug-attributes:
        pa6: PA6
      slot-attributes:
        sa6: SA6
    -
      plug-attributes:
        pa6: PA6alt`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.Interface, check.Equals, "iface")
	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 2)
	checkAttrs(c, rule.AllowInstallation[0].PlugAttributes, "a1", "A1")
	checkAttrs(c, rule.AllowInstallation[1].PlugAttributes, "a1", "A1alt")
	c.Assert(rule.DenyInstallation, check.HasLen, 2)
	checkAttrs(c, rule.DenyInstallation[0].PlugAttributes, "a2", "A2")
	checkAttrs(c, rule.DenyInstallation[1].PlugAttributes, "a2", "A2alt")
	// connection subrules
	c.Assert(rule.AllowConnection, check.HasLen, 2)
	checkAttrs(c, rule.AllowConnection[0].PlugAttributes, "pa3", "PA3")
	checkAttrs(c, rule.AllowConnection[0].SlotAttributes, "sa3", "SA3")
	checkAttrs(c, rule.AllowConnection[1].PlugAttributes, "pa3", "PA3alt")
	c.Assert(rule.DenyConnection, check.HasLen, 2)
	checkAttrs(c, rule.DenyConnection[0].PlugAttributes, "pa4", "PA4")
	checkAttrs(c, rule.DenyConnection[0].SlotAttributes, "sa4", "SA4")
	checkAttrs(c, rule.DenyConnection[1].PlugAttributes, "pa4", "PA4alt")
	// auto-connection subrules
	c.Assert(rule.AllowAutoConnection, check.HasLen, 2)
	checkAttrs(c, rule.AllowAutoConnection[0].PlugAttributes, "pa5", "PA5")
	checkAttrs(c, rule.AllowAutoConnection[0].SlotAttributes, "sa5", "SA5")
	checkAttrs(c, rule.AllowAutoConnection[1].PlugAttributes, "pa5", "PA5alt")
	c.Assert(rule.DenyAutoConnection, check.HasLen, 2)
	checkAttrs(c, rule.DenyAutoConnection[0].PlugAttributes, "pa6", "PA6")
	checkAttrs(c, rule.DenyAutoConnection[0].SlotAttributes, "sa6", "SA6")
	checkAttrs(c, rule.DenyAutoConnection[1].PlugAttributes, "pa6", "PA6alt")
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleShortcutTrue(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", "true")
	c.Assert(err, check.IsNil)

	c.Check(rule.Interface, check.Equals, "iface")
	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	c.Check(rule.AllowInstallation[0].PlugAttributes, check.Equals, asserts.AlwaysMatchAttributes)
	c.Assert(rule.DenyInstallation, check.HasLen, 1)
	c.Check(rule.DenyInstallation[0].PlugAttributes, check.Equals, asserts.NeverMatchAttributes)
	// connection subrules
	checkBoolPlugConnConstraints(c, "allow-connection", rule.AllowConnection, true)
	checkBoolPlugConnConstraints(c, "deny-connection", rule.DenyConnection, false)
	// auto-connection subrules
	checkBoolPlugConnConstraints(c, "allow-auto-connection", rule.AllowAutoConnection, true)
	checkBoolPlugConnConstraints(c, "deny-auto-connection", rule.DenyAutoConnection, false)
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleShortcutFalse(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", "false")
	c.Assert(err, check.IsNil)

	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	c.Check(rule.AllowInstallation[0].PlugAttributes, check.Equals, asserts.NeverMatchAttributes)
	c.Assert(rule.DenyInstallation, check.HasLen, 1)
	c.Check(rule.DenyInstallation[0].PlugAttributes, check.Equals, asserts.AlwaysMatchAttributes)
	// connection subrules
	checkBoolPlugConnConstraints(c, "allow-connection", rule.AllowConnection, false)
	checkBoolPlugConnConstraints(c, "deny-connection", rule.DenyConnection, true)
	// auto-connection subrules
	checkBoolPlugConnConstraints(c, "allow-auto-connection", rule.AllowAutoConnection, false)
	checkBoolPlugConnConstraints(c, "deny-auto-connection", rule.DenyAutoConnection, true)
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleDefaults(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", map[string]interface{}{
		"deny-auto-connection": "true",
	})
	c.Assert(err, check.IsNil)

	// everything follows the defaults...

	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	c.Check(rule.AllowInstallation[0].PlugAttributes, check.Equals, asserts.AlwaysMatchAttributes)
	c.Assert(rule.DenyInstallation, check.HasLen, 1)
	c.Check(rule.DenyInstallation[0].PlugAttributes, check.Equals, asserts.NeverMatchAttributes)
	// connection subrules
	checkBoolPlugConnConstraints(c, "allow-connection", rule.AllowConnection, true)
	checkBoolPlugConnConstraints(c, "deny-connection", rule.DenyConnection, false)
	// auto-connection subrules
	checkBoolPlugConnConstraints(c, "allow-auto-connection", rule.AllowAutoConnection, true)
	// ... but deny-auto-connection is on
	checkBoolPlugConnConstraints(c, "deny-auto-connection", rule.DenyAutoConnection, true)
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleInstalationConstraintsIDConstraints(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", map[string]interface{}{
		"allow-installation": map[string]interface{}{
			"plug-sdk-type": []interface{}{sdk.System.String(), sdk.Regular.String()},
		},
	})
	c.Assert(err, check.IsNil)

	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	cstrs := rule.AllowInstallation[0]
	c.Check(cstrs.PlugSdkTypes, check.DeepEquals, []string{sdk.System.String(), "regular"})
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleInstallationConstraintsPlugNames(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-installation: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.AllowInstallation[0].PlugNames, check.IsNil)

	tests := []struct {
		rule        string
		matching    []string
		notMatching []string
	}{
		{`iface:
  allow-installation:
    plug-names:
      - foo`, []string{"foo"}, []string{"bar"}},
		{`iface:
  allow-installation:
    plug-names:
      - foo
      - bar`, []string{"foo", "bar"}, []string{"baz"}},
		{`iface:
  allow-installation:
    plug-names:
      - foo[0-9]
      - bar`, []string{"foo0", "foo1", "bar"}, []string{"baz", "fooo", "foo12"}},
	}
	for _, t := range tests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		for _, matching := range t.matching {
			c.Check(rule.AllowInstallation[0].PlugNames.Check("plug name", matching, nil), check.IsNil)
		}
		for _, notMatching := range t.notMatching {
			c.Check(rule.AllowInstallation[0].PlugNames.Check("plug name", notMatching, nil), check.NotNil)
		}
	}
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleConnectionConstraintsIDConstraints(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", map[string]interface{}{
		"allow-connection": map[string]interface{}{
			"plug-sdk-type": []interface{}{sdk.Regular.String()},
			"slot-sdk-type": []interface{}{sdk.System.String(), sdk.Regular.String()},
		},
	})
	c.Assert(err, check.IsNil)

	c.Assert(rule.AllowConnection, check.HasLen, 1)
	cstrs := rule.AllowConnection[0]
	c.Check(cstrs.PlugSdkTypes, check.DeepEquals, []string{sdk.Regular.String()})
	c.Check(cstrs.SlotSdkTypes, check.DeepEquals, []string{sdk.System.String(), sdk.Regular.String()})
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleConnectionConstraintsPlugNamesSlotNames(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-connection: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.AllowConnection[0].PlugNames, check.IsNil)
	c.Check(rule.AllowConnection[0].SlotNames, check.IsNil)

	tests := []struct {
		rule        string
		matching    []string
		notMatching []string
	}{
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo
    slot-names:
      - Sfoo`, []string{"foo"}, []string{"bar"}},
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo
      - Pbar
    slot-names:
      - Sfoo
      - Sbar`, []string{"foo", "bar"}, []string{"baz"}},
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo[0-9]
      - Pbar
    slot-names:
      - Sfoo[0-9]
      - Sbar`, []string{"foo0", "foo1", "bar"}, []string{"baz", "fooo", "foo12"}},
	}
	for _, t := range tests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		for _, matching := range t.matching {
			c.Check(rule.AllowConnection[0].PlugNames.Check("plug name", "P"+matching, nil), check.IsNil)

			c.Check(rule.AllowConnection[0].SlotNames.Check("slot name", "S"+matching, nil), check.IsNil)
		}

		for _, notMatching := range t.notMatching {
			c.Check(rule.AllowConnection[0].SlotNames.Check("plug name", "P"+notMatching, nil), check.NotNil)

			c.Check(rule.AllowConnection[0].SlotNames.Check("slot name", "S"+notMatching, nil), check.NotNil)
		}
	}
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleConnectionConstraintsSideArityConstraints(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-auto-connection: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	// defaults
	c.Check(rule.AllowAutoConnection[0].SlotsPerPlug, check.Equals, asserts.SideArityConstraint{N: 1})
	c.Check(rule.AllowAutoConnection[0].PlugsPerSlot.Any(), check.Equals, true)

	c.Check(rule.AllowConnection[0].SlotsPerPlug.Any(), check.Equals, true)
	c.Check(rule.AllowConnection[0].PlugsPerSlot.Any(), check.Equals, true)

	// test that the arity constraints get normalized away to any
	// under allow-connection
	// see https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
	allowConnTests := []string{
		`iface:
  allow-connection:
    slots-per-plug: 1
    plugs-per-slot: 2`,
		`iface:
  allow-connection:
    slots-per-plug: *
    plugs-per-slot: 1`,
		`iface:
  allow-connection:
    slots-per-plug: 2
    plugs-per-slot: *`,
	}

	for _, t := range allowConnTests {
		m, err = asserts.ParseHeaders([]byte(t))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		c.Check(rule.AllowConnection[0].SlotsPerPlug.Any(), check.Equals, true)
		c.Check(rule.AllowConnection[0].PlugsPerSlot.Any(), check.Equals, true)
	}

	// test that under allow-auto-connection:
	// slots-per-plug can be * (any) or otherwise gets normalized to 1
	// plugs-per-slot gets normalized to any (*)
	// see https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
	allowAutoConnTests := []struct {
		rule         string
		slotsPerPlug asserts.SideArityConstraint
	}{
		{`iface:
  allow-auto-connection:
    slots-per-plug: 1
    plugs-per-slot: 2`, sideArityOne},
		{`iface:
  allow-auto-connection:
    slots-per-plug: *
    plugs-per-slot: 1`, sideArityAny},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 2
    plugs-per-slot: *`, sideArityOne},
	}

	for _, t := range allowAutoConnTests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompilePlugRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		c.Check(rule.AllowAutoConnection[0].SlotsPerPlug, check.Equals, t.slotsPerPlug)
		c.Check(rule.AllowAutoConnection[0].PlugsPerSlot.Any(), check.Equals, true)
	}
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleConnectionConstraintsAttributesDefault(c *check.C) {
	rule, err := asserts.CompilePlugRule("iface", map[string]interface{}{
		"allow-connection": map[string]interface{}{
			"slot-sdk-type": []interface{}{"regular"},
		},
	})
	c.Assert(err, check.IsNil)

	// attributes default to always matching here
	cstrs := rule.AllowConnection[0]
	c.Check(cstrs.PlugAttributes, check.Equals, asserts.AlwaysMatchAttributes)
	c.Check(cstrs.SlotAttributes, check.Equals, asserts.AlwaysMatchAttributes)
}

func (s *plugSlotRulesSuite) TestCompilePlugRuleErrors(c *check.C) {
	tests := []struct {
		stanza string
		err    string
	}{
		{`iface: foo`, `plug rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  - allow`, `plug rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-installation: foo`, `allow-installation in plug rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  deny-installation: foo`, `deny-installation in plug rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-connection: foo`, `allow-connection in plug rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-connection:
    - foo`, `alternative 1 of allow-connection in plug rule for interface "iface" must be a map`},
		{`iface:
  allow-connection:
    - true`, `alternative 1 of allow-connection in plug rule for interface "iface" must be a map`},
		{`iface:
  allow-installation:
    plug-attributes:
      a1: [`, `cannot compile plug-attributes in allow-installation in plug rule for interface "iface": cannot compile "a1" constraint .*`},
		{`iface:
  allow-connection:
    slot-attributes:
      a2: [`, `cannot compile slot-attributes in allow-connection in plug rule for interface "iface": cannot compile "a2" constraint .*`},
		{`iface:
  allow-connection:
    slot-sdk-type:
      - foo`, `slot-sdk-type in allow-connection in plug rule for interface "iface" contains an invalid element: "foo"`},
		{`iface:
  allow-connection:
    slot-sdk-type:
      - xapp`, `slot-sdk-type in allow-connection in plug rule for interface "iface" contains an invalid element: "xapp"`},
		{`iface:
  allow-connect: true`, `plug rule for interface "iface" must specify at least one of allow-installation, deny-installation, allow-connection, deny-connection, allow-auto-connection, deny-auto-connection`},
		{`iface:
  allow-installation:
    slots-per-plug: 1`, `allow-installation in plug rule for interface "iface" cannot specify a slots-per-plug constraint, they apply only to allow-\*connection`},
		{`iface:
  deny-connection:
    slots-per-plug: 1`, `deny-connection in plug rule for interface "iface" cannot specify a slots-per-plug constraint, they apply only to allow-\*connection`},
		{`iface:
  allow-auto-connection:
    plugs-per-slot: any`, `plugs-per-slot in allow-auto-connection in plug rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 00`, `slots-per-plug in allow-auto-connection in plug rule for interface "iface" has invalid prefix zeros: 00`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 99999999999999999999`, `slots-per-plug in allow-auto-connection in plug rule for interface "iface" is out of range: 99999999999999999999`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 0`, `slots-per-plug in allow-auto-connection in plug rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    slots-per-plug:
      what: 1`, `slots-per-plug in allow-auto-connection in plug rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    plug-names: true`, `plug-names constraints must be a list of regexps and special \$ values`},
		{`iface:
  allow-auto-connection:
    slot-names: true`, `slot-names constraints must be a list of regexps and special \$ values`},
	}

	for _, t := range tests {
		m, err := asserts.ParseHeaders([]byte(t.stanza))
		c.Assert(err, check.IsNil, check.Commentf(t.stanza))

		_, err = asserts.CompilePlugRule("iface", m["iface"])
		c.Check(err, check.ErrorMatches, t.err, check.Commentf(t.stanza))
	}
}

func (s *plugSlotRulesSuite) TestPlugRuleFeatures(c *check.C) {
	combos := []struct {
		subrule             string
		constraintsPrefixes []string
	}{
		{"allow-installation", []string{"plug-"}},
		{"deny-installation", []string{"plug-"}},
		{"allow-connection", []string{"plug-", "slot-"}},
		{"deny-connection", []string{"plug-", "slot-"}},
		{"allow-auto-connection", []string{"plug-", "slot-"}},
		{"deny-auto-connection", []string{"plug-", "slot-"}},
	}

	for _, combo := range combos {
		for _, attrConstrPrefix := range combo.constraintsPrefixes {
			attrConstraintMap := map[string]interface{}{
				"a":     "ATTR",
				"other": []interface{}{"x", "y"},
			}
			ruleMap := map[string]interface{}{
				combo.subrule: map[string]interface{}{
					attrConstrPrefix + "attributes": attrConstraintMap,
				},
			}

			rule, err := asserts.CompilePlugRule("iface", ruleMap)
			c.Assert(err, check.IsNil)
			c.Check(asserts.RuleFeature(rule, "dollar-attr-constraints"), check.Equals, false, check.Commentf("%v", ruleMap))

			c.Check(asserts.RuleFeature(rule, "device-scope-constraints"), check.Equals, false, check.Commentf("%v", ruleMap))
			c.Check(asserts.RuleFeature(rule, "name-constraints"), check.Equals, false, check.Commentf("%v", ruleMap))

			attrConstraintMap["a"] = "$MISSING"
			rule, err = asserts.CompilePlugRule("iface", ruleMap)
			c.Assert(err, check.IsNil)
			c.Check(asserts.RuleFeature(rule, "dollar-attr-constraints"), check.Equals, true, check.Commentf("%v", ruleMap))

			// covers also alternation
			attrConstraintMap["a"] = []interface{}{"$SLOT(a)"}
			rule, err = asserts.CompilePlugRule("iface", ruleMap)
			c.Assert(err, check.IsNil)
			c.Check(asserts.RuleFeature(rule, "dollar-attr-constraints"), check.Equals, true, check.Commentf("%v", ruleMap))

			c.Check(asserts.RuleFeature(rule, "device-scope-constraints"), check.Equals, false, check.Commentf("%v", ruleMap))
			c.Check(asserts.RuleFeature(rule, "name-constraints"), check.Equals, false, check.Commentf("%v", ruleMap))

		}

		for _, nameConstrPrefix := range combo.constraintsPrefixes {
			ruleMap := map[string]interface{}{
				combo.subrule: map[string]interface{}{
					nameConstrPrefix + "names": []interface{}{"foo"},
				},
			}

			rule, err := asserts.CompilePlugRule("iface", ruleMap)
			c.Assert(err, check.IsNil)
			c.Check(asserts.RuleFeature(rule, "name-constraints"), check.Equals, true, check.Commentf("%v", ruleMap))
		}
	}
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleInstallationConstraintsSlotNames(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-installation: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.AllowInstallation[0].SlotNames, check.IsNil)

	tests := []struct {
		rule        string
		matching    []string
		notMatching []string
	}{
		{`iface:
  allow-installation:
    slot-names:
      - foo`, []string{"foo"}, []string{"bar"}},
		{`iface:
  allow-installation:
    slot-names:
      - foo
      - bar`, []string{"foo", "bar"}, []string{"baz"}},
		{`iface:
  allow-installation:
    slot-names:
      - foo[0-9]
      - bar`, []string{"foo0", "foo1", "bar"}, []string{"baz", "fooo", "foo12"}},
	}
	for _, t := range tests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		for _, matching := range t.matching {
			c.Check(rule.AllowInstallation[0].SlotNames.Check("slot name", matching, nil), check.IsNil)
		}
		for _, notMatching := range t.notMatching {
			c.Check(rule.AllowInstallation[0].SlotNames.Check("slot name", notMatching, nil), check.NotNil)
		}
	}
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleConnectionConstraintsPlugNamesSlotNames(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-connection: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	c.Check(rule.AllowConnection[0].PlugNames, check.IsNil)
	c.Check(rule.AllowConnection[0].SlotNames, check.IsNil)

	tests := []struct {
		rule        string
		matching    []string
		notMatching []string
	}{
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo
    slot-names:
      - Sfoo`, []string{"foo"}, []string{"bar"}},
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo
      - Pbar
    slot-names:
      - Sfoo
      - Sbar`, []string{"foo", "bar"}, []string{"baz"}},
		{`iface:
  allow-connection:
    plug-names:
      - Pfoo[0-9]
      - Pbar
    slot-names:
      - Sfoo[0-9]
      - Sbar`, []string{"foo0", "foo1", "bar"}, []string{"baz", "fooo", "foo12"}},
	}
	for _, t := range tests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		for _, matching := range t.matching {
			c.Check(rule.AllowConnection[0].PlugNames.Check("plug name", "P"+matching, nil), check.IsNil)

			c.Check(rule.AllowConnection[0].SlotNames.Check("slot name", "S"+matching, nil), check.IsNil)
		}

		for _, notMatching := range t.notMatching {
			c.Check(rule.AllowConnection[0].SlotNames.Check("plug name", "P"+notMatching, nil), check.NotNil)

			c.Check(rule.AllowConnection[0].SlotNames.Check("slot name", "S"+notMatching, nil), check.NotNil)
		}
	}
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleInstallationConstraintsIDConstraints(c *check.C) {
	rule, err := asserts.CompileSlotRule("iface", map[string]interface{}{
		"allow-installation": map[string]interface{}{
			"slot-sdk-type": []interface{}{sdk.System.String(), sdk.Regular.String()},
		},
	})
	c.Assert(err, check.IsNil)

	c.Assert(rule.AllowInstallation, check.HasLen, 1)
	cstrs := rule.AllowInstallation[0]
	c.Check(cstrs.SlotSdkTypes, check.DeepEquals, []string{sdk.System.String(), sdk.Regular.String()})
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleConnectionConstraintsIDConstraints(c *check.C) {
	rule, err := asserts.CompileSlotRule("iface", map[string]interface{}{
		"allow-connection": map[string]interface{}{
			"plug-sdk-type": []interface{}{sdk.Regular.String()},
			"slot-sdk-type": []interface{}{sdk.System.String()},
		},
	})
	c.Assert(err, check.IsNil)

	c.Assert(rule.AllowConnection, check.HasLen, 1)
	cstrs := rule.AllowConnection[0]
	c.Check(cstrs.PlugSdkTypes, check.DeepEquals, []string{sdk.Regular.String()})
	c.Check(cstrs.SlotSdkTypes, check.DeepEquals, []string{sdk.System.String()})
}

func checkBoolSlotConnConstraints(c *check.C, subrule string, cstrs []*asserts.SlotConnectionConstraints) {
	c.Assert(cstrs, check.HasLen, 1)
	cstrs1 := cstrs[0]
	if strings.HasPrefix(subrule, "deny-") {
		undef := asserts.SideArityConstraint{}
		c.Check(cstrs1.SlotsPerPlug, check.Equals, undef)
		c.Check(cstrs1.PlugsPerSlot, check.Equals, undef)
	} else {
		c.Check(cstrs1.PlugsPerSlot, check.Equals, sideArityAny)
		if strings.HasSuffix(subrule, "-auto-connection") {
			c.Check(cstrs1.SlotsPerPlug, check.Equals, sideArityOne)
		} else {
			c.Check(cstrs1.SlotsPerPlug, check.Equals, sideArityAny)
		}
	}
	c.Check(cstrs1.PlugSdkTypes, check.HasLen, 0)
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleDefaults(c *check.C) {
	rule, err := asserts.CompileSlotRule("iface", map[string]interface{}{
		"deny-auto-connection": "true",
	})
	c.Assert(err, check.IsNil)

	// everything follows the defaults...

	// install subrules
	c.Assert(rule.AllowInstallation, check.HasLen, 1)

	// connection subrules
	checkBoolSlotConnConstraints(c, "allow-connection", rule.AllowConnection)
	checkBoolSlotConnConstraints(c, "deny-connection", rule.DenyConnection)
	// auto-connection subrules
	checkBoolSlotConnConstraints(c, "allow-auto-connection", rule.AllowAutoConnection)
	// ... but deny-auto-connection is on
	checkBoolSlotConnConstraints(c, "deny-auto-connection", rule.DenyAutoConnection)
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleConnectionConstraintsSideArityConstraints(c *check.C) {
	m, err := asserts.ParseHeaders([]byte(`iface:
  allow-auto-connection: true`))
	c.Assert(err, check.IsNil)

	rule, err := asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
	c.Assert(err, check.IsNil)

	// defaults
	c.Check(rule.AllowAutoConnection[0].SlotsPerPlug, check.Equals, asserts.SideArityConstraint{N: 1})
	c.Check(rule.AllowAutoConnection[0].PlugsPerSlot.Any(), check.Equals, true)

	c.Check(rule.AllowConnection[0].SlotsPerPlug.Any(), check.Equals, true)
	c.Check(rule.AllowConnection[0].PlugsPerSlot.Any(), check.Equals, true)

	// test that the arity constraints get normalized away to any
	// under allow-connection
	// see https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
	allowConnTests := []string{
		`iface:
  allow-connection:
    slots-per-plug: 1
    plugs-per-slot: 2`,
		`iface:
  allow-connection:
    slots-per-plug: *
    plugs-per-slot: 1`,
		`iface:
  allow-connection:
    slots-per-plug: 2
    plugs-per-slot: *`,
	}

	for _, t := range allowConnTests {
		m, err = asserts.ParseHeaders([]byte(t))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		c.Check(rule.AllowConnection[0].SlotsPerPlug.Any(), check.Equals, true)
		c.Check(rule.AllowConnection[0].PlugsPerSlot.Any(), check.Equals, true)
	}

	// test that under allow-auto-connection:
	// slots-per-plug can be * (any) or otherwise gets normalized to 1
	// plugs-per-slot gets normalized to any (*)
	// see https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
	allowAutoConnTests := []struct {
		rule         string
		slotsPerPlug asserts.SideArityConstraint
	}{
		{`iface:
  allow-auto-connection:
    slots-per-plug: 1
    plugs-per-slot: 2`, sideArityOne},
		{`iface:
  allow-auto-connection:
    slots-per-plug: *
    plugs-per-slot: 1`, sideArityAny},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 2
    plugs-per-slot: *`, sideArityOne},
	}

	for _, t := range allowAutoConnTests {
		m, err = asserts.ParseHeaders([]byte(t.rule))
		c.Assert(err, check.IsNil)

		rule, err = asserts.CompileSlotRule("iface", m["iface"].(map[string]interface{}))
		c.Assert(err, check.IsNil)

		c.Assert(rule.AllowAutoConnection[0].SlotsPerPlug, check.Equals, t.slotsPerPlug)
		c.Check(rule.AllowAutoConnection[0].PlugsPerSlot.Any(), check.Equals, true)
	}
}

func (s *plugSlotRulesSuite) TestCompileSlotRuleErrors(c *check.C) {
	tests := []struct {
		stanza string
		err    string
	}{
		{`iface: foo`, `slot rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  - allow`, `slot rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-installation: foo`, `allow-installation in slot rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  deny-installation: foo`, `deny-installation in slot rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-connection: foo`, `allow-connection in slot rule for interface "iface" must be a map or one of the shortcuts 'true' or 'false'`},
		{`iface:
  allow-connection:
    - foo`, `alternative 1 of allow-connection in slot rule for interface "iface" must be a map`},
		{`iface:
  allow-connection:
    - true`, `alternative 1 of allow-connection in slot rule for interface "iface" must be a map`},
		{`iface:
  allow-connection:
    plug-sdk-type:
      - foo`, `plug-sdk-type in allow-connection in slot rule for interface "iface" contains an invalid element: "foo"`},
		{`iface:
  allow-connection:
    plug-sdk-type:
      - xapp`, `plug-sdk-type in allow-connection in slot rule for interface "iface" contains an invalid element: "xapp"`},
		{`iface:
  allow-connection:
    plug-sdk-ids:
      - foo`, `allow-connection in slot rule for interface "iface" must specify at least one of plug-names, slot-names, plug-attributes, slot-attributes, plug-sdk-type, slot-sdk-type, slots-per-plug, plugs-per-slot`},
		{`iface:
  deny-connection:
    plug-sdk-ids:
        - foo`, `deny-connection in slot rule for interface "iface" must specify at least one of plug-names, slot-names, plug-attributes, slot-attributes, plug-sdk-type, slot-sdk-type, slots-per-plug, plugs-per-slot`},
		{`iface:
  allow-connect: true`, `slot rule for interface "iface" must specify at least one of allow-installation, deny-installation, allow-connection, deny-connection, allow-auto-connection, deny-auto-connection`},
		{`iface:
  allow-installation:
    slots-per-plug: 1`, `allow-installation in slot rule for interface "iface" cannot specify a slots-per-plug constraint, they apply only to allow-\*connection`},
		{`iface:
  deny-auto-connection:
    slots-per-plug: 1`, `deny-auto-connection in slot rule for interface "iface" cannot specify a slots-per-plug constraint, they apply only to allow-\*connection`},
		{`iface:
  deny-auto-connection:
    plug-sdk-ids:
      - foo`, `deny-auto-connection in slot rule for interface "iface" must specify at least one of plug-names, slot-names, plug-attributes, slot-attributes, plug-sdk-type, slot-sdk-type, slots-per-plug, plugs-per-slot`},
		{`iface:
  allow-auto-connection:
    plugs-per-slot: any`, `plugs-per-slot in allow-auto-connection in slot rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 00`, `slots-per-plug in allow-auto-connection in slot rule for interface "iface" has invalid prefix zeros: 00`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 99999999999999999999`, `slots-per-plug in allow-auto-connection in slot rule for interface "iface" is out of range: 99999999999999999999`},
		{`iface:
  allow-auto-connection:
    slots-per-plug: 0`, `slots-per-plug in allow-auto-connection in slot rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    slots-per-plug:
      what: 1`, `slots-per-plug in allow-auto-connection in slot rule for interface "iface" must be an integer >=1 or \*`},
		{`iface:
  allow-auto-connection:
    slot-names: true`, `slot-names constraints must be a list of regexps and special \$ values`},
		{`iface:
  allow-auto-connection:
    plug-sdk-ids:
      - foo`, `allow-auto-connection in slot rule for interface "iface" must specify at least one of plug-names, slot-names, plug-attributes, slot-attributes, plug-sdk-type, slot-sdk-type, slots-per-plug, plugs-per-slot`},
	}

	for i, t := range tests {
		m, err := asserts.ParseHeaders([]byte(t.stanza))
		c.Assert(err, check.IsNil, check.Commentf(t.stanza))
		_, err = asserts.CompileSlotRule("iface", m["iface"])
		c.Assert(err, check.ErrorMatches, t.err, check.Commentf("%d: %s", i, t.stanza))
	}
}

type nameConstraintsSuite struct{}

func (s *nameConstraintsSuite) TestCompileErrors(c *check.C) {
	_, err := asserts.CompileNameConstraints("slot-names", "true")
	c.Check(err, check.ErrorMatches, `slot-names constraints must be a list of regexps and special \$ values`)

	_, err = asserts.CompileNameConstraints("slot-names", []interface{}{map[string]interface{}{"foo": "bar"}})
	c.Check(err, check.ErrorMatches, `slot-names constraint entry must be a regexp or special \$ value`)

	_, err = asserts.CompileNameConstraints("plug-names", []interface{}{"["})
	c.Check(err, check.ErrorMatches, `cannot compile plug-names constraint entry "\[":.*`)

	_, err = asserts.CompileNameConstraints("plug-names", []interface{}{"$"})
	c.Check(err, check.ErrorMatches, `plug-names constraint entry special value "\$" is invalid`)

	_, err = asserts.CompileNameConstraints("slot-names", []interface{}{"$12"})
	c.Check(err, check.ErrorMatches, `slot-names constraint entry special value "\$12" is invalid`)

	_, err = asserts.CompileNameConstraints("plug-names", []interface{}{"a b"})
	c.Check(err, check.ErrorMatches, `plug-names constraint entry regexp contains unexpected spaces`)
}

func (s *nameConstraintsSuite) TestCheck(c *check.C) {
	nc, err := asserts.CompileNameConstraints("slot-names", []interface{}{"foo[0-9]", "bar"})
	c.Assert(err, check.IsNil)

	for _, matching := range []string{"foo0", "foo1", "bar"} {
		c.Check(nc.Check("slot name", matching, nil), check.IsNil)
	}

	for _, notMatching := range []string{"baz", "fooo", "foo12"} {
		c.Check(nc.Check("slot name", notMatching, nil), check.ErrorMatches, fmt.Sprintf(`slot name %q does not match constraints`, notMatching))
	}

}

func (s *nameConstraintsSuite) TestCheckSpecial(c *check.C) {
	nc, err := asserts.CompileNameConstraints("slot-names", []interface{}{"$INTERFACE"})
	c.Assert(err, check.IsNil)

	c.Check(nc.Check("slot name", "foo", nil), check.ErrorMatches, `slot name "foo" does not match constraints`)
	c.Check(nc.Check("slot name", "foo", map[string]string{"$INTERFACE": "foo"}), check.IsNil)
	c.Check(nc.Check("slot name", "bar", map[string]string{"$INTERFACE": "foo"}), check.ErrorMatches, `slot name "bar" does not match constraints`)
}
