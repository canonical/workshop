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
	"errors"
	"fmt"
	"net/http"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
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

// TestWorkshopCtlErrorExitCode checks that wrapped exit-code metadata is
// transported with the outer error's diagnostic, not the metadata's empty text.
func (s *apiSuite) TestWorkshopCtlErrorExitCode(c *check.C) {
	err := fmt.Errorf(
		"unlock the secret provider and try again%w",
		ctlcmd.CommandExitCodeError{ExitCode: 2},
	)

	rsp := workshopctlErrorResponse(err).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Check(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Result, check.DeepEquals, &errorResult{
		Message: "unlock the secret provider and try again",
		Value: map[string]any{
			"exit-code": 2,
			"stderr":    "unlock the secret provider and try again",
		},
	})
}

// TestWorkshopCtlErrorZeroExitCode checks that explicit process success is
// preserved even when carried by an API error response.
func (s *apiSuite) TestWorkshopCtlErrorZeroExitCode(c *check.C) {
	err := fmt.Errorf(
		"optional value unavailable%w",
		ctlcmd.CommandExitCodeError{ExitCode: 0},
	)

	rsp := workshopctlErrorResponse(err).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Check(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Result, check.DeepEquals, &errorResult{
		Message: "optional value unavailable",
		Value: map[string]any{
			"exit-code": 0,
			"stderr":    "optional value unavailable",
		},
	})
}

// TestWorkshopCtlErrorUnknown checks that errors without exit-code metadata
// retain the generic response without fabricated output or process status.
func (s *apiSuite) TestWorkshopCtlErrorUnknown(c *check.C) {
	err := errors.New("command failed")

	rsp := workshopctlErrorResponse(err).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Check(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Result, check.DeepEquals, &errorResult{
		Message: err.Error(),
	})
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
// cookie reaches SDK validation in the authenticated workshop rather than
// attempting to resolve placeholder project and workshop names.
func (s *apiSuite) TestWorkshopCtlAcceptsOwnedInstanceID(c *check.C) {
	logs, restore := logger.MockLogger()
	defer restore()
	s.daemon(c)
	s.addWorkshopWithInstanceID("instance-id")
	s.d.overlord.Loop()
	defer func() {
		c.Check(s.d.overlord.Stop(), check.IsNil)
	}()

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

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Check(rsp.Type, check.Equals, ResponseTypeError)
	c.Assert(rsp.Result, check.FitsTypeOf, &errorResult{})
	c.Check(rsp.Result.(*errorResult).Message, check.Equals,
		`cannot retrieve secret for plug "sdk.secret": internal error`)
	c.Check(logs.String(), check.Matches,
		"(?s).*requested sdk is not installed in workshop.*")

}

// TestWorkshopCtlPreservesInstanceIdentity checks the ephemeral context carries
// the project, workshop and authenticated user resolved from the instance ID.
func (s *apiSuite) TestWorkshopCtlPreservesInstanceIdentity(c *check.C) {
	s.daemon(c)
	s.addWorkshopWithInstanceID("instance-id")
	wctl := apiCmd("/v1/workshopctl")
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", nil)
	c.Assert(err, check.IsNil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		workshop.ContextWorkshopInstanceID,
		"instance-id",
	))

	hookContext, response := workshopctlHookContext(wctl, req, "")

	c.Assert(response, check.IsNil)
	c.Assert(hookContext, check.NotNil)
	identity, err := hookContext.WorkshopIdentity()
	c.Assert(err, check.IsNil)
	c.Check(identity.Project, check.DeepEquals, s.project)
	c.Check(identity.Workshop, check.Equals, "test-workshop")
	c.Check(identity.User, check.Equals,
		req.Context().Value(workshop.ContextUser))
}

// TestWorkshopCtlRejectsUnknownCookie checks that an invalid cookie produces
// a generic client error even when a valid workshop instance ID is supplied.
func (s *apiSuite) TestWorkshopCtlRejectsUnknownCookie(c *check.C) {
	s.daemon(c)
	s.addWorkshopWithInstanceID("instance-id")
	wctl := apiCmd("/v1/workshopctl")
	buf := bytes.NewBufferString(
		`{"context-id":"unknown-cookie","args":["get-secret","sdk.secret"]}`,
	)
	req, err := s.createProjectsRequest("POST", "/v1/workshopctl", buf)
	c.Assert(err, check.IsNil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		workshop.ContextWorkshopInstanceID,
		"instance-id",
	))

	rsp := v1PostWorkshopCtl(wctl, req, nil).(*resp)

	c.Check(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Check(rsp.Type, check.Equals, ResponseTypeError)
	c.Assert(rsp.Result, check.FitsTypeOf, &errorResult{})
	c.Check(rsp.Result.(*errorResult).Message, check.Equals,
		"invalid workshop cookie")
}
