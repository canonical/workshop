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
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"
)

type workshopInfo struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&workshopInfo{})

func (m *workshopInfo) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

var mockWorkshopWithSdks = `{"type":"sync","status-code":200,"status":"OK","result":{
    "name":"ws",
    "base":"ubuntu@22.04",
    "confinement":"container",
    "project-id":"42424242",
    "status":"Error",
    "hostname":"ws.sdkcraft.wp",
    "sdks":[{
      "name":"go",
      "version":"1.8.0",
      "channel":"latest/edge",
      "revision":"1",
      "built-at":"2017-02-19T17:23:05.592623Z",
      "installed-at":"2017-03-22T09:01:00.0Z"
    },{  
      "name":"sketch",
      "source":"%s/.local/share/workshop/id/42424242/ws/sdk/sketch/current",
      "revision":"x1",
      "installed-at":"2017-03-22T09:01:00.0Z"
    }],
    "notes":["missing-project"],
    "path":"/home/project/.workshop/ws.yaml"
},
"warning-timestamp":"2017-03-22T10:01:00.0Z",
"warning-count":1}`

func (m *workshopInfo) TestWorkshopInfo(c *check.C) {
	cmd := &CmdInfo{root: &CmdRoot{}}
	workshop := "ws"
	home, err := os.UserHomeDir()
	c.Assert(err, check.IsNil)
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
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus("Error"))
		case 3:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, fmt.Sprintf(mockWorkshopWithSdks, home))
		default:
			c.Errorf("expected 3 calls, now on %d", n)
		}
	})

	err = cmd.Run(cmd.Command(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(`name:         ws
base:         ubuntu@22.04
project:      %s
hostname:     ws\.sdkcraft\.wp
status:       error
confinement:  container
notes:        missing-project
sdks:
  go:
    tracking:   latest/edge
    installed:  1.8.0  2017-02-19  \(1\)
  sketch:
    tracking:   ~/.local/share/workshop/id/42424242/ws/sdk/sketch/current
    installed:  2017-03-22  \(x1\)
`, m.prjDir))
	c.Check(n, check.Equals, 3)
}

var mockWorkshopWithHealth = `{"type":"sync","status-code":200,"status":"OK","result":{
    "name":"ws",
    "base":"ubuntu@22.04",
    "confinement":"container",
    "project-id":"42424242",
    "status":"Pending",
    "notes":["workshop-note"],
    "sdks":[{
        "name":"go",
        "version":"1.8.0",
        "channel":"latest/edge",
        "revision":"1",
        "built-at":"2017-02-19T17:23:05.592623Z",
        "installed-at":"2017-03-22T09:01:00.0Z",
        "health-check":{"message":"Waiting for all required modules to be installed","code":"try-later"}
    }]
}}`

func (m *workshopInfo) TestWorkshopInfoWithSdkHealthReport(c *check.C) {
	cmd := &CmdInfo{root: &CmdRoot{}}
	workshop := "ws"
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
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithHealth)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{workshop})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(`name:         ws
base:         ubuntu@22.04
project:      %s
status:       pending
confinement:  container
notes:        workshop-note,try-later
sdks:
  go:
    tracking:   latest/edge
    installed:  1.8.0  2017-02-19  \(1\)
    message:    Waiting for all required modules to be installed
`, m.prjDir))
	c.Check(n, check.Equals, 2)
}

var mockWorkshopWithMounts = `{"type":"sync","status-code":200,"status":"OK","result":{
    "name":"ws",
    "base":"ubuntu@22.04",
    "confinement":"container",
    "project-id":"42424242",
    "status":"Ready",
    "sdks":[{
        "name":"go",
        "version":"1.8.0",
        "channel":"latest/edge",
        "revision":"1",
        "built-at":"2017-02-19T17:23:05.592623Z",
        "installed-at":"2017-03-22T09:01:00.0Z",
        "mounts":[{
            "host-source":"/home/user/src",
            "workshop-target":"/home/workshop/target", 
            "plug":{
                "project-id":"42ws42ws",
                "workshop":"ws",
                "sdk":"go",
                "plug":"plug-name"
            }
        },{
            "host-source":"%s/workshop/id/17942561/ws/mount/go/mod-cache",
            "workshop-target":"/home/workshop/target", 
            "plug":{
                "project-id":"42ws42ws",
                "workshop":"ws",
                "sdk":"go",
                "plug":"plug-default"
            }
        }]
    }]
}}`

var mockWorkshopWithMountsOutput = `name:         ws
base:         ubuntu@22.04
project:      %s
status:       ready
confinement:  container
notes:        %s
sdks:
  go:
    tracking:   latest/edge
    installed:  1.8.0  2017-02-19  \(1\)
    mounts:
      plug-default:
        host-source:      %s
        workshop-target:  /home/workshop/target
      plug-name:
        host-source:      /home/user/src
        workshop-target:  /home/workshop/target
`

func (m *workshopInfo) TestWorkshopInfoWithSdkMountsXdgUnset(c *check.C) {
	cmd := &CmdInfo{root: &CmdRoot{}}
	workshop := "ws"
	n := 0
	home := "/home/testuser"
	xdg := filepath.Join(home, ".local", "share")
	defer os.Setenv("XDG_DATA_HOME", os.Getenv("XDG_DATA_HOME"))
	os.Setenv("XDG_DATA_HOME", "")
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)

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
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, fmt.Sprintf(mockWorkshopWithMounts, xdg))
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{workshop})
	c.Assert(err, check.IsNil)
	hostSource := "~/.local/share/workshop/id/17942561/ws/mount/go/mod-cache"
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(mockWorkshopWithMountsOutput, m.prjDir, "--", hostSource))
	c.Check(n, check.Equals, 2)
}

func (m *workshopInfo) TestWorkshopInfoWithSdkMountsXdgSet(c *check.C) {
	cmd := &CmdInfo{root: &CmdRoot{}}
	workshop := "ws"
	n := 0
	home := "/home/testuser"
	xdg := filepath.Join(home, "xdghomedir")
	defer os.Setenv("XDG_DATA_HOME", os.Getenv("XDG_DATA_HOME"))
	os.Setenv("XDG_DATA_HOME", xdg)
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)

	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, fmt.Sprintf(mockWorkshopWithMounts, xdg))
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{workshop})
	c.Assert(err, check.IsNil)
	hostSource := "~/xdghomedir/workshop/id/17942561/ws/mount/go/mod-cache"
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(mockWorkshopWithMountsOutput, m.prjDir, "--", hostSource))
	c.Check(n, check.Equals, 2)

	cmd.Color = "always"
	cmd.Unicode = "always"
	m.ResetStdStreams()

	err = cmd.Run(cmd.Command(), []string{workshop})
	c.Assert(err, check.IsNil)
	hostSource = "\033]8;;file://.*/home/testuser/xdghomedir/workshop/id/17942561/ws/mount/go/mod-cache\033\\\\…/17942561/ws/mount/go/mod-cache\033]8;;\033\\\\"
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(mockWorkshopWithMountsOutput, m.prjDir, "–", hostSource))
	c.Check(n, check.Equals, 4)
}

var mockWorkshopWithTunnels = `{
  "type": "sync",
  "status-code": 200,
  "status": "OK",
  "result": {
    "name": "ws",
    "base": "ubuntu@22.04",
    "confinement": "container",
    "project-id": "42424242",
    "status": "Ready",
    "sdks": [
      {
        "name": "system",
        "revision": "1",
        "tunnels": [
          {
            "plug": {
              "project-id": "42ws42ws",
              "workshop": "ws",
              "sdk": "system",
              "plug": "gopls"
            },
            "from": {
              "protocol": "tcp",
              "host": "127.0.0.1",
              "port": 60915
            },
            "to": {
              "protocol": "unix",
              "path": "/run/user/%s/gopls.socket"
            }
          }
        ]
      },
      {
        "name": "go",
        "version": "1.8.0",
        "channel": "latest/edge",
        "revision": "1",
        "built-at": "2017-02-19T17:23:05.592623Z",
        "installed-at": "2017-03-22T09:01:00.0Z",
        "tunnels": [
          {
            "plug": {
              "project-id": "42ws42ws",
              "workshop": "ws",
              "sdk": "go",
              "plug": "snap-cache"
            },
            "from": {
              "protocol": "tcp",
              "host": "::",
              "port": 12345
            },
            "to": {
              "protocol": "unix",
              "path": "@snap-proxy"
            }
          }
        ]
      }
    ]
  }
}`

func (m *workshopInfo) TestWorkshopInfoWithSdkTunnels(c *check.C) {
	cmd := &CmdInfo{root: &CmdRoot{}}
	workshop := "ws"
	n := 0
	user, err := user.Current()
	c.Assert(err, check.IsNil)
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			_, err = fmt.Fprintln(w, r)
			c.Assert(err, check.IsNil)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			_, err = fmt.Fprintln(w, fmt.Sprintf(mockWorkshopWithTunnels, user.Uid))
			c.Assert(err, check.IsNil)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err = cmd.Run(cmd.Command(), []string{workshop})
	c.Assert(err, check.IsNil)
	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(`name:         ws
base:         ubuntu@22.04
project:      %s
status:       ready
confinement:  container
notes:        --
sdks:
  system:
    installed:  \(1\)
    tunnels:
      gopls:
        from:  127.0.0.1:60915/tcp
        to:    /run/user/%s/gopls.socket
  go:
    tracking:   latest/edge
    installed:  1.8.0  2017-02-19  \(1\)
    tunnels:
      snap-cache:
        from:  '\[::\]:12345/tcp'
        to:    '@snap-proxy'
`, m.prjDir, user.Uid))
	c.Check(n, check.Equals, 2)
}
