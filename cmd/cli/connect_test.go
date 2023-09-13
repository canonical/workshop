package cli

import (
	"fmt"
	"net/http"

	"gopkg.in/check.v1"
)

type connectSuite struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
}

var _ = check.Suite(&connectSuite{})

func (m *connectSuite) SetUpTest(c *check.C) {
	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	m.BaseWorkshopSuite.SetUpTest(c)
}

func (m *connectSuite) TestConnectAcrossWorkshops(c *check.C) {
	cmd := &CmdConnect{root: &CmdRoot{}}

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		default:
			c.Errorf("expected 1 call, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{"ws/sdk:plug", "ws2/sdk:slot"})
	c.Assert(err, check.ErrorMatches, "cannot connect plugs and slots across different workshops")
}

func (m *connectSuite) TestDisconnectPlugAndSlotProvided(c *check.C) {
	cmd := &CmdConnect{root: &CmdRoot{}}
	body := map[string]interface{}{
		"action": "connect",
		"plugs": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk",
				"plug":       "plug",
			},
		},
		"slots": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "system",
				"slot":       "mount",
			},
		},
	}

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
			c.Assert(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, body)
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

	err := cmd.Run(cmd.Command(), []string{"ws/sdk:plug", ":mount"})
	c.Assert(err, check.IsNil)

	n = 0
	body = map[string]interface{}{
		"action": "connect",
		"plugs": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk",
				"plug":       "plug2",
			},
		},
		"slots": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk-2",
				"slot":       "producer",
			},
		},
	}

	err = cmd.Run(cmd.Command(), []string{"ws/sdk:plug2", "ws/sdk-2:producer"})
	c.Assert(err, check.IsNil)

	n = 0
	body = map[string]interface{}{
		"action": "connect",
		"plugs": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk",
				"plug":       "plug2",
			},
		},
		"slots": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk-2",
				"slot":       "",
			},
		},
	}

	err = cmd.Run(cmd.Command(), []string{"ws/sdk:plug2", "ws/sdk-2"})
	c.Assert(err, check.IsNil)
}

func (m *connectSuite) TestDisconnectSlotNotProvided(c *check.C) {
	cmd := &CmdConnect{root: &CmdRoot{}}
	body := map[string]interface{}{
		"action": "connect",
		"plugs": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "sdk",
				"plug":       "plug",
			},
		},
		"slots": []interface{}{
			map[string]interface{}{
				"project-id": "42424242",
				"workshop":   "ws",
				"sdk":        "system",
				"slot":       "plug",
			},
		},
	}

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
			c.Assert(r.URL.Path, check.Equals, "/v1/connections")
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, body)
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

	err := cmd.Run(cmd.Command(), []string{"ws/sdk:plug"})
	c.Assert(err, check.IsNil)
}
