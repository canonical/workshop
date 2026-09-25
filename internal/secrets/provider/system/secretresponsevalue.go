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
	"bytes"
	"encoding/json"
	"io"

	"github.com/canonical/workshop/internal/secrets"
)

// SecretResponseValue encodes a [secrets.Secret] as a base64 JSON string.
// Marshalling consumes the secret, including through copies of this value.
// Unmarshalling creates an owned secret; callers must consume or close it.
// A zero value can be omitted with the JSON omitzero option, whereas an owned
// empty secret encodes as an empty string.
type SecretResponseValue struct {
	secrets.Secret
}

// MarshalJSON consumes the secret and encodes its bytes as a base64 JSON
// string. The temporary plaintext buffer is cleared on return; the encoded
// result remains sensitive. Repeated calls cannot reproduce consumed bytes.
// Callers must close any unread remainder if marshalling fails.
func (s SecretResponseValue) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	defer func() { clear(buf.Bytes()) }()

	_, err := io.Copy(&buf, s.Secret)
	if err != nil {
		return nil, err
	}
	return json.Marshal(buf.Bytes())
}

// UnmarshalJSON decodes secret bytes and takes ownership of them. It closes
// any previous secret, including through copies, even if decoding fails.
// On failure the receiver is reset to its zero value. JSON null is accepted
// as an owned empty secret, matching the decoding behaviour of a byte slice.
// The caller retains ownership of the sensitive JSON input.
func (s *SecretResponseValue) UnmarshalJSON(data []byte) error {
	_ = s.Close()
	s.Secret = secrets.Secret{}

	var value []byte
	err := json.Unmarshal(data, &value)
	if err != nil {
		clear(value)
		return err
	}
	s.Secret = secrets.NewSecret(value)
	return nil
}
