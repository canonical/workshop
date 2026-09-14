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
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
)

func (s *sdkSuite) TestInfo(c *check.C) {
	d1 := time.Date(2024, 11, 25, 0, 0, 0, 0, time.UTC)
	u1 := d1.Add(24 * time.Hour)
	r1 := u1.Add(24 * time.Hour)
	d2 := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)

	home := c.MkDir()
	nav := filepath.Join(home, "work", "nav2")
	lerobot := filepath.Join(home, "work", "lerobot")

	resp := client.SdkFullInfo{
		Name:        "openvino",
		PackageID:   "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
		Title:       "OpenVINO",
		Summary:     "Intel OpenVINO toolkit",
		Description: "Longer description\ncan be multiline.",
		License:     "Apache-2.0",
		Website:     "https://openvino.ai",
		Publisher: &client.StoreAccount{
			ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			Username:    "intel",
			DisplayName: "Intel",
			Validation:  "verified",
		},
		Channels: []*client.SdkRevision{{
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "85",
			BuiltAt:      &d1,
			UploadedAt:   &u1,
			ReleasedAt:   &r1,
			Version:      "2.1-084c8c8",
			Base:         "ubuntu@24.04",
			Arch:         "amd64",
			DownloadSize: 123,
		}, {
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "86",
			BuiltAt:      &d1,
			UploadedAt:   &u1,
			ReleasedAt:   &r1,
			Version:      "2.1-084c8c8",
			Base:         "ubuntu@24.04",
			Arch:         "arm64",
			DownloadSize: 1234,
		}, {
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "87",
			BuiltAt:      &d1,
			UploadedAt:   &u1,
			ReleasedAt:   &r1,
			Version:      "2.1-084c8c8",
			Base:         "ubuntu@24.04",
			Arch:         "riscv64",
			DownloadSize: 12345,
		}, {
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "88",
			BuiltAt:      &d1,
			UploadedAt:   &u1,
			ReleasedAt:   &r1,
			Version:      "2.1-084c8c8",
			Base:         "ubuntu@26.04",
			Arch:         "amd64",
			DownloadSize: 123456,
		}, {
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "89",
			BuiltAt:      &d1,
			UploadedAt:   &u1,
			ReleasedAt:   &r1,
			Version:      "2.1-084c8c8",
			Base:         "ubuntu@26.04",
			Arch:         "arm64",
			DownloadSize: 1234567,
		}, {
			Channel:      "latest/stable",
			Track:        "latest",
			Risk:         "stable",
			Revision:     "90",
			BuiltAt:      &r1,
			UploadedAt:   &r1,
			ReleasedAt:   &r1,
			Version:      "2.2-c8c8084",
			Base:         "ubuntu@26.04",
			Arch:         "riscv64",
			DownloadSize: 12345678,
		}, {
			Channel:      "latest/edge",
			Track:        "latest",
			Risk:         "edge",
			Revision:     "91",
			BuiltAt:      &d2,
			UploadedAt:   &d2,
			ReleasedAt:   &d2,
			Version:      "2.0",
			Base:         "",
			Arch:         "all",
			DownloadSize: 12345678,
		}},
		Installed: []client.SdkInstalled{
			{
				ProjectPath: nav,
				Workshop:    "ci",
				Channel:     "latest/stable",
				Base:        "ubuntu@24.04",
				Arch:        "amd64",
				SdkVolume: client.SdkVolume{
					Version:  "2.1-084c8c8",
					Revision: "85",
					BuiltAt:  &d1,
					Size:     109 * 1024 * 1024,
				},
			},
			{
				ProjectPath: nav,
				Workshop:    "dev",
				Channel:     "latest/stable",
				Base:        "ubuntu@24.04",
				Arch:        "amd64",
				SdkVolume: client.SdkVolume{
					Version:  "2.1-084c8c8",
					Revision: "85",
					BuiltAt:  &d1,
					Size:     109 * 1024 * 1024,
				},
			},
			{
				ProjectPath: lerobot,
				Workshop:    "dev",
				Channel:     "latest/edge",
				Base:        "",
				Arch:        "all",
				SdkVolume: client.SdkVolume{
					Version:  "2.0",
					Revision: "82",
					BuiltAt:  &d2,
					Size:     102 * 1024 * 1024,
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, check.Equals, "GET")
		c.Assert(r.URL.Path, check.Equals, "/v1/sdks/openvino")
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
	cmd.SetArgs([]string{"info", "openvino", "--arch=all"})
	c.Assert(cmd.Execute(), check.IsNil)

	maxProject := max(len("PROJECT"), len(nav), len(lerobot))
	want := fmt.Sprintf(`name:       openvino
publisher:  Intel**
license:    Apache-2.0
website:    https://openvino.ai

Longer description
can be multiline.

CHANNELS
  CHANNEL           VERSION      BUILD       BASE          ARCH     REV      SIZE
  latest/stable     2.1-084c8c8  2024-11-25  ubuntu@26.04  amd64     88  123.46kB
                                                           arm64     89    1.23MB
                    2.2-c8c8084  2024-11-27  ubuntu@26.04  riscv64   90   12.35MB
                    2.1-084c8c8  2024-11-25  ubuntu@24.04  amd64     85      123B
                                                           arm64     86    1.23kB
                                                           riscv64   87   12.35kB
  latest/candidate  ^                                                    
  latest/beta       ^                                                    
  latest/edge       2.0          2024-11-20  all           all       91   12.35MB

INSTALLED
  %-*s  WORKSHOP  CHANNEL        VERSION      BASE          ARCH   REV
  %-*s  dev       latest/edge    2.0          all           all     82
  %-*s  ci        latest/stable  2.1-084c8c8  ubuntu@24.04  amd64   85
  %-*s  dev       latest/stable  2.1-084c8c8  ubuntu@24.04  amd64   85
`, maxProject, "PROJECT", maxProject, lerobot, maxProject, nav, maxProject, nav)

	c.Check(s.Stdout(), check.Equals, want)
	s.ResetStdStreams()

	cmd = (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"info", "openvino", "--arch=amd64"})
	c.Assert(cmd.Execute(), check.IsNil)

	want = fmt.Sprintf(`name:       openvino
publisher:  Intel**
license:    Apache-2.0
website:    https://openvino.ai

Longer description
can be multiline.

CHANNELS
  CHANNEL           VERSION      BUILD       BASE          REV      SIZE
  latest/stable     2.1-084c8c8  2024-11-25  ubuntu@26.04   88  123.46kB
                                             ubuntu@24.04   85      123B
  latest/candidate  ^                                           
  latest/beta       ^                                           
  latest/edge       2.0          2024-11-20  all            91   12.35MB

INSTALLED
  %-*s  WORKSHOP  CHANNEL        VERSION      BASE          REV
  %-*s  dev       latest/edge    2.0          all            82
  %-*s  ci        latest/stable  2.1-084c8c8  ubuntu@24.04   85
  %-*s  dev       latest/stable  2.1-084c8c8  ubuntu@24.04   85
`, maxProject, "PROJECT", maxProject, lerobot, maxProject, nav, maxProject, nav)

	c.Check(s.Stdout(), check.Equals, want)
	s.ResetStdStreams()

	cmd = (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"info", "openvino", "--arch=riscv64"})
	c.Assert(cmd.Execute(), check.IsNil)

	maxProject = max(len("PROJECT"), len(lerobot))
	want = fmt.Sprintf(`name:       openvino
publisher:  Intel**
license:    Apache-2.0
website:    https://openvino.ai

Longer description
can be multiline.

CHANNELS
  CHANNEL           VERSION      BUILD       BASE          REV     SIZE
  latest/stable     2.2-c8c8084  2024-11-27  ubuntu@26.04   90  12.35MB
                    2.1-084c8c8  2024-11-25  ubuntu@24.04   87  12.35kB
  latest/candidate  ^                                           
  latest/beta       ^                                           
  latest/edge       2.0          2024-11-20  all            91  12.35MB

INSTALLED
  %-*s  WORKSHOP  CHANNEL      VERSION  BASE  REV
  %-*s  dev       latest/edge  2.0      all    82
`, maxProject, "PROJECT", maxProject, lerobot)

	c.Check(s.Stdout(), check.Equals, want)
	s.ResetStdStreams()

	cmd = (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"info", "openvino", "--arch=all", "--base=ubuntu@26.04"})
	c.Assert(cmd.Execute(), check.IsNil)

	want = fmt.Sprintf(`name:       openvino
publisher:  Intel**
license:    Apache-2.0
website:    https://openvino.ai

Longer description
can be multiline.

CHANNELS
  CHANNEL           VERSION      BUILD       ARCH     REV      SIZE
  latest/stable     2.1-084c8c8  2024-11-25  amd64     88  123.46kB
                                             arm64     89    1.23MB
                    2.2-c8c8084  2024-11-27  riscv64   90   12.35MB
  latest/candidate  ^                                      
  latest/beta       ^                                      
  latest/edge       2.0          2024-11-20  all       91   12.35MB

INSTALLED
  %-*s  WORKSHOP  CHANNEL      VERSION  ARCH  REV
  %-*s  dev       latest/edge  2.0      all    82
`, maxProject, "PROJECT", maxProject, lerobot)

	c.Check(s.Stdout(), check.Equals, want)
	s.ResetStdStreams()

	cmd = (&CmdRoot{}).Command()
	cmd.SetArgs([]string{"info", "openvino", "--arch=amd64", "--base=ubuntu@24.04"})
	c.Assert(cmd.Execute(), check.IsNil)

	maxProject = max(len("PROJECT"), len(nav), len(lerobot))
	want = fmt.Sprintf(`name:       openvino
publisher:  Intel**
license:    Apache-2.0
website:    https://openvino.ai

Longer description
can be multiline.

CHANNELS
  CHANNEL           VERSION      BUILD       REV     SIZE
  latest/stable     2.1-084c8c8  2024-11-25   85     123B
  latest/candidate  ^                             
  latest/beta       ^                             
  latest/edge       2.0          2024-11-20   91  12.35MB

INSTALLED
  %-*s  WORKSHOP  CHANNEL        VERSION      REV
  %-*s  dev       latest/edge    2.0           82
  %-*s  ci        latest/stable  2.1-084c8c8   85
  %-*s  dev       latest/stable  2.1-084c8c8   85
`, maxProject, "PROJECT", maxProject, lerobot, maxProject, nav, maxProject, nav)

	c.Check(s.Stdout(), check.Equals, want)
	s.ResetStdStreams()
}

func (s *sdkSuite) TestInfoUnverifiedPublisher(c *check.C) {
	resp := client.SdkFullInfo{
		Name:      "openvimo",
		PackageID: "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
		Publisher: &client.StoreAccount{
			ID:          "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
			Username:    "hunter2",
			DisplayName: "Hunter Two",
			Validation:  "unproven",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, check.Equals, "GET")
		c.Assert(r.URL.Path, check.Equals, "/v1/sdks/openvimo")
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
	cmd.SetArgs([]string{"info", "openvimo"})
	c.Assert(cmd.Execute(), check.IsNil)

	want := `name:       openvimo
publisher:  Hunter Two (hunter2)
`
	c.Check(s.Stdout(), check.Equals, want)
}
