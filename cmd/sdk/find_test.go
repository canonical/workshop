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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func (s *sdkSuite) TestFind(c *check.C) {
	d1 := time.Date(2024, 11, 25, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)

	resp := []client.SdkSummary{{
		Name:        "openvino",
		PackageID:   "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
		Summary:     "Intel OpenVINO toolkit",
		Description: "Longer description\ncan be multiline.",
		License:     "Apache-2.0",
		Publisher: &client.StoreAccount{
			ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			Username:    "intel",
			DisplayName: "Intel",
			Validation:  "verified",
		},
		Channel:    "latest/stable",
		Track:      "latest",
		Risk:       "stable",
		Revision:   "85",
		ReleasedAt: &d1,
		Version:    "2.1-084c8c8",
		Base:       "ubuntu@24.04",
		Arch:       "amd64",
	}, {
		Name:        "openvino-notebooks",
		PackageID:   "geGY07WPXyvnQahmRP1oOegGUyjurXrY",
		Summary:     "Jupyter notebook tutorials for OpenVINO",
		Description: "A collection of ready-to-run Jupyter notebooks for learning and experimenting with the OpenVINO™ Toolkit.",
		License:     "Apache-2.0",
		Publisher: &client.StoreAccount{
			ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			Username:    "hunter2",
			DisplayName: "Hunter Two",
			Validation:  "unproven",
		},
		Channel:    "latest/beta",
		Track:      "latest",
		Risk:       "beta",
		Revision:   "7",
		ReleasedAt: &d2,
		Version:    "0.1",
		Base:       "",
		Arch:       "all",
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, check.Equals, "GET")
		c.Assert(r.URL.Path, check.Equals, "/v1/find")
		c.Assert(r.URL.Query().Get("q"), check.Equals, "openvino")
		body := map[string]any{
			"type":   "sync",
			"result": resp,
		}
		encoder := json.NewEncoder(w)
		c.Assert(encoder.Encode(body), check.IsNil)
	}))
	defer srv.Close()

	ClientConfig.BaseURL = srv.URL

	cmd := (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"find", "openvino"})
	c.Assert(cmd.Execute(), check.IsNil)

	want := `NAME                VERSION      PUBLISHER   SUMMARY
openvino            2.1-084c8c8  Intel**     Intel OpenVINO toolkit
openvino-notebooks  0.1          Hunter Two  Jupyter notebook tutorials for OpenVINO
`
	c.Check(s.Stdout(), check.Equals, want)
}
