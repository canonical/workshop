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
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/user"
	"slices"
	"time"

	"github.com/gorilla/websocket"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
)

type ContextKeyProjectId string
type ContextKeyUser string

const (
	ContextProjectId = ContextKeyProjectId("project-id")
	ContextUser      = ContextKeyUser("user")

	Uid = 1000
	Gid = 1000

	RootUmask   = os.FileMode(0022)
	NormalUmask = os.FileMode(0002)
)

var (
	ErrWorkshopNotLaunched   = errors.New("workshop not launched")
	ErrVolumeNotFound        = errors.New("volume not found")
	ErrVolumeAlreadyExists   = errors.New("volume already exists")
	ErrVolumeInUse           = errors.New("volume is in use")
	ErrSnapshotNotFound      = errors.New("snapshot not found")
	ErrSnapshotAlreadyExists = errors.New("snapshot already exists")
	ErrSdkProfileNotFound    = errors.New("sdk profile not found")
	ErrIncompatibleBackend   = errors.New("incompatible backend")

	User = user.User{
		Uid:      "1000",
		Gid:      "1000",
		Username: "workshop",
		HomeDir:  "/home/workshop",
	}
)

type ErrExec struct {
	Status int
}

func (e *ErrExec) Error() string {
	return fmt.Sprintf("command exit code %d", e.Status)
}

type Stash interface {
	// Make a stash of the workshop. The workshop will be stopped and will not
	// be available to other workshop operations, e.g. list, stop, start and so
	// on. A new workshop with the same name can be launched for the same
	// project-id.
	StashWorkshop(ctx context.Context, name string) error

	// Restore the workshop from the stash (if exists, see StashWorkshop). The
	// workshop will be restored and become visible to the backend operations.
	// Fails if a workshop with the same name exists.
	UnstashWorkshop(ctx context.Context, name string) error

	// Delete the workshop from stash (if exists).
	RemoveWorkshopStash(ctx context.Context, name string) error
}

type SdkVolume struct {
	sdk.Meta
	// Project ID / Workshop pairs that the volume is attached to.
	Workshops map[string][]string
	// Size reports the current volume usage in bytes when available.
	Size uint64
}

type BaseImage struct {
	// Base name (e.g. ubuntu@24.04).
	Name string `json:"name"`
	// Type of sandbox used to run the image (e.g. container).
	Confinement Confinement `json:"confinement"`
	// Base image identifier, typically a hash.
	Fingerprint string `json:"fingerprint"`
}

type BaseImageManager interface {
	// Lookup the latest image for the given base.
	GetBase(ctx context.Context, base string, confinement Confinement) (BaseImage, error)
	// Download the given base image.
	DownloadBase(ctx context.Context, image BaseImage, report *progress.Reporter) error
}

// Snapshot identifies a workshop snapshot without depending on
// backend-specific details. For example snapshot names in LXD are subject to
// length limits and a restricted character set.
type Snapshot struct {
	Format sdk.Revision
	Image  BaseImage
	Sdks   []sdk.ContentID
}

func (s Snapshot) Equal(other Snapshot) bool {
	return s.Format == other.Format && s.Image == other.Image && slices.Equal(s.Sdks, other.Sdks)
}

// IsBasedOn returns true if other is a prefix of s.
func (s Snapshot) IsBasedOn(other Snapshot) bool {
	if s.Format != other.Format || s.Image != other.Image {
		return false
	}
	if len(s.Sdks) < len(other.Sdks) {
		return false
	}
	s.Sdks = s.Sdks[:len(other.Sdks)]
	return slices.Equal(s.Sdks, other.Sdks)
}

// BaseOnly identifies a "snapshot" which consists of a base image only.
func BaseOnly(format sdk.Revision, name string, confinement Confinement, fingerprint string) Snapshot {
	return Snapshot{Format: format, Image: BaseImage{Name: name, Confinement: confinement, Fingerprint: fingerprint}}
}

// SdkSnapshot identifies a snapshot consisting of a base image and a sequence
// of installed SDKs.
func SdkSnapshot(format sdk.Revision, image BaseImage, sdks []sdk.Setup) Snapshot {
	snapshot := Snapshot{Format: format, Image: image, Sdks: make([]sdk.ContentID, 0, len(sdks))}
	for _, s := range sdks {
		snapshot.Sdks = append(snapshot.Sdks, sdk.SetupContentID(s))
	}
	return snapshot
}

// IsBase returns whether the snapshot is just a base image.
func (s Snapshot) IsBase() bool {
	return len(s.Sdks) == 0
}

type SnapshotInfo struct {
	Snapshot
	// Project ID / Workshop pairs that are based on the snapshot.
	Workshops map[string][]string
}

type SdkManager interface {
	// Import an SDK tarball as a new volume.
	ImportSdk(ctx context.Context, meta sdk.Meta, tarball *os.File) error

	// Delete an SDK volume. It does not unmount the volume from workshops
	// where it is mounted. No error is returned if the SDK does not exist.
	DeleteSdk(ctx context.Context, setup sdk.Setup) error

	// List available SDK volumes.
	Sdks(ctx context.Context) ([]SdkVolume, error)

	// Get the SDK volume information.
	Sdk(ctx context.Context, setup sdk.Setup) (SdkVolume, error)
}

type NetworkManager interface {
	// InterfaceAddrs lists the IP addresses configured for the given device.
	InterfaceAddrs(ctx context.Context, iface string) ([]netip.Addr, error)
}

type ExecArgs struct {
	// Command is the user-requested command. It is used for user-facing
	// summaries and diagnostics.
	Command []string

	// CommandPrefix is a Workshop-managed wrapper prepended to [Command] at the
	// execution boundary. It is not part of the user-requested command.
	CommandPrefix []string
	UserId        int
	GroupId       int
	WorkDir       string
	Timeout       time.Duration
	Environment   map[string]string
	Interactive   bool
	Terminal      bool
	SplitStderr   bool
	Width         int
	Height        int
}

// EffectiveCommand returns the command that should be sent to the execution
// backend, including any Workshop-managed command prefix.
func (args ExecArgs) EffectiveCommand() []string {
	command := make(
		[]string,
		0,
		len(args.CommandPrefix)+len(args.Command),
	)
	command = append(command, args.CommandPrefix...)
	return append(command, args.Command...)
}

type ExecControls struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Control func(conn *websocket.Conn)
}

type Execution struct {
	ExecArgs
	ExecControls
}

type ExecContext struct {
	Environment   map[string]string
	WaitExecution func(ctx context.Context) error
}

type Backend interface {
	Stash
	BaseImageManager
	SdkManager

	// The backend will attempt to load a project for the given path
	// using its mapping between the path and a project id. If the project
	// is not found, e.g. .lock file was removed by the user, but there is still
	// an existing record for the path, it will first attempt to restore .lock.
	// If unsuccessful, a new project will be created for the path provided.
	CreateOrLoadProject(ctx context.Context, path string) (*Project, bool, error)

	// Returns a list of projects known to the backend. The returned map
	// has a username key that the corresponding projects belong to.
	Projects(ctx context.Context) (map[string][]Project, error)

	// Loads a workshop instance.
	Workshop(ctx context.Context, name string) (*Workshop, error)

	// Returns a workshop's file system interface.
	WorkshopFs(ctx context.Context, name string) (fsutil.Fs, error)

	// Returns a list of workshops for the project in context.
	ProjectWorkshops(ctx context.Context) ([]*Workshop, error)

	// Number representing workshop and snapshot compatibility level.
	FormatRevision() sdk.Revision

	// Look up the given snapshot. On success, returns the given snapshot and
	// the workshops that are based on it. On failure, returns an error. In the
	// unlikely event the error is caused by a snapshot conflict (i.e. a name
	// collision), also returns the conflicting snapshot.
	Snapshot(ctx context.Context, snapshot Snapshot) (*SnapshotInfo, error)

	// Launch a clean workshop instance. If the workshop exists, wipe out
	// its rootfs and rebuild it from the given snapshot (which may be just
	// a base image). Configuration and devices of the rebuilt workshop
	// will be reset to the default one.
	LaunchOrRebuildWorkshop(ctx context.Context, file *File, snapshot Snapshot) error

	// Create a snapshot of the workshop's rootfs. The snapshot can be used
	// by passing an identical Snapshot to LaunchOrRebuildWorkshop.
	TakeSnapshot(ctx context.Context, name string, snapshot Snapshot) error

	// Remove the given snapshot.
	RemoveSnapshot(ctx context.Context, snapshot Snapshot) error

	// Delete workshop. Stop the workshop forcefully if not in Stopped before deleting
	RemoveWorkshop(ctx context.Context, name string) error

	// Starts a workshop and waits until it is ready
	// to accept commands
	StartWorkshop(ctx context.Context, name string) error

	// Stops workshop gracefully (i.e. waits for the graceful instance and all
	// its running services termination) unless force is used.
	StopWorkshop(ctx context.Context, name string, force bool) error

	// Adds a workshop mount described by the properties.
	AddWorkshopMount(ctx context.Context, name string, mount Mount) error

	// Removes a workshop mount.
	RemoveWorkshopMount(ctx context.Context, name, mount string) error

	// Mount an SDK volume and add the SDK to the Sdks field.
	InstallSdk(ctx context.Context, name string, setup sdk.Setup) error

	// Remove an SDK from the Sdks field and unmount the SDK volume.
	UninstallSdk(ctx context.Context, name, sk string) error

	// Execute a command in a given workshop. The client should differentiate
	// between the errors that occurred during the execution but not related to
	// the command (i.e. the workshop does not exist) and the errors that were
	// produced by the command itself (i.e. return code != 0). If the latter, an
	// instance of ErrExec with the status code will be returned.
	// The callback ExecContext.WaitExecution will initite the command execution
	// and redirect its IO using args.ExecControls. ExecContext.Environment will
	// contain full (actual)
	Exec(ctx context.Context, name string, args *Execution) (ExecContext, error)
}

type cachedBackendKey struct{}

// ReplaceBackend replaces the store used by the manager.
func ReplaceBackend(state *state.State, backend Backend) {
	state.Lock()
	state.Cache(cachedBackendKey{}, backend)
	state.Unlock()
}

func cachedBackend(st *state.State) Backend {
	backend := st.Cached(cachedBackendKey{})
	if backend == nil {
		return nil
	}
	return backend.(Backend)
}

// Store returns the store service provided by the optional device context or
// the one used by the snapstate package if the former has no
// override.
func WorkshopBackend(st *state.State) Backend {
	if backend := cachedBackend(st); backend != nil {
		return backend
	}
	panic("internal error: needing the store before managers have initialized it")
}
