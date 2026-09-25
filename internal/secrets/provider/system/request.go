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

package system

import (
	"errors"
	"strings"
)

// Request describes a lookup against a user's host Secret Service.
type Request struct {
	Attributes map[string]string
	Collection string
	UID        string
}

// validateRequest checks that request identifies a collection and at least
// one named search attribute.
func validateRequest(request Request) error {
	if strings.TrimSpace(request.Collection) == "" {
		return errors.New("secret request collection is missing")
	}
	if len(request.Attributes) == 0 {
		return errors.New("secret request attributes are missing")
	}
	for name := range request.Attributes {
		if strings.TrimSpace(name) == "" {
			return errors.New("secret request attribute name is empty")
		}
	}
	return nil
}
