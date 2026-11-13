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

package lxd_device

import (
	"context"
	"encoding/json"
	"os/user"
	"testing"

	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

func Test(t *testing.T) { check.TestingT(t) }

type lxdSpecSuite struct {
	restoreUserLookup func()
	restoreUserEnv    func()
	restoreLxdInfo    func()
}

var _ = check.Suite(&lxdSpecSuite{})

func (s *lxdSpecSuite) SetUpTest(c *check.C) {
	s.restoreUserLookup = osutil.FakeUserLookup(func(name string) (*user.User, error) {
		return &user.User{Username: "testuser", Uid: "1000", Gid: "1000", HomeDir: "/home/testuser"}, nil
	})
	s.restoreUserEnv = osutil.FakeUserEnvironment(func(u *user.User) (map[string]string, error) {
		return nil, nil
	})
}

func (s *lxdSpecSuite) TearDownTest(c *check.C) {
	if s.restoreLxdInfo != nil {
		s.restoreLxdInfo()
	}
	s.restoreUserEnv()
	s.restoreUserLookup()
}

func (s *lxdSpecSuite) TestSetGpuCDINoGpuDetected(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{Total: 0}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)

	// CDI with no GPUs: no devices created.
	c.Assert(spec.devices, check.HasLen, 0)
}

func (s *lxdSpecSuite) TestSetGpuCDINvidia(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "10de", DRM: &api.ResourcesGPUCardDRM{ID: 0}},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)

	c.Assert(spec.devices, check.HasLen, 1)
	dev, ok := spec.devices["test-sdk_gpu_nvidia"]
	c.Assert(ok, check.Equals, true)
	c.Check(dev["type"], check.Equals, "gpu")
	c.Check(dev["gputype"], check.Equals, "physical")
	c.Check(dev["id"], check.Equals, "nvidia.com/gpu=all")
}

func (s *lxdSpecSuite) TestSetGpuCDIAmd(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "1002", DRM: &api.ResourcesGPUCardDRM{ID: 1}},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)

	c.Assert(spec.devices, check.HasLen, 1)
	dev, ok := spec.devices["test-sdk_gpu_amd"]
	c.Assert(ok, check.Equals, true)
	c.Check(dev["id"], check.Equals, "amd.com/gpu=all")
}

func (s *lxdSpecSuite) TestSetGpuCDIIntel(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "8086", DRM: &api.ResourcesGPUCardDRM{ID: 3}},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)

	// Intel falls back to physical GPU with specific ID.
	c.Assert(spec.devices, check.HasLen, 1)
	dev, ok := spec.devices["test-sdk_gpu_intel_3"]
	c.Assert(ok, check.Equals, true)
	c.Check(dev["type"], check.Equals, "gpu")
	c.Check(dev["gputype"], check.Equals, "physical")
	c.Check(dev["id"], check.Equals, "3")
	c.Check(dev["uid"], check.Equals, workshop.User.Uid)
	c.Check(dev["gid"], check.Equals, workshop.User.Gid)
}

func (s *lxdSpecSuite) TestSetGpuCDIMultipleVendors(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 3,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "10de", DRM: &api.ResourcesGPUCardDRM{ID: 0}},
				{VendorID: "10de", DRM: &api.ResourcesGPUCardDRM{ID: 1}},
				{VendorID: "8086", DRM: &api.ResourcesGPUCardDRM{ID: 2}},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)

	// nvidia deduplicates to one "all" entry, intel gets its own per-card entry.
	c.Assert(spec.devices, check.HasLen, 2)
	_, hasNvidia := spec.devices["test-sdk_gpu_nvidia"]
	c.Check(hasNvidia, check.Equals, true)
	_, hasIntel := spec.devices["test-sdk_gpu_intel_2"]
	c.Check(hasIntel, check.Equals, true)
}

func (s *lxdSpecSuite) TestSetGpuCDIUnknownVendor(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "ffff", DRM: &api.ResourcesGPUCardDRM{ID: 0}},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)
	c.Assert(spec.devices, check.HasLen, 0)
}

func (s *lxdSpecSuite) TestSetGpuCDIIntelWithoutDRM(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "8086"},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)
	c.Assert(spec.devices, check.HasLen, 0)
}

func (s *lxdSpecSuite) TestSetGpuCDIUnknownVendorWithoutDRM(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{GPU: api.ResourcesGPU{
			Total: 1,
			Cards: []api.ResourcesGPUCard{
				{VendorID: "ffff"},
			},
		}}, nil
	})

	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.SetGpu(workshop.Gpu{Name: "gpu"})
	c.Assert(err, check.IsNil)
	c.Assert(spec.devices, check.HasLen, 0)
}

func (s *lxdSpecSuite) TestAddCustomDeviceUSB(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.AddCustomDevice(workshop.CustomDevice{
		Name:      "probe",
		Subsystem: "usb",
		VendorID:  "1366",
		ProductID: "1065",
	})
	c.Assert(err, check.IsNil)

	c.Assert(spec.devices, check.HasLen, 2)
	c.Check(spec.devices["test-sdk_probe"], check.DeepEquals, map[string]string{
		"type": "none",
	})
	c.Check(spec.config[lxdbackend.DeviceTypeConfigKey("test-sdk_probe")], check.Equals, "custom-device")

	var stored workshop.CustomDevice
	err = json.Unmarshal([]byte(spec.config[lxdbackend.DeviceConfigKey("test-sdk_probe")]), &stored)
	c.Assert(err, check.IsNil)
	c.Check(stored, check.DeepEquals, workshop.CustomDevice{
		Name:      "probe",
		Subsystem: "usb",
		VendorID:  "1366",
		ProductID: "1065",
	})

	c.Check(spec.devices["test-sdk_probe_usb"], check.DeepEquals, map[string]string{
		"type":              "usb",
		"vendorid":          "1366",
		"productid":         "1065",
		"required":          "false",
		"ownership.inherit": "true",
	})
	_, hasType := spec.config[lxdbackend.DeviceTypeConfigKey("test-sdk_probe_usb")]
	c.Check(hasType, check.Equals, false)
}

func (s *lxdSpecSuite) TestAddCustomDeviceSubsystem(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.AddCustomDevice(workshop.CustomDevice{
		Name:      "serial",
		Subsystem: "tty",
		VendorID:  "0403",
		ProductID: "6001",
	})
	c.Assert(err, check.IsNil)

	c.Assert(spec.devices, check.HasLen, 1)
	c.Check(spec.devices["test-sdk_serial"], check.DeepEquals, map[string]string{
		"type":              "unix-hotplug",
		"subsystem":         "tty",
		"vendorid":          "0403",
		"productid":         "6001",
		"required":          "false",
		"ownership.inherit": "true",
	})
	_, hasUSB := spec.devices["test-sdk_serial_usb"]
	c.Check(hasUSB, check.Equals, false)
}

func (s *lxdSpecSuite) TestAddCustomDeviceAnySubsystem(c *check.C) {
	s.restoreLxdInfo = MockLxdServerInfo(func(ctx context.Context) (*api.Resources, error) {
		return &api.Resources{}, nil
	})
	spec, err := NewSpecification("testuser", "test-sdk")
	c.Assert(err, check.IsNil)

	err = spec.AddCustomDevice(workshop.CustomDevice{
		Name:      "probe",
		VendorID:  "1366",
		ProductID: "1065",
	})
	c.Assert(err, check.IsNil)

	c.Assert(spec.devices, check.HasLen, 2)
	c.Check(spec.devices["test-sdk_probe"], check.DeepEquals, map[string]string{
		"type":              "unix-hotplug",
		"vendorid":          "1366",
		"productid":         "1065",
		"required":          "false",
		"ownership.inherit": "true",
	})
	c.Check(spec.devices["test-sdk_probe_usb"], check.DeepEquals, map[string]string{
		"type":              "usb",
		"vendorid":          "1366",
		"productid":         "1065",
		"required":          "false",
		"ownership.inherit": "true",
	})
}
