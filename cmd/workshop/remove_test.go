package main

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type workshopRemove struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
	user   *user.User
}

var _ = check.Suite(&workshopRemove{})

func (m *workshopRemove) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)

	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	var err error
	m.user, err = osutil.UserMaybeSudoUser()
	c.Assert(err, check.IsNil)
}

func (m *workshopRemove) TearDownTest(c *check.C) {
	hackdir := sdk.ProjectHackSdkDir(m.user.HomeDir, m.prjId)
	err := os.RemoveAll(hackdir)
	c.Assert(err, check.IsNil)
}

func (m *workshopRemove) TestRemoveSuccess(c *check.C) {
	cmd := &CmdRemove{root: &CmdRoot{}}
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
	c.Assert(m.stdout.String(), check.Matches, `"ws" removed\n"ws-1" removed\n`)
}

func (m *workshopHack) TestRemoveDropsHack(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")

	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		return []byte(inContent), nil
	})
	defer restore()

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)

	cmdremove := &CmdRemove{root: &CmdRoot{}}
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

	m.ResetStdStreams()
	err = cmdremove.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, `"ws" removed\n`)

	hackdir := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	exist, _, _ := osutil.ExistsIsDir(hackdir)
	c.Assert(exist, check.Equals, false)

	storedir := sdk.WorkshopHackSdkStored(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(storedir, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}
