// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3.

package sdkstore

import (
	"net/http"
	"net/url"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdkstore/path"
)

//go:generate mockgen -typed -package sdkstore -destination client_mock_test.go github.com/canonical/workshop/internal/https HTTPClient
//go:generate mockgen -typed -package sdkstore -destination http_mock_test.go github.com/canonical/workshop/internal/sdkstore RESTClient
//go:generate mockgen -typed -package sdkstore -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *testing.T) {
	check.TestingT(t)
}

func MustParseURL(c *check.C, path string) *url.URL {
	u, err := url.Parse(path)
	c.Assert(err, check.IsNil)
	return u
}

func MustMakePath(c *check.C, p string) path.Path {
	u := MustParseURL(c, p)
	return path.MakePath(u)
}

func MakeContentTypeHeader(name string) http.Header {
	h := make(http.Header)
	h.Set("content-type", name)
	return h
}

func MustNewRequest(c *check.C, path string) *http.Request {
	req, err := http.NewRequest("GET", path, nil)
	c.Assert(err, check.IsNil)

	return req
}
