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

package builtin_test

import (
	"context"
	"os/user"

	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type virtualizationSuite struct {
	iface     interfaces.Interface
	projectId string

	restoreUserLookup func()
	restoreUserEnv    func()
	restoreLxdInfo    func()
}

var _ = check.Suite(&virtualizationSuite{
	iface: builtin.MustInterface("virtualization"),
})

func (s *virtualizationSuite) SetUpSuite(c *check.C) {
	s.projectId = "42424242"
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return &testuser, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})
	s.restoreLxdInfo = lxd_device.MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
}

func (s *virtualizationSuite) TearDownSuite(c *check.C) {
	s.restoreLxdInfo()
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *virtualizationSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "virtualization")
}

func (s *virtualizationSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *virtualizationSuite) TestVirtualizationInterface(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
 virtualization:
  interface: virtualization
`, s.projectId, "ws", "consumer", "virtualization")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
 virtualization:
`, s.projectId, "ws", "producer", "virtualization")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)

	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)

	// Validate the device specification.
	expectedDevice := &workshop.Virtualization{Name: plug.Name}
	c.Assert(deviceSpec.Profile.Virtualization, check.DeepEquals, expectedDevice)
}

func (s *virtualizationSuite) TestSanitizePlugUnknownAttribute(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  virtualization:
    interface: virtualization
    subsystem: kvm
`, s.projectId, "ws", "consumer", "virtualization")
	err := interfaces.BeforePreparePlug(s.iface, plug)
	c.Check(err, check.ErrorMatches, `unknown attribute for virtualization interface plug: "subsystem"`)
}
