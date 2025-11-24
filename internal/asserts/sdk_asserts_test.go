package asserts_test

import (
	"strings"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/asserts"
	"github.com/canonical/workshop/internal/sdk"
)

var _ = check.Suite(&baseDeclSuite{})

type baseDeclSuite struct{}

func (s *baseDeclSuite) TestDecodeOK(c *check.C) {
	encoded := `type: base-declaration
authority-id: canonical
series: 1
plugs:
  interface1:
    deny-installation: false
    allow-auto-connection:
      slot-sdk-type:
        - system
      slot-attributes:
        a1: /foo/.*
      plug-attributes:
        b1: B1
    deny-auto-connection:
      slot-attributes:
        a1: !A1
      plug-attributes:
        b1: !B1
  interface2:
    allow-installation: true
    allow-connection:
      plug-attributes:
        a2: A2
      slot-attributes:
        b2: B2
    deny-connection:
      plug-attributes:
        a2: !A2
      slot-attributes:
        b2: !B2
slots:
  interface3:
    deny-installation: false
    allow-auto-connection:
      plug-sdk-type:
        - system
      slot-attributes:
        c1: /foo/.*
      plug-attributes:
        d1: C1
    deny-auto-connection:
      slot-attributes:
        c1: !C1
      plug-attributes:
        d1: !D1
  interface4:
    allow-connection:
      plug-attributes:
        c2: C2
      slot-attributes:
        d2: D2
    deny-connection:
      plug-attributes:
        c2: !D2
      slot-attributes:
        d2: !D2
    allow-installation:
      slot-sdk-type:
        - system
        - regular
      slot-attributes:
        e1: E1
timestamp: 2016-09-29T19:50:49Z
sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij

AXNpZw==`
	a, err := asserts.Decode([]byte(encoded))
	c.Assert(err, check.IsNil)
	baseDecl := a.(*asserts.BaseDeclaration)
	c.Check(baseDecl.Series(), check.Equals, "1")
	ts, err := time.Parse(time.RFC3339, "2016-09-29T19:50:49Z")
	c.Assert(err, check.IsNil)
	c.Check(baseDecl.Timestamp().Equal(ts), check.Equals, true)

	c.Check(baseDecl.PlugRule("interfaceX"), check.IsNil)
	c.Check(baseDecl.SlotRule("interfaceX"), check.IsNil)

	slotRule3 := baseDecl.SlotRule("interface3")
	c.Assert(slotRule3, check.NotNil)
	c.Assert(slotRule3.DenyInstallation, check.HasLen, 1)
	c.Assert(slotRule3.AllowAutoConnection, check.HasLen, 1)
	c.Check(slotRule3.AllowAutoConnection[0].PlugSdkTypes, check.DeepEquals, []string{sdk.System.String()})

	slotRule4 := baseDecl.SlotRule("interface4")
	c.Assert(slotRule4, check.NotNil)
	c.Check(slotRule4.AllowInstallation[0].SlotSdkTypes, check.DeepEquals, []string{sdk.System.String(), "regular"})
	c.Assert(slotRule4.DenyInstallation, check.HasLen, 1)
	c.Assert(slotRule3.AllowConnection, check.HasLen, 1)
	c.Assert(slotRule3.DenyConnection, check.HasLen, 1)
}

const (
	baseDeclErrPrefix = "assertion base-declaration: "
)

func (s *baseDeclSuite) TestDecodeInvalid(c *check.C) {
	tsLine := "timestamp: 2016-09-29T19:50:49Z\n"

	encoded := "type: base-declaration\n" +
		"authority-id: canonical\n" +
		"series: 16\n" +
		"plugs:\n  interface1: true\n" +
		"slots:\n  interface2: true\n" +
		tsLine +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
		"\n\n" +
		"AXNpZw=="

	invalidTests := []struct{ original, invalid, expectedErr string }{
		{"series: 16\n", "", `"series" header is mandatory`},
		{"series: 16\n", "series: \n", `"series" header should not be empty`},
		{"plugs:\n  interface1: true\n", "plugs: \n", `"plugs" header must be a map`},
		{"plugs:\n  interface1: true\n", "plugs:\n  intf1:\n    foo: bar\n", `plug rule for interface "intf1" must specify at least one of.*`},
		{"slots:\n  interface2: true\n", "slots: \n", `"slots" header must be a map`},
		{"slots:\n  interface2: true\n", "slots:\n  intf1:\n    foo: bar\n", `slot rule for interface "intf1" must specify at least one of.*`},
		{tsLine, "", `"timestamp" header is mandatory`},
		{tsLine, "timestamp: 12:30\n", `"timestamp" header is not a RFC3339 date: .*`},
	}

	for _, test := range invalidTests {
		invalid := strings.Replace(encoded, test.original, test.invalid, 1)
		_, err := asserts.Decode([]byte(invalid))
		c.Check(err, check.ErrorMatches, baseDeclErrPrefix+test.expectedErr)
	}
}

func (s *baseDeclSuite) TestBuiltin(c *check.C) {
	baseDecl := asserts.BuiltinBaseDeclaration()
	c.Check(baseDecl, check.IsNil)

	defer asserts.InitBuiltinBaseDeclaration(nil)

	const headers = `
type: base-declaration
authority-id: canonical
series: 1
revision: 0
plugs:
  network: true
slots:
  network:
    allow-installation:
      slot-sdk-type:
        - system
`

	err := asserts.InitBuiltinBaseDeclaration([]byte(headers))
	c.Assert(err, check.IsNil)

	baseDecl = asserts.BuiltinBaseDeclaration()
	c.Assert(baseDecl, check.NotNil)

	cont, _ := baseDecl.Signature()
	c.Check(string(cont), check.Equals, strings.TrimSpace(headers))

	c.Check(baseDecl.AuthorityID(), check.Equals, "canonical")
	c.Check(baseDecl.Series(), check.Equals, "1")
	c.Check(baseDecl.PlugRule("network").AllowAutoConnection[0].SlotAttributes, check.Equals, asserts.AlwaysMatchAttributes)
	c.Check(baseDecl.SlotRule("network").AllowInstallation[0].SlotSdkTypes, check.DeepEquals, []string{sdk.System.String()})

	enc := asserts.Encode(baseDecl)
	// it's expected that it cannot be decoded
	_, err = asserts.Decode(enc)
	c.Check(err, check.NotNil)
}

func (s *baseDeclSuite) TestBuiltinInitErrors(c *check.C) {
	defer asserts.InitBuiltinBaseDeclaration(nil)

	tests := []struct {
		headers string
		err     string
	}{
		{"", `header entry missing ':' separator: ""`},
		{"type: foo\n", `the builtin base-declaration "type" header is not set to expected value "base-declaration"`},
		{"type: base-declaration", `the builtin base-declaration "authority-id" header is not set to expected value "canonical"`},
		{"type: base-declaration\nauthority-id: canonical", `the builtin base-declaration "series" header is not set to expected value "1"`},
		{"type: base-declaration\nauthority-id: canonical\nseries: 1\nrevision: zzz", `cannot assemble the builtin-base declaration: "revision" header is not an integer: zzz`},
		{"type: base-declaration\nauthority-id: canonical\nseries: 1\nplugs: foo", `cannot assemble the builtin base-declaration: "plugs" header must be a map`},
	}

	for _, t := range tests {
		err := asserts.InitBuiltinBaseDeclaration([]byte(t.headers))
		c.Check(err, check.ErrorMatches, t.err, check.Commentf(t.headers))
	}
}
