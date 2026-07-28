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

//go:build integration

package helper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk"
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

	args := lxd.GetInstancesArgs{InstanceType: api.InstanceTypeAny}
	instances, err := cli.GetInstances(args)
	c.Check(err, check.IsNil)
	for _, i := range instances {
		op, err := cli.DeleteInstance(i.Name, true)
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

	op, err := cli.DeleteProject(project, false)
	c.Check(err, check.IsNil)
	if c.Check(err, check.IsNil) {
		c.Check(op.Wait(), check.IsNil)
	}
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

	err = os.WriteFile(path, []byte(testYaml), 0644)
	c.Assert(err, check.IsNil)

	_, _, err = bd.CreateOrLoadProject(ctx, dir)
	c.Assert(err, check.IsNil)

	snapshot := workshop.BaseOnly(bd.FormatRevision(), image.Name, image.Fingerprint)
	err = bd.LaunchOrRebuildWorkshop(ctx, wf, snapshot)
	c.Assert(err, check.IsNil)

	err = bd.StartWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)
}

func RemoveTestWorkshop(c *check.C, ctx context.Context, bd workshop.Backend) {
	_ = bd.StopWorkshop(ctx, "test", true)
	err := bd.RemoveWorkshop(ctx, "test")
	c.Assert(err, check.IsNil)
}

func ExecOutput(ctx context.Context, bd workshop.Backend, name string, args workshop.ExecArgs) (string, error) {
	var stdout, stderr strings.Builder
	exec := workshop.Execution{
		ExecArgs: args,
		ExecControls: workshop.ExecControls{
			Stdout: &stdout,
			Stderr: &stderr,
		},
	}
	exectx, err := bd.Exec(ctx, name, &exec)
	if err != nil {
		return "", err
	}
	if err := exectx.WaitExecution(ctx); err != nil {
		return "", fmt.Errorf("%w\n%s", err, stderr.String())
	}
	return stdout.String(), err
}

func MockSdkTarball(c *check.C, sdkname, sdkYaml string) string {
	path := c.MkDir()
	sdkfs := filepath.Join(path, sdkname)

	metadir := filepath.Join(sdkfs, "meta")
	err := os.MkdirAll(metadir, 0755)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(sdkYaml), 0644)
	c.Assert(err, check.IsNil)

	hooksdir := filepath.Join(sdkfs, "sdk", "hooks")
	err = os.MkdirAll(hooksdir, 0755)
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

func MockSdkVolume(c *check.C, ctx context.Context, bd workshop.Backend, meta sdk.Meta) {
	tarball := MockSdkTarball(c, meta.Name, meta.SdkYAML)
	file, err := os.Open(tarball)
	c.Assert(err, check.IsNil)
	defer file.Close()

	err = bd.ImportSdk(ctx, meta, file)
	c.Assert(err, check.IsNil)
}
