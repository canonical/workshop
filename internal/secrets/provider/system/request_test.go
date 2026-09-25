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

import "gopkg.in/check.v1"

// requestSuite tests lookup request validation independently of D-Bus.
type requestSuite struct{}

var _ = check.Suite(&requestSuite{})

// TestValidateRequest checks a complete lookup request is accepted.
func (s *requestSuite) TestValidateRequest(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example"},
		Collection: "default",
		UID:        "1000",
	}

	err := validateRequest(request)

	c.Check(err, check.IsNil)
}

// TestValidateRequestEmptyAttributeName checks an unnamed search attribute
// is rejected.
func (s *requestSuite) TestValidateRequestEmptyAttributeName(c *check.C) {
	request := Request{
		Attributes: map[string]string{"": "example"},
		Collection: "default",
		UID:        "1000",
	}

	err := validateRequest(request)

	c.Check(err, check.ErrorMatches, "secret request attribute name is empty")
}

// TestValidateRequestMissingAttributes checks a lookup without search
// attributes is rejected.
func (s *requestSuite) TestValidateRequestMissingAttributes(c *check.C) {
	request := Request{
		Collection: "default",
		UID:        "1000",
	}

	err := validateRequest(request)

	c.Check(err, check.ErrorMatches, "secret request attributes are missing")
}

// TestValidateRequestMissingCollection checks a lookup without a collection
// is rejected.
func (s *requestSuite) TestValidateRequestMissingCollection(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example"},
		UID:        "1000",
	}

	err := validateRequest(request)

	c.Check(err, check.ErrorMatches, "secret request collection is missing")
}
