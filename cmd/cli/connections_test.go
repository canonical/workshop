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

package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"gopkg.in/check.v1"
	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

type connectionsSuite struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&connectionsSuite{})

func (m *connectionsSuite) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

func (s *connectionsSuite) TestConnectionsNoneConnected(c *check.C) {
	n := 0
	query := url.Values{"project-id": []string{"42424242"}}
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			result := client.Connections{}
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{})

	c.Check(err, check.IsNil)
	c.Assert(s.Stdout(), check.Equals, "")
	c.Assert(s.Stderr(), check.Equals, "")

	s.ResetStdStreams()

	query = url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
	}

	allCmd := cmd.Command()
	cmd.all = true
	err = cmd.Run(allCmd, []string{})
	c.Check(err, IsNil)
	c.Assert(s.Stdout(), check.Equals, "")
	c.Assert(s.Stderr(), check.Equals, "")
}

func (s *connectionsSuite) TestConnectionsNotInstalled(c *C) {
	query := url.Values{
		"project-id": []string{"42424242"},
		"workshop":   []string{"foo"},
		"select":     []string{"all"},
	}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, Equals, "GET")
			c.Check(r.URL.Path, Equals, "/v1/connections")
			c.Check(r.URL.Query(), DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, IsNil)
			c.Check(body, DeepEquals, []byte{})
			fmt.Fprintln(w, `{"type": "error", "result": {"message": "not found"}, "status-code": 404}`)
		}
	})
	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{"foo"})
	c.Check(err, ErrorMatches, `not found`)
	c.Assert(s.Stdout(), Equals, "")
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsNoneConnectedPlugs(c *C) {
	query := url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
	}
	result := client.Connections{
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "capslock-led",
				Interface: "leds",
			},
		},
	}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	cmd := &CmdConnections{root: &CmdRoot{}}
	command := cmd.Command()
	cmd.all = true
	err := cmd.Run(command, []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                                 Slot  Notes\n" +
		"leds       keyboard-lights/lights:capslock-led  -     -\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")

	s.ResetStdStreams()

	query = url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
		"workshop":   []string{"keyboard-lights"},
	}

	err = cmd.Run(cmd.Command(), []string{"keyboard-lights"})
	c.Assert(err, IsNil)
	expectedStdout = "" +
		"Interface  Plug                                 Slot  Notes\n" +
		"leds       keyboard-lights/lights:capslock-led  -     -\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsNoneConnectedSlots(c *C) {
	result := client.Connections{}
	query := url.Values{"project-id": []string{"42424242"}, "select": []string{"all"}, "workshop": []string{"foo"}}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})
	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{"foo"})
	c.Check(err, IsNil)
	c.Assert(s.Stdout(), check.Equals, "")
	c.Assert(s.Stderr(), check.Equals, "")

	s.ResetStdStreams()

	result = client.Connections{
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
			},
		},
	}
	err = cmd.Run(cmd.Command(), []string{"foo"})
	c.Assert(err, check.IsNil)
	expectedStdout := "" +
		"Interface  Plug  Slot                                 Notes\n" +
		"leds       -     leds-provider/provider:capslock-led  -\n"
	c.Assert(s.Stdout(), check.Equals, expectedStdout)
	c.Assert(s.Stderr(), check.Equals, "")
}

func (s *connectionsSuite) TestConnectionsSomeConnected(c *C) {
	result := client.Connections{
		Established: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "capslock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "leds-provider", Sdk: "provider", Name: "capslock-led"},
				Interface: "leds",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "numlock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "numlock-led"},
				Interface: "leds",
				Manual:    true,
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "scrollock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "scrollock-led"},
				Interface: "leds",
			},
		},
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "capslock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "leds-provider",
					Sdk:       "provider",
					Name:      "capslock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "numlock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "system",
					Name:      "numlock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "scrollock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "system",
					Name:      "scrollock-led",
				}},
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "numlock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "numlock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "scrollock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "scrollock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "capslock",
				}},
			},
		},
	}
	n := 0
	query := url.Values{"project-id": []string{"42424242"}}
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})
	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                              Slot                                 Notes\n" +
		"leds       keyboard-lights/lights:capslock   leds-provider/provider:capslock-led  -\n" +
		"leds       keyboard-lights/lights:numlock    :numlock-led                         manual\n" +
		"leds       keyboard-lights/lights:scrollock  :scrollock-led                       -\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsSomeConnectedBound(c *C) {
	result := client.Connections{
		Established: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "capslock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "leds-provider", Sdk: "provider", Name: "capslock-led"},
				Interface: "leds",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "numlock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "numlock-led"},
				Interface: "leds",
				Manual:    true,
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "scrollock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "scrollock-led"},
				Interface: "leds",
				Manual:    true,
			},
		},
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "capslock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "leds-provider",
					Sdk:       "provider",
					Name:      "capslock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "numlock",
				Interface: "leds",
				Bind: &client.PlugRef{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "scrollock",
				},
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "system",
					Name:      "numlock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "scrollock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "system",
					Name:      "scrollock-led",
				}},
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "numlock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "numlock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "scrollock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "scrollock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "capslock",
				}},
			},
		},
	}
	n := 0
	query := url.Values{"project-id": []string{"42424242"}}
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})
	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                              Slot                                 Notes\n" +
		"leds       keyboard-lights/lights:capslock   leds-provider/provider:capslock-led  -\n" +
		"leds       keyboard-lights/lights:numlock    :numlock-led                         manual,bind.3\n" +
		"leds       keyboard-lights/lights:scrollock  :scrollock-led                       manual,bind.3\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsSomeDisconnected(c *C) {
	result := client.Connections{
		Established: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "scrollock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "scrollock-led"},
				Interface: "leds",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "capslock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "leds-provider", Sdk: "provider", Name: "capslock-led"},
				Interface: "leds",
			},
		},
		Undesired: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "numlock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "numlock-led"},
				Interface: "leds",
				Manual:    true,
			},
		},
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "capslock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "leds-provider",
					Sdk:       "provider",
					Name:      "capslock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "numlock",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "scrollock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "system",
					Name:      "scrollock-led",
				}},
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "system",
				Name:      "capslock-led",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "numlock-led",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "scrollock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "scrollock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "capslock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "numlock-provider",
				Name:      "numlock-led",
				Interface: "leds",
			},
		},
	}
	n := 0
	query := url.Values{"project-id": []string{"42424242"}, "select": []string{"all"}}
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	cmd := &CmdConnections{root: &CmdRoot{}}
	cmdAll := cmd.Command()
	cmd.all = true
	err := cmd.Run(cmdAll, []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                              Slot                                          Notes\n" +
		"leds       -                                 :capslock-led                                 -\n" +
		"leds       -                                 :numlock-led                                  -\n" +
		"leds       -                                 keyboard-lights/numlock-provider:numlock-led  -\n" +
		"leds       keyboard-lights/lights:capslock   leds-provider/provider:capslock-led           -\n" +
		"leds       keyboard-lights/lights:numlock    -                                             -\n" +
		"leds       keyboard-lights/lights:scrollock  :scrollock-led                                -\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsSomeDisconnectedBound(c *C) {
	result := client.Connections{
		Established: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "capslock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "leds-provider", Sdk: "provider", Name: "capslock-led"},
				Interface: "leds",
			},
		},
		Undesired: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "numlock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "system", Name: "numlock-led"},
				Interface: "leds",
				Manual:    true,
			},
		},
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "capslock",
				Interface: "leds",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "leds-provider",
					Sdk:       "provider",
					Name:      "capslock-led",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "numlock",
				Interface: "leds",
				Bind: &client.PlugRef{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "scrollock",
				},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "lights",
				Name:      "scrollock",
				Interface: "leds",
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "system",
				Name:      "leds",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "numlock-led",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "system",
				Name:      "scrollock-led",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "keyboard-lights",
					Sdk:       "lights",
					Name:      "capslock",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "keyboard-lights",
				Sdk:       "numlock-provider",
				Name:      "numlock-led",
				Interface: "leds",
			},
		},
	}
	n := 0
	query := url.Values{"project-id": []string{"42424242"}, "select": []string{"all"}}
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	cmd := &CmdConnections{root: &CmdRoot{}}
	cmdAll := cmd.Command()
	cmd.all = true
	err := cmd.Run(cmdAll, []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                              Slot                                          Notes\n" +
		"leds       -                                 :numlock-led                                  -\n" +
		"leds       -                                 :scrollock-led                                -\n" +
		"leds       -                                 keyboard-lights/numlock-provider:numlock-led  -\n" +
		"leds       keyboard-lights/lights:capslock   leds-provider/provider:capslock-led           -\n" +
		"leds       keyboard-lights/lights:numlock    -                                             bind.6\n" +
		"leds       keyboard-lights/lights:scrollock  -                                             bind.6\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsOnlyDisconnected(c *C) {
	result := client.Connections{
		Undesired: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "keyboard-lights", Sdk: "lights", Name: "numlock"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "leds-provider", Sdk: "numlock-provider", Name: "numlock-led"},
				Interface: "leds",
				Manual:    true,
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "provider",
				Name:      "capslock-led",
				Interface: "leds",
			}, {
				ProjectId: "42424242",
				Workshop:  "leds-provider",
				Sdk:       "numlock-provider",
				Name:      "numlock-led",
				Interface: "leds",
			},
		},
	}
	query := url.Values{
		"project-id": []string{"42424242"},
		"workshop":   []string{"leds-provider"},
		"select":     []string{"all"},
	}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	cmd := &CmdConnections{root: &CmdRoot{}}
	err := cmd.Run(cmd.Command(), []string{"leds-provider"})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug  Slot                                        Notes\n" +
		"leds       -     leds-provider/numlock-provider:numlock-led  -\n" +
		"leds       -     leds-provider/provider:capslock-led         -\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}

func (s *connectionsSuite) TestConnectionsFiltering(c *C) {
	result := client.Connections{}
	query := url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
	}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})

	query = url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
		"workshop":   []string{"mouse-buttons"},
	}
	cmd := &CmdConnections{root: &CmdRoot{}}

	err := cmd.Run(cmd.Command(), []string{"mouse-buttons"})
	c.Assert(err, IsNil)

	cmdAll := cmd.Command()
	cmd.all = true
	err = cmd.Run(cmdAll, []string{"mouse-buttons"})
	c.Assert(err, ErrorMatches, "cannot use --all with workshop name")
}

func (s *connectionsSuite) TestConnectionsSorting(c *C) {
	result := client.Connections{
		Established: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "foo", Name: "plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "a-content-provider", Name: "data"},
				Interface: "mount",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "foo", Name: "plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "b-content-provider", Name: "data"},
				Interface: "mount",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "foo", Name: "desktop-plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "system", Name: "desktop"},
				Interface: "desktop",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "foo", Name: "x11-plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "system", Name: "x11"},
				Interface: "x11",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "def", Sdk: "foo", Name: "a-x11-plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "def", Sdk: "system", Name: "x11"},
				Interface: "x11",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "a-foo", Name: "plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "a-content-provider", Name: "data"},
				Interface: "mount",
			}, {
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "def", Sdk: "keyboard-app", Name: "x11"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "def", Sdk: "system", Name: "x11"},
				Interface: "x11",
				Manual:    true,
			},
		},
		Undesired: []client.Connection{
			{
				Plug:      client.PlugRef{ProjectId: "42424242", Workshop: "abc", Sdk: "foo", Name: "plug"},
				Slot:      client.SlotRef{ProjectId: "42424242", Workshop: "abc", Sdk: "c-content-provider", Name: "data"},
				Interface: "mount",
				Manual:    true,
			},
		},
		Plugs: []client.Plug{
			{
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "foo",
				Name:      "plug",
				Interface: "mount",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "a-content-provider",
					Name:      "data",
				}, {
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "b-content-provider",
					Name:      "data",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "foo",
				Name:      "desktop-plug",
				Interface: "desktop",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "system",
					Name:      "desktop",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "foo",
				Name:      "x11-plug",
				Interface: "x11",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "system",
					Name:      "x11",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "foo",
				Name:      "a-x11-plug",
				Interface: "x11",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "system",
					Name:      "x11",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "a-foo",
				Name:      "plug",
				Interface: "mount",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "a-content-provider",
					Name:      "data",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "def",
				Sdk:       "keyboard-app",
				Name:      "x11",
				Interface: "x11",
				Connections: []client.SlotRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "system",
					Name:      "x11",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "keyboard-lights",
				Name:      "numlock",
				Interface: "leds",
			},
		},
		Slots: []client.Slot{
			{
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "c-content-provider",
				Name:      "data",
				Interface: "mount",
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "a-content-provider",
				Name:      "data",
				Interface: "mount",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "foo",
					Name:      "plug",
				}, {
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "a-foo",
					Name:      "plug",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "b-content-provider",
				Name:      "data",
				Interface: "mount",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "foo",
					Name:      "plug",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "def",
				Sdk:       "system",
				Name:      "x11",
				Interface: "x11",
				Connections: []client.PlugRef{{
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "foo",
					Name:      "a-x11-plug",
				}, {
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "foo",
					Name:      "x11-plug",
				}, {
					ProjectId: "42424242",
					Workshop:  "abc",
					Sdk:       "keyboard-app",
					Name:      "x11",
				}},
			}, {
				ProjectId: "42424242",
				Workshop:  "abc",
				Sdk:       "leds-provider",
				Name:      "numlock-led",
				Interface: "leds",
			},
		},
	}
	query := url.Values{
		"project-id": []string{"42424242"},
		"select":     []string{"all"},
	}
	n := 0
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, s.prjId, s.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Check(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(r.URL.Query(), check.DeepEquals, query)
			body, err := io.ReadAll(r.Body)
			c.Check(err, check.IsNil)
			c.Check(body, check.DeepEquals, []byte{})
			EncodeResponseBody(c, w, map[string]interface{}{
				"type":   "sync",
				"result": result,
			})
		}
	})
	cmd := &CmdConnections{root: &CmdRoot{}}
	cmdAll := cmd.Command()
	cmd.all = true
	err := cmd.Run(cmdAll, []string{})
	c.Assert(err, IsNil)
	expectedStdout := "" +
		"Interface  Plug                         Slot                           Notes\n" +
		"desktop    abc/foo:desktop-plug         :desktop                       -\n" +
		"leds       -                            abc/leds-provider:numlock-led  -\n" +
		"leds       abc/keyboard-lights:numlock  -                              -\n" +
		"mount      -                            abc/c-content-provider:data    -\n" +
		"mount      abc/a-foo:plug               abc/a-content-provider:data    -\n" +
		"mount      abc/foo:plug                 abc/a-content-provider:data    -\n" +
		"mount      abc/foo:plug                 abc/b-content-provider:data    -\n" +
		"x11        abc/foo:x11-plug             :x11                           -\n" +
		"x11        def/foo:a-x11-plug           :x11                           -\n" +
		"x11        def/keyboard-app:x11         :x11                           manual\n"
	c.Assert(s.Stdout(), Equals, expectedStdout)
	c.Assert(s.Stderr(), Equals, "")
}
