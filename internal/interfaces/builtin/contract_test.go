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

package builtin

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/interfaces/lxd_device"
)

// contractSuite asserts that each built-in interface satisfies the full
// set of contracts its backends discover by type assertion. A drifted or
// misspelled hook method would silently never be called; assigning a
// concrete value to the contract type turns that mistake into a compile
// error naming the missing method.
type contractSuite struct{}

var _ = check.Suite(&contractSuite{})

// TestCameraImplementsContract verifies that the camera interface
// satisfies its expected contracts.
func (s *contractSuite) TestCameraImplementsContract(c *check.C) {
	// The full set of contracts the camera interface must satisfy.
	type cameraContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		lxd_device.MountConnectedPlugDefiner
	}
	var iface cameraContract = &cameraInterface{}
	c.Check(iface, check.NotNil)
}

// TestCustomDeviceImplementsContract verifies that the custom-device
// interface satisfies its expected contracts.
func (s *contractSuite) TestCustomDeviceImplementsContract(c *check.C) {
	// The full set of contracts the custom-device interface must satisfy.
	type customDeviceContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		interfaces.PlugSanitizer
		lxd_device.MountConnectedPlugDefiner
	}
	var iface customDeviceContract = &customDeviceInterface{}
	c.Check(iface, check.NotNil)
}

// TestDesktopImplementsContract verifies that the desktop interface
// satisfies its expected contracts.
func (s *contractSuite) TestDesktopImplementsContract(c *check.C) {
	// The full set of contracts the desktop interface must satisfy.
	type desktopContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		lxd_device.MountConnectedPlugDefiner
	}
	var iface desktopContract = &desktopInterface{}
	c.Check(iface, check.NotNil)
}

// TestGpuImplementsContract verifies that the gpu interface satisfies
// its expected contracts.
func (s *contractSuite) TestGpuImplementsContract(c *check.C) {
	// The full set of contracts the gpu interface must satisfy.
	type gpuContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		lxd_device.MountConnectedPlugDefiner
	}
	var iface gpuContract = &gpuInterface{}
	c.Check(iface, check.NotNil)
}

// TestMountImplementsContract verifies that the mount interface
// satisfies its expected contracts.
func (s *contractSuite) TestMountImplementsContract(c *check.C) {
	// The full set of contracts the mount interface must satisfy.
	type mountContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		interfaces.PlugSanitizer
		interfaces.SlotSanitizer
		lxd_device.MountConnectedPlugDefiner
	}
	var iface mountContract = &mountInterface{}
	c.Check(iface, check.NotNil)
}

// TestSshAgentImplementsContract verifies that the ssh-agent interface
// satisfies its expected contracts.
func (s *contractSuite) TestSshAgentImplementsContract(c *check.C) {
	// The full set of contracts the ssh-agent interface must satisfy.
	type sshAgentContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		lxd_device.MountConnectedPlugDefiner
	}
	var iface sshAgentContract = &sshAgentInterface{}
	c.Check(iface, check.NotNil)
}

// TestTunnelImplementsContract verifies that the tunnel interface
// satisfies its expected contracts.
func (s *contractSuite) TestTunnelImplementsContract(c *check.C) {
	// The full set of contracts the tunnel interface must satisfy.
	type tunnelContract interface {
		interfaces.Interface
		interfaces.StaticInfoProvider
		interfaces.PlugSanitizer
		interfaces.SlotSanitizer
		lxd_device.MountConnectedPlugDefiner
	}
	var iface tunnelContract = &tunnelInterface{}
	c.Check(iface, check.NotNil)
}
