package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

type workshopLaunch struct {
	BaseWorkshopSuite
	client *client.Client
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopLaunch{})

func (m *workshopLaunch) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
	m.client = &client.Client{}
}

func (m *workshopLaunch) TestLaunchSuccess(c *check.C) {
	cmd := &CmdLaunch{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockReadyChangeJSON)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{"ws", "ws-1", "ws"})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `"ws" launched\n"ws-1" launched\n`)
}

func (m *workshopLaunch) TestLaunchWaitOnErrorFailed(c *check.C) {
	cmd := &CmdLaunch{root: &CmdRoot{}}
	cmd.WaitOnError = true

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "launch",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"mode": "wait-on-error"}})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockWaitChangeJSON)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "cannot complete launch for \"ws\", execution is paused\n\n"+
		"To proceed, resolve the issue and run 'workshop launch --continue ws'\n"+
		"To cancel and undo: 'workshop launch --abort ws'\n"+
		"To view more information: 'workshop tasks 42'")
}

func (m *workshopLaunch) TestLaunchWaitOnErrorAbortedSuccessfully(c *check.C) {
	cmd := &CmdLaunch{root: &CmdRoot{}}
	cmd.Abort = true

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "launch",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"mode": "abort"}})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockAbortedChangeJSON)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `"ws" launch aborted\n`)
}

func (m *workshopLaunch) TestLaunchWaitOnErrorContinuedSuccessfully(c *check.C) {
	cmd := &CmdLaunch{root: &CmdRoot{}}
	cmd.Continue = true

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "launch",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"mode": "continue"}})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockReadyChangeJSON)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `"ws" launched\n`)
}

func (m *workshopLaunch) TestLaunchCompletions(c *check.C) {
	wsInfo := []*client.WorkshopInfo{
		{
			ProjectId: m.prjId,
			Name:      "workshop-exists",
			Status:    "Ready",
		},
	}

	wsFile := []*client.WorkshopFile{
		{
			ProjectId: m.prjId,
			Name:      "workshop-exists",
			Path:      "/tmp/workshop",
		},
		{
			ProjectId: m.prjId,
			Name:      "workshop-notexists",
			Path:      "/tmp/workshop1",
		},
	}

	w := client.Workshops{
		Workshops: wsInfo,
		Files:     wsFile,
	}

	cmd := CmdLaunch{
		root: &CmdRoot{},
	}

	m.listRedirectHelper(c, w, m.prjId, m.prjDir, len(wsFile))

	completions, compDirective := cmd.complete(cmd.Command(), nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(completions, check.DeepEquals, []string{"workshop-notexists"})
}
