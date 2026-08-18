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

package client_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func (cs *clientSuite) TestClientRunWorkshopCtlCallsEndpoint(c *check.C) {
	options := &client.WorkshopCtlOptions{
		ContextID: "1234ABCD",
		Args:      []string{"foo", "bar"},
	}
	cs.cli.RunWorkshopctl(options)
	c.Check(cs.req.Method, check.Equals, "POST")
	c.Check(cs.req.URL.Path, check.Equals, "/v1/workshopctl")
}

func (cs *clientSuite) TestClientRunWorkshoctl(c *check.C) {
	cs.rsp = `{
		"type": "sync",
        "status-code": 200,
		"result": {
			"stdout": "test stdout",
			"stderr": "test stderr"
		}
	}`

	mockStdin := bytes.NewBufferString("some-input")
	options := &client.WorkshopCtlOptions{
		ContextID: "1234ABCD",
		Args:      []string{"foo", "bar"},
		Stdin:     mockStdin,
	}

	stdout, stderr, err := cs.cli.RunWorkshopctl(options)
	c.Assert(err, check.IsNil)
	c.Check(string(stdout), check.Equals, "test stdout")
	c.Check(string(stderr), check.Equals, "test stderr")

	var body map[string]any
	decoder := json.NewDecoder(cs.req.Body)
	err = decoder.Decode(&body)
	c.Check(err, check.IsNil)
	c.Check(body, check.DeepEquals, map[string]any{
		"context-id": "1234ABCD",
		"args":       []any{"foo", "bar"},

		// json byte-stream is b64 encoded
		"stdin": base64.StdEncoding.EncodeToString([]byte("some-input")),
	})
}

func (cs *clientSuite) TestClientRunWorkshoctlReadLimitOneTooMuch(c *check.C) {
	cs.rsp = `{
		"type": "sync",
        "status-code": 200,
		"result": {
		}
	}`

	mockStdin := bytes.NewReader(make([]byte, 4_000_000+1))
	options := &client.WorkshopCtlOptions{
		ContextID: "1234ABCD",
		Args:      []string{"foo", "bar"},
		Stdin:     mockStdin,
	}

	_, _, err := cs.cli.RunWorkshopctl(options)
	c.Check(err, check.ErrorMatches, "cannot read more than 4000000 bytes of data from stdin")
}

func (cs *clientSuite) TestClientRunWorkshoctlReadLimitExact(c *check.C) {
	cs.rsp = `{
		"type": "sync",
        "status-code": 200,
		"result": {
		}
	}`

	mockStdin := bytes.NewReader(make([]byte, 4_000_000))
	options := &client.WorkshopCtlOptions{
		ContextID: "1234ABCD",
		Args:      []string{"foo", "bar"},
		Stdin:     mockStdin,
	}

	_, _, err := cs.cli.RunWorkshopctl(options)
	c.Check(err, check.IsNil)
}
