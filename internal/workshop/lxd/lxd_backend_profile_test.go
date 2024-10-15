package lxdbackend_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type ProfileSuite struct {
}

var _ = check.Suite(&ProfileSuite{})

func (f *LxdBeTests) TestLxdToSdkProfileOK(c *check.C) {
	expected := []workshop.SdkProfile{
		{Sdk: "sdk", Camera: &workshop.Camera{Name: "camera"}, Mounts: map[string]workshop.Mount{}},
		{Sdk: "sdk", Mounts: map[string]workshop.Mount{}, Gpu: &workshop.Gpu{Name: "gpu"}},
		{Sdk: "sdk", Mounts: map[string]workshop.Mount{"userdisk": {Name: "userdisk", What: "/home", Where: "/opt", Type: workshop.HostWorkshop}}},
		{Sdk: "sdk", Mounts: map[string]workshop.Mount{}, Proxies: map[string]workshop.Proxy{"ssh-agent": {Name: "ssh-agent", Connect: "unix:.host.socket", Listen: "unix:.workshop.socket"}}},
		{Sdk: "sdk", Mounts: map[string]workshop.Mount{"plug": {Name: "plug", What: "/var", Where: "/etc", Type: workshop.WorkshopWorkshop}}},
	}

	for i, t := range []struct {
		name string
		devs map[string]map[string]string
		cfg  map[string]string
	}{
		{"sdk", map[string]map[string]string{"camera": {"type": "none"}, "camera/video0": {"type": "unix-char", "source": "/dev/video0", "path": "/dev/video0", "required": "false", "uid": "1000", "gid": "1000"}}, map[string]string{"user.workshop.sdk.camera": `{"name": "camera"}`, "user.workshop.sdk.camera.type": "camera", "user.workshop.sdk.camera/video0.type": "camera"}},
		{"sdk", map[string]map[string]string{"gpu": {"type": "gpu", "gputype": "physical", "uid": "1000", "gid": "1000"}}, map[string]string{}},
		{"sdk", map[string]map[string]string{"userdisk": {"type": "disk", "source": "/home", "path": "/opt"}}, map[string]string{}},
		{"sdk", map[string]map[string]string{"ssh-agent": {"type": "proxy", "connect": "unix:.host.socket", "listen": "unix:.workshop.socket", "uid": "1000", "gid": "1000", "bind": "instance"}}, map[string]string{}},
		{"sdk", map[string]map[string]string{"plug": {"type": "none"}}, map[string]string{"user.workshop.sdk.plug": `{"name": "plug", "what": "/var", "where": "/etc", "type": 1}`, "user.workshop.sdk.plug.type": "mount"}},
	} {
		res, err := lxdbackend.LxdToSdkProfile(t.name, t.devs, t.cfg)
		c.Assert(err, check.IsNil)
		c.Assert(res.Camera, check.DeepEquals, expected[i].Camera)
		c.Assert(res.Gpu, check.DeepEquals, expected[i].Gpu)
		c.Assert(res.Proxies["ssh-agent"], check.DeepEquals, expected[i].Proxies["ssh-agent"])
		c.Assert(res.Mounts, testutil.DeepUnsortedMatches, expected[i].Mounts)
	}
}
