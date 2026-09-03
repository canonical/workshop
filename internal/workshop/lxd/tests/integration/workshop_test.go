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
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/syscheck"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/workshop/lxd/tests/helper"
)

type wsOps struct {
	bd                 *lxdbackend.Backend
	ctx                context.Context
	usr                *user.User
	project            workshop.Project
	restoreUserLookup  func()
	restoreUserEnv     func()
	restoreNewId       func()
	restoreDevices     func()
	restoreImageServer func()
}

var _ = check.Suite(&wsOps{})

func (f *wsOps) SetUpSuite(c *check.C) {
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)

	var err error
	f.bd, err = lxdbackend.New()
	c.Assert(err, check.IsNil)

	currentUser, err := user.Current()
	c.Assert(err, check.IsNil)
	f.usr = &user.User{
		Username: "testuser",
		Uid:      currentUser.Uid,
		Gid:      currentUser.Gid,
		HomeDir:  c.MkDir(),
	}
	f.project = workshop.Project{
		ProjectId: "42424242",
		Path:      filepath.Join(c.MkDir(), "testprj"),
	}
	c.Assert(os.Mkdir(f.project.Path, os.ModePerm), check.IsNil)
	f.ctx = helper.CreateTestContext(f.usr.Username, "42424242")

	f.restoreDevices = workshop.FakeDefaultDevices(helper.DefaultTestDevices)
	f.restoreImageServer = lxdbackend.FakeImageServer(helper.MinimalImageServer)
	f.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return f.usr, nil
	})
	f.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})
	f.restoreNewId = testutil.FakeFunc(func() (string, error) {
		return f.project.ProjectId, nil
	}, &workshop.NewProjectId)
}

func (f *wsOps) TearDownSuite(c *check.C) {
	lxdclient, err := f.bd.LxdClient(f.ctx)
	c.Check(err, check.IsNil)
	defer lxdclient.Disconnect()

	helper.CleanupLxdProject(c, lxdclient, "workshop."+f.usr.Username)
	helper.CleanupLxdProject(c, lxdclient, "workshop-snapshots."+f.usr.Username)
	f.restoreUserEnv()
	f.restoreUserLookup()
	f.restoreNewId()

	f.restoreDevices()
	f.restoreImageServer()

	err = os.RemoveAll(f.project.Path)
	c.Check(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopStashUnstash(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Collect workshop metadata.
	f.waitForNetwork(c, "test")
	preStash := f.workshopMetadata(c, "test")
	c.Check(preStash.addresses, check.Not(check.HasLen), 0)

	// Stop workshop.
	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	// Validate metadata changes.
	stopped := f.workshopMetadata(c, "test")
	config := maps.Clone(stopped.config)
	c.Check(config["boot.autostart"], check.Equals, "false")
	config["boot.autostart"] = "true"
	c.Check(config, check.DeepEquals, preStash.config)
	c.Check(stopped.devices, check.DeepEquals, preStash.devices)

	// Stash workshop.
	err = f.bd.StashWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer func() {
		err := f.bd.RemoveWorkshopStash(f.ctx, "test")
		c.Check(err, check.IsNil)
	}()

	// Validate metadata changes.
	postStash := f.workshopMetadata(c, "test")
	c.Check(postStash.config, check.DeepEquals, stopped.config)
	c.Check(postStash.devices, check.DeepEquals, stopped.devices)

	stash := f.stashMetadata(c, "test")
	config = maps.Clone(postStash.config)
	c.Check(config["user.workshop.snapshot-type"], check.Equals, "")
	config["user.workshop.snapshot-type"] = "stash"
	c.Check(stash.config, check.DeepEquals, config)
	c.Check(stash.devices, check.DeepEquals, postStash.devices)

	// Rebuild workshop.
	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@22.04",
	}
	image, err := f.bd.GetBase(f.ctx, wf.Base)
	c.Assert(err, check.IsNil)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Assert(err, check.IsNil)
	snapshot := workshop.BaseOnly(f.bd.FormatRevision(), image.Name, image.Fingerprint)
	err = f.bd.LaunchOrRebuildWorkshop(f.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	// Unstash workshop.
	err = f.bd.UnstashWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate workshop metadata.
	f.waitForNetwork(c, "test")
	postUnstash := f.workshopMetadata(c, "test")
	c.Check(postUnstash.config, check.DeepEquals, preStash.config)
	c.Check(postUnstash.devices, check.DeepEquals, preStash.devices)
	c.Check(postUnstash.addresses, testutil.DeepUnsortedMatches, preStash.addresses)
}

// Wait until workshop acquires both an IPv4 and an IPv6 address.
func (f *wsOps) waitForNetwork(c *check.C, name string) {
	args := workshop.Execution{
		ExecArgs: workshop.ExecArgs{
			Command: []string{
				"/usr/lib/systemd/systemd-networkd-wait-online",
				"--ipv4",
				"--ipv6",
				"--operational-state=routable",
			},
			WorkDir: "/",
			Timeout: time.Minute,
		},
	}
	exectx, err := f.bd.Exec(f.ctx, name, &args)
	c.Assert(err, check.IsNil)
	c.Assert(exectx.WaitExecution(f.ctx), check.IsNil)
}

type metadata struct {
	config    map[string]string
	devices   map[string]map[string]string
	addresses []string
}

func (f *wsOps) workshopMetadata(c *check.C, name string) metadata {
	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	return instanceMetadata(c, conn, lxdbackend.InstanceName(name, f.project.ProjectId))
}

func (f *wsOps) stashMetadata(c *check.C, name string) metadata {
	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	conn = conn.UseProject("workshop-snapshots." + f.usr.Username)

	return instanceMetadata(c, conn, "stash-"+lxdbackend.InstanceName(name, f.project.ProjectId))
}

func instanceMetadata(c *check.C, conn lxd.InstanceServer, name string) metadata {
	inst := fullInstance(c, conn, name)
	return metadata{inst.Config, inst.Devices, ipAddresses(inst)}
}

func fullInstance(c *check.C, conn lxd.InstanceServer, name string) *api.InstanceFull {
	inst, _, err := conn.GetInstanceFull(name)
	c.Assert(err, check.IsNil)

	maps.DeleteFunc(inst.Config, func(k, v string) bool { return !includeWhenCopying(k) })

	return inst
}

func includeWhenCopying(key string) bool {
	if strings.HasPrefix(key, "user.ed25519-key.") {
		return false
	}
	if !strings.HasPrefix(key, "volatile.") {
		return true
	}
	return slices.Contains(api.InstanceRemoteCopyConfigKeyPolicy.Immutable, key)
}

func ipAddresses(inst *api.InstanceFull) []string {
	var addresses []string
	for _, address := range inst.State.Network["eth0"].Addresses {
		if !slices.Contains([]string{"inet", "inet6"}, address.Family) {
			continue
		}
		if slices.Contains([]string{"link", "local"}, address.Scope) {
			continue
		}
		addresses = append(addresses, address.Address)
	}
	return addresses
}

func (f *wsOps) TestLxdBackendWorkshopStashRemove(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Execute
	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)
	err = f.bd.StashWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Execute
	err = f.bd.RemoveWorkshopStash(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate
	err = f.bd.UnstashWorkshop(f.ctx, "test")
	c.Assert(err, check.ErrorMatches, "workshop not launched")
}

var testsdk = `name: test-sdk
title: title
base: ubuntu@20.04
version: '0.1.2'
summary: summary
description: SDK
sdkcraft-started-at: '2020-04-22T19:12:07.903032+00:00'
`

func (f *wsOps) TestLxdBackendImportSdkOK(c *check.C) {
	// Execute
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: testsdk,
	}
	tarball := helper.MockSdkTarball(c, meta.Name, testsdk)

	var wg sync.WaitGroup

	var successCnt, existCnt int32
	for range 5 {
		wg.Go(func() {
			file, err := os.Open(tarball)
			c.Assert(err, check.IsNil)
			defer file.Close()
			if err := f.bd.ImportSdk(f.ctx, meta, file); err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
				atomic.AddInt32(&existCnt, 1)
			} else {
				c.Assert(err, check.IsNil)
			}
		})
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(4))

	vinfo, err := f.bd.Sdk(f.ctx, meta.Setup)
	c.Check(err, check.IsNil)
	meta.Channel = ""
	c.Check(vinfo.Meta, check.Equals, meta)
	c.Check(vinfo.Workshops, check.HasLen, 0)

	// Check again without meta.Channel.
	vinfo, err = f.bd.Sdk(f.ctx, meta.Setup)
	c.Check(err, check.IsNil)
	c.Check(vinfo.Meta, check.Equals, meta)
	c.Check(vinfo.Workshops, check.HasLen, 0)

	err = f.bd.DeleteSdk(f.ctx, meta.Setup)
	c.Assert(err, check.IsNil)
}

// This test detects changes to the import operation type and conflict error
// message. See IsImportSdkOperation and IsImportSdkConflict.
func (f *wsOps) TestLxdBackendImportSdkConflict(c *check.C) {
	// Execute
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: testsdk,
	}
	tarball := helper.MockSdkTarball(c, meta.Name, testsdk)

	// Create test project without racing for it.
	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	conn.Disconnect()

	var wg sync.WaitGroup

	var successCnt, existCnt int32
	for range 2 {
		wg.Go(func() {
			conn, err := f.bd.LxdClient(f.ctx)
			c.Assert(err, check.IsNil)
			defer conn.Disconnect()

			file, err := os.Open(tarball)
			c.Assert(err, check.IsNil)
			defer file.Close()

			args := lxd.StoragePoolVolumeBackupArgs{
				BackupFile: file,
				Name:       sdk.VolumeName(meta.Name, meta.Revision),
			}
			op, err := conn.CreateStoragePoolVolumeFromTarball("workshop", args)
			if err == nil {
				isImport, err1 := lxdbackend.IsImportSdkOperation(op.Get(), args.Name)
				c.Assert(err1, check.IsNil)
				c.Check(isImport, check.Equals, true, check.Commentf("%#v", op.Get()))
				err = op.Wait()
			}
			if lxdbackend.IsImportSdkConflict(err) {
				atomic.AddInt32(&existCnt, 1)
				return
			} else if !c.Check(err, check.IsNil) {
				return
			}

			atomic.AddInt32(&successCnt, 1)

			volume, etag, err := conn.GetStoragePoolVolume("workshop", "custom", args.Name)
			c.Assert(err, check.IsNil)
			volume.Config["user.kind"] = "sdk"
			op, err = conn.UpdateStoragePoolVolume("workshop", "custom", args.Name, volume.Writable(), etag)
			c.Assert(err, check.IsNil)
			isImport, err := lxdbackend.IsImportSdkOperation(op.Get(), args.Name)
			c.Assert(err, check.IsNil)
			c.Check(isImport, check.Equals, true, check.Commentf("%#v", op.Get()))
			c.Assert(op.Wait(), check.IsNil)
		})
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(1))

	err = f.bd.DeleteSdk(f.ctx, meta.Setup)
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendImportSdkInterrupted(c *check.C) {
	// Execute
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: testsdk,
	}
	tarball := helper.MockSdkTarball(c, meta.Name, testsdk)

	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	// Simulate operation started by previous workshopd run.
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)

		file, err := os.Open(tarball)
		c.Assert(err, check.IsNil)
		defer file.Close()

		vol := lxd.StoragePoolVolumeBackupArgs{
			BackupFile: file,
			Name:       sdk.VolumeName(meta.Name, meta.Revision),
		}
		op, err := conn.CreateStoragePoolVolumeFromTarball("workshop", vol)
		c.Assert(err, check.IsNil)
		close(started)
		_ = op.Wait()
	}()

	file, err := os.Open(tarball)
	if c.Check(err, check.IsNil) {
		defer file.Close()

		err = f.bd.ImportSdk(f.ctx, meta, file)
		c.Check(err, check.IsNil)

		select {
		case <-started:
		case <-done:
		}

		err = f.bd.DeleteSdk(f.ctx, meta.Setup)
		c.Check(err, check.IsNil)
	}

	<-done
}

func (f *wsOps) TestLxdBackendDeleteWorkshop(c *check.C) {
	// Launch
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)

	// Validate
	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)
	err = f.bd.RemoveWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Check(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
}

// List images marked by Workshop for the given base.
func (f *wsOps) listWorkshopImages(c *check.C, base string) []api.Image {
	cli, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer cli.Disconnect()

	images, err := cli.GetImagesWithFilter([]string{"type=container", "properties.workshop-base=" + base})
	c.Assert(err, check.IsNil)

	return images
}

// List images for the given base, including those unknown to Workshop. Only
// tested for ubuntu and ubuntu-minimal images.
func (f *wsOps) listAllImages(c *check.C, base string) []api.Image {
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '@' })
	c.Assert(parts, check.HasLen, 2)

	cli, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer cli.Disconnect()

	images, err := cli.GetImagesWithFilter([]string{"type=container", "properties.os=" + parts[0], "properties.version=" + parts[1]})
	c.Assert(err, check.IsNil)

	return images
}

func (f *wsOps) deleteImages(c *check.C, base string) {
	images := f.listAllImages(c, base)

	cli, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer cli.Disconnect()

	for _, image := range images {
		op, err := cli.DeleteImage(image.Fingerprint)
		c.Assert(err, check.IsNil)
		c.Assert(op.Wait(), check.IsNil)
	}
}

func (f *wsOps) TestLxdBackendDownloadBase(c *check.C) {
	// ensure there is no image in LXD storage
	f.deleteImages(c, "ubuntu@22.04")

	image, err := f.bd.GetBase(f.ctx, "ubuntu@22.04")
	c.Assert(err, check.IsNil)
	c.Check(image.Name, check.Equals, "ubuntu@22.04")
	c.Assert(image.Fingerprint, check.Not(check.Equals), "")

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			err := f.bd.DownloadBase(f.ctx, image, nil)
			c.Check(err, check.IsNil)
		})
	}
	wg.Wait()

	images := f.listWorkshopImages(c, "ubuntu@22.04")
	c.Assert(images, check.HasLen, 1)
	c.Check(images[0].AutoUpdate, check.Equals, false)
	c.Check(images[0].Cached, check.Equals, false)
	c.Check(images[0].Fingerprint, check.Equals, image.Fingerprint)

	// Check behaviour when image already downloaded.
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.IsNil)

	images2 := f.listWorkshopImages(c, "ubuntu@22.04")
	c.Assert(images2, check.HasLen, 1)
	c.Check(images2[0], check.DeepEquals, images[0])
}

func (f *wsOps) TestLxdBackendGetOrDownloadMalformedBase(c *check.C) {
	image := workshop.BaseImage{Name: "ubuntu:24.04", Fingerprint: ""}
	_, err := f.bd.GetBase(f.ctx, image.Name)
	c.Check(err, check.ErrorMatches, `invalid base "ubuntu:24.04" \(expected <NAME>@<VERSION>\)`)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.ErrorMatches, `invalid base "ubuntu:24.04" \(expected <NAME>@<VERSION>\)`)

	image.Name = "ubuntu@"
	_, err = f.bd.GetBase(f.ctx, image.Name)
	c.Check(err, check.ErrorMatches, `invalid base "ubuntu@" \(expected <NAME>@<VERSION>\)`)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.ErrorMatches, `invalid base "ubuntu@" \(expected <NAME>@<VERSION>\)`)

	image.Name = "canonical@ubuntu@24.04"
	_, err = f.bd.GetBase(f.ctx, image.Name)
	c.Check(err, check.ErrorMatches, `invalid base "canonical@ubuntu@24.04" \(expected <NAME>@<VERSION>\)`)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.ErrorMatches, `invalid base "canonical@ubuntu@24.04" \(expected <NAME>@<VERSION>\)`)
}

func (f *wsOps) TestLxdBackendDownloadBaseImageNotFound(c *check.C) {
	_, err := f.bd.GetBase(f.ctx, "ubuntu@1.01")
	c.Check(err, check.ErrorMatches, `base "ubuntu@1.01" not found.*`)

	image := workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "##################"}
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.ErrorMatches, `"ubuntu@22.04" download failed.*`)
}

func (f *wsOps) TestLxdBackendDownloadProtocolNotSupported(c *check.C) {
	defer lxdbackend.FakeImageServer("https://cloud-images.ubuntu.com/minimal/releases")()

	image := workshop.BaseImage{Name: "ubuntu@20.04", Fingerprint: ""}
	_, err := f.bd.GetBase(f.ctx, image.Name)
	c.Check(err, check.ErrorMatches, `unknown image server URL prefix \(supported: simplestreams, lxd\)`)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Check(err, check.ErrorMatches, `unknown image server URL prefix \(supported: simplestreams, lxd\)`)
}

func (f *wsOps) TestLxdBackendDownloadConcurrentErrors(c *check.C) {
	image := workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: "##################"}

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			err := f.bd.DownloadBase(f.ctx, image, nil)
			c.Check(err, check.ErrorMatches, `"ubuntu@22.04" download failed.*`)
		})
	}
	wg.Wait()
}

func (f *wsOps) TestLxdBackendDownloadBaseResumeAfterCancellation(c *check.C) {
	// ensure there is no image in LXD storage
	f.deleteImages(c, "ubuntu@22.04")

	image, err := f.bd.GetBase(f.ctx, "ubuntu@22.04")
	c.Assert(err, check.IsNil)
	c.Check(image.Name, check.Equals, "ubuntu@22.04")
	c.Assert(image.Fingerprint, check.Not(check.Equals), "")

	wcancel, cancel := context.WithCancel(f.ctx)
	defer cancel()

	var wg sync.WaitGroup
	var once sync.Once
	for range 3 {
		wg.Go(func() {
			r := &progress.Reporter{
				Name: "1",
				Report: func(label string, done, total int64) {
					once.Do(func() { cancel() })
				},
			}
			err := f.bd.DownloadBase(wcancel, image, r)
			c.Check(err, testutil.ErrorIs, context.Canceled)
		})
	}
	wg.Wait()

	// attempt to download after interruption (must pickup an ongoing operation
	// and wait for it).
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Assert(err, check.IsNil)

	images := f.listWorkshopImages(c, "ubuntu@22.04")
	c.Assert(images, check.HasLen, 1)
	c.Check(images[0].AutoUpdate, check.Equals, false)
	c.Check(images[0].Cached, check.Equals, false)
	c.Check(images[0].Fingerprint, check.Equals, image.Fingerprint)
}

func (f *wsOps) TestLxdBackendDownloadMultipleBasesConcurrently(c *check.C) {
	// ensure there is no image in LXD storage
	for _, b := range workshop.SupportedBases {
		f.deleteImages(c, b)
	}

	fingerprints := make([]string, len(workshop.SupportedBases))

	var wg sync.WaitGroup
	for i, b := range workshop.SupportedBases {
		wg.Go(func() {
			image, err := f.bd.GetBase(f.ctx, b)
			c.Assert(err, check.IsNil)
			c.Check(image.Name, check.Equals, b)
			c.Assert(image.Fingerprint, check.Not(check.Equals), "")
			fingerprints[i] = image.Fingerprint

			err = f.bd.DownloadBase(f.ctx, image, nil)
			c.Assert(err, check.IsNil)
		})
	}
	wg.Wait()

	for i, b := range workshop.SupportedBases {
		images := f.listWorkshopImages(c, b)
		c.Assert(images, check.HasLen, 1)
		c.Check(images[0].AutoUpdate, check.Equals, false)
		c.Check(images[0].Cached, check.Equals, false)
		c.Check(images[0].Fingerprint, check.Equals, fingerprints[i])
	}
}

// Since our LXD projects don't contain images, we have to put them in the
// default project. So our images are likely to be shared with containers
// created using lxc. The next few tests ensure that it doesn't matter whether
// Workshop or lxc downloads the image first, and that LXD won't try to prune
// images shared in this way. However, there's a bug in LXD which causes it to
// prune images shared with non-default projects. This is the reason why we use
// the default project in the first place. So far the issue hasn't been
// observed in the wild, but it probably would have been if *craft used the
// same image server as Workshop. See this issue for details:
// https://github.com/canonical/lxd/issues/16515
func (f *wsOps) TestLxdBackendReuseDownloadedBase(c *check.C) {
	// Attempt twice in case the image is updated partway.
	for range 2 {
		// ensure there is no image in LXD storage
		f.deleteImages(c, "ubuntu@22.04")

		images := f.listAllImages(c, "ubuntu@22.04")
		c.Assert(images, check.HasLen, 0)

		image, err := f.bd.GetBase(f.ctx, "ubuntu@22.04")
		c.Assert(err, check.IsNil)
		c.Check(image.Name, check.Equals, "ubuntu@22.04")
		c.Assert(image.Fingerprint, check.Not(check.Equals), "")
		err = f.bd.DownloadBase(f.ctx, image, nil)
		c.Assert(err, check.IsNil)

		images = f.listAllImages(c, "ubuntu@22.04")
		c.Assert(images, check.HasLen, 1)
		imageDownloaded := images[0]

		c.Check(imageDownloaded.AutoUpdate, check.Equals, false)
		c.Check(imageDownloaded.Cached, check.Equals, false)
		c.Check(imageDownloaded.Fingerprint, check.Equals, image.Fingerprint)
		c.Check(imageDownloaded.Properties["workshop-base"], check.Equals, "ubuntu@22.04")
		c.Check(imageDownloaded.UpdateSource, check.IsNil)

		tempInstance := fmt.Sprintf("%08x-test", rand.Uint32())
		init := exec.Command("lxc", "init", "ubuntu-minimal:22.04", tempInstance)
		c.Assert(init.Run(), check.IsNil)
		cleanup := exec.Command("lxc", "delete", tempInstance)
		c.Assert(cleanup.Run(), check.IsNil)

		images = f.listAllImages(c, "ubuntu@22.04")
		if len(images) > 1 || images[0].Fingerprint != image.Fingerprint {
			// Alias was just updated, try again.
			continue
		}

		c.Assert(images, check.HasLen, 1)
		imageCached := images[0]

		c.Check(imageCached.LastUsedAt, check.Not(check.Equals), imageDownloaded.LastUsedAt)
		imageCached.LastUsedAt = imageDownloaded.LastUsedAt
		c.Check(imageCached, check.DeepEquals, imageDownloaded)

		break
	}
}

func (f *wsOps) TestLxdBackendReuseCachedBase(c *check.C) {
	// ensure there is no image in LXD storage
	f.deleteImages(c, "ubuntu@22.04")

	images := f.listAllImages(c, "ubuntu@22.04")
	c.Assert(images, check.HasLen, 0)

	tempInstance := fmt.Sprintf("%08x-test", rand.Uint32())
	init := exec.Command("lxc", "init", "ubuntu-minimal:22.04", tempInstance)
	c.Assert(init.Run(), check.IsNil)
	cleanup := exec.Command("lxc", "delete", tempInstance)
	c.Assert(cleanup.Run(), check.IsNil)

	images = f.listAllImages(c, "ubuntu@22.04")
	c.Assert(images, check.HasLen, 1)
	imageCached := images[0]

	c.Check(imageCached.AutoUpdate, check.Equals, true)
	c.Check(imageCached.Cached, check.Equals, true)
	_, ok := imageCached.Properties["workshop-base"]
	c.Check(ok, check.Equals, false)
	c.Check(imageCached.UpdateSource, check.NotNil)

	image := workshop.BaseImage{Name: "ubuntu@22.04", Fingerprint: imageCached.Fingerprint}
	err := f.bd.DownloadBase(f.ctx, image, nil)
	c.Assert(err, check.IsNil)

	images = f.listAllImages(c, "ubuntu@22.04")
	c.Assert(images, check.HasLen, 1)
	imageDownloaded := images[0]

	// CreateImage updates Public, AutoUpdate, Filename, Properties
	// and Profiles if any of them are set. It also does so if
	// Profiles is nil (but not if it's a non-nil empty slice).
	// This works for us since we want to unset AutoUpdate and add the
	// workshop-base property. As a side effect, the Filename is lost as
	// well. This doesn't seem to have any practical significance.
	imageCached.Filename = ""
	imageCached.AutoUpdate = false
	imageCached.Cached = false
	imageCached.Properties["workshop-base"] = "ubuntu@22.04"
	c.Check(imageDownloaded, check.DeepEquals, imageCached)
}

func (f *wsOps) TestLxdBackendWorkshopStartFailed(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Check(err, check.IsNil)

	// Leaves the workshop instance in a started state with a failed start
	// command. The StartWorkshop API must clean up its previous progress, i.e.
	// set the workshop to the Stopped state.
	defer lxdbackend.FakeStartCommand("exit 1")()

	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Check(err, check.NotNil)

	w, err := f.bd.Workshop(f.ctx, "test")
	c.Check(err, check.IsNil)
	c.Check(w.Running, check.Equals, false)
}

func (f *wsOps) TestLxdBackendWorkshopStartStopIdempotent(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Check(err, check.IsNil)

	err = f.bd.StopWorkshop(f.ctx, "test", true)
	c.Check(err, check.IsNil)

	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Check(err, check.IsNil)

	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Check(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopMount(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	mount := workshop.Mount{
		Name:  "test-mount",
		Type:  workshop.HostWorkshop,
		What:  filepath.Join(c.MkDir(), "mnt"),
		Where: "/mnt",
	}

	err := os.MkdirAll(filepath.Join(mount.What, "test-dir"), 0755)
	c.Assert(err, check.IsNil)

	err = f.bd.AddWorkshopMount(f.ctx, "test", mount)
	c.Assert(err, check.IsNil)

	entries := f.lsMnt(c)
	c.Assert(entries, check.HasLen, 1)
	c.Check(entries[0].Name(), check.Equals, "test-dir")

	err = f.bd.AddWorkshopMount(f.ctx, "test", mount)
	c.Assert(err, check.IsNil)

	entries = f.lsMnt(c)
	c.Assert(entries, check.HasLen, 1)
	c.Check(entries[0].Name(), check.Equals, "test-dir")

	mount.What = filepath.Join(c.MkDir(), "mnt")
	err = os.MkdirAll(filepath.Join(mount.What, "test-dir2"), 0755)
	c.Assert(err, check.IsNil)

	err = f.bd.AddWorkshopMount(f.ctx, "test", mount)
	c.Assert(err, check.IsNil)

	entries = f.lsMnt(c)
	c.Assert(entries, check.HasLen, 1)
	c.Check(entries[0].Name(), check.Equals, "test-dir2")

	err = f.bd.RemoveWorkshopMount(f.ctx, "test", mount.Name)
	c.Assert(err, check.IsNil)

	entries = f.lsMnt(c)
	c.Check(entries, check.HasLen, 0)

	err = f.bd.RemoveWorkshopMount(f.ctx, "test", mount.Name)
	c.Check(err, check.IsNil)

	entries = f.lsMnt(c)
	c.Check(entries, check.HasLen, 0)
}

func (f *wsOps) lsMnt(c *check.C) []os.FileInfo {
	fs, err := f.bd.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer func() {
		c.Check(fs.Close(), check.IsNil)
	}()

	entries, err := fs.ReadDir("/mnt")
	c.Assert(err, check.IsNil)

	return entries
}

func (f *wsOps) TestLxdBackendWorkshopLaunch(c *check.C) {
	image, err := f.bd.GetBase(f.ctx, "ubuntu@24.04")
	c.Assert(err, check.IsNil)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Assert(err, check.IsNil)

	wf := &workshop.File{Name: "test", Base: "ubuntu@24.04"}
	snapshot := workshop.BaseOnly(f.bd.FormatRevision(), image.Name, image.Fingerprint)
	err = f.bd.LaunchOrRebuildWorkshop(f.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	w, err := f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	c.Check(w.Image, check.Equals, image)
}

func (f *wsOps) TestLxdBackendSetupIsIdempotent(c *check.C) {
	// The backend setup already ran once in SetUpSuite, creating the storage
	// pool and network. Running it again must reconcile the existing resources
	// rather than fail, so the degraded-mode recovery loop can safely re-run it
	// once LXD is installed or refreshed.
	_, err := lxdbackend.New()
	c.Check(err, check.IsNil)

	_, err = lxdbackend.New()
	c.Check(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendStorageSpaceCheckHealthy(c *check.C) {
	// With a freshly created pool the storage space check must not put the
	// daemon into degraded mode.
	c.Check(syscheck.CheckSystem(), check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopRebuild(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Collect workshop metadata.
	f.waitForNetwork(c, "test")
	original := f.workshopMetadata(c, "test")
	maps.DeleteFunc(original.config, func(k, v string) bool { return modifiedByRebuild(k) })
	c.Check(original.addresses, check.Not(check.HasLen), 0)

	// Stop workshop.
	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	// Rebuild workshop.
	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@22.04",
	}
	image, err := f.bd.GetBase(f.ctx, "ubuntu@22.04")
	c.Assert(err, check.IsNil)
	err = f.bd.DownloadBase(f.ctx, image, nil)
	c.Assert(err, check.IsNil)
	snapshot := workshop.BaseOnly(f.bd.FormatRevision(), image.Name, image.Fingerprint)
	err = f.bd.LaunchOrRebuildWorkshop(f.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	// Start workshop.
	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate workshop metadata.
	f.waitForNetwork(c, "test")
	rebuilt := f.workshopMetadata(c, "test")
	c.Check(rebuilt.config["image.workshop-base"], check.Equals, "ubuntu@22.04")
	c.Check(rebuilt.config["user.workshop.file"], check.Equals, "name: test\nbase: ubuntu@22.04\n")
	c.Check(rebuilt.config["user.workshop.base-fingerprint"], check.Equals, image.Fingerprint)
	maps.DeleteFunc(rebuilt.config, func(k, v string) bool { return modifiedByRebuild(k) })
	c.Check(rebuilt.config, check.DeepEquals, original.config)
	c.Check(rebuilt.devices, check.DeepEquals, original.devices)
	c.Check(rebuilt.addresses, testutil.DeepUnsortedMatches, original.addresses)
}

func modifiedByRebuild(key string) bool {
	switch key {
	case "user.workshop.file", "user.workshop.base-fingerprint":
		return true
	}
	return strings.HasPrefix(key, "image.") || strings.HasPrefix(key, "volatile.")
}

func (f *wsOps) TestLxdBackendWorkshopRestoreResetsSdkConfiguration(c *check.C) {
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:     "test-sdk",
			Source:   sdk.TrySource,
			Revision: sdk.R(5),
			Sha3_384: "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
		},
		SdkYAML: testsdk,
	}
	helper.MockSdkVolume(c, f.ctx, f.bd, meta)
	defer func() { c.Check(f.bd.DeleteSdk(f.ctx, meta.Setup), check.IsNil) }()

	meta2 := sdk.Meta{
		Setup: sdk.Setup{
			Name:     "test-sdk-2",
			Source:   sdk.TrySource,
			Revision: sdk.R(5),
			Sha3_384: "d4089378c26310627268153caa216240311f2a3193c778e96ed6dd895dc10c82db50f4f39676b29d23d9813b21e14b9b",
		},
		SdkYAML: testsdk,
	}
	helper.MockSdkVolume(c, f.ctx, f.bd, meta2)
	defer func() { c.Check(f.bd.DeleteSdk(f.ctx, meta2.Setup), check.IsNil) }()

	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	w, err := f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	image := w.Image

	err = f.bd.InstallSdk(f.ctx, "test", meta.Setup)
	c.Assert(err, check.IsNil)
	defer func() { c.Check(f.bd.UninstallSdk(f.ctx, "test", meta.Name), check.IsNil) }()

	// Check that "test-sdk" directory is mounted.
	fs, err := f.bd.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	_, err = fs.Stat(sdk.SdkMetaPath(meta.Name))
	fs.Close()
	c.Check(err, check.IsNil)

	info, err := f.bd.Sdk(f.ctx, meta.Setup)
	c.Assert(err, check.IsNil)
	saved := meta
	saved.Source = 0
	c.Check(info.Meta, check.Equals, saved)
	c.Check(info.Workshops, check.DeepEquals, map[string][]string{f.project.ProjectId: {"test"}})

	snapshot := workshop.SdkSnapshot(w.Format, w.Image, []sdk.Setup{meta.Setup})
	err = f.bd.TakeSnapshot(f.ctx, "test", snapshot)
	c.Assert(err, check.IsNil)

	// Install SDK 2 post-snapshot. It should be gone post-restore.
	err = f.bd.InstallSdk(f.ctx, "test", meta2.Setup)
	c.Assert(err, check.IsNil)
	defer func() { _ = f.bd.UninstallSdk(f.ctx, "test", meta2.Name) }()

	// Restore the workshop from the snapshot.
	err = f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	wf := &workshop.File{Name: "test", Base: "ubuntu@24.04"}
	err = f.bd.LaunchOrRebuildWorkshop(f.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	w, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	c.Check(w.Running, check.Equals, false)

	// Check that Restore uses the provided "user.workshop.file," keeps its
	// base fingerprint and removes both SDKs from the workshop.
	c.Check(w.File, check.DeepEquals, wf)
	c.Check(w.Image, check.Equals, image)
	c.Check(w.Sdks, check.HasLen, 0)

	fs, err = f.bd.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer fs.Close()
	// Check that "test-sdk" directory is present but not mounted.
	_, err = fs.Stat(sdk.SdkDir(meta.Name))
	c.Check(err, check.IsNil)
	_, err = fs.Stat(sdk.SdkMetaPath(meta.Name))
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
	// Check that "test-sdk-2" directory is not present in the filesystem.
	_, err = fs.Stat(sdk.SdkDir(meta2.Name))
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
}

func (f *wsOps) TestLxdBackendWorkshopUsedByInVolumeInfoOK(c *check.C) {
	// First workshop
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Second workshop
	other := workshop.Project{ProjectId: "24242424", Path: c.MkDir()}
	otherCtx := helper.CreateTestContext(f.usr.Username, other.ProjectId)
	helper.LaunchTestWorkshop(c, otherCtx, f.bd, other.Path)
	defer helper.RemoveTestWorkshop(c, otherCtx, f.bd)

	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test-sdk",
			PackageID: "t5tqUClfNeHiiOpvPvT29O0HkxeaXBOq",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "d024fbe91c6b99d0064306d52006c17a5d0406822ff253fbbe6a934ca9be50d3ff9a6ec3bac3be8396006029a1ff453a",
		},
		SdkYAML: testsdk,
	}
	helper.MockSdkVolume(c, f.ctx, f.bd, meta)
	defer func() { c.Check(f.bd.DeleteSdk(f.ctx, meta.Setup), check.IsNil) }()

	// Attach the volume to both workshops.
	err := f.bd.InstallSdk(f.ctx, "test", meta.Setup)
	c.Assert(err, check.IsNil)
	err = f.bd.InstallSdk(otherCtx, "test", meta.Setup)
	c.Assert(err, check.IsNil)

	// Ensure the volume cannot be deleted while attached to workshops.
	err = f.bd.DeleteSdk(f.ctx, meta.Setup)
	c.Assert(err, testutil.ErrorIs, workshop.ErrVolumeInUse)

	// Validate UsedBy in VolumeInfo.
	info, err := f.bd.Sdk(f.ctx, meta.Setup)
	c.Assert(err, check.IsNil)
	meta.Channel = ""
	c.Check(info.Meta, check.Equals, meta)
	c.Check(info.Workshops, check.DeepEquals, map[string][]string{f.project.ProjectId: {"test"}, other.ProjectId: {"test"}})

	// Detach the volume from the first workshop.
	err = f.bd.UninstallSdk(f.ctx, "test", meta.Name)
	c.Assert(err, check.IsNil)

	// Validate UsedBy in VolumeInfo.
	info, err = f.bd.Sdk(f.ctx, meta.Setup)
	c.Assert(err, check.IsNil)
	c.Check(info.Meta, check.Equals, meta)
	c.Check(info.Workshops, check.DeepEquals, map[string][]string{other.ProjectId: {"test"}})

	err = f.bd.UninstallSdk(otherCtx, "test", meta.Name)
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendSnapshotOK(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	inst, _, err := conn.GetInstance(lxdbackend.InstanceName("test", f.project.ProjectId))
	c.Assert(err, check.IsNil)
	for i := range 4 {
		name := fmt.Sprint("test", i+2)
		args := &lxd.InstanceCopyArgs{Name: lxdbackend.InstanceName(name, f.project.ProjectId)}
		inst.Config[workshop.ConfigWorkshopName] = name
		op, err := conn.CopyInstance(conn, *inst, args)
		c.Assert(err, check.IsNil)
		c.Assert(op.Wait(), check.IsNil)
		defer func() { _ = f.bd.RemoveWorkshop(f.ctx, name) }()
	}

	snapshot := workshop.Snapshot{
		Format: f.bd.FormatRevision(),
		Image: workshop.BaseImage{
			Name:        "ubuntu@24.04",
			Fingerprint: "0b9429c9855cb158b90159bb818e6f98eab9b5b1260ace11b30ddb936e4f78979abc7cdc5e4e9fad51e3e290a2190ac2",
		},
		Sdks: []sdk.ContentID{{
			Name:     "system",
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			IsVolume: true,
		}},
	}

	_, err = f.bd.Snapshot(f.ctx, snapshot)
	c.Check(err, testutil.ErrorIs, workshop.ErrSnapshotNotFound)

	defer func() { _ = f.bd.RemoveSnapshot(f.ctx, snapshot) }()
	var wg sync.WaitGroup
	var successCnt, existCnt int32
	for i := range 5 {
		wg.Go(func() {
			name := "test"
			if i > 0 {
				name = fmt.Sprint("test", i+1)
			}
			if err := f.bd.TakeSnapshot(f.ctx, name, snapshot); err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else if errors.Is(err, workshop.ErrSnapshotAlreadyExists) {
				atomic.AddInt32(&existCnt, 1)
			} else {
				c.Errorf("unexpected error: %v", err)
			}

			info, err := f.bd.Snapshot(f.ctx, snapshot)
			c.Assert(err, check.IsNil)
			c.Check(info.Snapshot, check.DeepEquals, snapshot)
			workshops := map[string][]string{
				f.project.ProjectId: {"test", "test2", "test3", "test4", "test5"},
			}
			c.Check(info.Workshops, testutil.DeepUnsortedMatches, workshops)
		})
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(4))
}

// This test detects changes to the snapshot operation type and conflict error
// message. See IsInstanceOperation and IsInstanceConflict.
func (f *wsOps) TestLxdBackendSnapshotConflict(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	snapshot := workshop.Snapshot{
		Format: f.bd.FormatRevision(),
		Image: workshop.BaseImage{
			Name:        "ubuntu@24.04",
			Fingerprint: "0b9429c9855cb158b90159bb818e6f98eab9b5b1260ace11b30ddb936e4f78979abc7cdc5e4e9fad51e3e290a2190ac2",
		},
		Sdks: []sdk.ContentID{{
			Name:     "system",
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			IsVolume: true,
		}},
	}
	digest, err := f.bd.HashSnapshot(snapshot)
	c.Assert(err, check.IsNil)

	// Create test project without racing for it.
	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	conn.Disconnect()

	var wg sync.WaitGroup

	var successCnt, existCnt int32
	for range 2 {
		wg.Go(func() {
			conn, err := f.bd.LxdClient(f.ctx)
			c.Assert(err, check.IsNil)
			defer conn.Disconnect()
			lxdProject := "workshop-snapshots." + f.usr.Username
			snapshotConn := conn.UseProject(lxdProject)

			inst, _, err := conn.GetInstance(lxdbackend.InstanceName("test", f.project.ProjectId))
			c.Assert(err, check.IsNil)

			args := &lxd.InstanceCopyArgs{Name: "system-" + digest[:16]}
			rop, err := snapshotConn.CopyInstance(conn, *inst, args)
			if err == nil {
				op, err1 := rop.GetTarget()
				c.Assert(err1, check.IsNil)
				isInstance, err1 := lxdbackend.IsInstanceOperation(*op, lxdProject, args.Name)
				c.Assert(err1, check.IsNil)
				c.Check(isInstance, check.Equals, true, check.Commentf("%#v", op))
				err = rop.Wait()
			}
			if lxdbackend.IsInstanceConflict(err, args.Name) {
				atomic.AddInt32(&existCnt, 1)
				return
			} else if !c.Check(err, check.IsNil) {
				return
			}

			atomic.AddInt32(&successCnt, 1)

			inst, etag, err := snapshotConn.GetInstance(args.Name)
			c.Assert(err, check.IsNil)
			inst.Config[workshop.ConfigWorkshopSnapshotType] = "sdk"
			op, err := snapshotConn.UpdateInstance(args.Name, inst.Writable(), etag)
			c.Assert(err, check.IsNil)
			isInstance, err := lxdbackend.IsInstanceOperation(op.Get(), lxdProject, args.Name)
			c.Assert(err, check.IsNil)
			c.Check(isInstance, check.Equals, true, check.Commentf("%#v", op.Get()))
			c.Assert(op.Wait(), check.IsNil)
		})
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(1))

	err = f.bd.RemoveSnapshot(f.ctx, snapshot)
	c.Check(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendSnapshotInterrupted(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()
	snapshotConn := conn.UseProject("workshop-snapshots." + f.usr.Username)

	snapshot := workshop.Snapshot{
		Format: f.bd.FormatRevision(),
		Image: workshop.BaseImage{
			Name:        "ubuntu@24.04",
			Fingerprint: "0b9429c9855cb158b90159bb818e6f98eab9b5b1260ace11b30ddb936e4f78979abc7cdc5e4e9fad51e3e290a2190ac2",
		},
		Sdks: []sdk.ContentID{{
			Name:     "system",
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			IsVolume: true,
		}},
	}
	digest, err := f.bd.HashSnapshot(snapshot)
	c.Assert(err, check.IsNil)

	// Simulate operation started by previous workshopd run.
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)

		inst, _, err := conn.GetInstance(lxdbackend.InstanceName("test", f.project.ProjectId))
		c.Assert(err, check.IsNil)

		op, err := snapshotConn.CopyInstance(conn, *inst, &lxd.InstanceCopyArgs{Name: "system-" + digest[:16]})
		c.Assert(err, check.IsNil)
		close(started)
		_ = op.Wait()
	}()

	err = f.bd.TakeSnapshot(f.ctx, "test", snapshot)
	c.Check(err, check.IsNil)

	select {
	case <-started:
	case <-done:
	}

	err = f.bd.RemoveSnapshot(f.ctx, snapshot)
	c.Check(err, check.IsNil)

	<-done
}

func (f *wsOps) TestLxdBackendSnapshotHashCollision(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	snapshot := workshop.Snapshot{
		Format: f.bd.FormatRevision(),
		Image: workshop.BaseImage{
			Name:        "ubuntu@24.04",
			Fingerprint: "0b9429c9855cb158b90159bb818e6f98eab9b5b1260ace11b30ddb936e4f78979abc7cdc5e4e9fad51e3e290a2190ac2",
		},
		Sdks: []sdk.ContentID{{
			Name:     "system",
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			IsVolume: true,
		}},
	}
	digest, err := f.bd.HashSnapshot(snapshot)
	c.Assert(err, check.IsNil)

	err = f.bd.TakeSnapshot(f.ctx, "test", snapshot)
	c.Assert(err, check.IsNil)
	defer func() { _ = f.bd.RemoveSnapshot(f.ctx, snapshot) }()

	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()
	snapshotConn := conn.UseProject("workshop-snapshots." + f.usr.Username)

	inst, etag, err := snapshotConn.GetInstance("system-" + digest[:16])
	c.Assert(err, check.IsNil)
	inst.Config[workshop.ConfigWorkshopBase] = "ubuntu@22.04"
	op, err := snapshotConn.UpdateInstance(inst.Name, inst.Writable(), etag)
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)

	_, err = f.bd.Snapshot(f.ctx, snapshot)
	c.Check(err, check.ErrorMatches, `hash collision detected: "system-.*" snapshot has "ubuntu@22.04" base; required: "ubuntu@24.04"`)

	inst.Config[workshop.ConfigWorkshopBase] = "ubuntu@24.04"
	inst.Devices[workshop.SdkDeviceName("system")]["user.sdk.sha3-384"] = "2870b68e0d49e89556af56a80c86e297a4af45242bf01b6ae57e56c443f08fd66b175d0284d49d20649676147817607e"
	op, err = snapshotConn.UpdateInstance(inst.Name, inst.Writable(), "")
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)

	err = f.bd.TakeSnapshot(f.ctx, "test", snapshot)
	c.Check(err, check.ErrorMatches, `hash collision detected: "system-.*" snapshot has unexpected revision of "system" SDK`)
}

func (f *wsOps) TestLxdBackendWorkshopCNAMEs(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Create second project.
	projectDir := filepath.Join(c.MkDir(), "testprj")
	createWFile(c, projectDir, "other", "name: other")
	restore := testutil.FakeFunc(func() (string, error) {
		return "24242424", nil
	}, &workshop.NewProjectId)
	_, _, err := f.bd.CreateOrLoadProject(f.ctx, projectDir)
	restore()
	c.Assert(err, check.IsNil)

	// Check initial CNAME.
	conn, err := f.bd.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)
	defer conn.Disconnect()

	network, _, err := conn.GetNetwork("workshopbr0")
	c.Assert(err, check.IsNil)
	want := `
cname=test.42424242.wp,test.testprj.wp,test-42424242.wp,0
`[1:]
	c.Check(network.Config["raw.dnsmasq"], check.Equals, want)

	// Check CNAME removed on stop.
	err = f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	network, etag, err := conn.GetNetwork(network.Name)
	c.Assert(err, check.IsNil)
	c.Check(network.Config["raw.dnsmasq"], check.Equals, "")

	// Test config merging.
	network.Config["raw.dnsmasq"] = `
# fake custom config line
cname=other.24242424.wp,other.testprj.wp,other-24242424.wp,0
# fake custom config line 2
`[1:]
	op, err := conn.UpdateNetwork(network.Name, network.Writable(), etag)
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)

	err = f.bd.StartWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	network, _, err = conn.GetNetwork(network.Name)
	c.Assert(err, check.IsNil)
	want = `
cname=other.24242424.wp,other.testprj.wp,other-24242424.wp,0
cname=test.42424242.wp,test-42424242.wp,0  # hostname-fallback
# fake custom config line
# fake custom config line 2
`[1:]
	c.Check(network.Config["raw.dnsmasq"], check.Equals, want)

	// Prune second project.
	err = os.RemoveAll(projectDir)
	c.Assert(err, check.IsNil)
	_, err = f.bd.Projects(f.ctx)
	c.Assert(err, check.IsNil)

	network, _, err = conn.GetNetwork(network.Name)
	c.Assert(err, check.IsNil)
	want = `
cname=test.42424242.wp,test-42424242.wp,0  # hostname-fallback
# fake custom config line
# fake custom config line 2
`[1:]
	c.Check(network.Config["raw.dnsmasq"], check.Equals, want)

	// Remove CNAME again.
	err = f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	network, etag, err = conn.GetNetwork(network.Name)
	c.Assert(err, check.IsNil)
	want = `
# fake custom config line
# fake custom config line 2
`[1:]
	c.Check(network.Config["raw.dnsmasq"], check.Equals, want)

	network.Config["raw.dnsmasq"] = ""
	op, err = conn.UpdateNetwork(network.Name, network.Writable(), etag)
	c.Assert(err, check.IsNil)
	c.Assert(op.Wait(), check.IsNil)
}
