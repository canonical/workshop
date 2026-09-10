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

type sshAgentSuite struct {
	iface     interfaces.Interface
	projectId string

	restoreUserLookup func()
	restoreUserEnv    func()
	restoreLxdInfo    func()
}

var _ = check.Suite(&sshAgentSuite{
	iface: builtin.MustInterface("ssh-agent"),
})

func (s *sshAgentSuite) SetUpTest(c *check.C) {
	s.projectId = "42424242"
	testuser.HomeDir = c.MkDir()
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return &testuser, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return map[string]string{"SSH_AUTH_SOCK": "/tmp/dir/ssh"}, nil
	})
	s.restoreLxdInfo = lxd_device.MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
}

func (s *sshAgentSuite) TearDownTest(c *check.C) {
	s.restoreLxdInfo()
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *sshAgentSuite) TestName(c *check.C) {
	c.Assert(s.iface.Name(), check.Equals, "ssh-agent")
}

func (s *sshAgentSuite) TestInterfaces(c *check.C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}

func (s *sshAgentSuite) TestSshAgentInterface(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  ssh-agent:
    interface: ssh-agent
`, s.projectId, "ws", "consumer", "ssh-agent")
	connectedPlug := interfaces.NewConnectedPlug(plug, nil, nil)

	slot := builtin.MockSlot(c, `name: producer
base: ubuntu@22.04
slots:
  ssh-agent:
`, s.projectId, "ws", "producer", "ssh-agent")
	connectedSlot := interfaces.NewConnectedSlot(slot, nil, nil)
	deviceSpec, err := lxd_device.NewSpecification(testuser.Username, "consumer")
	c.Assert(err, check.IsNil)

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedProxy := &workshop.SshAgent{ProxyEntry: workshop.ProxyEntry{
		Name: plug.Name,
		Connect: workshop.ProxyTarget{
			Address:  "/tmp/dir/ssh",
			Protocol: "unix",
		},
		Listen: workshop.ProxyTarget{
			Address:  "/var/lib/workshop/run/ssh-agent.sock",
			Protocol: "unix",
		},
		Direction: workshop.WorkshopToHost,
	}}
	c.Assert(deviceSpec.Profile.Agent, check.DeepEquals, expectedProxy)
}

func (s *sshAgentSuite) TestSshAgentInterfaceRequiresContainer(c *check.C) {
	plug := builtin.MockPlug(c, `name: consumer
base: ubuntu@22.04
plugs:
  ssh-agent:
    interface: ssh-agent
`, s.projectId, "ws", "consumer", "ssh-agent")

	container := &lxd_device.Instance{Confinement: workshop.ConfinementContainer}
	c.Check(container.SupportsPlug(s.iface, plug), check.IsNil)

	vm := &lxd_device.Instance{Confinement: workshop.ConfinementVirtualMachine}
	c.Check(vm.SupportsPlug(s.iface, plug), check.ErrorMatches, `ssh-agent interface only available to containers`)
}
