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

package system

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/secrets"
)

// secretResponseValueSuite tests secret JSON encoding and ownership.
type secretResponseValueSuite struct{}

var _ = check.Suite(&secretResponseValueSuite{})

// TestBinaryRoundTrip checks binary response bytes survive JSON encoding and
// decoding, while encoding consumes the original secret.
func (s *secretResponseValueSuite) TestBinaryRoundTrip(c *check.C) {
	original := secrets.NewSecret([]byte{'a', 0, 0xff, '\n'})
	defer original.Close()
	data, err := json.Marshal(DelegatedDBusResponse{
		Secret: SecretResponseValue{Secret: original},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":"YQD/Cg=="}`)

	var remaining [1]byte
	n, err := original.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)

	var decoded DelegatedDBusResponse
	err = json.Unmarshal(data, &decoded)
	defer decoded.Secret.Close()
	c.Assert(err, check.IsNil)
	c.Check(decoded.Error, check.Equals, "")
	contents, err := io.ReadAll(decoded.Secret)
	defer clear(contents)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte{'a', 0, 0xff, '\n'})
}

// TestMarshalEmpty checks an owned empty secret is an empty JSON string and
// the successful response omits its error field.
func (s *secretResponseValueSuite) TestMarshalEmpty(c *check.C) {
	value := secrets.NewSecret([]byte{})
	defer value.Close()
	data, err := json.Marshal(DelegatedDBusResponse{
		Secret: SecretResponseValue{Secret: value},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestMarshalError checks an error response omits its zero-value secret.
func (s *secretResponseValueSuite) TestMarshalError(c *check.C) {
	data, err := json.Marshal(DelegatedDBusResponse{
		Error: ErrorSecretNotFound.Error(),
	})
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"error":"secret not found"}`)
}

// TestMarshalNilBytes checks an owned nil buffer encodes as an empty string,
// not null or an omitted secret.
func (s *secretResponseValueSuite) TestMarshalNilBytes(c *check.C) {
	value := secrets.NewSecret(nil)
	defer value.Close()
	data, err := json.Marshal(DelegatedDBusResponse{
		Secret: SecretResponseValue{Secret: value},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestMarshalSharedConsumption checks a copied wrapper cannot encode bytes
// already consumed through the original wrapper.
func (s *secretResponseValueSuite) TestMarshalSharedConsumption(c *check.C) {
	value := SecretResponseValue{
		Secret: secrets.NewSecret([]byte("service-token")),
	}
	defer value.Close()
	copied := value
	first, err := value.MarshalJSON()
	defer clear(first)
	c.Assert(err, check.IsNil)
	c.Check(string(first), check.Equals, `"c2VydmljZS10b2tlbg=="`)

	second, err := copied.MarshalJSON()
	defer clear(second)
	c.Assert(err, check.IsNil)
	c.Check(string(second), check.Equals, `""`)
}

// TestUnmarshalBinary checks base64 decoding produces the original binary
// bytes in a readable secret.
func (s *secretResponseValueSuite) TestUnmarshalBinary(c *check.C) {
	var value SecretResponseValue
	err := json.Unmarshal([]byte(`"YQD/Cg=="`), &value)
	defer value.Close()
	c.Assert(err, check.IsNil)

	contents, err := io.ReadAll(value)
	defer clear(contents)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte{'a', 0, 0xff, '\n'})
}

// TestUnmarshalEmpty checks an empty string creates an owned empty secret
// that remains present when encoded in a successful response.
func (s *secretResponseValueSuite) TestUnmarshalEmpty(c *check.C) {
	var value SecretResponseValue
	err := json.Unmarshal([]byte(`""`), &value)
	defer value.Close()
	c.Assert(err, check.IsNil)
	c.Check(value.Secret, check.Not(check.Equals), secrets.Secret{})

	var remaining [1]byte
	n, err := value.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)

	data, err := json.Marshal(DelegatedDBusResponse{Secret: value})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestUnmarshalInvalidBase64 checks a partially decoded invalid value returns
// the base64 error and leaves a zero secret.
func (s *secretResponseValueSuite) TestUnmarshalInvalidBase64(c *check.C) {
	var value SecretResponseValue
	err := json.Unmarshal([]byte(`"YQD/!"`), &value)
	defer value.Close()
	c.Check(errors.Is(err, base64.CorruptInputError(4)), check.Equals, true)
	c.Check(value, check.Equals, SecretResponseValue{})
}

// TestUnmarshalInvalidReplacement checks a failed replacement closes the
// original shared secret and resets the wrapper to zero.
func (s *secretResponseValueSuite) TestUnmarshalInvalidReplacement(c *check.C) {
	original := secrets.NewSecret([]byte("previous-service-token"))
	defer original.Close()
	value := SecretResponseValue{Secret: original}
	err := json.Unmarshal([]byte(`"YQD/!"`), &value)
	defer value.Close()
	c.Check(errors.Is(err, base64.CorruptInputError(4)), check.Equals, true)
	c.Check(value, check.Equals, SecretResponseValue{})

	var remaining [1]byte
	n, err := original.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)
}

// TestUnmarshalNull checks null follows byte-slice decoding semantics by
// creating an owned empty secret rather than leaving a zero wrapper.
func (s *secretResponseValueSuite) TestUnmarshalNull(c *check.C) {
	var value SecretResponseValue
	err := json.Unmarshal([]byte(`null`), &value)
	defer value.Close()
	c.Assert(err, check.IsNil)
	c.Check(value.Secret, check.Not(check.Equals), secrets.Secret{})

	var remaining [1]byte
	n, err := value.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)

	data, err := json.Marshal(DelegatedDBusResponse{Secret: value})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestUnmarshalReplacement checks valid replacement closes the original
// shared secret while keeping the replacement readable.
func (s *secretResponseValueSuite) TestUnmarshalReplacement(c *check.C) {
	original := secrets.NewSecret([]byte("previous-service-token"))
	defer original.Close()
	value := SecretResponseValue{Secret: original}
	err := json.Unmarshal([]byte(`"YQD/Cg=="`), &value)
	defer value.Close()
	c.Assert(err, check.IsNil)

	var remaining [1]byte
	n, err := original.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)

	contents, err := io.ReadAll(value)
	defer clear(contents)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte{'a', 0, 0xff, '\n'})
}
