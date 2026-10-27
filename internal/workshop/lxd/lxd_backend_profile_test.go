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
					Mode:      0123,
					Owner:     45,
					Group:     67}},
		}, {
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			Tunnels: []workshop.Tunnel{{
				ProxyEntry: workshop.ProxyEntry{
					Name: "http",
					Connect: workshop.ProxyTarget{
						Address:  "127.0.0.1:8080",
						Protocol: "tcp"},
					Listen: workshop.ProxyTarget{
						Address:  "0.0.0.0:8000",
						Protocol: "tcp"},
					Direction: workshop.HostToWorkshop}}},
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
					Direction: workshop.WorkshopToHost}},
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
					Direction: workshop.WorkshopToHost}},
		}, {
			Sdk: "sdk",
			Mounts: map[string]workshop.Mount{
				"plug": {
					Name:  "plug",
					What:  "/var",
					Where: "/etc",
					Type:  workshop.WorkshopWorkshop}}},
		{
			Sdk:    "sdk",
			Mounts: map[string]workshop.Mount{},
			CustomDevices: []workshop.CustomDevice{{
				Name:      "mydevice",
				Subsystem: "tty",
				VendorID:  "0403",
				ProductID: "6001"}},
		},
		{
			Sdk:            "sdk",
			Mounts:         map[string]workshop.Mount{},
			Virtualization: &workshop.Virtualization{Name: "virtualization"},
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
					"ownership.inherit": "true"}},
			map[string]string{
				"user.workshop.sdk_camera":                  `{"name": "camera"}`,
				"user.workshop.sdk_camera.type":             "camera",
				"user.workshop.sdk_camera_video4linux.type": "camera",
				"user.workshop.sdk_camera_media.type":       "camera"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_gpu": {
					"type":    "gpu",
					"gputype": "physical",
					"uid":     "1000",
					"gid":     "1000"}},
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
					"user.path-group":  "67"}},
			map[string]string{},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_http": {
					"type":    "proxy",
					"connect": "tcp:127.0.0.1:8080",
					"listen":  "tcp:0.0.0.0:8000",
					"bind":    "host"}},
			map[string]string{
				"user.workshop.sdk_http.type": "tunnel"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_ssh-agent": {
					"type":    "proxy",
					"connect": "unix:.host.socket",
					"listen":  "unix:.workshop.socket",
					"uid":     "1000",
					"gid":     "1000",
					"bind":    "instance"}},
			map[string]string{
				"user.workshop.sdk_ssh-agent.type": "ssh-agent"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_desktop": {
					"type":    "proxy",
					"connect": "unix:.host.socket",
					"listen":  "unix:.workshop.socket",
					"uid":     "1000",
					"gid":     "1000",
					"bind":    "instance"}},
			map[string]string{
				"user.workshop.sdk_desktop.type": "desktop-wayland"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_plug": {
					"type": "none"}},
			map[string]string{
				"user.workshop.sdk_plug": `{
          "name": "plug",
          "what": "/var",
          "where": "/etc", 
          "type": 1}`,
				"user.workshop.sdk_plug.type": "mount"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_mydevice": {
					"type":              "unix-hotplug",
					"subsystem":         "tty",
					"vendorid":          "0403",
					"productid":         "6001",
					"required":          "false",
					"ownership.inherit": "true"}},
			map[string]string{
				"user.workshop.sdk_mydevice.type": "custom-device"},
		}, {
			"sdk",
			map[string]map[string]string{
				"sdk_virtualization": {
					"type": "none"},
				"sdk_virtualization_kvm": {
					"type":     "unix-char",
					"source":   "/dev/kvm",
					"path":     "/dev/kvm",
					"gid":      "1000",
					"mode":     "0660",
					"required": "false"},
				"sdk_virtualization_vhost-net": {
					"type":     "unix-char",
					"source":   "/dev/vhost-net",
					"path":     "/dev/vhost-net",
					"gid":      "1000",
					"mode":     "0660",
					"required": "false"}},
			map[string]string{
				"user.workshop.sdk_virtualization":                `{"name": "virtualization"}`,
				"user.workshop.sdk_virtualization.type":           "virtualization",
				"user.workshop.sdk_virtualization_kvm.type":       "virtualization",
				"user.workshop.sdk_virtualization_vhost-net.type": "virtualization"},
		},
	} {
		res, err := lxdbackend.LxdToSdkProfile(t.name, t.devs, t.cfg)
		c.Assert(err, check.IsNil)
		c.Assert(res.Agent, check.DeepEquals, expected[i].Agent)
		c.Assert(res.Desktop, check.DeepEquals, expected[i].Desktop)
		c.Assert(res.Camera, check.DeepEquals, expected[i].Camera)
		c.Assert(res.CustomDevices, testutil.DeepUnsortedMatches, expected[i].CustomDevices)
		c.Assert(res.Gpu, check.DeepEquals, expected[i].Gpu)
		c.Assert(res.Virtualization, check.DeepEquals, expected[i].Virtualization)
		c.Assert(res.Mounts, testutil.DeepUnsortedMatches, expected[i].Mounts)
		c.Assert(res.Tunnels, testutil.DeepUnsortedMatches, expected[i].Tunnels)
	}
}
