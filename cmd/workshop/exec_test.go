package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/check.v1"
)

type workshopExec struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopExec{})

var (
	mockWorkshopNoActions   = `{"type":"sync","status-code":200,"status":"OK","result":{}}`
	mockWorkshopWithActions = `{"type":"sync","status-code":200,"status":"OK","result":{
    "foo":{
        "script":"echo foo"
    },
    "bar":{
        "script":"echo bar\n"
    }
}}`
)

var mockWorkshopActionsError = `{"type":"error","status-code":404,"status":"Not Found","result":{
    "message":"workshop not found"
}}`

func (m *workshopExec) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)

	m.prjDir = c.MkDir()
	m.prjId = "42424242"
}

func (m *workshopExec) TestWorkshopActions(c *check.C) {
	cmd := &CmdActions{root: &CmdRoot{}}
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 4, 6:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case 3, 5:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId))
			w.WriteHeader(200)
			if n == 3 {
				fmt.Fprintln(w, mockWorkshopNoActions)
			} else {
				fmt.Fprintln(w, mockWorkshopWithActions)
			}
		case 7:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/wrong-name/actions", m.prjId))
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopActionsError)
		default:
			c.Errorf("expected 7 calls, now on %d", n)
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

	err = cmd.Run(cmd.Command(), []string{"wrong-name"})
	c.Check(err, check.ErrorMatches, "workshop not found")

	c.Check(n, check.Equals, 7)
}

func (m *workshopExec) TestWorkshopRunCompletion(c *check.C) {
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3, 5, 7, 10, 12, 14:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2, 8, 13, 15:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case 4, 9, 11, 16:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithActions)
		case 6:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/wrong-name/actions", m.prjId))
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopActionsError)
		default:
			c.Errorf("expected 16 calls, now on %d", n)
		}
	})

	cmd := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	run := cmd.Command()

	// TODO: remove this workaround once this bug is fixed:
	// https://github.com/spf13/cobra/issues/1877
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"workshop", "__complete", "run", ""}
	result, compDirective := run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"ws"})

	os.Args = []string{"workshop", "__complete", "run", "ws", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "wrong-name", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"wrong-name"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveError)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "f"}
	result, compDirective = run.ValidArgsFunction(run, nil, "f")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	c.Check(n, check.Equals, 16)
}
