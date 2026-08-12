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

package client

import (
	"bytes"
	"errors"
	"net/http"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
)

// workshopInstanceIDRoundTripperSuite tests Workshop instance-ID HTTP
// transport wrappers.
type workshopInstanceIDRoundTripperSuite struct{}

var _ = check.Suite(&workshopInstanceIDRoundTripperSuite{})

// TestAddsInstanceIDHeader verifies that a valid instance ID is sent to the
// next transport without changing the request.
func (workshopInstanceIDRoundTripperSuite) TestAddsInstanceIDHeader(
	c *check.C,
) {
	instanceID := "0123456789abcdef0123456789abcdef"
	wrapper := newWorkshopInstanceIDRoundTripper(func() (string, error) {
		return instanceID, nil
	}, nil)

	nextCalls := 0
	response := &http.Response{Body: http.NoBody, StatusCode: http.StatusOK}
	next := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		nextCalls++
		c.Check(req.Header.Get(workshopInstanceIDHeader), check.Equals, instanceID)
		return response, nil
	})
	req, err := http.NewRequest(http.MethodGet, "http://workshop.test", nil)
	c.Assert(err, check.IsNil)

	actual, err := wrapper(next).RoundTrip(req)

	c.Check(err, check.IsNil)
	c.Check(actual, check.Equals, response)
	c.Check(nextCalls, check.Equals, 1)
	// The wrapper must not modify the original request.
	c.Check(req.Header.Get(workshopInstanceIDHeader), check.Equals, "")
}

// TestContinuesWhenInstanceIDIsNotFound verifies that a missing instance ID
// emits a warning and leaves requests unchanged.
func (workshopInstanceIDRoundTripperSuite) TestContinuesWhenInstanceIDIsNotFound(
	c *check.C,
) {
	var warnings bytes.Buffer
	wrapper := newWorkshopInstanceIDRoundTripper(func() (string, error) {
		return "", osutil.ErrorWorkshopInstanceIDNotFound
	}, &warnings)

	nextCalls := 0
	response := &http.Response{Body: http.NoBody, StatusCode: http.StatusOK}
	next := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		nextCalls++
		c.Check(req.Header.Get(workshopInstanceIDHeader), check.Equals, "")
		return response, nil
	})
	req, err := http.NewRequest(http.MethodGet, "http://workshop.test", nil)
	c.Assert(err, check.IsNil)

	actual, err := wrapper(next).RoundTrip(req)

	c.Check(err, check.IsNil)
	c.Check(actual, check.Equals, response)
	c.Check(nextCalls, check.Equals, 1)
	c.Check(
		warnings.String(),
		check.Equals,
		"warning: cannot read workshop instance ID: workshop instance ID not found; ",
	)
}

// TestReturnsReadErrors verifies that an unexpected instance-ID read error
// prevents the request from reaching the next transport.
func (workshopInstanceIDRoundTripperSuite) TestReturnsReadErrors(
	c *check.C,
) {
	readError := errors.New("permission denied")
	wrapper := newWorkshopInstanceIDRoundTripper(func() (string, error) {
		return "", readError
	}, nil)

	nextCalls := 0
	next := RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		nextCalls++
		return nil, nil
	})
	req, err := http.NewRequest(http.MethodGet, "http://workshop.test", nil)
	c.Assert(err, check.IsNil)

	_, err = wrapper(next).RoundTrip(req)

	c.Check(err, check.ErrorMatches, "cannot read workshop instance ID: permission denied")
	c.Check(errors.Is(err, readError), check.Equals, true)
	c.Check(nextCalls, check.Equals, 0)
}
