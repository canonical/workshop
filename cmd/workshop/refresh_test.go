package main

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type workshopRefresh struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopRefresh{})

var mockSingleWorkshopJSON = `{"type":"sync","status-code":200,"status":"OK","result":{
    "workshops":[{
        "name":"ws",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready"
    }]
}}`

var mockReadyChangeJSON = `{"type": "sync", "result":{
    "id":   "42",
    "kind": "refresh",
    "summary": "...",
    "status": "Done",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Done", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"}]
}}`

var mockWaitChangeJSON = `{"type": "sync", "result":{
    "id":   "42",
    "kind": "refresh",
    "summary": "...",
    "status": "Wait",
    "ready": false,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Wait", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"}]
}}`

var mockSketchWaitChangeJSON = `{"type": "sync", "result":{
    "id":   "43",
    "kind": "refresh",
    "summary": "...",
    "status": "Wait",
    "ready": false,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Wait", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"}]
}}`

var mockAbortedChangeJSON = `{"type": "sync", "result":{
    "id":   "42",
    "kind": "refresh",
    "summary": "...",
    "status": "Undone",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Undone", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"},{"kind": "foo", "summary": "some summary", "status": "Hold", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z" , "log":["2015-02-21T01:02:03Z INFO Aborting for workshop \"ws\"..."], "data":{"workshop":"ws"}}]
}}`

var mockSketchAbortedChangeJSON = `{"type": "sync", "result":{
    "id":   "43",
    "kind": "refresh",
    "summary": "...",
    "status": "Undone",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Undone", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"},{"kind": "foo", "summary": "some summary", "status": "Hold", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z" , "log":["2015-02-21T01:02:03Z INFO Aborting for workshop \"ws\"..."], "data":{"workshop":"ws"}}]
}}`

var mockChangeWithError = `{"type": "sync", "result":{
    "id":   "42",
    "kind": "refresh",
    "summary": "...",
    "status": "Error",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
	"err": "no answer",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Undone", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"},{"kind": "foo", "summary": "some summary", "status": "Error", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z" , "log":["2015-02-21T01:02:03Z ERROR No answer found"], "data":{"workshop":["ws","ws-1"]}}]
}}`

func (m *workshopRefresh) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

func (m *workshopRefresh) TestRefreshTransactionalSuccess(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
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

	err := cmd.Run(cmd.Command(), []string{"ws", "ws"})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `"ws" refreshed\n`)
	c.Check(n, check.Equals, 3)
}

func (m *workshopRefresh) TestRefreshTransactionalFailedAndAborted(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
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
			fmt.Fprintln(w, mockChangeWithError)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{"ws", "ws-1"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `(?s).*"ws", "ws-1" refresh aborted`)
	c.Check(n, check.Equals, 3)
}

func (m *workshopRefresh) TestRefreshWaitOnErrorFailed(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
	cmd.WaitOnError = true
	cmd.Restore = true

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
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			fmt.Fprintln(w, mockSingleWorkshopJSON)
		case 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{"ws"}, "options": map[string]interface{}{"mode": "wait-on-error", "refresh-option": "restore"},
			})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockWaitChangeJSON)
		default:
			c.Errorf("expected 4 calls, now on %d", n)
		}
	})

	err := cmd.Run(nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "cannot complete refresh for \"ws\", execution is paused\n\n"+
		"To proceed, resolve the issue and run 'workshop refresh --continue ws'\n"+
		"To cancel and undo: 'workshop refresh --abort ws'\n"+
		"To view more information: 'workshop tasks 42'")
	c.Check(n, check.Equals, 4)
}

func (m *workshopRefresh) TestRefreshWaitOnErrorAbortedSuccessfully(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
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
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{"ws"}, "options": map[string]interface{}{"mode": "abort"},
			})
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
	c.Assert(m.stdout.String(), check.Matches, `"ws" refresh aborted\n`)
	c.Check(n, check.Equals, 3)
}

func (m *workshopRefresh) TestRefreshWaitOnErrorContinuedSuccessfully(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
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
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{"ws"}, "options": map[string]interface{}{"mode": "continue"},
			})
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
	c.Assert(m.stdout.String(), check.Matches, `"ws" refreshed\n`)
	c.Check(n, check.Equals, 3)
}
