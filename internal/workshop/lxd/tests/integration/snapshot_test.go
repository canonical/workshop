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
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
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

	restoreLookupUsr   func()
	restoreUserEnv     func()
	restoreImageServer func()

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
	s.restoreImageServer = lxdbackend.FakeImageServer(helper.MinimalImageServer)

	dirs.SetRootDir(c.MkDir())
	dirs.WorkshopCtlPath = filepath.Join(c.MkDir(), "workshopctl")
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
	s.restoreImageServer()
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
	image, err := s.bd.GetBase(s.ctx, "ubuntu@24.04")
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
	snapshot := workshop.BaseOnly(s.bd.FormatRevision(), image.Name, image.Fingerprint)

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

	for _, base := range workshop.SupportedBases {
		c.Logf("Testing snapshot integrity for base %q", base)
		s.snapshotDiff(c, base)
	}
}

func (s *snapshotSuite) snapshotDiff(c *check.C, base string) {
	// Download base image.
	image, err := s.bd.GetBase(s.ctx, base)
	c.Assert(err, check.IsNil)
	err = s.bd.DownloadBase(s.ctx, image, nil)
	c.Assert(err, check.IsNil)

	// Launch first workshop.
	wf1 := &workshop.File{
		Name: "test1",
		Base: base,
	}
	baseOnly := workshop.BaseOnly(s.bd.FormatRevision(), image.Name, image.Fingerprint)
	remove := s.launchWorkshop(c, wf1, baseOnly)
	defer remove()

	// Start first workshop to take snapshot.
	err = s.bd.StartWorkshop(s.ctx, "test1")
	c.Assert(err, check.IsNil)
	snapshot := baseOnly
	snapshot.Sdks = []sdk.ContentID{{
		Name:     "system",
		Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
		IsVolume: true,
	}}
	err1 := s.bd.TakeSnapshot(s.ctx, "test1", snapshot)
	mount1, err2 := s.rootfsMount("test1")
	err3 := s.bd.StopWorkshop(s.ctx, "test1", true)
	c.Assert(cmp.Or(err1, err2, err3), check.IsNil)

	// Launch second workshop.
	wf2 := &workshop.File{
		Name: "test2",
		Base: base,
	}
	remove = s.launchWorkshop(c, wf2, baseOnly)
	defer remove()

	// Start second workshop to run cloud-init.
	err = s.bd.StartWorkshop(s.ctx, "test2")
	c.Assert(err, check.IsNil)
	mount2, err1 := s.rootfsMount("test2")
	err2 = s.bd.StopWorkshop(s.ctx, "test2", true)
	c.Assert(cmp.Or(err1, err2), check.IsNil)

	// Launch third workshop from snapshot.
	wf3 := &workshop.File{
		Name: "test3",
		Base: base,
	}
	remove = s.launchWorkshop(c, wf3, snapshot)
	defer remove()

	// Start third workshop to run cloud-init (again).
	err = s.bd.StartWorkshop(s.ctx, "test3")
	c.Assert(err, check.IsNil)
	snapshot2 := snapshot
	snapshot2.Sdks = append(snapshot2.Sdks, sdk.ContentID{
		Name:     "test-sdk",
		Sha3_384: "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
		IsVolume: false,
	})
	err1 = s.bd.TakeSnapshot(s.ctx, "test3", snapshot2)
	mount3, err2 := s.rootfsMount("test3")
	err3 = s.bd.StopWorkshop(s.ctx, "test3", true)
	c.Assert(cmp.Or(err1, err2, err3), check.IsNil)

	// Mount each rootfs for comparison
	workdir := c.MkDir()
	for i := range 3 {
		err := os.Mkdir(filepath.Join(workdir, fmt.Sprint(i)), os.ModePerm)
		c.Assert(err, check.IsNil)
	}
	var roots []string
	var files []uniqueFiles
	for i, m := range []mount{mount1, mount2, mount3} {
		if m.Fstype != "zfs" {
			c.Skip("workshop storage pool is not using ZFS")
		}
		target := filepath.Join(workdir, fmt.Sprint(i))
		err := syscall.Mount(m.Source, target, m.Fstype, 0, "")
		c.Assert(err, check.IsNil)
		defer func() {
			err1 := syscall.Unmount(target, 0)
			c.Check(err1, check.IsNil)
		}()

		root := filepath.Join(target, m.Fsroot)
		roots = append(roots, root)
		files = append(files, extractUniqueFiles(c, root))
	}

	// Ensure certain files are unique.
	c.Check(files[0].hostname, check.Not(check.Equals), files[1].hostname)
	c.Check(files[0].machineID, check.Not(check.Equals), files[1].machineID)
	c.Check(files[0].networkCfg, check.Not(check.Equals), files[1].networkCfg)
	c.Check(files[0].sshKey, check.Not(check.Equals), files[1].sshKey)

	c.Check(files[0].hostname, check.Not(check.Equals), files[2].hostname)
	c.Check(files[0].machineID, check.Not(check.Equals), files[2].machineID)
	c.Check(files[0].networkCfg, check.Not(check.Equals), files[2].networkCfg)
	c.Check(files[0].sshKey, check.Not(check.Equals), files[2].sshKey)

	// Check for unexpected differences.
	output, err := exec.Command("diff", "--brief", "--no-dereference", "--recursive", roots[0], roots[1]).CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", output))

	output, err = exec.Command("diff", "--brief", "--no-dereference", "--recursive", roots[0], roots[2]).CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", output))

	// Restore first workshop from its own snapshot.
	err = syscall.Unmount(filepath.Join(workdir, "0"), 0)
	c.Assert(err, check.IsNil)
	_ = s.launchWorkshop(c, wf1, snapshot)

	// Restart it to give cloud-init a chance to run.
	err = s.bd.StartWorkshop(s.ctx, "test1")
	c.Assert(err, check.IsNil)
	mount1, err1 = s.rootfsMount("test1")
	err2 = s.bd.StopWorkshop(s.ctx, "test1", true)
	c.Assert(cmp.Or(err1, err2), check.IsNil)

	// Remount the rootfs.
	if mount1.Fstype != "zfs" {
		c.Skip("workshop storage pool is not using ZFS")
	}
	err = syscall.Mount(mount1.Source, filepath.Join(workdir, "0"), mount1.Fstype, 0, "")
	c.Assert(err, check.IsNil)
	restored := extractUniqueFiles(c, roots[0])

	// Check that only Workshop-managed attributes are preserved.
	c.Check(files[0].hostname, check.Equals, restored.hostname)
	c.Check(files[0].machineID, check.Equals, restored.machineID)
	c.Check(files[0].networkCfg, check.Equals, restored.networkCfg)
	c.Check(files[0].sshKey, check.Equals, restored.sshKey)

	// Check for unexpected differences.
	output, err = exec.Command("diff", "--brief", "--no-dereference", "--recursive", roots[1], roots[0]).CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", output))

	// Refresh first workshop from a snapshot of third workshop.
	err = syscall.Unmount(filepath.Join(workdir, "0"), 0)
	c.Assert(err, check.IsNil)
	_ = s.launchWorkshop(c, wf1, snapshot2)

	// Restart first workshop to run cloud-init.
	err = s.bd.StartWorkshop(s.ctx, "test1")
	c.Assert(err, check.IsNil)
	mount1, err1 = s.rootfsMount("test1")

	err2 = s.bd.StopWorkshop(s.ctx, "test1", true)
	c.Assert(cmp.Or(err1, err2), check.IsNil)

	// Remount the rootfs.
	if mount1.Fstype != "zfs" {
		c.Skip("workshop storage pool is not using ZFS")
	}
	err = syscall.Mount(mount1.Source, filepath.Join(workdir, "0"), mount1.Fstype, 0, "")
	c.Assert(err, check.IsNil)
	refreshed := extractUniqueFiles(c, roots[0])

	// Check that only Workshop-managed attributes are preserved.
	c.Check(files[0].hostname, check.Equals, refreshed.hostname)
	c.Check(files[0].machineID, check.Equals, refreshed.machineID)
	c.Check(files[0].networkCfg, check.Equals, refreshed.networkCfg)
	c.Check(files[0].sshKey, check.Equals, refreshed.sshKey)

	// Check for unexpected differences.
	output, err = exec.Command("diff", "--brief", "--no-dereference", "--recursive", roots[2], roots[0]).CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", output))
}

type mount struct {
	Fstype string `json:"fstype"`
	Source string `json:"source"`
	Fsroot string `json:"fsroot"`
}

// rootfsMount returns the source ZFS dataset, and subdirectory within that, of
// the given container's rootfs.
func (s *snapshotSuite) rootfsMount(name string) (mount, error) {
	args := workshop.ExecArgs{
		Command: []string{"findmnt", "--json", "--mountpoint=/", "--nofsroot", "--output=fsroot,fstype,source"},
		WorkDir: "/",
		Timeout: time.Second,
	}
	output, err := helper.ExecOutput(s.ctx, s.bd, name, args)
	if err != nil {
		return mount{}, err
	}

	var result struct {
		Filesystems []mount `json:"filesystems"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return mount{}, err
	}
	if len(result.Filesystems) != 1 {
		return mount{}, fmt.Errorf("expected 1 filesystem, found:\n%s", output)
	}

	return result.Filesystems[0], nil
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
func extractUniqueFiles(c *check.C, path string) uniqueFiles {
	hostname, err := os.ReadFile(filepath.Join(path, "etc", "hostname"))
	c.Assert(err, check.IsNil)
	machineID, err := os.ReadFile(filepath.Join(path, "etc", "machine-id"))
	c.Assert(err, check.IsNil)
	networkCfg, err := os.ReadFile(filepath.Join(path, "etc", "systemd", "network", "10-cloud-init-eth0.network.d", "workshop.conf"))
	c.Assert(err, check.IsNil)
	sshKey, err := os.ReadFile(filepath.Join(path, "etc", "ssh", "ssh_host_ed25519_key.pub"))
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
		"var/lib/workshop/run/workshop.socket.untrusted",
		"var/log/cloud-init.log",
		"var/log/cloud-init-output.log",
		"var/log/unattended-upgrades/unattended-upgrades-shutdown.log",
		"var/log/wtmp",
	}
	for _, file := range files {
		local, err := filepath.Localize(file)
		c.Assert(err, check.IsNil)

		err = os.Remove(filepath.Join(path, local))
		if !errors.Is(err, os.ErrNotExist) {
			c.Assert(err, check.IsNil)
		}
	}

	dirs := []string{
		"var/cache/snapd",
		"var/lib/cloud",
		"var/lib/snapd",
		"var/log/journal",
		"var/tmp",
	}
	for _, dir := range dirs {
		local, err := filepath.Localize(dir)
		c.Assert(err, check.IsNil)

		err = os.RemoveAll(filepath.Join(path, local))
		c.Assert(err, check.IsNil)
	}

	return uniqueFiles{
		hostname:   string(hostname),
		machineID:  string(machineID),
		networkCfg: string(networkCfg),
		sshKey:     string(sshKey),
	}
}
