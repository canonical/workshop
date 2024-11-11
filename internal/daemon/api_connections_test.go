// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019-2020 Canonical Ltd
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

package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"github.com/canonical/x-go/strutil"
	"github.com/spf13/afero"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// Tests for GET /v1/connections

const (
	consumerYaml = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: test
  key: value
  label: label
`
	consumerYamlManyPlugs = `
name: consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: test
  key: value
  label: label
 plug2:
  interface: test
  key: value2
  label: label2
`
	producerYaml = `
name: producer
base: ubuntu@22.04
slots:
 slot:
  interface: test
  key: value
  label: label
`
)

func (s *apiSuite) workshopFile(ws string, sdks []*sdk.Info) *workshop.File {
	file := &workshop.File{Name: ws, Base: "ubuntu@20.04"}
	for _, s := range sdks {
		file.Sdks = append(file.Sdks, workshop.SdkRecord{
			Name:    s.Name,
			Channel: "latest/stable",
			Plugs:   make(map[string]workshop.Plug),
		})
	}
	return file
}

func (s *apiSuite) writeSDKMetaFile(c *check.C, fs workshop.WorkshopFs, name, yaml string) {
	sdkPath := filepath.Join(dirs.WorkshopSdksDir, name, "0", "meta")
	c.Assert(fs.MkdirAll(sdkPath, 0755), check.IsNil)
	metaPath := filepath.Join(sdkPath, "sdk.yaml")
	c.Assert(afero.WriteFile(fs, metaPath, []byte(yaml), 0644), check.IsNil)
}

func (s *apiSuite) mockInstalledSDK(c *check.C, yaml string, w string) *workshop.Workshop {

	info := sdk.MockInfo(c, yaml, s.project.ProjectId, w)
	wf := s.workshopFile(w, []*sdk.Info{info})
	c.Assert(s.b.LaunchWorkshop(s.ctx, wf), check.IsNil)

	wp, err := s.b.Workshop(s.ctx, w)
	c.Check(err, check.IsNil)

	wfs, err := s.b.WorkshopFs(s.ctx, w)
	s.writeSDKMetaFile(c, wfs, info.Name, yaml)

	err = wp.LinkSdk(s.ctx, sdk.Setup{Name: info.Name, Channel: info.Channel, Revision: info.Revision})
	c.Assert(err, check.IsNil)

	c.Assert(s.d.overlord.InterfaceManager().Repository().AddSdk(info), check.IsNil)

	return wp
}

func (s *apiSuite) mockInstalledSDKBoundPlug(c *check.C, yaml string, w string, from, to string) *workshop.Workshop {
	info := sdk.MockInfo(c, yaml, s.project.ProjectId, w)
	info.PlugBinds[from] = &sdk.PlugBind{
		ProjectId: s.project.ProjectId,
		Workshop:  w,
		Sdk:       info.Name,
		Name:      to}
	c.Assert(s.d.overlord.InterfaceManager().Repository().AddSdk(info), check.IsNil)
	wf := s.workshopFile(w, []*sdk.Info{info})
	c.Assert(s.b.LaunchWorkshop(s.ctx, wf), check.IsNil)
	wp, err := s.b.Workshop(s.ctx, w)
	c.Check(err, check.IsNil)
	return wp
}

func (s *apiSuite) testConnectionsConnected(c *check.C, d *Daemon, query string, connsState map[string]interface{}, repoConnected []string, expected map[string]interface{}) {
	repo := d.Overlord().InterfaceManager().Repository()
	for crefStr, cstate := range connsState {
		// if repoConnected is defined, then given connection must be on
		// list, otherwise it's not going to be connected in the repository
		// to simulate missing plugs/slots.
		if repoConnected != nil && !strutil.ListContains(repoConnected, crefStr) {
			continue
		}
		cref, err := interfaces.ParseConnRef(crefStr)
		c.Assert(err, check.IsNil)
		conn := cstate.(map[string]interface{})
		if undesiredRaw, ok := conn["undesired"]; ok {
			undesired, ok := undesiredRaw.(bool)
			c.Assert(ok, check.Equals, true, check.Commentf("unexpected value for key 'undesired': %v", cstate))
			if undesired {
				// do not add connections that are undesired
				continue
			}
		}
		staticPlugAttrs, _ := conn["plug-static"].(map[string]interface{})
		dynamicPlugAttrs, _ := conn["plug-dynamic"].(map[string]interface{})
		staticSlotAttrs, _ := conn["slot-static"].(map[string]interface{})
		dynamicSlotAttrs, _ := conn["slot-dynamic"].(map[string]interface{})
		_, err = repo.Connect(cref, staticPlugAttrs, dynamicPlugAttrs, staticSlotAttrs, dynamicSlotAttrs, nil)
		c.Assert(err, check.IsNil)
	}

	st := d.Overlord().State()
	st.Lock()
	st.Set("conns", connsState)
	st.Unlock()

	s.testConnections(c, query, expected)
}

func (s *apiSuite) testConnections(c *check.C, query string, expected map[string]interface{}) {
	cmd := apiCmd("/v1/connections")
	req, err := s.createProjectsRequest("GET", query, nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1GetConnections(cmd, req, nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 200)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, expected)
}

func (s *apiSuite) TestConnectionsUnhappy(c *check.C) {
	s.daemon(c)
	cmd := apiCmd("/v1/connections")
	req, err := s.createProjectsRequest("GET", "/v2/connections?project-id=b8639dea&select=not-found", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1GetConnections(cmd, req, nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": "unsupported select qualifier"},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestConnectionsEmpty(c *check.C) {
	s.daemon(c)
	s.testConnections(c, "/v2/connections?project-id=b8639dea", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs":       []interface{}{},
			"slots":       []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs":       []interface{}{},
			"slots":       []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsNotFound(c *check.C) {
	s.daemon(c)
	cmd := apiCmd("/v1/connections")
	req, err := s.createProjectsRequest("GET", "/v2/connections?project-id=b8639dea&workshop=not-found", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1GetConnections(cmd, req, nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 404)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": connectionsNoWorkshop,
		},
		"status":      "Not Found",
		"status-code": 404.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestConnectionsUnconnected(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "ws-consumer")
	s.mockInstalledSDK(c, producerYaml, "ws-producer")

	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": s.project.ProjectId,
					"workshop":   "ws-consumer",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": s.project.ProjectId,
					"workshop":   "ws-producer",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsByWorkshopName(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all&workshop=producer-ws", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": s.project.ProjectId,
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"plugs": []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})

	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all&workshop=consumer-ws", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": s.project.ProjectId,
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"slots": []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea&workshop=producer-ws", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"manual":    true,
					"interface": "test",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsMissingPlugSlotFilteredOut(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	for _, missingPlugOrSlot := range []string{"b8639dea/consumer-ws/consumer:plug2 b8639dea/producer-ws/producer:slot", "b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot2"} {
		s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea&workshop=producer-ws", map[string]interface{}{
			"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
				"interface": "test",
			},
			missingPlugOrSlot: map[string]interface{}{
				"interface": "test",
			},
		},
			[]string{"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot"},
			map[string]interface{}{
				"result": map[string]interface{}{
					"plugs": []interface{}{
						map[string]interface{}{
							"project-id": "b8639dea",
							"workshop":   "consumer-ws",
							"sdk":        "consumer",
							"plug":       "plug",
							"interface":  "test",
							"attrs":      map[string]interface{}{"key": "value"},
							"label":      "label",
							"connections": []interface{}{
								map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
							},
						},
					},
					"slots": []interface{}{
						map[string]interface{}{
							"project-id": "b8639dea",
							"workshop":   "producer-ws",
							"sdk":        "producer",
							"slot":       "slot",
							"interface":  "test",
							"attrs":      map[string]interface{}{"key": "value"},
							"label":      "label",
							"connections": []interface{}{
								map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
							},
						},
					},
					"established": []interface{}{
						map[string]interface{}{
							"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
							"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
							"manual":    true,
							"interface": "test",
						},
					},
				},
				"status":      "OK",
				"status-code": 200.0,
				"type":        "sync",
			})
	}
}

func (s *apiSuite) TestConnectionsByIfaceName(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()
	restore = builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "different"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")
	var differentProducerYaml = `
name: different-producer
base: ubuntu@22.04
slots:
 slot:
  interface: different
  key: value
  label: label
`
	var differentConsumerYaml = `
name: different-consumer
base: ubuntu@22.04
plugs:
 plug:
  interface: different
  key: value
  label: label
`
	s.mockInstalledSDK(c, differentConsumerYaml, "consumer-diff-ws")
	s.mockInstalledSDK(c, differentProducerYaml, "producer-diff-ws")

	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all&interface=test", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
	s.testConnections(c, "/v2/connections?project-id=b8639dea&select=all&interface=different", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-diff-ws",
					"sdk":        "different-consumer",
					"plug":       "plug",
					"interface":  "different",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-diff-ws",
					"sdk":        "different-producer",
					"slot":       "slot",
					"interface":  "different",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})

	// modifies state internally
	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea&interfaces=test", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"manual":    true,
					"interface": "test",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
	// use state modified by previous call
	s.testConnections(c, "/v2/connections?project-id=b8639dea&interface=different", map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"slots":       []interface{}{},
			"plugs":       []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsDefaultManual(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"manual":    true,
					"interface": "test",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsDefaultAuto(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
			"plug-static": map[string]interface{}{
				"key": "value",
			},
			"plug-dynamic": map[string]interface{}{
				"foo-plug-dynamic": "bar-dynamic",
			},
			"slot-static": map[string]interface{}{
				"key": "value",
			},
			"slot-dynamic": map[string]interface{}{
				"foo-slot-dynamic": "bar-dynamic",
			},
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"interface": "test",
					"plug-attrs": map[string]interface{}{
						"key":              "value",
						"foo-plug-dynamic": "bar-dynamic",
					},
					"slot-attrs": map[string]interface{}{
						"key":              "value",
						"foo-slot-dynamic": "bar-dynamic",
					},
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsBoundPlug(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDKBoundPlug(c, consumerYamlManyPlugs, "consumer-ws", "plug", "plug2")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea&select=all", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
			"plug-static": map[string]interface{}{
				"key": "value",
			},
			"plug-dynamic": map[string]interface{}{
				"foo-plug-dynamic": "bar-dynamic",
			},
			"slot-static": map[string]interface{}{
				"key": "value",
			},
			"slot-dynamic": map[string]interface{}{
				"foo-slot-dynamic": "bar-dynamic",
			},
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"bind":       map[string]interface{}{"plug": "plug2", "project-id": "b8639dea", "sdk": "consumer", "workshop": "consumer-ws"},
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug2",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value2"},
					"label":      "label2",
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"interface": "test",
					"plug-attrs": map[string]interface{}{
						"key":              "value",
						"foo-plug-dynamic": "bar-dynamic",
					},
					"slot-attrs": map[string]interface{}{
						"key":              "value",
						"foo-slot-dynamic": "bar-dynamic",
					},
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsAll(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea&select=all", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
			"undesired": true,
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
				},
			},
			"undesired": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"manual":    true,
					"interface": "test",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsOnlyConnected(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
			"undesired": true,
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"established": []interface{}{},
			"plugs":       []interface{}{},
			"slots":       []interface{}{},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

func (s *apiSuite) TestConnectionsSorted(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	var anotherConsumerYaml = `
name: another-consumer-%s
base: ubuntu@22.04
plugs:
 plug:
  interface: test
  key: value
  label: label
`
	var anotherProducerYaml = `
name: another-producer
base: ubuntu@22.04
slots:
 slot:
  interface: test
  key: value
  label: label
`
	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, fmt.Sprintf(anotherConsumerYaml, "def"), "consumer-ws-def")
	s.mockInstalledSDK(c, fmt.Sprintf(anotherConsumerYaml, "abc"), "consumer-ws-abc")

	s.mockInstalledSDK(c, producerYaml, "producer-ws")
	s.mockInstalledSDK(c, anotherProducerYaml, "another-producer-ws")

	s.testConnectionsConnected(c, d, "/v2/connections?project-id=b8639dea", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
		},
		"b8639dea/consumer-ws-def/another-consumer-def:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
		},
		"b8639dea/consumer-ws-abc/another-consumer-abc:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
		},
		"b8639dea/consumer-ws-def/another-consumer-def:plug b8639dea/another-producer-ws/another-producer:slot": map[string]interface{}{
			"interface": "test",
			"auto":      true,
		},
	}, nil, map[string]interface{}{
		"result": map[string]interface{}{
			"plugs": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws",
					"sdk":        "consumer",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws-abc",
					"sdk":        "another-consumer-abc",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "consumer-ws-def",
					"sdk":        "another-consumer-def",
					"plug":       "plug",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "another-producer-ws", "sdk": "another-producer", "slot": "slot"},
						map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					},
				},
			},
			"slots": []interface{}{
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "another-producer-ws",
					"sdk":        "another-producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-def", "sdk": "another-consumer-def", "plug": "plug"},
					},
				},
				map[string]interface{}{
					"project-id": "b8639dea",
					"workshop":   "producer-ws",
					"sdk":        "producer",
					"slot":       "slot",
					"interface":  "test",
					"attrs":      map[string]interface{}{"key": "value"},
					"label":      "label",
					"connections": []interface{}{
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-abc", "sdk": "another-consumer-abc", "plug": "plug"},
						map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-def", "sdk": "another-consumer-def", "plug": "plug"},
					},
				},
			},
			"established": []interface{}{
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws", "sdk": "consumer", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"interface": "test",
				},
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-abc", "sdk": "another-consumer-abc", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"interface": "test",
				},
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-def", "sdk": "another-consumer-def", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "another-producer-ws", "sdk": "another-producer", "slot": "slot"},
					"interface": "test",
				},
				map[string]interface{}{
					"plug":      map[string]interface{}{"project-id": "b8639dea", "workshop": "consumer-ws-def", "sdk": "another-consumer-def", "plug": "plug"},
					"slot":      map[string]interface{}{"project-id": "b8639dea", "workshop": "producer-ws", "sdk": "producer", "slot": "slot"},
					"interface": "test",
				},
			},
		},
		"status":      "OK",
		"status-code": 200.0,
		"type":        "sync",
	})
}

// Tests for POST /v1/connections

func (s *apiSuite) TestConnectPlugSuccess(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	d.Overlord().Loop()
	defer d.Overlord().Stop()

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 202)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	id := body["change"].(string)

	st := d.Overlord().State()
	st.Lock()
	chg := st.Change(id)
	st.Unlock()
	c.Assert(chg, check.NotNil)

	<-chg.Ready()

	st.Lock()
	err = chg.Err()
	st.Unlock()
	c.Assert(err, check.IsNil)

	repo := d.Overlord().InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 1)
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}})
}

func (s *apiSuite) TestConnectBoundPlugSuccess(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	s.mockInstalledSDK(c, consumerYamlManyPlugs, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	wp, err := s.b.Workshop(s.ctx, "consumer-ws")
	c.Check(err, check.IsNil)
	wp.File.Sdks[0].Plugs = make(map[string]workshop.Plug)
	wp.File.Sdks[0].Plugs["plug2"] = workshop.Plug{Bind: &workshop.PlugRef{Sdk: "consumer", Name: "plug"}}

	d.Overlord().Loop()
	defer d.Overlord().Stop()

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 202)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	id := body["change"].(string)

	st := d.Overlord().State()
	st.Lock()
	chg := st.Change(id)
	st.Unlock()
	c.Assert(chg, check.NotNil)

	<-chg.Ready()

	st.Lock()
	err = chg.Err()
	st.Unlock()
	c.Assert(err, check.IsNil)

	repo := d.Overlord().InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 2)

	mainConn := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	boundConn := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug2"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	c.Check(ifaces.Connections, check.DeepEquals, []*interfaces.ConnRef{mainConn, boundConn})

	var conns map[string]interface{}
	s.d.state.Lock()
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 2)
	c.Assert(conns, check.DeepEquals,
		map[string]interface{}{
			"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
				"interface":   "test",
				"plug-static": map[string]interface{}{"key": "value"},
				"slot-static": map[string]interface{}{"key": "value"},
			},
			"b8639dea/consumer-ws/consumer:plug2 b8639dea/producer-ws/producer:slot": map[string]interface{}{
				"interface":    "test",
				"plug-static":  map[string]interface{}{"key": "value2"},
				"slot-static":  map[string]interface{}{"key": "value"},
				"plug-dynamic": map[string]interface{}{"bind": "b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot"},
			},
		})
	s.d.state.Unlock()
}

func mockIface(c *check.C, d *Daemon, iface interfaces.Interface) {
	err := d.Overlord().InterfaceManager().Repository().AddInterface(iface)
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestConnectPlugFailureInterfaceMismatch(c *check.C) {
	d := s.daemon(c)

	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "test"})
	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "different"})

	var differentProducerYaml = `
name: producer
base: ubuntu@22.04
slots:
  slot:
    interface: different
    key: value 
    label: label
`

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, differentProducerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": connectIncompatible,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
	repo := d.Overlord().InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 0)
}

func (s *apiSuite) TestConnectPlugFailureNoSuchPlug(c *check.C) {
	d := s.daemon(c)

	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "test"})
	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "different"})

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "missingplug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": consumerNoPlug,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
	repo := d.Overlord().InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 0)
}

func (s *apiSuite) TestConnectPlugFailureNoSuchSlot(c *check.C) {
	d := s.daemon(c)

	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "test"})
	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "different"})

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "missingslot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": producerNoSlot,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
	repo := d.Overlord().InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 0)
}

func (s *apiSuite) TestConnectAlreadyConnected(c *check.C) {
	d := s.daemon(c)

	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "test"})
	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "different"})

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")
	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	st := d.Overlord().State()
	st.Lock()
	st.Set("conns", map[string]interface{}{
		"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
			"interface": "test",
		},
	})
	st.Unlock()

	d.Overlord().Loop()
	defer d.Overlord().Stop()

	_, err := d.overlord.InterfaceManager().Repository().Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	cmd := apiCmd("/v1/connections")
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 202)
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	id := body["change"].(string)

	st.Lock()
	chg := st.Change(id)
	st.Unlock()
	c.Assert(chg, check.NotNil)

	<-chg.Ready()

	st.Lock()
	err = chg.Err()
	st.Unlock()
	c.Assert(err, check.IsNil)
}

func (s *apiSuite) TestConnectFailureOnConflict(c *check.C) {
	d := s.daemon(c)

	mockIface(c, d, &ifacetest.TestInterface{InterfaceName: "test"})

	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "connect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}

	st := s.d.state
	st.Lock()
	chg := st.NewChange("conflict", "...")
	tsk := st.NewTask("stop-workshop", "...")
	chg.AddTask(tsk)
	tsk.Set("workshop", "producer-ws")
	chg.Set("project-id", "b8639dea")
	st.Unlock()

	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": `workshop "producer-ws" has "conflict" change in progress`,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

type disconnectOpts struct {
	auto   bool
	forget bool
	bind   map[string]workshop.Plug
}

func (s *apiSuite) connect(c *check.C, plug string) *interfaces.ConnRef {
	repo := s.d.Overlord().InterfaceManager().Repository()
	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: plug},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	_, err := repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)
	return connRef
}

func (s *apiSuite) testDisconnect(c *check.C, pW, pSdk, pName string, sW, sSdk, sName string, opts *disconnectOpts) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)

	wp := s.mockInstalledSDK(c, consumerYamlManyPlugs, "consumer-ws")
	wp.File.Sdks[0].Plugs = opts.bind
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	conns := map[string]interface{}{}
	connRef := s.connect(c, "plug")
	conns[connRef.ID()] = map[string]interface{}{"interface": "test", "auto": opts.auto}

	for n := range opts.bind {
		cRef := s.connect(c, n)
		conns[cRef.ID()] = map[string]interface{}{"interface": "test", "auto": opts.auto,
			"plug-dynamic": map[string]interface{}{"bind": connRef.ID()}}
	}

	st := d.Overlord().State()
	st.Lock()
	st.Set("conns", conns)
	st.Unlock()

	d.Overlord().Loop()
	defer d.Overlord().Stop()

	action := &client.InterfaceAction{
		Action: "disconnect",
		Forget: opts.forget,
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: pW, Sdk: pSdk, Name: pName}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: sW, Sdk: sSdk, Name: sName}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", "/v1/connections", buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	cmd := apiCmd("/v1/connections")
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 202)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	id := body["change"].(string)

	st.Lock()
	chg := st.Change(id)
	st.Unlock()
	c.Assert(chg, check.NotNil)
	c.Assert(chg.Summary(), check.Equals, fmt.Sprintf(`Disconnect %s/%s:%s`, pW, pSdk, pName))

	<-chg.Ready()

	st.Lock()
	err = chg.Err()
	st.Unlock()
	c.Assert(err, check.IsNil)

	repo := s.d.overlord.InterfaceManager().Repository()
	ifaces := repo.Interfaces()
	c.Assert(ifaces.Connections, check.HasLen, 0)
}

func (s *apiSuite) TestDisconnectPlugSuccess(c *check.C) {
	s.testDisconnect(c, "consumer-ws", "consumer", "plug", "producer-ws", "producer", "slot", &disconnectOpts{})
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectPlugAutoSuccess(c *check.C) {
	s.testDisconnect(c, "consumer-ws", "consumer", "plug", "producer-ws", "producer", "slot", &disconnectOpts{auto: true})
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 1)
	c.Assert(conns, check.DeepEquals,
		map[string]interface{}{
			"b8639dea/consumer-ws/consumer:plug b8639dea/producer-ws/producer:slot": map[string]interface{}{
				"auto": true, "interface": "test", "undesired": true,
			}})
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectPlugForgetSuccess(c *check.C) {
	opts := &disconnectOpts{
		auto:   true,
		forget: true,
	}
	s.testDisconnect(c, "consumer-ws", "consumer", "plug", "producer-ws", "producer", "slot", opts)
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectBoundPlugMasterSuccess(c *check.C) {
	opts := &disconnectOpts{
		bind: map[string]workshop.Plug{
			"plug2": {Bind: &workshop.PlugRef{Sdk: "consumer", Name: "plug"}},
		},
	}
	s.testDisconnect(c, "consumer-ws", "consumer", "plug", "producer-ws", "producer", "slot", opts)
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectBoundWithEmptyPlug(c *check.C) {
	opts := &disconnectOpts{
		bind: map[string]workshop.Plug{
			"plug2": {Bind: &workshop.PlugRef{Sdk: "consumer", Name: "plug"}},
		},
	}
	s.testDisconnect(c, "", "", "", "producer-ws", "producer", "slot", opts)
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectBoundPlugSlaveSuccess(c *check.C) {
	opts := &disconnectOpts{
		bind: map[string]workshop.Plug{
			"plug2": {Bind: &workshop.PlugRef{Sdk: "consumer", Name: "plug"}},
		},
	}
	s.testDisconnect(c, "consumer-ws", "consumer", "plug2", "producer-ws", "producer", "slot", opts)
	s.d.state.Lock()
	var conns map[string]interface{}
	s.d.state.Get("conns", &conns)
	c.Assert(conns, check.HasLen, 0)
	s.d.state.Unlock()
}

func (s *apiSuite) TestDisconnectPlugSuccessWithEmptyPlug(c *check.C) {
	s.testDisconnect(c, "", "", "", "producer-ws", "producer", "slot", &disconnectOpts{})
}

func (s *apiSuite) TestDisconnectPlugSuccessWithEmptySlot(c *check.C) {
	s.testDisconnect(c, "consumer-ws", "consumer", "plug", "", "", "", &disconnectOpts{})
}

func (s *apiSuite) TestDisconnectPlugFailureNoSuchPlug(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)
	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	repo := d.Overlord().InterfaceManager().Repository()
	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	_, err := repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	action := &client.InterfaceAction{
		Action: "disconnect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "missingplug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": disconnectNoPlug,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) testDisconnectFailureNoWorkshop(c *check.C, installedWorkshop string) {
	// validity, either consumer or producer needs to be enabled
	cmd := apiCmd("/v1/connections")

	consumer := installedWorkshop == "consumer-ws"
	producer := installedWorkshop == "producer-ws"
	c.Assert(consumer || producer, check.Equals, true, check.Commentf("launched workshop must be consumer-ws or producer-ws"))

	revert := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer revert()
	s.daemon(c)

	if consumer {
		s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	}
	if producer {
		s.mockInstalledSDK(c, producerYaml, "producer-ws")
	}

	action := &client.InterfaceAction{
		Action: "disconnect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 404)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)

	if producer {
		c.Check(body, check.DeepEquals, map[string]interface{}{
			"result": map[string]interface{}{
				"message": disconnectNoConsumer,
			},
			"status":      "Not Found",
			"status-code": 404.0,
			"type":        "error",
		})
	} else {
		c.Check(body, check.DeepEquals, map[string]interface{}{
			"result": map[string]interface{}{
				"message": disconnectNoProducer,
			},
			"status":      "Not Found",
			"status-code": 404.0,
			"type":        "error",
		})
	}
}

func (s *apiSuite) TestDisconnectPlugFailureNoPlugWorkshop(c *check.C) {
	s.testDisconnectFailureNoWorkshop(c, "producer-ws")
}

func (s *apiSuite) TestDisconnectPlugFailureNoSlotWorkshop(c *check.C) {
	s.testDisconnectFailureNoWorkshop(c, "consumer-ws")
}

func (s *apiSuite) TestDisconnectPlugNothingToDo(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	_ = s.daemon(c)
	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "disconnect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "", Sdk: "", Name: ""}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": disconnectNothing,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestDisconnectPlugFailureNotConnected(c *check.C) {
	revert := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer revert()
	s.daemon(c)
	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "disconnect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)

	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": disconnectAgain,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestDisconnectForgetPlugFailureNotConnected(c *check.C) {
	revert := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer revert()
	s.daemon(c)
	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	action := &client.InterfaceAction{
		Action: "disconnect",
		Forget: true,
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)

	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": disconnectForget,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestDisconnectFailureOnConflict(c *check.C) {
	restore := builtin.MockInterface(&ifacetest.TestInterface{InterfaceName: "test"})
	defer restore()

	d := s.daemon(c)
	cmd := apiCmd("/v1/connections")

	s.mockInstalledSDK(c, consumerYaml, "consumer-ws")
	s.mockInstalledSDK(c, producerYaml, "producer-ws")

	repo := d.Overlord().InterfaceManager().Repository()
	connRef := &interfaces.ConnRef{
		PlugRef: interfaces.PlugRef{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"},
		SlotRef: interfaces.SlotRef{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"},
	}
	_, err := repo.Connect(connRef, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	action := &client.InterfaceAction{
		Action: "disconnect",
		Plugs:  []client.Plug{{ProjectId: "b8639dea", Workshop: "consumer-ws", Sdk: "consumer", Name: "plug"}},
		Slots:  []client.Slot{{ProjectId: "b8639dea", Workshop: "producer-ws", Sdk: "producer", Name: "slot"}},
	}

	st := s.d.state
	st.Lock()
	chg := st.NewChange("conflict", "...")
	tsk := st.NewTask("stop-workshop", "...")
	chg.AddTask(tsk)
	tsk.Set("workshop", "consumer-ws")
	chg.Set("project-id", "b8639dea")
	st.Unlock()

	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": `workshop "consumer-ws" has "conflict" change in progress`,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestUnsupportedInterfaceRequest(c *check.C) {
	s.daemon(c)
	cmd := apiCmd("/v1/connections")
	buf := bytes.NewBuffer([]byte(`garbage`))
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": "cannot decode request body into an interface action: invalid character 'g' looking for beginning of value",
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestMissingInterfaceAction(c *check.C) {
	s.daemon(c)
	cmd := apiCmd("/v1/connections")
	action := &client.InterfaceAction{}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": "interface action not specified",
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}

func (s *apiSuite) TestUnsupportedInterfaceAction(c *check.C) {
	s.daemon(c)
	cmd := apiCmd("/v1/connections")
	action := &client.InterfaceAction{Action: "foo"}
	text, err := json.Marshal(action)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(text)
	req, err := http.NewRequest("POST", cmd.Path, buf)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	v1PostConnections(cmd, req.WithContext(s.ctx), nil).ServeHTTP(rec, req)
	c.Check(rec.Code, check.Equals, 400)
	var body map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]interface{}{
		"result": map[string]interface{}{
			"message": `unsupported interface action: "foo"`,
		},
		"status":      "Bad Request",
		"status-code": 400.0,
		"type":        "error",
	})
}
