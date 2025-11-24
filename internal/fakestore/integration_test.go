//go:build integration

package store_test

import (
	"context"
	"fmt"
	"os"
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
	oldRoot  string
	oldCache string
}

var _ = check.Suite(&storeIntegration{})

func (f *storeIntegration) SetUpTest(c *check.C) {
	c.Assert(os.Setenv("SDK_STORE_URL", "http://localhost:8080/storage/v1/"), check.IsNil)
	f.oldRoot = dirs.BaseDir
	f.oldCache = dirs.CacheDir
	dirs.SetRootDir(c.MkDir())
	dirs.SetCacheDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)
}

func (f *storeIntegration) TearDownTest(c *check.C) {
	c.Assert(os.Unsetenv("SDK_STORE_URL"), check.IsNil)
	dirs.SetCacheDir(f.oldCache)
	dirs.SetRootDir(f.oldRoot)
}

func (f *storeIntegration) TestStoreDownloadOK(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.R(1)}
	result, err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.IsNil)
	setup.Revision = result.Revision
	setup.Sha3_384 = result.Sha3_384
	c.Check(result.Setup, check.Equals, setup)
	c.Check(result.Revision, check.Not(check.Equals), sdk.R(0))
	c.Check(result.Sha3_384, check.Not(check.Equals), "")
	c.Check(result.SdkYAML, check.Not(check.Equals), "")
	c.Assert(result.Filepath(), testutil.FilePresent)
}

func (f *storeIntegration) TestStoreDownloadProgressReport(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.R(1)}
	done, total := 0, 0
	r := &progress.Reporter{Name: "1", Report: func(label string, d, t int) {
		done = d
		total = t
	}}
	result, err := s.DownloadSdk(context.Background(), setup, r)
	c.Assert(err, check.IsNil)
	c.Assert(result.Filepath(), testutil.FilePresent)
	c.Check(done, testutil.IntGreaterThan, 0)
	c.Check(total, testutil.IntGreaterThan, 0)
	c.Check(done, check.Equals, total)
}

func (f *storeIntegration) TestStoreDownloadCleanupPrevious(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.R(1)}
	prev := setup
	prev.Revision = sdk.R(5)
	c.Assert(os.WriteFile(prev.Filepath(), nil, 0o644), check.IsNil)
	other := setup
	other.Revision = sdk.R(2)
	c.Assert(os.WriteFile(other.Filepath(), nil, 0o644), check.IsNil)

	result, err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.IsNil)
	c.Check(result.Filepath(), testutil.FilePresent)
	if prev.Revision != result.Revision {
		c.Check(prev.Filepath(), testutil.FileAbsent)
	}
	if other.Revision != result.Revision {
		c.Check(other.Filepath(), testutil.FileAbsent)
	}
}

func (f *storeIntegration) TestStoreDownloadNotfound(c *check.C) {
	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-unknown", Channel: "latest/stable", Revision: sdk.R(1)}
	_, err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.ErrorMatches, `SDK not found in "latest/stable"`)
	c.Check(dirs.SdkDownloads, testutil.DirEquals, []string{})
}

func (f *storeIntegration) TestStoreDownloadLocksSDKForExclusiveAccess(c *check.C) {
	os.Setenv("WORKSHOP_DEBUG", "1")
	defer os.Unsetenv("WORKSHOP_DEBUG")

	m, r := logger.MockLogger()
	defer r()

	s := store.New()
	setup := sdk.Setup{
		Name:     "test-sdk-basic",
		Channel:  "latest/stable",
		Revision: sdk.Revision{N: 1},
		Sha3_384: "19133c4b2d9157b9edf9128e583245a621f172725abb5965282c8b2d9dfa60eae592d08cb0a0a5d48205ce8f25d2be91",
	}

	// Lock the file to emulate a concurrent download is going on
	target := setup.Filepath()
	fl, err := sdk.OpenLock(setup.Name)
	c.Assert(err, check.IsNil)
	c.Assert(fl.Lock(), check.IsNil)

	var wg sync.WaitGroup
	wg.Go(func() {
		result, err := s.DownloadSdk(context.Background(), setup, nil)
		c.Assert(err, check.IsNil)
		c.Check(result.Setup, check.Equals, setup)
		c.Check(result.SdkYAML, check.Equals, "name: test-sdk-basic")
		c.Check(m.String(), check.Matches, fmt.Sprintf(`(?s)*.DEBUG: SDK Store on Download: SDK "test-sdk-basic" found locally: %s/test-sdk-basic_1.sdk.*`, dirs.SdkDownloads))
	})

	// "download" is finished
	c.Assert(os.WriteFile(target, nil, 0o666), check.IsNil)
	c.Assert(os.WriteFile(target+".sha3-384", []byte(setup.Sha3_384), 0o666), check.IsNil)
	c.Assert(os.WriteFile(target+".yaml", []byte("name: test-sdk-basic"), 0o666), check.IsNil)
	fl.Close()
	wg.Wait()
}

type failingReader struct{}

func (*failingReader) Read(p []byte) (n int, err error) {
	return 0, context.DeadlineExceeded
}
func (*failingReader) Close() error { return nil }

func (f *storeIntegration) TestStoreDownloadRemoveUnfinished(c *check.C) {
	r := store.FakeSdkStoreSdkReader(func(ctx context.Context, setup sdk.Setup) (*store.SdkReader, error) {
		return &store.SdkReader{ReadCloser: &failingReader{}, Revision: setup.Revision}, nil
	})
	defer r()

	s := store.New()
	setup := sdk.Setup{Name: "test-sdk-basic", Channel: "latest/stable", Revision: sdk.Revision{N: 55}}
	_, err := s.DownloadSdk(context.Background(), setup, nil)
	c.Assert(err, check.NotNil)
	c.Assert(dirs.SdkDownloads, testutil.DirEquals, []string{})
}

func (s *storeIntegration) TestSdkActionInstallStoreError(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	store := store.New()
	acts := []sdk.SdkAction{{
		ProjectId: "24242424",
		Workshop:  "test-workshop",
		Action:    sdk.Install,
		Name:      "test-sdk-unknown",
		Base:      "ubuntu@22.04",
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
		Base:      "ubuntu@22.04",
		Channel:   "latest/stable",
	}}
	_, err := s.SdkAction(context.Background(), acts)

	// Restore address for remaining tests
	c.Assert(os.Setenv("SDK_STORE_URL", "http://localhost:8080/storage/v1/"), check.IsNil)

	c.Assert(err, check.ErrorMatches, `(?s).*cannot connect to store.*`)
}
