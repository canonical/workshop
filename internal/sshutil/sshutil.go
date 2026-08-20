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

package sshutil

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

type PublicKey struct {
	key     ssh.PublicKey
	comment string
}

type PrivateKey struct {
	key     crypto.PrivateKey
	comment string
}

type equalPrivateKey interface {
	Equal(crypto.PrivateKey) bool
}

func (k PublicKey) Equal(other PublicKey) bool {
	return bytes.Equal(k.key.Marshal(), other.key.Marshal()) && k.comment == other.comment
}

func (k PublicKey) String() string {
	data := ssh.MarshalAuthorizedKey(k.key)
	data = bytes.TrimSuffix(data, []byte("\n"))
	if k.comment != "" {
		data = fmt.Append(data, " ", k.comment)
	}
	return string(data)
}

func (k PublicKey) Fingerprint() string {
	return ssh.FingerprintSHA256(k.key)
}

func (k PublicKey) Comment() string {
	return k.comment
}

func (k PrivateKey) Equal(other PrivateKey) bool {
	key, ok := k.key.(equalPrivateKey)
	return ok && key.Equal(other.key) && k.comment == other.comment
}

func (k PrivateKey) MarshalText() ([]byte, error) {
	block, err := ssh.MarshalPrivateKey(k.key, k.comment)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}

func (k PrivateKey) SignHostKey(key PublicKey, principals []string) (*PublicKey, error) {
	signer, err := ssh.NewSignerFromKey(k.key)
	if err != nil {
		return nil, err
	}

	certificate := &ssh.Certificate{
		Key:             key.key,
		CertType:        ssh.HostCert,
		KeyId:           key.comment,
		ValidPrincipals: principals,
		ValidBefore:     ssh.CertTimeInfinity,
	}
	if err := certificate.SignCert(rand.Reader, signer); err != nil {
		return nil, err
	}

	return &PublicKey{key: certificate, comment: key.comment}, nil
}

func (k PrivateKey) SignUserKey(key PublicKey, principals []string) (*PublicKey, error) {
	signer, err := ssh.NewSignerFromKey(k.key)
	if err != nil {
		return nil, err
	}

	certificate := &ssh.Certificate{
		Key:             key.key,
		CertType:        ssh.UserCert,
		KeyId:           key.comment,
		ValidPrincipals: principals,
		ValidBefore:     ssh.CertTimeInfinity,
		Permissions: ssh.Permissions{
			Extensions: map[string]string{
				"permit-agent-forwarding": "",
				"permit-port-forwarding":  "",
				"permit-pty":              "",
				"permit-user-rc":          "",
				"permit-X11-forwarding":   "",
			},
		},
	}
	if err := certificate.SignCert(rand.Reader, signer); err != nil {
		return nil, err
	}

	return &PublicKey{key: certificate, comment: key.comment}, nil
}

func GenerateKey(comment string) (*PublicKey, *PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh-keygen -C %q: %w", comment, err)
	}

	wrapped, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}

	return &PublicKey{key: wrapped, comment: comment}, &PrivateKey{key: priv, comment: comment}, nil
}

func ParsePublicKey(content []byte) (*PublicKey, error) {
	key, comment, _, _, err := ssh.ParseAuthorizedKey(content)
	if err != nil {
		return nil, err
	}
	return &PublicKey{key: key, comment: comment}, nil
}

// ParsePrivateKey unmarshals an SSH private key and attaches the given
// comment. The data already contains the comment, but it's better to present
// this awkward API than attempt to reimplement the parsing ourselves.
func ParsePrivateKey(content []byte, comment string) (*PrivateKey, error) {
	key, err := ssh.ParseRawPrivateKey(content)
	if err != nil {
		return nil, err
	}
	return &PrivateKey{key: key, comment: comment}, nil
}
