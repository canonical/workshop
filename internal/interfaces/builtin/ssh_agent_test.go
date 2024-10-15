package builtin_test

import (
	"os/user"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	"gopkg.in/check.v1"
)

type sshAgentSuite struct {
	iface     interfaces.Interface
	projectId string
	restore   func()
}

var _ = check.Suite(&sshAgentSuite{
	iface: builtin.MustInterface("ssh-agent"),
})

func (s *sshAgentSuite) SetUpTest(c *check.C) {
	s.projectId = "42424242"
	usr, err := user.Current()
	c.Assert(err, check.IsNil)
	homeDir := c.MkDir()
	s.restore = testutil.FakeFunc(func(name string) (*user.User, error) {
		u := &user.User{
			Name:     usr.Name,
			Username: usr.Name,
			Uid:      usr.Uid,
			Gid:      usr.Gid,
			HomeDir:  homeDir,
		}
		return u, nil
	}, &workshop.LookupUsername)
}

func (s *sshAgentSuite) TearDown(c *check.C) {
	s.restore()
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
	deviceSpec := lxd_device.NewSpecification("testuser", s.projectId, "consumer")

	fake := testutil.FakeCommand(c, "sudo", `
echo "SSH_AUTH_SOCK=/tmp/dir/ssh"
exit 0`)
	defer fake.Restore()

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.IsNil)
	expectedProxy := &workshop.Proxy{Name: "consumer-" + plug.Name, Connect: "/tmp/dir/ssh", Listen: "/var/lib/workshop/run/consumer-ssh-agent.ssh"}
	// We cannot compare the environment characteristics, these are not part of the lxd profile however are used by the install hook
	proxySpec := deviceSpec.Profile.Proxies["consumer-ssh-agent"]
	proxySpec.Env = nil
	c.Assert(proxySpec, check.DeepEquals, *expectedProxy)
}

func (s *sshAgentSuite) TestSshAgentInterfaceSystemctlFails(c *check.C) {
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
	deviceSpec := lxd_device.NewSpecification("testuser", s.projectId, "consumer")

	fake := testutil.FakeCommand(c, "sudo", `
>&2 echo -n "No medium found"
exit 1`)
	defer fake.Restore()

	c.Assert(deviceSpec.AddConnectedPlug(s.iface, connectedPlug, connectedSlot), check.ErrorMatches, `No medium found`)
}
