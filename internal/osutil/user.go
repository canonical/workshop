// Copyright (c) 2014-2020 Canonical Ltd
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

package osutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil/sys"
)

var (
	UserCurrent       = user.Current
	UserLookup        = user.Lookup
	UserLookupGroup   = user.LookupGroup
	UserEnv           = userEnvironment
	CurrentUserAndEnv = currentUserAndEnv
	Timezone          = timezone
)

// RealUser finds the user behind a sudo invocation when root, if applicable
// and possible.
//
// Don't check SUDO_USER when not root and simply return the current uid
// to properly support sudo'ing from root to a non-root user
func RealUser() (*user.User, error) {
	cur, err := UserCurrent()
	if err != nil {
		return nil, err
	}

	// not root, so no sudo invocation we care about
	if cur.Uid != "0" {
		return cur, nil
	}

	realName := os.Getenv("SUDO_USER")
	if realName == "" {
		// not sudo; current is correct
		return cur, nil
	}

	real, err := user.Lookup(realName)
	// can happen when sudo is used to enter a chroot (e.g. pbuilder)
	if _, ok := err.(user.UnknownUserError); ok {
		return cur, nil
	}
	if err != nil {
		return nil, err
	}

	return real, nil
}

// UidGid returns the uid and gid of the given user, as uint32s
//
// XXX this should go away soon
func UidGid(u *user.User) (sys.UserID, sys.GroupID, error) {
	// XXX this will be wrong for high uids on 32-bit arches (for now)
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return sys.FlagID, sys.FlagID, fmt.Errorf("cannot parse user id %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return sys.FlagID, sys.FlagID, fmt.Errorf("cannot parse group id %q: %w", u.Gid, err)
	}

	return sys.UserID(uid), sys.GroupID(gid), nil
}

// NormalizeUidGid returns the "normalized" UID and GID for the given IDs and
// names. If both uid and username are specified, the username's UID must match
// the given uid (similar for gid and group), otherwise an error is returned.
func NormalizeUidGid(uid, gid *int, username, group string) (*int, *int, error) {
	if uid == nil && username == "" && gid == nil && group == "" {
		return nil, nil, nil
	}
	if username != "" {
		u, err := UserLookup(username)
		if err != nil {
			return nil, nil, err
		}
		n, _ := strconv.Atoi(u.Uid)
		if uid != nil && *uid != n {
			return nil, nil, fmt.Errorf("user %q UID (%d) does not match user-id (%d)",
				username, n, *uid)
		}
		uid = &n
		if gid == nil && group == "" {
			// Group not specified; use user's primary group ID
			gidVal, _ := strconv.Atoi(u.Gid)
			gid = &gidVal
		}
	}
	if group != "" {
		g, err := UserLookupGroup(group)
		if err != nil {
			return nil, nil, err
		}
		n, _ := strconv.Atoi(g.Gid)
		if gid != nil && *gid != n {
			return nil, nil, fmt.Errorf("group %q GID (%d) does not match group-id (%d)",
				group, n, *gid)
		}
		gid = &n
	}
	if uid == nil && gid != nil {
		return nil, nil, fmt.Errorf("must specify user, not just group")
	}
	if uid != nil && gid == nil {
		return nil, nil, fmt.Errorf("must specify group, not just UID")
	}
	return uid, gid, nil
}

// UserMaybeSudoUser finds the user behind a sudo invocation when root, if
// applicable and possible. Otherwise the current user is returned.
//
// Don't check SUDO_USER when not root and simply return the current uid
// to properly support sudo'ing from root to a non-root user
func UserMaybeSudoUser() (*user.User, error) {
	cur, err := UserCurrent()
	if err != nil {
		return nil, err
	}

	// not root, so no sudo invocation we care about
	if cur.Uid != "0" {
		return cur, nil
	}

	realName := os.Getenv("SUDO_USER")
	if realName == "" {
		// not sudo; current is correct
		return cur, nil
	}

	real, err := user.Lookup(realName)
	// This is a best effort, see the comment in findGidNoGetentFallback in
	// group.go.
	//
	// But here the effect is not worrisome, because if we fail to
	// identify the error as unknown user, we will just fail here and won't
	// inadvertently raise or lower permissions, as the current user is already
	// root in this codepath
	if isUnknownUserOrEnoent(err) {
		return cur, nil
	}
	if err != nil {
		return nil, err
	}

	return real, nil
}

func UserAndEnv(name string) (*user.User, map[string]string, error) {
	usr, err := UserLookup(name)
	if err != nil {
		return nil, nil, err
	}

	env, err := UserEnv(usr)
	if err != nil {
		return nil, nil, err
	}

	return usr, env, err
}

func currentUserAndEnv() (*user.User, map[string]string, error) {
	usr, err := UserCurrent()
	if err != nil {
		return nil, nil, err
	}

	env, err := userEnvironment(usr)
	if err != nil {
		return nil, nil, err
	}

	return usr, env, err
}

// Returns the environment for the user as set by systemd, or the system
// environment if user is root. This is the equivalent of running
// `systemctl [--user] show-environment`.
func userEnvironment(user *user.User) (map[string]string, error) {
	// When running as the target user, systemctl can connect to the user
	// bus directly, but this requires XDG_RUNTIME_DIR to be set. Other
	// users have to use a more complicated connection process via the
	// --machine argument. It's likely that non-root users won't have
	// permission to do this, but we leave that up to systemd. In practice
	// we can define XDG_RUNTIME_DIR and pass --machine in both cases, but
	// --machine is ignored in the first case (given that it matches the
	// current user), and XDG_RUNTIME_DIR is incorrect in the second case.
	// See https://github.com/systemd/systemd/issues/39838.
	var args []string
	env := os.Environ()
	uid, err := strconv.ParseInt(user.Uid, 10, 64)
	if err == nil && uid == 0 {
		args = []string{"show-environment"}
	} else if err == nil && uid == int64(os.Geteuid()) {
		args = []string{"--user", "show-environment"}
		defaultXdg := filepath.Join(dirs.XdgRuntimeDirBase, user.Uid)
		defaultEnv := []string{"XDG_RUNTIME_DIR=" + defaultXdg}
		env = append(defaultEnv, env...)
	} else {
		machine := fmt.Sprintf("--machine=%s@.host", user.Uid)
		args = []string{machine, "--user", "show-environment"}
	}
	cmd := exec.Command("systemctl", args...)
	cmd.Env = env

	out, errOut, err := RunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("systemctl show-environment: %s", errOut)
	}

	// TODO: use --output=json once systemd >= 250.
	rawEnv := strings.FieldsFunc(string(out), func(r rune) bool { return r == '\n' })
	userEnv, err := parseSystemctlEnvironment(rawEnv)
	if err != nil {
		return nil, err
	}

	// Work around microsoft/WSL#12436: the systemd --user manager on WSL
	// lacks the WSLg display variables, so fill them in here.
	supplementWSLgDisplayEnv(user, userEnv)

	return userEnv, nil
}

// wslgMountDir is where WSLg exposes the display sockets and PulseAudio
// server. Its presence is the signal that WSLg's DISPLAY/WAYLAND_DISPLAY
// sockets are actually available on this WSL instance.
var wslgMountDir = "/mnt/wslg"

// IsWSL reports whether the host is running under Windows Subsystem for Linux.
func IsWSL() bool {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return false
	}
	data := utsname.Release[:]
	if idx := bytes.IndexByte(data, 0); idx >= 0 {
		data = data[:idx]
	}
	version := strings.ToLower(string(data))
	return strings.Contains(version, "microsoft") || strings.Contains(version, "wsl2")
}

// onWSLg reports whether the host is a WSL instance with WSLg available.
var onWSLg = func() bool {
	if !IsWSL() {
		return false
	}
	_, err := os.Stat(wslgMountDir)
	return err == nil
}

// supplementWSLgDisplayEnv works around microsoft/WSL#12436. On WSL, WSLg
// injects DISPLAY, WAYLAND_DISPLAY and friends only into login shells (via
// /etc/profile.d) and never into the systemd --user manager. As a result
// `systemctl --user show-environment` omits them, and the desktop interface
// reports "neither DISPLAY nor WAYLAND_DISPLAY are set" even though the host
// compositor is reachable. When WSLg is available we fill in its well-known,
// stable socket values for any variable the user manager did not provide.
func supplementWSLgDisplayEnv(user *user.User, env map[string]string) {
	if !onWSLg() {
		return
	}
	defaults := map[string]string{
		"DISPLAY":         ":0",
		"WAYLAND_DISPLAY": "wayland-0",
		"XDG_RUNTIME_DIR": filepath.Join(dirs.XdgRuntimeDirBase, user.Uid),
	}
	for key, value := range defaults {
		if env[key] == "" {
			env[key] = value
		}
	}
}

func timezone() (string, error) {
	// Compatibility: timedatectl show was added in systemd v239:
	// https://github.com/systemd/systemd/pull/9250
	cmd := exec.Command("timedatectl", "show", "--property=Timezone", "--value")
	out, errOut, err := RunCmd(cmd)
	if err != nil {
		return "", fmt.Errorf("timedatectl show: %s", errOut)
	}

	timezone := strings.TrimSpace(string(out))
	return timezone, nil
}

func FakeUserEnvironment(f func(user *user.User) (map[string]string, error)) func() {
	UserEnv = f
	return func() {
		UserEnv = userEnvironment
	}
}

func FakeCurrentUserAndEnv(f func() (*user.User, map[string]string, error)) func() {
	CurrentUserAndEnv = f
	return func() {
		CurrentUserAndEnv = currentUserAndEnv
	}
}

func FakeUserCurrent(f func() (*user.User, error)) func() {
	realUserCurrent := UserCurrent
	UserCurrent = f

	return func() { UserCurrent = realUserCurrent }
}

func FakeUserLookup(f func(name string) (*user.User, error)) func() {
	oldUserLookup := UserLookup
	UserLookup = f
	return func() { UserLookup = oldUserLookup }
}

func FakeUserLookupGroup(f func(name string) (*user.Group, error)) func() {
	oldUserLookupGroup := UserLookupGroup
	UserLookupGroup = f
	return func() { UserLookupGroup = oldUserLookupGroup }
}

func FakeTimezone(f func() (string, error)) func() {
	oldTimezone := Timezone
	Timezone = f
	return func() { Timezone = oldTimezone }
}

// Note: this is best effort, comparing err here with UnknownUserError
// is inherently flawed and may end up missing some legitimate unknown
// user errors, see the comment on findGidNoGetentFallback in group.go
// for more details. It seems the most common return value is ENOENT so
// check for that too (e.g. when the sssd package is installed).
func isUnknownUserOrEnoent(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(user.UnknownUserError); ok {
		return true
	}
	// Check for ENOENT, ideally go itself would handle this, see
	// https://github.com/golang/go/issues/40334 for the upstream
	// bug
	return strings.HasSuffix(err.Error(), syscall.ENOENT.Error())
}
