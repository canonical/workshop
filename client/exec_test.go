// Copyright (c) 2021 Canonical Ltd
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

package client_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

type execSuite struct {
	clientSuite
	controlWs *testWebsocket
	stdioWs   *testWebsocket
	stdoutWs  *testWebsocket
	stderrWs  *testWebsocket
}

var _ = Suite(&execSuite{})

var (
	websocketRedirectexp = regexp.MustCompile(`^http://localhost/v1/tasks/T\d+/websocket/(\w+)$`)
	websocketRegexp      = regexp.MustCompile(`^ws://localhost/v1/tasks/T\d+/websocket/(\w+)$`)
)

func (s *execSuite) SetUpTest(c *C) {
	s.clientSuite.SetUpTest(c)

	s.cli.SetDoer(s)

	s.stdioWs = &testWebsocket{}
	s.stdoutWs = &testWebsocket{}
	s.controlWs = &testWebsocket{}
	s.stderrWs = &testWebsocket{}
	s.cli.SetGetWebsocket(func(url string) (client.ClientWebsocket, error) {
		matches := websocketRegexp.FindStringSubmatch(url)
		if matches == nil {
			return nil, fmt.Errorf("invalid websocket URL %q", url)
		}
		id := matches[1]
		switch id {
		case "control":
			return s.controlWs, nil
		case "stdio":
			return s.stdioWs, nil
		case "stdout":
			return s.stdoutWs, nil
		case "stderr":
			return s.stderrWs, nil
		default:
			return nil, fmt.Errorf("invalid websocket ID %q", id)
		}
	})
}

func (cs *execSuite) Do(req *http.Request) (*http.Response, error) {
	if req.Method == "GET" {
		// check if the request is after a websocket. these are redirected
		// to LXD now
		matches := websocketRedirectexp.FindStringSubmatch(req.URL.String())
		if matches != nil {
			header := make(http.Header)
			header.Set("Location", fmt.Sprintf("ws://localhost%s", req.URL.Path))
			return &http.Response{
				StatusCode: http.StatusTemporaryRedirect,
				Header:     header,
			}, nil
		}
	}
	return cs.clientSuite.Do(req)
}

func (s *execSuite) TearDownTest(c *C) {
	s.clientSuite.TearDownTest(c)
}

func (s *execSuite) TestExitZero(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"true"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"true"},
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
}

func (s *execSuite) TestExitNonZero(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"false"},
	}
	process, reqBody := s.exec(c, opts, 1)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"false"},
	})
	err := s.wait(c, process)
	exitError, ok := err.(*client.ExitError)
	c.Assert(ok, Equals, true, Commentf("expected *client.ExitError, got %T", err))
	c.Assert(exitError.ExitCode(), Equals, 1)
}

func (s *execSuite) TestTimeout(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"sleep", "3"},
		Timeout: time.Second,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"sleep", "3"},
		"timeout": "1s",
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
	c.Assert(s.req.URL.String(), Equals, "http://localhost/v1/changes/123/wait?timeout=2s")
}

func (s *execSuite) TestOtherOptions(c *C) {
	userID := 1000
	groupID := 2000
	opts := &client.ExecOptions{
		Command:     []string{"echo", "foo"},
		Environment: map[string]string{"K1": "V1", "K2": "V2"},
		WorkingDir:  "WD",
		UserId:      &userID,
		GroupId:     &groupID,
		Terminal:    true,
		Width:       12,
		Height:      34,
		Stderr:      io.Discard,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command":     []interface{}{"echo", "foo"},
		"environment": map[string]interface{}{"K1": "V1", "K2": "V2"},
		"working-dir": "WD",
		"user-id":     1000.0,
		"group-id":    2000.0,
		"terminal":    true,
		"width":       12.0,
		"height":      34.0,
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
}

func (s *execSuite) TestWaitChangeError(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"foo"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"foo"},
	})

	// Make /v1/changes/{id}/wait return a "change error"
	s.rsps[len(s.rsps)-1] = `{
		"result": {
			"id": "123",
			"kind": "exec",
			"ready": true,
			"err": "change error!"
		},
		"status": "OK",
		"status-code": 200,
		"type": "sync"
	}`
	err := s.wait(c, process)
	c.Assert(err, ErrorMatches, "change error!")
}

func (s *execSuite) TestWaitTasksError(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"foo"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"foo"},
	})

	// Make /v1/changes/{id}/wait return no tasks
	s.rsps[len(s.rsps)-1] = `{
		"result": {
			"id": "123",
			"kind": "exec",
			"ready": true
		},
		"status": "OK",
		"status-code": 200,
		"type": "sync"
	}`
	err := s.wait(c, process)
	c.Assert(err, ErrorMatches, "expected exec change to contain an exec task")
}

func (s *execSuite) TestWaitExitCodeError(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"foo"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"foo"},
	})

	// Make /v1/changes/{id}/wait return no exit code
	s.rsps[len(s.rsps)-1] = `{
		"result": {
			"id": "123",
			"kind": "exec",
			"ready": true,
			"tasks": [{
				"id": "T123",
				"kind": "exec"
			}]
		},
		"status": "OK",
		"status-code": 200,
		"type": "sync"
	}`
	err := s.wait(c, process)
	c.Assert(err, ErrorMatches, "cannot get exit code: .*")
}

func (s *execSuite) TestSendSignal(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"server"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"server"},
	})
	err := process.SendSignal(syscall.SIGHUP)
	c.Assert(err, IsNil)
	err = process.SendSignal(syscall.SIGUSR1)
	c.Assert(err, IsNil)
	c.Assert(s.controlWs.writes, DeepEquals, []write{
		{websocket.TextMessage, `{"command":"signal","args":null,"signal":1}`},
		{websocket.TextMessage, `{"command":"signal","args":null,"signal":10}`},
	})
}

func (s *execSuite) TestSendResize(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"server"},
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"server"},
	})
	err := process.SendResize(150, 50)
	c.Assert(err, IsNil)
	err = process.SendResize(80, 25)
	c.Assert(err, IsNil)
	c.Assert(s.controlWs.writes, DeepEquals, []write{
		{websocket.TextMessage, `{"command":"window-resize","args":{"height":"50","width":"150"},"signal":0}`},
		{websocket.TextMessage, `{"command":"window-resize","args":{"height":"25","width":"80"},"signal":0}`},
	})
}

func (s *execSuite) TestOutputCombinedForInteractive(c *C) {
	stdout := bytes.Buffer{}
	s.stdioWs.reads = append(s.stdioWs.reads,
		read{websocket.BinaryMessage, "OUT\n"},
		read{websocket.BinaryMessage, "ERR\n"},
		read{websocket.TextMessage, ``},
	)
	opts := &client.ExecOptions{
		Command:     []string{"/bin/sh", "-c", "echo OUT; echo ERR >err"},
		Stdout:      &stdout,
		Interactive: true,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command":     []interface{}{"/bin/sh", "-c", "echo OUT; echo ERR >err"},
		"interactive": true,
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, "OUT\nERR\n")
}

func (s *execSuite) TestOutputSplitForNonInteractive(c *C) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	s.stdoutWs.reads = append(s.stdoutWs.reads,
		read{websocket.BinaryMessage, "OUT\n"},
		read{websocket.TextMessage, ``},
	)
	s.stderrWs.reads = append(s.stderrWs.reads,
		read{websocket.BinaryMessage, "ERR\n"},
		read{websocket.TextMessage, ``},
	)
	opts := &client.ExecOptions{
		Command: []string{"/bin/sh", "-c", "echo OUT; echo ERR >err"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"/bin/sh", "-c", "echo OUT; echo ERR >err"},
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, "OUT\n")
	c.Assert(stderr.String(), Equals, "ERR\n")
}

func (s *execSuite) TestStdinAndStdout(c *C) {
	stdout := bytes.Buffer{}
	s.stdoutWs.reads = append(s.stdoutWs.reads,
		read{websocket.BinaryMessage, "FOO\nBAR BAZZ\n"},
		read{websocket.TextMessage, ""},
	)
	opts := &client.ExecOptions{
		Command: []string{"awk", "{ print toupper($0) }"},
		Stdin:   bytes.NewBufferString("foo\nBar BAZZ\n"),
		Stdout:  &stdout,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"awk", "{ print toupper($0) }"},
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
	c.Check(stdout.String(), Equals, "FOO\nBAR BAZZ\n")
	c.Assert(s.stdioWs.writes, DeepEquals, []write{
		{websocket.BinaryMessage, "foo\nBar BAZZ\n"},
		{websocket.TextMessage, ""},
	})
}

func (s *execSuite) TestAction(c *C) {
	opts := &client.ExecOptions{
		Command: []string{"tests"},
		Action:  true,
	}
	process, reqBody := s.exec(c, opts, 0)
	c.Assert(reqBody, DeepEquals, map[string]interface{}{
		"command": []interface{}{"tests"},
		"action":  true,
	})
	err := s.wait(c, process)
	c.Assert(err, IsNil)
}

type testWebsocket struct {
	reads  []read
	writes []write
}

type read struct {
	messageType int
	data        string
}

type write struct {
	messageType int
	data        string
}

func (w *testWebsocket) NextReader() (messageType int, r io.Reader, err error) {
	if len(w.reads) == 0 {
		return websocket.CloseMessage, nil, nil
	}
	read := w.reads[0]
	w.reads = w.reads[1:]
	return read.messageType, bytes.NewBufferString(read.data), nil
}

func (w *testWebsocket) WriteMessage(messageType int, data []byte) error {
	w.writes = append(w.writes, write{messageType, string(data)})
	return nil
}

func (w *testWebsocket) Close() error {
	return nil
}

func (w *testWebsocket) WriteJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.writes = append(w.writes, write{websocket.TextMessage, string(data)})
	return nil
}

func (s *execSuite) exec(c *C, opts *client.ExecOptions, exitCode int) (process *client.ExecProcess, requestBody map[string]interface{}) {
	if opts.Action {
		s.addRunResponses("123", exitCode)
	} else {
		s.addResponses("123", exitCode)
	}
	process, err := s.cli.Exec(opts, "workshop", "42424242")
	c.Assert(err, IsNil)
	c.Assert(s.req.Method, Equals, "POST")
	c.Assert(s.req.URL.String(), Equals, "http://localhost/v1/projects/42424242/workshops/workshop/exec")
	err = json.NewDecoder(s.req.Body).Decode(&requestBody)
	c.Assert(err, IsNil)
	return process, requestBody
}

func (s *execSuite) wait(c *C, process *client.ExecProcess) error {
	err := process.Wait()
	c.Assert(s.req.Method, Equals, "GET")
	c.Assert(s.req.URL.Scheme, Equals, "http")
	c.Assert(s.req.URL.Host, Equals, "localhost")
	c.Assert(s.req.URL.Path, Equals, "/v1/changes/123/wait")
	process.WaitStdinDone()
	return err
}

func (s *execSuite) addResponses(changeID string, exitCode int) {
	// Add /v1/exec response
	taskID := "T" + changeID
	s.rsps = append(s.rsps, fmt.Sprintf(`{
		"change": "%s",
		"result": {"task-id": "%s"},
		"status": "Accepted",
		"status-code": 202,
		"type": "async"
	}`, changeID, taskID))

	// Add /v1/changes/{id}/wait response
	s.rsps = append(s.rsps, fmt.Sprintf(`{
		"result": {
			"id": "%s",
			"kind": "exec",
			"ready": true,
			"tasks": [{
				"data": {"exit-code": %d},
				"id": "%s",
				"kind": "exec"
			}]
		},
		"status": "OK",
		"status-code": 200,
		"type": "sync"
	}`, changeID, exitCode, taskID))
}

func (s *execSuite) addRunResponses(changeID string, exitCode int) {
	// Add /v1/exec response
	taskID := "T" + changeID
	s.rsps = append(s.rsps, fmt.Sprintf(`{
		"change": "%s",
		"result": {"task-id": "%s"},
		"status": "Accepted",
		"status-code": 202,
		"type": "async"
	}`, changeID, taskID))

	// Add /v1/changes/{id}/wait response
	installID := "I" + changeID
	s.rsps = append(s.rsps, fmt.Sprintf(`{
		"result": {
			"id": "%s",
			"kind": "exec",
			"ready": true,
			"tasks": [{
				"id": "%s",
				"kind": "install-action"
			}, {
				"data": {"exit-code": %d},
				"id": "%s",
				"kind": "exec"
			}]
		},
		"status": "OK",
		"status-code": 200,
		"type": "sync"
	}`, changeID, installID, exitCode, taskID))
}
