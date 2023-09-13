package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"
	. "gopkg.in/check.v1"
)

type WorkshopList struct {
}

var _ = Suite(&WorkshopList{})

func TestMain(t *testing.T) { TestingT(t) }

func (m *WorkshopList) TestHomeDirectoryPathContraction(c *C) {
	home, _ := os.UserHomeDir()
	r := contractHomeDirectory(filepath.Join(home, "test"))
	c.Assert(r, Equals, "~/test")
	r = contractHomeDirectory(filepath.Join(home, "///test"))
	c.Assert(r, Equals, "~/test")
	r = contractHomeDirectory(home)
	c.Assert(r, Equals, "~")
	r = contractHomeDirectory("/sys")
	c.Assert(r, Equals, "/sys")

	/* This will fail because of how filepath handles path prefixes (not path aware)
	r = contractHomeDirectory(home + "4")
	assert.Equal(t, "~", r)
	*/
}

var mockWorkshopList = `{"type":"sync","status-code":200,"status":"OK","result":[{"name":"ws","base":"ubuntu@22.04","project-id":"42424242","status":"Error","notes":["missing-file"]}, {"name":"as-1","base":"ubuntu@22.04","project-id":"42424242","status":"Ready"}]}`

var mockWorkshopList2 = `{"type":"sync","status-code":200,"status":"OK","result":[{"name":"ws","base":"ubuntu@22.04","project-id":"2","status":"Ready"}]}`

func (m *WorkshopInfo) TestWorkshopList(c *check.C) {
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
/home/project  ws        Error   missing-file
`)
}

func (m *WorkshopInfo) TestWorkshopListGlobal(c *check.C) {
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
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	cmd.global = true
	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `Project          Workshop  Status  Notes
/home/project-1  as-1      Ready   -
/home/project-1  ws        Error   missing-file
/home/project-2  ws        Ready   -
`)
}

func (m *WorkshopInfo) TestWorkshopListGlobalEmpty(c *check.C) {
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
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	cmd.global = true
	err := cmd.runList()
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, ``)
}
