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
	"fmt"
	"net/http"
	"os/user"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
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

func (m *workshopRemove) TestRemoveSuccess(c *check.C) {
	for _, name := range []string{"remove", "rm"} {
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

		cmd := (&CmdRoot{}).Command()
		cmd.SetArgs([]string{name, "ws", "ws-1", "ws"})
		err := cmd.Execute()
		c.Assert(err, check.IsNil)
		c.Assert(m.stdout.String(), check.Matches, `"ws" removed\n"ws-1" removed\n`)
		c.Check(n, check.Equals, 3)
		m.ResetStdStreams()
	}
}
