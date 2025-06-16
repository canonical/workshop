/*
 * Copyright (C) 2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func TestT(t *testing.T) { check.TestingT(t) }

type workshopctlSuite struct {
	server            *httptest.Server
	oldArgs           []string
	expectedContextID string
	expectedArgs      []string
	expectedStdin     []byte
}

var _ = check.Suite(&workshopctlSuite{})

func (s *workshopctlSuite) SetUpTest(c *check.C) {
	os.Setenv("WORKSHOP_COOKIE", "workshop-context-test")
	n := 0
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n {
		case 0:
			c.Assert(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/workshopctl")
			c.Assert(r.Header.Get("Authorization"), check.Equals, "")

			var workshopctlPostData client.WorkshopCtlPostData
			decoder := json.NewDecoder(r.Body)
			c.Assert(decoder.Decode(&workshopctlPostData), check.IsNil)
			c.Assert(workshopctlPostData.ContextID, check.Equals, s.expectedContextID)
			c.Assert(workshopctlPostData.Args, check.DeepEquals, s.expectedArgs)
			c.Assert(workshopctlPostData.Stdin, check.DeepEquals, s.expectedStdin)

			fmt.Fprintln(w, `{"type": "sync", "result": {"stdout": "test stdout", "stderr": "test stderr"}}`)
		default:
			c.Fatalf("expected to get 1 request, now on %d", n+1)
		}

		n++
	}))
	clientConfig.BaseURL = s.server.URL
	s.oldArgs = os.Args
	os.Args = []string{"workshopctl"}
	s.expectedContextID = "workshop-context-test"
	s.expectedArgs = []string{}
}

func (s *workshopctlSuite) TearDownTest(c *check.C) {
	c.Assert(os.Unsetenv("WORKSHOP_COOKIE"), check.IsNil)
	clientConfig.BaseURL = ""
	s.server.Close()
	os.Args = s.oldArgs
}

func (s *workshopctlSuite) TestWorkshopctl(c *check.C) {
	stdout, stderr, err := run(nil)
	c.Check(err, check.IsNil)
	c.Check(string(stdout), check.Equals, "test stdout")
	c.Check(string(stderr), check.Equals, "test stderr")
}

func (s *workshopctlSuite) TestWorkshopctlWithArgs(c *check.C) {
	os.Args = []string{"workshopctl", "foo", "--bar"}

	s.expectedArgs = []string{"foo", "--bar"}
	stdout, stderr, err := run(nil)
	c.Check(err, check.IsNil)
	c.Check(string(stdout), check.Equals, "test stdout")
	c.Check(string(stderr), check.Equals, "test stderr")
}

func (s *workshopctlSuite) TestWorkshopctlHelp(c *check.C) {
	os.Unsetenv("WORKSHOP_COOKIE")
	s.expectedContextID = ""

	os.Args = []string{"workshopctl", "-h"}
	s.expectedArgs = []string{"-h"}

	_, _, err := run(nil)
	c.Check(err, check.IsNil)
}

func (s *workshopctlSuite) TestWorkshopctlWithStdin(c *check.C) {
	s.expectedStdin = []byte("hello")
	mockStdin := bytes.NewBuffer(s.expectedStdin)

	_, _, err := run(mockStdin)
	c.Check(err, check.IsNil)
}
