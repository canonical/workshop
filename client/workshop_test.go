package client_test

import (
	"fmt"
	"io"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func (cs *clientSuite) TestClientListProjectWorkshops(c *check.C) {
	cs.rsp = `{"type": "sync", "result": {"workshops": [{"name":"workshop",
		"base":"ubuntu@20.04",
		"project-id":"42ws42ws",
		"status":"Ready",
		"notes":["missing-project"],
		"sdks":[{"name":"go","channel":"latest/stable","revision":"453","install-time":"2023-04-25T01:02:03Z","health-check":{"timestamp":"2023-04-25T01:02:03Z", "message":"hello from health-check", "code":"check-waiting"}}]
	}]}}`
	workshops, _, err := cs.cli.List(&client.ListOptions{ProjectId: "42ws42ws"})
	c.Assert(err, check.IsNil)
	c.Assert(workshops, check.DeepEquals, []*client.WorkshopInfo{
		{
			ProjectId: "42ws42ws",
			Name:      "workshop",
			Base:      "ubuntu@20.04",
			Status:    "Ready",
			Notes:     []string{"missing-project"},
			Sdks: []*client.Sdk{
				{
					Name:        "go",
					Channel:     "latest/stable",
					Revision:    "453",
					InstallTime: time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC),
					Health: &client.HealthCheck{
						Timestamp: time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC),
						Message:   "hello from health-check",
						Code:      "check-waiting",
					},
				},
			},
		},
	})
	c.Check(cs.req.Method, check.Equals, "GET")
}

func (cs *clientSuite) TestClientSingleWorkshop(c *check.C) {
	project := &client.Project{Id: "42ws42ws", Path: "/home/user/project"}
	expectedInfo := client.WorkshopInfo{
		ProjectId: "42ws42ws",
		Name:      "workshop",
		Base:      "ubuntu@20.04",
		Status:    "Ready",
		Sdks:      []*client.Sdk{},
		Notes:     []string{},
	}

	// Workshop only
	cs.rsp = `{"type": "sync", "result": {"workshops":[{"name":"workshop","base":"ubuntu@20.04","project-id":"42ws42ws","status":"Ready","notes":[],"sdks":[]}]}}`
	workshop, err := cs.cli.SingleWorkshop(project)
	c.Assert(err, check.IsNil)
	c.Assert(workshop, check.DeepEquals, &client.Workshop{WorkshopInfo: expectedInfo})
	c.Check(cs.req.Method, check.Equals, "GET")

	name, err := cs.cli.SingleWorkshopName(project)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, expectedInfo.Name)
	c.Check(cs.req.Method, check.Equals, "GET")

	// File only
	cs.rsp = `{"type": "sync", "result": {"files":[{"name":"workshop","project-id":"42ws42ws","path":"/home/user/project/workshop.yaml"}]}}`
	workshop, err = cs.cli.SingleWorkshop(project)
	c.Assert(workshop, check.IsNil)
	c.Assert(err, check.ErrorMatches, "workshop not launched")
	c.Check(cs.req.Method, check.Equals, "GET")

	name, err = cs.cli.SingleWorkshopName(project)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, expectedInfo.Name)
	c.Check(cs.req.Method, check.Equals, "GET")

	// Workshop and file
	cs.rsp = `{"type": "sync", "result": {"workshops":[{"name":"workshop","base":"ubuntu@20.04","project-id":"42ws42ws","status":"Ready","notes":[],"sdks":[]}], "files":[{"name":"workshop","project-id":"42ws42ws","path":"/home/user/project/workshop.yaml"}]}}`
	workshop, err = cs.cli.SingleWorkshop(project)
	c.Assert(err, check.IsNil)
	c.Assert(workshop, check.DeepEquals, &client.Workshop{
		WorkshopInfo: expectedInfo,
		Path:         "/home/user/project/workshop.yaml",
	})
	c.Check(cs.req.Method, check.Equals, "GET")

	name, err = cs.cli.SingleWorkshopName(project)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, expectedInfo.Name)
	c.Check(cs.req.Method, check.Equals, "GET")
}

func (cs *clientSuite) TestClientNoWorkshops(c *check.C) {
	cs.rsp = `{"type": "sync", "result": {}}`
	workshop, err := cs.cli.SingleWorkshop(&client.Project{Id: "42ws42ws", Path: "/home/user/project"})
	c.Assert(workshop, check.IsNil)
	c.Assert(err, check.ErrorMatches, `cannot infer workshop name: no workshops found in "/home/user/project"`)
	c.Check(cs.req.Method, check.Equals, "GET")
}

func (cs *clientSuite) TestClientMultipleWorkshops(c *check.C) {
	project := &client.Project{Id: "42ws42ws", Path: "/home/user/project"}

	// Two workshops
	cs.rsp = `{"type": "sync", "result": {"workshops":[{"name":"ci","base":"ubuntu@20.04","project-id":"42ws42ws","status":"Ready","notes":[],"sdks":[]},{"name":"dev","base":"ubuntu@24.04","project-id":"42ws42ws","status":"Ready","notes":[],"sdks":[]}]}}`
	workshop, err := cs.cli.SingleWorkshop(project)
	c.Assert(workshop, check.IsNil)
	c.Assert(err, check.ErrorMatches, `cannot infer workshop name: multiple workshops found: "ci", "dev"`)
	c.Check(cs.req.Method, check.Equals, "GET")

	// Workshop and file
	cs.rsp = `{"type": "sync", "result": {"workshops":[{"name":"ci","base":"ubuntu@20.04","project-id":"42ws42ws","status":"Ready","notes":[],"sdks":[]}],"files":[{"name":"dev","project-id":"42ws42ws","path":"/home/user/project/.workshop/dev.yaml"}]}}`
	workshop, err = cs.cli.SingleWorkshop(project)
	c.Assert(workshop, check.IsNil)
	c.Assert(err, check.ErrorMatches, `cannot infer workshop name: multiple workshops found: "ci", "dev"`)
	c.Check(cs.req.Method, check.Equals, "GET")

	// Two files
	cs.rsp = `{"type": "sync", "result": {"files":[{"name":"ci","project-id":"42ws42ws","path":"/home/user/project/.workshop/ci.yaml"},{"name":"dev","project-id":"42ws42ws","path":"/home/user/project/.workshop/dev.yaml"}]}}`
	name, err := cs.cli.SingleWorkshopName(project)
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.ErrorMatches, `cannot infer workshop name: multiple workshops found: "ci", "dev"`)
	c.Check(cs.req.Method, check.Equals, "GET")
}

func (cs *clientSuite) TestClientProjectWorkshop(c *check.C) {
	cs.rsp = `{"type": "sync", "result": {"name":"workshop","base":"ubuntu@20.04","project-id":"42ws42ws","status":"Ready",
	"sdks":[
		{"name":"go",
		"version":"1.20.3",
		"channel":"latest/stable",
		"revision":"453",
		"build-time":"2023-04-06T04:51:36.152964Z", 
		"install-time":"2023-04-25T01:02:03Z", 
		"health-check":{"timestamp":"2023-04-25T01:02:03Z", "message":"hello from health-check", "code":"check-waiting"},
		"mounts":[{"host-source":"/home/user/src","workshop-target":"/home/workshop/target", "plug":{"project-id":"42ws42ws","workshop":"workshop","sdk":"go","plug":"plug-name"}},{"workshop-source":"/home","workshop-target":"/mnt", "plug":{"project-id":"42ws42ws","workshop":"workshop","sdk":"go","plug":"plug-name-2"}}]
	}],"path":"/home/projects/.workshop/workshop.yaml"}}`
	wp, err := cs.cli.Workshop("42ws42ws", "workshop")
	c.Assert(err, check.IsNil)
	c.Assert(wp, check.DeepEquals, &client.Workshop{
		WorkshopInfo: client.WorkshopInfo{
			ProjectId: "42ws42ws",
			Name:      "workshop",
			Base:      "ubuntu@20.04",
			Status:    "Ready",
			Sdks: []*client.Sdk{
				{
					Name:        "go",
					Version:     "1.20.3",
					Channel:     "latest/stable",
					Revision:    "453",
					BuildTime:   time.Date(2023, 04, 6, 4, 51, 36, 152964000, time.UTC),
					InstallTime: time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC),
					Health: &client.HealthCheck{
						Timestamp: time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC),
						Message:   "hello from health-check",
						Code:      "check-waiting",
					},
					Mounts: []*client.Mount{
						{HostSource: "/home/user/src", WorkshopTarget: "/home/workshop/target", Plug: client.PlugRef{ProjectId: "42ws42ws", Workshop: "workshop", Sdk: "go", Name: "plug-name"}},
						{WorkshopSource: "/home", WorkshopTarget: "/mnt", Plug: client.PlugRef{ProjectId: "42ws42ws", Workshop: "workshop", Sdk: "go", Name: "plug-name-2"}},
					},
				},
			},
		},
		Path: "/home/projects/.workshop/workshop.yaml",
	})
	c.Check(cs.req.Method, check.Equals, "GET")
}

func (cs *clientSuite) TestClientListScripts(c *check.C) {
	cs.rsp = `{"type": "sync", "result": {"foo": {"script": "echo bar\n"}}}`

	scripts, err := cs.cli.ListScripts("42ws42ws", "ws")

	c.Assert(err, check.IsNil)
	c.Check(cs.req.Method, check.Equals, "GET")
	c.Check(scripts, check.DeepEquals, map[string]client.Script{"foo": {Script: "echo bar\n"}})
}

func (cs *clientSuite) TestRemountRequest(c *check.C) {
	cs.rsp = `{"type": "async", "status-code": 202, "change": "24"}`

	source := c.MkDir()
	id, err := cs.cli.Remount(&client.PlugRef{ProjectId: "4242", Workshop: "ws", Sdk: "sdk", Name: "plug"}, source)

	c.Check(cs.req.Method, check.Equals, "POST")
	c.Assert(id, check.Equals, "24")
	c.Assert(err, check.IsNil)

	body, err := io.ReadAll(cs.req.Body)
	c.Assert(err, check.IsNil)

	c.Assert(string(body), check.Matches, fmt.Sprintf(`{"action":"remount","plug":{"project-id":"4242","workshop":"ws","sdk":"sdk","plug":"plug"},"host-source":%q}\n`, source))
}
