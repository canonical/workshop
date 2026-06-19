// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/workshop"
)

type workshopExec struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopExec{})

var mockWorkshopNoActions = `{"type":"sync","status-code":200,"status":"OK","result":{}}`
var mockWorkshopWithActions = `{"type":"sync","status-code":200,"status":"OK","result":{
    "foo":{
        "script":"echo foo"
    },
    "bar":{
        "script":"echo bar\n"
    }
}}`
var mockWorkshopActionsError = `{"type":"error","status-code":404,"status":"Not Found","result":{
    "message":"workshop not found"
}}`
var mockWorkshopExecError = `{"type":"error","status-code":404,"status":"Not Found","result":{
    "message":"cannot perform the following tasks:\n- Install action \"foo\" (action not found)"
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
	m.ResetStdStreams()
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

// TestWorkshopExecSendsUserCommand verifies the CLI sends the requested
// command to the API and relies on the daemon for user/group execution.
func (m *workshopExec) TestWorkshopExecSendsUserCommand(c *check.C) {
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			c.Check(r.Method, check.Equals, "POST")
			res := fmt.Sprintf(
				`{"type": "sync", "result": {"id":"%s","path":"%s"}}`,
				m.prjId,
				m.prjDir,
			)
			fmt.Fprintln(w, res)
		case fmt.Sprintf("/v1/projects/%s/workshops", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case fmt.Sprintf("/v1/projects/%s/workshops/ws/exec", m.prjId):
			c.Check(r.Method, check.Equals, "POST")
			body := DecodedRequestBody(c, r)
			userId := fmt.Sprint(workshop.Uid)
			groupId := fmt.Sprint(workshop.Gid)
			c.Check(body["command"], check.DeepEquals, []any{"echo", "foo"})
			c.Check(body["user-id"], check.DeepEquals, json.Number(userId))
			c.Check(body["group-id"], check.DeepEquals, json.Number(groupId))

			expectedEnvironment := map[string]any{
				"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/" + userId + "/bus",
				"XDG_RUNTIME_DIR":          "/run/user/" + userId,
			}
			if term, ok := os.LookupEnv("TERM"); ok {
				expectedEnvironment["TERM"] = term
			}
			c.Check(body["environment"], check.DeepEquals, expectedEnvironment)

			expectedPrefix := []any{"sudo", "-u", "#" + userId, "-g", "#" + groupId}
			expectedPrefix = append(
				expectedPrefix,
				"--preserve-env=DBUS_SESSION_BUS_ADDRESS",
			)
			if _, ok := os.LookupEnv("TERM"); ok {
				expectedPrefix = append(expectedPrefix, "--preserve-env=TERM")
			}
			expectedPrefix = append(
				expectedPrefix,
				"--preserve-env=XDG_RUNTIME_DIR",
				"--",
				"bash",
				"-l",
				"-c",
				`exec -- "$0" "$@"`,
			)
			c.Check(body["command-prefix"], check.DeepEquals, expectedPrefix)

			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopExecError)
		default:
			c.Errorf("unexpected API call: %s", r.URL.Path)
		}
	})

	cmd := &CmdExec{root: &CmdRoot{cwd: m.prjDir}}
	exec := cmd.Command()
	exec.SilenceUsage = true
	exec.SilenceErrors = true
	exec.SetArgs([]string{"-I", "ws", "--", "echo", "foo"})

	err := exec.Execute()
	c.Check(err, check.ErrorMatches, `(?s).*Install action "foo".*`)
}

func (m *workshopExec) TestSingleWorkshopRunErrors(c *check.C) {
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			c.Check(r.Method, check.Equals, "POST")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case fmt.Sprintf("/v1/projects/%s/workshops", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case fmt.Sprintf("/v1/projects/%s/workshops/ws/exec", m.prjId):
			c.Check(r.Method, check.Equals, "POST")
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopExecError)
		case fmt.Sprintf("/v1/projects/%s/workshops/foo/exec", m.prjId):
			c.Check(r.Method, check.Equals, "POST")
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopActionsError)
		default:
			c.Errorf("unexpected API call:", r.URL.Path)
		}
	})

	run := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	command := func(args ...string) *cobra.Command {
		cmd := run.Command()
		run.flags.Interactive = true
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs(args)
		return cmd
	}

	cmd := command("ws")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "ws": must specify action`)

	cmd = command("ws", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("foo")
	run.flags.Interactive = false
	c.Check(cmd.Execute(), check.ErrorMatches, `unclear if "foo" names a workshop or action: try "workshop run -- foo"`)

	cmd = command("--")
	c.Check(cmd.Execute(), check.ErrorMatches, `requires at least 1 arg\(s\), only received 0`)

	cmd = command("--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("ws", "--")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "ws": must specify action`)

	cmd = command("foo", "--")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "foo": must specify action`)

	cmd = command("ws", "--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("foo", "--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `workshop not found`)
}

func (m *workshopExec) TestMultipleWorkshopRunErrors(c *check.C) {
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			c.Check(r.Method, check.Equals, "POST")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case fmt.Sprintf("/v1/projects/%s/workshops", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList2)
		case fmt.Sprintf("/v1/projects/%s/workshops/ws/exec", m.prjId):
			c.Check(r.Method, check.Equals, "POST")
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopExecError)
		case fmt.Sprintf("/v1/projects/%s/workshops/foo/exec", m.prjId):
			c.Check(r.Method, check.Equals, "POST")
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopActionsError)
		default:
			c.Errorf("unexpected API call:", r.URL.Path)
		}
	})

	run := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	command := func(args ...string) *cobra.Command {
		cmd := run.Command()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs(args)
		return cmd
	}

	cmd := command("ws")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "ws": must specify action`)

	cmd = command("ws", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot infer workshop name: multiple workshops found: "ws", "ws2"`)

	cmd = command("--")
	c.Check(cmd.Execute(), check.ErrorMatches, `requires at least 1 arg\(s\), only received 0`)

	cmd = command("--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot infer workshop name: multiple workshops found: "ws", "ws2"`)

	cmd = command("ws", "--")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "ws": must specify action`)

	cmd = command("foo", "--")
	c.Check(cmd.Execute(), check.ErrorMatches, `cannot run action in "foo": must specify action`)

	cmd = command("ws", "--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `(?s).*\(action not found\)`)

	cmd = command("foo", "--", "foo")
	c.Check(cmd.Execute(), check.ErrorMatches, `workshop not found`)
}

func (m *workshopExec) TestSingleWorkshopRunCompletion(c *check.C) {
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			c.Check(r.Method, check.Equals, "POST")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case fmt.Sprintf("/v1/projects/%s/workshops", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithActions)
		default:
			c.Errorf("unexpected API call:", r.URL.Path)
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
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "w"}
	result, compDirective = run.ValidArgsFunction(run, nil, "w")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"ws"})

	os.Args = []string{"workshop", "__complete", "run", "f"}
	result, compDirective = run.ValidArgsFunction(run, nil, "f")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "zyx"}
	result, compDirective = run.ValidArgsFunction(run, nil, "zyx")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "ws", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)
}

func (m *workshopExec) TestRunCompletionUsesLocalSingleWorkshopActions(c *check.C) {
	writeRunCompletionWorkshopFile(c, m.prjDir, "workshop.yaml", "ws")

	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Errorf("unexpected API call: %s", r.URL.Path)
	})

	cmd := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	run := cmd.Command()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"workshop", "__complete", "run", "--", ""}
	result, compDirective := run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bootstrap", "build"})
}

func (m *workshopExec) TestRunCompletionUsesLocalNamedWorkshopActions(c *check.C) {
	c.Assert(os.Mkdir(filepath.Join(m.prjDir, workshop.Directory), 0755), check.IsNil)
	writeRunCompletionWorkshopFile(c, filepath.Join(m.prjDir, workshop.Directory), "ws1.yaml", "ws1")
	writeRunCompletionWorkshopFile(c, filepath.Join(m.prjDir, workshop.Directory), "ws2.yaml", "ws2")

	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Errorf("unexpected API call: %s", r.URL.Path)
	})

	cmd := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	run := cmd.Command()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"workshop", "__complete", "run", "ws1", "--", ""}
	result, compDirective := run.ValidArgsFunction(run, []string{"ws1", "--"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bootstrap", "build"})
}

func (m *workshopExec) TestRunCompletionFallsBackToDaemonForSingleWorkshopActions(c *check.C) {
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
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Ready"))
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithActions)
		default:
			c.Errorf("unexpected API call %d: %s", n, r.URL.Path)
		}
	})

	cmd := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	run := cmd.Command()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"workshop", "__complete", "run", "--", ""}
	result, compDirective := run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})
	c.Check(n, check.Equals, 3)
}

func (m *workshopExec) TestRunCompletionFallsBackToDaemonForNamedWorkshopActions(c *check.C) {
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
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithActions)
		default:
			c.Errorf("unexpected API call %d: %s", n, r.URL.Path)
		}
	})

	cmd := &CmdRun{root: &CmdRoot{cwd: m.prjDir}}
	run := cmd.Command()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", ""}
	result, compDirective := run.ValidArgsFunction(run, []string{"ws", "--"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})
	c.Check(n, check.Equals, 2)
}

func writeRunCompletionWorkshopFile(c *check.C, dir, filename, name string) {
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf(`name: %s
base: ubuntu@24.04
sdks:
  - name: go
    channel: latest/stable
actions:
  bootstrap: |
    cd /project
    ./scripts/build.sh bootstrap "$@"
  build: |
    cd /project
    ./scripts/build.sh build "$@"
`, name)
	c.Assert(os.WriteFile(path, []byte(content), 0644), check.IsNil)
}

func (m *workshopExec) TestMultipleWorkshopRunCompletion(c *check.C) {
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			c.Check(r.Method, check.Equals, "POST")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case fmt.Sprintf("/v1/projects/%s/workshops", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopList2)
		case fmt.Sprintf("/v1/projects/%s/workshops/ws/actions", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithActions)
		case fmt.Sprintf("/v1/projects/%s/workshops/foo/actions", m.prjId):
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(404)
			fmt.Fprintln(w, mockWorkshopActionsError)
		default:
			c.Errorf("unexpected API call:", r.URL.Path)
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

	os.Args = []string{"workshop", "__complete", "run", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "ws", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, nil, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
	c.Check(result, check.DeepEquals, []string{"bar", "foo"})

	os.Args = []string{"workshop", "__complete", "run", "ws", "--", "foo", ""}
	result, compDirective = run.ValidArgsFunction(run, []string{"ws", "--", "foo"}, "")
	c.Assert(compDirective, check.Equals, cobra.ShellCompDirectiveDefault)
	c.Check(result, check.HasLen, 0)
}
