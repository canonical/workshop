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

package lxdbackend

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sshutil"
	"github.com/canonical/workshop/internal/workshop"
)

func sshConfig(usr *user.User, hostname string) (map[string]string, error) {
	if err := ensureConfigFile(); err != nil {
		return nil, err
	}

	identity, authority, err := createOrLoadCAKeys(usr)
	if err != nil {
		return nil, err
	}

	pub, priv, err := sshutil.GenerateKey("root@" + hostname)
	if err != nil {
		return nil, err
	}
	data, err := priv.MarshalText()
	if err != nil {
		return nil, err
	}

	cert, err := authority.SignHostKey(*pub)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"user.ed25519-key.private":     string(data),
		"user.ed25519-key.public":      pub.String(),
		"user.ed25519-key.certificate": cert.String(),
		"user.ed25519-key.workshop-ca": identity.String(),
	}, nil
}

func ensureConfigFile() error {
	path := filepath.Join(dirs.WorkshopSSHDir, "config")
	if osutil.FileExists(path) {
		return nil
	}

	configTemplate := `
Host "*.{{sshescape .Domain}}"
	CertificateFile "{{sshescape .Path}}/%i/id_ed25519-cert.pub"
	IdentitiesOnly yes
	IdentityFile "{{sshescape .Path}}/%i/id_ed25519"
	User "{{sshescape .User}}"
	UserKnownHostsFile "{{sshescape .Path}}/%i/known_hosts"
`[1:]

	var config bytes.Buffer
	funcs := map[string]any{
		"sshescape": sshEscape,
	}
	dot := struct {
		Domain string
		Path   string
		User   string
	}{
		Domain: networkDomain,
		Path:   dirs.WorkshopSSHDir,
		User:   workshop.User.Username,
	}
	t := template.Must(template.New("ssh_config").Funcs(funcs).Parse(configTemplate))
	if err := t.Execute(&config, dot); err != nil {
		return err
	}

	return osutil.AtomicWrite(path, &config, 0644, 0)
}

var sshEscaper = strings.NewReplacer("%", "%%", "\\", "\\\\", "\"", "\\\"")

func sshEscape(text string) (string, error) {
	if strings.Contains(text, "${") || strings.Contains(text, "\n") || strings.Contains(text, "\x00") {
		return "", fmt.Errorf("unrepresentable SSH config value: %q", text)
	}
	return sshEscaper.Replace(text), nil
}

func createOrLoadCAKeys(usr *user.User) (*sshutil.PublicKey, *sshutil.PrivateKey, error) {
	identity, authority, err := loadCAKeys(usr)
	if !errors.Is(err, os.ErrNotExist) {
		return identity, authority, err
	}

	if err := ensureCAKeys(usr); err != nil {
		return nil, nil, err
	}
	return loadCAKeys(usr)
}

func loadCAKeys(usr *user.User) (*sshutil.PublicKey, *sshutil.PrivateKey, error) {
	data, err := os.ReadFile(filepath.Join(dirs.WorkshopSSHDir, usr.Uid, "id_ed25519_ca.pub"))
	if err != nil {
		return nil, nil, err
	}

	identity, err := sshutil.ParsePublicKey(data)
	if err != nil {
		return nil, nil, err
	}

	data, err = os.ReadFile(filepath.Join(dirs.WorkshopSSHDir, usr.Uid, "id_ed25519_ca"))
	if err != nil {
		return nil, nil, err
	}

	authority, err := sshutil.ParsePrivateKey(data, identity.Comment())
	if err != nil {
		return nil, nil, err
	}

	return identity, authority, nil
}

func ensureCAKeys(usr *user.User) error {
	if err := os.MkdirAll(dirs.WorkshopSSHDir, 0755); err != nil {
		return err
	}

	removeTemp := revert.New()
	defer removeTemp.Fail()

	temp, err := os.MkdirTemp(dirs.WorkshopSSHDir, usr.Uid+".*~")
	if err != nil {
		return err
	}
	removeTemp.Add(func() { _ = os.RemoveAll(temp) })

	closeDir := revert.New()
	defer closeDir.Fail()

	d, err := os.Open(temp)
	if err != nil {
		return err
	}
	closeDir.Add(func() { d.Close() })

	if err := d.Chmod(0755); err != nil {
		return err
	}

	if err := writeCAKeys(usr, temp); err != nil {
		return err
	}

	if err := d.Sync(); err != nil {
		return err
	}
	if err := d.Close(); err != nil {
		return err
	}
	closeDir.Success()

	target := filepath.Join(dirs.WorkshopSSHDir, usr.Uid)
	// One error comes from Go's pre-existence check, the other from syscall.Rename.
	if err := os.Rename(temp, target); errors.Is(err, os.ErrExist) || errors.Is(err, syscall.ENOTEMPTY) {
		// Someone else beat us to it, discard the keys and temp dir.
		return nil
	} else if err != nil {
		return err
	}

	removeTemp.Success()
	return nil
}

func writeCAKeys(usr *user.User, temp string) error {
	identity, authority, err := sshutil.GenerateKey("Workshop-CA")
	if err != nil {
		return err
	}

	pub, priv, err := sshutil.GenerateKey(workshop.User.Username + "@" + networkDomain)
	if err != nil {
		return err
	}

	cert, err := authority.SignUserKey(*pub, []string{workshop.User.Username})
	if err != nil {
		return err
	}

	uid, gid, err := osutil.UidGid(usr)
	if err != nil {
		return err
	}

	knownHosts := fmt.Sprintf("@cert-authority *.%s %s\n", networkDomain, identity)

	if err := writePublicKey(filepath.Join(temp, "id_ed25519_ca.pub"), *identity, osutil.NoChown, osutil.NoChown); err != nil {
		return err
	}
	if err := writePrivateKey(filepath.Join(temp, "id_ed25519_ca"), *authority, osutil.NoChown, osutil.NoChown); err != nil {
		return err
	}
	if err := writePublicKey(filepath.Join(temp, "id_ed25519.pub"), *pub, uid, gid); err != nil {
		return err
	}
	if err := writePrivateKey(filepath.Join(temp, "id_ed25519"), *priv, uid, gid); err != nil {
		return err
	}
	if err := writePublicKey(filepath.Join(temp, "id_ed25519-cert.pub"), *cert, uid, gid); err != nil {
		return err
	}
	return writeFileSync(filepath.Join(temp, "known_hosts"), []byte(knownHosts), 0644, uid, gid)
}

func writePublicKey(name string, key sshutil.PublicKey, uid sys.UserID, gid sys.GroupID) error {
	return writeFileSync(name, []byte(key.String()+"\n"), 0644, uid, gid)
}

func writePrivateKey(name string, key sshutil.PrivateKey, uid sys.UserID, gid sys.GroupID) error {
	pem, err := key.MarshalText()
	if err != nil {
		return err
	}
	return writeFileSync(name, pem, 0600, uid, gid)
}

func writeFileSync(name string, data []byte, perm os.FileMode, uid sys.UserID, gid sys.GroupID) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil && (uid != osutil.NoChown || gid != osutil.NoChown) {
		err = sys.Chown(f, uid, gid)
	}
	if err == nil {
		err = f.Sync()
	}
	return cmp.Or(err, f.Close())
}
