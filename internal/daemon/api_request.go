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
	"net/http"

	"github.com/canonical/workshop/internal/workshop"
)

// workshopInstanceIDHeader identifies the workshop instance from which an API
// request originated.
const workshopInstanceIDHeader = "workshop-instance-id"

// withWorkshopInstanceID returns a response function that adds the workshop
// instance ID header value to the request context when the header is present,
// then calls next.
func withWorkshopInstanceID(next ResponseFunc) ResponseFunc {
	return func(c *Command, r *http.Request, user *userState) Response {
		instanceID := r.Header.Get(workshopInstanceIDHeader)
		if instanceID != "" {
			ctx := context.WithValue(
				r.Context(),
				workshop.ContextWorkshopInstanceID,
				instanceID,
			)
			r = r.WithContext(ctx)
		}

		return next(c, r, user)
	}
}
