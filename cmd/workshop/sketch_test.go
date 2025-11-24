package main

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type workshopSketch struct {
	BaseWorkshopSuite
	prjDir         string
	prjId          string
	userDataDir    string
	restoreUserEnv func()
}

var _ = check.Suite(&workshopSketch{})

var mockWorkshopWithSdksReady = `{"type":"sync","status-code":200,"status":"OK","result":{
    "name":"ws",
    "base":"ubuntu@22.04",
    "project-id":"42424242",
    "status":"Ready",
    "sdks":[{
      "name":"go",
      "version":"1.8.0",
      "channel":"latest/edge",
      "revision":"1",
      "build-time":"2017-02-19T17:23:05.592623Z",
      "install-time":"2017-03-22T09:01:00.0Z"
    },{
      "name":"sketch",
      "source":"/home/.local/share/workshop/id/42424242/ws/sdk/sketch/current",
      "revision":"x1",
      "install-time":"2017-03-22T09:01:00.0Z"
    }],
    "path":"/home/project/.workshop/ws.yaml"
}}`

var mockWorkshopWithSdksWaiting = `{"type":"sync","status-code":200,"status":"OK","result":{
    "name":"ws",
    "base":"ubuntu@22.04",
    "project-id":"42424242",
    "status":"Waiting",
    "notes":["wait-on-error"],
    "sdks":[{
      "name":"go",
      "version":"1.8.0",
      "channel":"latest/edge",
      "revision":"1",
      "build-time":"2017-02-19T17:23:05.592623Z",
      "install-time":"2017-03-22T09:01:00.0Z"
    },{
      "name":"sketch",
      "source":"/home/.local/share/workshop/id/42424242/ws/sdk/sketch/current",
      "revision":"x2",
      "install-time":"2017-03-22T09:01:00.0Z"
    }],
    "path":"/home/project/.workshop/ws.yaml"
}}`

var mockWorkshopsListWithSketch = `{"type":"sync","status-code":200,"status":"OK","result":{
    "workshops":[{
        "name":"ws",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready",
        "sdks":[{
            "name":"sketch",
            "source":"/home/.local/share/workshop/id/42424242/ws/sdk/sketch/current",
            "revision":"x1",
            "install-time":"2017-03-22T09:01:00.0Z"
        }]
        },{
        "name":"nosketch",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready"
        },{
        "name":"both",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready",
        "sdks":[{
            "name":"sketch",
            "source":"/home/.local/share/workshop/id/42424242/both/sdk/sketch/current",
            "revision":"x3",
            "install-time":"2017-03-22T09:01:00.0Z"
        }]
        },{
        "name":"none",
        "base":"ubuntu@22.04",
        "project-id":"42424242",
        "status":"Ready"
    }]
},
"warning-timestamp":"2017-03-22T10:01:00.0Z",
"warning-count":1}`

var simpleSketchMeta = `name: sketch
`

func (m *workshopSketch) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)
	m.prjId = "42424242"
	m.prjDir = c.MkDir()

	usr := &user.User{HomeDir: c.MkDir()}

	m.restoreUserEnv = osutil.FakeCurrentUserAndEnv(func() (*user.User, map[string]string, error) {
		return usr, map[string]string{}, nil
	})

	m.userDataDir = workshop.UserDataRootDir(usr.HomeDir, nil)
}

func (m *workshopSketch) TearDownTest(c *check.C) {
	sketch := workshop.SketchSdkDir(m.userDataDir, m.prjId, "ws")
	err := os.RemoveAll(sketch)
	c.Assert(err, check.IsNil)
	m.restoreUserEnv()
}

func (m *workshopSketch) mockMinimalSketchSdk(c *check.C, ws string, current bool, meta []byte) (metapath string, hookspath string) {
	var sketchDir string
	if current {
		sketchDir = workshop.SketchSdkCurrent(m.userDataDir, m.prjId, ws)
	} else {
		sketchDir = workshop.SketchSdkStash(m.userDataDir, m.prjId, ws)
	}

	c.Assert(writeSketchSdk(filepath.Join(sketchDir, "sdk.yaml"), meta), check.IsNil)
	c.Assert(writeSketchHooks(sketchDir, meta), check.IsNil)

	return sketchDir, filepath.Join(sketchDir, "hooks")
}

func (m *workshopSketch) mockSketchHappyRefreshPath(c *check.C, refreshname string, mode string) {
	n := 0
	workshop := "ws"
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
			fmt.Fprintln(w, mockWorkshopWithSdksReady)
		case 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{refreshname}, "options": map[string]interface{}{"mode": mode, "refresh-option": "update"},
			})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockReadyChangeJSON)
		default:
			c.Errorf("expected 4 calls, now on %d", n)
		}
	})
}

func (m *workshopSketch) mockSketchesHappyPath(c *check.C, resp string) {
	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			res := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, res)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects/"+m.prjId+"/workshops")
			w.WriteHeader(200)
			fmt.Fprintln(w, resp)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})
}

func (m *workshopSketch) TestSketchSdkMetaOnlySuccess(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")

	sketchContent := fmt.Sprintf(sketchTemplate, "/home/project/.workshop/ws.yaml")
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, inContent), check.IsNil)
		}
		return inContent, nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	current := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "sdk.yaml"), testutil.FileEquals, sketchContent)
	c.Assert(filepath.Join(current, "hooks"), testutil.FileAbsent)
}

func (m *workshopSketch) TestSketchSdkSuccess(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")

	sketchContent := `name: sketch

hooks:
    setup-base: |-
        echo "Hello"
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, []byte(sketchContent)), check.IsNil)
		}
		return []byte(sketchContent), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	current := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "sdk.yaml"), testutil.FileEquals, sketchContent)
	c.Assert(filepath.Join(current, "hooks", "setup-base"), testutil.FileEquals, "echo \"Hello\"\n")
	c.Assert(m.stdout.String(), check.Matches, `"ws" sketch refreshed\n`)
}

func (m *workshopSketch) TestSketchSdkUpdateHooks(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")

	meta, hooks := m.mockMinimalSketchSdk(c, "ws", true, []byte(`name: sketch

hooks:
  setup-base: |-
    echo "Hello"
  check-health: |-
    workshopctl set-health okay
`))

	sketchContent := `name: sketch

hooks:
    save-state: |-
        # saves state
    restore-state: |-
        # restores state
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, []byte(sketchContent)), check.IsNil)
		}
		return []byte(sketchContent), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)
	c.Assert(filepath.Join(meta, "sdk.yaml"), testutil.FileEquals, sketchContent)
	c.Assert(filepath.Join(hooks, "setup-base"), testutil.FileAbsent)
	c.Assert(filepath.Join(hooks, "check-health"), testutil.FileAbsent)
	c.Assert(filepath.Join(hooks, "save-state"), testutil.FileEquals, "# saves state\n")
	c.Assert(filepath.Join(hooks, "restore-state"), testutil.FileEquals, "# restores state\n")
}

func (m *workshopSketch) TestSketchSdkEditExistingMeta(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")

	dir := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	err := os.MkdirAll(dir, 0o755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(filepath.Join(dir, "sdk.yaml"), []byte(simpleSketchMeta), 0o644)
	c.Assert(err, check.IsNil)

	sketchContent := `name: sketch
plugs:
  gpu:
    interface: gpu
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, []byte(sketchContent)), check.IsNil)
		}
		return []byte(sketchContent), nil
	})
	defer restore()

	err = cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(filepath.Join(dir, "sdk.yaml"), testutil.FileEquals, sketchContent)
}

func (m *workshopSketch) TestSketchSdkPreserveBadEdits(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}

	n := 0
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
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/ws", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithSdksReady)
		default:
			c.Errorf("expected 4 calls, now on %d", n)
		}
	})

	sketchContent := `name: sketch
hooks:
  setup-base:
    check-health:
  # comment
      save-state:
`
	checked := false
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		if n >= 4 {
			checked = true
			c.Check(string(inContent), check.Equals, sketchContent)
		}
		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, []byte(sketchContent)), check.IsNil)
		}
		return []byte(sketchContent), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.NotNil)
	dir := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Check(filepath.Join(dir, "sdk.yaml"), testutil.FileEquals, sketchContent)

	err = cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.NotNil)
	c.Check(checked, check.Equals, true)
	c.Check(n, check.Equals, 4)
}

func (m *workshopSketch) TestSketchSdkFixRefreshError(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}

	// Runs sketch-sdk with a failing setup-base hook,
	// then immediately re-runs it to fix the hook.
	// The second run should automatically abort the first refresh.
	// The API calls break down as follows:
	// 1-2: get workshop info
	// 3-4: refresh --wait-on-error (fails due to setup-base)
	// 5-6: get workshop info
	// 7-8. refresh --abort
	// 9-10. refresh --wait-on-error
	n := 0
	change := 42
	workshop := "ws"
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 5:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects", check.Commentf("call: %d", n))
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2, 6:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			switch n {
			case 2:
				fmt.Fprintln(w, mockWorkshopWithSdksReady)
			case 6:
				fmt.Fprintln(w, mockWorkshopWithSdksWaiting)
			}
		case 3, 7, 9:
			mode := "wait-on-error"
			name := workshop
			if n == 7 {
				mode = "abort"
			}

			change += 1
			response := fmt.Sprintf(`{"type":"async", "change": "%d", "status-code": 202}`, change)

			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))

			if mode == "wait-on-error" {
				c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
					"action": "refresh",
					"names":  []interface{}{name}, "options": map[string]interface{}{"mode": mode, "refresh-option": "update"},
				})
			} else {
				c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
					"action": "refresh",
					"names":  []interface{}{name}, "options": map[string]interface{}{"mode": mode},
				})
			}
			w.WriteHeader(202)
			fmt.Fprintln(w, response)
		case 4, 8, 10:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/changes/%d", change))
			switch n {
			case 4:
				fmt.Fprintln(w, mockSketchWaitChangeJSON)
			case 8:
				fmt.Fprintln(w, mockSketchAbortedChangeJSON)
			case 10:
				fmt.Fprintln(w, mockReadyChangeJSON)
			}
		default:
			c.Errorf("expected 10 calls, now on %d", n)
		}
	})

	attempts := 0
	sketchSetup := `name: sketch

hooks:
    setup-base: |
        %s
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		var content string
		attempts += 1
		switch attempts {
		case 1:
			content = fmt.Sprintf(sketchSetup, "false")
		case 2:
			content = fmt.Sprintf(sketchSetup, "true")
		default:
			return nil, fmt.Errorf("expected 2 attempts, now on %d", attempts)
		}

		if inPath != "" {
			c.Assert(writeSketchSdk(inPath, []byte(content)), check.IsNil)
		}
		return []byte(content), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.ErrorMatches, "cannot complete sketch refresh for \"ws\", execution is paused\n\n"+
		"To proceed, resolve the issue and run 'workshop refresh --continue ws'\n"+
		"To cancel and undo: 'workshop refresh --abort ws'\n"+
		"To view more information: 'workshop tasks 43'")

	err = cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(n, check.Equals, 10)
	c.Assert(attempts, check.Equals, 2)
}

func (m *workshopSketch) TestSketchSdkStashOK(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdStash := cmd.Command()
	cmd.stash = true

	restore := workshop.SketchSdkStash(m.userDataDir, m.prjId, "ws")
	c.Assert(filepath.Join(restore, "sdk.yaml"), testutil.FileAbsent)

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	metadir, _ := m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))

	err := cmd.Run(cmdStash, []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(metadir, testutil.FileAbsent)
	c.Assert(filepath.Join(restore, "sdk.yaml"), testutil.FileEquals, simpleSketchMeta)
	c.Assert(m.stdout.String(), check.Matches, `"ws" sketch stashed\n`)
}

func (m *workshopSketch) TestSketchSdkOverwritesExistingStash(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdStash := cmd.Command()
	cmd.stash = true

	stash := workshop.SketchSdkStash(m.userDataDir, m.prjId, "ws")
	_, stashedhooks := m.mockMinimalSketchSdk(c, "ws", false, []byte(`name: sketch
hooks:
    setup-base: |
        touch /home/workshop/stash
    check-health: |
        exit 0
`))

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	metadir, _ := m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))

	err := cmd.Run(cmdStash, []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(metadir, testutil.FileAbsent)
	c.Assert(filepath.Join(stash, "sdk.yaml"), testutil.FileEquals, simpleSketchMeta)
	c.Assert(filepath.Join(stashedhooks, "setup-base"), testutil.FileAbsent)
	c.Assert(filepath.Join(stashedhooks, "check-health"), testutil.FileAbsent)
}

func (m *workshopSketch) TestSketchSdkStashRevertOnFail(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdStash := cmd.Command()
	cmd.stash = true

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
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{"ws"}, "options": map[string]interface{}{"mode": "transactional", "refresh-option": "update"},
			})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockChangeWithError)
		default:
			c.Errorf("expected 4 calls, now on %d", n)
		}
	})

	metadir, _ := m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))

	err := cmd.Run(cmdStash, nil)
	c.Assert(err, check.NotNil)
	c.Assert(n, check.Equals, 4)

	c.Assert(filepath.Join(metadir, "sdk.yaml"), testutil.FileEquals, simpleSketchMeta)
	restore := workshop.SketchSdkStash(m.userDataDir, m.prjId, "ws")
	recs, err := os.ReadDir(restore)
	c.Assert(recs, check.HasLen, 0)
	c.Assert(err, check.IsNil)
}

func (m *workshopSketch) TestSketchSdkRestoreOK(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRestore := cmd.Command()
	cmd.restore = true

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")
	m.mockMinimalSketchSdk(c, "ws", false, []byte(simpleSketchMeta))

	err := cmd.Run(cmdRestore, []string{"ws"})
	c.Assert(err, check.IsNil)

	current := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "sdk.yaml"), testutil.FileEquals, simpleSketchMeta)
	c.Assert(m.stdout.String(), check.Matches, `"ws" sketch restored\n`)
}

func (m *workshopSketch) TestSketchSdkRestoreNoStoredSketch(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRestore := cmd.Command()
	cmd.restore = true

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")

	err := cmd.Run(cmdRestore, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot restore: stashed "sketch" SDK not found`)
}

func (m *workshopSketch) TestSketchSdkRestoreFailsIfCurrentExists(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRestore := cmd.Command()
	cmd.restore = true

	m.mockSketchHappyRefreshPath(c, "ws", "wait-on-error")
	stored := `name: sketch
plugs:
  gpu:
    interface: gpu
`
	// current
	m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))
	// stored
	m.mockMinimalSketchSdk(c, "ws", false, []byte(stored))

	err := cmd.Run(cmdRestore, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot restore: "sketch" SDK exists; run 'workshop sketch-sdk --remove' to remove it from the workshop`)
}

func (m *workshopSketch) TestSketchSdkEjectOK(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true
	cmd.name = "notsketch"

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	m.mockMinimalSketchSdk(c, "ws", true, []byte(`name: sketch

hooks:
  # comment1
  setup-base: |
    echo "Hello"
# comment2
  check-health: |-
    workshopctl set-health okay

plugs:
  ssh-agent:
   # comment3
    interface: ssh-agent
     # comment4

slots:
  tunnel:
    interface: tunnel
    endpoint: 1234
    # comment5
`))

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Assert(err, check.IsNil)

	current := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Check(current, testutil.DirEquals, []string{})

	notsketch := workshop.ProjectSdkPath(m.prjDir, "notsketch")
	c.Check(filepath.Join(notsketch, "sdk.yaml"), testutil.FileEquals, `name: notsketch
plugs:
  ssh-agent:
    # comment3
    interface: ssh-agent
    # comment4
slots:
  tunnel:
    interface: tunnel
    endpoint: 1234
    # comment5
`)
	c.Check(filepath.Join(notsketch, "hooks", "setup-base"), testutil.FileEquals, `echo "Hello"
`)
	c.Check(filepath.Join(notsketch, "hooks", "check-health"), testutil.FileEquals, `workshopctl set-health okay
`)
	c.Check(m.stdout.String(), check.Equals, `"ws" sketch ejected to ".workshop/notsketch"
To use it, add "project-notsketch" to the list of SDKs and run 'workshop refresh ws'
`)
}

func (m *workshopSketch) TestSketchSdkEjectInferName(c *check.C) {
	m.prjDir = filepath.Join(m.prjDir, "myproject")
	c.Assert(os.Mkdir(m.prjDir, os.ModePerm), check.IsNil)

	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	m.mockMinimalSketchSdk(c, "ws", true, []byte(`name: sketch
`))

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Assert(err, check.IsNil)

	myproject := workshop.ProjectSdkPath(m.prjDir, "myproject")
	c.Check(filepath.Join(myproject, "sdk.yaml"), testutil.FileEquals, `name: myproject
`)
}

func (m *workshopSketch) TestSketchSdkEjectCannotInferName(c *check.C) {
	m.prjDir = filepath.Join(m.prjDir, "_____")

	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Check(err, check.ErrorMatches, `cannot eject: flag --name required: invalid SDK name "_____"`)
}

func (m *workshopSketch) TestSketchSdkEjectPreservesExisting(c *check.C) {
	c.Assert(os.MkdirAll(workshop.ProjectSdkPath(m.prjDir, "name"), os.ModePerm), check.IsNil)

	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true
	cmd.name = "name"

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Check(err, testutil.ErrorIs, os.ErrExist)
}

func (m *workshopSketch) TestSketchSdkEjectCurrentNotExist(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true
	cmd.name = "name"

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot eject: "sketch" SDK not found`)
}

func (m *workshopSketch) TestSketchSdkEjectNoName(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdEject := cmd.Command()
	cmd.eject = true
	cmd.name = "name"

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	sketchDir := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Assert(writeSketchSdk(filepath.Join(sketchDir, "sdk.yaml"), []byte("{}")), check.IsNil)

	err := cmd.Run(cmdEject, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot eject: "sketch" SDK name not found`)
}

func (m *workshopSketch) TestSketchSdkRemoveOK(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRemove := cmd.Command()
	cmd.remove = true

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")
	m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))

	err := cmd.Run(cmdRemove, []string{"ws"})
	c.Assert(err, check.IsNil)

	current := workshop.SketchSdkCurrent(m.userDataDir, m.prjId, "ws")
	c.Assert(current, testutil.DirEquals, []string{})
	c.Assert(m.stdout.String(), check.Matches, `"ws" sketch removed\n`)
}

func (m *workshopSketch) TestSketchSdkRemoveCurrentNotExist(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRemove := cmd.Command()
	cmd.remove = true

	m.mockSketchHappyRefreshPath(c, "ws", "transactional")

	err := cmd.Run(cmdRemove, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot remove: "sketch" SDK not found`)
}

func (m *workshopSketch) TestSketchSdkRemoveRevertOnFail(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdRemove := cmd.Command()
	cmd.remove = true

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
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{
				"action": "refresh",
				"names":  []interface{}{"ws"}, "options": map[string]interface{}{"mode": "transactional", "refresh-option": "update"},
			})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 4:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockChangeWithError)
		default:
			c.Errorf("expected 4 calls, now on %d", n)
		}
	})

	metadir, _ := m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))

	err := cmd.Run(cmdRemove, nil)
	c.Assert(err, check.NotNil)
	c.Assert(n, check.Equals, 4)

	c.Assert(filepath.Join(metadir, "sdk.yaml"), testutil.FileEquals, simpleSketchMeta)
}

func (m *workshopSketch) TestSketchesOK(c *check.C) {
	cmd := &CmdSketches{root: &CmdRoot{}}

	m.mockSketchesHappyPath(c, mockWorkshopsListWithSketch)
	m.mockMinimalSketchSdk(c, "ws", true, []byte(simpleSketchMeta))
	m.mockMinimalSketchSdk(c, "nosketch", false, []byte(simpleSketchMeta))
	m.mockMinimalSketchSdk(c, "both", false, []byte(simpleSketchMeta))

	err := cmd.Run(nil, nil)
	c.Assert(err, check.IsNil)

	c.Assert(m.stdout.String(), check.Matches, fmt.Sprintf(`Project +Workshop  Rev  Notes
%s  ws        x1   current
%s  nosketch  -    stashed
%s  both      x3   current,stashed
`, m.prjDir, m.prjDir, m.prjDir))
}

func (m *workshopSketch) TestSketchesEmpty(c *check.C) {
	cmd := &CmdSketches{root: &CmdRoot{}}

	m.mockSketchesHappyPath(c, mockWorkshopList)

	err := cmd.Run(nil, nil)
	c.Assert(err, check.IsNil)

	c.Assert(m.stdout.String(), check.Matches, "")
}

func (m *workshopSketch) TestSketchSdkWorkshopStatusNotReady(c *check.C) {
	cmd := &CmdSketch{root: &CmdRoot{}}
	cmdStash := cmd.Command()
	cmd.stash = true

	status := []string{"Pending", "Error", "Stopped"}

	n := 0
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3, 5:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2, 4, 6:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockSingleWorkshopSpecifyStatus(status[n/2-1]))
		default:
			c.Errorf("expected 6 calls, now on %d", n)
		}
	})

	for i := 1; i <= len(status); i++ {
		err := cmd.Run(cmdStash, nil)
		c.Assert(err, check.NotNil)
		c.Assert(n, check.Equals, i*2)
	}
}
