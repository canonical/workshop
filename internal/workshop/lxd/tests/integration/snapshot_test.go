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

package lxdbackend_integration_test

import (
	"cmp"
	"context"
	"crypto/sha3"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/canonical/lxd/shared/api"
	"golang.org/x/sys/unix"
	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/workshop/lxd/tests/helper"
)

type snapshotSuite struct {
	usr     *user.User
	project workshop.Project
	ctx     context.Context

	restoreLookupUsr func()
	restoreUserEnv   func()

	bd *lxdbackend.Backend
}

var _ = check.Suite(&snapshotSuite{})

func (s *snapshotSuite) SetUpSuite(c *check.C) {
	s.usr = &user.User{Username: "testuser", Uid: "1000", Gid: "1000", HomeDir: c.MkDir()}
	s.project = workshop.Project{
		ProjectId: "42424242",
		Path:      c.MkDir(),
	}
	s.ctx = helper.CreateTestContext(s.usr.Username, s.project.ProjectId)

	s.restoreLookupUsr = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return s.usr, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

	dirs.SetRootDir(c.MkDir())
	dirs.SocketPath = filepath.Join(dirs.DataDir, "workshop.socket")
	c.Assert(dirs.CreateDirs(), check.IsNil)

	var err error
	s.bd, err = lxdbackend.New()
	c.Assert(err, check.IsNil)

	conn, err := s.bd.LxdClient(s.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()
}

func (s *snapshotSuite) TearDownSuite(c *check.C) {
	conn, err := s.bd.LxdClient(s.ctx)
	c.Check(err, check.IsNil)
	defer conn.Disconnect()

	helper.CleanupLxdProject(c, conn, "workshop."+s.usr.Username)
	helper.CleanupLxdProject(c, conn, "workshop-snapshots."+s.usr.Username)

	s.restoreLookupUsr()
	s.restoreUserEnv()
}

// This suite deliberately doesn't override the default devices, so the test
// snapshots match the real ones more closely. As a consequence we have to do a
// bit of extra work to launch a workshop.
func (s *snapshotSuite) launchWorkshop(c *check.C, file *workshop.File, snapshot workshop.Snapshot) func() {
	err := os.MkdirAll(workshop.AptCacheDir(s.project.ProjectId, file.Name), 0755)
	c.Assert(err, check.IsNil)

	err = s.bd.LaunchOrRebuildWorkshop(s.ctx, file, snapshot)
	c.Assert(err, check.IsNil)

	return func() {
		reverr := s.bd.RemoveWorkshop(s.ctx, file.Name)
		c.Check(reverr, check.IsNil)
	}
}

//go:embed snapshot-format.yaml
var snapshotFormat []byte

// Attempt to specify the filesystem layout of a snapshot. Changes to this may
// invalidate snapshots of existing workshops, so the snapshot format revision
// number should be bumped to force a full refresh. This test is mainly
// concerned with workshop config and devices. While these aren't generally
// copied to snapshots, they can influence the filesystem before the snapshot
// is taken (e.g. cloud-config). Direct changes to the filesystem and other
// backend-agnostic conventions are covered by `apiSuite.TestSnapshotFormat`.
func (s *snapshotSuite) TestLxdBackendSnapshotFormat(c *check.C) {
	var format map[string]any
	err := yaml.Unmarshal(snapshotFormat, &format)
	c.Assert(err, check.IsNil)

	// Launch workshop.
	image, err := s.bd.GetBase(s.ctx, "ubuntu@24.04", workshop.ConfinementContainer)
	c.Assert(err, check.IsNil)
	err = s.bd.DownloadBase(s.ctx, image, nil)
	c.Assert(err, check.IsNil)
	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@24.04",
		Sdks: []workshop.SdkRecord{
			{Name: "store-sdk", Channel: "latest/stable"},
			{Name: "local-sdk", Source: sdk.ProjectSource},
		},
	}
	snapshot := workshop.BaseOnly(s.bd.FormatRevision(), image.Name, workshop.ConfinementContainer, image.Fingerprint)

	remove := s.launchWorkshop(c, wf, snapshot)
	defer remove()

	// Validate post-launch metadata.
	launched := s.workshopFormat(c, wf, snapshot)
	c.Check(launched, testutil.JsonEquals, format["launched"])

	// Start workshop.
	err = s.bd.StartWorkshop(s.ctx, wf.Name)
	c.Assert(err, check.IsNil)
	defer func() {
		reverr := s.bd.StopWorkshop(s.ctx, wf.Name, true)
		c.Check(reverr, check.IsNil)
	}()

	// Validate post-start metadata.
	started := s.workshopFormat(c, wf, snapshot)
	c.Check(started, testutil.JsonEquals, format["started"])

	// Install Store SDK.
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "store-sdk",
			PackageID: "7MW8x1TQWSOXMR6t8kvcsLiYJomy4eSz",
			Channel:   "latest/stable",
			Revision:  sdk.R(23),
			Sha3_384:  "18b8ce233667942e94e1f5bdd22bcd516c4a375030a359a5bb09220b416d215fffda138d8d45eaab419ae2403c81ec5d",
		},
		SdkYAML: `name: store-sdk
`,
	}
	helper.MockSdkVolume(c, s.ctx, s.bd, meta)
	defer func() { _ = s.bd.DeleteSdk(s.ctx, meta.Setup) }()
	err = s.bd.InstallSdk(s.ctx, wf.Name, meta.Setup)
	c.Assert(err, check.IsNil)
	defer func() { _ = s.bd.UninstallSdk(s.ctx, wf.Name, meta.Name) }()

	// Validate post-install metadata.
	snapshot.Sdks = append(snapshot.Sdks, sdk.SetupContentID(meta.Setup))
	sdkAttached := s.workshopFormat(c, wf, snapshot)
	c.Check(sdkAttached, testutil.JsonEquals, format["sdk-attached"])

	// Install in-project SDK.
	setup2 := sdk.Setup{
		Name:     "local-sdk",
		Source:   sdk.ProjectSource,
		Revision: sdk.R(-34),
		Sha3_384: "dc00101dfd688cdc058e31d3b82e680df123f85935f741fabcb8f0dfd29d80612f131db8487621abad7ee856223bede1",
	}
	userDataDir := workshop.UserDataRootDir(s.usr.HomeDir, nil)
	sdkDir := workshop.LocalSdkDir(userDataDir, s.project.ProjectId, wf.Name, setup2.Name)
	err = os.MkdirAll(filepath.Join(sdkDir, setup2.Sha3_384), 0755)
	c.Assert(err, check.IsNil)
	err = s.bd.InstallSdk(s.ctx, wf.Name, setup2)
	c.Assert(err, check.IsNil)

	// Validate post-install metadata.
	snapshot.Sdks = append(snapshot.Sdks, sdk.SetupContentID(setup2))
	sdkMounted := s.workshopFormat(c, wf, snapshot)
	c.Check(sdkMounted, testutil.JsonEquals, format["sdk-mounted"])

	// Snapshot workshop.
	err = s.bd.TakeSnapshot(s.ctx, wf.Name, snapshot)
	c.Assert(err, check.IsNil)
	defer func() { _ = s.bd.RemoveSnapshot(s.ctx, snapshot) }()

	// Validate snapshot metadata.
	sdkSnapshot := s.snapshotFormat(c, snapshot)

	c.Check(sdkSnapshot, testutil.JsonEquals, format["snapshot"])
}

func (s *snapshotSuite) workshopFormat(c *check.C, file *workshop.File, snapshot workshop.Snapshot) api.InstancePut {
	conn, err := s.bd.LxdClient(s.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	name := lxdbackend.InstanceName(file.Name, s.project.ProjectId)
	inst := fullInstance(c, conn, name).Writable()

	// Remove architecture to make the test hardware-agnostic. It already
	// affects the base image fingerprint so we don't need to worry about
	// it too much.
	inst.Architecture = ""

	// Remove config options which aren't constant.
	for k := range inst.Config {
		if strings.HasPrefix(k, "volatile.") || strings.HasPrefix(k, "image.") {
			delete(inst.Config, k)
		}
	}
	c.Check(inst.Config["user.workshop.base-fingerprint"], check.Equals, snapshot.Image.Fingerprint)
	delete(inst.Config, "user.workshop.base-fingerprint")

	// Marshalling might be nondeterministic.
	var wf workshop.File
	c.Assert(yaml.Unmarshal([]byte(inst.Config["user.workshop.file"]), &wf), check.IsNil)
	c.Check(&wf, check.DeepEquals, file)
	delete(inst.Config, "user.workshop.file")

	// Avoid having to update the saved configs when bumping the revision.
	c.Check(inst.Config["user.workshop.format-revision"], check.Equals, snapshot.Format.String())
	delete(inst.Config, "user.workshop.format-revision")

	// This one is a bit long, replace with hash for readability.
	digest := sha3.Sum384([]byte(inst.Config["cloud-init.user-data"]))
	inst.Config["cloud-init.user-data"] = hex.EncodeToString(digest[:])

	// Host paths of default devices can change without affecting the
	// workshop, so we exclude them from the hash. Other device options
	// should be included in case they influence the rootfs.
	delete(inst.Devices["workshop.bin"], "source")
	delete(inst.Devices["cache.apt"], "source")
	delete(inst.Devices["workshop.socket"], "connect")

	for _, sk := range snapshot.Sdks {
		device := inst.Devices[workshop.SdkDeviceName(sk.Name)]
		var installedAt time.Time
		c.Assert(installedAt.UnmarshalText([]byte(device["user.sdk.installed-at"])), check.IsNil)
		c.Check(installedAt.IsZero(), check.Equals, false)
		delete(device, "user.sdk.installed-at")

		if _, ok := device["pool"]; !ok {
			delete(device, "source")
		}
	}

	return inst
}

func (s *snapshotSuite) snapshotFormat(c *check.C, snapshot workshop.Snapshot) api.InstancePut {
	conn, err := s.bd.LxdClient(s.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()
	snapshotConn := conn.UseProject("workshop-snapshots." + s.usr.Username)

	sk := snapshot.Sdks[len(snapshot.Sdks)-1].Name
	digest, err := s.bd.HashSnapshot(snapshot)
	c.Assert(err, check.IsNil)
	snapshotName := sk + "-" + digest[:16]
	inst := fullInstance(c, snapshotConn, snapshotName).Writable()

	// Remove architecture to make the test hardware-agnostic. It already
	// affects the base image fingerprint so we don't need to worry about
	// it too much.
	inst.Architecture = ""

	// Remove config options which aren't constant.
	for k := range inst.Config {
		if strings.HasPrefix(k, "volatile.") || strings.HasPrefix(k, "image.") {
			delete(inst.Config, k)
		}
	}
	c.Check(inst.Config["user.workshop.base-fingerprint"], check.Equals, snapshot.Image.Fingerprint)
	delete(inst.Config, "user.workshop.base-fingerprint")

	// Avoid having to update the saved configs when bumping the revision.
	c.Check(inst.Config["user.workshop.format-revision"], check.Equals, snapshot.Format.String())
	delete(inst.Config, "user.workshop.format-revision")
	c.Check(inst.Config["user.workshop.sha3-384"], check.Equals, digest)
	delete(inst.Config, "user.workshop.sha3-384")

	return inst
}

// Launches 2 workshops from scratch and another from a snapshot of the first,
// then checks that the third workshop is indistinguishable from the other two.
func (s *snapshotSuite) TestLxdBackendSnapshotDiff(c *check.C) {
	if os.Geteuid() != 0 {
		c.Skip("requires root to mount and compare workshop filesystems")
	}

	for _, confinement := range []workshop.Confinement{workshop.ConfinementContainer, workshop.ConfinementVirtualMachine} {
		kind, err := confinement.MarshalText()
		c.Assert(err, check.IsNil)
		for _, base := range workshop.SupportedBases {
			c.Logf("Testing snapshot integrity for %s %ss", base, kind)
			s.snapshotDiff(c, base, confinement)
		}
	}
}

func (s *snapshotSuite) snapshotDiff(c *check.C, base string, confinement workshop.Confinement) {
	// Download base image.
	image, err := s.bd.GetBase(s.ctx, base, confinement)
	c.Assert(err, check.IsNil)
	err = s.bd.DownloadBase(s.ctx, image, nil)
	c.Assert(err, check.IsNil)

	// Launch original workshop.
	originFile := &workshop.File{
		Name:        "origin",
		Base:        base,
		Confinement: confinement,
	}
	baseOnly := workshop.BaseOnly(s.bd.FormatRevision(), image.Name, confinement, image.Fingerprint)
	remove := s.launchWorkshop(c, originFile, baseOnly)
	defer remove()

	// Start original workshop and take a snapshot.
	err = s.bd.StartWorkshop(s.ctx, "origin")
	c.Assert(err, check.IsNil)
	originSnapshot := baseOnly
	originSnapshot.Sdks = []sdk.ContentID{{
		Name:     "system",
		Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
		IsVolume: true,
	}}
	err1 := s.bd.TakeSnapshot(s.ctx, "origin", originSnapshot)
	originRootFS, err2 := s.workshopRootFS("origin", confinement)
	err3 := s.bd.StopWorkshop(s.ctx, "origin", true)
	c.Assert(cmp.Or(err1, err2, err3), check.IsNil)

	// Launch a completely independent workshop.
	siblingFile := &workshop.File{
		Name:        "sibling",
		Base:        base,
		Confinement: confinement,
	}
	remove = s.launchWorkshop(c, siblingFile, baseOnly)
	defer remove()

	// Start independent workshop to run cloud-init.
	err = s.bd.StartWorkshop(s.ctx, "sibling")
	c.Assert(err, check.IsNil)
	siblingRootFS, err1 := s.workshopRootFS("sibling", confinement)
	err2 = s.bd.StopWorkshop(s.ctx, "sibling", true)
	c.Assert(cmp.Or(err1, err2), check.IsNil)

	// Launch clone of the first workshop.
	cloneFile := &workshop.File{
		Name:        "clone",
		Base:        base,
		Confinement: confinement,
	}
	remove = s.launchWorkshop(c, cloneFile, originSnapshot)
	defer remove()

	// Start cloned workshop and take another snapshot.
	err = s.bd.StartWorkshop(s.ctx, "clone")
	c.Assert(err, check.IsNil)
	cloneSnapshot := originSnapshot
	cloneSnapshot.Sdks = append(cloneSnapshot.Sdks, sdk.ContentID{
		Name:     "test-sdk",
		Sha3_384: "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
		IsVolume: false,
	})
	err1 = s.bd.TakeSnapshot(s.ctx, "clone", cloneSnapshot)
	cloneRootFS, err2 := s.workshopRootFS("clone", confinement)
	err3 = s.bd.StopWorkshop(s.ctx, "clone", true)
	c.Assert(cmp.Or(err1, err2, err3), check.IsNil)

	// Launch another independent workshop.
	wf := &workshop.File{
		Name:        "test",
		Base:        base,
		Confinement: confinement,
	}
	remove = s.launchWorkshop(c, wf, baseOnly)
	defer remove()
	err = s.bd.StartWorkshop(s.ctx, "test")
	c.Assert(err, check.IsNil)
	defer func() {
		err1 := s.bd.StopWorkshop(s.ctx, "test", true)
		c.Check(err1, check.IsNil)
	}()

	// Mount the other workshops inside the new one.
	unmountOrigin := s.mountRootFS(c, originRootFS, "test", "/mnt/origin")
	defer unmountOrigin.Fail()
	unmountSibling := s.mountRootFS(c, siblingRootFS, "test", "/mnt/sibling")
	defer unmountSibling.Fail()
	unmountClone := s.mountRootFS(c, cloneRootFS, "test", "/mnt/clone")
	defer unmountClone.Fail()

	// Ensure ID files are unique, while most others are identical.
	originFiles := s.extractUniqueFiles(c, "test", "/mnt/origin")
	siblingFiles := s.extractUniqueFiles(c, "test", "/mnt/sibling")
	cloneFiles := s.extractUniqueFiles(c, "test", "/mnt/clone")

	c.Check(originFiles.hostname, check.Not(check.Equals), siblingFiles.hostname)
	c.Check(originFiles.machineID, check.Not(check.Equals), siblingFiles.machineID)
	c.Check(originFiles.networkCfg, check.Not(check.Equals), siblingFiles.networkCfg)
	c.Check(originFiles.sshKey, check.Not(check.Equals), siblingFiles.sshKey)

	c.Check(originFiles.hostname, check.Not(check.Equals), cloneFiles.hostname)
	if confinement == workshop.ConfinementContainer {
		c.Check(originFiles.machineID, check.Not(check.Equals), cloneFiles.machineID)
	} else {
		// TODO: fix /etc/machine-id in VMs.
		c.Check(originFiles.machineID, check.Equals, cloneFiles.machineID)
	}
	c.Check(originFiles.networkCfg, check.Not(check.Equals), cloneFiles.networkCfg)
	c.Check(originFiles.sshKey, check.Not(check.Equals), cloneFiles.sshKey)

	s.execDiff(c, "test", "/mnt/origin", "/mnt/sibling")
	s.execDiff(c, "test", "/mnt/origin", "/mnt/clone")

	names := []string{"origin", "clone"}
	for i, snapshot := range []workshop.Snapshot{originSnapshot, cloneSnapshot} {
		c.Logf("Restoring snapshot of %q workshop", names[i])

		// Restore original workshop from snapshot.
		clone := unmountOrigin.Clone()
		unmountOrigin.Success()
		clone.Fail()
		_ = s.launchWorkshop(c, originFile, snapshot)

		// Restart it to give services a chance to run.
		err = s.bd.StartWorkshop(s.ctx, "origin")
		c.Assert(err, check.IsNil)
		originRootFS, err1 := s.workshopRootFS("origin", confinement)
		err2 = s.bd.StopWorkshop(s.ctx, "origin", true)
		c.Assert(cmp.Or(err1, err2), check.IsNil)

		// Remount the rootfs into the "test" workshop.
		unmountOrigin = s.mountRootFS(c, originRootFS, "test", "/mnt/origin")
		defer unmountOrigin.Fail()

		// Check ID files, and most others, are preserved.
		restoredFiles := s.extractUniqueFiles(c, "test", "/mnt/origin")

		c.Check(restoredFiles.hostname, check.Equals, originFiles.hostname)
		if confinement == workshop.ConfinementContainer || i == 0 {
			c.Check(restoredFiles.machineID, check.Equals, originFiles.machineID)
		} else {
			// TODO: fix /etc/machine-id in VMs.
			c.Check(restoredFiles.machineID, check.Equals, cloneFiles.machineID)
		}
		c.Check(restoredFiles.networkCfg, check.Equals, originFiles.networkCfg)
		c.Check(restoredFiles.sshKey, check.Equals, originFiles.sshKey)

		s.execDiff(c, "test", "/mnt/sibling", "/mnt/origin")
	}
}

type filesystem struct {
	name        string
	confinement workshop.Confinement

	Fstype string `json:"fstype"`
	Source string `json:"source"`
	Fsroot string `json:"fsroot"`
}

// workshopRootFS performs operations while the workshop is running to make it
// easy to mount its rootfs elsewhere once it has stopped. For containers, it
// returns the source ZFS dataset, and the subdirectory of the rootfs within
// that. For VMs, it relabels the filesystem to make it easier to locate when
// mounting the parent block device in another VM.
func (s *snapshotSuite) workshopRootFS(name string, confinement workshop.Confinement) (filesystem, error) {
	args := workshop.ExecArgs{
		Command: []string{"findmnt", "--json", "--mountpoint=/", "--nofsroot", "--output=fsroot,fstype,source"},
		WorkDir: "/",
		Timeout: time.Second,
	}
	output, err := helper.ExecOutput(s.ctx, s.bd, name, args)
	if err != nil {
		return filesystem{}, err
	}

	var filesystems struct {
		Filesystems []filesystem `json:"filesystems"`
	}
	if err := json.Unmarshal([]byte(output), &filesystems); err != nil {
		return filesystem{}, err
	}
	if len(filesystems.Filesystems) != 1 {
		return filesystem{}, fmt.Errorf("expected 1 filesystem, found:\n%s", output)
	}

	rootfs := filesystems.Filesystems[0]
	rootfs.name = name
	rootfs.confinement = confinement
	if confinement == workshop.ConfinementContainer {
		return rootfs, nil
	}

	if rootfs.Fstype != "ext4" {
		return filesystem{}, fmt.Errorf("unexpected rootfs type %q", rootfs.Fstype)
	}
	args.Command = []string{"e2label", "/dev/disk/by-label/cloudimg-rootfs", "workshop-" + name}
	_, err = helper.ExecOutput(s.ctx, s.bd, name, args)
	if err != nil {
		return filesystem{}, err
	}

	rootfs.Source = "/dev/disk/by-label/workshop-" + name
	return rootfs, nil
}

func (s *snapshotSuite) mountRootFS(c *check.C, source filesystem, name, path string) *revert.Reverter {
	if source.confinement == workshop.ConfinementContainer {
		return s.mountContainerRootFS(c, source, name, path)
	}
	return s.mountVMRootFS(c, source, name, path)
}

// mountContainerRootFS mounts the rootfs of one container inside another
// container. LXD doesn't support this directly, so we first mount the rootfs
// on the host and then bind-mount the mountpoint into the other workshop. The
// host mount is ID-mapped so the container can modify the files.
func (s *snapshotSuite) mountContainerRootFS(c *check.C, source filesystem, name, path string) *revert.Reverter {
	c.Assert(source.Fstype, check.Equals, "zfs")

	userns := s.workshopUserNS(c, name)
	defer userns.Close()

	rev := revert.New()
	defer rev.Fail()

	mountpoint := c.MkDir()
	s.idmappedMount(c, source, mountpoint, userns)
	rev.Add(func() {
		err1 := unix.Unmount(mountpoint, 0)
		c.Check(err1, check.IsNil)
	})

	mount := workshop.Mount{
		Name:      "root_" + source.name,
		Type:      workshop.HostWorkshop,
		What:      filepath.Join(mountpoint, source.Fsroot),
		Where:     path,
		MakeWhere: true,
	}
	err := s.bd.AddWorkshopMount(s.ctx, name, mount)
	c.Assert(err, check.IsNil)
	rev.Add(func() {
		err1 := s.bd.RemoveWorkshopMount(s.ctx, name, mount.Name)
		c.Check(err1, check.IsNil)
	})

	clone := rev.Clone()
	rev.Success()
	return clone
}

func (s *snapshotSuite) workshopUserNS(c *check.C, name string) *os.File {
	conn, err := s.bd.LxdClient(s.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	state, _, err := conn.GetInstanceState(lxdbackend.InstanceName(name, s.project.ProjectId))
	c.Assert(err, check.IsNil)
	c.Assert(state.Pid > 0, check.Equals, true)

	userns, err := os.Open(filepath.Join("/proc", fmt.Sprint(state.Pid), "ns", "user"))
	c.Assert(err, check.IsNil)
	return userns
}

func (s *snapshotSuite) idmappedMount(c *check.C, source filesystem, target string, userns *os.File) {
	fsfd, err := unix.Fsopen(source.Fstype, unix.FSOPEN_CLOEXEC)
	c.Assert(err, check.IsNil)
	defer unix.Close(fsfd)

	err = unix.FsconfigSetString(fsfd, "source", source.Source)
	c.Assert(err, check.IsNil)
	err = unix.FsconfigCreate(fsfd)
	c.Assert(err, check.IsNil)

	tree, err := unix.Fsmount(fsfd, unix.FSMOUNT_CLOEXEC, 0)
	c.Assert(err, check.IsNil)
	defer unix.Close(tree)

	attr := &unix.MountAttr{
		Attr_set:  unix.MOUNT_ATTR_IDMAP,
		Userns_fd: uint64(userns.Fd()),
	}
	err = unix.MountSetattr(tree, "", unix.AT_EMPTY_PATH, attr)
	c.Assert(err, check.IsNil)

	err = unix.MoveMount(tree, "", unix.AT_FDCWD, target, unix.MOVE_MOUNT_F_EMPTY_PATH)
	c.Assert(err, check.IsNil)
}

// mountVMRootFS mounts the rootfs of one VM inside another VM. LXD supports
// this directly, but only creates the block device without mounting it.
// Luckily we already relabeled the rootfs, so we can use that to mount it
// after udev has created the appropriate symlinks.
func (s *snapshotSuite) mountVMRootFS(c *check.C, source filesystem, name, path string) *revert.Reverter {
	conn, err := s.bd.LxdClient(s.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	inst, etag1, err := conn.GetInstance(lxdbackend.InstanceName(name, s.project.ProjectId))
	c.Assert(err, check.IsNil)

	vol, etag2, err := conn.GetStoragePoolVolume(inst.Devices["root"]["pool"], inst.Type, lxdbackend.InstanceName(source.name, s.project.ProjectId))
	c.Assert(err, check.IsNil)
	vol.Config["security.shared"] = "true"
	op, err := conn.UpdateStoragePoolVolume(vol.Pool, vol.Type, vol.Name, vol.Writable(), etag2)
	c.Assert(err, check.IsNil)
	c.Assert(op.WaitContext(s.ctx), check.IsNil)

	rev := revert.New()
	defer rev.Fail()

	inst.Devices["root_"+source.name] = map[string]string{
		"type":        "disk",
		"pool":        vol.Pool,
		"source":      vol.Name,
		"source.type": vol.Type,
	}
	op, err = conn.UpdateInstance(inst.Name, inst.Writable(), etag1)
	c.Assert(err, check.IsNil)
	c.Assert(op.WaitContext(s.ctx), check.IsNil)

	rev.Add(func() {
		inst1, etag3, err1 := conn.GetInstance(inst.Name)
		if c.Check(err1, check.IsNil) {
			delete(inst1.Devices, "root_"+source.name)

			op1, err1 := conn.UpdateInstance(inst1.Name, inst1.Writable(), etag3)
			if c.Check(err1, check.IsNil) {
				c.Check(op1.WaitContext(s.ctx), check.IsNil)
			}
		}
	})

	args := workshop.ExecArgs{
		Command: []string{"udevadm", "settle"},
		WorkDir: "/",
		Timeout: time.Second,
	}
	_, err = helper.ExecOutput(s.ctx, s.bd, name, args)
	c.Assert(err, check.IsNil)

	// Check filesystem integrity; nonzero exit codes can indicate the rootfs
	// was repaired successfully, but could be a sign that the filesystem
	// wasn't properly frozen or the workshop wasn't stopped cleanly.
	args.Command = []string{"fsck.ext4", "-fy", source.Source}
	out, err := helper.ExecOutput(s.ctx, s.bd, name, args)
	c.Check(err, check.IsNil, check.Commentf("%s", out))

	args.Command = []string{"mkdir", "-p", path}
	_, err = helper.ExecOutput(s.ctx, s.bd, name, args)
	c.Assert(err, check.IsNil)

	args.Command = []string{"mount", source.Source, path}
	_, err = helper.ExecOutput(s.ctx, s.bd, name, args)
	c.Assert(err, check.IsNil)
	rev.Add(func() {
		args1 := workshop.ExecArgs{
			Command: []string{"umount", path},
			WorkDir: "/",
			Timeout: time.Second,
		}
		_, err1 := helper.ExecOutput(s.ctx, s.bd, name, args1)
		c.Check(err1, check.IsNil)
	})

	clone := rev.Clone()
	rev.Success()
	return clone
}

type uniqueFiles struct {
	hostname   string
	machineID  string
	networkCfg string
	sshKey     string
}

// extractUniqueFiles prepares a rootfs for diff comparison. It removes files
// that are likely be different (most of which are inconsequential) and returns
// the contents of the files that really ought to be different.
func (s *snapshotSuite) extractUniqueFiles(c *check.C, name, path string) uniqueFiles {
	fs, err := s.bd.WorkshopFs(s.ctx, name)
	c.Assert(err, check.IsNil)
	defer fs.Close()

	hostname, err := fs.ReadFile(filepath.Join(path, "etc", "hostname"))
	c.Assert(err, check.IsNil)
	machineID, err := fs.ReadFile(filepath.Join(path, "etc", "machine-id"))
	c.Assert(err, check.IsNil)
	networkCfg, err := fs.ReadFile(filepath.Join(path, "etc", "systemd", "network", "10-cloud-init-eth0.network.d", "workshop.conf"))
	c.Assert(err, check.IsNil)
	sshKey, err := fs.ReadFile(filepath.Join(path, "etc", "ssh", "ssh_host_ed25519_key.pub"))
	c.Assert(err, check.IsNil)

	files := []string{
		"etc/hostname",
		"etc/machine-id",
		"etc/ssh/ssh_host_ed25519_key",
		"etc/ssh/ssh_host_ed25519_key.pub",
		"etc/ssh/ssh_host_ed25519_key-cert.pub",
		"etc/sudoers.d/90-cloud-init-users",
		"etc/systemd/network/10-cloud-init-eth0.network.d/workshop.conf",
		"var/cache/ldconfig/aux-cache",
		"var/lib/systemd/random-seed",
		"var/lib/workshop/run/workshop.socket.untrusted",
	}
	for _, file := range files {
		local, err := filepath.Localize(file)
		c.Assert(err, check.IsNil)

		err = fs.Remove(filepath.Join(path, local))
		if !errors.Is(err, os.ErrNotExist) {
			c.Assert(err, check.IsNil)
		}
	}

	dirs := []string{
		"tmp",
		"var/cache/apparmor",
		"var/cache/snapd",
		"var/lib/cloud",
		"var/lib/snapd",
		"var/log",
		"var/snap/lxd/common",
		"var/tmp",
	}
	for _, dir := range dirs {
		local, err := filepath.Localize(dir)
		c.Assert(err, check.IsNil)

		err = fs.RemoveAll(filepath.Join(path, local))
		c.Assert(err, check.IsNil)
	}

	return uniqueFiles{
		hostname:   string(hostname),
		machineID:  string(machineID),
		networkCfg: string(networkCfg),
		sshKey:     string(sshKey),
	}
}

func (s *snapshotSuite) execDiff(c *check.C, name, a, b string) {
	args := workshop.ExecArgs{
		Command: []string{"diff", "--brief", "--no-dereference", "--recursive", a, b},
		WorkDir: "/",
		Timeout: time.Second,
	}
	out, err := helper.ExecOutput(s.ctx, s.bd, name, args)
	c.Check(err, check.IsNil, check.Commentf("diff %s %s:\n%s", a, b, out))
}
