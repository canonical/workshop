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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func TestT(t *testing.T) { check.TestingT(t) }

type workshopctlSuite struct {
	server            *httptest.Server
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
	s.expectedContextID = "workshop-context-test"
	s.expectedArgs = []string{}
}

func (s *workshopctlSuite) TearDownTest(c *check.C) {
	c.Assert(os.Unsetenv("WORKSHOP_COOKIE"), check.IsNil)
	clientConfig.BaseURL = ""
	s.server.Close()
}

func (s *workshopctlSuite) TestWorkshopctl(c *check.C) {
	var stdout, stderr bytes.Buffer
	c.Check(run([]string{}, nil, &stdout, &stderr), check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "test stdout")
	c.Check(stderr.String(), check.Equals, "test stderr")
}

func (s *workshopctlSuite) TestWorkshopctlWithArgs(c *check.C) {
	s.expectedArgs = []string{"foo", "--bar"}
	var stdout, stderr bytes.Buffer
	c.Check(run([]string{"foo", "--bar"}, nil, &stdout, &stderr), check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "test stdout")
	c.Check(stderr.String(), check.Equals, "test stderr")
}

func (s *workshopctlSuite) TestWorkshopctlHelp(c *check.C) {
	os.Unsetenv("WORKSHOP_COOKIE")
	s.expectedContextID = ""
	s.expectedArgs = []string{"-h"}

	var stdout, stderr bytes.Buffer
	c.Check(run([]string{"-h"}, nil, &stdout, &stderr), check.Equals, 0)
}

// TestWorkshopctlStdinNotForwarded checks that stdin is not forwarded to
// the daemon by default; interceptors must opt in to forwarding it.
func (s *workshopctlSuite) TestWorkshopctlStdinNotForwarded(c *check.C) {
	s.expectedStdin = nil
	mockStdin := &fakeFdReader{Reader: bytes.NewReader([]byte("hello"))}

	var stdout, stderr bytes.Buffer
	c.Check(run([]string{}, mockStdin, &stdout, &stderr), check.Equals, 0)
}

// fakeFdReader is an fdReader backed by an in-memory reader, for tests that
// do not exercise the file descriptor.
type fakeFdReader struct {
	io.Reader
}

func (f *fakeFdReader) Fd() uintptr {
	return 0
}

// TestWorkshopctlGetSecretSystemd checks that get-secret --systemd decodes
// the request from the stdin connection and forwards it to the daemon as a
// regular get-secret invocation.
func (s *workshopctlSuite) TestWorkshopctlGetSecretSystemd(c *check.C) {
	addr := "\x00" + fmt.Sprintf("%d-main", os.Getpid()) + "/unit/ollama.service/ollama.ollama-api-key"
	name := filepath.Join(c.MkDir(), "conn")

	server, err := net.ListenUnix("unix", &net.UnixAddr{Name: name})
	c.Assert(err, check.IsNil)
	defer server.Close()

	type acceptResult struct {
		conn *net.UnixConn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := server.AcceptUnix()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	clientConn, err := net.DialUnix("unix",
		&net.UnixAddr{Name: addr},
		&net.UnixAddr{Name: name},
	)
	c.Assert(err, check.IsNil)
	defer clientConn.Close()

	result := <-accepted
	c.Assert(result.err, check.IsNil)
	defer result.conn.Close()

	f, err := result.conn.File()
	c.Assert(err, check.IsNil)
	defer f.Close()

	s.expectedArgs = []string{"get-secret", "ollama.ollama-api-key"}

	var stdout, stderr bytes.Buffer
	args := []string{"get-secret", "--systemd"}
	c.Check(run(args, f, &stdout, &stderr), check.Equals, 0)
	c.Check(stdout.String(), check.Equals, "test stdout")
	c.Check(stderr.String(), check.Equals,
		"processed systemd load credential request for unit \"ollama.service\", sdk \"ollama\" and secret \"ollama-api-key\"")
}
