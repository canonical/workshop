package asserts_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/asserts"
)

type assertsSuite struct{}

var _ = check.Suite(&assertsSuite{})

func TestAsserts(t *testing.T) { check.TestingT(t) }

func (as *assertsSuite) TestType(c *check.C) {
	c.Check(asserts.Type("test-only"), check.Equals, asserts.TestOnlyType)
}

func (as *assertsSuite) TestUnknown(c *check.C) {
	c.Check(asserts.Type(""), check.IsNil)
	c.Check(asserts.Type("unknown"), check.IsNil)
}

func (as *assertsSuite) TestTypeNames(c *check.C) {
	c.Check(asserts.TypeNames(), check.DeepEquals, []string{
		"base-declaration",
		"test-only",
		"test-only-2",
		"test-only-no-authority",
	})
}

func (as *assertsSuite) TestPrimaryKeyHelpers(c *check.C) {
	headers, err := asserts.HeadersFromPrimaryKey(asserts.TestOnlyType, []string{"one"})
	c.Assert(err, check.IsNil)
	c.Check(headers, check.DeepEquals, map[string]string{
		"primary-key": "one",
	})

	headers, err = asserts.HeadersFromPrimaryKey(asserts.TestOnly2Type, []string{"bar", "baz"})
	c.Assert(err, check.IsNil)
	c.Check(headers, check.DeepEquals, map[string]string{
		"pk1": "bar",
		"pk2": "baz",
	})

	_, err = asserts.HeadersFromPrimaryKey(asserts.TestOnly2Type, []string{"bar"})
	c.Check(err, check.ErrorMatches, `primary key has wrong length for "test-only-2" assertion`)

	_, err = asserts.HeadersFromPrimaryKey(asserts.TestOnly2Type, []string{"", "baz"})
	c.Check(err, check.ErrorMatches, `primary key "pk1" header cannot be empty`)

	pk, err := asserts.PrimaryKeyFromHeaders(asserts.TestOnly2Type, headers)
	c.Assert(err, check.IsNil)
	c.Check(pk, check.DeepEquals, []string{"bar", "baz"})

	headers["other"] = "foo"
	pk1, err := asserts.PrimaryKeyFromHeaders(asserts.TestOnly2Type, headers)
	c.Assert(err, check.IsNil)
	c.Check(pk1, check.DeepEquals, pk)

	delete(headers, "pk2")
	_, err = asserts.PrimaryKeyFromHeaders(asserts.TestOnly2Type, headers)
	c.Check(err, check.ErrorMatches, `must provide primary key: pk2`)
}

func (as *assertsSuite) TestPrimaryKeyHelpersOptionalPrimaryKeys(c *check.C) {
	// optional primary key headers
	r := asserts.MockOptionalPrimaryKey(asserts.TestOnlyType, "opt1", "o1-defl")
	defer r()

	pk, err := asserts.PrimaryKeyFromHeaders(asserts.TestOnlyType, map[string]string{"primary-key": "k1"})
	c.Assert(err, check.IsNil)
	c.Check(pk, check.DeepEquals, []string{"k1", "o1-defl"})

	pk, err = asserts.PrimaryKeyFromHeaders(asserts.TestOnlyType, map[string]string{"primary-key": "k1", "opt1": "B"})
	c.Assert(err, check.IsNil)
	c.Check(pk, check.DeepEquals, []string{"k1", "B"})

	hdrs, err := asserts.HeadersFromPrimaryKey(asserts.TestOnlyType, []string{"k1", "B"})
	c.Assert(err, check.IsNil)
	c.Check(hdrs, check.DeepEquals, map[string]string{
		"primary-key": "k1",
		"opt1":        "B",
	})

	hdrs, err = asserts.HeadersFromPrimaryKey(asserts.TestOnlyType, []string{"k1"})
	c.Assert(err, check.IsNil)
	c.Check(hdrs, check.DeepEquals, map[string]string{
		"primary-key": "k1",
		"opt1":        "o1-defl",
	})

	_, err = asserts.HeadersFromPrimaryKey(asserts.TestOnlyType, nil)
	c.Check(err, check.ErrorMatches, `primary key has wrong length for "test-only" assertion`)

	_, err = asserts.HeadersFromPrimaryKey(asserts.TestOnlyType, []string{"pk", "opt1", "what"})
	c.Check(err, check.ErrorMatches, `primary key has wrong length for "test-only" assertion`)
}

func (as *assertsSuite) TestRef(c *check.C) {
	ref := &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz")
}

func (as *assertsSuite) TestRefString(c *check.C) {
	ref := &asserts.Ref{
		Type:       asserts.BaseDeclarationType,
		PrimaryKey: []string{"canonical"},
	}

	c.Check(ref.String(), check.Equals, "base-declaration (canonical)")

	ref2 := &asserts.Ref{
		Type: asserts.TestOnlyNoAuthorityType,
	}

	c.Check(ref2.String(), check.Equals, "test-only-no-authority (-)")
}

func (as *assertsSuite) TestRefResolveError(c *check.C) {
	ref := &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc"},
	}
	_, err := ref.Resolve(nil)
	c.Check(err, check.ErrorMatches, `"test-only-2" assertion reference primary key has the wrong length \(expected \[pk1 pk2\]\): \[abc\]`)
}

func (as *assertsSuite) TestReducePrimaryKey(c *check.C) {
	// optional primary key headers
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt1", "o1-defl")()
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt2", "o2-defl")()

	tests := []struct {
		pk      []string
		reduced []string
	}{
		{nil, nil},
		{[]string{"k1"}, []string{"k1"}},
		{[]string{"k1", "k2"}, []string{"k1", "k2"}},
		{[]string{"k1", "k2", "A"}, []string{"k1", "k2", "A"}},
		{[]string{"k1", "k2", "o1-defl"}, []string{"k1", "k2"}},
		{[]string{"k1", "k2", "A", "o2-defl"}, []string{"k1", "k2", "A"}},
		{[]string{"k1", "k2", "A", "B"}, []string{"k1", "k2", "A", "B"}},
		{[]string{"k1", "k2", "o1-defl", "B"}, []string{"k1", "k2", "o1-defl", "B"}},
		{[]string{"k1", "k2", "o1-defl", "o2-defl"}, []string{"k1", "k2"}},
		{[]string{"k1", "k2", "o1-defl", "o2-defl", "what"}, []string{"k1", "k2", "o1-defl", "o2-defl", "what"}},
	}

	for _, t := range tests {
		c.Check(asserts.ReducePrimaryKey(asserts.TestOnly2Type, t.pk), check.DeepEquals, t.reduced)
	}
}

func (as *assertsSuite) TestRefOptionalPrimaryKeys(c *check.C) {
	// optional primary key headers
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt1", "o1-defl")()
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt2", "o2-defl")()

	ref := &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "o1-defl"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "o1-defl", "o2-defl"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "A"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz/A")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc opt1:A)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "A", "o2-defl"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz/A")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc opt1:A)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "o1-defl", "B"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz/o1-defl/B")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc opt2:B)`)

	ref = &asserts.Ref{
		Type:       asserts.TestOnly2Type,
		PrimaryKey: []string{"abc", "xyz", "A", "B"},
	}
	c.Check(ref.Unique(), check.Equals, "test-only-2/abc/xyz/A/B")
	c.Check(ref.String(), check.Equals, `test-only-2 (xyz; pk1:abc opt1:A opt2:B)`)
}

func (as *assertsSuite) TestAcceptablePrimaryKey(c *check.C) {
	// optional primary key headers
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt1", "o1-defl")()
	defer asserts.MockOptionalPrimaryKey(asserts.TestOnly2Type, "opt2", "o2-defl")()

	tests := []struct {
		pk []string
		ok bool
	}{
		{nil, false},
		{[]string{"k1"}, false},
		{[]string{"k1", "k2"}, true},
		{[]string{"k1", "k2", "A"}, true},
		{[]string{"k1", "k2", "o1-defl"}, true},
		{[]string{"k1", "k2", "A", "B"}, true},
		{[]string{"k1", "k2", "o1-defl", "o2-defl", "what"}, false},
	}

	for _, t := range tests {
		c.Check(asserts.TestOnly2Type.AcceptablePrimaryKey(t.pk), check.Equals, t.ok)
	}
}

func (as *assertsSuite) TestAtRevisionString(c *check.C) {
	ref := asserts.Ref{
		Type:       asserts.BaseDeclarationType,
		PrimaryKey: []string{"canonical"},
	}

	at := &asserts.AtRevision{
		Ref: ref,
	}
	c.Check(at.String(), check.Equals, "base-declaration (canonical) at revision 0")

	at = &asserts.AtRevision{
		Ref:      ref,
		Revision: asserts.RevisionNotKnown,
	}
	c.Check(at.String(), check.Equals, "base-declaration (canonical)")
}

const exKeyID = "Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij"

const exampleEmptyBodyAllDefaults = "type: test-only\n" +
	"authority-id: auth-id1\n" +
	"primary-key: abc\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
	"\n\n" +
	"AXNpZw=="

func (as *assertsSuite) TestDecodeEmptyBodyAllDefaults(c *check.C) {
	a, err := asserts.Decode([]byte(exampleEmptyBodyAllDefaults))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	_, ok := a.(*asserts.TestOnly)
	c.Check(ok, check.Equals, true)
	c.Check(a.Revision(), check.Equals, 0)
	c.Check(a.Format(), check.Equals, 0)
	c.Check(a.Body(), check.IsNil)
	c.Check(a.Header("header1"), check.IsNil)
	c.Check(a.HeaderString("header1"), check.Equals, "")
	c.Check(a.AuthorityID(), check.Equals, "auth-id1")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)
}

const exampleEmptyBodyOptionalPrimaryKeySet = "type: test-only\n" +
	"authority-id: auth-id1\n" +
	"primary-key: abc\n" +
	"opt1: A\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
	"\n\n" +
	"AXNpZw=="

func (as *assertsSuite) TestDecodeOptionalPrimaryKeys(c *check.C) {
	r := asserts.MockOptionalPrimaryKey(asserts.TestOnlyType, "opt1", "o1-defl")
	defer r()

	a, err := asserts.Decode([]byte(exampleEmptyBodyAllDefaults))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	_, ok := a.(*asserts.TestOnly)
	c.Check(ok, check.Equals, true)
	c.Check(a.Revision(), check.Equals, 0)
	c.Check(a.Format(), check.Equals, 0)
	c.Check(a.Body(), check.IsNil)
	c.Check(a.HeaderString("opt1"), check.Equals, "o1-defl")
	c.Check(a.Header("header1"), check.IsNil)
	c.Check(a.HeaderString("header1"), check.Equals, "")
	c.Check(a.AuthorityID(), check.Equals, "auth-id1")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)

	a, err = asserts.Decode([]byte(exampleEmptyBodyOptionalPrimaryKeySet))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	_, ok = a.(*asserts.TestOnly)
	c.Check(ok, check.Equals, true)
	c.Check(a.Revision(), check.Equals, 0)
	c.Check(a.Format(), check.Equals, 0)
	c.Check(a.Body(), check.IsNil)
	c.Check(a.HeaderString("opt1"), check.Equals, "A")
	c.Check(a.Header("header1"), check.IsNil)
	c.Check(a.HeaderString("header1"), check.Equals, "")
	c.Check(a.AuthorityID(), check.Equals, "auth-id1")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)
}

const exampleEmptyBody2NlNl = "type: test-only\n" +
	"authority-id: auth-id1\n" +
	"primary-key: xyz\n" +
	"revision: 0\n" +
	"body-length: 0\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
	"\n\n" +
	"\n\n" +
	"AXNpZw==\n"

func (as *assertsSuite) TestDecodeEmptyBodyNormalize2NlNl(c *check.C) {
	a, err := asserts.Decode([]byte(exampleEmptyBody2NlNl))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	c.Check(a.Revision(), check.Equals, 0)
	c.Check(a.Format(), check.Equals, 0)
	c.Check(a.Body(), check.IsNil)
}

const exampleBodyAndExtraHeaders = "type: test-only\n" +
	"format: 1\n" +
	"authority-id: auth-id2\n" +
	"primary-key: abc\n" +
	"revision: 5\n" +
	"header1: value1\n" +
	"header2: value2\n" +
	"body-length: 8\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
	"THE-BODY" +
	"\n\n" +
	"AXNpZw==\n"

func (as *assertsSuite) TestDecodeWithABodyAndExtraHeaders(c *check.C) {
	a, err := asserts.Decode([]byte(exampleBodyAndExtraHeaders))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	c.Check(a.AuthorityID(), check.Equals, "auth-id2")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)
	c.Check(a.Header("primary-key"), check.Equals, "abc")
	c.Check(a.Revision(), check.Equals, 5)
	c.Check(a.Format(), check.Equals, 1)
	c.Check(a.SupportedFormat(), check.Equals, true)
	c.Check(a.Header("header1"), check.Equals, "value1")
	c.Check(a.Header("header2"), check.Equals, "value2")
	c.Check(a.Body(), check.DeepEquals, []byte("THE-BODY"))
}

const exampleUnsupportedFormat = "type: test-only\n" +
	"format: 77\n" +
	"authority-id: auth-id2\n" +
	"primary-key: abc\n" +
	"revision: 5\n" +
	"header1: value1\n" +
	"header2: value2\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
	"AXNpZw==\n"

func (as *assertsSuite) TestDecodeUnsupportedFormat(c *check.C) {
	a, err := asserts.Decode([]byte(exampleUnsupportedFormat))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	c.Check(a.AuthorityID(), check.Equals, "auth-id2")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)
	c.Check(a.Header("primary-key"), check.Equals, "abc")
	c.Check(a.Revision(), check.Equals, 5)
	c.Check(a.Format(), check.Equals, 77)
	c.Check(a.SupportedFormat(), check.Equals, false)
}

func (as *assertsSuite) TestDecodeGetSignatureBits(c *check.C) {
	content := "type: test-only\n" +
		"authority-id: auth-id1\n" +
		"primary-key: xyz\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY"
	encoded := content +
		"\n\n" +
		"AXNpZw=="
	a, err := asserts.Decode([]byte(encoded))
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	c.Check(a.AuthorityID(), check.Equals, "auth-id1")
	c.Check(a.SignKeyID(), check.Equals, exKeyID)
	cont, signature := a.Signature()
	c.Check(signature, check.DeepEquals, []byte("AXNpZw=="))
	c.Check(cont, check.DeepEquals, []byte(content))
}

func (as *assertsSuite) TestDecodeNoSignatureSplit(c *check.C) {
	for _, encoded := range []string{"", "foo"} {
		_, err := asserts.Decode([]byte(encoded))
		c.Check(err, check.ErrorMatches, "assertion content/signature separator not found")
	}
}

func (as *assertsSuite) TestDecodeHeaderParsingErrors(c *check.C) {
	headerParsingErrorsTests := []struct{ encoded, expectedErr string }{
		{string([]byte{255, '\n', '\n'}), "header is not utf8"},
		{"foo: a\nbar\n\n", `header entry missing ':' separator: "bar"`},
		{"TYPE: foo\n\n", `invalid header name: "TYPE"`},
		{"foo: a\nbar:>\n\n", `header entry should have a space or newline \(for multiline\) before value: "bar:>"`},
		{"foo: a\nbar:\n\n", `expected 4 chars nesting prefix after multiline introduction "bar:": EOF`},
		{"foo: a\nbar:\nbaz: x\n\n", `expected 4 chars nesting prefix after multiline introduction "bar:": "baz: x"`},
		{"foo: a:\nbar: b\nfoo: x\n\n", `repeated header: "foo"`},
	}

	for _, test := range headerParsingErrorsTests {
		_, err := asserts.Decode([]byte(test.encoded))
		c.Check(err, check.ErrorMatches, "parsing assertion headers: "+test.expectedErr)
	}
}

func (as *assertsSuite) TestDecodeInvalid(c *check.C) {
	keyIDHdr := "sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n"
	encoded := "type: test-only\n" +
		"format: 0\n" +
		"authority-id: auth-id\n" +
		"primary-key: abc\n" +
		"revision: 0\n" +
		"body-length: 5\n" +
		keyIDHdr +
		"\n" +
		"abcde" +
		"\n\n" +
		"AXNpZw=="

	invalidAssertTests := []struct{ original, invalid, expectedErr string }{
		{"body-length: 5", "body-length: z", `assertion: "body-length" header is not an integer: z`},
		{"body-length: 5", "body-length: 3", "assertion body length and declared body-length don't match: 5 != 3"},
		{"authority-id: auth-id\n", "", `assertion: "authority-id" header is mandatory`},
		{"authority-id: auth-id\n", "authority-id: \n", `assertion: "authority-id" header should not be empty`},
		{keyIDHdr, "", `assertion: "sign-key-sha3-384" header is mandatory`},
		{keyIDHdr, "sign-key-sha3-384: \n", `assertion: "sign-key-sha3-384" header should not be empty`},
		{keyIDHdr, "sign-key-sha3-384: $\n", `assertion: "sign-key-sha3-384" header cannot be decoded: .*`},
		{keyIDHdr, "sign-key-sha3-384: eHl6\n", `assertion: "sign-key-sha3-384" header does not have the expected bit length: 24`},
		{"AXNpZw==", "", "empty assertion signature"},
		{"type: test-only\n", "", `assertion: "type" header is mandatory`},
		{"type: test-only\n", "type: unknown\n", `unknown assertion type: "unknown"`},
		{"revision: 0\n", "revision: Z\n", `assertion: "revision" header is not an integer: Z`},
		{"revision: 0\n", "revision:\n  - 1\n", `assertion: "revision" header is not an integer: \[1\]`},
		{"revision: 0\n", "revision: 00\n", `assertion: "revision" header has invalid prefix zeros: 00`},
		{"revision: 0\n", "revision: -10\n", "assertion: revision should be positive: -10"},
		{"revision: 0\n", "revision: 99999999999999999999\n", `assertion: "revision" header is out of range: 99999999999999999999`},
		{"format: 0\n", "format: Z\n", `assertion: "format" header is not an integer: Z`},
		{"format: 0\n", "format: -10\n", "assertion: format should be positive: -10"},
		{"primary-key: abc\n", "", `assertion test-only: "primary-key" header is mandatory`},
		{"primary-key: abc\n", "primary-key:\n  - abc\n", `assertion test-only: "primary-key" header must be a string`},
		{"primary-key: abc\n", "primary-key: a/c\n", `assertion test-only: "primary-key" primary key header cannot contain '/'`},
		{"abcde", "ab\xffde", "assertion body is not utf8"},
	}

	for _, test := range invalidAssertTests {
		invalid := strings.Replace(encoded, test.original, test.invalid, 1)
		_, err := asserts.Decode([]byte(invalid))
		c.Check(err, check.ErrorMatches, test.expectedErr)
	}
}

func (as *assertsSuite) TestDecodeNoAuthorityInvalid(c *check.C) {
	invalid := "type: test-only-no-authority\n" +
		"authority-id: auth-id1\n" +
		"hdr: FOO\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
		"\n\n" +
		"openpgp c2ln"

	_, err := asserts.Decode([]byte(invalid))
	c.Check(err, check.ErrorMatches, `"test-only-no-authority" assertion cannot have authority-id set`)
}

func checkContent(c *check.C, a asserts.Assertion, encoded string) {
	expected, err := asserts.Decode([]byte(encoded))
	c.Assert(err, check.IsNil)
	expectedCont, _ := expected.Signature()

	cont, _ := a.Signature()
	c.Check(cont, check.DeepEquals, expectedCont)
}

func (as *assertsSuite) TestEncoderDecoderHappy(c *check.C) {
	stream := new(bytes.Buffer)
	enc := asserts.NewEncoder(stream)
	enc.WriteEncoded([]byte(exampleEmptyBody2NlNl))
	enc.WriteEncoded([]byte(exampleBodyAndExtraHeaders))
	enc.WriteEncoded([]byte(exampleEmptyBodyAllDefaults))

	decoder := asserts.NewDecoder(stream)
	a, err := decoder.Decode()
	c.Assert(err, check.IsNil)
	c.Check(a.Type(), check.Equals, asserts.TestOnlyType)
	_, ok := a.(*asserts.TestOnly)
	c.Check(ok, check.Equals, true)
	checkContent(c, a, exampleEmptyBody2NlNl)

	a, err = decoder.Decode()
	c.Assert(err, check.IsNil)
	checkContent(c, a, exampleBodyAndExtraHeaders)

	a, err = decoder.Decode()
	c.Assert(err, check.IsNil)
	checkContent(c, a, exampleEmptyBodyAllDefaults)

	a, err = decoder.Decode()
	c.Assert(err, check.Equals, io.EOF)
	c.Check(a, check.IsNil)
}

func (as *assertsSuite) TestDecodeEmptyStream(c *check.C) {
	stream := new(bytes.Buffer)
	decoder := asserts.NewDecoder(stream)
	_, err := decoder.Decode()
	c.Check(err, check.Equals, io.EOF)
}

func (as *assertsSuite) TestDecoderHappyWithSeparatorsVariations(c *check.C) {
	streams := []string{
		exampleBodyAndExtraHeaders,
		exampleEmptyBody2NlNl,
		exampleEmptyBodyAllDefaults,
	}

	for _, streamData := range streams {
		stream := bytes.NewBufferString(streamData)
		decoder := asserts.NewDecoderStressed(stream, 16, 1024, 1024, 1024)
		a, err := decoder.Decode()
		c.Assert(err, check.IsNil, check.Commentf("stream: %q", streamData))

		checkContent(c, a, streamData)

		a, err = decoder.Decode()
		c.Check(a, check.IsNil)
		c.Check(err, check.Equals, io.EOF, check.Commentf("stream: %q", streamData))
	}
}

func (as *assertsSuite) TestDecoderHappyWithTrailerDoubleNewlines(c *check.C) {
	streams := []string{
		exampleBodyAndExtraHeaders,
		exampleEmptyBody2NlNl,
		exampleEmptyBodyAllDefaults,
	}

	for _, streamData := range streams {
		stream := bytes.NewBufferString(streamData)
		if strings.HasSuffix(streamData, "\n") {
			stream.WriteString("\n")
		} else {
			stream.WriteString("\n\n")
		}

		decoder := asserts.NewDecoderStressed(stream, 16, 1024, 1024, 1024)
		a, err := decoder.Decode()
		c.Assert(err, check.IsNil, check.Commentf("stream: %q", streamData))

		checkContent(c, a, streamData)

		a, err = decoder.Decode()
		c.Check(a, check.IsNil)
		c.Check(err, check.Equals, io.EOF, check.Commentf("stream: %q", streamData))
	}
}

func (as *assertsSuite) TestDecoderUnexpectedEOF(c *check.C) {
	streamData := exampleBodyAndExtraHeaders + "\n" + exampleEmptyBodyAllDefaults
	fstHeadEnd := strings.Index(exampleBodyAndExtraHeaders, "\n\n")
	sndHeadEnd := len(exampleBodyAndExtraHeaders) + 1 + strings.Index(exampleEmptyBodyAllDefaults, "\n\n")

	for _, brk := range []int{1, fstHeadEnd / 2, fstHeadEnd, fstHeadEnd + 1, fstHeadEnd + 2, fstHeadEnd + 6} {
		stream := bytes.NewBufferString(streamData[:brk])
		decoder := asserts.NewDecoderStressed(stream, 16, 1024, 1024, 1024)
		_, err := decoder.Decode()
		c.Check(err, check.Equals, io.ErrUnexpectedEOF, check.Commentf("brk: %d", brk))
	}

	for _, brk := range []int{sndHeadEnd, sndHeadEnd + 1} {
		stream := bytes.NewBufferString(streamData[:brk])
		decoder := asserts.NewDecoder(stream)
		_, err := decoder.Decode()
		c.Assert(err, check.IsNil)

		_, err = decoder.Decode()
		c.Check(err, check.Equals, io.ErrUnexpectedEOF, check.Commentf("brk: %d", brk))
	}
}

func (as *assertsSuite) TestDecoderBrokenBodySeparation(c *check.C) {
	streamData := strings.Replace(exampleBodyAndExtraHeaders, "THE-BODY\n\n", "THE-BODY", 1)
	decoder := asserts.NewDecoder(bytes.NewBufferString(streamData))
	_, err := decoder.Decode()
	c.Assert(err, check.ErrorMatches, "missing content/signature separator")

	streamData = strings.Replace(exampleBodyAndExtraHeaders, "THE-BODY\n\n", "THE-BODY\n", 1)
	decoder = asserts.NewDecoder(bytes.NewBufferString(streamData))
	_, err = decoder.Decode()
	c.Assert(err, check.ErrorMatches, "missing content/signature separator")
}

func (as *assertsSuite) TestDecoderHeadTooBig(c *check.C) {
	decoder := asserts.NewDecoderStressed(bytes.NewBufferString(exampleBodyAndExtraHeaders), 4, 4, 1024, 1024)
	_, err := decoder.Decode()
	c.Assert(err, check.ErrorMatches, `error reading assertion headers: maximum size exceeded while looking for delimiter "\\n\\n"`)
}

func (as *assertsSuite) TestDecoderBodyTooBig(c *check.C) {
	decoder := asserts.NewDecoderStressed(bytes.NewBufferString(exampleBodyAndExtraHeaders), 1024, 1024, 5, 1024)
	_, err := decoder.Decode()
	c.Assert(err, check.ErrorMatches, "assertion body length 8 exceeds maximum body size")
}

func (as *assertsSuite) TestDecoderSignatureTooBig(c *check.C) {
	decoder := asserts.NewDecoderStressed(bytes.NewBufferString(exampleBodyAndExtraHeaders), 4, 1024, 1024, 7)
	_, err := decoder.Decode()
	c.Assert(err, check.ErrorMatches, `error reading assertion signature: maximum size exceeded while looking for delimiter "\\n\\n"`)
}

func (as *assertsSuite) TestDecoderDefaultMaxBodySize(c *check.C) {
	enc := strings.Replace(exampleBodyAndExtraHeaders, "body-length: 8", "body-length: 2097153", 1)
	decoder := asserts.NewDecoder(bytes.NewBufferString(enc))
	_, err := decoder.Decode()
	c.Assert(err, check.ErrorMatches, "assertion body length 2097153 exceeds maximum body size")
}

func (as *assertsSuite) TestDecoderWithTypeMaxBodySize(c *check.C) {
	ex1 := strings.Replace(exampleBodyAndExtraHeaders, "body-length: 8", "body-length: 2097152", 1)
	ex1 = strings.Replace(ex1, "THE-BODY", strings.Repeat("B", 2*1024*1024), 1)
	ex1toobig := strings.Replace(exampleBodyAndExtraHeaders, "body-length: 8", "body-length: 2097153", 1)
	ex1toobig = strings.Replace(ex1toobig, "THE-BODY", strings.Repeat("B", 2*1024*1024+1), 1)
	const ex2 = `type: test-only-2
authority-id: auth-id1
pk1: foo
pk2: bar
body-length: 3
sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij

XYZ

AXNpZw==`

	decoder := asserts.NewDecoderWithTypeMaxBodySize(bytes.NewBufferString(ex1+"\n"+ex2), map[*asserts.AssertionType]int{
		asserts.TestOnly2Type: 3,
	})
	a1, err := decoder.Decode()
	c.Assert(err, check.IsNil)
	c.Check(a1.Body(), check.HasLen, 2*1024*1024)
	a2, err := decoder.Decode()
	c.Assert(err, check.IsNil)
	c.Check(a2.Body(), check.DeepEquals, []byte("XYZ"))

	decoder = asserts.NewDecoderWithTypeMaxBodySize(bytes.NewBufferString(ex1+"\n"+ex2), map[*asserts.AssertionType]int{
		asserts.TestOnly2Type: 2,
	})
	a1, err = decoder.Decode()
	c.Assert(err, check.IsNil)
	c.Check(a1.Body(), check.HasLen, 2*1024*1024)
	_, err = decoder.Decode()
	c.Assert(err, check.ErrorMatches, `assertion body length 3 exceeds maximum body size 2 for "test-only-2" assertions`)

	decoder = asserts.NewDecoderWithTypeMaxBodySize(bytes.NewBufferString(ex2+"\n\n"+ex1toobig), map[*asserts.AssertionType]int{
		asserts.TestOnly2Type: 3,
	})
	a2, err = decoder.Decode()
	c.Assert(err, check.IsNil)
	c.Check(a2.Body(), check.DeepEquals, []byte("XYZ"))
	_, err = decoder.Decode()
	c.Assert(err, check.ErrorMatches, "assertion body length 2097153 exceeds maximum body size")
}

func (as *assertsSuite) TestEncode(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: xyz\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)
	encodeRes := asserts.Encode(a)
	c.Check(encodeRes, check.DeepEquals, encoded)
}

func (as *assertsSuite) TestEncoderOK(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: xyzyz\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a0, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)
	cont0, _ := a0.Signature()

	stream := new(bytes.Buffer)
	enc := asserts.NewEncoder(stream)
	enc.Encode(a0)

	c.Check(bytes.HasSuffix(stream.Bytes(), []byte{'\n'}), check.Equals, true)

	dec := asserts.NewDecoder(stream)
	a1, err := dec.Decode()
	c.Assert(err, check.IsNil)

	cont1, _ := a1.Signature()
	c.Check(cont1, check.DeepEquals, cont0)
}

func (as *assertsSuite) TestEncoderSingleDecodeOK(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: abc\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a0, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)
	cont0, _ := a0.Signature()

	stream := new(bytes.Buffer)
	enc := asserts.NewEncoder(stream)
	enc.Encode(a0)

	a1, err := asserts.Decode(stream.Bytes())
	c.Assert(err, check.IsNil)

	cont1, _ := a1.Signature()
	c.Check(cont1, check.DeepEquals, cont0)
}

func (as *assertsSuite) TestHeaders(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: abc\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)

	hs := a.Headers()
	c.Check(hs, check.DeepEquals, map[string]interface{}{
		"type":              "test-only",
		"authority-id":      "auth-id2",
		"primary-key":       "abc",
		"revision":          "5",
		"header1":           "value1",
		"header2":           "value2",
		"body-length":       "8",
		"sign-key-sha3-384": exKeyID,
	})
}

func (as *assertsSuite) TestHeadersReturnsCopy(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: xyz\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)

	hs := a.Headers()
	// casual later result mutation doesn't trip us
	delete(hs, "primary-key")
	c.Check(a.Header("primary-key"), check.Equals, "xyz")
}

func (as *assertsSuite) TestAssembleRoundtrip(c *check.C) {
	encoded := []byte("type: test-only\n" +
		"format: 1\n" +
		"authority-id: auth-id2\n" +
		"primary-key: abc\n" +
		"revision: 5\n" +
		"header1: value1\n" +
		"header2: value2\n" +
		"body-length: 8\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"THE-BODY" +
		"\n\n" +
		"AXNpZw==")
	a, err := asserts.Decode(encoded)
	c.Assert(err, check.IsNil)

	cont, sig := a.Signature()
	reassembled, err := asserts.Assemble(a.Headers(), a.Body(), cont, sig)
	c.Assert(err, check.IsNil)

	c.Check(reassembled.Headers(), check.DeepEquals, a.Headers())
	c.Check(reassembled.Body(), check.DeepEquals, a.Body())

	reassembledEncoded := asserts.Encode(reassembled)
	c.Check(reassembledEncoded, check.DeepEquals, encoded)
}

func (as *assertsSuite) TestAssembleHeadersCheck(c *check.C) {
	cont := []byte("type: test-only\n" +
		"authority-id: auth-id2\n" +
		"primary-key: abc\n" +
		"revision: 5")
	headers := map[string]interface{}{
		"type":         "test-only",
		"authority-id": "auth-id2",
		"primary-key":  "abc",
		"revision":     5, // must be a string actually!
	}

	_, err := asserts.Assemble(headers, nil, cont, nil)
	c.Check(err, check.ErrorMatches, `header "revision": header values must be strings or nested lists or maps with strings as the only scalars: 5`)
}
