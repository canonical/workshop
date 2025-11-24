package lxdbackend_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

type ProfileSuite struct{}

var _ = check.Suite(&ProfileSuite{})

func (f *LxdBeTests) TestLxdToSdkProfileOK(c *check.C) {
	expected := []workshop.SdkProfile{
		{
			Sdk:    "sdk",
			Camera: &workshop.Camera{Name: "camera"},
			Mounts: map[string]workshop.Mount{},
		}, {
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			Gpu:    &workshop.Gpu{Name: "gpu"},
		}, {
			Sdk: "sdk",
			Mounts: map[string]workshop.Mount{
				"userdisk": {
					Name:      "userdisk",
					Type:      workshop.HostWorkshop,
					What:      "/home",
					Where:     "/opt",
					MakeWhere: true,
					Mode:      0o123,
					Owner:     45,
					Group:     67,
				},
			},
		}, {
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			Tunnels: []workshop.Tunnel{{
				ProxyEntry: workshop.ProxyEntry{
					Name: "http",
					Connect: workshop.ProxyTarget{
						Address:  "127.0.0.1:8080",
						Protocol: "tcp",
					},
					Listen: workshop.ProxyTarget{
						Address:  "0.0.0.0:8000",
						Protocol: "tcp",
					},
					Direction: workshop.HostToWorkshop,
				},
			}},
		}, {
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			Agent: &workshop.SshAgent{
				ProxyEntry: workshop.ProxyEntry{
					Name: "ssh-agent",
					Connect: workshop.ProxyTarget{
						Address:  ".host.socket",
						Protocol: "unix",
					},
					Listen: workshop.ProxyTarget{
						Address:  ".workshop.socket",
						Protocol: "unix",
					},
					Direction: workshop.WorkshopToHost,
				},
			},
		}, {
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			Desktop: &workshop.Desktop{
				Wayland: &workshop.ProxyEntry{
					Name: "desktop",
					Connect: workshop.ProxyTarget{
						Address:  ".host.socket",
						Protocol: "unix",
					},
					Listen: workshop.ProxyTarget{
						Address:  ".workshop.socket",
						Protocol: "unix",
					},
					Direction: workshop.WorkshopToHost,
				},
			},
		}, {
			Sdk: "sdk",
			Mounts: map[string]workshop.Mount{
				"plug": {
					Name:  "plug",
					What:  "/var",
					Where: "/etc",
					Type:  workshop.WorkshopWorkshop,
				},
			},
		},
	}

	for i, t := range []struct {
		name string
		devs map[string]map[string]string
		cfg  map[string]string
	}{
		{
			"sdk",
			map[string]map[string]string{
				"sdk_camera": {
					"type": "none",
				},
				"sdk_camera_video4linux": {
					"type":              "unix-hotplug",
					"subsystem":         "video4linux",
					"required":          "false",
					"ownership.inherit": "true",
				},
				"sdk_camera_media": {
					"type":              "unix-hotplug",
					"subsystem":         "media",
					"required":          "false",
					"ownership.inherit": "true",
				},
			},
			map[string]string{
				"user.workshop.sdk_camera":                  `{"name": "camera"}`,
				"user.workshop.sdk_camera.type":             "camera",
				"user.workshop.sdk_camera_video4linux.type": "camera",
				"user.workshop.sdk_camera_media.type":       "camera",
			},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_gpu": {
					"type":    "gpu",
					"gputype": "physical",
					"uid":     "1000",
					"gid":     "1000",
				},
			},
			map[string]string{},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_userdisk": {
					"type":             "disk",
					"source":           "/home",
					"path":             "/opt",
					"user.make-source": "false",
					"user.make-path":   "true",
					"user.path-mode":   "0o123",
					"user.path-owner":  "45",
					"user.path-group":  "67",
				},
			},
			map[string]string{},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_http": {
					"type":    "proxy",
					"connect": "tcp:127.0.0.1:8080",
					"listen":  "tcp:0.0.0.0:8000",
					"bind":    "host",
				},
			},
			map[string]string{
				"user.workshop.sdk_http.type": "tunnel",
			},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_ssh-agent": {
					"type":    "proxy",
					"connect": "unix:.host.socket",
					"listen":  "unix:.workshop.socket",
					"uid":     "1000",
					"gid":     "1000",
					"bind":    "instance",
				},
			},
			map[string]string{
				"user.workshop.sdk_ssh-agent.type": "ssh-agent",
			},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_desktop": {
					"type":    "proxy",
					"connect": "unix:.host.socket",
					"listen":  "unix:.workshop.socket",
					"uid":     "1000",
					"gid":     "1000",
					"bind":    "instance",
				},
			},
			map[string]string{
				"user.workshop.sdk_desktop.type": "desktop-wayland",
			},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_plug": {
					"type": "none",
				},
			},
			map[string]string{
				"user.workshop.sdk_plug": `{
          "name": "plug",
          "what": "/var",
          "where": "/etc", 
          "type": 1}`,
				"user.workshop.sdk_plug.type": "mount",
			},
		},
	} {
		res, err := lxdbackend.LxdToSdkProfile(t.name, t.devs, t.cfg)
		c.Assert(err, check.IsNil)
		c.Assert(res.Agent, check.DeepEquals, expected[i].Agent)
		c.Assert(res.Desktop, check.DeepEquals, expected[i].Desktop)
		c.Assert(res.Camera, check.DeepEquals, expected[i].Camera)
		c.Assert(res.Gpu, check.DeepEquals, expected[i].Gpu)
		c.Assert(res.Mounts, testutil.DeepUnsortedMatches, expected[i].Mounts)
		c.Assert(res.Tunnels, testutil.DeepUnsortedMatches, expected[i].Tunnels)
	}
}
