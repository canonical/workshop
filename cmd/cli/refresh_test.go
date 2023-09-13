package cli

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type WorkshopRefresh struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&WorkshopRefresh{})

var mockReadyChangeJSON = `{"type": "sync", "result":{
    "id":   "four",
    "kind": "refresh",
    "summary": "...",
    "status": "Done",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Done", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"}]
}}`

var mockWaitChangeJSON = `{"type": "sync", "result":{
    "id":   "four",
    "kind": "refresh",
    "summary": "...",
    "status": "Wait",
    "ready": false,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Wait", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"}]
}}`

var mockAbortedChangeJSON = `{"type": "sync", "result":{
    "id":   "four",
    "kind": "refresh",
    "summary": "...",
    "status": "Error",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Undone", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"},{"kind": "foo", "summary": "some summary", "status": "Error", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z" , "log":["2015-02-21T01:02:03Z INFO Aborting the \"ws\" workshop refresh..."], "data":{"workshop":"ws"}}]
}}`

var mockChangeWithError = `{"type": "sync", "result":{
    "id":   "four",
    "kind": "refresh",
    "summary": "...",
    "status": "Error",
    "ready": true,
    "spawn-time": "2015-02-21T01:02:03Z",
    "ready-time": "2015-02-21T01:02:04Z",
	"err": "no answer",
    "tasks": [{"kind": "bar", "summary": "some summary", "status": "Undone", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z"},{"kind": "foo", "summary": "some summary", "status": "Error", "progress": {"done": 1, "total": 1}, "spawn-time": "2015-02-21T01:02:03Z", "ready-time": "2015-02-21T01:02:04Z" , "log":["2015-02-21T01:02:03Z ERROR No answer found"], "data":{"workshop":["ws","ws-1"]}}]
}}`

func (m *WorkshopRefresh) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

func (m *WorkshopRefresh) TestRefreshTransactionalSuccess(c *check.C) {
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
}

func (m *WorkshopRefresh) TestRefreshTransactionalFailedAndAborted(c *check.C) {
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
}

func (m *WorkshopRefresh) TestRefreshWaitOnErrorFailed(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
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
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "refresh",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"refresh-mode": "wait-on-error"}})
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
	c.Assert(err, check.ErrorMatches, `cannot refresh; fix the errors reported,\nthen run "workshop refresh --continue ws".\nTo abort and revert, run "workshop refresh --abort ws"`)
}

func (m *WorkshopRefresh) TestRefreshWaitOnErrorAbortedSuccessfully(c *check.C) {
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
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "refresh",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"refresh-mode": "abort"}})
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
}

func (m *WorkshopRefresh) TestRefreshWaitOnErrorContinuedSuccessfully(c *check.C) {
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
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "refresh",
				"names": []interface{}{"ws"}, "options": map[string]interface{}{"refresh-mode": "continue"}})
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
}

func (m *WorkshopRefresh) TestRefreshIncompatibleOptions(c *check.C) {
	cmd := &CmdRefresh{root: &CmdRoot{}}
	cmd.Abort = true
	cmd.Continue = true

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.ErrorMatches, "cannot refresh: '--abort' incompatible with '--continue'")

	cmd.WaitOnError = true
	cmd.Abort = false
	cmd.Continue = true

	err = cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.ErrorMatches, "cannot refresh: '--wait-on-error' incompatible with '--continue'")

	cmd.WaitOnError = true
	cmd.Abort = true
	cmd.Continue = false

	err = cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.ErrorMatches, "cannot refresh: '--wait-on-error' incompatible with '--abort'")

	cmd.WaitOnError = true
	cmd.Abort = false
	cmd.Continue = false

	err = cmd.Run(nil, []string{"ws", "ws-1"})
	c.Assert(err, check.ErrorMatches, "cannot refresh: '--wait-on-error' incompatible with multiple workshops")
}
