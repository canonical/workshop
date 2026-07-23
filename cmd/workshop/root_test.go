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
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/testutil"
)

type BaseWorkshopSuite struct {
	testutil.BaseTest
	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func Test(t *testing.T) { check.TestingT(t) }

func (s *BaseWorkshopSuite) SetUpTest(*check.C) {
	path := os.Getenv("PATH")
	s.AddCleanup(func() {
		os.Setenv("PATH", path)
	})

	s.stdin = bytes.NewBuffer(nil)
	s.stdout = bytes.NewBuffer(nil)
	s.stderr = bytes.NewBuffer(nil)

	Stdin = s.stdin
	Stdout = s.stdout
	Stderr = s.stderr
}

func (s *BaseWorkshopSuite) TearDownTest(c *check.C) {
	Stdin = os.Stdin
	Stdout = os.Stdout
	Stderr = os.Stderr
}

func (s *BaseWorkshopSuite) RedirectClientToTestServer(handler func(http.ResponseWriter, *http.Request)) {
	server := httptest.NewServer(http.HandlerFunc(handler))
	s.AddCleanup(func() { server.Close() })
	ClientConfig.BaseURL = server.URL
	s.AddCleanup(func() { ClientConfig.BaseURL = "" })
}

func (s *BaseWorkshopSuite) ResetStdStreams() {
	s.stdin.Reset()
	s.stdout.Reset()
	s.stderr.Reset()
}

func (s *BaseWorkshopSuite) Stdout() string {
	return s.stdout.String()
}

func (s *BaseWorkshopSuite) Stderr() string {
	return s.stderr.String()
}

type rootSuite struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&rootSuite{})

func (s *rootSuite) SetUpTest(c *check.C) {
	s.prjDir = c.MkDir()
	s.prjId = "42424242"
	s.BaseWorkshopSuite.SetUpTest(c)
}

func (s *rootSuite) TestProjectNameCompletion(c *check.C) {
	otherPrj := c.MkDir()
	otherId := "24242424"

	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, check.Equals, "GET")
		c.Assert(r.URL.Path, check.Equals, "/v1/projects")
		fmt.Fprintf(w, `{"type": "sync", "result": [{"id":"%s","path":"%s"},{"id":"%s","path":"%s"}]}`, s.prjId, s.prjDir, otherId, otherPrj)
	})

	cmd := (&CmdRoot{}).Command()
	complete, ok := cmd.GetFlagCompletionFunc("project")
	c.Assert(ok, check.Equals, true)

	subcmds := cmd.Commands()
	idx := slices.IndexFunc(subcmds, func(c *cobra.Command) bool {
		return c.Name() == "launch"
	})
	c.Assert(idx, testutil.IntGreaterEqual, 0)

	completions, directive := complete(subcmds[idx], nil, "")
	c.Check(completions, check.HasLen, 0)
	c.Check(directive, check.Equals, cobra.ShellCompDirectiveFilterDirs)

	idx = slices.IndexFunc(subcmds, func(c *cobra.Command) bool {
		return c.Name() == "refresh"
	})
	c.Assert(idx, testutil.IntGreaterEqual, 0)

	completions, directive = complete(subcmds[idx], nil, "/")
	c.Check(completions, testutil.DeepUnsortedMatches, []string{s.prjDir, otherPrj})
	c.Check(directive, check.Equals, cobra.ShellCompDirectiveDefault)
}

func (s *rootSuite) TestWorkshopNameCompletion(c *check.C) {
	statuses := []string{"Ready", "Pending", "Waiting", "Stopped", "Error"}
	expected := make(map[string][]string)

	var wsInfo []*client.WorkshopInfo
	for i := range 20 {
		index := rand.Intn(len(statuses))
		status := statuses[index]
		info := &client.WorkshopInfo{
			ProjectId: "42424242",
			Name:      "test" + strconv.Itoa(i),
			Status:    status,
		}
		wsInfo = append(wsInfo, info)
		expected[status] = append(expected[status], info.Name)
	}

	w := client.Workshops{
		Workshops: wsInfo,
	}

	cmd := &CmdRoot{cwd: s.prjDir}

	s.listRedirectHelper(c, w, s.prjId, s.prjDir, len(statuses)*8)

	for _, st := range statuses {
		single := cmd.completeWorkshopName([]string{st})
		multiple := cmd.completeWorkshopNames([]string{st})

		result, compDirective := single(cmd.Command(), nil, "")
		c.Check(result, check.DeepEquals, expected[st])
		c.Check(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)

		if len(expected[st]) > 0 {
			result, compDirective = single(cmd.Command(), expected[st][:1], "")
			c.Check(result, check.HasLen, 0)
			c.Check(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
		}

		result, compDirective = multiple(cmd.Command(), nil, "")
		c.Check(result, check.DeepEquals, expected[st])
		c.Check(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)

		if len(expected[st]) > 0 {
			result, compDirective = multiple(cmd.Command(), expected[st][:1], "")
			rest := expected[st][1:]
			if len(rest) == 0 {
				rest = nil
			}
			c.Check(result, check.DeepEquals, rest)
			c.Check(compDirective, check.Equals, cobra.ShellCompDirectiveNoFileComp)
		}
	}
}

func (m *BaseWorkshopSuite) listRedirectHelper(c *check.C, w client.Workshops, prjId, prjDir string, expected int) {
	workshops, err := json.Marshal(w)
	c.Assert(err, check.IsNil)

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch {
		case n%2 != 0 && n <= expected:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, prjId, prjDir)
			fmt.Fprintln(w, r)
		case n%2 == 0 && n <= expected:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects/42424242/workshops")
			rsp := fmt.Sprintf(`{"type": "sync", "result": %s}`, workshops)
			fmt.Fprintln(w, rsp)
		}
	})
}

func (m *BaseWorkshopSuite) connectionsRedirectHelper(c *check.C, conns client.Connections, prjId, prjDir string, expected int) {
	connections, err := json.Marshal(conns)
	c.Assert(err, check.IsNil)

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch {
		case n%2 != 0 && n <= expected:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, prjId, prjDir)
			fmt.Fprintln(w, r)
		case n%2 == 0 && n <= expected:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/connections")
			r := fmt.Sprintf(`{"type": "sync", "result": %s}`, connections)
			fmt.Fprintln(w, r)
		default:
			c.Errorf("expected %d calls, now on %d", expected, n)
		}
	})
}

// EncodeResponseBody writes JSON-serialized body to the response writer.
func EncodeResponseBody(c *check.C, w http.ResponseWriter, body any) {
	encoder := json.NewEncoder(w)
	err := encoder.Encode(body)
	c.Assert(err, check.IsNil)
}

// DecodedRequestBody returns the JSON-decoded body of the request.
func DecodedRequestBody(c *check.C, r *http.Request) map[string]any {
	var body map[string]any
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	err := decoder.Decode(&body)
	c.Assert(err, check.IsNil)
	return body
}

func testPlugsSlots(projectId string) ([]client.Plug, []client.Slot) {
	plugs := []client.Plug{
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "desktop",
			Interface: "desktop",
		},
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "ssh-agent",
			Interface: "ssh-agent",
		},
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "mount",
			Interface: "mount",
		},
		{
			ProjectId: projectId,
			Workshop:  "another-workshop",
			Sdk:       "sdk",
			Name:      "desktop",
			Interface: "desktop",
		},
	}

	slots := []client.Slot{
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "desktop",
			Interface: "desktop",
		},
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "ssh-agent",
			Interface: "ssh-agent",
		},
		{
			ProjectId: projectId,
			Workshop:  "workshop",
			Sdk:       "sdk",
			Name:      "mount",
			Interface: "mount",
		},
		{
			ProjectId: projectId,
			Workshop:  "another-workshop",
			Sdk:       "sdk",
			Name:      "desktop",
			Interface: "desktop",
		},
	}
	return plugs, slots
}

func plugSlotToConn(plug client.Plug, slot client.Slot, manual bool) client.Connection {
	return client.Connection{
		Plug: client.PlugRef{
			ProjectId: plug.ProjectId,
			Workshop:  plug.Workshop,
			Sdk:       plug.Sdk,
			Name:      plug.Name,
		},
		Slot: client.SlotRef{
			ProjectId: slot.ProjectId,
			Workshop:  slot.Workshop,
			Sdk:       slot.Sdk,
			Name:      slot.Name,
		},
		Interface: plug.Interface,
		Manual:    manual,
	}
}

func mockSingleWorkshopSpecifyStatus(status string) string {
	return fmt.Sprintf(`{"type":"sync","status-code":200,"status":"OK","result":{
      "workshops":[{
          "name":"ws",
          "base":"ubuntu@22.04",
          "project-id":"42424242",
          "status":%q,
          "notes":["missing-project"
          ]}
      ]}
  }`, status,
	)
}
