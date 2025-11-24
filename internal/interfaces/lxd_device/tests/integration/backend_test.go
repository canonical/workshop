//go:build integration

package lxd_device_test

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	lxd "github.com/canonical/lxd/client"
	"github.com/pkg/sftp"
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
	ctx          context.Context
	be           *lxdbackend.Backend
	client       lxd.InstanceServer
	repo         *interfaces.Repository
	usr          *user.User
	pid          string
	restoreUser  func()
	restoreEnv   func()
	restoreNewId func()
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
	defer fs.Close()
	buf, err := fs.ReadFile(fname)
	c.Assert(err, check.IsNil)
	return string(buf)
}

func defaultTestDevices(pid, w string) ([]workshop.Mount, []workshop.ProxyEntry) {
	cwd, _ := os.Getwd()
	mounts := []workshop.Mount{{
		Name:  workshop.ConfigProjectPathDevice,
		Type:  workshop.HostWorkshop,
		What:  cwd,
		Where: workshop.WorkshopProjectPath,
	}}
	return mounts, nil
}

func (f *backendDeviceSuite) SetUpTest(c *check.C) {
	dirs.SetRootDir(c.MkDir())
	dirs.SetCacheDir(c.MkDir())
	c.Assert(dirs.CreateDirs(), check.IsNil)

	var err error
	f.pid = "42424242"
	f.usr = &user.User{
		Username: "testuser",
		Uid:      "1000",
		Gid:      "1000",
		HomeDir:  c.MkDir(),
	}

	f.restoreUser = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return f.usr, nil
	})

	f.restoreEnv = osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return nil, nil
	})
	f.restoreNewId = testutil.FakeFunc(func() (string, error) {
		return f.pid, nil
	}, &workshop.NewProjectId)

	f.ctx = helper.CreateTestContext(f.usr.Username, "42424242")

	f.be, err = lxdbackend.New()
	c.Assert(err, check.IsNil)
	f.client, err = f.be.LxdClient(f.ctx)
	c.Assert(err, check.IsNil)

	f.setupRepo(c)

	defer workshop.FakeDefaultDevices(defaultTestDevices)()
	helper.LaunchTestWorkshop(c, f.ctx, f.be, c.MkDir())
}

func (f *backendDeviceSuite) TearDownTest(c *check.C) {
	helper.RemoveTestWorkshop(c, f.ctx, f.be)
	helper.CleanupLxdProject(c, f.client, "workshop."+f.usr.Username)
	helper.CleanupLxdProject(c, f.client, "workshop-layers."+f.usr.Username)
	f.restoreNewId()
	f.restoreEnv()
	f.restoreUser()
	f.client.Disconnect()
}

func TestWorkshopBackendLxdDevice(t *testing.T) { check.TestingT(t) }

var consumer = []byte(`name: consumer
base: ubuntu@24.04
plugs:
    one:
        interface: mount
        workshop-target: /opt
        mode: 0
        uid: 0
        gid: 0
        read-only: false
    two:
        interface: mount
        workshop-target: /mnt/a/b
        mode: 0o777
        uid: 0
        gid: 100
        read-only: false
    ssh-agent:
        interface: ssh-agent
    desktop:
        interface: desktop
`)

var system = []byte(`name: system
base: ubuntu@24.04
type: system
slots:
    mount:
        interface: mount
    ssh-agent:
        interface: ssh-agent
    desktop:
        interface: desktop
`)

var producer = []byte(`name: producer
base: ubuntu@24.04
slots:
    slot:
        interface: mount
        workshop-source: /usr/local/workshop-source
    etc:
        interface: mount
        workshop-source: /etc/config-file
`)

func (f *backendDeviceSuite) TestSetupWorkshopMounts(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	fs, err := f.be.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer fs.Close()
	err = fs.WriteFile("/etc/config-file", nil, 0o644)
	c.Assert(err, check.IsNil)

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	pinfo, err := sdk.ReadSdkInfo(producer, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(pinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["one"].Ref(),
		SlotRef: pinfo.Slots["slot"].Ref(),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	connref = &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["two"].Ref(),
		SlotRef: pinfo.Slots["etc"].Ref(),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.ErrorMatches, "stat /usr/local/workshop-source: file does not exist")

	err = fs.WriteFile("/usr/local/workshop-source", nil, 0o644)
	c.Assert(err, check.IsNil)
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.ErrorMatches, `mount /opt: is a directory`)

	err = fs.Remove("/usr/local/workshop-source")
	c.Assert(err, check.IsNil)
	err = fs.Mkdir("/usr/local/workshop-source", 0o755)
	c.Assert(err, check.IsNil)
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	// Check the LXD profile correctness
	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Mounts, check.HasLen, 2)
	one := workshop.Mount{
		Name:      "one",
		Type:      workshop.WorkshopWorkshop,
		What:      "/usr/local/workshop-source",
		Where:     "/opt",
		MakeWhere: true,
	}
	c.Check(prof.Mounts["one"], check.Equals, one)
	two := workshop.Mount{
		Name:      "two",
		Type:      workshop.WorkshopWorkshop,
		What:      "/etc/config-file",
		Where:     "/mnt/a/b",
		MakeWhere: true,
		Mode:      0o777,
		Group:     100,
	}
	c.Check(prof.Mounts["two"], check.Equals, two)

	// Check /etc/fstab and mount
	fstab := f.readWorkshopFile(c, "/etc/fstab")
	lines := strings.Split(string(fstab), "\n")
	c.Check(lines, check.HasLen, 3)
	c.Check(lines[2], check.Equals, "")
	c.Check(lines, testutil.DeepUnsortedMatches, []string{
		"/usr/local/workshop-source /opt none bind,x-systemd.requires=/project 0 0",
		"/etc/config-file /mnt/a/b none bind,x-systemd.requires=/project 0 0",
		"",
	})

	// Check that the bind mount is created for /usr/local/workshop-source -> /opt
	err = fs.WriteFile("/opt/tmp", nil, 0o644)
	c.Assert(err, check.IsNil)
	_, err = fs.Stat("/usr/local/workshop-source/tmp")
	c.Assert(err, check.IsNil)

	// Check that the bind mount is created for /etc/config-file -> /mnt/a/b
	file, err := fs.OpenFile("/etc/config-file", os.O_RDWR, 0o644)
	c.Assert(err, check.IsNil)
	_, err = file.Write([]byte("data"))
	file.Close()
	c.Assert(err, check.IsNil)
	info, err := fs.Stat("/mnt/a/b")
	c.Assert(err, check.IsNil)
	c.Check(info.Size(), check.Equals, int64(len("data")))

	// Check mode of created directory
	info, err = fs.Stat("/mnt/a")
	c.Assert(err, check.IsNil)
	c.Check(info.IsDir(), check.Equals, true)
	c.Check(info.Mode().Perm(), check.Equals, os.FileMode(0o777))
	stat, ok := info.Sys().(*sftp.FileStat)
	c.Assert(ok, check.Equals, true)
	c.Check(stat.UID, check.Equals, uint32(0))
	c.Check(stat.GID, check.Equals, uint32(100))

	// Check the LXD profile is removed
	err = b.Remove(f.ctx, sdk.Ref{ProjectId: f.pid, Workshop: "test", Sdk: "consumer"})
	c.Assert(err, check.IsNil)
	_, err = lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, testutil.ErrorIs, workshop.ErrSdkProfileNotFound)

	// Check the fstab record was removed
	fstab = f.readWorkshopFile(c, "/etc/fstab")
	c.Check(string(fstab), check.Equals, "")

	// Check that the bind mount is removed for /usr/local/workshop-source -> /opt
	_, err = fs.Stat("/opt/tmp")
	c.Assert(err, testutil.ErrorIs, os.ErrNotExist)

	// Check that the bind mount is removed for /etc/config-file -> /mnt/a/b
	info, err = fs.Stat("/mnt/a/b")
	c.Assert(err, check.IsNil)
	c.Check(info.Size(), check.Equals, int64(0))
}

func (f *backendDeviceSuite) TestSetupHostWorkshopMounts(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	sinfo, err := sdk.ReadSdkInfo(system, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(sinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["one"].Ref(),
		SlotRef: sinfo.Slots["mount"].Ref(),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	defer osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return map[string]string{"XDG_RUNTIME_DIR": c.MkDir()}, nil
	})()

	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Mounts, check.HasLen, 1)
	one := workshop.Mount{
		Name:      "one",
		Type:      workshop.HostWorkshop,
		What:      filepath.Join(f.usr.HomeDir, ".local/share/workshop/id/42424242/test/mount/consumer/one"),
		MakeWhat:  true,
		Where:     "/opt",
		MakeWhere: true,
	}
	c.Check(prof.Mounts["one"], check.Equals, one)
}

func (f *backendDeviceSuite) TestSetupUpdateProfile(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	sinfo, err := sdk.ReadSdkInfo(system, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(sinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["one"].Ref(),
		SlotRef: sinfo.Slots["mount"].Ref(),
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
}

func (f *backendDeviceSuite) TestSetupSshAgent(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()
	defer mockWorkshopRunDir()()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	sinfo, err := sdk.ReadSdkInfo(system, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(sinfo), check.IsNil)

	connref := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["ssh-agent"].Ref(),
		SlotRef: sinfo.Slots["ssh-agent"].Ref(),
	}

	_, err = f.repo.Connect(connref, nil, nil, nil, nil, nil)
	c.Assert(err, check.IsNil)

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	defer osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return map[string]string{"SSH_AUTH_SOCK": "/run/.workshop.socket"}, nil
	})()

	// Setup profile.
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
	c.Assert(err, check.IsNil)
	c.Assert(prof.Agent, check.NotNil)
	c.Check(prof.Agent.Name, check.Equals, "ssh-agent")
	c.Check(prof.Agent.Connect.Address, check.Equals, "/run/.workshop.socket")
	c.Check(prof.Agent.Connect.Protocol, check.Equals, "unix")
	c.Check(prof.Agent.Listen.Address, check.Equals, "/run/ssh-agent.sock")
	c.Check(prof.Agent.Listen.Protocol, check.Equals, "unix")
	c.Check(prof.Agent.Direction, check.Equals, workshop.WorkshopToHost)

	buf := f.readWorkshopFile(c, "/etc/profile.d/workshop-ssh-agent.sh")
	c.Check(buf, check.Equals, "export SSH_AUTH_SOCK=/run/ssh-agent.sock\n")

	f.repo.DisconnectAll([]*interfaces.ConnRef{connref})

	// Update profile (ssh-agent must be removed as it was disconnected).
	err = b.Setup(f.ctx, cref, f.repo)
	c.Assert(err, check.IsNil)

	fs, err := f.be.WorkshopFs(f.ctx, "test")
	c.Assert(err, check.IsNil)
	defer fs.Close()
	_, err = fs.Stat("/etc/profile.d/workshop-ssh-agent.sh")
	c.Assert(osutil.IsDirNotExist(err), check.Equals, true)
}

func (f *backendDeviceSuite) TestSetupMultipleInterfaces(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()
	defer mockWorkshopRunDir()()

	cinfo, err := sdk.ReadSdkInfo(consumer, f.pid, "test")
	c.Assert(err, check.IsNil)

	sinfo, err := sdk.ReadSdkInfo(system, f.pid, "test")
	c.Assert(err, check.IsNil)

	c.Assert(f.repo.AddSdk(cinfo), check.IsNil)
	c.Assert(f.repo.AddSdk(sinfo), check.IsNil)

	sshConnRef := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["ssh-agent"].Ref(),
		SlotRef: sinfo.Slots["ssh-agent"].Ref(),
	}

	desktopConnRef := &interfaces.ConnRef{
		PlugRef: cinfo.Plugs["desktop"].Ref(),
		SlotRef: sinfo.Slots["desktop"].Ref(),
	}

	b := lxd_device.Backend{}
	cref := sdk.Ref{ProjectId: "42424242", Workshop: "test", Sdk: "consumer"}

	defer osutil.FakeUserEnvironment(func(user *user.User) (map[string]string, error) {
		return map[string]string{
			"XDG_RUNTIME_DIR": "/tmp",
			"SSH_AUTH_SOCK":   "/run/.workshop.socket",
			"WAYLAND_DISPLAY": "1",
			"DISPLAY":         ":0",
		}, nil
	})()

	setupSshAgent := func() {
		_, err = f.repo.Connect(sshConnRef, nil, nil, nil, nil, nil)
		c.Assert(err, check.IsNil)
		err = b.Setup(f.ctx, cref, f.repo)
		c.Assert(err, check.IsNil)
	}

	setupDesktop := func() {
		_, err = f.repo.Connect(desktopConnRef, nil, nil, nil, nil, nil)
		c.Assert(err, check.IsNil)
		err = b.Setup(f.ctx, cref, f.repo)
		c.Assert(err, check.IsNil)
	}

	validateAndDisconnect := func() {
		// Validate Profile
		prof, err := lxdbackend.Profile(f.client, f.pid, "test", "consumer")
		c.Assert(err, check.IsNil)

		c.Assert(prof.Agent, check.NotNil)
		c.Assert(prof.Desktop, check.NotNil)

		// Validate Filesystem
		fs, err := f.be.WorkshopFs(f.ctx, "test")
		c.Assert(err, check.IsNil)
		defer fs.Close()
		_, err = fs.Stat("/etc/profile.d/workshop-ssh-agent.sh")
		c.Assert(err, check.IsNil)
		_, err = fs.Stat("/etc/profile.d/workshop-desktop.sh")
		c.Assert(err, check.IsNil)

		// Disconnect and setup
		f.repo.DisconnectAll([]*interfaces.ConnRef{sshConnRef, desktopConnRef})
		err = b.Setup(f.ctx, cref, f.repo)
		c.Assert(err, check.IsNil)
		_, err = fs.Stat("/etc/profile.d/workshop-ssh-agent.sh")
		c.Assert(err, check.NotNil)
		_, err = fs.Stat("/etc/profile.d/workshop-desktop.sh")
		c.Assert(err, check.NotNil)
	}

	setupSshAgent()
	setupDesktop()
	validateAndDisconnect()

	// Repeat with interfaces in reverse order
	setupDesktop()
	setupSshAgent()
	validateAndDisconnect()
}

func mockWorkshopRunDir() (restore func()) {
	old := dirs.WorkshopRunDir
	dirs.WorkshopRunDir = "/run"
	return func() {
		dirs.WorkshopRunDir = old
	}
}
