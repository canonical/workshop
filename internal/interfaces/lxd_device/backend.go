package lxd_device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"slices"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/spf13/afero"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
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

func lxdClient(ctx context.Context) (lxd.InstanceServer, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	srv, err := lxd.ConnectLXDUnixWithContext(ctx, LxdSock, nil)
	if err != nil {
		return nil, err
	}

	return srv.UseProject("workshop." + user), nil
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

		check := func(me osutil.MountEntry) bool { return me.Name == dev.What && me.Dir == dev.Where }
		if slices.ContainsFunc(mounts.Entries, check) {
			return false, nil
		}

		entry := osutil.MountEntry{Name: dev.What, Dir: dev.Where, Type: "none", Options: []string{"bind", "x-systemd.requires=/project"}}
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

func reloadMounts(conn lxd.InstanceServer, pid, w string) error {
	return runMountCommand(conn, pid, w, []string{
		"mount",
		"-a",
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

func removeDesktop(fs workshop.WorkshopFs) error {
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
		return fmt.Errorf("cannot obtain device snippets for workshop %q: %w", sdkInfo.Workshop, err)
	}
	spec := s.(*Specification)

	name := lxdbackend.ProfileName(sdkInfo.ProjectId, sdkInfo.Workshop, sdkInfo.Sdk)
	newp := api.ProfilePut{
		Devices:     spec.devices,
		Config:      spec.config,
		Description: fmt.Sprintf("%q SDK profile for %q workshop", sdkInfo.Sdk, sdkInfo.Workshop),
	}

	conn, err := lxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	fs, err := workshopFs(conn, sdkInfo.ProjectId, sdkInfo.Workshop)
	if err != nil {
		return err
	}
	defer fs.Close()

	uname, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key user not found")
	}
	user, err := workshop.LookupUsername(uname)
	if err != nil {
		return err
	}

	reload := false
	for _, mnt := range spec.Profile.Mounts {
		if reload, err = installMount(user, fs, mnt); err != nil {
			return err
		}
	}
	if reload {
		err = reloadMounts(conn, sdkInfo.ProjectId, sdkInfo.Workshop)
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
		if prevp.Desktop != nil {
			if spec.Profile.Desktop == nil || *prevp.Desktop != *spec.Profile.Desktop {
				if err = removeDesktop(fs); err != nil {
					return err
				}
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
	conn, err := lxdClient(ctx)
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
func (b *Backend) NewSpecification(user *user.User, pid, sdk string) interfaces.Specification {
	return NewSpecification(user, sdk)
}

func MockWorkshopFs(f func(conn lxd.InstanceServer, pid, w string) (workshop.WorkshopFs, error)) func() {
	old := workshopFs
	workshopFs = f
	return func() {
		workshopFs = old
	}
}
