package main

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type remountSuite struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&remountSuite{})

func (m *remountSuite) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

func (m *remountSuite) TestRemountSuccess(c *check.C) {
	cmd := &CmdRemount{root: &CmdRoot{}}
	body := map[string]interface{}{
		"action": "remount",
		"plug": map[string]interface{}{
			"project-id": "42424242",
			"workshop":   "ws",
			"sdk":        "sdk",
			"plug":       "plug",
		},
		"host-source": "/new/source",
	}
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
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/mounts", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, body)
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

	err := cmd.Run(cmd.Command(), []string{"ws/sdk:plug", "/new/source"})
	c.Assert(err, check.IsNil)
}

func (m *remountSuite) TestRemountBrokenReference(c *check.C) {
	cmd := &CmdRemount{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{"ws:sdk:plug", "/new/source"})
	c.Assert(err, check.ErrorMatches, `unknown plug or slot reference "ws:sdk:plug"`)
}
