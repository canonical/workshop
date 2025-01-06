package daemon

import (
	"bytes"
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

func (s *apiSuite) runMountTest(c *check.C, wp string, buffers []*bytes.Buffer, expected []*expectedResp) {
	s.vars = map[string]string{"id": s.project.ProjectId, "name": wp}
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}/mounts")
	requests := []*http.Request{}

	for _, i := range buffers {
		req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/"+wp+"/mounts", i)
		c.Assert(err, check.IsNil)
		requests = append(requests, req)
	}

	for num, req := range requests {
		// Execute
		rsp := v1PostWorkshopMount(projectsCmd, req, nil).(*resp)

		// Verify
		c.Check(rsp.Type, check.Equals, expected[num].Type)
		c.Check(rsp.Status, check.Equals, expected[num].Status, check.Commentf("case: %v: %v", num, rsp))
		if rsp.Type == ResponseTypeError {
			c.Assert(rsp.Result.(*errorResult).Message, check.Equals, expected[num].Message)
		}

		if rsp.Type == ResponseTypeAsync {
			st := s.d.state
			st.Lock()
			change := s.d.state.Change(rsp.Change)
			st.Unlock()
			c.Assert(change, check.NotNil)
			c.Assert(change.Kind(), check.Equals, expected[num].Kind)
			c.Assert(change.Summary(), check.Equals, expected[num].Summary)
			<-change.Ready()
			st.Lock()
			if expected[num].ChangeErr != "" {
				c.Assert(change.Err(), check.ErrorMatches, expected[num].ChangeErr)
			} else {
				c.Assert(change.Err(), check.IsNil)
			}
			st.Unlock()
		}
	}
}

func (s *apiSuite) TestWorkshopRemountSuccess(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	repo := s.d.overlord.InterfaceManager().Repository()

	s.launchWorkshop(c, "manysdks", manysdks, apiSuiteSdks)
	ref, err := repo.Connected(s.project.ProjectId, "manysdks", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	conn, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	// The mock iface backend does not set this attribute as the actual one
	// would.
	c.Assert(conn.Slot.SetAttr("host-source", "/home/user/.local/share"), check.IsNil)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(fmt.Sprintf(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":%q}`, c.MkDir())),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "remount",
			Summary: `Remount manysdks/test-sdk:data`,
		},
	}

	s.runMountTest(c, "manysdks", requests, expected)
}

func (s *apiSuite) TestWorkshopRemountBoundPlugSuccess(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	repo := s.d.overlord.InterfaceManager().Repository()

	s.launchWorkshop(c, "manysdks", manysdks, apiSuiteSdks)
	ref, err := repo.Connected(s.project.ProjectId, "manysdks", "test-sdk", "data")
	c.Assert(err, check.IsNil)
	conn, err := repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	// The mock iface backend does not set this attribute as the actual one
	// would.
	c.Assert(conn.Slot.SetAttr("host-source", "/home/user/.local/share"), check.IsNil)

	// Setup
	src := c.MkDir()
	requests := []*bytes.Buffer{
		bytes.NewBufferString(fmt.Sprintf(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":%q}`, src)),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeAsync,
			Status:  http.StatusAccepted,
			Kind:    "remount",
			Summary: `Remount manysdks/test-sdk:data`,
		},
	}

	s.runMountTest(c, "manysdks", requests, expected)
	conn, err = repo.Connection(ref[0])
	c.Assert(err, check.IsNil)
	c.Assert(conn.Slot.DynamicAttrs(), check.DeepEquals, map[string]interface{}{"host-source": src})
}

func (s *apiSuite) TestWorkshopRemountNoWorkshop(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":"/srv/data"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusNotFound,
			Message: `cannot access workshop "missing": workshop not launched`,
		},
	}

	s.runMountTest(c, "missing", requests, expected)
}

func (s *apiSuite) TestWorkshopRemountPlugDisconnected(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "manysdks", manysdks, apiSuiteSdks)

	_, err := s.d.overlord.InterfaceManager().Repository().DisconnectSdk(s.project.ProjectId, "manysdks", "test-sdk")
	c.Check(err, check.IsNil)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":"/srv/data"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot remount "manysdks/test-sdk:data": plug is disconnected`,
		},
	}

	s.runMountTest(c, "manysdks", requests, expected)
}

func (s *apiSuite) TestWorkshopRemountInvalidSetup(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "manysdks", manysdks, apiSuiteSdks)

	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"action":"mount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":"/srv/data"}`),
		bytes.NewBufferString(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data","host-source":"/srv/data"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `unknown action "mount"`,
		},
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot decode data from request body: unexpected EOF`,
		},
	}

	s.runMountTest(c, "manysdks", requests, expected)
}

func (s *apiSuite) TestWorkshopRemountInvalidInterface(c *check.C) {
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "manysdks", manysdks, apiSuiteSdks)

	// Setup
	requests := []*bytes.Buffer{
		bytes.NewBufferString(`{"action":"remount","plug":{"sdk":"test-sdk-2","plug":"gpu"},"host-source":"/srv/data"}`),
	}

	expected := []*expectedResp{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot remount "manysdks/test-sdk-2:gpu": interface type should be "mount" (now: "gpu")`,
		},
	}

	s.runMountTest(c, "manysdks", requests, expected)
}

func (s *apiSuite) TestWorkshopRemountStaticSlotSourceFails(c *check.C) {
	// Setup
	s.daemon(c)
	s.d.Overlord().Loop()
	defer s.d.Overlord().Stop()

	s.launchWorkshop(c, "workshopconns", workshopconns, apiSuiteSdks)

	requests := []*bytes.Buffer{
		bytes.NewBufferString(fmt.Sprintf(`{"action":"remount","plug":{"sdk":"test-sdk","plug":"data"},"host-source":%q}`, c.MkDir())),
	}

	expected := []*expectedResp{
		{
			Type:      ResponseTypeAsync,
			Status:    http.StatusAccepted,
			Kind:      "remount",
			Summary:   `Remount workshopconns/test-sdk:data`,
			ChangeErr: `(?s).*SDK "system" does not have attribute "host-source" for interface "mount".*`,
		},
	}

	s.runMountTest(c, "workshopconns", requests, expected)
}
