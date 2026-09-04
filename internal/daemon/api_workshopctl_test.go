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
	"bytes"
	"context"
	"net/http"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

func (s *apiSuite) addWorkshopWithInstanceID(instanceID string) {
	s.b.Workshops[s.project.ProjectId] = map[string]*fakebackend.FakeWorkshop{
		"test-workshop": {
			Workshop: &workshop.Workshop{
				Name:       "test-workshop",
				Project:    s.project,
				InstanceID: instanceID,
			},
		},
	}
}

// TestWorkshopHelpCtlWithoutCookie checks that help works through the
// cookie-less workshopctl path. The test relies on the middleware-provided
// workshop instance ID being present in the request context so the handler can
// validate the caller and create an ephemeral hook context.
func (s *apiSuite) TestWorkshopHelpCtlWithoutCookie(c *check.C) {
	// Setup
	s.daemon(c)
	s.addWorkshopWithInstanceID("instance-id")
	wctl := apiCmd("/v1/workshopctl")

	buf := bytes.NewBufferString(`{"args":["-h"]}`)

	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)

	req = req.WithContext(context.WithValue(
		req.Context(),
		workshop.ContextWorkshopInstanceID,
		"instance-id",
	))

	// Execute
	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	_, err = rsp.MarshalJSON()
	c.Assert(err, check.IsNil)
}

// TestWorkshopCtlRequiresInstanceIDWithoutCookie checks that workshopctl
// requests without a hook cookie must identify their workshop instance.
func (s *apiSuite) TestWorkshopCtlRequiresInstanceIDWithoutCookie(c *check.C) {
	s.daemon(c)
	wctl := apiCmd("/v1/workshopctl")
	buf := bytes.NewBufferString(`{"args":["get-secret","sdk.secret"]}`)
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)

	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
}

// TestWorkshopCtlRejectsUnknownInstanceID checks that a request without a hook
// cookie cannot use an instance ID outside the requesting user's workshops.
func (s *apiSuite) TestWorkshopCtlRejectsUnknownInstanceID(c *check.C) {
	s.daemon(c)
	wctl := apiCmd("/v1/workshopctl")
	buf := bytes.NewBufferString(`{"args":["get-secret","sdk.secret"]}`)
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		workshop.ContextWorkshopInstanceID,
		"unknown-instance",
	))

	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusForbidden)
}

// TestWorkshopCtlAcceptsOwnedInstanceID checks that a request without a hook
// cookie may run after its instance ID is matched to the requesting user.
func (s *apiSuite) TestWorkshopCtlAcceptsOwnedInstanceID(c *check.C) {
	s.daemon(c)
	s.addWorkshopWithInstanceID("instance-id")

	wctl := apiCmd("/v1/workshopctl")
	buf := bytes.NewBufferString(
		`{"args":["get-secret","sdk.secret"],"stdin":"dGVzdA=="}`,
	)
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		workshop.ContextWorkshopInstanceID,
		"instance-id",
	))

	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusOK)
}

// TestWorkshopCtlCookieSkipsInstanceIDValidation checks that a valid hook
// cookie remains the authentication mechanism and does not require an instance
// ID lookup.
func (s *apiSuite) TestWorkshopCtlCookieSkipsInstanceIDValidation(c *check.C) {
	s.daemon(c)
	st := s.d.overlord.State()
	st.Lock()
	st.Set("workshop-cookies", map[string]string{"cookie-id": "test-workshop"})
	st.Unlock()

	wctl := apiCmd("/v1/workshopctl")
	buf := bytes.NewBufferString(
		`{"context-id":"cookie-id","args":["get-secret","sdk.secret"]}`,
	)
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)

	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusOK)
}
