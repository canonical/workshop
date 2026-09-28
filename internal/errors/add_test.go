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

package errors_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"gopkg.in/check.v1"

	workshoperrors "github.com/canonical/workshop/internal/errors"
)

// addSuite checks error matching without exposing added diagnostics.
type addSuite struct{}

var _ = check.Suite(&addSuite{})

func Test(t *testing.T) {
	check.TestingT(t)
}

// TestAddAs checks that types in a wrapped added error remain discoverable.
func (addSuite) TestAddAs(c *check.C) {
	added := &fs.PathError{
		Op: "open", Path: "/secret", Err: fs.ErrPermission,
	}
	err := workshoperrors.Add(
		errors.New("cannot retrieve secret"),
		fmt.Errorf("provider failed: %w", added),
	)

	got, ok := errors.AsType[*fs.PathError](err)

	c.Check(ok, check.Equals, true)
	c.Check(got, check.Equals, added)
}

// TestAddAsAddedFirst checks that the added error takes precedence over
// unwrapping when both errors contain the requested type.
func (addSuite) TestAddAsAddedFirst(c *check.C) {
	primary := &fs.PathError{
		Op: "open", Path: "/primary", Err: fs.ErrPermission,
	}
	added := &fs.PathError{
		Op: "open", Path: "/added", Err: fs.ErrNotExist,
	}
	err := workshoperrors.Add(primary, added)

	got, ok := errors.AsType[*fs.PathError](err)

	c.Check(ok, check.Equals, true)
	c.Check(got, check.Equals, added)
}

// TestAddAsPrimary checks that type matching falls back to the primary
// error when the added error does not contain the requested type.
func (addSuite) TestAddAsPrimary(c *check.C) {
	primary := &fs.PathError{
		Op: "open", Path: "/primary", Err: fs.ErrPermission,
	}
	err := workshoperrors.Add(primary, errors.New("provider locked"))

	got, ok := errors.AsType[*fs.PathError](err)

	c.Check(ok, check.Equals, true)
	c.Check(got, check.Equals, primary)
}

// TestAddIs checks that both error chains remain matchable through wrapping.
func (addSuite) TestAddIs(c *check.C) {
	primary := errors.New("cannot retrieve secret")
	added := errors.New("provider locked")
	err := workshoperrors.Add(primary, fmt.Errorf("lookup: %w", added))
	wrapped := fmt.Errorf("request failed: %w", err)

	c.Check(errors.Is(wrapped, primary), check.Equals, true)
	c.Check(errors.Is(wrapped, added), check.Equals, true)
}

// TestAddMessage checks that added diagnostics do not affect the message.
func (addSuite) TestAddMessage(c *check.C) {
	err := workshoperrors.Add(
		errors.New("cannot retrieve secret"),
		errors.New("internal provider details"),
	)

	c.Check(err.Error(), check.Equals, "cannot retrieve secret")
}

// TestAddNilAdded checks that a nil added error preserves the primary
// error's message and matching behaviour.
func (addSuite) TestAddNilAdded(c *check.C) {
	primary := errors.New("cannot retrieve secret")
	err := workshoperrors.Add(primary, nil)

	c.Check(err.Error(), check.Equals, primary.Error())
	c.Check(errors.Is(err, primary), check.Equals, true)
}

// TestAddNilPrimary checks that adding context cannot turn success into
// failure when no primary error exists.
func (addSuite) TestAddNilPrimary(c *check.C) {
	c.Check(workshoperrors.Add(nil, errors.New("provider locked")), check.IsNil)
}

// TestAddUnwrap checks that standard unwrapping returns the original error.
func (addSuite) TestAddUnwrap(c *check.C) {
	primary := errors.New("cannot retrieve secret")
	err := workshoperrors.Add(primary, errors.New("provider locked"))

	c.Check(errors.Unwrap(err), check.Equals, primary)
}
