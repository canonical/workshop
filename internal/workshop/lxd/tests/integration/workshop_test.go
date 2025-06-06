//go:build integration
// +build integration

package lxdbackend_integration_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"sync"
	"sync/atomic"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
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
	restoreLookupUsr   func()
	restoreNewId       func()
	restoreDevices     func()
	restoreImageServer func()
}

var _ = check.Suite(&wsOps{})

func (f *wsOps) SetUpSuite(c *check.C) {
	c.Assert(dirs.CreateDirs(), check.IsNil)

	var err error

	f.bd, err = lxdbackend.New()
	c.Assert(err, check.IsNil)

	f.usr = &user.User{Username: "testuser", Uid: "1000", Gid: "1000"}
	f.project = workshop.Project{
		ProjectId: "42424242",
		Path:      c.MkDir(),
	}
	f.ctx = helper.CreateTestContext(f.usr.Username, "42424242")

	f.restoreDevices = workshop.FakeDefaultDevices(helper.DefaultTestDevices)
	f.restoreImageServer = lxdbackend.FakeImageServer(helper.MinimalImageServer)
	f.restoreLookupUsr = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return f.usr, nil
	})
	f.restoreNewId = testutil.FakeFunc(func() (string, error) {
		return f.project.ProjectId, nil
	}, &workshop.NewProjectId)
}

func (f *wsOps) TearDownSuite(c *check.C) {
	lxdclient, err := f.bd.LxdClient(f.ctx)
	c.Check(err, check.IsNil)

	helper.CleanupLxdProject(c, lxdclient, "workshop."+f.usr.Username)
	helper.CleanupLxdProject(c, lxdclient, "workshop-stash."+f.usr.Username)
	f.restoreLookupUsr()
	f.restoreNewId()

	f.restoreDevices()
	f.restoreImageServer()

	err = os.RemoveAll(f.project.Path)
	c.Check(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopStashUnstash(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Execute
	err := f.bd.StashWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Execute
	err = f.bd.RemoveWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Execute
	err = f.bd.UnstashWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	err = f.bd.RemoveWorkshopStash(f.ctx, "test")
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopStashRemove(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Execute
	err := f.bd.StashWorkshop(f.ctx, "test")

	// Validate
	c.Assert(err, check.IsNil)
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Execute
	err = f.bd.RemoveWorkshopStash(f.ctx, "test")
	c.Assert(err, check.IsNil)

	// Validate
	err = f.bd.UnstashWorkshop(f.ctx, "test")
	c.Assert(err, check.ErrorMatches, "workshop not launched")
}

func (f *wsOps) TestLxdBackendStorageVolumeAddRemove(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	// Execute
	err := f.bd.CreateVolume(f.ctx, "test")

	// Validate
	c.Assert(err, check.IsNil)

	// Execute
	err = f.bd.DeleteVolume(f.ctx, "test")

	// Validate
	c.Assert(err, check.IsNil)
}

var testsdk = `name: test-sdk
title: title
base: ubuntu@20.04
version: '0.1.2'
summary: summary
description: SDK
sdkcraft-started-at: '2020-04-22T19:12:07.903032Z'
`

func (f *wsOps) TestLxdBackendStorageVolumeImportOK(c *check.C) {
	// Execute
	sdkfs := c.MkDir()
	tarball := helper.MockSdkTarball(c, "test", sdkfs, testsdk)

	cmd := testutil.FakeCommand(c, "tar", `/usr/bin/tar "$@"`)

	var wg sync.WaitGroup

	var successCnt, existCnt int32
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			err := f.bd.ImportVolume(f.ctx, "test-1", tarball)
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
				atomic.AddInt32(&existCnt, 1)
			} else {
				c.Logf("unexpected error: %v", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(4))

	c.Check(cmd.Calls(), check.HasLen, 2)

	vinfo, err := f.bd.Volume(f.ctx, "test-1")
	c.Check(err, check.IsNil)
	c.Check(vinfo.Name, check.Equals, "test-1")
	c.Check(vinfo.Config, check.DeepEquals, map[string]string{"user.sdk.meta": testsdk})

	err = f.bd.DeleteVolume(f.ctx, "test-1")
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendStorageVolumeImportInterrupted(c *check.C) {
	// Execute
	sdkfs := c.MkDir()
	tarball := helper.MockSdkTarball(c, "test", sdkfs, testsdk)

	cmd := testutil.FakeCommand(c, "tar", `/usr/bin/tar "$@"`)

	var wg sync.WaitGroup

	var successCnt, existCnt, canceled int32
	wg.Add(5)
	for i := 0; i < 5; i++ {
		newctx, cancel := context.WithCancel(f.ctx)

		if i%2 == 0 {
			cancel()
		} else {
			defer cancel()
		}

		go func() {
			err := f.bd.ImportVolume(newctx, "test-1", tarball)
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else if errors.Is(err, workshop.ErrVolumeAlreadyExists) {
				atomic.AddInt32(&existCnt, 1)
			} else if errors.Is(err, context.Canceled) {
				atomic.AddInt32(&canceled, 1)
			} else {
				c.Logf("unexpected error: %v", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	c.Check(atomic.LoadInt32(&successCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&existCnt), check.Equals, int32(1))
	c.Check(atomic.LoadInt32(&canceled), check.Equals, int32(3))

	c.Check(cmd.Calls(), check.HasLen, 2)

	vinfo, err := f.bd.Volume(f.ctx, "test-1")
	c.Check(err, check.IsNil)
	c.Check(vinfo.Name, check.Equals, "test-1")
	c.Check(vinfo.Config, check.DeepEquals, map[string]string{"user.sdk.meta": testsdk})

	err = f.bd.DeleteVolume(f.ctx, "test-1")
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendDeleteWorkshop(c *check.C) {
	// Execute
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)

	// Validate
	err := f.bd.RemoveWorkshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	_, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, testutil.ErrorIs, workshop.ErrWorkshopNotLaunched)
}

func (f *wsOps) image(c *check.C, alias string) (string, error) {
	cli, err := f.bd.LxdClient(f.ctx)
	c.Check(err, check.IsNil)
	entry, _, err := cli.GetImageAlias(lxdbackend.ImageAlias(alias))
	if err != nil {
		return "", err
	}
	return entry.Target, err
}

func (f *wsOps) deleteimage(c *check.C, fp string) error {
	cli, err := f.bd.LxdClient(f.ctx)
	c.Check(err, check.IsNil)
	op, err := cli.DeleteImage(fp)
	c.Check(err, check.IsNil)
	return op.Wait()
}

func (f *wsOps) TestLxdBackendDownloadWorkshopBase(c *check.C) {
	// ensure there is no image in LXD storage
	fp, err := f.image(c, "ubuntu@22.04")
	if err == nil {
		c.Assert(f.deleteimage(c, fp), check.IsNil)
	}

	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			err := f.bd.Download(f.ctx, "ubuntu@22.04", nil)
			c.Check(err, check.IsNil)
			wg.Done()
		}()
	}
	wg.Wait()
	fp, err = f.image(c, "ubuntu@22.04")
	c.Assert(err, check.IsNil)
	c.Assert(f.deleteimage(c, fp), check.IsNil)
}
func (f *wsOps) TestLxdBackendDownloadMalformedBase(c *check.C) {
	err := f.bd.Download(f.ctx, "ubuntu:24.04", nil)
	c.Check(err, check.ErrorMatches, `"ubuntu:24.04" is not a correct base name`)
	err = f.bd.Download(f.ctx, "ubuntu@", nil)
	c.Check(err, check.ErrorMatches, `"ubuntu@" is not a correct base name`)
}

func (f *wsOps) TestLxdBackendDownloadBaseImageNotFound(c *check.C) {
	err := f.bd.Download(f.ctx, "ubuntu@1.01", nil)
	c.Check(err, check.ErrorMatches, `"ubuntu@1.01" download failed.*`)
}

func (f *wsOps) TestLxdBackendDownloadProtocolNotSupported(c *check.C) {
	defer lxdbackend.FakeImageServer("https://cloud-images.ubuntu.com/minimal/releases/")()
	err := f.bd.Download(f.ctx, "ubuntu@20.04", nil)
	c.Check(err, check.ErrorMatches, `unknown image server URL prefix \(supported: simplestreams, lxd\)`)
}

func (f *wsOps) TestLxdBackendDownloadWorkshopBaseResumeAfterCancellation(c *check.C) {
	// ensure there is no image in LXD storage
	fp, err := f.image(c, "ubuntu@22.04")
	if err == nil {
		c.Assert(f.deleteimage(c, fp), check.IsNil)
	}

	wcancel, cancel := context.WithCancel(f.ctx)
	defer cancel()

	var wg sync.WaitGroup
	var once sync.Once
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			r := &progress.Reporter{
				Name: "1",
				Report: func(label string, done, total int) {
					once.Do(func() { cancel() })
				},
			}
			err := f.bd.Download(wcancel, "ubuntu@22.04", r)
			c.Check(err, testutil.ErrorIs, context.Canceled)
			wg.Done()
		}()
	}
	wg.Wait()

	// attempt to download after interruption (must pickup an ongoing operation
	// and wait for it).
	err = f.bd.Download(f.ctx, "ubuntu@22.04", nil)
	c.Assert(err, check.IsNil)

	fp, err = f.image(c, "ubuntu@22.04")
	c.Assert(err, check.IsNil)
	c.Assert(f.deleteimage(c, fp), check.IsNil)
}

func (f *wsOps) TestLxdBackendDownloadMultipleBasesConcurrently(c *check.C) {
	// ensure there is no image in LXD storage
	for _, b := range workshop.SupportedBases {
		fp, err := f.image(c, b)
		if err == nil {
			c.Assert(f.deleteimage(c, fp), check.IsNil)
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(workshop.SupportedBases))
	for i := 0; i < len(workshop.SupportedBases); i++ {
		idx := i
		go func() {
			err := f.bd.Download(f.ctx, workshop.SupportedBases[idx], nil)
			c.Check(err, check.IsNil)
			wg.Done()
		}()
	}
	wg.Wait()

	for _, b := range workshop.SupportedBases {
		fp, err := f.image(c, b)
		c.Assert(err, check.IsNil)
		c.Assert(f.deleteimage(c, fp), check.IsNil)
	}
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

func (f *wsOps) TestLxdBackendWorkshopRebuild(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	err := f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@24.04"}

	// Execute
	err = f.bd.LaunchOrRebuildWorkshop(f.ctx, wf)
	c.Assert(err, check.IsNil)
}

func (f *wsOps) TestLxdBackendWorkshopRestore(c *check.C) {
	helper.LaunchTestWorkshop(c, f.ctx, f.bd, f.project.Path)
	defer helper.RemoveTestWorkshop(c, f.ctx, f.bd)

	w, err := f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)

	setup := sdk.Setup{Name: sdk.System.String(), Revision: system.SystemSdkRevision}
	err = system.RetrieveSystemSdk(setup, nil)
	c.Assert(err, check.IsNil)

	volume := sdk.VolumeName(setup.Name, setup.Revision)
	err = f.bd.ImportVolume(f.ctx, volume, setup.Filepath())
	c.Assert(err, check.IsNil)

	err = f.bd.AttachVolume(f.ctx, "test", volume, sdk.SdkDir(setup.Name), true)
	c.Assert(err, check.IsNil)

	err = w.AddSdk(f.ctx, setup)
	c.Assert(err, check.IsNil)
	c.Check(w.Sdks, check.HasLen, 1)

	err = f.bd.StopWorkshop(f.ctx, "test", true)
	c.Assert(err, check.IsNil)

	err = f.bd.Snapshot(f.ctx, "test", "snapshot-1")
	c.Assert(err, check.IsNil)

	err = f.bd.AttachVolume(f.ctx, "test", volume, sdk.SdkDir("sdk"), true)
	c.Assert(err, check.IsNil)

	err = w.AddSdk(f.ctx, sdk.Setup{Name: "sdk", Revision: sdk.R(5)})
	c.Assert(err, check.IsNil)
	c.Check(w.Sdks, check.HasLen, 2)

	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@24.04",
	}
	// The instance's configuration only has the system SDK after this.
	err = f.bd.Restore(f.ctx, "test", "snapshot-1", wf)
	c.Assert(err, check.IsNil)

	w, err = f.bd.Workshop(f.ctx, "test")
	c.Assert(err, check.IsNil)
	c.Check(w.Running, check.Equals, false)
	// Check that Restore uses the provided "user.workshop.file" and removes "sdk".
	c.Check(w.File, check.DeepEquals, wf)
	c.Check(w.Sdks, check.HasLen, 1)

	fs, err := f.bd.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer fs.Close()
	_, err = fs.Stat(sdk.SdkDir("sdk"))
	c.Check(err, testutil.ErrorIs, os.ErrNotExist)
}
