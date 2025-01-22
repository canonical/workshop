//go:build integration
// +build integration

package lxd_device_test

import (
	"context"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	lxd "github.com/canonical/lxd/client"
	"github.com/spf13/afero"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/interfaces"
	backend "github.com/canonical/workshop/internal/interfaces/backends"
	"github.com/canonical/workshop/internal/interfaces/builtin"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/workshop/lxd/tests/helper"
)

type backendDeviceSuite struct {
	ctx      context.Context
	be       *lxdbackend.Backend
	client   lxd.InstanceServer
	repo     *interfaces.Repository
	username string
	userhome string
	pid      string

	lookupUserRestore func()
}

var _ = check.Suite(&backendDeviceSuite{})

func (f *backendDeviceSuite) setupRepo(c *check.C) {
	f.repo = interfaces.NewRepository()
	c.Assert(f.repo, check.NotNil)

	for _, iface := range builtin.Interfaces() {
		err := f.repo.AddInterface(iface)
		c.Assert(err, check.IsNil)
	}

	for _, backend := range backend.All() {
		err := backend.Initialize()
		c.Assert(err, check.IsNil)
		err = f.repo.AddBackend(backend)
		c.Assert(err, check.IsNil)
	}
}

func (f *backendDeviceSuite) readWorkshopFile(c *check.C, fname string) string {
	fs, err := f.be.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	file, err := fs.OpenFile(fname, os.O_CREATE|os.O_RDWR, 0744)
	c.Assert(err, check.IsNil)
	defer file.Close()
	buf, err := io.ReadAll(file)
	c.Assert(err, check.IsNil)
	return string(buf)
}

func defaultTestDevices() map[string]map[string]string {
	cwd, _ := os.Getwd()
	return map[string]map[string]string{
		"root":             {"type": "disk", "pool": "default", "path": "/"},
		"project":          {"type": "disk", "source": cwd, "path": "/project"},
		"workshop.network": {"type": "nic", "network": "lxdbr0", "name": "eth0"},
	}
}

func (f *backendDeviceSuite) SetUpTest(c *check.C) {
	var err error
	f.username = "testuser"
	f.userhome = c.MkDir()
	f.pid = "42424242"
	f.ctx = helper.CreateTestContext(f.username, "42424242")

	f.be, err = lxdbackend.New()
	c.Assert(err, check.IsNil)
	f.client, err = f.be.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)

	f.setupRepo(c)

	f.lookupUserRestore = testutil.FakeFunc(func(name string) (*user.User, error) {
		u := &user.User{
			Name:     f.username,
			Username: f.username,
			Uid:      "1000",
			Gid:      "1000",
			HomeDir:  f.userhome,
		}
		return u, nil
	}, &workshop.LookupUsername)

	defer lxdbackend.FakeDefaultDevices(defaultTestDevices)()
	helper.LaunchTestWorkshop(c, f.ctx, f.be, c.MkDir())
}

func (f *backendDeviceSuite) TearDownTest(c *check.C) {
	helper.CleanupLxdProject(c, f.client, lxdbackend.LxdProjectName(f.username))
	helper.CleanupLxdProject(c, f.client, lxdbackend.LxdSystemProjectName(f.username))
	f.lookupUserRestore()
	f.client.Disconnect()
}

func TestWorkshopBackendLxdDevice(t *testing.T) { check.TestingT(t) }

var consumer = []byte(`name: consumer
base: ubuntu@24.04
plugs:
    one:
        interface: mount
        workshop-target: /opt
    two:
        interface: mount
        workshop-target: /mnt
    ssh-agent:
        interface: ssh-agent
`)

var producer = []byte(`name: producer
base: ubuntu@24.04
type: system
slots:
    slot:
        interface: mount
        workshop-source: /usr/local
    home:
        interface: mount
        workshop-source: /home
    ssh-agent:
        interface: ssh-agent
`)

var producer2 = []byte(`name: producer2
base: ubuntu@24.04
slots:
    slot:
        interface: mount
`)

func (f *backendDeviceSuite) TestSetupWorkshopMounts(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	pinfo, err := sdk.ReadSdkInfo(producer, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(pinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: interfaces.NewPlugRef(cinfo.Plugs["one"]),
		SlotRef: interfaces.NewSlotRef(pinfo.Slots["slot"]),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	connref = &interfaces.ConnRef{
		PlugRef: interfaces.NewPlugRef(cinfo.Plugs["two"]),
		SlotRef: interfaces.NewSlotRef(pinfo.Slots["home"]),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	// Check the LXD profile correctness
	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Mounts, check.HasLen, 2)
	c.Check(prof.Mounts["one"].Name, check.Equals, "one")
	c.Check(prof.Mounts["one"].What, check.Equals, "/usr/local")
	c.Check(prof.Mounts["one"].Where, check.Equals, "/opt")
	c.Check(prof.Mounts["one"].Type, check.Equals, workshop.WorkshopWorkshop)
	c.Check(prof.Mounts["two"].Name, check.Equals, "two")
	c.Check(prof.Mounts["two"].What, check.Equals, "/home")
	c.Check(prof.Mounts["two"].Where, check.Equals, "/mnt")
	c.Check(prof.Mounts["two"].Type, check.Equals, workshop.WorkshopWorkshop)

	// Check /etc/fstab and mount
	fstab := f.readWorkshopFile(c, "/etc/fstab")
	lines := strings.Split(string(fstab), "\n")
	c.Check(lines, check.HasLen, 3)
	c.Check(lines[2], check.Equals, "")
	c.Check(lines, testutil.DeepUnsortedMatches, []string{
		"/usr/local /opt none bind,x-systemd.requires=/project 0 0",
		"/home /mnt none bind,x-systemd.requires=/project 0 0",
		"",
	})

	fs, err := f.be.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	// Check that the bind mount is created for /usr/local -> /opt
	file, err := fs.Create("/opt/tmp")
	c.Assert(err, check.IsNil)
	file.Close()
	_, err = fs.Stat("/usr/local/tmp")
	c.Assert(err, check.IsNil)

	// Check that the bind mount is created for /home -> /mnt
	file, err = fs.Create("/home/tmp")
	c.Assert(err, check.IsNil)
	file.Close()
	_, err = fs.Stat("/mnt/tmp")
	c.Assert(err, check.IsNil)

	// Check the LXD profile is removed
	err = b.Remove(f.ctx, "test", "consumer")
	c.Assert(err, check.IsNil)
	_, err = lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, testutil.ErrorIs, workshop.ErrSdkProfileNotFound)

	// Check the fstab record was removed
	fstab = f.readWorkshopFile(c, "/etc/fstab")
	c.Check(string(fstab), check.Equals, "")

	// Check that the bind mount is removed for /usr/local -> /opt
	_, err = fs.Stat("/opt/tmp")
	c.Assert(err, check.Equals, afero.ErrFileNotFound)

	// Check that the bind mount is removed for /home -> /mnt
	_, err = fs.Stat("/mnt/tmp")
	c.Assert(err, check.Equals, afero.ErrFileNotFound)
}

func (f *backendDeviceSuite) TestSetupHostWorkshopMounts(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	pinfo, err := sdk.ReadSdkInfo(producer2, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(pinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: interfaces.NewPlugRef(cinfo.Plugs["one"]),
		SlotRef: interfaces.NewSlotRef(pinfo.Slots["slot"]),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Mounts, check.HasLen, 1)
	c.Check(prof.Mounts["one"].Name, check.Equals, "one")
	c.Check(prof.Mounts["one"].What, check.Equals, filepath.Join(f.userhome, ".local/share/workshop/project/42424242/mount/test_consumer_one.sdk"))
	c.Check(prof.Mounts["one"].Where, check.Equals, "/opt")
	c.Check(prof.Mounts["one"].Type, check.Equals, workshop.HostWorkshop)
}

func (f *backendDeviceSuite) TestSetupUpdateProfile(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	pinfo, err := sdk.ReadSdkInfo(producer, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(pinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: interfaces.NewPlugRef(cinfo.Plugs["one"]),
		SlotRef: interfaces.NewSlotRef(pinfo.Slots["slot"]),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	// Setup a new profile.
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	f.repo.DisconnectAll([]*interfaces.ConnRef{connref})

	// Update profile.
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	// Check the LXD profile correctness
	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Mounts, check.HasLen, 0)

	// Check the fstab record was removed
	fstab := f.readWorkshopFile(c, "/etc/fstab")
	c.Check(string(fstab), check.Equals, "")
}

func (f *backendDeviceSuite) TestSetupSshAgent(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()
	old := dirs.WorkshopRunDir
	dirs.WorkshopRunDir = "/run"
	defer func() {
		dirs.WorkshopRunDir = old
	}()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	pinfo, err := sdk.ReadSdkInfo(producer, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(pinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: interfaces.NewPlugRef(cinfo.Plugs["ssh-agent"]),
		SlotRef: interfaces.NewSlotRef(pinfo.Slots["ssh-agent"]),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	systemdCmd := testutil.FakeCommand(c, "sudo", `
echo SSH_AUTH_SOCK=/run/.workshop.socket
exit 0
`)
	defer systemdCmd.Restore()

	// Setup profile.
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Agent, check.NotNil)
	c.Check(prof.Agent.Name, check.Equals, "consumer-ssh-agent")
	c.Check(prof.Agent.Connect, check.Equals, "unix:/run/.workshop.socket")
	c.Check(prof.Agent.Listen, check.Equals, "unix:/run/consumer-ssh-agent.ssh")

	buf := f.readWorkshopFile(c, "/etc/profile.d/consumer-ssh-agent.sh")
	c.Check(buf, check.Equals, "export SSH_AUTH_SOCK=/run/consumer-ssh-agent.ssh\n")

	f.repo.DisconnectAll([]*interfaces.ConnRef{connref})

	// Update profile (ssh-agent must be removed as it was disconnected).
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	fs, err := f.be.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	_, err = fs.Stat("/etc/profile.d/consumer-ssh-agent.sh")
	c.Assert(osutil.IsDirNotExist(err), check.Equals, true)
}
