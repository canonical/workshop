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

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type managerSuite struct {
	state   *state.State
	backend *fakebackend.FakeWorkshopBackend
	runner  *state.TaskRunner
	manager *workshopstate.WorkshopManager
}

var _ = check.Suite(&managerSuite{})

func (s *managerSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.runner = state.NewTaskRunner(s.state)
	s.manager = workshopstate.New(s.state, s.runner)
}

func (s *managerSuite) addWorkshop(
	c *check.C,
	ctx context.Context,
	name string,
	instanceID string,
) workshop.Project {
	project, _, err := s.backend.CreateOrLoadProject(ctx, c.MkDir())
	c.Assert(err, check.IsNil)

	if s.backend.Workshops[project.ProjectId] == nil {
		s.backend.Workshops[project.ProjectId] = make(
			map[string]*fakebackend.FakeWorkshop,
		)
	}
	s.backend.Workshops[project.ProjectId][name] = &fakebackend.FakeWorkshop{
		Workshop: &workshop.Workshop{
			Name:       name,
			Project:    *project,
			InstanceID: instanceID,
		},
	}
	return *project
}

func (s *managerSuite) TestAddHandlers(c *check.C) {
	workshopstate.New(s.state, s.runner)

	c.Assert(s.runner.KnownTaskKinds(), testutil.DeepUnsortedMatches, []string{
		"download-base",
		"create-workshop",
		"rebuild-workshop",
		"start-workshop",
		"stop-workshop",
		"remove-workshop",
		"configure-timezone",
		"mount-project",
		"create-workshop-storage",
		"remove-workshop-storage",
		"remove-workshop-stash",
		"stash-workshop",
		"create-state-storage",
		"remove-state-storage",
	})
}

// TestResolveWorkshopInstanceIDReturnsIdentity checks that the manager
// returns the owning project and workshop name for the user's instance ID.
func (s *managerSuite) TestResolveWorkshopInstanceIDReturnsIdentity(
	c *check.C,
) {
	ctx := context.WithValue(
		context.Background(),
		workshop.ContextUser,
		"test-user",
	)
	project := s.addWorkshop(c, ctx, "test-workshop", "instance-id")

	identity, err := s.manager.ResolveWorkshopInstanceID(ctx, "instance-id")

	c.Assert(err, check.IsNil)
	c.Assert(identity, check.NotNil)
	c.Check(identity.Project, check.DeepEquals, project)
	c.Check(identity.Workshop, check.Equals, "test-workshop")

	owns, err := s.manager.OwnsWorkshopInstanceID(ctx, "instance-id")
	c.Assert(err, check.IsNil)
	c.Check(owns, check.Equals, true)
}

// TestResolveWorkshopInstanceIDReturnsNilForUnknownID checks that the manager
// rejects an instance ID that does not identify one of the user's workshops.
func (s *managerSuite) TestResolveWorkshopInstanceIDReturnsNilForUnknownID(
	c *check.C,
) {
	ctx := context.WithValue(
		context.Background(),
		workshop.ContextUser,
		"test-user",
	)
	s.addWorkshop(c, ctx, "test-workshop", "instance-id")

	identity, err := s.manager.ResolveWorkshopInstanceID(ctx, "unknown-id")

	c.Assert(err, check.IsNil)
	c.Check(identity, check.IsNil)

	owns, err := s.manager.OwnsWorkshopInstanceID(ctx, "unknown-id")
	c.Assert(err, check.IsNil)
	c.Check(owns, check.Equals, false)
}

// TestResolveWorkshopInstanceIDReturnsNilForEmptyID checks that an empty
// instance ID does not resolve to a workshop.
func (s *managerSuite) TestResolveWorkshopInstanceIDReturnsNilForEmptyID(
	c *check.C,
) {
	ctx := context.WithValue(
		context.Background(),
		workshop.ContextUser,
		"test-user",
	)

	identity, err := s.manager.ResolveWorkshopInstanceID(ctx, "")

	c.Assert(err, check.IsNil)
	c.Check(identity, check.IsNil)
}

// TestResolveWorkshopInstanceIDRejectsAnotherUsersWorkshop checks that
// workshop ownership is scoped to the user in context.
func (s *managerSuite) TestResolveWorkshopInstanceIDRejectsAnotherUsersWorkshop(
	c *check.C,
) {
	ownerCtx := context.WithValue(
		context.Background(),
		workshop.ContextUser,
		"owner",
	)
	s.addWorkshop(c, ownerCtx, "test-workshop", "instance-id")
	requestCtx := context.WithValue(
		context.Background(),
		workshop.ContextUser,
		"another-user",
	)

	s.addWorkshop(c, requestCtx, "test-workshop", "another-instance-id")

	identity, err := s.manager.ResolveWorkshopInstanceID(
		requestCtx,
		"instance-id",
	)

	c.Assert(err, check.IsNil)
	c.Check(identity, check.IsNil)
}
