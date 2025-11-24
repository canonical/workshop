package daemon

import (
	"bytes"
	"net/http"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

var wsYaml = `name: ws
base: ubuntu@20.04
actions:
  lint: golangci-lint run
`

func (s *apiSuite) setupExec(c *check.C) *Command {
	s.daemon(c)
	projectsCmd := apiCmd("/v1/projects/{id}/workshops/{name}/exec")

	s.vars = map[string]string{"id": s.project.ProjectId, "name": "ws"}
	s.createWFile(c, "ws", wsYaml)

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Actions: map[string]workshop.Action{"lint": "\n\n\ngolangci-lint run\n"}}
	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}

	err := s.b.LaunchOrRebuildWorkshop(s.ctx, wf, image)
	c.Assert(err, check.IsNil)

	wp, err := s.b.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	wp.Running = true

	return projectsCmd
}

func (s *apiSuite) TestExecNoCommand(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":[]}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Assert(rsp.Result.(*errorResult).Message, check.Matches, ".*must specify command")
}

func (s *apiSuite) TestExecNoAction(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":[],"action":true}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Assert(rsp.Result.(*errorResult).Message, check.Matches, `cannot run action in "ws": must specify action`)
}

func (s *apiSuite) TestExecUnsupportedModes(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := []*bytes.Buffer{
		bytes.NewBufferString(`{"command":["ls"],"terminal":true}`),
		bytes.NewBufferString(`{"command":["ls"],"split-stderr":true}`),
	}

	expected := []*struct {
		Type    ResponseType
		Status  int
		Message string
	}{
		{
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot exec command in "ws": terminal mode is not supported`,
		}, {
			Type:    ResponseTypeError,
			Status:  http.StatusBadRequest,
			Message: `cannot exec command in "ws": splitting stderr is not supported`,
		},
	}

	requests := []*http.Request{}
	for _, r := range body {
		req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", r)
		c.Assert(err, check.IsNil)
		requests = append(requests, req)
	}

	soon := 0
	restoreEnsure := testutil.FakeFunc(func(st *state.State, d time.Duration) {
		soon++
	}, &ensureStateSoon)
	defer restoreEnsure()

	for i, r := range requests {
		// Execute
		rsp := v1PostWorkshopExec(projectsCmd, r, nil).(*resp)

		// Verify
		c.Assert(rsp.Type, check.Equals, expected[i].Type, check.Commentf("case: %v", i))
		c.Assert(rsp.Status, check.Equals, expected[i].Status)
		if rsp.Type == ResponseTypeError {
			c.Assert(rsp.Result.(*errorResult).Message, check.Equals, expected[i].Message)
		}
	}

	c.Assert(soon, check.Equals, 0)
}

func (s *apiSuite) TestExecSuccess(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":["ls"],"working-dir":"/"}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	soon := 0
	restoreEnsure := testutil.FakeFunc(func(st *state.State, d time.Duration) {
		soon++
	}, &ensureStateSoon)
	defer restoreEnsure()

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusAccepted)
	c.Assert(soon, check.Equals, 1)
}

func (s *apiSuite) TestExecActionSuccess(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":["lint", "--verbose"],"action":true,"working-dir":"/"}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	soon := 0
	restoreEnsure := testutil.FakeFunc(func(st *state.State, d time.Duration) {
		soon++
	}, &ensureStateSoon)
	defer restoreEnsure()

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusAccepted)
}

func (s *apiSuite) TestExecUserOrGroupNotProvided(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":["ls"], "user-id": 1000}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Assert(rsp.Result.(*errorResult).Message, check.Matches, "*.must specify group, not just user")

	// Setup
	body = bytes.NewBufferString(`{"command":["ls"], "group-id": 1000}`)

	req, err = s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	// Execute
	rsp = v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusBadRequest)
	c.Assert(rsp.Result.(*errorResult).Message, check.Matches, "*.must specify user, not just group")
}

func (s *apiSuite) TestExecSetEnvVariable(c *check.C) {
	// Setup
	projectsCmd := s.setupExec(c)

	body := bytes.NewBufferString(`{"command":["ls"],"working-dir":"/","environment":{"FOO":"BAR"}}`)

	req, err := s.createProjectsRequest("POST", "/v1/projects/"+s.project.ProjectId+"/workshops/ws/exec", body)
	c.Assert(err, check.IsNil)

	soon := 0
	restoreEnsure := testutil.FakeFunc(func(st *state.State, d time.Duration) {
		soon++
	}, &ensureStateSoon)
	defer restoreEnsure()

	// Execute
	rsp := v1PostWorkshopExec(projectsCmd, req, nil).(*resp)

	// Verify
	c.Assert(rsp.Status, check.Equals, http.StatusAccepted)
	c.Assert(soon, check.Equals, 1)
}
