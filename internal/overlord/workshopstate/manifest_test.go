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
	"context"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/sdkstore/transport"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type manifestSuite struct {
	state   *state.State
	backend *fakebackend.FakeWorkshopBackend
	store   *sdk.FakeStore
	runner  *state.TaskRunner
	manager *workshopstate.WorkshopManager
	ctx     context.Context
	project workshop.Project
	user    *user.User

	restoreUserEnv    func()
	restoreUserLookup func()
}

var _ = check.Suite(&manifestSuite{})

func (s *manifestSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.runner = state.NewTaskRunner(s.state)
	s.manager = workshopstate.New(s.state, s.runner)
	ctx := context.WithValue(context.TODO(), workshop.ContextUser, "testuser")

	// Real UID and GID are required to copy local SDKs and create the apt cache directory.
	// TODO: make filesystem operations more secure (e.g. drop privileges if possible) and easy to test.
	actual, err := user.Current()
	c.Assert(err, check.IsNil)
	s.user = &user.User{Username: "testuser", HomeDir: c.MkDir(), Uid: actual.Uid, Gid: actual.Gid}
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})

	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(ctx, workshop.ContextProjectId, s.project.ProjectId)

	s.store = sdk.NewFakeStore()
	s.store.SetResolveCallback(sdk.FakeResolve(storeSdks))
	sdk.ReplaceStore(s.state, s.store)
}

func (s *manifestSuite) TearDownTest(c *check.C) {
	s.restoreUserEnv()
	s.restoreUserLookup()
}

var storeSdks = map[string]sdk.Meta{
	"test": {
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: "name: test\n",
	},
	"noble": {
		Setup: sdk.Setup{
			Name:      "noble",
			PackageID: "WMwWl1i0hX4PuT4nXdZrjecHnJuBfJ2R",
			Revision:  sdk.R(1),
			Sha3_384:  "199a8c0bcf6f2348a795b7bc1d24595530a189b85c682c28b23d037365fb013e3dd71057ee21c99822b745c20349abd9",
		},
		SdkYAML: "name: noble\nbase: ubuntu@24.04\n",
	},
	"rust": {
		Setup: sdk.Setup{
			Name:      "rust",
			PackageID: "CJgdEGpmNHMx6dALcAexfIfai9fpjX9K",
			Revision:  sdk.R(1),
			Sha3_384:  "e7e13b16b131f1a8bc3ef00601beb13031d82de118bf5204757d3f5d8fa3c69431d0944ed9ee8cfd479acb0b16a93941",
		},
		SdkYAML: "name: rust\n",
	},
	"uv": {
		Setup: sdk.Setup{
			Name:      "uv",
			PackageID: "1yAowR5Y3zFp1oH2Rh4uqClLJUzasRNl",
			Revision:  sdk.R(1),
			Sha3_384:  "97c599c33ff9273e280277f29b18b501c388a1eac7bac4317b89e601639b863de66b7e4b3bfc7b08e762905b31ad38e1",
		},
		SdkYAML: "name: uv\n",
	},
	"node": {
		Setup: sdk.Setup{
			Name:      "node",
			PackageID: "6ugIQcZtfu3KKD5nmXJqjzFJ69PQquju",
			Revision:  sdk.R(1),
			Sha3_384:  "4656e208fe96c2b29f30e2341ede0c5e1600657ee89e27ed9b382a27069804897095dc76f3d5123deac41608e70bca1d",
		},
		SdkYAML: "name: node\n",
	},
}

func mockSdk(metadir, hooksdir string, meta string) error {
	if err := os.MkdirAll(metadir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(hooksdir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(meta), 0644)
}

func (s *manifestSuite) mockStoreSdk(c *check.C, meta sdk.Meta) {
	sdkdir := c.MkDir()
	metadir := filepath.Join(sdkdir, "meta")
	hooksdir := filepath.Join(sdkdir, "sdk", "hooks")
	c.Assert(mockSdk(metadir, hooksdir, meta.SdkYAML), check.IsNil)

	tarball, err := os.Open(sdkdir)
	c.Assert(err, check.IsNil)
	err = s.backend.ImportSdk(s.ctx, meta, tarball)
	tarball.Close()
	c.Assert(err, check.IsNil)
}

func (s *manifestSuite) mockTrySdk(c *check.C, name, filename string, meta string) string {
	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sdkdir := filepath.Join(workshop.TrySdkDir(userDataDir, name), filename)
	metadir := filepath.Join(sdkdir, "meta")
	hooksdir := filepath.Join(sdkdir, "sdk", "hooks")
	c.Assert(mockSdk(metadir, hooksdir, meta), check.IsNil)

	c.Assert(os.WriteFile(sdkdir+".yaml", []byte(meta), 0666), check.IsNil)

	digest := sha3.Sum384([]byte(meta))
	hexdigest := hex.AppendEncode(nil, digest[:])
	c.Assert(os.WriteFile(sdkdir+".sha3-384", hexdigest, 0666), check.IsNil)
	return string(hexdigest)
}

func (s *manifestSuite) mockProjectSdk(c *check.C, name string, meta string) {
	sdkdir := workshop.ProjectSdkPath(s.project.Path, name)
	hooksdir := filepath.Join(sdkdir, "hooks")
	c.Assert(mockSdk(sdkdir, hooksdir, meta), check.IsNil)
}

func (s *manifestSuite) mockSketchSdk(c *check.C, w string, meta string) {
	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sdkdir := workshop.SketchSdkCurrent(userDataDir, s.project.ProjectId, w)
	hooksdir := filepath.Join(sdkdir, "hooks")
	c.Assert(mockSdk(sdkdir, hooksdir, meta), check.IsNil)
}

func (s *manifestSuite) createWFile(c *check.C, ws, base string, sdks []workshop.SdkRecord) *workshop.File {
	wf := &workshop.File{
		Name: ws,
		Base: base,
		Sdks: sdks,
	}

	fileBlob, err := yaml.Marshal(wf)
	c.Assert(err, check.IsNil)

	path := workshop.Filepath(s.project.Path, ws)
	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, fileBlob, 0644)
	c.Assert(err, check.IsNil)

	return wf
}

func (s *manifestSuite) launchWorkshopWithSDKs(c *check.C, ws, base string, sdks []workshop.SdkRecord) *workshop.Workshop {
	wf := s.createWFile(c, ws, base, sdks)
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err := s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	workshop, err := s.backend.Workshop(s.ctx, ws)
	c.Assert(err, check.IsNil)
	workshop.Running = true
	return workshop
}

func (s *manifestSuite) TestLaunchOK(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.createWFile(c, "test-1", "ubuntu@20.04", nil)
	sdks := []workshop.SdkRecord{{Name: "test", Channel: "latest/edge"}}
	s.createWFile(c, "test-2", "ubuntu@20.04", sdks)

	manifests, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2"})
	c.Assert(err, check.IsNil)

	c.Assert(manifests, check.HasLen, 2)

	c.Check(manifests[0].File, check.DeepEquals, &workshop.File{
		Name: "test-1",
		Base: "ubuntu@20.04",
	})
	c.Check(manifests[1].File, check.DeepEquals, &workshop.File{
		Name: "test-2",
		Base: "ubuntu@20.04",
		Sdks: sdks,
	})

	c.Check(manifests[0].Image, check.Equals, workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	c.Check(manifests[1].Image, check.Equals, manifests[0].Image)

	systemSdk, err := system.SystemSdkMeta()
	c.Assert(err, check.IsNil)
	testSdk := storeSdks["test"]
	testSdk.Channel = "latest/edge"
	c.Check(manifests[0].Sdks, check.DeepEquals, []sdk.Setup{systemSdk.Setup})
	c.Check(manifests[1].Sdks, check.DeepEquals, []sdk.Setup{systemSdk.Setup, testSdk.Setup})

	c.Assert(s.store.ResolveCalls, check.HasLen, 1)
	c.Assert(s.store.ResolveCalls[0].Packages, check.HasLen, 4)
	c.Check(s.store.ResolveCalls[0].Packages[0].Name, check.Equals, "test")
}

func (s *manifestSuite) TestRefreshOK(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}}
	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", sdks)
	s.launchWorkshopWithSDKs(c, "test-2", "ubuntu@20.04", sdks)

	// Install try SDK in test-1 workshop.
	oldSdk := storeSdks["test"]
	oldSdk.Channel = ""
	oldSdk.Source = sdk.TrySource
	oldSdk.Revision = sdk.R(-1)
	s.mockStoreSdk(c, oldSdk)
	err := s.backend.InstallSdk(s.ctx, "test-1", oldSdk.Setup)
	c.Assert(err, check.IsNil)

	// Update base for test-2 workshop.
	fileBlob, err := os.ReadFile(workshop.Filepath(s.project.Path, "test-2"))
	c.Assert(err, check.IsNil)
	fileText := strings.Replace(string(fileBlob), "ubuntu@20.04", "ubuntu@22.04", 1)
	err = os.WriteFile(workshop.Filepath(s.project.Path, "test-2"), []byte(fileText), 0644)
	c.Assert(err, check.IsNil)

	// Update format revision number
	defer s.backend.SetFormatRevision(sdk.R(2))()

	// Retrieve manifests.
	current, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1", "test-2"}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)

	c.Assert(current, check.HasLen, 2)
	c.Assert(latest, check.HasLen, 2)

	// Sanity check for test-1.
	c.Check(current[0].File, check.DeepEquals, &workshop.File{
		Name: "test-1",
		Base: "ubuntu@20.04",
		Sdks: sdks,
	})
	c.Check(latest[0].File, check.DeepEquals, current[0].File)
	c.Check(current[0].Format, check.Equals, sdk.R(1))
	c.Check(latest[0].Format, check.Equals, sdk.R(2))
	c.Check(current[0].Image, check.Equals, workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	c.Check(latest[0].Image, check.Equals, current[0].Image)

	// Check base was updated for test-2.
	c.Check(current[1].File, check.DeepEquals, &workshop.File{
		Name: "test-2",
		Base: "ubuntu@20.04",
		Sdks: sdks,
	})
	c.Check(latest[1].File, check.DeepEquals, &workshop.File{
		Name: "test-2",
		Base: "ubuntu@22.04",
		Sdks: sdks,
	})
	c.Check(current[1].Format, check.Equals, sdk.R(1))
	c.Check(latest[1].Format, check.Equals, sdk.R(2))
	c.Check(current[1].Image, check.Equals, current[0].Image)
	c.Check(latest[1].Image, check.Equals, workshop.BaseImage{Name: "ubuntu@22.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})

	// Check current SDKs are loaded from running workshop.
	c.Check(current[0].Sdks, check.DeepEquals, []sdk.Setup{oldSdk.Setup})
	c.Check(current[1].Sdks, check.HasLen, 0)

	// Check new SDKs are loaded from the Store.
	systemSdk, err := system.SystemSdkMeta()
	c.Assert(err, check.IsNil)
	newSdk := storeSdks["test"]
	newSdk.Channel = "latest/stable"
	c.Check(latest[0].Sdks, check.DeepEquals, []sdk.Setup{systemSdk.Setup, newSdk.Setup})
	c.Check(latest[1].Sdks, check.DeepEquals, []sdk.Setup{systemSdk.Setup, newSdk.Setup})
}

func (s *manifestSuite) TestRefreshRestoreOK(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "test", Channel: "latest/stable"}}
	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", sdks)

	// Install try SDK in test-1 workshop.
	oldSdk := storeSdks["test"]
	oldSdk.Channel = ""
	oldSdk.Source = sdk.TrySource
	oldSdk.Revision = sdk.R(-1)
	s.mockStoreSdk(c, oldSdk)
	err := s.backend.InstallSdk(s.ctx, "test-1", oldSdk.Setup)
	c.Assert(err, check.IsNil)

	// Update base for test-1 workshop.
	fileBlob, err := os.ReadFile(workshop.Filepath(s.project.Path, "test-1"))
	c.Assert(err, check.IsNil)
	fileText := strings.Replace(string(fileBlob), "ubuntu@20.04", "ubuntu@22.04", 1)
	err = os.WriteFile(workshop.Filepath(s.project.Path, "test-1"), []byte(fileText), 0644)
	c.Assert(err, check.IsNil)

	// Retrieve manifests.
	current, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshRestore)
	c.Assert(err, check.IsNil)

	c.Assert(current, check.HasLen, 1)
	c.Assert(latest, check.HasLen, 1)
	c.Assert(current, check.DeepEquals, latest)

	c.Check(current[0].File, check.DeepEquals, &workshop.File{
		Name: "test-1",
		Base: "ubuntu@20.04",
		Sdks: sdks,
	})
	c.Check(current[0].Image, check.Equals, workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	c.Check(current[0].Sdks, check.DeepEquals, []sdk.Setup{oldSdk.Setup})
}

func (s *manifestSuite) TestLaunchRequiresStatusOff(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.createWFile(c, "test-1", "ubuntu@20.04", nil)
	s.launchWorkshopWithSDKs(c, "test-2", "ubuntu@20.04", nil)
	s.createWFile(c, "test-3", "ubuntu@20.04", nil)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-2": workshop exists`)
}

func (s *manifestSuite) TestLaunchAvoidsConflicts(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.createWFile(c, "test-1", "ubuntu@20.04", nil)
	s.createWFile(c, "test-2", "ubuntu@20.04", nil)
	s.createWFile(c, "test-3", "ubuntu@20.04", nil)

	chg := s.state.NewChange("launch", "...")
	chg.Set("project-id", s.project.ProjectId)
	task := s.state.NewTask("create-workshop", "...")
	setWorkshopProject("test-2", s.project, task)
	chg.AddTask(task)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-2": other changes in progress: workshop "test-2" has "launch" change in progress`)
}

func (s *manifestSuite) TestRefreshRequiresWorkshopExistence(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)
	s.createWFile(c, "test-2", "ubuntu@20.04", nil)
	s.launchWorkshopWithSDKs(c, "test-3", "ubuntu@20.04", nil)

	_, _, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"}, conflict.RefreshUpdate)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": workshop not launched`)

	_, _, err = s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"}, conflict.RefreshRestore)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": workshop not launched`)
}

func (s *manifestSuite) TestRefreshRequiresStatusReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)
	s.launchWorkshopWithSDKs(c, "test-2", "ubuntu@20.04", nil)
	err := s.backend.StopWorkshop(s.ctx, "test-2", false)
	c.Assert(err, check.IsNil)
	s.launchWorkshopWithSDKs(c, "test-3", "ubuntu@20.04", nil)

	_, _, err = s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"}, conflict.RefreshUpdate)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": not running`)

	_, _, err = s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1", "test-2", "test-3"}, conflict.RefreshRestore)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-2": not running`)
}

func (s *manifestSuite) TestRestoreRequiresCurrentFormat(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)

	defer s.backend.SetFormatRevision(sdk.R(2))()

	_, _, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshRestore)
	c.Assert(err, check.ErrorMatches, `cannot refresh "test-1": workshop created using incompatible Workshop version`)
}

// TestRefreshManifestsWaitingReturnsChangeConflict checks that refresh returns a
// typed [healthstate.ChangeInProgressError], rather than a generic health
// error, when a workshop is waiting on an errored change.
func (s *manifestSuite) TestRefreshManifestsWaitingReturnsChangeConflict(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)
	change := s.state.NewChange("refresh", "refresh test-1")
	change.Set("project-id", s.project.ProjectId)
	change.SetStatus(state.WaitStatus)
	task := s.state.NewTask("run-hook", "Run refresh hook")
	task.Set("workshop", "test-1")
	task.Set("project", s.project)
	change.AddTask(task)

	expected := healthstate.ChangeInProgressError{
		ChangeID:   change.ID(),
		ChangeKind: "refresh",
		ProjectID:  s.project.ProjectId,
		Workshop:   "test-1",
	}

	_, _, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
	var updateErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &updateErr), check.Equals, true)
	c.Check(updateErr, check.DeepEquals, expected)

	_, _, err = s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshRestore)
	var restoreErr healthstate.ChangeInProgressError
	c.Assert(errors.As(err, &restoreErr), check.Equals, true)
	c.Check(restoreErr, check.DeepEquals, expected)
}

func (s *manifestSuite) TestLaunchRequiresFile(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"nonexistent"})
	path := workshop.Filepath(s.project.Path, "nonexistent")
	message := fmt.Sprintf(`cannot launch "nonexistent": workshop definition %q not found`, path)
	c.Assert(err, check.ErrorMatches, message)
}

func (s *manifestSuite) TestLaunchRequiresBase(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)

	restoreBase := testutil.FakeFunc(func(ctx context.Context, base string, confinement workshop.Confinement) (workshop.BaseImage, error) {
		return workshop.BaseImage{}, errors.New("contrived error")
	}, &s.backend.GetBaseCallback)
	defer restoreBase()

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": contrived error`)
}

func (s *manifestSuite) TestLaunchMissingStoreSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "nonexistent", Channel: "latest/edge"}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": "nonexistent" SDK: Package not found`)
}

func (s *manifestSuite) TestLaunchMissingStoreSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{
		{Name: "nonexistent1", Channel: "latest/edge"},
		{Name: "nonexistent2", Channel: "latest/beta"},
	}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": multiple SDK Store errors:
- "nonexistent1" SDK: Package not found
- "nonexistent2" SDK: Package not found`)
}

func (s *manifestSuite) TestLaunchValidRequest(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	architecture := arch.ArchitectureType(arch.DpkgArchitecture())
	arch.SetArchitecture("mock64")
	defer arch.SetArchitecture(architecture)

	sdks := []workshop.SdkRecord{
		{Name: "test"},
		{Name: "node", Channel: "latest/edge"},
	}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	manifests, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)

	c.Assert(manifests, check.HasLen, 1)

	c.Check(manifests[0].File, check.DeepEquals, &workshop.File{
		Name: "test-1",
		Base: "ubuntu@20.04",
		Sdks: sdks,
	})
	c.Check(manifests[0].Image, check.Equals, workshop.BaseImage{Name: "ubuntu@20.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})

	systemSdk, err := system.SystemSdkMeta()
	c.Assert(err, check.IsNil)
	testSdk := storeSdks["test"]
	testSdk.Channel = "latest/stable"
	nodeSdk := storeSdks["node"]
	nodeSdk.Channel = "latest/edge"
	c.Check(manifests[0].Sdks, check.DeepEquals, []sdk.Setup{systemSdk.Setup, testSdk.Setup, nodeSdk.Setup})

	c.Assert(s.store.ResolveCalls, check.HasLen, 1)
	c.Check(s.store.ResolveCalls[0].Crafts, check.HasLen, 0)

	c.Assert(s.store.ResolveCalls[0].Packages, check.HasLen, 8)
	packages := []transport.ResolvePackage{{
		InstanceKey: s.store.ResolveCalls[0].Packages[0].InstanceKey,
		Namespace:   "sdk",
		Name:        "test",
		Channel:     "stable",
		Platform: transport.Platform{
			Name:         "ubuntu",
			Channel:      "20.04",
			Architecture: "mock64",
		},
	}, {
		InstanceKey: s.store.ResolveCalls[0].Packages[1].InstanceKey,
		Namespace:   "sdk",
		Name:        "node",
		Channel:     "latest/edge",
		Platform: transport.Platform{
			Name:         "ubuntu",
			Channel:      "20.04",
			Architecture: "mock64",
		},
	}}
	key, found := strings.CutSuffix(packages[0].InstanceKey, "_0")
	c.Assert(found, check.Equals, true)
	_, err = uuid.Parse(key)
	c.Assert(err, check.IsNil)
	key, found = strings.CutSuffix(packages[1].InstanceKey, "_0")
	c.Assert(found, check.Equals, true)
	_, err = uuid.Parse(key)
	c.Assert(err, check.IsNil)
	c.Check(s.store.ResolveCalls[0].Packages[:2], check.DeepEquals, packages)
}

func (s *manifestSuite) TestLaunchMissingTrySdkDir(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "nonexistent", Source: sdk.TrySource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": "try-nonexistent" SDK not found: open .*/try/nonexistent: no such file or directory`)
}

func (s *manifestSuite) TestLaunchFindTrySdkFile(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	architecture := arch.ArchitectureType(arch.DpkgArchitecture())
	arch.SetArchitecture("mock64")
	defer arch.SetArchitecture(architecture)

	all := `name: test
`
	focal := `name: test
architecture: all
base: ubuntu@20.04
`
	noble := `name: test
base: ubuntu@24.04
`
	mock16 := `name: test
architecture: mock16
`
	focal16 := `name: test
architecture: mock16
base: ubuntu@20.04
`
	mock64 := `name: test
architecture: mock64
`
	focal64 := `name: test
architecture: mock64
base: ubuntu@20.04
`
	noble64 := `name: test
architecture: mock64
base: ubuntu@24.04
`

	allDigest := s.mockTrySdk(c, "test", "test_all.sdk", all)
	focalDigest := s.mockTrySdk(c, "test", "test_all_ubuntu@20.04.sdk", focal)
	s.mockTrySdk(c, "test", "test_all_ubuntu@24.04.sdk", noble)
	s.mockTrySdk(c, "test", "test_mock16.sdk", mock16)
	s.mockTrySdk(c, "test", "test_mock16_ubuntu@20.04.sdk", focal16)
	mock64Digest := s.mockTrySdk(c, "test", "test_mock64.sdk", mock64)
	focal64Digest := s.mockTrySdk(c, "test", "test_mock64_ubuntu@20.04.sdk", focal64)
	s.mockTrySdk(c, "test", "test_mock64_ubuntu@24.04.sdk", noble64)

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.TrySource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	// Check arch- and base-specific SDK is picked first.
	manifests, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1].Sha3_384, check.Equals, focal64Digest)

	// Check base-specific SDK is picked if there isn't a closer match.
	arch.SetArchitecture("mock32")

	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1].Sha3_384, check.Equals, focalDigest)

	// Check generic SDK is picked if there isn't a closer match.
	s.createWFile(c, "test-1", "ubuntu@22.04", sdks)

	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1].Sha3_384, check.Equals, allDigest)

	// Check arch-specific SDK is picked if there isn't a closer match.
	arch.SetArchitecture("mock64")

	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1].Sha3_384, check.Equals, mock64Digest)

	// Check arch-specific SDK is picked ahead of base-specific SDK.
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)
	trySdkDir := workshop.TrySdkDir(workshop.UserDataRootDir(s.user.HomeDir, nil), "test")
	err = os.RemoveAll(filepath.Join(trySdkDir, "test_mock64_ubuntu@20.04.sdk"))
	c.Assert(err, check.IsNil)

	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1].Sha3_384, check.Equals, mock64Digest)

	// Check case where no SDK matches.
	err = os.RemoveAll(filepath.Join(trySdkDir, "test_mock64.sdk"))
	c.Assert(err, check.IsNil)
	err = os.RemoveAll(filepath.Join(trySdkDir, "test_all_ubuntu@20.04.sdk"))
	c.Assert(err, check.IsNil)
	err = os.RemoveAll(filepath.Join(trySdkDir, "test_all.sdk"))
	c.Assert(err, check.IsNil)

	_, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": "try-test" SDK not found: openat .*/test_mock64_ubuntu@20\.04\.sdk: no such file or directory`)
}

func (s *manifestSuite) TestLaunchMissingTryMetadata(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.mockTrySdk(c, "test", "test_all.sdk", "name: test\n")
	s.mockTrySdk(c, "test2", "test2_all.sdk", "name: test\n")

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	trySdkYaml := filepath.Join(workshop.TrySdkDir(userDataDir, "test"), "test_all.sdk.yaml")
	err := os.Remove(trySdkYaml)
	c.Assert(err, check.IsNil)
	trySdkDigest := filepath.Join(workshop.TrySdkDir(userDataDir, "test2"), "test2_all.sdk.sha3-384")
	err = os.Remove(trySdkDigest)
	c.Assert(err, check.IsNil)

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.TrySource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)
	sdks2 := []workshop.SdkRecord{{Name: "test2", Source: sdk.TrySource}}
	s.createWFile(c, "test-2", "ubuntu@20.04", sdks2)

	_, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": invalid "try-test" SDK: openat .*/test_all\.sdk\.yaml: no such file or directory`)

	_, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-2"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-2": invalid "try-test2" SDK: openat .*/test2_all\.sdk\.sha3-384: no such file or directory`)
}

func (s *manifestSuite) TestLaunchValidatesTrySdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.mockTrySdk(c, "test", "test_all_ubuntu@20.04.sdk", `name: foo
base: ubuntu@20.04
`)

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.TrySource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": SDK must be named "test" \(now: "foo"\)`)
}

func (s *manifestSuite) TestLaunchImportsTrySdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	volume := workshop.SdkVolume{
		Meta: sdk.Meta{
			Setup: sdk.Setup{
				Name:     "test",
				Source:   sdk.TrySource,
				Revision: sdk.R(-1),
			},
			SdkYAML: "name: test\n",
		},
		Workshops: map[string][]string{},
	}
	volume.Sha3_384 = s.mockTrySdk(c, volume.Name, "test_all.sdk", volume.SdkYAML)

	volume2 := workshop.SdkVolume{
		Meta: sdk.Meta{
			Setup: sdk.Setup{
				Name:     "test2",
				Source:   sdk.TrySource,
				Revision: sdk.R(-1),
			},
			SdkYAML: "name: test2\n",
		},
		Workshops: map[string][]string{},
	}
	volume2.Sha3_384 = s.mockTrySdk(c, volume2.Name, "test2_all.sdk", volume2.SdkYAML)

	sdks := []workshop.SdkRecord{
		{Name: "test", Source: sdk.TrySource},
		{Name: "test2", Source: sdk.TrySource},
	}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks[:1])
	s.createWFile(c, "test-2", "ubuntu@20.04", sdks)

	// Check launch creates both volumes at revision x1.
	manifests, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1:], check.DeepEquals, []sdk.Setup{volume.Setup})
	c.Check(manifests[1].Sdks[1:], check.DeepEquals, []sdk.Setup{volume.Setup, volume2.Setup})
	volumes, err := s.backend.Sdks(s.ctx)
	c.Assert(err, check.IsNil)
	c.Check(volumes, testutil.DeepUnsortedMatches, []workshop.SdkVolume{volume, volume2})

	// Update test SDK.
	volume_r2 := volume
	volume_r2.Revision = sdk.R(-2)
	volume_r2.SdkYAML = "name: test\nplugs: null\n"
	volume_r2.Sha3_384 = s.mockTrySdk(c, volume_r2.Name, "test_all.sdk", volume_r2.SdkYAML)

	// Check launch creates a test SDK volume at revision x2.
	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1:], check.DeepEquals, []sdk.Setup{volume_r2.Setup})
	c.Check(manifests[1].Sdks[1:], check.DeepEquals, []sdk.Setup{volume_r2.Setup, volume2.Setup})
	volumes, err = s.backend.Sdks(s.ctx)
	c.Assert(err, check.IsNil)
	c.Check(volumes, testutil.DeepUnsortedMatches, []workshop.SdkVolume{volume, volume_r2, volume2})

	// Update test2 SDK.
	volume2_r2 := volume2
	volume2_r2.Revision = sdk.R(-2)
	volume2_r2.SdkYAML = "name: test2\nplugs: null\n"
	volume2_r2.Sha3_384 = s.mockTrySdk(c, volume2_r2.Name, "test2_all.sdk", volume2_r2.SdkYAML)

	// Check launch creates a test2 SDK volume at revision x2.
	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1:], check.DeepEquals, []sdk.Setup{volume_r2.Setup})
	c.Check(manifests[1].Sdks[1:], check.DeepEquals, []sdk.Setup{volume_r2.Setup, volume2_r2.Setup})
	volumes, err = s.backend.Sdks(s.ctx)
	c.Assert(err, check.IsNil)
	c.Check(volumes, testutil.DeepUnsortedMatches, []workshop.SdkVolume{volume, volume_r2, volume2, volume2_r2})

	// Revert test SDK.
	s.mockTrySdk(c, volume.Name, "test_all.sdk", volume.SdkYAML)

	// Check launch reuses the initial test SDK volume.
	manifests, err = s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1", "test-2"})
	c.Assert(err, check.IsNil)
	c.Assert(manifests, check.HasLen, 2)
	c.Check(manifests[0].Sdks[1:], check.DeepEquals, []sdk.Setup{volume.Setup})
	c.Check(manifests[1].Sdks[1:], check.DeepEquals, []sdk.Setup{volume.Setup, volume2_r2.Setup})
	volumes, err = s.backend.Sdks(s.ctx)
	c.Assert(err, check.IsNil)
	c.Check(volumes, testutil.DeepUnsortedMatches, []workshop.SdkVolume{volume, volume_r2, volume2, volume2_r2})
}

func (s *manifestSuite) TestLaunchMissingProjectSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "nonexistent", Source: sdk.ProjectSource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": stat .*/\.workshop/nonexistent: no such file or directory`)
}

func (s *manifestSuite) TestRefreshKeepsInstalledProjectSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.ProjectSource}}
	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", sdks)

	sdkYamls := []string{
		"name: test\n",
		"name: test\nplugs: null\n",
		"name: test\nslots: null\n",
	}
	var setups []sdk.Setup

	// Create 3 revisions.
	for i, sdkYaml := range sdkYamls {
		s.mockProjectSdk(c, "test", sdkYaml)

		_, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
		c.Assert(err, check.IsNil)

		c.Assert(latest, check.HasLen, 1)
		c.Assert(latest[0].Sdks, check.HasLen, 2)
		c.Assert(latest[0].Sdks[0].Name, check.Equals, "system")

		setup := latest[0].Sdks[1]
		c.Assert(setup.Name, check.Equals, "test")
		c.Check(setup.Source, check.Equals, sdk.ProjectSource)
		c.Check(setup.Revision, check.Equals, sdk.R(-(i + 1)))
		setups = append(setups, setup)
	}

	// Install revision x1.
	err := s.backend.InstallSdk(s.ctx, "test-1", setups[0])
	c.Assert(err, check.IsNil)

	// Create revision x4.
	s.mockProjectSdk(c, "test", "name: test\nplugs: null\nslots: null\n")

	current, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)

	c.Assert(current, check.HasLen, 1)
	c.Assert(current[0].Sdks, check.HasLen, 1)
	c.Check(current[0].Sdks[0], check.Equals, setups[0])

	c.Assert(latest, check.HasLen, 1)
	c.Assert(latest[0].Sdks, check.HasLen, 2)
	c.Assert(latest[0].Sdks[0].Name, check.Equals, "system")

	setup := latest[0].Sdks[1]
	c.Assert(setup.Name, check.Equals, "test")
	c.Check(setup.Source, check.Equals, sdk.ProjectSource)
	c.Check(setup.Revision, check.Equals, sdk.R(-4))

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sdkdir := workshop.LocalSdkDir(userDataDir, s.project.ProjectId, "test-1", "test")
	c.Check(sdkdir, testutil.DirEquals, []string{
		"drwxr-xr-x " + setups[0].Sha3_384,
		"drwxr-xr-x " + setups[2].Sha3_384,
		"drwxr-xr-x " + setup.Sha3_384,
		"Lrwxrwxrwx x1",
		"Lrwxrwxrwx x3",
		"Lrwxrwxrwx x4",
	})
}

func (s *manifestSuite) TestLaunchValidatesProjectSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	architecture := arch.ArchitectureType(arch.DpkgArchitecture())
	arch.SetArchitecture("amd64")
	defer arch.SetArchitecture(architecture)

	s.mockProjectSdk(c, "test", `name: test
architecture: arm64
`)

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.ProjectSource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.ErrorMatches, `cannot launch "test-1": "test" SDK has "arm64" architecture; required: "amd64" or "all"`)
}

func (s *manifestSuite) TestLaunchRejectsProjectSdkUnknownField(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.mockProjectSdk(c, "test", `name: test
ssh-agent:
  interface: ssh-agent
`)

	sdks := []workshop.SdkRecord{{Name: "test", Source: sdk.ProjectSource}}
	s.createWFile(c, "test-1", "ubuntu@20.04", sdks)

	_, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	var unknown *sdk.UnknownYamlFieldsError
	ok := errors.As(err, &unknown)
	c.Assert(ok, check.Equals, true)
	c.Check(unknown.Fields, check.DeepEquals, map[string]sdk.UnknownYamlField{
		"ssh-agent": {
			Line:   3,
			Column: 3,
		},
	})
}

func (s *manifestSuite) TestRefreshDetectsSketchSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", nil)

	s.mockSketchSdk(c, "test-1", "name: sketch\n")

	current, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)

	c.Assert(current, check.HasLen, 1)
	c.Assert(current[0].Sdks, check.HasLen, 0)

	c.Assert(latest, check.HasLen, 1)
	c.Assert(latest[0].Sdks, check.HasLen, 2)
	c.Check(latest[0].Sdks[0].Name, check.Equals, "system")
	c.Check(latest[0].Sdks[1].Name, check.Equals, "sketch")
	c.Check(latest[0].Sdks[1].Source, check.Equals, sdk.SketchSource)
	c.Check(latest[0].Sdks[1].Revision, check.Equals, sdk.R(-1))
}

func (s *manifestSuite) TestLaunchRemovesSketchSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	s.createWFile(c, "test-1", "ubuntu@20.04", nil)

	userDataDir := workshop.UserDataRootDir(s.user.HomeDir, nil)
	sketchdir := workshop.SketchSdkCurrent(userDataDir, s.project.ProjectId, "test-1")
	stashdir := workshop.SketchSdkStash(userDataDir, s.project.ProjectId, "test-1")

	s.mockSketchSdk(c, "test-1", "name: sketch\n")

	// Mock stashed sketch.
	hooksdir := filepath.Join(stashdir, "hooks")
	err := mockSdk(stashdir, hooksdir, "name: sketch\nbase: ubuntu@20.04\n")
	c.Assert(err, check.IsNil)

	c.Check(sketchdir, testutil.FilePresent)
	c.Check(stashdir, testutil.FilePresent)

	manifests, err := s.manager.LaunchManifests(s.ctx, s.project, []string{"test-1"})
	c.Assert(err, check.IsNil)

	c.Assert(manifests, check.HasLen, 1)
	c.Assert(manifests[0].Sdks, check.HasLen, 1)
	c.Check(manifests[0].Sdks[0].Name, check.Equals, "system")

	c.Check(sketchdir, testutil.FileAbsent)
	c.Check(stashdir, testutil.FilePresent)
}

func (s *manifestSuite) TestRefreshRespectsExplicitSketchSdk(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{{
		Name:   "sketch",
		Source: sdk.SketchSource,
		Plugs:  map[string]workshop.PlugOrBind{"ssh-agent": {Plug: nil}},
	}}
	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", sdks)

	_, _, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
	c.Check(err, check.ErrorMatches, `cannot refresh "test-1": "sketch" SDK not found, but appears in workshop definition`)
}

func (s *manifestSuite) TestRefreshSortsSdks(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []workshop.SdkRecord{
		{Name: "rust", Channel: "latest/stable"},
		{Name: "linter", Source: sdk.ProjectSource},
		{Name: "sketch", Source: sdk.SketchSource},
		{Name: "uv", Channel: "latest/edge"},
		{Name: "jupyter", Source: sdk.TrySource},
		{Name: "system", Source: sdk.SystemSource},
		{Name: "node", Channel: "latest/stable"},
		{Name: "lsp", Source: sdk.ProjectSource},
		{Name: "rocm", Source: sdk.TrySource},
	}
	s.launchWorkshopWithSDKs(c, "test-1", "ubuntu@20.04", sdks)

	s.mockProjectSdk(c, "linter", "name: linter\n")
	s.mockTrySdk(c, "jupyter", "jupyter_all.sdk", "name: jupyter\n")
	s.mockProjectSdk(c, "lsp", "name: lsp\n")
	s.mockTrySdk(c, "rocm", "rocm_all.sdk", "name: rocm\n")
	s.mockSketchSdk(c, "test-1", "name: sketch\n")

	_, latest, err := s.manager.RefreshManifests(s.ctx, s.project, []string{"test-1"}, conflict.RefreshUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(latest, check.HasLen, 1)

	sorted := make([]string, 0, len(latest[0].Sdks))
	for _, s := range latest[0].Sdks {
		where := s.Channel
		if s.Source != sdk.StoreSource {
			source, err := s.Source.MarshalText()
			c.Assert(err, check.IsNil)
			where = string(source)
		}
		sorted = append(sorted, s.Name+": "+where)
	}

	expected := []string{
		"system: system",
		"rust: latest/stable",
		"linter: project",
		"uv: latest/edge",
		"jupyter: try",
		"node: latest/stable",
		"lsp: project",
		"rocm: try",
		"sketch: sketch",
	}
	c.Check(sorted, check.DeepEquals, expected)
}
