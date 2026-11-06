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

package workshop

import (
	"os"
	"path/filepath"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil/sys"
)

var DefaultDevices = defaultDevices

type MountType int

const (
	HostWorkshop MountType = iota
	WorkshopWorkshop
	Volume
)

type ProxyTarget struct {
	Address  string
	Protocol string
}

type ProxyDirection int

const (
	HostToWorkshop ProxyDirection = iota
	WorkshopToHost
)

type ProxyEntry struct {
	Name      string
	Connect   ProxyTarget
	Listen    ProxyTarget
	Direction ProxyDirection
}

func (p *ProxyEntry) Equal(other *ProxyEntry) bool {
	if p == nil || other == nil {
		return p == other
	}

	return *p == *other
}

type Camera struct {
	Name string `json:"name"`
}

type CustomDevice struct {
	Name      string `json:"name"`
	Subsystem string `json:"subsystem,omitempty"`
	VendorID  string `json:"vendorid,omitempty"`
	ProductID string `json:"productid,omitempty"`
}

type Mount struct {
	Name      string      `json:"name"`
	Type      MountType   `json:"type"`
	What      string      `json:"what"`
	MakeWhat  bool        `json:"make-what,omitempty"`
	Where     string      `json:"where"`
	MakeWhere bool        `json:"make-where,omitempty"`
	Mode      os.FileMode `json:"mode,omitempty"`
	Owner     sys.UserID  `json:"owner,omitempty"`
	Group     sys.GroupID `json:"group,omitempty"`
	ReadOnly  bool        `json:"readonly"`
}

type Tunnel struct {
	ProxyEntry
}

type SshAgent struct {
	ProxyEntry
}

func (s *SshAgent) Equal(other *SshAgent) bool {
	if s == nil || other == nil {
		return s == other
	}

	return *s == *other
}

type Desktop struct {
	Wayland *ProxyEntry
	X11     *ProxyEntry
}

func (d *Desktop) Equal(other *Desktop) bool {
	if d == nil || other == nil {
		return d == other
	}

	return d.Wayland.Equal(other.Wayland) && d.X11.Equal(other.X11)
}

type Gpu struct {
	Name string
}

type Virtualization struct {
	Name string `json:"name"`
}

type SdkProfile struct {
	Sdk string

	Camera         *Camera
	CustomDevices  []CustomDevice
	Mounts         map[string]Mount
	Tunnels        []Tunnel
	Agent          *SshAgent
	Gpu            *Gpu
	Desktop        *Desktop
	Virtualization *Virtualization
}

func NewSdkProfile(sdkName string) SdkProfile {
	return SdkProfile{
		Sdk:    sdkName,
		Mounts: make(map[string]Mount),
	}
}

func defaultDevices(pid, w string) ([]Mount, []ProxyEntry) {
	mounts := []Mount{{
		Name:     "workshop.bin",
		Type:     HostWorkshop,
		What:     filepath.Dir(dirs.WorkshopCtlPath),
		Where:    dirs.WorkshopGuestBinDir,
		ReadOnly: true,
	}, {
		Name:  "cache.apt",
		Type:  HostWorkshop,
		What:  AptCacheDir(pid, w),
		Where: dirs.AptCacheDir,
	}}

	// Connect to this daemon's untrusted socket on the host, but always expose
	// it inside the workshop at a fixed path. The host socket name varies (e.g.
	// under "go tool try"), so deriving the instance-side name from it would
	// leave workshopctl and hooks looking for a socket that does not exist.
	socketHost := dirs.SocketPath + ".untrusted"
	socketWorkshop := dirs.WorkshopSocketPath + ".untrusted"
	proxies := []ProxyEntry{{
		Name:      "workshop.socket",
		Connect:   ProxyTarget{Address: socketHost, Protocol: "unix"},
		Listen:    ProxyTarget{Address: socketWorkshop, Protocol: "unix"},
		Direction: WorkshopToHost,
	}}

	return mounts, proxies
}

func FakeDefaultDevices(f func(pid, w string) ([]Mount, []ProxyEntry)) func() {
	oldDefault := DefaultDevices
	DefaultDevices = f
	return func() { DefaultDevices = oldDefault }
}
