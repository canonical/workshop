package lxd_device

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/x-go/strutil/shlex"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/x11"
)

var workshopFs = sftpFs

type Backend struct {
}

func (b *Backend) Initialize() error {
	return nil
}

// Name returns the name of the backend.
func (b *Backend) Name() interfaces.SecuritySystem {
	return interfaces.SecurityLxdDevice
}

func setupMounts(conn lxd.InstanceServer, fs fsutil.Fs, user *user.User, pid, w string, prev, next map[string]workshop.Mount) (*revert.Reverter, error) {
	var mounts *osutil.MountProfile
	var content []byte
	if containsWorkshopWorkshop(prev) || containsWorkshopWorkshop(next) {
		var err error
		mounts, content, err = readMountProfile(fs)
		if err != nil {
			return nil, err
		}
	}

	var removed []string
	for _, mnt := range prev {
		if mnt == next[mnt.Name] {
			continue
		}
		if removeMountEntry(mounts, mnt) {
			removed = append(removed, mnt.Where)
		}
	}

	var added []string
	for _, mnt := range next {
		if mnt == prev[mnt.Name] {
			continue
		}
		if err := prepareMount(fs, user, mnt); err != nil {
			return nil, err
		}
		if addMountEntry(mounts, mnt) {
			added = append(added, mnt.Where)
		}
	}

	rev := revert.New()
	if len(removed) == 0 && len(added) == 0 {
		return rev, nil
	}
	defer rev.Fail()

	rev.Add(func() {
		if reverr := reloadMounts(conn, pid, w); reverr != nil {
			logger.Noticef("On setupMounts: cannot undo unmount in workshop %q: %v", w, reverr)
		}
	})

	for _, where := range removed {
		if err := unmount(conn, pid, w, where); err != nil {
			logger.Noticef("On setupMounts: cannot unmount in workshop %q: %v", w, err)
		}
	}

	if err := writeMountProfile(fs, mounts); err != nil {
		return nil, err
	}

	rev.Add(func() {
		for _, where := range added {
			if reverr := unmount(conn, pid, w, where); reverr != nil {
				logger.Noticef("On setupMounts: cannot undo mount in workshop %q: %v", w, reverr)
			}
		}
		if reverr := writeMountProfile(fs, bytes.NewBuffer(content)); reverr != nil {
			logger.Noticef("On setupMounts: cannot restore mount profile: %v", reverr)
		}
	})

	if err := reloadMounts(conn, pid, w); err != nil {
		return nil, err
	}

	clone := rev.Clone()
	rev.Success()
	return clone, nil
}

func removeMounts(conn lxd.InstanceServer, fs fsutil.Fs, pid, w string, prev map[string]workshop.Mount) error {
	var mounts *osutil.MountProfile
	if containsWorkshopWorkshop(prev) {
		var err error
		mounts, _, err = readMountProfile(fs)
		if err != nil {
			return err
		}
	}

	var removed []string
	for _, mnt := range prev {
		if removeMountEntry(mounts, mnt) {
			removed = append(removed, mnt.Where)
		}
	}

	if len(removed) == 0 {
		return nil
	}

	var errs []error
	for _, where := range removed {
		if err := unmount(conn, pid, w, where); err != nil {
			errs = append(errs, err)
		}
	}

	if err := writeMountProfile(fs, mounts); err != nil {
		errs = append(errs, err)
	} else if err := reloadMounts(conn, pid, w); err != nil {
		errs = append(errs, err)
	}

	return cmp.Or(errs...)
}

func containsWorkshopWorkshop(mounts map[string]workshop.Mount) bool {
	for _, mnt := range mounts {
		if mnt.Type == workshop.WorkshopWorkshop {
			return true
		}
	}
	return false
}

func prepareMount(fs fsutil.Fs, user *user.User, mnt workshop.Mount) error {
	switch mnt.Type {
	case workshop.HostWorkshop:
		return prepareHostWorkshopMount(fs, user, mnt)
	case workshop.WorkshopWorkshop:
		return prepareWorkshopWorkshopMount(fs, mnt)
	default:
		return fmt.Errorf(`unknown device type: %v`, mnt.Type)
	}
}

func prepareHostWorkshopMount(fs fsutil.Fs, user *user.User, mnt workshop.Mount) error {
	info, err := os.Stat(mnt.What)
	isDir := err == nil && info.IsDir()

	if mnt.MakeWhat && !isDir {
		uid, gid, err := osutil.UidGid(user)
		if err != nil {
			return err
		}

		if err := osutil.MkdirAllChown(mnt.What, 0755, uid, gid); err != nil {
			return err
		}
		// Only change permissions for mounted directory, and ignore umask.
		if err := os.Chmod(mnt.What, mnt.Mode); err != nil {
			return err
		}
		isDir = true
	} else if err != nil {
		return err
	}

	return prepareMountWhere(fs, mnt, isDir)
}

func prepareWorkshopWorkshopMount(fs fsutil.Fs, mnt workshop.Mount) error {
	isDir := true
	if mnt.MakeWhat {
		if err := fs.MkdirAllChmodChown(mnt.What, mnt.Mode, int(mnt.Owner), int(mnt.Group)); err != nil {
			return err
		}
	} else {
		info, err := fs.Stat(mnt.What)
		if err != nil {
			return err
		}
		isDir = info.IsDir()
	}

	return prepareMountWhere(fs, mnt, isDir)
}

func prepareMountWhere(fs fsutil.Fs, mnt workshop.Mount, isDir bool) error {
	if !mnt.MakeWhere {
		return checkMountWhere(fs, mnt, isDir)
	}

	if isDir {
		return fs.MkdirAllChmodChown(mnt.Where, mnt.Mode, int(mnt.Owner), int(mnt.Group))
	}

	parent := filepath.Dir(mnt.Where)
	if err := fs.MkdirAllChmodChown(parent, mnt.Mode, int(mnt.Owner), int(mnt.Group)); err != nil {
		return err
	}

	file, err := fs.OpenFile(mnt.Where, os.O_RDWR|os.O_CREATE|os.O_EXCL, mnt.Mode)
	if errors.Is(err, os.ErrExist) {
		return checkMountWhere(fs, mnt, isDir)
	}
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Chmod(mnt.Mode); err != nil {
		return err
	}
	return file.Chown(int(mnt.Owner), int(mnt.Group))
}

func checkMountWhere(fs fsutil.Fs, mnt workshop.Mount, isDir bool) error {
	info, err := fs.Stat(mnt.Where)
	if err != nil || info.IsDir() == isDir {
		return err
	}
	err = syscall.ENOTDIR
	if info.IsDir() {
		err = syscall.EISDIR
	}
	return &os.PathError{Op: "mount", Path: mnt.Where, Err: err}
}

func addMountEntry(mounts *osutil.MountProfile, mnt workshop.Mount) bool {
	if mnt.Type != workshop.WorkshopWorkshop {
		return false
	}

	check := func(me osutil.MountEntry) bool {
		return me.Name == mnt.What && me.Dir == mnt.Where && mnt.ReadOnly == slices.Contains(me.Options, "ro")
	}
	if slices.ContainsFunc(mounts.Entries, check) {
		return false
	}

	options := []string{"bind", "x-systemd.requires=/project"}
	if mnt.ReadOnly {
		options = append(options, "ro")
	}

	entry := osutil.MountEntry{Name: mnt.What, Dir: mnt.Where, Type: "none", Options: options}
	mounts.Entries = append(mounts.Entries, entry)
	return true
}

func removeMountEntry(mounts *osutil.MountProfile, mnt workshop.Mount) bool {
	if mnt.Type != workshop.WorkshopWorkshop {
		return false
	}

	cnt := len(mounts.Entries)
	deleter := func(me osutil.MountEntry) bool {
		return me.Name == mnt.What && me.Dir == mnt.Where
	}
	mounts.Entries = slices.DeleteFunc(mounts.Entries, deleter)
	return cnt != len(mounts.Entries)
}

func readMountProfile(fs fsutil.Fs) (*osutil.MountProfile, []byte, error) {
	fstab, err := fs.Open("/etc/fstab")
	if errors.Is(err, os.ErrNotExist) {
		return &osutil.MountProfile{}, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	defer fstab.Close()

	var content bytes.Buffer
	mounts, err := osutil.ReadMountProfile(io.TeeReader(fstab, &content))
	if err != nil {
		return nil, nil, err
	}
	return mounts, content.Bytes(), nil
}

func writeMountProfile(fs fsutil.Fs, mounts io.WriterTo) error {
	return fs.AtomicWriteTo(mounts, "/etc/fstab", 0644)
}

func runMountCommand(conn lxd.InstanceServer, pid, w string, cmd []string) error {
	c := api.InstanceExecPost{
		Command:     cmd,
		Interactive: false,
	}

	args := lxd.InstanceExecArgs{}

	op, err := conn.ExecInstance(lxdbackend.InstanceName(w, pid), c, &args)
	if err != nil {
		return err
	}
	if err := op.Wait(); err != nil {
		switch err.Error() {
		case "Command not executable", "Command not found":
			// Usually a nonzero exit status is not an error,
			// but LXD translates 126 and 127 into the above messages.
		default:
			return fmt.Errorf("%s: %w", strings.Join(cmd, " "), err)
		}
	}

	if status := int(op.Get().Metadata["return"].(float64)); status != 0 {
		err := &workshop.ErrExec{Status: status}
		return fmt.Errorf("%s: %w", strings.Join(cmd, " "), err)
	}
	return nil
}

func unmount(conn lxd.InstanceServer, pid, w string, where string) error {
	return runMountCommand(conn, pid, w, []string{"umount", where})
}

// 'systemd-fstab-generator' is responsible for creating mount entries from
// fstab. Because of this, we need to first ensure it runs (generating the
// on-demand unit files) by calling daemon-reload, and then activate the
// newly-creaed units by restarting a downstream target (ie. local-fs) see:
// https://www.freedesktop.org/software/systemd/man/latest/systemd.special.html
func reloadMounts(conn lxd.InstanceServer, pid, w string) error {
	if err := runMountCommand(conn, pid, w, []string{"systemctl", "daemon-reload"}); err != nil {
		return err
	}
	return runMountCommand(conn, pid, w, []string{"systemctl", "restart", "local-fs.target"})
}

func setupSshAgent(fs fsutil.Fs, prev, next *workshop.SshAgent) error {
	if prev.Equal(next) {
		return nil
	}
	if next == nil {
		return removeSshAgent(fs, prev)
	}

	script := "/etc/profile.d/workshop-ssh-agent.sh"
	if prev == nil {
		if _, err := fs.Stat(script); err == nil {
			return errors.New("ssh-agent interface already connected")
		}
	}

	envVars := map[string]string{"SSH_AUTH_SOCK": next.Listen.Address}
	return fs.AtomicWriteTo(envScript(envVars), script, 0644)
}

func removeSshAgent(fs fsutil.Fs, prev *workshop.SshAgent) error {
	if prev == nil {
		return nil
	}

	script := "/etc/profile.d/workshop-ssh-agent.sh"
	return fs.RemoveIfExists(script)
}

func setupDesktop(fs fsutil.Fs, user *user.User, env map[string]string, prev, next *workshop.Desktop) error {
	if prev.Equal(next) {
		return nil
	}
	if next == nil {
		return removeDesktop(fs, prev)
	}

	script := "/etc/profile.d/workshop-desktop.sh"
	if prev == nil {
		if _, err := fs.Stat(script); err == nil {
			return errors.New("desktop interface already connected")
		}
	}

	envVars := desktopEnvironment(user, env, *next)
	return fs.AtomicWriteTo(envScript(envVars), script, 0644)
}

func removeDesktop(fs fsutil.Fs, prev *workshop.Desktop) error {
	if prev == nil {
		return nil
	}

	script := "/etc/profile.d/workshop-desktop.sh"
	err := fs.RemoveIfExists(script)
	err2 := fs.RemoveIfExists("/tmp/.Xauthority")
	return cmp.Or(err, err2)
}

func desktopEnvironment(user *user.User, env map[string]string, dev workshop.Desktop) map[string]string {
	var envVars map[string]string
	// Use Wayland as the default backend in the case where it's unset
	backend := env["XDG_BACKEND"]
	if (backend == "wayland" || backend == "") && dev.Wayland != nil {
		envVars = map[string]string{
			"QT_QPA_PLATFORM":  "wayland-egl",
			"XDG_SESSION_TYPE": "wayland",
			"XDG_BACKEND":      "wayland",
		}
	} else {
		envVars = map[string]string{
			"QT_QPA_PLATFORM":  "xcb",
			"XDG_SESSION_TYPE": "x11",
			"XDG_BACKEND":      "x11",
		}
	}

	if dev.Wayland != nil {
		prefix := filepath.Join(dirs.XdgRuntimeDirBase, workshop.User.Uid) + "/"
		envVars["WAYLAND_DISPLAY"] = strings.TrimPrefix(dev.Wayland.Listen.Address, prefix)
	}

	if dev.X11 != nil {
		envVars["DISPLAY"] = ":" + strings.TrimPrefix(filepath.Base(dev.X11.Listen.Address), "X")
	}

	// The .Xauthority cookie contains a 128bit key used to authenticate consumers
	// of the X11 socket. It is generated on each boot with a random suffix,
	// because of this we need to ensure there exists a consistently-named copy
	// of the cookie for the LXC profile. There are two cases where we need to
	// copy the cookie, one is on workshopd startup as we iterate through the
	// list of projects, the other is on connect because this could be the first
	// workshop launched, in which case the user would not have had a project. We
	// handle it here for the connect, presence of the copied cookie after reboot
	// is the responsibility of the interface manager.
	xauth := env["XAUTHORITY"]
	if xauth != "" {
		envVars["XAUTHORITY"] = "/tmp/.Xauthority"
		if err := x11.MigrateXauthority(user, xauth); err != nil {
			logger.Noticef("cannot migrate Xauthority file for user %s, X11 applications may not work: %v", user.Username, err)
		}
	}

	envVars["ELECTRON_OZONE_PLATFORM_HINT"] = "auto"
	return envVars
}

type envScript map[string]string

func (e envScript) WriteTo(w io.Writer) (int64, error) {
	var n int64
	for k, v := range e {
		m, err := fmt.Fprintf(w, "export %s=%s\n", k, shlex.Quote(v))
		n += int64(m)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func maybeEnableAmdKfd(s *Specification) {
	const kfdPath = "/dev/kfd"
	_, err := os.Stat(kfdPath)
	if err != nil {
		return
	}

	kfdName := lxdbackend.DeviceName(s.Profile.Sdk, s.Profile.Gpu.Name) + "_kfd"
	s.devices[kfdName] = map[string]string{
		"type":     "unix-char",
		"source":   kfdPath,
		"path":     kfdPath,
		"required": "false",
		"gid":      workshop.User.Gid,
	}
}

func sftpFs(conn lxd.InstanceServer, pid, w string) (fsutil.Fs, error) {
	sftp, err := conn.GetInstanceFileSFTP(lxdbackend.InstanceName(w, pid))
	if err != nil {
		return fsutil.Fs{}, err
	}
	return fsutil.NewSftpFs(sftp, workshop.RootUmask), nil
}

func assignNewProfile(ctx context.Context, conn lxd.InstanceServer, sdkRef sdk.Ref) (*revert.Reverter, error) {
	rev := revert.New()
	defer rev.Fail()

	name := lxdbackend.ProfileName(sdkRef.ProjectId, sdkRef.Workshop, sdkRef.Sdk)
	description := fmt.Sprintf("%q SDK profile for %q workshop", sdkRef.Sdk, sdkRef.Workshop)
	lxdp := api.ProfilesPost{
		ProfilePut: api.ProfilePut{Description: description},
		Name:       name,
	}

	if err := conn.CreateProfile(lxdp); err != nil {
		return nil, err
	}
	rev.Add(func() {
		if reverr := conn.DeleteProfile(name); reverr != nil {
			logger.Noticef("On Setup: cannot remove %q SDK profile: %v", sdkRef.Sdk, reverr)
		}
	})

	iname := lxdbackend.InstanceName(sdkRef.Workshop, sdkRef.ProjectId)
	inst, etag, err := conn.GetInstance(iname)
	if err != nil {
		return nil, err
	}

	// Assigning the profile for the first time.
	inst.Profiles = append(inst.Profiles, name)
	op, err := conn.UpdateInstance(iname, inst.Writable(), etag)
	if err != nil {
		return nil, err
	}
	if err := op.WaitContext(ctx); err != nil {
		return nil, err
	}
	rev.Add(func() {
		inst, etag, reverr := conn.GetInstance(iname)
		if reverr != nil {
			logger.Noticef("On Setup: cannot unassign %q SDK profile: %v", sdkRef.Sdk, reverr)
			return
		}
		inst.Profiles = slices.DeleteFunc(inst.Profiles, func(n string) bool { return n == name })

		op, reverr := conn.UpdateInstance(iname, inst.Writable(), etag)
		if reverr != nil {
			logger.Noticef("On Setup: cannot unassign %q SDK profile: %v", sdkRef.Sdk, reverr)
			return
		}
		if reverr := op.WaitContext(ctx); reverr != nil {
			logger.Noticef("On Setup: cannot unassign %q SDK profile: %v", sdkRef.Sdk, reverr)
		}
	})

	clone := rev.Clone()
	rev.Success()
	return clone, nil
}

func setupProfile(conn lxd.InstanceServer, s *Specification, sdkRef sdk.Ref, prev workshop.SdkProfile) (*revert.Reverter, error) {
	fs, err := workshopFs(conn, sdkRef.ProjectId, sdkRef.Workshop)
	if err != nil {
		return nil, err
	}
	defer fs.Close()

	rev := revert.New()
	defer rev.Fail()

	if err := setupSshAgent(fs, prev.Agent, s.Profile.Agent); err != nil {
		return nil, err
	}
	rev.Add(func() {
		if reverr := setupSshAgent(fs, s.Profile.Agent, prev.Agent); reverr != nil {
			logger.Noticef("On setupProfile: cannot undo SSH agent changes: %v", reverr)
		}
	})

	if err := setupDesktop(fs, s.User, s.Environment, prev.Desktop, s.Profile.Desktop); err != nil {
		return nil, err
	}
	rev.Add(func() {
		if reverr := setupDesktop(fs, s.User, s.Environment, s.Profile.Desktop, prev.Desktop); reverr != nil {
			logger.Noticef("On setupProfile: cannot undo desktop interface changes: %v", reverr)
		}
	})

	if s.Profile.Gpu != nil {
		// LXD does not support AMD KFD passthrough,
		// so we need to create a device entry for it.
		maybeEnableAmdKfd(s)
	}

	// Setup mounts last so other interfaces can create directories to mount.
	r, err := setupMounts(conn, fs, s.User, sdkRef.ProjectId, sdkRef.Workshop, prev.Mounts, s.Profile.Mounts)
	if err != nil {
		return nil, err
	}
	revert.Copy(rev, r)

	clone := rev.Clone()
	rev.Success()
	return clone, nil
}

func cleanupProfile(conn lxd.InstanceServer, sdkRef sdk.Ref) error {
	prof, err := lxdbackend.Profile(conn, sdkRef.ProjectId, sdkRef.Workshop, sdkRef.Sdk)
	if err != nil {
		return err
	}

	fs, err := workshopFs(conn, sdkRef.ProjectId, sdkRef.Workshop)
	if err != nil {
		return err
	}
	defer fs.Close()

	err = removeMounts(conn, fs, sdkRef.ProjectId, sdkRef.Workshop, prof.Mounts)
	err2 := removeDesktop(fs, prof.Desktop)
	err3 := removeSshAgent(fs, prof.Agent)
	return cmp.Or(err, err2, err3)
}

func checkListenSocketPaths(devices map[string]map[string]string) error {
	for _, dev := range devices {
		if dev["type"] != "proxy" || dev["bind"] != "instance" {
			continue
		}
		listen := dev["listen"]
		if !strings.HasPrefix(listen, "unix:/") {
			continue
		}
		listen = strings.TrimPrefix(listen, "unix:")
		if strings.HasPrefix(listen, "/tmp/") || strings.HasPrefix(listen, dirs.WorkshopRunDir+"/") {
			continue
		}
		if listen == filepath.Join(dirs.XdgRuntimeDirBase, workshop.User.Uid, "wayland-0-inside-workshop") {
			continue
		}
		return fmt.Errorf("currently unsafe to create socket %q using LXD", listen)
	}
	return nil
}

// Setup creates mount profile specific to a given sdk.
func (b *Backend) Setup(ctx context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
	s, err := repo.SdkSpecification(ctx, b.Name(), sdkRef)
	if err != nil {
		return err
	}
	spec := s.(*Specification)

	if err := checkListenSocketPaths(spec.devices); err != nil {
		return err
	}

	conn, err := lxdbackend.ConnectLxd(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	name := lxdbackend.ProfileName(sdkRef.ProjectId, sdkRef.Workshop, sdkRef.Sdk)
	prev := workshop.NewSdkProfile(sdkRef.Sdk)

	// Load previous profile, if in use.
	lxdp, etag, err := lxdbackend.LxdProfile(conn, sdkRef.ProjectId, sdkRef.Workshop, sdkRef.Sdk)
	if err != nil {
		if !errors.Is(err, workshop.ErrSdkProfileNotFound) {
			return err
		}
		lxdp, etag = nil, ""
	} else if len(lxdp.UsedBy) == 0 {
		if err := conn.DeleteProfile(name); err != nil {
			return err
		}
		lxdp, etag = nil, ""
	} else {
		prev, err = lxdbackend.LxdToSdkProfile(sdkRef.Sdk, lxdp.Devices, lxdp.Config)
		if err != nil {
			return err
		}
	}

	rev, err := setupProfile(conn, spec, sdkRef, prev)
	if err != nil {
		return err
	}
	defer rev.Fail()

	if lxdp == nil {
		r, err := assignNewProfile(ctx, conn, sdkRef)
		if err != nil {
			return err
		}
		revert.Copy(rev, r)
	}

	prof := api.ProfilePut{
		Description: fmt.Sprintf("%q SDK profile for %q workshop", sdkRef.Sdk, sdkRef.Workshop),
		Config:      spec.config,
		Devices:     spec.devices,
	}
	if err := conn.UpdateProfile(name, prof, etag); err != nil {
		if lxdp == nil {
			return err
		}

		// By design, LXD does not roll back profile changes if the update failed,
		// so we have to do it ourselves.
		_, etag2, err2 := conn.GetProfile(name)
		if err2 != nil {
			logger.Noticef("cannot get updated profile: %v", err2)
			return err
		}
		if etag2 == etag {
			return err
		}

		if err2 := conn.UpdateProfile(name, lxdp.Writable(), etag2); err2 != nil {
			logger.Noticef("cannot restore original profile: %v", err2)
		}
		return err
	}

	rev.Success()
	return nil
}

// Remove removes profile of a given sdk.
//
// This method should be called after removing a sdk.
func (b *Backend) Remove(ctx context.Context, sdkRef sdk.Ref) error {
	conn, err := lxdbackend.ConnectLxd(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	// 1. Unassign the profile from the workshop
	iname := lxdbackend.InstanceName(sdkRef.Workshop, sdkRef.ProjectId)
	inst, etag, err := conn.GetInstance(iname)
	if err != nil {
		return err
	}

	lxdname := lxdbackend.ProfileName(sdkRef.ProjectId, sdkRef.Workshop, sdkRef.Sdk)
	if idx := slices.Index(inst.Profiles, lxdname); idx >= 0 {
		inst.Profiles = slices.Delete(inst.Profiles, idx, idx+1)

		op, err := conn.UpdateInstance(iname, inst.Writable(), etag)
		if err != nil {
			return err
		}
		if err = op.WaitContext(ctx); err != nil {
			return err
		}
	}

	err = cleanupProfile(conn, sdkRef)
	err2 := conn.DeleteProfile(lxdname)
	return cmp.Or(err, err2)
}

// NewSpecification returns a new mount specification.
func (b *Backend) NewSpecification(user string, sdk string) (interfaces.Specification, error) {
	return NewSpecification(user, sdk)
}

func MockWorkshopFs(f func(conn lxd.InstanceServer, pid, w string) (fsutil.Fs, error)) func() {
	old := workshopFs
	workshopFs = f
	return func() {
		workshopFs = old
	}
}
