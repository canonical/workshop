package x11_test

import (
	"bytes"
	"encoding/binary"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/x11"
)

var (
	// len (16) + 'MIT_MAGIC_COOKIE-1' in hex
	magicCookie = []byte{0x00, 0x12, 0x4d, 0x49, 0x54, 0x5f, 0x4d, 0x41, 0x47, 0x49, 0x43, 0x5f, 0x43, 0x4f, 0x4f, 0x4b, 0x49, 0x45, 0x2d, 0x31}
	// len (14) + auth data in hex
	magicCookieAuth = []byte{0x00, 0x10, 0xdc, 0x71, 0xd2, 0xe2, 0x0a, 0x8b, 0x64, 0xbd, 0x78, 0x4e, 0xcb, 0x1b, 0xcf, 0xdc, 0xe4, 0x12}
	// len (7) + 'example' in hex
	hostExample = []byte{0x00, 0x07, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65}
	// len (1) + '0' in hex
	displayZero = []byte{0x00, 0x01, 0x30}
	// len (0), null display
	displayNone = []byte{0x00, 0x00}
)

func (x *X11TestSuit) TestProcessFile(c *check.C) {
	cookie := constructTestCookie(displayZero)

	entries, err := x11.ProcessFile(bytes.NewReader(cookie))
	c.Assert(err, check.IsNil)
	c.Check(entries[0].Family, check.Equals, x11.FamilyLocal)
	c.Check(entries[0].Host, check.DeepEquals, hostExample[2:])
	c.Check(entries[0].Display, check.DeepEquals, []byte{0x30})
	c.Check(entries[0].AuthorisationMethod, check.DeepEquals, magicCookie[2:])
	c.Check(entries[0].AuthorisationData, check.DeepEquals, magicCookieAuth[2:])
}

func (x *X11TestSuit) TestProcessFileNoDisplay(c *check.C) {
	cookie := constructTestCookie(displayNone)

	entries, err := x11.ProcessFile(bytes.NewReader(cookie))
	c.Assert(err, check.IsNil)
	c.Check(entries[0].Family, check.Equals, x11.FamilyLocal)
	c.Check(entries[0].Host, check.DeepEquals, hostExample[2:])
	c.Check(entries[0].Display, check.DeepEquals, []byte{})
	c.Check(entries[0].AuthorisationMethod, check.DeepEquals, magicCookie[2:])
	c.Check(entries[0].AuthorisationData, check.DeepEquals, magicCookieAuth[2:])
}

func (x *X11TestSuit) TestEncodeEntries(c *check.C) {
	cookie := constructTestCookie(displayZero)

	entries, err := x11.ProcessFile(bytes.NewReader(cookie))
	c.Assert(err, check.IsNil)

	encoded := x11.EncodeEntries(entries)
	c.Assert(cookie, check.DeepEquals, encoded)
}

func (x *X11TestSuit) TestEncodeEntriesNoDisplay(c *check.C) {
	cookie := constructTestCookie(displayNone)

	entries, err := x11.ProcessFile(bytes.NewReader(cookie))
	c.Assert(err, check.IsNil)

	encoded := x11.EncodeEntries(entries)
	c.Assert(cookie, check.DeepEquals, encoded)
}

func constructTestCookie(display []byte) []byte {
	// Construct a fake, however 'correct' cookie
	cookie := binary.BigEndian.AppendUint16([]byte{}, uint16(x11.FamilyLocal))
	cookie = append(cookie, hostExample...)
	cookie = append(cookie, display...)
	cookie = append(cookie, magicCookie...)
	cookie = append(cookie, magicCookieAuth...)
	return cookie
}
