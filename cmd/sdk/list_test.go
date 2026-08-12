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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/testutil"
)

type sdkSuite struct {
	testutil.BaseTest
	stdout bytes.Buffer
	stderr bytes.Buffer
}

var _ = check.Suite(&sdkSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *sdkSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)

	Stdout = &s.stdout
	Stderr = &s.stderr
}

func (s *sdkSuite) TearDownTest(_ *check.C) {
	s.stdout.Reset()
	s.stderr.Reset()
	Stdout = os.Stdout
	Stderr = os.Stderr
	ClientConfig.BaseURL = ""
}

func (s *sdkSuite) Stdout() string {
	return s.stdout.String()
}

func (s *sdkSuite) ResetStdStreams() {
	s.stdout.Reset()
	s.stderr.Reset()
}

func (s *sdkSuite) TestList(c *check.C) {
	sdks := []client.SdkVolume{
		{Name: "ollama", Version: "1.0-053", Revision: "82"},
		{Name: "ros2", Revision: "5", Size: 1024 * 1024},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, check.Equals, "GET")
		c.Assert(r.URL.Path, check.Equals, "/v1/sdks")
		body := map[string]any{
			"type":   "sync",
			"result": sdks,
		}
		encoder := json.NewEncoder(w)
		c.Assert(encoder.Encode(body), check.IsNil)
	}))
	defer srv.Close()

	ClientConfig.BaseURL = srv.URL

	cmd := (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"list"})
	c.Assert(cmd.Execute(), check.IsNil)

	c.Check(s.Stdout(), check.Equals, `NAME    VERSION  REV    SIZE
ollama  1.0-053   82      0B
ros2    -          5  1.05MB
`)
	s.ResetStdStreams()

	cmd = (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"list", "--no-headers"})
	c.Assert(cmd.Execute(), check.IsNil)

	c.Check(s.Stdout(), check.Equals, `ollama  1.0-053  82      0B
ros2    -         5  1.05MB
`)
	s.ResetStdStreams()
}
