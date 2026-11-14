// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3.

package sdkstore

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"go.uber.org/mock/gomock"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdkstore/path"
	"github.com/canonical/workshop/internal/sdkstore/transport"
	"github.com/canonical/workshop/internal/testutil"
)

//go:embed test-resolve-name.raw.json
var testResolveNameRaw []byte

//go:embed test-resolve-name.json
var testResolveNameResponse []byte

//go:embed test-resolve-id.raw.json
var testResolveIDRaw []byte

//go:embed test-resolve-not-found.raw.json
var testResolveNotFoundRaw []byte

type ResolveSuite struct{}

var _ = check.Suite(&ResolveSuite{})

func (s *ResolveSuite) TestResolve(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)

	restClient := NewMockRESTClient(ctrl)
	s.expectPost(restClient, path)

	client := newResolveClient(path, restClient)
	response, err := client.Resolve(context.Background(), transport.ResolveRequest{})
	c.Assert(err, check.IsNil)
	c.Assert(response.PackageResults, check.HasLen, 1)
	c.Check(response.PackageResults[0], check.DeepEquals, transport.ResolvePackageResponse{ID: "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1"})
}

func (s *ResolveSuite) TestResolveFailure(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)

	restClient := NewMockRESTClient(ctrl)
	s.expectPostFailure(restClient)

	client := newResolveClient(path, restClient)
	_, err := client.Resolve(context.Background(), transport.ResolveRequest{})
	c.Assert(err, check.ErrorMatches, "boom")
}

func (s *ResolveSuite) TestResolveRetriesAfterTimeout(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	p := path.MakePath(baseURL)

	restClient := NewMockRESTClient(ctrl)

	gomock.InOrder(
		restClient.EXPECT().Post(gomock.Any(), p, gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{}, timeoutError{}),
		restClient.EXPECT().Post(gomock.Any(), p, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ path.Path, _ http.Header, _ any, result any) (restResponse, error) {
				result.(*resolveResponse).PackageResults = []transport.ResolvePackageResponse{{ID: "id"}}
				return restResponse{}, nil
			}),
	)

	clockMock := NewMockClock(ctrl)
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	clockMock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clockMock.EXPECT().After(defaultRetryDelay).Return(ch).Times(1)

	client := newResolveClient(p, restClient)
	client.clock = clockMock

	response, err := client.Resolve(context.Background(), transport.ResolveRequest{})
	c.Assert(err, check.IsNil)
	c.Check(response.PackageResults, check.DeepEquals, []transport.ResolvePackageResponse{{ID: "id"}})
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func (s *ResolveSuite) expectPost(client *MockRESTClient, p path.Path) {
	client.EXPECT().Post(gomock.Any(), p, gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ path.Path, _ http.Header, _, r any) (restResponse, error) {
		resp := transport.ResolveResponse{
			PackageResults: []transport.ResolvePackageResponse{{
				ID: "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			}},
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return restResponse{}, err
		}
		err = json.Unmarshal(data, r)
		return restResponse{}, err
	})
}

func (s *ResolveSuite) expectPostFailure(client *MockRESTClient) {
	client.EXPECT().Post(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{StatusCode: http.StatusInternalServerError}, errors.New("boom"))
}

func (s *ResolveSuite) TestResolvePayload(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.URL.Path, check.Equals, "/v2/revisions/resolve")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(testResolveNameRaw)
		c.Check(err, check.IsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	resolvePath := resolvePath(MustParseURL(c, server.URL))

	apiRequester := newAPIRequester(DefaultHTTPClient())
	restClient := newHTTPRESTClient(apiRequester)

	client := newResolveClient(resolvePath, restClient)
	req := transport.ResolveRequest{
		Packages: []transport.ResolvePackage{{
			InstanceKey: "random123",
			Namespace:   "sdk",
			Name:        "test-sdk-info-multi-base",
			Channel:     "latest/stable",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "22.04",
				Architecture: "amd64",
			},
		}},
	}
	response, err := client.Resolve(context.Background(), req)
	c.Assert(err, check.IsNil)

	var expected any
	err = json.Unmarshal(testResolveNameResponse, &expected)
	c.Assert(err, check.IsNil)
	c.Check(response, testutil.JsonEquals, expected)
}
