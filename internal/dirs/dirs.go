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

package dirs

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// defaultDataDir is the Workshop directory used if $WORKSHOP_DATA is not
	// set. It is created by the daemon ("workshopd run") if it doesn't exist,
	// and also used by the workshop client.
	defaultDataDir = "/var/lib/workshop"

	// defaultCacheDir is the Workshop directory used if $WORKSHOP_CACHE is not
	// set. It is created by the daemon ("workshopd run") if it doesn't exist.
	defaultCacheDir = "/var/cache/workshop"
)

// Variables for paths inside a workshop
var (
	// base directory inside a workshop
	WorkshopBaseDir = defaultDataDir

	// Directory for mounted binaries (i.e. workshopctl)
	WorkshopGuestBinDir = filepath.Join(WorkshopBaseDir, "bin")

	// SDKs directory to install an SDK in a workshop
	WorkshopSdksDir = filepath.Join(WorkshopBaseDir, "sdk")

	// Base directory for the state storage
	WorkshopStateDir = filepath.Join(WorkshopBaseDir, "state")

	// Run directory inside workshop
	WorkshopRunDir = filepath.Join(WorkshopBaseDir, "run")

	// Path to the daemon's unix socket as seen from inside a workshop. The
	// host daemon's untrusted socket is proxied to this fixed path so that
	// workshopctl and hooks always find it regardless of the daemon's host
	// socket name (which varies, e.g. under "go tool try").
	WorkshopSocketPath = filepath.Join(WorkshopRunDir, "workshop.socket")

	// Directory for actions inside workshop
	WorkshopActionsDir = filepath.Join(WorkshopRunDir, "actions")

	// Cache directory for deb packages
	AptCacheDir = "/var/cache/apt/archives"

	// Symlink to workshopctl that freezes VM filesystems.
	FsFreezePath = "/usr/local/lib/workshop/fsfreeze"
)

// Variables for workshopd (host paths)
var (
	// Directory for data tied to the current daemon installation.
	DataDir string
	// Directory for data shared between parallel daemon installations.
	CommonDir string
	// Cache directory for workshopd
	CacheDir string
	// Path for workshopctl executable
	WorkshopCtlPath string
	// The directory to store downloaded base images and associated metadata
	BaseDownloads string
	// The directory to store downloaded SDKs
	SdkDownloads string
	// Path to the daemon's unix socket
	SocketPath string
	// State lock file
	WorkshopStateLockFile string
	// Base for the XDG runtime directory of a host user
	XdgRuntimeDirBase string
	// Run directory
	WorkshopdRunDir string
	// Locks directory
	WorkshopdLocksDir string
	// SSH keys
	WorkshopSSHDir string
	// Certificates
	WorkshopTlsDir string
)

func getEnvPaths() (dataDir, commonDir, cacheDir, socketPath string) {
	dataDir = os.Getenv("WORKSHOP_DATA")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	commonDir = os.Getenv("WORKSHOP_COMMON")
	if commonDir == "" {
		commonDir = dataDir
	}
	cacheDir = os.Getenv("WORKSHOP_CACHE")
	if cacheDir == "" {
		cacheDir = defaultCacheDir
	}
	socketPath = os.Getenv("WORKSHOP_SOCKET")
	if socketPath == "" {
		socketPath = filepath.Join(commonDir, "workshop.socket")
	}
	return dataDir, commonDir, cacheDir, socketPath
}

func getWorkshopCtlPath() string {
	execPath, err := os.Executable()
	if err != nil {
		panic(fmt.Errorf("cannot get executable path: %w", err))
	}

	// Packages use a dedicated $prefix/lib/workshop/guest directory.
	binDir := filepath.Dir(execPath)
	workshopctl := filepath.Join(filepath.Dir(binDir), "lib", "workshop", "guest", "workshopctl")
	if _, err := os.Stat(workshopctl); err == nil {
		return workshopctl
	}

	// Local development often uses `go install`, which places all binaries in
	// the same directory.
	return filepath.Join(binDir, "workshopctl")
}

func init() {
	XdgRuntimeDirBase = "/run/user"
	DataDir, CommonDir, CacheDir, SocketPath = getEnvPaths()
	setDataDir(DataDir)
	setCommonDir(CommonDir)
	setCacheDir(CacheDir)
	WorkshopCtlPath = getWorkshopCtlPath()
}

// SetRootDir is used by tests to set all host directories in one go.
func SetRootDir(baseDir string) {
	setDataDir(filepath.Join(baseDir, "data"))
	setCommonDir(DataDir)
	setCacheDir(filepath.Join(baseDir, "cache"))
}

func setDataDir(dataDir string) {
	if !filepath.IsAbs(dataDir) {
		panic(fmt.Sprintf("cannot set data dir: path %q is not absolute", dataDir))
	}
	DataDir = dataDir

	WorkshopStateLockFile = filepath.Join(DataDir, "state.lock")
}

func setCommonDir(commonDir string) {
	if !filepath.IsAbs(commonDir) {
		panic(fmt.Sprintf("cannot set common dir: path %q is not absolute", commonDir))
	}
	CommonDir = commonDir

	WorkshopSSHDir = filepath.Join(CommonDir, "ssh")
	WorkshopTlsDir = filepath.Join(CommonDir, "tls")

	// Runtime data (X cookies, SDK locks) is used as an LXD mount source, so it
	// must live on a revision-independent path to survive snap refreshes.
	WorkshopdRunDir = filepath.Join(CommonDir, "run", "workshopd")
	WorkshopdLocksDir = filepath.Join(WorkshopdRunDir, "locks")
}

func setCacheDir(cachedir string) {
	if !filepath.IsAbs(cachedir) {
		panic(fmt.Sprintf("cannot set cache dir: path %q is not absolute", cachedir))
	}
	CacheDir = cachedir

	BaseDownloads = filepath.Join(CacheDir, "base")
	SdkDownloads = filepath.Join(CacheDir, "sdk")
}

func CreateDirs() error {
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(CommonDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(CacheDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(BaseDownloads, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(SdkDownloads, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(WorkshopdRunDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(WorkshopdLocksDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(WorkshopSSHDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(WorkshopTlsDir, 0755); err != nil {
		return err
	}
	return nil
}
