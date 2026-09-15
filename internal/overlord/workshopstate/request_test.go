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

package workshopstate_test

import (
	"bytes"
	"context"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type requestSuite struct {
	state   *state.State
	user    *user.User
	project workshop.Project
	backend *fakebackend.FakeWorkshopBackend
	mgr     *workshopstate.WorkshopManager
	ctx     context.Context

	restoreUserLookup func()
	restoreUserEnv    func()
}

var _ = check.Suite(&requestSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *requestSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.ctx = context.WithValue(context.Background(), workshop.ContextUser, "testuser")

	s.user = &user.User{Username: "testuser", HomeDir: c.MkDir()}
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser" {
			return nil, user.UnknownUserError("not found")
		}
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.mgr = workshopstate.New(s.state, state.NewTaskRunner(s.state))
	project, _, err := s.backend.CreateOrLoadProject(s.ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(s.ctx, workshop.ContextProjectId, s.project.ProjectId)
}

func (s *requestSuite) TearDownTest(c *check.C) {
	s.restoreUserEnv()
	s.restoreUserLookup()
}

var workshopTemplate = `name: %s
base: ubuntu@20.04
sdks:
  {{- range . }}
  - name: {{ .Name}}
    channel: {{.Channel}}
  {{- end }}
`

var sdkTemplate = `name: %s
base: ubuntu@20.04
`

func (s *requestSuite) importSdkVolume(c *check.C, meta sdk.Meta) {
	vfs := c.MkDir()
	path := filepath.Join(vfs, "meta", "sdk.yaml")
	c.Assert(os.MkdirAll(filepath.Dir(path), 0755), check.IsNil)
	c.Assert(os.WriteFile(path, []byte(meta.SdkYAML), 0644), check.IsNil)

	tarball, err := os.Open(vfs)
	c.Assert(err, check.IsNil)
	defer tarball.Close()

	c.Assert(s.backend.ImportSdk(s.ctx, meta, tarball), check.IsNil)
}

func (s *requestSuite) launchWorkshopWithSDKs(c *check.C, ws string, sdks []workshop.SdkRecord) {
	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	var workshopFile = bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	path := workshop.Filepath(s.project.Path, ws)

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, workshopFile.Bytes(), 0644)
	c.Assert(err, check.IsNil)

	wf := workshop.File{Name: ws, Base: "ubuntu@20.04", Sdks: sdks}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err = s.backend.LaunchOrRebuildWorkshop(s.ctx, &wf, snapshot)
	c.Assert(err, check.IsNil)

	for _, sd := range sdks {
		sdkYaml := fmt.Sprintf(sdkTemplate, sd.Name)
		digest := sha3.Sum384([]byte(sdkYaml))
		meta := sdk.Meta{
			Setup: sdk.Setup{
				Name:      sd.Name,
				PackageID: sdk.FakePackageID(sd.Name),
				Channel:   sd.Channel,
				Revision:  sdk.R(1),
				Sha3_384:  hex.EncodeToString(digest[:]),
			},
			SdkYAML: sdkYaml,
		}
		s.importSdkVolume(c, meta)
		c.Assert(s.backend.InstallSdk(s.ctx, ws, meta.Setup), check.IsNil)
	}
}

func (s *requestSuite) TestRefreshHasUpdates(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := `name: dev
base: ubuntu@22.04
sdks:
  - name: system
    slots:
      postgres:
        interface: tunnel
        endpoint: 5432
  - name: node
    channel: latest/stable
  - name: vscode-remote
    channel: latest/edge
  - name: sketch
    plugs:
      db:
        interface: tunnel
        endpoint: 5432
    slots:
      tmpfs:
        interface: mount
        workshop-source: /mnt
connections:
  - plug: sketch:db
    slot: system:postgres
  - plug: node:npm-cache
    slot: sketch:tmpfs
  - plug: node:yarn-cache
    slot: sketch:tmpfs
`

	var oldf, newf workshop.File
	err := yaml.Unmarshal([]byte(file), &oldf)
	c.Assert(err, check.IsNil)
	err = yaml.Unmarshal([]byte(file), &newf)
	c.Assert(err, check.IsNil)

	current := []workshopstate.Manifest{{
		File:   &oldf,
		Format: sdk.R(1),
		Image:  workshop.BaseImage{Name: newf.Base, Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"},
		Sdks: []sdk.Setup{{
			Name:     "system",
			Source:   sdk.SystemSource,
			Revision: sdk.R(1),
			Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
		}, {
			Name:      "node",
			PackageID: "6ugIQcZtfu3KKD5nmXJqjzFJ69PQquju",
			Channel:   "latest/stable",
			Revision:  sdk.R(42),
			Sha3_384:  "4656e208fe96c2b29f30e2341ede0c5e1600657ee89e27ed9b382a27069804897095dc76f3d5123deac41608e70bca1d",
		}, {
			Name:      "vscode-remote",
			PackageID: "B9awcrknIhDHnDUPfbY4TVjdzqM8NEgL",
			Channel:   "latest/edge",
			Revision:  sdk.R(8),
			Sha3_384:  "5083cf36b902ced693c34a47fe0437916dd812d1e7b1b6b9685984bab5dc23acf812cc2d7f7578c322e1a2fbdcff068d",
		}, {
			Name:     "sketch",
			Source:   sdk.SketchSource,
			Revision: sdk.R(-2),
			Sha3_384: "dd4b5a4cba8539e858e5fdcc318e46d9a2940439b0d8e7bd9c6bfc8b474f410d91aee43f5d4e18cb2c1b7dbaaba06fc3",
		}},
	}}
	latest := []workshopstate.Manifest{{
		File:   &newf,
		Format: current[0].Format,
		Image:  current[0].Image,
		Sdks:   slices.Clone(current[0].Sdks),
	}}

	// No updates.
	ts, err := s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.HasLen, 0)

	// No updates but user requested --restore.
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshRestore)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)

	// Updated format.
	latest[0].Format = sdk.R(2)
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	latest[0].Format = current[0].Format

	// Updated SDK.
	latest[0].Sdks[1].Revision = sdk.R(43)
	latest[0].Sdks[1].Sha3_384 = "463f6529595798396cef8c91560f6726e02c00ea2f56e57f14dbff4a3d546a92a87618cd5bd6c148ad6308e7c612e78e"
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	latest[0].Sdks = slices.Clone(current[0].Sdks)

	// Deleted SDK.
	latest[0].Sdks = latest[0].Sdks[:3]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	latest[0].Sdks = slices.Clone(current[0].Sdks)

	// Added SDK.
	current[0].Sdks = slices.Delete(current[0].Sdks, 1, 2)
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	current[0].Sdks = slices.Clone(latest[0].Sdks)

	// Updated plug.
	plugs := latest[0].File.Sdks[3].Plugs
	db, ok := plugs["db"]
	c.Assert(ok, check.Equals, true)
	plugs["db"] = workshop.PlugOrBind{
		Plug: map[string]any{
			"interface": "tunnel",
			"endpoint":  2345,
		},
	}
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	plugs["db"] = db

	// Updated slot.
	slots := latest[0].File.Sdks[0].Slots
	postgres, ok := slots["postgres"]
	c.Assert(ok, check.Equals, true)
	slots["postgres"] = map[string]any{
		"interface": "tunnel",
		"endpoint":  2345,
	}
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	slots["postgres"] = postgres

	// Rearranged implicit SDKs.
	sdks := latest[0].File.Sdks
	sdks[0], sdks[1] = sdks[1], sdks[0]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.HasLen, 0)

	sdks[1], sdks[3] = sdks[3], sdks[1]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.HasLen, 0)
	sdks[0], sdks[1], sdks[3] = sdks[3], sdks[0], sdks[1]

	// Rearranged explicit SDKs.
	sdks[1], sdks[2] = sdks[2], sdks[1]
	latest[0].Sdks[1], latest[0].Sdks[2] = latest[0].Sdks[2], latest[0].Sdks[1]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	latest[0].Sdks[1], latest[0].Sdks[2] = latest[0].Sdks[2], latest[0].Sdks[1]
	sdks[1], sdks[2] = sdks[2], sdks[1]

	// Deleted connection.
	connections := latest[0].File.Connections
	latest[0].File.Connections = latest[0].File.Connections[:2]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	latest[0].File.Connections = connections

	// Added connection.
	connections = current[0].File.Connections
	current[0].File.Connections = current[0].File.Connections[1:]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.Not(check.HasLen), 0)
	current[0].File.Connections = connections

	// Rearranged connections.
	connections = latest[0].File.Connections
	connections[0], connections[1], connections[2] = connections[1], connections[2], connections[0]
	ts, err = s.mgr.RefreshMany(s.ctx, s.project, current, latest, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Check(ts, check.HasLen, 0)
	connections[0], connections[1], connections[2] = connections[2], connections[0], connections[1]
}

func (s *requestSuite) TestLaunchManySnapshots(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := `name: dev
base: ubuntu@22.04
sdks:
  - name: uv
    channel: latest/stable
  - name: go
    channel: latest/stable
  - name: rust
    channel: latest/stable
  - name: node
    channel: latest/stable
`

	var wf workshop.File
	err := yaml.Unmarshal([]byte(file), &wf)
	c.Assert(err, check.IsNil)

	image := workshop.BaseImage{Name: "ubuntu@22.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"}

	uv := sdk.Setup{
		Name:     "uv",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "97c599c33ff9273e280277f29b18b501c388a1eac7bac4317b89e601639b863de66b7e4b3bfc7b08e762905b31ad38e1",
	}
	golang := sdk.Setup{
		Name:     "go",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "a814c86992000179fc3dd470db43c46cbcb0f84d164b721592b5b74e03440a8c245e17242ed1c5ce9392288883ce459b",
	}
	rust := sdk.Setup{
		Name:     "rust",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "e7e13b16b131f1a8bc3ef00601beb13031d82de118bf5204757d3f5d8fa3c69431d0944ed9ee8cfd479acb0b16a93941",
	}
	rust_r2 := sdk.Setup{
		Name:     "rust",
		Channel:  "latest/stable",
		Revision: sdk.R(2),
		Sha3_384: "657ff423b8eedeace42fa55a36a1625fe9f8eea4cc4712a50d55de5e14d1d5170a6e5f214cf23cc66f295afd0ecfa0ad",
	}
	node := sdk.Setup{
		Name:     "node",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "4656e208fe96c2b29f30e2341ede0c5e1600657ee89e27ed9b382a27069804897095dc76f3d5123deac41608e70bca1d",
	}

	// Final snapshot already exists.
	manifest := workshopstate.Manifest{
		File:   &wf,
		Format: sdk.R(1),
		Image:  image,
		Sdks:   []sdk.Setup{uv, golang, rust, node},
	}
	s.backend.Snapshots = []fakebackend.FakeSnapshot{
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:1]), Id: 0},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:2]), Id: 1},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:3]), Id: 2},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:4]), Id: 3},
	}

	tasksets, err := s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	newSdks := hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.HasLen, 0)

	lastIntact := lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "node")

	// Only an intermediate snapshot exists.
	s.backend.Snapshots = s.backend.Snapshots[:2]

	tasksets, err = s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"rust", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "go")

	// No snapshots exist.
	s.backend.Snapshots = nil

	tasksets, err = s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"uv", "go", "rust", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "")

	// Snapshots only exist for outdated workshop.
	s.backend.Snapshots = []fakebackend.FakeSnapshot{
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:1]), Id: 0},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:2]), Id: 1},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:3]), Id: 2},
		{Snapshot: workshop.SdkSnapshot(manifest.Format, image, manifest.Sdks[:4]), Id: 3},
	}
	manifest.Sdks[2] = rust_r2

	tasksets, err = s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	manifest.Sdks[2] = rust

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"rust", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "go")

	// Snapshots exist for permutation of SDKs.
	wf.Sdks[1], wf.Sdks[2] = wf.Sdks[2], wf.Sdks[1]
	manifest.Sdks[1], manifest.Sdks[2] = manifest.Sdks[2], manifest.Sdks[1]

	tasksets, err = s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	manifest.Sdks[1], manifest.Sdks[2] = manifest.Sdks[2], manifest.Sdks[1]
	wf.Sdks[1], wf.Sdks[2] = wf.Sdks[2], wf.Sdks[1]

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"rust", "go", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "uv")

	// Snapshots exist for outdated base image.
	manifest.Image.Fingerprint = "fakeimage456"

	tasksets, err = s.mgr.LaunchMany(s.ctx, s.project, []workshopstate.Manifest{manifest})
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	manifest.Image.Fingerprint = "fakeimage123"

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"uv", "go", "rust", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "")
}

func (s *requestSuite) TestRefreshManySaveRestore(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	file := `name: dev
base: ubuntu@22.04
sdks:
  - name: uv
    channel: latest/stable
  - name: go
    channel: latest/stable
  - name: rust
    channel: latest/stable
  - name: node
    channel: latest/stable
`

	var wf workshop.File
	err := yaml.Unmarshal([]byte(file), &wf)
	c.Assert(err, check.IsNil)

	image := workshop.BaseImage{Name: "ubuntu@22.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"}

	uv := sdk.Setup{
		Name:     "uv",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "97c599c33ff9273e280277f29b18b501c388a1eac7bac4317b89e601639b863de66b7e4b3bfc7b08e762905b31ad38e1",
	}
	golang := sdk.Setup{
		Name:     "go",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "a814c86992000179fc3dd470db43c46cbcb0f84d164b721592b5b74e03440a8c245e17242ed1c5ce9392288883ce459b",
	}
	golang_r2 := sdk.Setup{
		Name:     "go",
		Channel:  "latest/stable",
		Revision: sdk.R(2),
		Sha3_384: "a104d9688e4b0b87d454fa1f37c037c081131cc991e0be39a096874cedbcf638aa02028ca528b5ee9d95d9be0c5df969",
	}
	rust := sdk.Setup{
		Name:     "rust",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "e7e13b16b131f1a8bc3ef00601beb13031d82de118bf5204757d3f5d8fa3c69431d0944ed9ee8cfd479acb0b16a93941",
	}
	rust_r2 := sdk.Setup{
		Name:     "rust",
		Channel:  "latest/stable",
		Revision: sdk.R(2),
		Sha3_384: "657ff423b8eedeace42fa55a36a1625fe9f8eea4cc4712a50d55de5e14d1d5170a6e5f214cf23cc66f295afd0ecfa0ad",
	}
	node := sdk.Setup{
		Name:     "node",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "4656e208fe96c2b29f30e2341ede0c5e1600657ee89e27ed9b382a27069804897095dc76f3d5123deac41608e70bca1d",
	}
	deno := sdk.Setup{
		Name:     "deno",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "9cb19574077f07a44d980e9e84bc155951f37d97fa527ae6007cb0252274d8b392523110d10101cef1f0bde11fd95dee",
	}

	// Updated SDKs.
	current := workshopstate.Manifest{
		File:   &wf,
		Format: sdk.R(1),
		Image:  image,
		Sdks:   []sdk.Setup{uv, golang, rust, node},
	}
	latest := workshopstate.Manifest{
		File:   &wf,
		Format: sdk.R(1),
		Image:  image,
		Sdks:   []sdk.Setup{uv, golang_r2, rust_r2, node},
	}
	s.backend.Snapshots = []fakebackend.FakeSnapshot{
		{Snapshot: workshop.SdkSnapshot(current.Format, image, current.Sdks[:1]), Id: 0},
		{Snapshot: workshop.SdkSnapshot(current.Format, image, current.Sdks[:2]), Id: 1},
		{Snapshot: workshop.SdkSnapshot(current.Format, image, current.Sdks[:3]), Id: 2},
		{Snapshot: workshop.SdkSnapshot(current.Format, image, current.Sdks[:4]), Id: 3},
	}

	tasksets, err := s.mgr.RefreshMany(s.ctx, s.project, []workshopstate.Manifest{current}, []workshopstate.Manifest{latest}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	saveSdks := hookSdks(c, tasksets[0], hookstate.SaveState)
	c.Check(saveSdks, check.DeepEquals, []string{"uv", "go", "rust", "node"})
	restoreSdks := hookSdks(c, tasksets[0], hookstate.RestoreState)
	c.Check(restoreSdks, check.DeepEquals, []string{"uv", "go", "rust", "node"})

	newSdks := hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"go", "rust", "node"})

	lastIntact := lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "uv")

	// Different SDKs.
	fileDeno := `name: dev
base: ubuntu@22.04
sdks:
  - name: uv
    channel: latest/stable
  - name: go
    channel: latest/stable
  - name: deno
    channel: latest/stable
`

	var wfDeno workshop.File
	err = yaml.Unmarshal([]byte(fileDeno), &wfDeno)
	c.Assert(err, check.IsNil)

	latest.File = &wfDeno
	latest.Sdks = []sdk.Setup{uv, golang, deno}

	tasksets, err = s.mgr.RefreshMany(s.ctx, s.project, []workshopstate.Manifest{current}, []workshopstate.Manifest{latest}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	saveSdks = hookSdks(c, tasksets[0], hookstate.SaveState)
	c.Check(saveSdks, check.DeepEquals, []string{"uv", "go"})
	restoreSdks = hookSdks(c, tasksets[0], hookstate.RestoreState)
	c.Check(restoreSdks, check.DeepEquals, []string{"uv", "go"})

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"deno"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "go")

	// Original SDKs.
	current.File = latest.File
	current.Sdks = latest.Sdks
	latest.File = &wf
	latest.Sdks = []sdk.Setup{uv, golang, rust, node}
	snapshot := fakebackend.FakeSnapshot{Snapshot: workshop.SdkSnapshot(current.Format, image, current.Sdks), Id: 4}
	s.backend.Snapshots = append(s.backend.Snapshots, snapshot)

	tasksets, err = s.mgr.RefreshMany(s.ctx, s.project, []workshopstate.Manifest{current}, []workshopstate.Manifest{latest}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	saveSdks = hookSdks(c, tasksets[0], hookstate.SaveState)
	c.Check(saveSdks, check.DeepEquals, []string{"uv", "go"})
	restoreSdks = hookSdks(c, tasksets[0], hookstate.RestoreState)
	c.Check(restoreSdks, check.DeepEquals, []string{"uv", "go"})

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.HasLen, 0)

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "node")

	// Rearranged SDKs.
	current.File = latest.File
	current.Sdks = latest.Sdks
	latest.File.Sdks = slices.Clone(latest.File.Sdks)
	latest.File.Sdks[1], latest.File.Sdks[2] = latest.File.Sdks[2], latest.File.Sdks[1]
	latest.Sdks = slices.Clone(latest.Sdks)
	latest.Sdks[1], latest.Sdks[2] = latest.Sdks[2], latest.Sdks[1]

	tasksets, err = s.mgr.RefreshMany(s.ctx, s.project, []workshopstate.Manifest{current}, []workshopstate.Manifest{latest}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(tasksets, check.HasLen, 1)

	saveSdks = hookSdks(c, tasksets[0], hookstate.SaveState)
	c.Check(saveSdks, check.DeepEquals, []string{"uv", "go", "rust", "node"})
	restoreSdks = hookSdks(c, tasksets[0], hookstate.RestoreState)
	c.Check(restoreSdks, check.DeepEquals, []string{"uv", "rust", "go", "node"})

	newSdks = hookSdks(c, tasksets[0], hookstate.SetupBase)
	c.Check(newSdks, check.DeepEquals, []string{"rust", "go", "node"})

	lastIntact = lastIntactSdk(c, tasksets[0])
	c.Check(lastIntact, check.Equals, "uv")
}

// List SDKs for which the given type of hook is part of the TaskSet.
func hookSdks(c *check.C, tasks *state.TaskSet, hookType hookstate.WorkshopHookType) []string {
	var sdks []string
	for _, task := range tasks.Tasks() {
		if task.Kind() != "run-hook" {
			continue
		}

		var hook hookstate.HookSetup
		c.Assert(task.Get("hook-setup", &hook), check.IsNil)
		if hook.HookType == hookType {
			sdks = append(sdks, hook.Sdk)
		}
	}
	return sdks
}

func lastIntactSdk(c *check.C, taskset *state.TaskSet) string {
	tasks := taskset.Tasks()
	idx := slices.IndexFunc(tasks, func(t *state.Task) bool {
		return t.Kind() == "create-workshop" || t.Kind() == "rebuild-workshop"
	})
	c.Assert(idx, testutil.IntGreaterEqual, 0)

	s, err := handlersetup.MaybeLastIntactSdk(tasks[idx])
	c.Assert(err, check.IsNil)
	return s
}

func (s *requestSuite) TestStartMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts := workshopstate.StartManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "start-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "start-workshop")
}

// TestStartManyWaitingReturnsChangeConflict checks that start returns a typed
// [healthstate.ChangeInProgressError], rather than a generic health error, when
// a workshop is waiting on an errored change.
func (s *requestSuite) TestStartManyWaitingReturnsChangeConflict(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "ws", nil)
	c.Assert(s.backend.StopWorkshop(s.ctx, "ws", true), check.IsNil)
	change := s.state.NewChange("refresh", "refresh ws")
	change.Set("project-id", s.project.ProjectId)
	change.SetStatus(state.WaitStatus)
	task := s.state.NewTask("run-hook", "Run refresh hook")
	task.Set("workshop", "ws")
	task.Set("project", s.project)
	change.AddTask(task)

	_, err := s.mgr.StartMany(s.ctx, []string{"ws"}, s.project.ProjectId)
	var conflictErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, healthstate.ChangeInProgressError{
		ChangeID:   change.ID(),
		ChangeKind: "refresh",
		ProjectID:  s.project.ProjectId,
		Workshop:   "ws",
	})
}

func (s *requestSuite) TestStopMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts := workshopstate.StopManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "stop-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "stop-workshop")

	var force bool
	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)

	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)
}

// TestStopManyWaitingReturnsChangeConflict checks that stop returns a typed
// [conflict.ChangeConflictError], rather than a generic health error, when a
// workshop is waiting on an errored change.
func (s *requestSuite) TestStopManyWaitingReturnsChangeConflict(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "ws", nil)
	change := s.state.NewChange("refresh", "refresh ws")
	change.Set("project-id", s.project.ProjectId)
	change.SetStatus(state.WaitStatus)
	task := s.state.NewTask("run-hook", "Run refresh hook")
	task.Set("workshop", "ws")
	task.Set("project", s.project)
	change.AddTask(task)

	_, err := s.mgr.StopMany(s.ctx, []string{"ws"}, s.project.ProjectId)
	var conflictErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &conflictErr), check.Equals, true)
	c.Check(conflictErr, check.DeepEquals, healthstate.ChangeInProgressError{
		ChangeID:   change.ID(),
		ChangeKind: "refresh",
		ProjectID:  s.project.ProjectId,
		Workshop:   "ws",
	})
}

func (s *requestSuite) TestRemountSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	sdks := []workshop.SdkRecord{
		{Name: "sdk-1", Channel: "latest/stable"},
	}
	source := c.MkDir()

	s.launchWorkshopWithSDKs(c, "ws-1", sdks)

	ts, err := s.mgr.Remount(s.ctx, s.state, plug, source)
	c.Assert(err, check.IsNil)
	c.Assert(ts.Tasks(), check.HasLen, 1)

	var w string
	var p workshop.Project
	task := ts.Tasks()[0]
	c.Assert(task.Get("workshop", &w), check.IsNil)
	c.Assert(task.Get("project", &p), check.IsNil)
	c.Assert(task.Summary(), check.Equals, `Remount "ws-1/sdk-1:plug"`)
	c.Assert(w, check.Equals, "ws-1")
	c.Assert(p, check.DeepEquals, s.project)

	var plugRef sdk.PlugRef
	var src string
	c.Assert(task.Get("plug", &plugRef), check.IsNil)
	c.Assert(plugRef, check.DeepEquals, plug)
	c.Assert(task.Get("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, source)
}

func (s *requestSuite) TestRemountWorkshopNotReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	sdks := []workshop.SdkRecord{
		{Name: "sdk-1", Channel: "latest/stable"},
	}

	s.launchWorkshopWithSDKs(c, "ws-1", sdks)

	// pretend there is another change running that would conflict with this one.
	change := s.state.NewChange("refresh", "test")
	task := s.state.NewTask("task", "test")
	task.Set("workshop", "ws-1")
	change.AddTask(task)
	change.Set("project-id", s.project.ProjectId)

	_, err := s.mgr.Remount(s.ctx, s.state, plug, c.MkDir())
	c.Assert(err, check.ErrorMatches, `cannot remount "ws-1/sdk-1:plug": workshop "ws-1" has "refresh" change in progress`)
}
