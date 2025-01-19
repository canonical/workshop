package main

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type workshopExec struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopExec{})

var mockWorkshopNoScripts = `{"type":"sync","status-code":200,"status":"OK","result":{}}`
var mockWorkshopWithScripts = `{"type":"sync","status-code":200,"status":"OK","result":{"foo":{"script":"echo foo"},"bar":{"script":"echo bar\n"}}}`

func (m *workshopExec) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)

	m.prjDir = c.MkDir()
	m.prjId = "42424242"
}

func (m *workshopExec) TestWorkshopScripts(c *check.C) {
	cmd := &CmdScripts{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 4:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshop)
		case 3, 5:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/scripts", m.prjId))
			w.WriteHeader(200)
			if n == 3 {
				fmt.Fprintln(w, mockWorkshopNoScripts)
			} else {
				fmt.Fprintln(w, mockWorkshopWithScripts)
			}
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), nil)
	c.Assert(err, check.IsNil)
	c.Check(m.stdout.String(), check.Equals, "")
	m.stdout.Reset()
	c.Check(n, check.Equals, 3)

	err = cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)
	c.Check(m.stdout.String(), check.Matches, `bar:
  script: |
    echo bar
foo:
  script: echo foo
`)
	c.Check(n, check.Equals, 5)
}
