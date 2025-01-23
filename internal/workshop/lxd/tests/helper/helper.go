//go:build integration
// +build integration

package helper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/workshop"
)

var testYaml = `name: test
base: ubuntu@22.04
scripts:
  info: |
    pwd
    whoami
    printf '%s\n' "$@"
`

var MinimalImageServer = "simplestreams:https://cloud-images.ubuntu.com/minimal/releases/"

func DefaultTestDevices() map[string]map[string]string {
	return map[string]map[string]string{
		"root":             {"type": "disk", "pool": "default", "path": "/"},
		"workshop.network": {"type": "nic", "network": "lxdbr0", "name": "eth0"},
	}
}

func CleanupLxdProject(c *check.C, client lxd.InstanceServer, project string) {
	cli := client.UseProject(project)
	fingers, err := cli.GetImageFingerprints()
	c.Check(err, check.IsNil)
	for _, i := range fingers {
		op, err := cli.DeleteImage(i)
		c.Check(err, check.IsNil)
		if err == nil {
			c.Check(op.Wait(), check.IsNil)
		}
	}

	instances, err := cli.GetInstances(api.InstanceType("container"))
	c.Check(err, check.IsNil)
	for _, i := range instances {
		if i.Status == "Running" {
			req := api.InstanceStatePut{
				Action:  "stop",
				Timeout: 1,
				Force:   true,
			}

			op, err := cli.UpdateInstanceState(i.Name, req, "")
			c.Check(err, check.IsNil)
			if err == nil {
				c.Check(op.Wait(), check.IsNil)
			}
		}

		op, err := cli.DeleteInstance(i.Name)
		c.Check(err, check.IsNil)
		if err == nil {
			c.Check(op.Wait(), check.IsNil)
		}
	}

	profiles, err := cli.GetProfileNames()
	c.Check(err, check.IsNil)
	for _, p := range profiles {
		if p == "default" {
			continue
		}
		err := cli.DeleteProfile(p)
		c.Check(err, check.IsNil)
	}

	err = cli.DeleteProject(project)
	c.Check(err, check.IsNil)
}

func CreateTestContext(username, projectId string) context.Context {
	ctx := context.WithValue(context.Background(), workshop.ContextUser, username)
	ctx = context.WithValue(ctx, workshop.ContextProjectId, projectId)
	return ctx
}

func LaunchTestWorkshop(c *check.C, ctx context.Context, bd workshop.Backend, dir string) {
	var err error

	err = bd.Download(ctx, "ubuntu@24.04", nil)
	c.Assert(err, check.IsNil)

	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@24.04",
		Scripts: map[string]workshop.Script{"info": `



pwd
whoami
printf '%s\n' "$@"
`},
	}

	path := workshop.Filepath(dir, "test")

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(testYaml), 0644)
	c.Assert(err, check.IsNil)

	prj, _, err := bd.CreateOrLoadProject(ctx, dir)
	c.Assert(err, check.IsNil)

	err = bd.LaunchWorkshop(ctx, wf)
	c.Assert(err, check.IsNil)

	volume := workshop.AptCacheVolumeName(wf.Name, prj.ProjectId)
	err = bd.CreateVolume(ctx, volume)
	c.Assert(err, check.IsNil)
	err = bd.AttachVolume(ctx, wf.Name, volume, dirs.AptCachePath)
	c.Assert(err, check.IsNil)

	err = bd.StartWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)
}

func RemoveTestWorkshop(c *check.C, ctx context.Context, bd workshop.Backend) {
	err := bd.RemoveWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)

	RemoveTestVolume(c, ctx, bd)
}

func RemoveTestVolume(c *check.C, ctx context.Context, bd workshop.Backend) {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	c.Assert(ok, check.Equals, true)

	volume := workshop.AptCacheVolumeName("test", projectId)
	err := bd.DeleteVolume(ctx, volume)
	c.Assert(err, check.IsNil)
}

func MockSdkTarball(c *check.C, name, path, meta string) string {
	sdkfs := filepath.Join(path, name)

	metadir := filepath.Join(sdkfs, "meta")
	err := os.MkdirAll(metadir, 0755)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(meta), 0644)
	c.Assert(err, check.IsNil)

	hooksdir := filepath.Join(sdkfs, "sdk", "hooks")
	err = os.MkdirAll(hooksdir, 0755)
	c.Assert(err, check.IsNil)

	tarball := filepath.Join(path, fmt.Sprintf("%s_1.sdk", name))
	pack := exec.CommandContext(context.Background(), "tar",
		"--strip-components=1",
		"--remove-files",
		"--create",
		"--file",
		tarball,
		"--directory="+sdkfs,
		"--no-same-owner",
		".",
	)
	output, err := pack.CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", string(output)))

	return tarball
}
