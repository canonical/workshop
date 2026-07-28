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

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdkstore"
	"github.com/canonical/workshop/internal/sdkstore/transport"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/timeutil"
	"github.com/canonical/workshop/internal/workshop"
)

func (s *apiSuite) createSdksRequest(method, url string) (*http.Request, error) {
	return s.createProjectsRequest(method, url, nil)
}

func (s *apiSuite) TestSdksGetOk(c *check.C) {
	s.daemon(c)

	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "ollama",
			PackageID: "H8HFZNByrb2npBRYjHHYh9kF4qZUSSDy",
			Channel:   "latest/stable",
			Revision:  sdk.R(82),
			Sha3_384:  "9f4936de807da0c0d56f5418f223a4e8f91e99cbba66be6adf560642007e023435a1bfcfc5aa4fc92611dd12f848dd04",
		},
		SdkYAML: `name: ollama
version: 1.0-053c828
summary: Large language model runtime
sdkcraft-started-at: 2024-11-25T00:00:00+00:00
`,
	}
	s.importSdkVolume(c, meta, 109*1024*1024)

	meta = sdk.Meta{
		Setup: sdk.Setup{
			Name:      "openvino",
			PackageID: "o0BEpd0l4HsBviYXxhDPKZ33iDeEumzK",
			Channel:   "latest/stable",
			Revision:  sdk.R(85),
			Sha3_384:  "e7439cbefe3266aa299e09c99a6ce7b0163aceea20a67e178445f588d3433fd7a3cd55036be3f92ceb0cfae54934da73",
		},
		SdkYAML: `name: openvino
version: 2.1-084c8c8
summary: Intel OpenVINO toolkit
sdkcraft-started-at: 2024-11-20T00:00:00+00:00
`,
	}
	s.importSdkVolume(c, meta, 112*1024*1024)

	meta = sdk.Meta{
		Setup: sdk.Setup{
			Name:      "openvino",
			PackageID: "o0BEpd0l4HsBviYXxhDPKZ33iDeEumzK",
			Channel:   "latest/edge",
			Revision:  sdk.R(82),
			Sha3_384:  "6ca811792aa9c9a4859726425bc6f3059bf90aefd1e3371b81bca7317b6429fceb9546fa47457eacbf84b373a8215059",
		},
		SdkYAML: `name: openvino
version: 2.0
summary: Intel OpenVINO toolkit (legacy)
`,
	}
	s.importSdkVolume(c, meta, 101*1024*1024)

	meta = sdk.Meta{
		Setup: sdk.Setup{
			Name:      "ros2",
			PackageID: "dYqYJ4goprUt3GkJo5j63HnLSrHsOaUu",
			Channel:   "jazzy/stable",
			Revision:  sdk.R(5),
			Sha3_384:  "07d4793aff4203fc1e2069bd885a35f177903ea07d215972ebc8dc5e26635775c298c5c80bd336824a22751766005e72",
		},
		SdkYAML: `name: ros2
version: 1.0
summary: ROS2 SDK
`,
	}
	s.importSdkVolume(c, meta, 96*1024*1024)

	meta = sdk.Meta{
		Setup: sdk.Setup{
			Name:     "system",
			Source:   sdk.SystemSource,
			Revision: sdk.R(-1),
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
		},
		SdkYAML: `name: system
`,
	}
	s.importSdkVolume(c, meta, 64*1024*1024)

	req, err := s.createSdksRequest("GET", "/v1/sdks")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdks(apiCmd("/v1/sdks"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	result, ok := rsp.Result.([]sdkstate.SdkVolume)
	c.Assert(ok, check.Equals, true)
	olBuild := time.Date(2024, 11, 25, 0, 0, 0, 0, time.UTC).UTC().Round(0)
	opHigh := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC).UTC().Round(0)
	c.Assert(result, testutil.DeepUnsortedMatches, []sdkstate.SdkVolume{
		{
			Name:     "ollama",
			Version:  "1.0-053c828",
			Revision: "82",
			BuiltAt:  &olBuild,
			Size:     109 * 1024 * 1024,
		},
		{
			Name:     "openvino",
			Version:  "2.1-084c8c8",
			Revision: "85",
			BuiltAt:  &opHigh,
			Size:     112 * 1024 * 1024,
		},
		{
			Name:     "openvino",
			Version:  "2.0",
			Revision: "82",
			Size:     101 * 1024 * 1024,
		},
		{
			Name:     "ros2",
			Version:  "1.0",
			Revision: "5",
			Size:     96 * 1024 * 1024,
		},
		{
			Name:     "system",
			Revision: "x1",
			Size:     64 * 1024 * 1024,
		},
	})
}

func (s *apiSuite) TestSdksGetInvalidMetadata(c *check.C) {
	s.daemon(c)

	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "bad",
			PackageID: "NFOfRaZ9hvTRXfVZBEcC5WJZpqy0ykN6",
			Channel:   "latest/edge",
			Revision:  sdk.R(1),
			Sha3_384:  "71baf962e8a7abd38480289e173d6a48364c8f6f5f8a391fdb83465b4ad676aa7504d100906a0efa8749c97fd61d367e",
		},
		SdkYAML: "[",
	}
	s.importSdkVolume(c, meta, 0)

	req, err := s.createSdksRequest("GET", "/v1/sdks")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdks(apiCmd("/v1/sdks"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Assert(rsp.Status, check.Equals, http.StatusInternalServerError)
}

func (s *apiSuite) importSdkVolume(c *check.C, meta sdk.Meta, size uint64) {
	vfs := c.MkDir()
	path := filepath.Join(vfs, "meta", "sdk.yaml")
	c.Assert(os.MkdirAll(filepath.Dir(path), 0755), check.IsNil)
	c.Assert(os.WriteFile(path, []byte(meta.SdkYAML), 0644), check.IsNil)

	tarball, err := os.Open(vfs)
	c.Assert(err, check.IsNil)
	defer tarball.Close()

	c.Assert(s.b.ImportSdk(s.ctx, meta, tarball), check.IsNil)

	name := sdk.VolumeName(meta.Name, meta.Revision)
	info := s.b.Volumes[name]
	info.Size = size
	s.b.Volumes[name] = info
}

func (s *apiSuite) TestFindSdksOk(c *check.C) {
	s.daemon(c)

	stableReleased := time.Date(2026, 3, 10, 23, 48, 34, 474879000, time.UTC)
	betaReleased := time.Date(2026, 1, 10, 23, 48, 34, 474879000, time.UTC)

	restore := s.store.SetFindCallback(func(ctx context.Context, query string, options ...sdkstore.FindOption) ([]transport.FindResponse, error) {
		if query != "openvino" {
			return nil, nil
		}

		return []transport.FindResponse{{
			Name:      "openvino",
			PackageID: "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
			Metadata: transport.FindMetadata{
				Description: "Accelerated toolkit",
				License:     "Apache-2.0",
				Publisher: transport.Publisher{
					ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
					Username:    "hunter2",
					DisplayName: "Hunter Two",
					Validation:  "unproven",
				},
				Summary: "Intel OpenVINO toolkit",
			},
			DefaultRelease: transport.FindChannelMap{
				Channel: transport.Channel{
					Name:  "latest/stable",
					Track: "latest",
					Risk:  "stable",
					Platform: transport.Platform{
						Name:         "ubuntu",
						Channel:      "20.04",
						Architecture: "amd64",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&stableReleased),
				},
				Revision: 85,
				Version:  "2.1-084c8c8",
			},
		}, {
			Name:      "openvino-notebooks",
			PackageID: "geGY07WPXyvnQahmRP1oOegGUyjurXrY",
			Metadata: transport.FindMetadata{
				Description: "A collection of ready-to-run Jupyter notebooks for learning and experimenting with the OpenVINO™ Toolkit.",
				License:     "Apache-2.0",
				Publisher: transport.Publisher{
					ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
					Username:    "hunter2",
					DisplayName: "Hunter Two",
					Validation:  "unproven",
				},
				Summary: "Jupyter notebook tutorials for OpenVINO",
			},
			DefaultRelease: transport.FindChannelMap{
				Channel: transport.Channel{
					Name:  "latest/beta",
					Track: "latest",
					Risk:  "beta",
					Platform: transport.Platform{
						Name:         "all",
						Channel:      "all",
						Architecture: "all",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&betaReleased),
				},
				Revision: 7,
				Version:  "0.1",
			},
		}}, nil
	})
	defer restore()

	req, err := s.createSdksRequest("GET", "/v1/find?q=openvino")
	c.Assert(err, check.IsNil)

	rsp := v1FindSdks(apiCmd("/v1/find"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	sdks, ok := rsp.Result.([]sdkstate.SdkSummary)
	c.Assert(ok, check.Equals, true)

	c.Check(sdks, testutil.DeepUnsortedMatches, []sdkstate.SdkSummary{{
		Name:        "openvino",
		PackageID:   "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
		Summary:     "Intel OpenVINO toolkit",
		Description: "Accelerated toolkit",
		License:     "Apache-2.0",
		Publisher: &sdkstate.StoreAccount{
			ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			Username:    "hunter2",
			DisplayName: "Hunter Two",
			Validation:  "unproven",
		},
		Channel:    "latest/stable",
		Track:      "latest",
		Risk:       "stable",
		Revision:   "85",
		ReleasedAt: &stableReleased,
		Version:    "2.1-084c8c8",
		Base:       "ubuntu@20.04",
		Arch:       "amd64",
	}, {
		Name:        "openvino-notebooks",
		PackageID:   "geGY07WPXyvnQahmRP1oOegGUyjurXrY",
		Summary:     "Jupyter notebook tutorials for OpenVINO",
		Description: "A collection of ready-to-run Jupyter notebooks for learning and experimenting with the OpenVINO™ Toolkit.",
		License:     "Apache-2.0",
		Publisher: &sdkstate.StoreAccount{
			ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
			Username:    "hunter2",
			DisplayName: "Hunter Two",
			Validation:  "unproven",
		},
		Channel:    "latest/beta",
		Track:      "latest",
		Risk:       "beta",
		Revision:   "7",
		ReleasedAt: &betaReleased,
		Version:    "0.1",
		Base:       "",
		Arch:       "all",
	}})
}

func (s *apiSuite) TestFindSdksNoMatches(c *check.C) {
	s.daemon(c)

	req, err := s.createSdksRequest("GET", "/v1/find?q=openvino")
	c.Assert(err, check.IsNil)

	rsp := v1FindSdks(apiCmd("/v1/find"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	sdks, ok := rsp.Result.([]sdkstate.SdkSummary)
	c.Assert(ok, check.Equals, true)
	c.Check(sdks, check.HasLen, 0)
}

func (s *apiSuite) TestFindSdksStoreDown(c *check.C) {
	s.daemon(c)

	restore := s.store.SetFindCallback(func(ctx context.Context, query string, options ...sdkstore.FindOption) ([]transport.FindResponse, error) {
		return nil, errors.New("destination unreachable")
	})
	defer restore()

	req, err := s.createSdksRequest("GET", "/v1/find?q=openvino")
	c.Assert(err, check.IsNil)

	rsp := v1FindSdks(apiCmd("/v1/find"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Status, check.Equals, http.StatusInternalServerError)
	c.Check(rsp.Result.(*errorResult).Message, check.Equals, `cannot find SDKs matching "openvino": destination unreachable`)
}

func (s *apiSuite) TestSdkInfoGetOk(c *check.C) {
	s.daemon(c)

	stableReleased := time.Date(2026, 3, 10, 23, 48, 34, 474879000, time.UTC)
	stableUploaded := time.Date(2026, 3, 10, 23, 45, 32, 474879000, time.UTC)
	edgeReleased := time.Date(2026, 1, 10, 23, 48, 34, 474879000, time.UTC)
	edgeUploaded := time.Date(2026, 1, 10, 23, 45, 32, 474879000, time.UTC)

	restore := s.store.SetInfoCallback(func(ctx context.Context, name string, options ...sdkstore.InfoOption) (transport.InfoResponse, error) {
		if name != "openvino" {
			return transport.InfoResponse{}, &sdkstore.SdkNotFoundError{Name: name}
		}

		return transport.InfoResponse{
			Name:      "openvino",
			PackageID: "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
			Metadata: transport.InfoMetadata{
				Description: "Accelerated toolkit",
				License:     "Apache-2.0",
				Publisher: transport.Publisher{
					ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
					Username:    "hunter2",
					DisplayName: "Hunter Two",
					Validation:  "unproven",
				},
			},
			ChannelMap: []transport.InfoChannelMap{{
				Channel: transport.Channel{
					Name:  "latest/stable",
					Track: "latest",
					Risk:  "stable",
					Platform: transport.Platform{
						Name:         "ubuntu",
						Channel:      "20.04",
						Architecture: "amd64",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&stableReleased),
				},
				Revision: transport.InfoRevision{
					CreatedAt: (*timeutil.TimeUTC)(&stableUploaded),
					Download:  transport.Download{Size: 1234},
					Revision:  85,
					Version:   "2.1-084c8c8",
					SdkYAML:   json.RawMessage(`{"sdkcraft-started-at": "2024-11-20T00:00:00+00:00","website": "https://openvino.ai"}`),
				},
			}, {
				Channel: transport.Channel{
					Name:  "latest/edge",
					Track: "latest",
					Risk:  "edge",
					Platform: transport.Platform{
						Name:         "ubuntu",
						Channel:      "20.04",
						Architecture: "amd64",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&edgeReleased),
				},
				Revision: transport.InfoRevision{
					CreatedAt: (*timeutil.TimeUTC)(&edgeUploaded),
					Download:  transport.Download{Size: 4321},
					Revision:  82,
					Version:   "2.0",
					SdkYAML:   json.RawMessage(`{"sdkcraft-started-at": "2024-11-25T00:00:00+00:00"}`),
				},
			}},
		}, nil
	})
	defer restore()

	// Create two workshops in the same project.
	s.createWFile(c, "nav2", "name: nav2\nbase: ubuntu@20.04\n")
	s.createWFile(c, "lerobot", "name: lerobot\nbase: ubuntu@20.04\n")

	wf := &workshop.File{Name: "nav2", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	c.Assert(s.b.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot), check.IsNil)

	wf = &workshop.File{Name: "lerobot", Base: "ubuntu@20.04"}
	snapshot = workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	c.Assert(s.b.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot), check.IsNil)

	// Add SDK setups with channels so the endpoint can report channels.
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "openvino",
			PackageID: "o0BEpd0l4HsBviYXxhDPKZ33iDeEumzK",
			Channel:   "latest/stable",
			Revision:  sdk.R(85),
			Sha3_384:  "e7439cbefe3266aa299e09c99a6ce7b0163aceea20a67e178445f588d3433fd7a3cd55036be3f92ceb0cfae54934da73",
		},
		SdkYAML: `name: openvino
version: 2.1-084c8c8
summary: Intel OpenVINO toolkit
description: Latest release
sdkcraft-started-at: 2024-11-20T00:00:00+00:00
`,
	}
	s.importSdkVolume(c, meta, 112*1024*1024)
	c.Assert(s.b.InstallSdk(s.ctx, "nav2", meta.Setup), check.IsNil)

	meta = sdk.Meta{
		Setup: sdk.Setup{
			Name:      "openvino",
			PackageID: "o0BEpd0l4HsBviYXxhDPKZ33iDeEumzK",
			Channel:   "latest/edge",
			Revision:  sdk.R(82),
			Sha3_384:  "6ca811792aa9c9a4859726425bc6f3059bf90aefd1e3371b81bca7317b6429fceb9546fa47457eacbf84b373a8215059",
		},
		SdkYAML: `name: openvino
version: 2.0
summary: Intel OpenVINO toolkit (legacy)
description: Legacy release
website: https://openvino.ai
sdkcraft-started-at: 2024-11-25T00:00:00+00:00
`,
	}
	s.importSdkVolume(c, meta, 101*1024*1024)
	c.Assert(s.b.InstallSdk(s.ctx, "lerobot", meta.Setup), check.IsNil)

	s.vars = map[string]string{"name": "openvino"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/openvino")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	full, ok := rsp.Result.(*sdkstate.SdkFullInfo)
	c.Assert(ok, check.Equals, true)

	c.Check(full.Name, check.Equals, "openvino")
	c.Check(full.PackageID, check.Equals, "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e")
	c.Check(full.Title, check.Equals, "")
	c.Check(full.Summary, check.Not(check.Equals), "")
	c.Check(full.Description, check.Equals, "Accelerated toolkit")
	c.Check(full.License, check.Equals, "Apache-2.0")
	c.Check(full.Website, check.Equals, "https://openvino.ai")
	c.Check(full.Publisher, check.DeepEquals, &sdkstate.StoreAccount{
		ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
		Username:    "hunter2",
		DisplayName: "Hunter Two",
		Validation:  "unproven",
	})

	d20 := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)
	d25 := time.Date(2024, 11, 25, 0, 0, 0, 0, time.UTC)

	c.Check(full.Channels, testutil.DeepUnsortedMatches, []sdkstate.SdkRevision{{
		Channel:      "latest/stable",
		Track:        "latest",
		Risk:         "stable",
		Revision:     "85",
		BuiltAt:      &d20,
		UploadedAt:   &stableUploaded,
		ReleasedAt:   &stableReleased,
		Version:      "2.1-084c8c8",
		Base:         "ubuntu@20.04",
		Arch:         "amd64",
		DownloadSize: 1234,
	}, {
		Channel:      "latest/edge",
		Track:        "latest",
		Risk:         "edge",
		Revision:     "82",
		BuiltAt:      &d25,
		UploadedAt:   &edgeUploaded,
		ReleasedAt:   &edgeReleased,
		Version:      "2.0",
		Base:         "ubuntu@20.04",
		Arch:         "amd64",
		DownloadSize: 4321,
	}})

	c.Check(full.Installed, testutil.DeepUnsortedMatches, []sdkstate.SdkInstalled{
		{
			ProjectPath: s.project.Path,
			Workshop:    "nav2",
			Channel:     "latest/stable",
			Arch:        "all",
			SdkVolume: sdkstate.SdkVolume{
				Name:     "openvino",
				Version:  "2.1-084c8c8",
				Revision: "85",
				BuiltAt:  &d20,
				Size:     112 * 1024 * 1024,
			},
		},
		{
			ProjectPath: s.project.Path,
			Workshop:    "lerobot",
			Channel:     "latest/edge",
			Arch:        "all",
			SdkVolume: sdkstate.SdkVolume{
				Name:     "openvino",
				Version:  "2.0",
				Revision: "82",
				BuiltAt:  &d25,
				Size:     101 * 1024 * 1024,
			},
		},
	})
}

func (s *apiSuite) TestSdkInfoStoreOnly(c *check.C) {
	s.daemon(c)

	stableReleased := time.Date(2026, 3, 10, 23, 48, 34, 474879000, time.UTC)
	stableUploaded := time.Date(2026, 3, 10, 23, 45, 32, 474879000, time.UTC)

	restore := s.store.SetInfoCallback(func(ctx context.Context, name string, options ...sdkstore.InfoOption) (transport.InfoResponse, error) {
		if name != "openvino" {
			return transport.InfoResponse{}, &sdkstore.SdkNotFoundError{Name: name}
		}

		return transport.InfoResponse{
			Name:      "openvino",
			PackageID: "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e",
			Metadata: transport.InfoMetadata{
				Description: "Accelerated toolkit",
				License:     "Apache-2.0",
				Publisher: transport.Publisher{
					ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
					Username:    "hunter2",
					DisplayName: "Hunter Two",
					Validation:  "unproven",
				},
			},
			ChannelMap: []transport.InfoChannelMap{{
				Channel: transport.Channel{
					Name:  "latest/stable",
					Track: "latest",
					Risk:  "stable",
					Platform: transport.Platform{
						Name:         "ubuntu",
						Channel:      "20.04",
						Architecture: "amd64",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&stableReleased),
				},
				Revision: transport.InfoRevision{
					CreatedAt: (*timeutil.TimeUTC)(&stableUploaded),
					Download:  transport.Download{Size: 1234},
					Revision:  85,
					Version:   "2.1-084c8c8",
					SdkYAML:   json.RawMessage(`{"sdkcraft-started-at": "2024-11-20T00:00:00+00:00","website": "https://openvino.ai"}`),
				},
			}},
		}, nil
	})
	defer restore()

	s.vars = map[string]string{"name": "openvino"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/openvino")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	full, ok := rsp.Result.(*sdkstate.SdkFullInfo)
	c.Assert(ok, check.Equals, true)

	c.Check(full.Name, check.Equals, "openvino")
	c.Check(full.PackageID, check.Equals, "U9IBEcXiDkuaPLYjyUBpRZMETAr7g39e")
	c.Check(full.Title, check.Equals, "")
	c.Check(full.Summary, check.Equals, "")
	c.Check(full.Description, check.Equals, "Accelerated toolkit")
	c.Check(full.License, check.Equals, "Apache-2.0")
	c.Check(full.Website, check.Equals, "https://openvino.ai")
	c.Check(full.Publisher, check.DeepEquals, &sdkstate.StoreAccount{
		ID:          "ZeW8fMKBPHZBsaSm6LBPbpDZDpVcIHy1",
		Username:    "hunter2",
		DisplayName: "Hunter Two",
		Validation:  "unproven",
	})

	d20 := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)

	c.Check(full.Channels, check.DeepEquals, []sdkstate.SdkRevision{{
		Channel:      "latest/stable",
		Track:        "latest",
		Risk:         "stable",
		Revision:     "85",
		BuiltAt:      &d20,
		UploadedAt:   &stableUploaded,
		ReleasedAt:   &stableReleased,
		Version:      "2.1-084c8c8",
		Base:         "ubuntu@20.04",
		Arch:         "amd64",
		DownloadSize: 1234,
	}})

	c.Check(full.Installed, check.HasLen, 0)
}

func (s *apiSuite) TestSdkInfoLocalOnly(c *check.C) {
	s.daemon(c)

	s.createWFile(c, "nav2", "name: nav2\nbase: ubuntu@20.04\n")
	wf := &workshop.File{Name: "nav2", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	c.Assert(s.b.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot), check.IsNil)

	// Add SDK setup with channels so the endpoint can report channels.
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "openvino",
			PackageID: "o0BEpd0l4HsBviYXxhDPKZ33iDeEumzK",
			Channel:   "latest/stable",
			Revision:  sdk.R(85),
			Sha3_384:  "e7439cbefe3266aa299e09c99a6ce7b0163aceea20a67e178445f588d3433fd7a3cd55036be3f92ceb0cfae54934da73",
		},
		SdkYAML: `name: openvino
version: 2.1-084c8c8
summary: Intel OpenVINO toolkit
description: Latest release
sdkcraft-started-at: 2024-11-20T00:00:00+00:00
`,
	}
	s.importSdkVolume(c, meta, 112*1024*1024)
	c.Assert(s.b.InstallSdk(s.ctx, "nav2", meta.Setup), check.IsNil)

	s.vars = map[string]string{"name": "openvino"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/openvino")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeSync)
	c.Assert(rsp.Status, check.Equals, http.StatusOK)

	full, ok := rsp.Result.(*sdkstate.SdkFullInfo)
	c.Assert(ok, check.Equals, true)

	c.Check(full.Name, check.Equals, "openvino")
	c.Check(full.PackageID, check.Equals, "")
	c.Check(full.Title, check.Equals, "")
	c.Check(full.Summary, check.Not(check.Equals), "")
	c.Check(full.Description, check.Not(check.Equals), "")
	c.Check(full.License, check.Equals, "")
	c.Check(full.Publisher, check.IsNil)

	c.Check(full.Channels, check.HasLen, 0)

	d20 := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)

	c.Check(full.Installed, check.DeepEquals, []sdkstate.SdkInstalled{
		{
			ProjectPath: s.project.Path,
			Workshop:    "nav2",
			Channel:     "latest/stable",
			Arch:        "all",
			SdkVolume: sdkstate.SdkVolume{
				Name:     "openvino",
				Version:  "2.1-084c8c8",
				Revision: "85",
				BuiltAt:  &d20,
				Size:     112 * 1024 * 1024,
			},
		},
	})
}

func (s *apiSuite) TestSdkInfoStoreDown(c *check.C) {
	s.daemon(c)

	restore := s.store.SetInfoCallback(func(ctx context.Context, name string, options ...sdkstore.InfoOption) (transport.InfoResponse, error) {
		return transport.InfoResponse{}, errors.New("destination unreachable")
	})
	defer restore()

	s.vars = map[string]string{"name": "openvino"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/openvino")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Status, check.Equals, http.StatusInternalServerError)
	c.Check(rsp.Result.(*errorResult).Message, check.Equals, "destination unreachable")
}

func (s *apiSuite) TestSdkInfoInvalidStoreMetadata(c *check.C) {
	s.daemon(c)

	stableReleased := time.Date(2026, 3, 10, 23, 48, 34, 474879000, time.UTC)

	restore := s.store.SetInfoCallback(func(ctx context.Context, name string, options ...sdkstore.InfoOption) (transport.InfoResponse, error) {
		if name != "bad" {
			return transport.InfoResponse{}, &sdkstore.SdkNotFoundError{Name: name}
		}

		return transport.InfoResponse{
			Name: "bad",
			ChannelMap: []transport.InfoChannelMap{{
				Channel: transport.Channel{
					Name:  "latest/stable",
					Track: "latest",
					Risk:  "stable",
					Platform: transport.Platform{
						Name:         "ubuntu",
						Channel:      "20.04",
						Architecture: "amd64",
					},
					ReleasedAt: (*timeutil.TimeUTC)(&stableReleased),
				},
				Revision: transport.InfoRevision{
					Revision: 42,
					SdkYAML:  json.RawMessage(`[`),
				},
			}},
		}, nil
	})
	defer restore()

	s.vars = map[string]string{"name": "bad"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/bad")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)

	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Check(rsp.Status, check.Equals, http.StatusInternalServerError)
	c.Check(rsp.Result.(*errorResult).Message, check.Equals, `invalid "bad" SDK (42) metadata: yaml: line 1: did not find expected node content`)
}

func (s *apiSuite) TestSdkInfoGetInvalidLocalMetadata(c *check.C) {
	s.daemon(c)

	s.createWFile(c, "ws", "name: ws\nbase: ubuntu@20.04\n")
	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04"}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	c.Assert(s.b.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot), check.IsNil)

	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "bad",
			PackageID: "NFOfRaZ9hvTRXfVZBEcC5WJZpqy0ykN6",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "71baf962e8a7abd38480289e173d6a48364c8f6f5f8a391fdb83465b4ad676aa7504d100906a0efa8749c97fd61d367e",
		},
		SdkYAML: "[",
	}
	s.importSdkVolume(c, meta, 0)
	c.Assert(s.b.InstallSdk(s.ctx, "ws", meta.Setup), check.IsNil)

	s.vars = map[string]string{"name": "bad"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/bad")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)
	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Assert(rsp.Status, check.Equals, http.StatusInternalServerError)
}

func (s *apiSuite) TestSdkInfoGetNotFound(c *check.C) {
	s.daemon(c)

	s.vars = map[string]string{"name": "bad"}
	req, err := s.createSdksRequest("GET", "/v1/sdks/not-found")
	c.Assert(err, check.IsNil)

	rsp := v1GetSdkInfo(apiCmd("/v1/sdks/{name}"), req, nil).(*resp)
	c.Assert(rsp.Type, check.Equals, ResponseTypeError)
	c.Assert(rsp.Status, check.Equals, http.StatusNotFound)
}
