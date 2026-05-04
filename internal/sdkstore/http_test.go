// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3.

package sdkstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.uber.org/mock/gomock"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/https"
)

type APIRequesterSuite struct{}

var _ = check.Suite(&APIRequesterSuite{})

func (s *APIRequesterSuite) TestDo(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient)
	resp, err := requester.Do(req)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, check.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoWithFailure(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), errors.New("boom")) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient)
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, check.NotNil)
}

func (s *APIRequesterSuite) TestDoWithInvalidContentType(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(invalidContentTypeResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient)
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, check.NotNil)
}

func (s *APIRequesterSuite) TestDoWithNotFoundResponse(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(notFoundResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient)
	resp, err := requester.Do(req)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, check.Equals, http.StatusNotFound)
}

func (s *APIRequesterSuite) TestDoRetrySuccess(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, check.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetrySuccessBody(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("POST", "http://api.foo.bar", strings.NewReader("body"))
	c.Assert(err, check.IsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		body, err := req.GetBody()
		c.Assert(err, check.IsNil)
		b, err := io.ReadAll(body)
		c.Assert(err, check.IsNil)
		c.Assert(string(b), check.Equals, "body")
		return nil, io.EOF
	})
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		body, err := req.GetBody()
		c.Assert(err, check.IsNil)
		b, err := io.ReadAll(body)
		c.Assert(err, check.IsNil)
		c.Assert(string(b), check.Equals, "body")
		return emptyResponse(), nil //nolint:bodyclose
	})

	requester := newAPIRequester(mockHTTPClient)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, check.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetryMaxAttempts(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)

	start := time.Now()
	requester := newAPIRequester(mockHTTPClient)
	requester.retryDelay = time.Microsecond
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, check.ErrorMatches, `attempt count exceeded: EOF`)
	elapsed := time.Since(start)
	c.Assert(elapsed >= (1+2+4)*time.Microsecond, check.Equals, true)
}

func (s *APIRequesterSuite) TestDoRetryContextCanceled(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel right away
	req, err := http.NewRequestWithContext(ctx, "GET", "http://api.foo.bar", nil)
	c.Assert(err, check.IsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)

	start := time.Now()
	requester := newAPIRequester(mockHTTPClient)
	requester.retryDelay = time.Second
	_, err = requester.Do(req) //nolint:bodyclose
	c.Assert(err, check.ErrorMatches, `retry stopped`)
	elapsed := time.Since(start)
	c.Assert(elapsed < 250*time.Millisecond, check.Equals, true)
}

type RESTSuite struct{}

var _ = check.Suite(&RESTSuite{})

var retryPolicy = https.RetryPolicy{
	Attempts: 3,
	Delay:    50 * time.Millisecond,
	MaxDelay: 10 * time.Second,
}

func (s *RESTSuite) TestGet(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var receivedURL string

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		receivedURL = req.URL.String()
		return emptyResponse(), nil
	}) //nolint:bodyclose

	base := MustMakePath(c, "http://api.foo.bar")

	client := newHTTPRESTClient(mockHTTPClient)

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.IsNil)
	c.Assert(receivedURL, check.Equals, "http://api.foo.bar")
}

func (s *RESTSuite) TestGetWithInvalidContext(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result any
	_, err := client.Get(nil, base, &result) //nolint:staticcheck // Deliberately nil Context.
	c.Assert(err, check.NotNil)
}

func (s *RESTSuite) TestGetWithFailure(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(emptyResponse(), errors.New("boom")) //nolint:bodyclose

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.NotNil)
}

func (s *RESTSuite) TestGetWithFailureRetry(c *check.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	httpClient := https.NewClient(https.WithRequestRetrier(retryPolicy))
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.NotNil)
	c.Assert(called, check.Equals, 3)
}

func (s *RESTSuite) TestGetWithFailureWithoutRetry(c *check.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	httpClient := https.NewClient(https.WithRequestRetrier(retryPolicy))
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.NotNil)
	c.Assert(called, check.Equals, 1)
}

func (s *RESTSuite) TestGetWithNoRetry(c *check.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	httpClient := https.NewClient(https.WithRequestRetrier(retryPolicy))
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, 1)
}

func (s *RESTSuite) TestGetWithUnmarshalFailure(c *check.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(invalidResponse(), nil) //nolint:bodyclose

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result any
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, check.NotNil)
}

func emptyResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{}")),
	}
}

func invalidResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("/\\!")),
	}
}

func invalidContentTypeResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("text/plain"),
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewBufferString("")),
	}
}

func notFoundResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusNotFound,
		Body: io.NopCloser(bytes.NewBufferString(`
{
	"code":"404",
	"message":"not-found"
}
		`)),
	}
}
