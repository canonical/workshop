//go:build integration

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

	"github.com/canonical/workshop/internal/workshop"
)

var testYaml = `name: test
base: ubuntu@22.04
actions:
  info: |
    pwd
    whoami
    printf '%s\n' "$@"
`

var MinimalImageServer = "simplestreams:https://cloud-images.ubuntu.com/minimal/releases"

func DefaultTestDevices(pid, w string) ([]workshop.Mount, []workshop.ProxyEntry) {
	return nil, nil
}

func CleanupLxdProject(c *check.C, client lxd.InstanceServer, project string) {
	cli := client.UseProject(project)
	fingers, err := cli.GetImageFingerprints()
	c.Check(err, check.IsNil)
	for _, i := range fingers {
		op, err := cli.DeleteImage(i)
		if c.Check(err, check.IsNil) {
			c.Check(op.Wait(), check.IsNil)
		}
	}

	instances, err := cli.GetInstances(api.InstanceTypeContainer)
	c.Check(err, check.IsNil)
	for _, i := range instances {
		if i.Status == "Running" {
			req := api.InstanceStatePut{
				Action:  "stop",
				Timeout: 1,
				Force:   true,
			}

			op, err := cli.UpdateInstanceState(i.Name, req, "")
			if c.Check(err, check.IsNil) {
				c.Check(op.Wait(), check.IsNil)
			}
		}

		op, err := cli.DeleteInstance(i.Name)
		if c.Check(err, check.IsNil) {
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
	image, err := bd.GetBase(ctx, "ubuntu@24.04")
	c.Assert(err, check.IsNil)
	err = bd.DownloadBase(ctx, image, nil)
	c.Assert(err, check.IsNil)

	wf := &workshop.File{
		Name: "test",
		Base: "ubuntu@24.04",
		Actions: map[string]workshop.Action{"info": `



pwd
whoami
printf '%s\n' "$@"
`},
	}

	path := workshop.Filepath(dir, "test")

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, []byte(testYaml), 0o644)
	c.Assert(err, check.IsNil)

	_, _, err = bd.CreateOrLoadProject(ctx, dir)
	c.Assert(err, check.IsNil)

	err = bd.LaunchOrRebuildWorkshop(ctx, wf, image)
	c.Assert(err, check.IsNil)

	err = bd.StartWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)
}

func RemoveTestWorkshop(c *check.C, ctx context.Context, bd workshop.Backend) {
	err := bd.RemoveWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)
}

func MockSdkTarball(c *check.C, sdkname, path, meta string) string {
	sdkfs := filepath.Join(path, sdkname)

	metadir := filepath.Join(sdkfs, "meta")
	err := os.MkdirAll(metadir, 0o755)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(meta), 0o644)
	c.Assert(err, check.IsNil)

	hooksdir := filepath.Join(sdkfs, "sdk", "hooks")
	err = os.MkdirAll(hooksdir, 0o755)
	c.Assert(err, check.IsNil)

	tarball := filepath.Join(path, fmt.Sprintf("%s_1.sdk", sdkname))
	pack := exec.CommandContext(context.Background(), "tar",
		"--create",
		"--format=posix",
		"--use-compress-program=zstd -10 --threads=0",
		"--mode=a-st,go-w",
		"--owner=root:0",
		"--group=root:0",
		"--mtime=2020-04-22T19:12:07.903032Z",
		"--sort=name",
		"--force-local",
		"--file="+tarball,
		"--remove-files",
		"--directory="+sdkfs,
		"meta",
		"sdk",
	)
	output, err := pack.CombinedOutput()
	c.Check(err, check.IsNil, check.Commentf("%s", string(output)))

	return tarball
}
