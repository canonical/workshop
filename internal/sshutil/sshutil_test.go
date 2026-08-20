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

//go:build integration

package sshutil_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sshutil"
)

func Test(t *testing.T) { check.TestingT(t) }

type sshSuite struct{}

var _ = check.Suite(&sshSuite{})

func (s *sshSuite) TestSignHostKey(c *check.C) {
	pub, _, err := sshutil.GenerateKey("root@dev-42424242.wp")
	c.Assert(err, check.IsNil)

	identity, authority, err := sshutil.GenerateKey("Workshop-CA")
	c.Assert(err, check.IsNil)

	cert, err := authority.SignHostKey(*pub, []string{"dev-42424242.wp"})
	c.Assert(err, check.IsNil)

	scratch := c.MkDir()
	err = os.WriteFile(filepath.Join(scratch, "id_ed25519-cert.pub"), []byte(cert.String()+"\n"), 0600)
	c.Assert(err, check.IsNil)

	cmd := exec.Command("ssh-keygen", "-Lf", "id_ed25519-cert.pub")
	cmd.Dir = scratch
	out, err := cmd.Output()
	c.Assert(err, check.IsNil)

	fingerprint := regexp.QuoteMeta(pub.Fingerprint())
	ca := regexp.QuoteMeta(identity.Fingerprint())

	template := `id_ed25519-cert.pub:
\s*Type: ssh-ed25519.* host certificate
\s*Public key: ED25519-CERT %s
\s*Signing CA: ED25519 %s \(using ssh-ed25519\)
\s*Key ID: "root@dev-42424242\.wp"
\s*Serial: 0
\s*Valid: forever
\s*Principals:\s*
\s*dev-42424242\.wp
\s*Critical Options: \(none\)
\s*Extensions: \(none\)
`
	pattern := fmt.Sprintf(template, fingerprint, ca)
	c.Check(string(out), check.Matches, pattern)
}

func (s *sshSuite) TestSignUserKey(c *check.C) {
	pub, _, err := sshutil.GenerateKey("workshop@wp")
	c.Assert(err, check.IsNil)

	identity, authority, err := sshutil.GenerateKey("Workshop-CA")
	c.Assert(err, check.IsNil)

	cert, err := authority.SignUserKey(*pub, []string{"workshop"})
	c.Assert(err, check.IsNil)

	scratch := c.MkDir()
	err = os.WriteFile(filepath.Join(scratch, "id_ed25519-cert.pub"), []byte(cert.String()+"\n"), 0600)
	c.Assert(err, check.IsNil)

	cmd := exec.Command("ssh-keygen", "-Lf", "id_ed25519-cert.pub")
	cmd.Dir = scratch
	out, err := cmd.Output()
	c.Assert(err, check.IsNil)

	fingerprint := regexp.QuoteMeta(pub.Fingerprint())
	ca := regexp.QuoteMeta(identity.Fingerprint())

	template := `id_ed25519-cert.pub:
\s*Type: ssh-ed25519.* user certificate
\s*Public key: ED25519-CERT %s
\s*Signing CA: ED25519 %s \(using ssh-ed25519\)
\s*Key ID: "workshop@wp"
\s*Serial: 0
\s*Valid: forever
\s*Principals:\s*
\s*workshop
\s*Critical Options: \(none\)
\s*Extensions:\s*
\s*permit-X11-forwarding
\s*permit-agent-forwarding
\s*permit-port-forwarding
\s*permit-pty
\s*permit-user-rc
`
	pattern := fmt.Sprintf(template, fingerprint, ca)
	c.Check(string(out), check.Matches, pattern)
}

func (s *sshSuite) TestGenerateKey(c *check.C) {
	pub, priv, err := sshutil.GenerateKey("root@dev-42424242.wp")
	c.Assert(err, check.IsNil)

	data, err := priv.MarshalText()
	c.Assert(err, check.IsNil)

	scratch := c.MkDir()
	err = os.WriteFile(filepath.Join(scratch, "id_ed25519"), data, 0600)
	c.Assert(err, check.IsNil)

	cmd := exec.Command("ssh-keygen", "-yf", "id_ed25519")
	cmd.Dir = scratch
	out, err := cmd.Output()
	c.Assert(err, check.IsNil)
	c.Check(string(out), check.Equals, pub.String()+"\n")
}

func (s *sshSuite) TestParsePublicKey(c *check.C) {
	pub, _, err := sshutil.GenerateKey("root@dev-42424242.wp")
	c.Assert(err, check.IsNil)

	key, err := sshutil.ParsePublicKey([]byte(pub.String() + "\n"))
	c.Assert(err, check.IsNil)
	c.Check(key.Equal(*pub), check.Equals, true)
}

func (s *sshSuite) TestParsePrivateKey(c *check.C) {
	_, priv, err := sshutil.GenerateKey("root@dev-42424242.wp")
	c.Assert(err, check.IsNil)

	data, err := priv.MarshalText()
	c.Assert(err, check.IsNil)

	key, err := sshutil.ParsePrivateKey(data, "root@dev-42424242.wp")
	c.Assert(err, check.IsNil)
	c.Check(key.Equal(*priv), check.Equals, true)
}
