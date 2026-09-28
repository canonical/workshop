// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package hookstate

import (
	check "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// identitySuite tests identity resolution for taskless and task-backed hooks.
type identitySuite struct{}

var _ = check.Suite(&identitySuite{})

// TestWorkshopIdentityFromStoredValue checks that a taskless context stores
// and returns the workshop identity by value, isolating it from caller changes.
func (s *identitySuite) TestWorkshopIdentityFromStoredValue(c *check.C) {
	ctx, err := NewContext(nil, state.New(nil), nil, nil, "test-context")
	c.Assert(err, check.IsNil)

	want := WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/home/test-user/project",
			ProjectId: "12345678",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	}
	ctx.SetWorkshopIdentity(want)

	got, err := ctx.WorkshopIdentity()

	c.Check(err, check.IsNil)
	c.Check(got, check.DeepEquals, want)
	got.Project.Path = "/changed/path"
	got.User = "changed-user"
	got.Workshop = "changed-workshop"
	stored, err := ctx.WorkshopIdentity()
	c.Check(err, check.IsNil)
	c.Check(stored, check.DeepEquals, want)
}

// TestWorkshopIdentityFromTask checks that [Context.WorkshopIdentity] returns
// the project, user and workshop name from task metadata.
func (s *identitySuite) TestWorkshopIdentityFromTask(c *check.C) {
	st := state.New(nil)
	want := WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/home/test-user/project",
			ProjectId: "12345678",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	}
	st.Lock()
	task := st.NewTask("run-hook", "Run test hook")
	change := st.NewChange("test-change", "Test hook identity")
	change.AddTask(task)
	task.Set("project", want.Project)
	task.Set("workshop", want.Workshop)
	task.Change().Set("user", want.User)
	st.Unlock()

	ctx, err := NewContext(task, st, nil, nil, "test-context")
	c.Assert(err, check.IsNil)

	got, err := ctx.WorkshopIdentity()

	c.Check(err, check.IsNil)
	c.Check(got, check.DeepEquals, want)
}

// TestWorkshopIdentityFromTaskMissingMetadata checks that missing task metadata
// produces a contextual error rather than falling back to the stored identity.
func (s *identitySuite) TestWorkshopIdentityFromTaskMissingMetadata(c *check.C) {
	st := state.New(nil)
	st.Lock()
	task := st.NewTask("run-hook", "Run test hook")
	change := st.NewChange("test-change", "Test hook identity")
	change.AddTask(task)
	st.Unlock()
	ctx, err := NewContext(task, st, nil, nil, "test-context")
	c.Assert(err, check.IsNil)
	ctx.SetWorkshopIdentity(WorkshopIdentity{User: "ignored-user"})

	_, err = ctx.WorkshopIdentity()

	c.Check(err, check.ErrorMatches,
		`cannot get workshop identity: cannot get project, task "1": .*`)
}

// TestWorkshopIdentityFromTaskOverridesStoredValue checks that setting a stored
// identity does not override the identity supplied by task metadata.
func (s *identitySuite) TestWorkshopIdentityFromTaskOverridesStoredValue(
	c *check.C,
) {
	st := state.New(nil)
	want := WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/home/test-user/project",
			ProjectId: "12345678",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	}
	st.Lock()
	task := st.NewTask("run-hook", "Run test hook")
	change := st.NewChange("test-change", "Test hook identity")
	change.AddTask(task)
	task.Set("project", want.Project)
	task.Set("workshop", want.Workshop)
	change.Set("user", want.User)
	st.Unlock()

	ctx, err := NewContext(task, st, nil, nil, "test-context")
	c.Assert(err, check.IsNil)
	ctx.SetWorkshopIdentity(WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/home/other-user/project",
			ProjectId: "87654321",
		},
		User:     "other-user",
		Workshop: "other-workshop",
	})

	got, err := ctx.WorkshopIdentity()

	c.Check(err, check.IsNil)
	c.Check(got, check.DeepEquals, want)
}

// TestWorkshopIdentityMissing checks that a taskless context returns an error
// when no workshop identity has been set.
func (s *identitySuite) TestWorkshopIdentityMissing(c *check.C) {
	ctx, err := NewContext(nil, state.New(nil), nil, nil, "test-context")
	c.Assert(err, check.IsNil)

	_, err = ctx.WorkshopIdentity()

	c.Check(err, check.ErrorMatches, "missing workshop identity")
}
