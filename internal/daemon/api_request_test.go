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

package daemon

import (
	"net/http"
	"net/http/httptest"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/workshop"
)

type apiRequestSuite struct{}

var _ = check.Suite(&apiRequestSuite{})

// TestWithWorkshopInstanceIDAddsIDToContext checks that the middleware copies
// the workshop instance ID header into the request context before calling the
// next response function. It asserts that the next function's response is
// returned and that the context contains the header value.
func (apiRequestSuite) TestWithWorkshopInstanceIDAddsIDToContext(c *check.C) {
	request := httptest.NewRequest(http.MethodPost, "/v1/workshopctl", nil)
	request.Header.Set(workshopInstanceIDHeader, "instance-id")

	var instanceID string
	next := func(_ *Command, request *http.Request, _ *userState) Response {
		instanceID, _ = request.Context().
			Value(workshop.ContextWorkshopInstanceID).(string)
		return nil
	}

	response := withWorkshopInstanceID(next)(nil, request, nil)

	c.Check(response, check.IsNil)
	c.Check(instanceID, check.Equals, "instance-id")
}

// TestWithWorkshopInstanceIDLeavesContextUnsetWithoutHeader checks that the
// middleware calls the next response function without adding a workshop
// instance ID when the header is absent. It asserts that the next function's
// response is returned and that the context value remains unset.
func (apiRequestSuite) TestWithWorkshopInstanceIDLeavesContextUnsetWithoutHeader(
	c *check.C,
) {
	request := httptest.NewRequest(http.MethodPost, "/v1/workshopctl", nil)

	var instanceID any
	next := func(_ *Command, request *http.Request, _ *userState) Response {
		instanceID = request.Context().Value(workshop.ContextWorkshopInstanceID)
		return nil
	}

	response := withWorkshopInstanceID(next)(nil, request, nil)

	c.Check(response, check.IsNil)
	c.Check(instanceID, check.IsNil)
}

// TestWithWorkshopInstanceIDCallsNextWithHeader checks that the middleware
// calls the next response function and returns its response when the workshop
// instance ID header is present.
func (apiRequestSuite) TestWithWorkshopInstanceIDCallsNextWithHeader(c *check.C) {
	command := &Command{}
	request := httptest.NewRequest(http.MethodPost, "/v1/workshopctl", nil)
	request.Header.Set(workshopInstanceIDHeader, "instance-id")
	user := &userState{}
	expected := SyncResponse(nil, http.StatusAccepted)

	called := false
	next := func(
		actualCommand *Command,
		_ *http.Request,
		actualUser *userState,
	) Response {
		called = true
		c.Check(actualCommand, check.Equals, command)
		c.Check(actualUser, check.Equals, user)
		return expected
	}

	response := withWorkshopInstanceID(next)(command, request, user)

	c.Check(called, check.Equals, true)
	c.Check(response, check.Equals, expected)
}

// TestWithWorkshopInstanceIDCallsNextWithoutHeader checks that the middleware
// calls the next response function and returns its response when the workshop
// instance ID header is absent.
func (apiRequestSuite) TestWithWorkshopInstanceIDCallsNextWithoutHeader(c *check.C) {
	command := &Command{}
	request := httptest.NewRequest(http.MethodPost, "/v1/workshopctl", nil)
	user := &userState{}
	expected := SyncResponse(nil, http.StatusAccepted)

	called := false
	next := func(
		actualCommand *Command,
		actualRequest *http.Request,
		actualUser *userState,
	) Response {
		called = true
		c.Check(actualCommand, check.Equals, command)
		c.Check(actualRequest, check.Equals, request)
		c.Check(actualUser, check.Equals, user)
		return expected
	}

	response := withWorkshopInstanceID(next)(command, request, user)

	c.Check(called, check.Equals, true)
	c.Check(response, check.Equals, expected)
}
