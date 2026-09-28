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
	"errors"
	"fmt"

	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/workshop"
)

// WorkshopIdentity identifies the workshop and user associated with a context.
type WorkshopIdentity struct {
	// Project is the project containing the workshop.
	Project workshop.Project

	// User is the user on whose behalf the context operates.
	User string

	// Workshop is the workshop name within the project.
	Workshop string
}

// SetWorkshopIdentity sets the identity for a taskless context. It is intended
// to initialise the context before dispatch. Task-backed contexts use task
// metadata instead.
//
// This method acquires its own lock; callers must not hold the context lock.
func (c *Context) SetWorkshopIdentity(identity WorkshopIdentity) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.workshopIdentity = &identity
}

// WorkshopIdentity returns the task metadata identity for a task-backed
// context, or the stored identity for a taskless context. It returns an error
// if the task metadata or stored identity is missing.
//
// This method acquires its own locks; callers must not hold the context or
// state lock.
func (c *Context) WorkshopIdentity() (WorkshopIdentity, error) {
	if c.task != nil {
		user, project, name, err := handlersetup.UserProjectWorkshop(c.task)
		if err != nil {
			return WorkshopIdentity{}, fmt.Errorf(
				"cannot get workshop identity: %w", err,
			)
		}
		return WorkshopIdentity{
			Project:  *project,
			User:     user,
			Workshop: name,
		}, nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.workshopIdentity == nil {
		return WorkshopIdentity{}, errors.New("missing workshop identity")
	}
	return *c.workshopIdentity, nil
}
