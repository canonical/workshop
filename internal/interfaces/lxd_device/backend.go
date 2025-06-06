package lxd_device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/spf13/afero"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/x11"
)

const (
	LxdSock = "/var/snap/lxd/common/lxd/unix.socket"
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

func installMount(user *user.User, fs workshop.WorkshopFs, dev workshop.Mount) (reload bool, err error) {
	if dev.Type == workshop.WorkshopWorkshop {
		if _, err = fs.Stat(dev.What); err != nil {
			return false, fmt.Errorf(`stat workshop-source %q: %v`, dev.What, err)
		}

		if _, err = fs.Stat(dev.Where); err != nil {
			return false, fmt.Errorf(`stat workshop-target %q: %v`, dev.Where, err)
		}

		mounts, err := readMountProfile(fs)
		if err != nil {
			return false, err
		}

		check := func(me osutil.MountEntry) bool {
			return me.Name == dev.What && me.Dir == dev.Where && dev.ReadOnly == slices.Contains(me.Options, "ro")
		}
		if slices.ContainsFunc(mounts.Entries, check) {
			return false, nil
		}

		options := []string{"bind", "x-systemd.requires=/project"}
		if dev.ReadOnly {
			options = append(options, "ro")
		}

		entry := osutil.MountEntry{Name: dev.What, Dir: dev.Where, Type: "none", Options: options}
		mounts.Entries = append(mounts.Entries, entry)
		if err = writeMountProfile(fs, mounts); err != nil {
			return false, err
		}
		return true, nil
	}

	if dev.Type == workshop.HostWorkshop {
		// Ensure that the source path exists here. LXD allows to
		// require the source attribute when updating an instance
		// configuration but it would fail and still save changes to the
		// instace profile even if the source does not exist. For
		// Workshop that would mean that the interface connection would
		// fail but there will still be changes made to the instance
		// configuration which is not acceptable.
		// The dir is being dynamically created (no source attribute
		// provided by the slot).
		sourceExists, sourceIsDir, err := osutil.ExistsIsDir(dev.What)
		if err != nil {
			return false, err
		}

		// We cannot infer what the user intended to mount if the source doesn't
		// exist. In this case - inline with the above - we create a directory.
		if !sourceExists {
			uid, gid, err := osutil.UidGid(user)
			if err != nil {
				return false, err
			}

			if err = osutil.MkdirAllChown(dev.What, 0755, uid, gid); err != nil {
				return false, err
			}
		}

		if !sourceIsDir {
			return false, nil
		}

		_, err = fs.Stat(dev.Where)
		if !osutil.IsDirNotExist(err) {
			return false, err
		} else {
			// FIXME: workaround LXD empty directory issue (which, if the
			// connection was disconnected earlier, was removed by LXD).
			if err = fs.MkdirAll(dev.Where, os.ModePerm); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	return false, fmt.Errorf(`unknown device type: %v`, dev.Type)
}

func readMountProfile(fs workshop.WorkshopFs) (*osutil.MountProfile, error) {
	fstab, err := fs.Open("/etc/fstab")
	if errors.Is(err, os.ErrNotExist) {
		return &osutil.MountProfile{}, nil
	} else if err != nil {
		return nil, err
	}
	defer fstab.Close()

	return osutil.ReadMountProfile(fstab)
}

func writeMountProfile(fs workshop.WorkshopFs, mounts *osutil.MountProfile) error {
	return workshop.AtomicWrite(fs, "/etc/fstab", mounts, 0644)
}

func runMountCommand(conn lxd.InstanceServer, pid, w string, cmd []string) error {
	var out bytes.Buffer

	c := api.InstanceExecPost{
		Command:     cmd,
		Interactive: false,
	}

	args := lxd.InstanceExecArgs{Stderr: &out}

	op, err := conn.ExecInstance(lxdbackend.InstanceName(w, pid), c, &args)
	if err != nil {
		return err
	}

	if err = op.Wait(); err != nil {
		logger.Noticef("On workshop mount: %v (%s)", err, out.String())
	}
	return err
}

// 'systemd-fstab-generator' is responsible for creating mount entries from
// fstab. Because of this, we need to first ensure it runs (generating the
// on-demand unit files) by calling daemon-reload, and then activate the
// newly-creaed units by restarting a downstream target (ie. local-fs) see:
// https://www.freedesktop.org/software/systemd/man/latest/systemd.special.html
func reloadMounts(conn lxd.InstanceServer, pid, w string) error {
	return runMountCommand(conn, pid, w, []string{
		"/bin/bash",
		"-c",
		"systemctl daemon-reload && systemctl restart local-fs.target",
	})
}

func removeMount(conn lxd.InstanceServer, fs workshop.WorkshopFs, pid, w string, mnt workshop.Mount) error {
	if mnt.Type != workshop.WorkshopWorkshop {
		return nil
	}

	mounts, err := readMountProfile(fs)
	if err != nil {
		return err
	}

	cnt := len(mounts.Entries)
	deleter := func(me osutil.MountEntry) bool {
		return me.Name == mnt.What && me.Dir == mnt.Where
	}
	mounts.Entries = slices.DeleteFunc(mounts.Entries, deleter)
	if cnt == len(mounts.Entries) {
		return nil
	}

	if err = writeMountProfile(fs, mounts); err != nil {
		return err
	}

	return runMountCommand(conn, pid, w, []string{
		"umount",
		mnt.Where,
	})
}

func installSshAgent(fs workshop.WorkshopFs, dev workshop.SshAgent, workshop, sdkName string) error {
	script := fmt.Sprintf("%s_%s.sh", sdkName, dev.Name)
	env, err := fs.Create(filepath.Join("/etc", "profile.d", script))
	if err != nil {
		return fmt.Errorf("cannot set SSH_AUTH_SOCK for %q: %w", workshop, err)
	}
	defer env.Close()

	_, err = fmt.Fprintf(env, "export SSH_AUTH_SOCK=%s\n", dev.Listen.Address)
	if err != nil {
		return fmt.Errorf("cannot set SSH_AUTH_SOCK for %q: %w", workshop, err)
	}
	return nil
}

func removeSshAgent(fs workshop.WorkshopFs, dev workshop.SshAgent, sdkName string) error {
	script := fmt.Sprintf("%s_%s.sh", sdkName, dev.Name)
	return fs.Remove(filepath.Join("/etc", "profile.d", script))
}

func installDesktop(fs workshop.WorkshopFs, dev workshop.Desktop, user *user.User, env map[string]string, ws string) error {
	backend := env["XDG_BACKEND"]

	var envVars map[string]string
	envFile, err := fs.Create("/etc/profile.d/desktop.sh")
	if err != nil {
		return fmt.Errorf("cannot configure required environment for %q: %w", ws, err)
	}
	defer envFile.Close()

	// Use Wayland as the default backend in the case where it's unset
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

	for key, val := range envVars {
		_, err = envFile.WriteString("export " + key + "=" + val + "\n")
		if err != nil {
			return fmt.Errorf("cannot set %s for %q: %w", key, ws, err)
		}
	}

	return nil
}

func removeDesktop(fs workshop.WorkshopFs) error {
	if err := fs.Remove("/etc/profile.d/desktop.sh"); err != nil {
		return err
	}

	// The Xauth cookie may not always exist. Ignore any errors relating to this
	if err := fs.Remove("/tmp/.Xauthority"); err != nil {
		if !errors.Is(err, afero.ErrFileNotFound) {
			return err
		}
	}
	return nil
}

func sftpFs(conn lxd.InstanceServer, pid, w string) (workshop.WorkshopFs, error) {
	sftp, err := conn.GetInstanceFileSFTP(lxdbackend.InstanceName(w, pid))
	if err != nil {
		return nil, err
	}
	return workshop.NewWorkshopFs(sftp), nil
}

// Setup creates mount profile specific to a given sdk.
func (b *Backend) Setup(ctx context.Context, sdkInfo sdk.Ref, repo *interfaces.Repository) error {
	s, err := repo.SdkSpecification(ctx, b.Name(), sdkInfo)
	if err != nil {
		return err
	}
	spec := s.(*Specification)

	name := lxdbackend.ProfileName(sdkInfo.ProjectId, sdkInfo.Workshop, sdkInfo.Sdk)
	newp := api.ProfilePut{
		Devices:     spec.devices,
		Config:      spec.config,
		Description: fmt.Sprintf("%q SDK profile for %q workshop", sdkInfo.Sdk, sdkInfo.Workshop),
	}

	conn, err := lxdbackend.ConnectLxd(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	fs, err := workshopFs(conn, sdkInfo.ProjectId, sdkInfo.Workshop)
	if err != nil {
		return err
	}
	defer fs.Close()

	reload := false
	for _, mnt := range spec.Profile.Mounts {
		rld, err := installMount(spec.User, fs, mnt)
		if err != nil {
			return err
		}

		if rld {
			reload = true
		}
	}

	if reload {
		err = reloadMounts(conn, sdkInfo.ProjectId, sdkInfo.Workshop)
		if err != nil {
			return err
		}
	}

	if spec.Profile.Agent != nil {
		err = installSshAgent(fs, *spec.Profile.Agent, sdkInfo.Workshop, sdkInfo.Sdk)
		if err != nil {
			return err
		}
	}

	if spec.Profile.Desktop != nil {
		err = installDesktop(fs, *spec.Profile.Desktop, spec.User, spec.Environment, sdkInfo.Workshop)
		if err != nil {
			return err
		}
	}

	// Either create or update an existing LXD profile for the SDK so that later
	// it can be assigned to the required workshop.
	prevp, err := lxdbackend.Profile(conn, sdkInfo.ProjectId, sdkInfo.Workshop, sdkInfo.Sdk)
	if err == nil {
		// Find the difference between a set of old and new devices to detect if any
		// clean up is required when a new profile will be assigned (updated).
		for key, dev := range prevp.Mounts {
			if _, exist := spec.Profile.Mounts[key]; !exist {
				if err = removeMount(conn, fs, sdkInfo.ProjectId, sdkInfo.Workshop, dev); err != nil {
					return err
				}
			}
		}

		if prevp.Agent != nil && !prevp.Agent.Equal(spec.Profile.Agent) {
			if err = removeSshAgent(fs, *prevp.Agent, sdkInfo.Sdk); err != nil {
				return err
			}
		}

		if prevp.Desktop != nil && !prevp.Desktop.Equal(spec.Profile.Desktop) {
			if err = removeDesktop(fs); err != nil {
				return err
			}
		}

		return conn.UpdateProfile(name, newp, "")
	}

	if errors.Is(err, workshop.ErrSdkProfileNotFound) {
		if err = conn.CreateProfile(api.ProfilesPost{ProfilePut: newp, Name: name}); err != nil {
			return err
		}

		iname := lxdbackend.InstanceName(sdkInfo.Workshop, sdkInfo.ProjectId)
		inst, etag, err := conn.GetInstance(iname)
		if err != nil {
			return err
		}

		// Assigning the profile for the first time.
		inst.Profiles = append(inst.Profiles, name)
		op, err := conn.UpdateInstance(iname, inst.Writable(), etag)
		if err != nil {
			return err
		}

		return op.WaitContext(ctx)
	}

	return err
}

// Remove removes profile of a given sdk.
//
// This method should be called after removing a sdk.
func (b *Backend) Remove(ctx context.Context, w, profile string) error {
	conn, err := lxdbackend.ConnectLxd(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	fs, err := workshopFs(conn, projectId, w)
	if err != nil {
		return err
	}
	defer fs.Close()

	inst, etag, err := conn.GetInstance(lxdbackend.InstanceName(w, projectId))
	if err != nil {
		return err
	}

	prof, err := lxdbackend.Profile(conn, projectId, w, profile)
	if err != nil {
		return err
	}

	for _, dev := range prof.Mounts {
		if err = removeMount(conn, fs, projectId, w, dev); err != nil {
			return err
		}
	}

	if prof.Agent != nil {
		if err = removeSshAgent(fs, *prof.Agent, profile); err != nil {
			return err
		}
	}

	if prof.Desktop != nil {
		if err = removeDesktop(fs); err != nil {
			return err
		}
	}

	// 1. Unassign the profile from the workshop
	lxdname := lxdbackend.ProfileName(projectId, w, profile)
	if idx := slices.Index(inst.Profiles, lxdname); idx != -1 {
		inst.Profiles = slices.Delete(inst.Profiles, idx, idx+1)
		op, err := conn.UpdateInstance(lxdbackend.InstanceName(w, projectId), inst.Writable(), etag)
		if err != nil {
			return err
		}
		if err = op.WaitContext(ctx); err != nil {
			return err
		}
	}

	return conn.DeleteProfile(lxdname)
}

// NewSpecification returns a new mount specification.
func (b *Backend) NewSpecification(user string, pid, sdk string) (interfaces.Specification, error) {
	return NewSpecification(user, sdk)
}

func MockWorkshopFs(f func(conn lxd.InstanceServer, pid, w string) (workshop.WorkshopFs, error)) func() {
	old := workshopFs
	workshopFs = f
	return func() {
		workshopFs = old
	}
}
