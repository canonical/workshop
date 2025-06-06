//go:build integration
// +build integration

package lxdbackend_integration_test

import (
	"bytes"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	lxd "github.com/canonical/lxd/client"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/daemon"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
	"github.com/canonical/workshop/internal/workshop/lxd/tests/helper"
)

type wsExec struct {
	lxdClient           lxd.InstanceServer
	be                  workshop.Backend
	ctx                 context.Context
	usr                 *user.User
	client              *client.Client
	daemon              *daemon.Daemon
	project             workshop.Project
	lookupUserRestore   func()
	lookupUserIdRestore func()
	newProjectidRestore func()
	restoreImageServer  func()
	restoreDevices      func()
}

var _ = check.Suite(&wsExec{})

func execTestDevices(projectDir string) func(pid, w string) ([]workshop.Mount, []workshop.ProxyEntry) {
	mounts := []workshop.Mount{{
		Name:  workshop.ConfigProjectPathDevice,
		What:  projectDir,
		Where: workshop.WorkshopProjectPath,
		Type:  workshop.HostWorkshop,
	}}
	return func(pid, w string) ([]workshop.Mount, []workshop.ProxyEntry) {
		return mounts, nil
	}
}

func (f *wsExec) SetUpSuite(c *check.C) {
	f.restoreImageServer = lxdbackend.FakeImageServer(helper.MinimalImageServer)

	socketPath := c.MkDir() + ".workshop.socket"
	var err error
	f.be, err = lxdbackend.New()
	c.Assert(err, check.IsNil)

	d, err := daemon.New(&daemon.Options{
		Dir:        c.MkDir(),
		SocketPath: socketPath,
	})
	c.Assert(err, check.IsNil)
	err = d.Init()
	c.Assert(err, check.IsNil)
	d.Start()
	f.daemon = d

	c.Check(err, check.IsNil)
	f.client, err = client.New(&client.Config{
		Socket: socketPath,
	})
	c.Assert(err, check.IsNil)

	f.project = workshop.Project{
		ProjectId: "42424242",
		Path:      c.MkDir(),
	}

	f.usr = &user.User{
		Username: "testuser",
		Uid:      "1000",
		Gid:      "1000",
	}

	f.ctx = helper.CreateTestContext(f.usr.Username, f.project.ProjectId)

	f.lxdClient, err = f.be.(*lxdbackend.Backend).LxdClient(f.ctx)
	c.Check(err, check.IsNil)

	f.lookupUserRestore = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return f.usr, nil
	})

	f.lookupUserIdRestore = testutil.FakeFunc(func(uid string) (*user.User, error) {
		return f.usr, nil
	}, &daemon.LookupUserId)

	f.restoreDevices = workshop.FakeDefaultDevices(execTestDevices(c.MkDir()))

	f.newProjectidRestore = testutil.FakeFunc(func() (string, error) {
		return f.project.ProjectId, nil
	}, &workshop.NewProjectId)

	helper.LaunchTestWorkshop(c, f.ctx, f.be, f.project.Path)
}

func (f *wsExec) TearDownSuite(c *check.C) {
	helper.RemoveTestWorkshop(c, f.ctx, f.be)
	err := f.daemon.Stop(nil)
	c.Check(err, check.IsNil)
	f.lookupUserRestore()
	f.lookupUserIdRestore()
	f.newProjectidRestore()
	f.restoreImageServer()
	f.restoreDevices()
	helper.CleanupLxdProject(c, f.lxdClient, "workshop."+f.usr.Username)
	helper.CleanupLxdProject(c, f.lxdClient, "workshop-stash."+f.usr.Username)
}

func (f *wsExec) exec(stdin string, workshop, projectId string, opts *client.ExecOptions) (stdout, stderr string, waitErr error) {
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	opts.Stdin = strings.NewReader(stdin)
	opts.Stdout = outBuf
	opts.Stderr = errBuf
	process, err := f.client.Exec(opts, workshop, projectId)
	if err != nil {
		return "", "", err
	}
	waitErr = process.Wait()
	return outBuf.String(), errBuf.String(), waitErr
}

func (f *wsExec) TestLxdBackendExecTrivial(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"ls"},
		WorkingDir: "/",
	}
	_, _, err := f.exec("", "test", f.project.ProjectId, opts)
	c.Assert(err, check.IsNil)
}

func (f *wsExec) TestLxdBackendExecScript(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"info", "-arg"},
		Script:     true,
		WorkingDir: "/",
	}
	stdout, _, err := f.exec("", "test", f.project.ProjectId, opts)
	c.Assert(err, check.IsNil)
	c.Assert(stdout, check.Equals, "/\nworkshop\n-arg\n")
}

func (f *wsExec) TestLxdBackendExecMissingScript(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"missing"},
		Script:     true,
		WorkingDir: "/",
	}
	_, _, err := f.exec("", "test", f.project.ProjectId, opts)
	c.Assert(err, check.ErrorMatches, `(?s)cannot perform the following tasks:.*Install script "missing" \(script not found\)`)
}

func (f *wsExec) TestLxdBackendExecCannotReadScript(c *check.C) {
	otherFile := filepath.Join(f.project.Path, ".workshop.yaml")
	_, err := os.Create(otherFile)
	c.Assert(err, check.IsNil)
	defer os.Remove(otherFile)

	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"info", "-arg"},
		Script:     true,
		WorkingDir: "/",
	}
	_, _, err = f.exec("", "test", f.project.ProjectId, opts)
	c.Assert(err, check.ErrorMatches, `(?s)cannot perform the following tasks:.*Install script "info" \(multiple workshops found.*\)`)
}

func (f *wsExec) TestLxdBackendExecWorkingDirectoryDoesNotExist(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"ls"},
		WorkingDir: "/no/such/dir",
	}

	// Exec
	_, _, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.ErrorMatches, `cannot exec command in "test": working directory "/no/such/dir" not found`)
}

func (f *wsExec) TestLxdBackendExecDefaultUserGroup(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"/bin/sh", "-c", "id -n -u && id -n -g"},
		WorkingDir: "/",
	}

	// Exec
	stdout, stderr, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(stdout, check.Equals, "workshop\nworkshop\n")
	c.Assert(stderr, check.Equals, "")
}

func (f *wsExec) TestLxdBackendExecCustomUserGroup(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"/bin/sh", "-c", "id -n -u && id -n -g"},
		WorkingDir: "/",
		UserId:     new(int),
		GroupId:    new(int),
	}

	// Exec
	stdout, stderr, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(stdout, check.Equals, "root\nroot\n")
	c.Assert(stderr, check.Equals, "")
}

func (f *wsExec) TestLxdBackendExecAddEnvVar(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:     []string{"/bin/sh", "-c", "printenv"},
		WorkingDir:  "/",
		Environment: map[string]string{"FOO": "BAR"},
	}

	// Exec
	stdout, stderr, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.IsNil)
	raw := strings.FieldsFunc(stdout, func(r rune) bool { return r == '\n' })
	env, err := osutil.ParseEnvironment(raw)
	c.Check(err, check.IsNil)
	c.Check(env["USER"], check.Equals, "workshop")
	c.Check(env["HOME"], check.Equals, "/home/workshop")
	c.Check(env["PATH"], check.Equals, "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin")
	c.Check(env["FOO"], check.Equals, "BAR")
	c.Check(env["LANG"], check.Equals, "C.UTF-8")
	c.Check(env["PWD"], check.Equals, "/")
	c.Assert(stderr, check.Equals, "")
}

func (f *wsExec) TestLxdBackendExecNoninteractive(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"/bin/sh", "-c", "echo -n STDOUT; echo -n STDERR >&2; exit 42"},
		WorkingDir: "/",
		UserId:     new(int),
		GroupId:    new(int),
	}

	// Exec
	stdout, stderr, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	var exitCode int
	if exitError, ok := err.(*client.ExitError); ok {
		exitCode = exitError.ExitCode()
	}
	c.Check(exitCode, check.Equals, 42)
	c.Assert(stdout, check.Equals, "STDOUT")
	c.Assert(stderr, check.Equals, "STDERR")
}

func (f *wsExec) TestLxdBackendExecTimeout(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"/bin/bash", "-c", "sleep 5"},
		WorkingDir: "/",
		UserId:     new(int),
		GroupId:    new(int),
		Timeout:    100 * time.Millisecond,
	}

	// Exec
	_, _, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.ErrorMatches, "(?s).*timed out after 100ms.*")
}

func (f *wsExec) TestLxdBackendExecValidateCloudInitConfig(c *check.C) {
	// Setup
	opts := &client.ExecOptions{
		Command:    []string{"cloud-init", "schema", "--system", "--annotate"},
		WorkingDir: "/",
		UserId:     new(int),
		GroupId:    new(int),
	}

	// Exec
	stdout, stderr, err := f.exec("", "test", f.project.ProjectId, opts)

	// Validate
	c.Assert(err, check.IsNil)
	c.Assert(stderr, check.Equals, "")
	c.Assert(strings.Contains(stdout, "Valid schema user-data"), check.Equals, true)
	c.Assert(strings.Contains(stdout, "Error"), check.Equals, false)
}
