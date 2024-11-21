// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/check.v1"
)

type warningSuite struct {
	BaseWorkshopSuite
	cmdW *CmdWarnings
	cmdO *CmdOkay
}

var _ = check.Suite(&warningSuite{})

func (m *warningSuite) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)
	m.cmdW = &CmdWarnings{root: &CmdRoot{}}
	m.cmdO = &CmdOkay{root: m.cmdW.root}

	os.Setenv("WORKSHOPD_LAST_WARNING_TIMESTAMP_FILENAME", filepath.Join(c.MkDir(), "warnings.json"))
}

const twoWarnings = `{
			"result": [
			    {
				"expire-after": "672h0m0s",
				"first-added": "2018-09-19T12:41:18.505007495Z",
				"last-added": "2018-09-19T12:41:18.505007495Z",
				"message": "hello world number one",
				"repeat-after": "24h0m0s"
			    },
			    {
				"expire-after": "672h0m0s",
				"first-added": "2018-09-19T12:44:19.680362867Z",
				"last-added": "2018-09-19T12:44:19.680362867Z",
				"message": "hello world number two",
				"repeat-after": "24h0m0s"
			    }
			],
			"status": "OK",
			"status-code": 200,
			"type": "sync"
		}`

func mkWarningsFakeHandler(c *check.C, body string) func(w http.ResponseWriter, r *http.Request) {
	var called bool
	return func(w http.ResponseWriter, r *http.Request) {
		if called {
			c.Fatalf("expected a single request")
		}
		called = true
		c.Check(r.URL.Path, check.Equals, "/v1/warnings")
		c.Check(r.URL.Query(), check.HasLen, 0)

		buf, err := io.ReadAll(r.Body)
		c.Assert(err, check.IsNil)
		c.Check(string(buf), check.Equals, "")
		c.Check(r.Method, check.Equals, "GET")
		w.WriteHeader(200)
		fmt.Fprintln(w, body)
	}
}

func (s *warningSuite) TestNoWarningsEver(c *check.C) {
	s.RedirectClientToTestServer(mkWarningsFakeHandler(c, `{"type": "sync", "status-code": 200, "result": []}`))

	s.cmdW.AbsTime = true
	err := s.cmdW.Run(s.cmdW.Command(), nil)
	c.Assert(err, check.IsNil)
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, "No warnings.\n")
}

func (s *warningSuite) TestNoFurtherWarnings(c *check.C) {
	WriteWarningTimestamp(time.Now())

	s.RedirectClientToTestServer(mkWarningsFakeHandler(c, `{"type": "sync", "status-code": 200, "result": []}`))

	s.cmdW.AbsTime = true
	err := s.cmdW.Run(s.cmdW.Command(), nil)
	c.Assert(err, check.IsNil)
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, "No further warnings.\n")
}

func (s *warningSuite) TestWarnings(c *check.C) {
	s.RedirectClientToTestServer(mkWarningsFakeHandler(c, twoWarnings))
	cmd := s.cmdW.Command()
	s.cmdW.AbsTime = true
	s.cmdW.Unicode = "never"
	err := s.cmdW.Run(cmd, nil)
	c.Assert(err, check.IsNil)
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, `
last-occurrence:  2018-09-19T12:41:18Z
warning: |
  hello world number one
---
last-occurrence:  2018-09-19T12:44:19Z
warning: |
  hello world number two
`[1:])
}

func (s *warningSuite) TestVerboseWarnings(c *check.C) {
	s.RedirectClientToTestServer(mkWarningsFakeHandler(c, twoWarnings))
	cmd := s.cmdW.Command()
	s.cmdW.AbsTime = true
	s.cmdW.Verbose = true
	s.cmdW.Unicode = "never"
	err := s.cmdW.Run(cmd, nil)
	c.Assert(err, check.IsNil)
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, `
first-occurrence:  2018-09-19T12:41:18Z
last-occurrence:   2018-09-19T12:41:18Z
acknowledged:      --
repeats-after:     1d00h
expires-after:     28d0h
warning: |
  hello world number one
---
first-occurrence:  2018-09-19T12:44:19Z
last-occurrence:   2018-09-19T12:44:19Z
acknowledged:      --
repeats-after:     1d00h
expires-after:     28d0h
warning: |
  hello world number two
`[1:])
}

func (s *warningSuite) TestOkay(c *check.C) {
	t0 := time.Now()
	WriteWarningTimestamp(t0)

	var n int
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n != 1 {
			c.Fatalf("expected 1 request, now on %d", n)
		}
		c.Check(r.URL.Path, check.Equals, "/v1/warnings")
		c.Check(r.URL.Query(), check.HasLen, 0)
		c.Assert(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "okay", "timestamp": t0.Format(time.RFC3339Nano)})
		c.Check(r.Method, check.Equals, "POST")
		w.WriteHeader(200)
		fmt.Fprintln(w, `{
			"status": "OK",
			"status-code": 200,
			"type": "sync"
		}`)
	})

	err := s.cmdO.Run(s.cmdO.Command(), nil)
	c.Assert(err, check.IsNil)
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, "")
}

func (s *warningSuite) TestOkayBeforeWarnings(c *check.C) {
	err := s.cmdO.Run(s.cmdO.Command(), nil)
	c.Assert(err, check.ErrorMatches, "you must have looked at the warnings before acknowledging them. Try 'workshop warnings'.")
	c.Check(s.Stderr(), check.Equals, "")
	c.Check(s.Stdout(), check.Equals, "")
}

func (s *warningSuite) TestListWithWarnings(c *check.C) {
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, "42424242", "/home/project")
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects/42424242/workshops")
			buf, err := io.ReadAll(r.Body)
			c.Assert(err, check.IsNil)
			c.Check(string(buf), check.Equals, "")
			c.Check(r.Method, check.Equals, "GET")
			w.WriteHeader(200)
			fmt.Fprintln(w, `{
					"result": [{}],
					"status": "OK",
					"status-code": 200,
					"type": "sync",
					"warning-count": 2,
					"warning-timestamp": "2018-09-19T12:44:19.680362867Z"
				}`)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}

	})
	cmdList := &CmdList{root: &CmdRoot{}}
	err := cmdList.runList()
	c.Assert(err, check.IsNil)

	{
		// TODO: I hope to get to refactor run() so we can
		// call it from tests and not have to do this (whole
		// block) by hand

		cli, err := cmdList.root.client()
		c.Assert(err, check.IsNil)
		count, stamp := cli.WarningsSummary()
		c.Check(count, check.Equals, 2)
		c.Check(stamp, check.Equals, time.Date(2018, 9, 19, 12, 44, 19, 680362867, time.UTC))
		maybePresentWarnings(count, stamp)

	}

	c.Check(s.Stdout(), check.Equals, `
Project        Workshop  Status  Notes
/home/project                    -
`[1:])
	c.Check(s.Stderr(), check.Equals, "WARNING: There are 2 new warnings. See 'workshop warnings'.\n")

}
