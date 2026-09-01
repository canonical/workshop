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

package sdkstate_test

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type sdkStateSuite struct {
	backend     *fakebackend.FakeWorkshopBackend
	state       *state.State
	runner      *state.TaskRunner
	se          *overlord.StateEngine
	sdkmgr      *sdkstate.SdkManager
	repo        *interfaces.Repository
	ctx         context.Context
	user        *user.User
	project     workshop.Project
	installedAt time.Time

	restoreUserLookup  func()
	restoreUserEnv     func()
	restoreProjectId   func()
	restoreInstallTime func()
}

var _ = check.Suite(&sdkStateSuite{})

func TestMain(m *testing.M) {
	// Ensure consistent file permissions for sdkStateSuite.
	syscall.Umask(0002)
	m.Run()
}

func Test(t *testing.T) { check.TestingT(t) }

func fakeHandler(task *state.Task, _ *tomb.Tomb) error {
	return nil
}

func setWorkshopProject(w string, p workshop.Project, tasks ...*state.Task) {
	for _, i := range tasks {
		i.Set("workshop", w)
		i.Set("project", p)
	}
}

var ErrTrigger = errors.New("error out")

var sdkYaml = `
name: test
base: ubuntu@22.04
plugs:
  plug:
    interface: test-interface
    attr: value
  plug2:
    interface: test-interface
    attr2: value2
`

var sdkYamlViolatesPolicy = `
name: test-broken
base: ubuntu@22.04
plugs:
  plug:
    interface: test-interface
    attr: value
  plug2:
    interface: test-interface
    attr2: value2
slots:
  slot:
    interface: ssh-agent
`

func (s *sdkStateSuite) SetUpTest(c *check.C) {
	var err error
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)

	ctx := context.WithValue(context.TODO(), workshop.ContextProjectId, "projectId")
	s.ctx = context.WithValue(ctx, workshop.ContextUser, "testuser")

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
	c.Check(err, check.IsNil)

	s.project = workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "projectId",
	}
	s.restoreProjectId = testutil.FakeFunc(func() (string, error) { return s.project.ProjectId, nil }, &workshop.NewProjectId)
	s.backend.CreateOrLoadProject(s.ctx, s.project.Path)

	s.state = state.New(nil)
	s.runner = state.NewTaskRunner(s.state)

	workshop.ReplaceBackend(s.state, s.backend)

	sdk.ReplaceStore(s.state, sdk.NewFakeStore())

	/* empty task handler */
	s.runner.AddHandler("fake-task", fakeHandler, nil)

	s.repo = interfaces.NewRepository()
	mockIface(c, s.repo, &ifacetest.TestInterface{InterfaceName: "test-interface"})
	s.sdkmgr = sdkstate.New(s.state, s.runner, s.repo)

	/* error-provoking task handler */
	erroringHandler := func(task *state.Task, _ *tomb.Tomb) error {
		return ErrTrigger
	}
	errorOnceHandler := func(task *state.Task, _ *tomb.Tomb) error {
		st := task.State()
		st.Lock()
		attempt, err := conflict.ChangeAttempt(task.Change())
		st.Unlock()
		if err != nil {
			return err
		}
		if attempt <= 1 {
			return ErrTrigger
		}
		return nil
	}
	retryHandler := func(task *state.Task, _ *tomb.Tomb) error {
		// to keep the change not ready
		return &state.Retry{After: 1 * time.Hour}
	}
	s.runner.AddHandler("error-trigger", erroringHandler, nil)
	s.runner.AddHandler("error-once", handlersetup.OnDo(errorOnceHandler), nil)
	s.runner.AddHandler("retry-task", retryHandler, nil)

	s.se = overlord.NewStateEngine(s.state)
	s.se.AddManager(s.sdkmgr)
	s.se.AddManager(s.runner)
	err = s.se.StartUp()
	c.Assert(err, check.IsNil)

	s.installedAt = time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC)
	s.restoreInstallTime = testutil.FakeFunc(func() time.Time { return s.installedAt }, &workshop.InstallTimeNow)

	wf := &workshop.File{Name: "ws", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{
		{Name: "test", Channel: "latest/stable"},
		{Name: "test-broken", Channel: "latest/stable"},
	}}
	snapshot := workshop.BaseOnly(sdk.R(1), wf.Base, workshop.ConfinementContainer, "fakeimage123")
	err = s.backend.LaunchOrRebuildWorkshop(s.ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	wf2 := &workshop.File{Name: "ws2", Base: "ubuntu@20.04", Sdks: []workshop.SdkRecord{
		{Name: "test", Channel: "latest/stable"},
	}}
	err = s.backend.LaunchOrRebuildWorkshop(s.ctx, wf2, snapshot)
	c.Assert(err, check.IsNil)
}

func (s *sdkStateSuite) mockSdk(c *check.C, meta sdk.Meta) {
	vfs := c.MkDir()

	path := filepath.Join(vfs, "meta", "sdk.yaml")
	err := os.MkdirAll(filepath.Dir(path), 0755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(path, []byte(meta.SdkYAML), 0644)
	c.Assert(err, check.IsNil)
	file, err := os.Open(vfs)
	c.Assert(err, check.IsNil)
	defer file.Close()
	err = s.backend.ImportSdk(s.ctx, meta, file)
	c.Assert(err, check.IsNil)
}

func mockIface(c *check.C, repo *interfaces.Repository, iface interfaces.Interface) {
	err := repo.AddInterface(iface)
	c.Assert(err, check.IsNil)
}

func (s *sdkStateSuite) TearDownTest(c *check.C) {
	s.restoreUserLookup()
	s.restoreUserEnv()
	s.restoreProjectId()
	s.restoreInstallTime()
}

func (s *sdkStateSuite) TestDoInstallSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	newSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(2),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, newSdk)

	t := s.state.NewTask("install-sdk", "test")
	t.Set("sdk", newSdk.Name)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t)
	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	chg.AddTask(t)

	s.state.Unlock()
	c.Check(s.se.Ensure(), check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Check(chg.Err(), check.IsNil)
	c.Check(chg.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.Volumes, check.HasLen, 1)
	volume := s.backend.Volumes[sdk.VolumeName(newSdk.Name, newSdk.Revision)]
	c.Assert(volume.Mounts, check.HasLen, 1)
	c.Check(volume.Mounts[0].ProjectId, check.Equals, "projectId")
	c.Check(volume.Mounts[0].Workshop, check.Equals, "ws")
	c.Check(volume.Mounts[0].Where, check.Equals, "/var/lib/workshop/sdk/test")

	props, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	c.Check(props.Sdks["test"].Setup, check.DeepEquals, newSdk.Setup)
	c.Check(props.Sdks["test"].InstalledAt, check.Equals, s.installedAt)

	sdkInfo, err := props.SdkInfo(s.ctx, "test")
	c.Assert(err, check.IsNil)
	c.Assert(sdkInfo.Plugs, check.HasLen, 2)
	c.Assert(sdkInfo.Slots, check.HasLen, 0)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 2)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.NotNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.NotNil)
}

func (s *sdkStateSuite) TestDoInstallSdkFailedPolicyCheck(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	testSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test-broken",
			PackageID: "8pp61flaU7ZSSChRTvrDw1PImyu83v6P",
			Channel:   "latest/stable",
			Revision:  sdk.R(2),
			Sha3_384:  "eee11792d075bd015406afe6450ac4f5080d78867da10cc5aa9380c383f31b71c8c71d831edd53c67eafc4b745a6bc80",
		},
		SdkYAML: sdkYamlViolatesPolicy,
	}
	s.mockSdk(c, testSdk)

	t := s.state.NewTask("install-sdk", "...")
	t.Set("sdk", testSdk.Name)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t)
	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{testSdk.Setup})
	chg.AddTask(t)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Assert(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*installation not allowed by "slot" slot rule of interface "ssh-agent".*`)

	// not in the fs (removed)
	wfs, err := s.backend.WorkshopFs(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	defer wfs.Close()
	_, err = wfs.Stat(sdk.SdkDir("test-broken"))
	c.Check(osutil.IsDirNotExist(err), check.Equals, true)

	// not in the SDK list (unlinked)
	wp, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, ok := wp.Sdks["test-broken"]
	c.Check(ok, check.Equals, false)

	// not in the repo (removed)
	c.Check(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Check(s.repo.Slots(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
}

func (s *sdkStateSuite) TestDoInstallSdkBadInterfacesFound(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	newSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, newSdk)
	t := s.state.NewTask("install-sdk", "...")
	t.Set("sdk", newSdk.Name)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t)

	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	chg.AddTask(t)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.ErrorMatches, `(?s).*"test" SDK has bad plugs or slots: plug, plug2 \(unknown interface "test-interface"\).*`)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.IsNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.IsNil)
}

func (s *sdkStateSuite) TestUndoInstallSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	newSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, newSdk)

	t := s.state.NewTask("install-sdk", "test")
	t.Set("sdk", newSdk.Name)

	terr := s.state.NewTask("error-trigger", "provoking total undo")
	terr.WaitFor(t)

	chg := s.state.NewChange("sample", "...")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	chg.AddTask(t)
	chg.AddTask(terr)

	setWorkshopProject("ws", s.project, t, terr)

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Check(t.Status(), check.Equals, state.UndoneStatus)

	c.Assert(s.repo.Plugs(s.project.ProjectId, "ws", "test"), check.HasLen, 0)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug"), check.IsNil)
	c.Assert(s.repo.Plug(s.project.ProjectId, "ws", "test", "plug2"), check.IsNil)

	props, err := s.backend.Workshop(s.ctx, "ws")
	c.Assert(err, check.IsNil)
	_, ok := props.Sdks["test"]
	c.Check(ok, check.Equals, false)

	c.Check(s.backend.Volumes, check.HasLen, 1)
	volume := s.backend.Volumes[sdk.VolumeName(newSdk.Name, newSdk.Revision)]
	c.Check(volume.Mounts, check.HasLen, 0)
}

func (s *sdkStateSuite) TestRetrieveSystemSdkSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	newSdk := sdk.Setup{
		Name:     sdk.System.String(),
		Source:   sdk.SystemSource,
		Revision: system.SystemSdkRevision,
		Sha3_384: system.SystemSdkDigest,
	}
	t := s.state.NewTask("retrieve-sdk", "retrieve")
	t.Set("sdk", newSdk.Name)

	chg := s.state.NewChange("sample", "...")
	setWorkshopProject("ws", s.project, t)
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseOnly(sdk.R(1), "ubuntu@22.04", workshop.ConfinementContainer, "fakeimage123"))
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk})
	chg.AddTask(t)

	s.state.Unlock()
	c.Check(s.se.Ensure(), check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)

	f, err := os.Open(newSdk.Filepath())
	c.Assert(err, check.IsNil)
	defer f.Close()

	tr := tar.NewReader(f)
	var entries []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, check.IsNil)

		info := hdr.FileInfo()
		entry := fmt.Sprintf("%s %s", info.Mode(), hdr.Name)
		entries = append(entries, entry)

		if info.IsDir() {
			continue
		}

		expected, err := system.SystemSdkFs.ReadFile(hdr.Name)
		c.Assert(err, check.IsNil)
		actual, err := io.ReadAll(tr)
		c.Assert(err, check.IsNil)
		c.Check(actual, check.DeepEquals, expected)
	}

	c.Check(entries, check.DeepEquals, []string{"drwxr-xr-x meta/", "-rw-r--r-- meta/sdk.yaml"})

	expected, err := system.SystemSdkFs.ReadFile("meta/sdk.yaml")
	c.Assert(err, check.IsNil)
	c.Check(newSdk.Filepath()+".yaml", testutil.FileEquals, expected)
}

func (s *sdkStateSuite) TestSnapshotSdkTwice(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	image := workshop.BaseImage{Name: "ubuntu@22.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"}
	sdks := []sdk.Setup{{
		Name:     "test",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
	}, {
		Name:     "next",
		Channel:  "latest/edge",
		Revision: sdk.R(2),
		Sha3_384: "f77092aae283a4d8ffe7ae51017569d6c999afdbe7c4264ab82f44e79a82151994de9b90ae43216dee70aaec91b23647",
	}}
	snapshot1 := workshop.SdkSnapshot(sdk.R(1), image, sdks[:1])
	snapshot2 := workshop.SdkSnapshot(sdk.R(1), image, sdks)

	t1 := s.state.NewTask("snapshot-sdk", "...")
	t1.Set("sdk", "test")
	t2 := s.state.NewTask("snapshot-sdk", "...")
	t2.Set("sdk", "next")
	t2.WaitFor(t1)
	setWorkshopProject("ws", s.project, t1, t2)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", image)
	chg.Set("ws_new_sdks", sdks)
	chg.AddTask(t1)
	chg.AddTask(t2)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(s.backend.SnapshotCalls, check.HasLen, 2)
	c.Check(s.backend.SnapshotCalls[0].Snapshot, check.DeepEquals, snapshot1)
	c.Check(s.backend.SnapshotCalls[1].Snapshot, check.DeepEquals, snapshot2)
	c.Assert(s.backend.Snapshots, check.HasLen, 2)
	c.Check(s.backend.Snapshots[0].Snapshot, check.DeepEquals, snapshot1)
	c.Check(s.backend.Snapshots[1].Snapshot, check.DeepEquals, snapshot2)

	t := s.state.NewTask("snapshot-sdk", "...")
	t.Set("sdk", "test")
	setWorkshopProject("ws", s.project, t)

	chg = s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", image)
	chg.Set("ws_new_sdks", sdks[:1])
	chg.AddTask(t)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(s.backend.SnapshotCalls, check.HasLen, 3)
	c.Check(s.backend.SnapshotCalls[0].Snapshot, check.DeepEquals, snapshot1)
	c.Check(s.backend.SnapshotCalls[1].Snapshot, check.DeepEquals, snapshot2)
	c.Check(s.backend.SnapshotCalls[2].Snapshot, check.DeepEquals, snapshot1)
	c.Assert(s.backend.Snapshots, check.HasLen, 2)
	c.Check(s.backend.Snapshots[0].Snapshot, check.DeepEquals, snapshot1)
	c.Check(s.backend.Snapshots[1].Snapshot, check.DeepEquals, snapshot2)
}

func (s *sdkStateSuite) TestSnapshotSkippedPostResume(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	sdks := []sdk.Setup{{
		Name:     "test",
		Channel:  "latest/stable",
		Revision: sdk.R(1),
		Sha3_384: "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
	}}

	t1 := s.state.NewTask("error-once", "...")

	t2 := s.state.NewTask("snapshot-sdk", "...")
	t2.Set("sdk", "test")
	setWorkshopProject("ws", s.project, t2)
	t2.WaitFor(t1)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	chg.Set("project-id", s.project.ProjectId)
	chg.Set("ws_new_format", sdk.R(1))
	chg.Set("ws_new_base", workshop.BaseImage{Name: "ubuntu@22.04", Confinement: workshop.ConfinementContainer, Fingerprint: "fakeimage123"})
	chg.Set("ws_new_sdks", sdks)
	chg.Set("wait-setup", conflict.ChangeSetup{Mode: conflict.ChangeWaitOnError.String()})
	chg.AddTask(t1)
	chg.AddTask(t2)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.Status(), check.Equals, state.WaitStatus)

	chg2, err := conflict.ResumeAfterWait(s.state, sdk.R(1), "ws", s.project.ProjectId, conflict.ChangeContinue, "launch")
	c.Assert(err, check.IsNil)
	c.Assert(chg2.ID(), check.Equals, chg.ID())

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Assert(chg.Err(), check.IsNil)
	c.Assert(chg.Status(), check.Equals, state.DoneStatus)

	c.Check(s.backend.SnapshotCalls, check.HasLen, 0)
	log := t2.Log()
	c.Assert(log, check.HasLen, 1)
	c.Check(log[0], check.Matches, ".* Skipping snapshot after launch was resumed")
}

func (s *sdkStateSuite) TestSDKVolumeRemovedAfterCooldownOK(c *check.C) {
	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)
	t := s.state.NewTask("uninstall-sdk", "...")
	t.Set("sdk", oldSdk.Name)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)
	s.state.Unlock()

	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)
	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.Equals, workshop.ErrVolumeNotFound)
	c.Assert(t.IsClean(), check.Equals, true)
}

func (s *sdkStateSuite) TestSDKVolumeRemovedAfterFailedLaunch(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	s.state.Lock()
	newSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, newSdk)
	t := s.state.NewTask("install-sdk", "...")
	t.Set("sdk", newSdk.Name)

	t2 := s.state.NewTask("error-trigger", "...")
	t2.WaitFor(t)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t, t2)
	chg.AddTask(t)
	chg.AddTask(t2)
	s.state.Unlock()

	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(t.Status(), check.Equals, state.UndoneStatus)
	_, err := s.backend.Sdk(s.ctx, newSdk.Setup)
	c.Assert(err, check.Equals, workshop.ErrVolumeNotFound)
	c.Assert(t.IsClean(), check.Equals, true)
}

func (s *sdkStateSuite) TestSDKVolumeExitCleanupAfterSuccessfulLaunch(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	s.state.Lock()
	newSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, newSdk)
	t := s.state.NewTask("install-sdk", "...")
	t.Set("sdk", newSdk.Name)

	chg := s.state.NewChange("launch", "...")
	chg.Set("user", "testuser")
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)
	s.state.Unlock()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(t.Status(), check.Equals, state.DoneStatus)
	_, err := s.backend.Sdk(s.ctx, newSdk.Setup)
	c.Assert(err, check.IsNil)
	c.Assert(t.IsClean(), check.Equals, true)
}

func (s *sdkStateSuite) TestSDKVolumeNotRemovedBeforeCooldown(c *check.C) {
	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)
	t := s.state.NewTask("uninstall-sdk", "...")
	t.Set("sdk", oldSdk.Name)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)
	s.state.Unlock()

	// Set cooldown to a large value so it never passes
	defer sdkstate.FakeSdkVolumeCooldownTime(24 * time.Hour)()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()
	defer s.state.Unlock()
	c.Check(chg.Err(), check.IsNil)
	// The volume should still exist
	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.IsNil)
	// The task should not be clean (cleanup not performed)
	c.Assert(t.IsClean(), check.Equals, false)
}

func (s *sdkStateSuite) TestTaskSDKVolumeExitCleanupIfUsedAgain(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)
	t := s.state.NewTask("uninstall-sdk", "...")
	t.Set("sdk", oldSdk.Name)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	other := s.state.NewChange("launch", "...")
	other.Set("user", "testuser")
	other.Set("ws2_new_sdks", []sdk.Setup{oldSdk.Setup})
	t2 := s.state.NewTask("install-sdk", "t2")
	t2.Set("sdk", oldSdk.Name)
	setWorkshopProject("ws2", s.project, t2)
	other.AddTask(t2)
	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	s.state.Unlock()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()

	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.IsNil)
	c.Assert(t.IsClean(), check.Equals, true)
	c.Assert(t2.IsClean(), check.Equals, true)
}

func (s *sdkStateSuite) TestTaskSDKVolumeRetriesCleanupIfBlockingChangesArePresent(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)
	t := s.state.NewTask("uninstall-sdk", "...")
	t.Set("sdk", oldSdk.Name)

	chg := s.state.NewChange("sample", "...")
	chg.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t)
	chg.AddTask(t)

	other := s.state.NewChange("launch", "...")
	other.Set("user", "testuser")
	other.Set("ws2_new_sdks", []sdk.Setup{oldSdk.Setup})
	t2 := s.state.NewTask("install-sdk", "t2")
	t2.Set("sdk", oldSdk.Name)
	t2.SetToWait(state.DoStatus)
	t3 := s.state.NewTask("error-trigger", "t3")
	t3.WaitFor(t2)
	setWorkshopProject("ws2", s.project, t2, t3)
	other.AddTask(t2)
	other.AddTask(t3)

	s.state.Unlock()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()

	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Check(err, check.IsNil)
	c.Check(t.IsClean(), check.Equals, false)

	// Finish the "launch" change that would enable the t cleanup to finish.
	waited := t2.WaitedStatus()
	t2.SetStatus(waited)
	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	s.state.Unlock()

	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()
	defer s.state.Unlock()
	c.Check(t.IsClean(), check.Equals, true)
	_, err = s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.Equals, workshop.ErrVolumeNotFound)
	c.Check(t2.IsClean(), check.Equals, true)
}

func (s *sdkStateSuite) TestSDKVolumeCleanupPerformedByLatestUser(c *check.C) {
	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)

	t1 := s.state.NewTask("uninstall-sdk", "t1")
	t1.Set("sdk", oldSdk.Name)

	t2 := s.state.NewTask("uninstall-sdk", "t2")
	t2.Set("sdk", oldSdk.Name)
	t2.WaitFor(t1)

	// Add both tasks to their own changes
	chg1 := s.state.NewChange("refresh", "chg1")
	chg1.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg1.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg1.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t1)
	chg1.AddTask(t1)

	chg2 := s.state.NewChange("refresh", "chg2")
	chg2.Set("user", "testuser")
	chg2.Set("ws2_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg2.Set("ws2_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws2", s.project, t2)
	chg2.AddTask(t2)

	s.state.Unlock()

	// Use default cooldown (1h), so t2 will not be clean (cooldown not passed)
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}

	s.state.Lock()
	defer s.state.Unlock()

	// The first task should be clean (cleanup skipped due to newer task)
	c.Assert(t1.IsClean(), check.Equals, true)
	// The second task should not be clean (cleanup not performed, cooldown not passed)
	c.Assert(t2.IsClean(), check.Equals, false)
	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.IsNil)
}

// Check that uninstall-sdk blocks cleanup for the cooldown period, even if
// the clenanup handler for another Task runs immediately afterwards.
func (s *sdkStateSuite) TestSDKVolumeCleanupBlockedBeforeUninstall(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	s.state.Lock()
	oldSdk := sdk.Meta{
		Setup: sdk.Setup{
			Name:      "test",
			PackageID: "a9J51jhjzpckN8VxhqoZ8dNKcZ7pOrBb",
			Channel:   "latest/stable",
			Revision:  sdk.R(1),
			Sha3_384:  "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, oldSdk)

	// Ensure SDK is in use via ws2.
	t := s.state.NewTask("install-sdk", "t")
	t.Set("sdk", oldSdk.Name)
	chg := s.state.NewChange("launch", "chg")
	chg.Set("user", "testuser")
	chg.Set("ws2_new_sdks", []sdk.Setup{oldSdk.Setup})
	setWorkshopProject("ws2", s.project, t)
	chg.AddTask(t)

	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Check(t.Status(), check.Equals, state.DoneStatus)

	defer sdkstate.FakeSdkVolumeCooldownTime(time.Hour)()
	t1time := time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)
	defer sdkstate.MockTime(t1time)()

	t1 := s.state.NewTask("uninstall-sdk", "t1")
	t1.Set("sdk", oldSdk.Name)
	chg1 := s.state.NewChange("refresh", "chg1")
	chg1.Set("user", "testuser")
	newSdk := oldSdk
	newSdk.Revision = sdk.R(2)
	chg1.Set("ws_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg1.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t1)
	chg1.AddTask(t1)

	// Uninstall SDK but don't clean it up (no fake time passes).
	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()

	c.Check(t1.Status(), check.Equals, state.DoneStatus)

	// Advance time and set up final uninstall.
	t2time := t1time.Add(time.Hour - time.Second)
	defer sdkstate.MockTime(t2time)()

	t2 := s.state.NewTask("uninstall-sdk", "t2")
	t2.Set("sdk", oldSdk.Name)
	t2.WaitFor(t1)
	chg2 := s.state.NewChange("refresh", "chg2")
	chg2.Set("user", "testuser")
	chg2.Set("ws2_old_sdks", []sdk.Setup{oldSdk.Setup})
	chg2.Set("ws2_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws2", s.project, t2)
	chg2.AddTask(t2)

	// Uninstall SDK from ws2 and attempt another cleanup.
	s.state.Unlock()
	c.Check(s.se.Ensure(), check.IsNil)
	s.se.Wait()
	s.state.Lock()

	c.Check(t2.Status(), check.Equals, state.DoneStatus)

	// Advance time again..
	defer sdkstate.MockTime(t2time.Add(2 * time.Second))()

	// Run the first post-cooldown cleanup for ws, and the first cleanup for
	// ws2. Even if ws wins the race, the SDK should not be deleted.
	s.state.Unlock()
	for range 6 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()
	defer s.state.Unlock()

	c.Check(t1.IsClean(), check.Equals, true)
	c.Check(t2.IsClean(), check.Equals, false)

	_, err := s.backend.Sdk(s.ctx, oldSdk.Setup)
	c.Assert(err, check.IsNil)
}

func (s *sdkStateSuite) TestSDKVolumeExitCleanupOnNonvolume(c *check.C) {
	s.state.Lock()
	sdkProject := sdk.Meta{
		Setup: sdk.Setup{
			Name:     "test",
			Source:   sdk.ProjectSource,
			Revision: sdk.R(-1),
			Sha3_384: "e516dabb23b6e30026863543282780a3ae0dccf05551cf0295178d7ff0f1b41eecb9db3ff219007c4e097260d58621bd",
		},
		SdkYAML: sdkYaml,
	}
	s.mockSdk(c, sdkProject)

	t1 := s.state.NewTask("uninstall-sdk", "t1")
	t1.Set("sdk", sdkProject.Name)
	chg1 := s.state.NewChange("refresh", "chg1")
	chg1.Set("user", "testuser")
	newSdk := sdkProject
	newSdk.Revision = sdk.R(-2)
	chg1.Set("ws_old_sdks", []sdk.Setup{sdkProject.Setup})
	chg1.Set("ws_new_sdks", []sdk.Setup{newSdk.Setup})
	setWorkshopProject("ws", s.project, t1)
	chg1.AddTask(t1)

	defer sdkstate.FakeSdkVolumeCooldownTime(0)()

	s.state.Unlock()
	for i := 0; i < 6; i = i + 1 {
		c.Check(s.se.Ensure(), check.IsNil)
		s.se.Wait()
	}
	s.state.Lock()
	defer s.state.Unlock()
	c.Assert(t1.IsClean(), check.Equals, true)
}
