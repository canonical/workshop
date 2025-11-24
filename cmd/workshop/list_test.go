package main

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type workshopList struct{}

var _ = check.Suite(&workshopList{})

var mockWorkshopList = `{"type":"sync","status-code":200,"status":"OK","result":{
    "workshops":[{
        "name":"ws",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Error",
        "notes":["missing-project"]
    },{
        "name":"as-1",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready"
    }],
    "files":[{
        "name":"ws",
        "project-id":"2",
        "path":"/home/projects/.workshop/ws.yaml"
    },{
        "name":"as-1",
        "project-id":"2",
        "path":"/home/projects/.workshop/as-1.yaml"
    },{
        "name":"zs-1",
        "project-id":"2",
        "path":"/home/projects/.workshop/zs-1.yaml"
    },{
        "name":"ds-1",
        "project-id":"2",
        "path":"/home/projects/.workshop/ds-1.yaml"
    }]
},
"warning-timestamp":"1970-01-01T00:00:00.00000000Z",
"warning-count":1}`

var mockWorkshopList2 = `{"type":"sync","status-code":200,"status":"OK","result":{
    "workshops":[{
        "name":"ws",
        "base":"ubuntu@22.04",
        "project-id":"2",
        "status":"Ready"
    }],
    "files":[{
        "name":"ws",
        "project-id":"2",
        "path":"/home/projects/ws"
    },{
        "name":"ws2",
        "project-id":"2",
        "path":"/home/projects/ws"
    }]
},
"warning-timestamp":"1970-01-01T00:00:00.00000000Z",
"warning-count":1}`

var mockWorkshopList3 = `{"type":"sync","status-code":200,"status":"OK","result":{
    "files":[{
        "name":"ws",
        "project-id":"2",
        "path":"/home/projects/.workshop/ws.yaml"
    },{
        "name":"as-1",
        "project-id":"2",
        "path":"/home/projects/.workshop/as-1.yaml"
    }]
}}`

func (m *workshopInfo) TestWorkshopListFilesOnly(c *check.C) {
	cmd := &CmdList{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, "/home/project")
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList3)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `Project        Workshop  Status  Notes
/home/project  as-1      Off     -
/home/project  ws        Off     -
`)
	c.Check(n, check.Equals, 2)
}

func (m *workshopInfo) TestWorkshopList(c *check.C) {
	cmd := &CmdList{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, "/home/project")
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `Project        Workshop  Status  Notes
/home/project  as-1      Ready   -
/home/project  ws        Error   missing-project
/home/project  ds-1      Off     -
/home/project  zs-1      Off     -
`)
	c.Check(n, check.Equals, 2)
}

func (m *workshopInfo) TestWorkshopListGlobal(c *check.C) {
	cmd := &CmdList{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := `{"type": "sync", "result": [{"id":"1","path":"/home/project-1"}, {"id":"2","path":"/home/project-2"}]}`
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects/1/workshops")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList)
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects/2/workshops")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList2)
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	cmd.global = true
	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `Project          Workshop  Status  Notes
/home/project-1  as-1      Ready   -
/home/project-1  ws        Error   missing-project
/home/project-2  ws        Ready   -
`)
	c.Check(n, check.Equals, 3)
}

func (m *workshopInfo) TestWorkshopListGlobalEmpty(c *check.C) {
	cmd := &CmdList{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := `{"type": "sync", "result": []}`
			fmt.Fprintln(w, r)
		default:
			c.Errorf("expected 1 call, now on %d", n)
		}
	})

	cmd.global = true
	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, ``)
	c.Check(n, check.Equals, 1)
}
