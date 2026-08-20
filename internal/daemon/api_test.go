// Copyright (c) 2014-2020 Canonical Ltd
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
	"net/http"
	"os/user"
	"testing"
	"time"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/ifacetest"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

var _ = check.Suite(&apiSuite{})

type apiSuite struct {
	d          *Daemon
	b          *fakebackend.FakeWorkshopBackend
	secBackend *ifacetest.TestSecurityBackend
	store      *sdk.FakeStore

	workshopDir string
	user        *user.User
	installedAt time.Time
	project     workshop.Project
	ctx         context.Context

	vars map[string]string

	restoreMuxVars    func()
	restoreRetrieve   func()
	restoreProjectId  func()
	restoreUserEnv    func()
	restoreUserLookup func()
	restoreTime       func()
	restoreSanitize   func()
	restoreSecBackend func()
}

func TestApi(t *testing.T) { check.TestingT(t) }

func (s *apiSuite) SetUpTest(c *check.C) {
	s.restoreMuxVars = FakeMuxVars(s.muxVars)
	s.workshopDir = c.MkDir()

	// Real UID and GID are required to copy local SDKs and create the apt cache directory.
	// TODO: make filesystem operations more secure (e.g. drop privileges if possible) and easy to test.
	actual, err := user.Current()
	c.Assert(err, check.IsNil)
	s.user = &user.User{Username: "testuser", HomeDir: c.MkDir(), Uid: actual.Uid, Gid: actual.Gid}

	s.project = workshop.Project{
		Path:      s.workshopDir,
		ProjectId: "b8639dea",
	}

	s.store = sdk.NewFakeStore()
	s.restoreRetrieve = system.FakeRetrieveSystemSdk(systemDownload(c))

	s.b, err = fakebackend.New(c.MkDir())
	c.Check(err, check.IsNil)

	s.installedAt = time.Date(2023, 04, 25, 1, 2, 3, 0, time.UTC)
	s.restoreTime = testutil.FakeFunc(func() time.Time { return s.installedAt }, &workshop.InstallTimeNow)

	// will be called when project is created
	s.restoreProjectId = testutil.FakeFunc(func() (string, error) { return s.project.ProjectId, nil }, &workshop.NewProjectId)

	ctx := context.WithValue(context.TODO(), workshop.ContextProjectId, s.project.ProjectId)
	s.ctx = context.WithValue(ctx, workshop.ContextUser, s.user.Username)

	_, _, err = s.b.CreateOrLoadProject(s.ctx, s.project.Path)
	c.Assert(err, check.IsNil)

	s.restoreSanitize = sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})
	s.secBackend = &ifacetest.TestSecurityBackend{BackendName: "api-suite"}
	s.restoreSecBackend = ifacestate.MockSecurityBackends([]interfaces.SecurityBackend{s.secBackend})

	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		if name != "testuser" {
			return nil, user.UnknownUserError("not found")
		}
		return s.user, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})
}

func (s *apiSuite) TearDownTest(c *check.C) {
	s.d = nil
	s.workshopDir = ""
	s.vars = nil
	s.restoreMuxVars()
	s.restoreRetrieve()
	s.restoreProjectId()
	s.restoreUserEnv()
	s.restoreUserLookup()
	s.restoreTime()
	s.restoreSanitize()
	s.restoreSecBackend()
}

func (s *apiSuite) muxVars(*http.Request) map[string]string {
	return s.vars
}

func (s *apiSuite) daemon(c *check.C) *Daemon {
	if s.d != nil {
		panic("called daemon() twice")
	}
	dirs.SetRootDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)
	undo := overlord.MockWorkshopBackend(s.b)
	defer undo()

	d, err := New(&Options{Dir: s.workshopDir})
	c.Assert(err, check.IsNil)

	sdk.ReplaceStore(d.state, s.store)

	c.Assert(d.overlord.StartUp(), check.IsNil)
	d.addRoutes()
	s.d = d

	return d
}

func apiCmd(path string) *Command {
	for _, cmd := range api {
		if cmd.Path == path {
			return cmd
		}
	}
	panic("no command with path " + path)
}
