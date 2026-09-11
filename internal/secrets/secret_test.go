// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package secrets

import (
	"fmt"
	"io"

	"gopkg.in/check.v1"
)

// secretSuite checks destructive reads and explicit secret disposal.
type secretSuite struct{}

var _ = check.Suite(&secretSuite{})

// TestClose checks closing a partially read secret clears all remaining bytes
// and remains safe when repeated.
func (s *secretSuite) TestClose(c *check.C) {
	secret := NewSecret([]byte("token"))
	backing := secret.state.value
	defer secret.Close()
	buffer := make([]byte, 2)
	n, err := secret.Read(buffer)
	c.Assert(err, check.IsNil)
	c.Check(n, check.Equals, 2)

	err = secret.Close()
	c.Check(err, check.IsNil)
	c.Check(backing, check.DeepEquals, make([]byte, 5))
	c.Check(secret.state.value, check.IsNil)
	c.Check(secret.Close(), check.IsNil)
	n, err = secret.Read(buffer)
	c.Check(n, check.Equals, 0)
	c.Check(err, check.Equals, io.EOF)
}

// TestCloseUnread checks closing without reading clears the complete secret.
func (s *secretSuite) TestCloseUnread(c *check.C) {
	secret := NewSecret([]byte("token"))
	backing := secret.state.value

	err := secret.Close()

	c.Check(err, check.IsNil)
	c.Check(backing, check.DeepEquals, make([]byte, 5))
	c.Check(secret.state.value, check.IsNil)
}

// TestFormatPointer checks formatting a pointer emits nothing, including with
// flags, width and precision, and leaves the secret available for reading.
func (s *secretSuite) TestFormatPointer(c *check.C) {
	secret := NewSecret([]byte("token"))
	defer secret.Close()

	c.Check(fmt.Sprintf("%v", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%+v", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%#v", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%s", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%q", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%x", secret), check.Equals, "")
	c.Check(fmt.Sprintf("%20.3s", secret), check.Equals, "")

	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("token"))
}

// TestFormatValue checks formatting a value copy emits nothing without
// consuming the shared secret.
func (s *secretSuite) TestFormatValue(c *check.C) {
	secret := NewSecret([]byte("token"))
	defer secret.Close()

	c.Check(fmt.Sprint(*secret), check.Equals, "")
	c.Check(fmt.Sprintf("%v", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%+v", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%#v", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%s", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%q", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%x", *secret), check.Equals, "")
	c.Check(fmt.Sprintf("%20.3s", *secret), check.Equals, "")

	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("token"))
}

// TestReadAll checks normal reader consumers receive the value once while
// the owned buffer is cleared and released.
func (s *secretSuite) TestReadAll(c *check.C) {
	secret := NewSecret([]byte("token"))
	backing := secret.state.value
	defer secret.Close()

	value, err := io.ReadAll(secret)

	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("token"))
	c.Check(backing, check.DeepEquals, make([]byte, 5))
	c.Check(secret.state.value, check.IsNil)
	n, err := secret.Read(make([]byte, 1))
	c.Check(n, check.Equals, 0)
	c.Check(err, check.Equals, io.EOF)
}

// TestReadPartial checks each read wipes only the consumed bytes and preserves
// the unread suffix for subsequent reads.
func (s *secretSuite) TestReadPartial(c *check.C) {
	secret := NewSecret([]byte("token"))
	backing := secret.state.value
	defer secret.Close()
	buffer := make([]byte, 2)

	n, err := secret.Read(buffer)

	c.Check(err, check.IsNil)
	c.Check(n, check.Equals, 2)
	c.Check(buffer, check.DeepEquals, []byte("to"))
	c.Check(backing, check.DeepEquals, []byte{0, 0, 'k', 'e', 'n'})

	remainder, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(remainder, check.DeepEquals, []byte("ken"))
	c.Check(backing, check.DeepEquals, make([]byte, 5))
}

// TestReadZeroLength checks a zero-length read leaves the secret untouched.
func (s *secretSuite) TestReadZeroLength(c *check.C) {
	secret := NewSecret([]byte("token"))
	defer secret.Close()

	n, err := secret.Read(nil)

	c.Check(err, check.IsNil)
	c.Check(n, check.Equals, 0)
	value, err := io.ReadAll(secret)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("token"))
}

// TestCopiesShareReads checks a struct copy consumes the same unread bytes
// and leaves both copies exhausted after the final read.
func (s *secretSuite) TestCopiesShareReads(c *check.C) {
	secret := NewSecret([]byte("token"))
	defer secret.Close()
	other := *secret
	buffer := make([]byte, 2)

	n, err := secret.Read(buffer)
	c.Check(err, check.IsNil)
	c.Check(n, check.Equals, 2)
	c.Check(buffer, check.DeepEquals, []byte("to"))

	value, err := io.ReadAll(&other)
	c.Check(err, check.IsNil)
	c.Check(value, check.DeepEquals, []byte("ken"))
	n, err = secret.Read(buffer)
	c.Check(n, check.Equals, 0)
	c.Check(err, check.Equals, io.EOF)
}

// TestCopiesShareClose checks closing a copy wipes the shared buffer and
// prevents further reads through the original secret.
func (s *secretSuite) TestCopiesShareClose(c *check.C) {
	secret := NewSecret([]byte("token"))
	backing := secret.state.value
	other := *secret

	err := other.Close()

	c.Check(err, check.IsNil)
	c.Check(backing, check.DeepEquals, make([]byte, 5))
	n, err := secret.Read(make([]byte, 1))
	c.Check(n, check.Equals, 0)
	c.Check(err, check.Equals, io.EOF)
	c.Check(secret.Close(), check.IsNil)
}

// TestZeroValue checks an empty secret supports reading and closing safely.
func (s *secretSuite) TestZeroValue(c *check.C) {
	var secret Secret

	n, err := secret.Read(make([]byte, 1))
	c.Check(n, check.Equals, 0)
	c.Check(err, check.Equals, io.EOF)
	n, err = secret.Read(nil)
	c.Check(n, check.Equals, 0)
	c.Check(err, check.IsNil)
	c.Check(secret.Close(), check.IsNil)
}
