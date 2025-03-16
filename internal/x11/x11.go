package x11

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
)

// Copies the user's $XAUTHORITY file to the Workshopd run directory.
func MigrateXauthority(user *user.User, xauthPath string) (err error) {
	if xauthPath == "" {
		return fmt.Errorf("xauth cannot be empty")
	}

	// We place the Xauthority inside a parent folder to ensure that the mounted
	// cookie is updated when the host cookie changes (ie. reboot). This entire
	// parent folder is mounted inside the workshop.
	// https://discuss.linuxcontainers.org/t/mount-single-file/17975
	destDir := filepath.Join(dirs.WorkshopdRunDir, user.Uid, "Xauthority")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// We are performing a Stat() here to ensure that the user can't steal
	// another user's Xauthority file. Note that while Stat() uses fstat() on the
	// file descriptor created during Open(), the file might have changed
	// ownership between the Open() and the Stat(). That's ok because we aren't
	// trying to block access that the user already has: if the user has the
	// privileges to chown another user's Xauthority file, we won't block that
	// since the user can just steal it without having to use workshop. This code
	// is just to ensure that a user who doesn't have those privileges can't
	// steal the file via 'workshop connect'
	f, err := os.Stat(xauthPath)
	if err != nil {
		return err
	}
	fsys := f.Sys()
	if fsys == nil {
		return fmt.Errorf("cannot validate owner of file %s", f.Name())
	}
	// cheap comparison as the current uid is only available as a string
	// but it is better to convert the uid from the stat result to a
	// string than a string into a number.
	if fmt.Sprintf("%d", fsys.(*syscall.Stat_t).Uid) != user.Uid {
		return fmt.Errorf("Xauthority file isn't owned by the current user %s", user.Uid)
	}

	xauth, err := os.Open(xauthPath)
	if err != nil {
		return err
	}
	defer xauth.Close()

	xauthEntries, err := ProcessFile(xauth)
	if err != nil {
		return err
	}

	for i := range xauthEntries {
		xauthEntries[i].Family = FamilyWild
		xauthEntries[i].Host = []byte("workshop")
	}

	b := EncodeEntries(xauthEntries)

	err = os.WriteFile(filepath.Join(destDir, ".Xauthority"), b, 0644)
	if err != nil {
		return err
	}

	uid, gid, err := osutil.UidGid(user)
	if err != nil {
		return err
	}

	if err = sys.ChownPath(filepath.Join(destDir, ".Xauthority"), uid, gid); err != nil {
		return err
	}

	return nil
}
