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
func MigrateXauthority(user *user.User, xauth string) (err error) {
	if xauth == "" {
		return fmt.Errorf("xauth cannot be empty")
	}

	uid, gid, err := osutil.UidGid(user)
	if err != nil {
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
	f, err := os.Stat(xauth)
	if err != nil {
		return err
	}
	fuid, _, err := sys.FileOwner(f)
	if err != nil {
		return &os.PathError{Op: "stat", Path: xauth, Err: err}
	}
	if fuid != uid {
		return &os.PathError{Op: "open", Path: xauth, Err: syscall.EACCES}
	}

	// We place the Xauthority inside a parent folder to ensure that the mounted
	// cookie is updated when the host cookie changes (ie. reboot). This entire
	// parent folder is mounted inside the workshop.
	// https://discuss.linuxcontainers.org/t/mount-single-file/17975
	destDir := filepath.Join(dirs.WorkshopdRunDir, user.Uid, "Xauthority")
	if err := os.MkdirAll(dirs.WorkshopdRunDir, 0o755); err != nil {
		return err
	}
	if err := osutil.MkdirAllChown(destDir, 0o700, uid, gid); err != nil {
		return err
	}

	destFile := filepath.Join(destDir, ".Xauthority")
	err = osutil.CopyFile(xauth, filepath.Join(destDir, ".Xauthority"), osutil.CopyFlagOverwrite)
	if err != nil {
		return err
	}

	if err = sys.ChownPath(destFile, uid, gid); err != nil {
		return err
	}

	return nil
}
