package store_test

import (
	"context"
	"testing"

	"gopkg.in/check.v1"

	store "github.com/canonical/workshop/internal/fakestore"
	"github.com/canonical/workshop/internal/sdk"
)

type storeSuite struct{}

var _ = check.Suite(&storeSuite{})

func Test(t *testing.T) {
	check.TestingT(t)
}

var testSdk = `name: test-sdk
base: ubuntu@20.04
license: LGPL-2.1
summary: The Go programming language
description: |
  Go is an open source programming language that enables
  the production of simple, efficient and reliable software at scale.
plugs:
  data:
    interface: mount
    workshop-target: /test
  gpu:
    interface: gpu
`

var testSdkNoBase = `name: test-sdk
`

func (s *storeSuite) TestSdkActionInstallOK(c *check.C) {
	r := store.FakeSdkStoreInfo(func(ctx context.Context, name, channel string) (store.StoreSdk, error) {
		s := store.StoreSdk{
			Name:     "test-sdk",
			Channel:  channel,
			Revision: sdk.Revision{N: 123},
			SdkYAML:  testSdk,
		}
		return s, nil
	})
	defer r()
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	store := store.New()
	acts := []sdk.SdkAction{
		{
			ProjectId: "24242424",
			Workshop:  "test-workshop",
			Action:    sdk.Install,
			Name:      "test-sdk",
			Base:      "ubuntu@20.04",
			Channel:   "latest/stable",
		},
	}
	res, err := store.SdkAction(context.Background(), acts)
	c.Assert(res, check.HasLen, 1)
	c.Assert(err, check.IsNil)
}

func (s *storeSuite) TestSdkActionInstallNoBase(c *check.C) {
	r := store.FakeSdkStoreInfo(func(ctx context.Context, name, channel string) (store.StoreSdk, error) {
		s := store.StoreSdk{
			Name:     "test-sdk",
			Channel:  channel,
			Revision: sdk.Revision{N: 123},
			SdkYAML:  testSdkNoBase,
		}
		return s, nil
	})
	defer r()
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	store := store.New()
	acts := []sdk.SdkAction{
		{
			ProjectId: "24242424",
			Workshop:  "test-workshop",
			Action:    sdk.Install,
			Name:      "test-sdk",
			Base:      "ubuntu@20.04",
			Channel:   "latest/stable",
		},
	}
	res, err := store.SdkAction(context.Background(), acts)
	c.Assert(res, check.HasLen, 1)
	c.Assert(err, check.IsNil)
}
