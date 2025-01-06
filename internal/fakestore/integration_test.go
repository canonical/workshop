//go:build integration
// +build integration

package store_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	store "github.com/canonical/workshop/internal/fakestore"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type storeIntegration struct {
	oldRoot string
}

var _ = check.Suite(&storeIntegration{})

func (f *storeIntegration) SetUpSuite(c *check.C) {
	c.Assert(os.Setenv("SDK_STORE_URL", "http://localhost:8080/storage/v1/"), check.IsNil)
	f.oldRoot = dirs.BaseDir
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)
}

func (f *storeIntegration) TearDownSuite(c *check.C) {
	c.Assert(os.Unsetenv("SDK_STORE_URL"), check.IsNil)
	dirs.SetRootDir(f.oldRoot)
}

func (f *storeIntegration) TestStoreDownloadOK(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable"}
	err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.IsNil)
	c.Assert(setup.Filepath(), testutil.FilePresent)
	c.Assert(os.Remove(setup.Filepath()), check.IsNil)
}

func (f *storeIntegration) TestStoreDownloadProgressReport(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable"}
	done, total := 0, 0
	r := &progress.Reporter{Name: "1", Report: func(label string, d, t int) {
		done += d
		total = t
	}}
	err := s.DownloadSdk(context.Background(), setup, r)
	c.Assert(err, check.IsNil)
	c.Assert(setup.Filepath(), testutil.FilePresent)
	c.Check(done > 0, check.Equals, true)
	c.Check(total > 0, check.Equals, true)
	c.Check(done == total, check.Equals, true)
	c.Assert(os.Remove(setup.Filepath()), check.IsNil)
}

func (f *storeIntegration) TestStoreDownloadCleanupPrevious(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.Revision{N: 1}}
	prev := filepath.Join(dirs.SdkDownloads, setup.Name) + "_5.sdk"
	_, err := os.Create(prev)
	c.Assert(err, check.IsNil)
	err = s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.IsNil)
	c.Assert(setup.Filepath(), testutil.FilePresent)
	c.Assert(prev, testutil.FileAbsent)
	c.Assert(os.Remove(setup.Filepath()), check.IsNil)
}

func (f *storeIntegration) TestStoreDownloadNotfound(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-unknown", Channel: "latest/stable"}
	err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.ErrorMatches, `SDK not found in "latest/stable"`)
	c.Assert(setup.Filepath(), testutil.FileAbsent)
}

func (f *storeIntegration) TestStoreDownloadLocksSDKForExclusiveAccess(c *check.C) {
	os.Setenv("WORKSHOP_DEBUG", "1")
	defer os.Unsetenv("WORKSHOP_DEBUG")

	m, r := logger.MockLogger()
	defer r()

	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.Revision{N: 1}}

	// Lock the file to emulate a concurrent download is going on
	target := setup.Filepath()
	fl, err := sdk.OpenLock(setup.Name)
	c.Assert(err, check.IsNil)
	c.Assert(fl.Lock(), check.IsNil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.DownloadSdk(context.Background(), setup, nil)
		c.Check(err, check.IsNil)
		c.Check(m.String(), check.Matches, fmt.Sprintf(`(?s)*.DEBUG: SDK Store on Download: SDK "test-sdk-basic" found locally: %s/test-sdk-basic_1.sdk.*`, dirs.SdkDownloads))
		wg.Done()
	}()

	// "download" is finished
	_, err = os.Create(target)
	c.Assert(err, check.IsNil)
	fl.Close()
	wg.Wait()
}

type failingReader struct{}

func (*failingReader) Read(p []byte) (n int, err error) {
	return 0, context.DeadlineExceeded
}
func (*failingReader) Close() error { return nil }

func (f *storeIntegration) TestStoreDownloadRemoveUnfinished(c *check.C) {
	r := store.FakeSdkStoreSdkReader(func(ctx context.Context, setup sdk.Setup) (io.ReadCloser, error) {
		return &failingReader{}, nil
	})
	defer r()

	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.Revision{N: 55}}
	err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.NotNil)
	c.Assert(setup.Filepath(), testutil.FileAbsent)
}

func (s *storeIntegration) TestSdkActionInstallStoreError(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	store := store.New()
	acts := []sdk.SdkAction{{
		ProjectId: "24242424",
		Workshop:  "test-workshop",
		Action:    sdk.Install,
		Name:      "test-sdk-unknown",
		Channel:   "latest/stable",
	}}
	res, err := store.SdkAction(context.Background(), acts)
	c.Assert(res, check.HasLen, 0)
	c.Assert(err, check.ErrorMatches, `(?s).*SDK not found in "latest/stable".*`)
}

func (f *storeIntegration) TestSdkActionTimeout(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	// Deliberately set to malformed address
	c.Assert(os.Setenv("SDK_STORE_URL", "http://localhost:8181/storage/v1/"), check.IsNil)
	s := store.New()
	acts := []sdk.SdkAction{{
		ProjectId: "24242424",
		Workshop:  "test-workshop",
		Action:    sdk.Install,
		Name:      "test-sdk-unknown",
		Channel:   "latest/stable",
	}}
	_, err := s.SdkAction(context.Background(), acts)

	// Restore address for remaining tests
	c.Assert(os.Setenv("SDK_STORE_URL", "http://localhost:8080/storage/v1/"), check.IsNil)

	c.Assert(err, check.ErrorMatches, `(?s).*cannot connect to store.*`)
}
