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

// machineIDRoundTripperSuite tests machine-ID HTTP transport wrappers.
type machineIDRoundTripperSuite struct{}

var _ = check.Suite(&machineIDRoundTripperSuite{})

// TestAddsMachineIDHeader verifies that a valid machine ID is sent to the next
// transport without changing the request.
func (machineIDRoundTripperSuite) TestAddsMachineIDHeader(
	c *check.C,
) {
	machineID := "0123456789abcdef0123456789abcdef"
	wrapper := newMachineIDRoundTripper(func() (string, error) {
		return machineID, nil
	}, nil)

	nextCalls := 0
	response := &http.Response{Body: http.NoBody, StatusCode: http.StatusOK}
	next := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		nextCalls++
		c.Check(req.Header.Get(workshopMachineIDHeader), check.Equals, machineID)
		return response, nil
	})
	req, err := http.NewRequest(http.MethodGet, "http://workshop.test", nil)
	c.Assert(err, check.IsNil)

	actual, err := wrapper(next).RoundTrip(req)

	c.Check(err, check.IsNil)
	c.Check(actual, check.Equals, response)
	c.Check(nextCalls, check.Equals, 1)
	// The wrapper must not modify the original request.
	c.Check(req.Header.Get(workshopMachineIDHeader), check.Equals, "")
}

// TestContinuesWhenMachineIDIsNotFound verifies that a missing machine ID
// emits a warning and leaves requests unchanged.
func (machineIDRoundTripperSuite) TestContinuesWhenMachineIDIsNotFound(
	c *check.C,
) {
	var warnings bytes.Buffer
	wrapper := newMachineIDRoundTripper(func() (string, error) {
		return "", osutil.ErrorMachineIDNotFound
	}, &warnings)

	nextCalls := 0
	response := &http.Response{Body: http.NoBody, StatusCode: http.StatusOK}
	next := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		nextCalls++
		c.Check(req.Header.Get(workshopMachineIDHeader), check.Equals, "")
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
		"warning: cannot read workshop machine ID: machine ID not found; ",
	)
}

// TestReturnsReadErrors verifies that an unexpected machine-ID read error
// prevents the request from reaching the next transport.
func (machineIDRoundTripperSuite) TestReturnsReadErrors(
	c *check.C,
) {
	readError := errors.New("permission denied")
	wrapper := newMachineIDRoundTripper(func() (string, error) {
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

	c.Check(err, check.ErrorMatches, "cannot read machine ID: permission denied")
	c.Check(errors.Is(err, readError), check.Equals, true)
	c.Check(nextCalls, check.Equals, 0)
}
