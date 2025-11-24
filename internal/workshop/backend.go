package workshop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"time"

	"github.com/gorilla/websocket"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
)

type (
	ContextKeyProjectId string
	ContextKeyUser      string
)

type WorkshopConfigFilter func(config map[string]string) bool

const (
	ContextProjectId = ContextKeyProjectId("project-id")
	ContextUser      = ContextKeyUser("user")

	Uid = 1000
	Gid = 1000

	RootUmask   = os.FileMode(0o022)
	NormalUmask = os.FileMode(0o002)
)

var (
	ErrWorkshopNotLaunched = errors.New("workshop not launched")
	ErrVolumeNotFound      = errors.New("volume not found")
	ErrVolumeAlreadyExists = errors.New("volume already exists")
	ErrVolumeInUse         = errors.New("volume is in use")
	ErrSdkProfileNotFound  = errors.New("sdk profile not found")
	ErrIncompatibleBackend = errors.New("incompatible backend")

	User = user.User{
		Uid:      "1000",
		Gid:      "1000",
		Username: "workshop",
		HomeDir:  "/home/workshop",
	}
)

func NewWorkshopConfigFilter(key string, value string) WorkshopConfigFilter {
	return func(config map[string]string) bool {
		return config[key] == value
	}
}

type ErrExec struct {
	Status int
}

func (e *ErrExec) Error() string {
	return fmt.Sprintf("command exit code %d", e.Status)
}

type WorkshopConfigValue struct {
	Name  string
	Value string
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

type VolumeSetup struct {
	// Volume name.
	Name string
	// Kind of volume, e.g. "sdk" or "state-storage."
	Kind string
	// Hash of tarball used to create volume.
	Sha3_384 string
	// Name of SDK associated with volume, if any.
	Sdk string
	// Revision of SDK associated with volume, if any.
	Revision sdk.Revision
	// For SDK volumes, a copy of meta/sdk.yaml.
	Metadata string
}

type VolumeInfo struct {
	VolumeSetup
	// Project ID / Workshop pairs that the volume is attached to.
	Workshops map[string][]string
	// Size reports the current volume usage in bytes when available.
	Size uint64
}

type VolumeManager interface {
	// Create a temporary storage volume for the workshop. It does not
	// mount the device to the workshop, it must be mounted to the required
	// workshop as a separate operation.
	CreateVolume(ctx context.Context, info VolumeSetup) error

	// Import a tarball into the volume.
	ImportVolume(ctx context.Context, info VolumeSetup, tarball *os.File) error

	// Attach the volume to the workshop. The volume must be created before.
	AttachVolume(ctx context.Context, wp, name, where string, ro bool) error

	// Detach the volume from the workshop.
	DetachVolume(ctx context.Context, wp, name string) error

	// Delete a temporary storage volume for the workshop. It does not unmount
	// the volume from the workshop if mounted. No error is returned if the
	// volume does not exist.
	DeleteVolume(ctx context.Context, name string) error

	// List volumes of a given kind.
	Volumes(ctx context.Context, kind string) ([]VolumeInfo, error)

	// Get the volume information.
	Volume(ctx context.Context, name string) (VolumeInfo, error)
}

type BaseImage struct {
	// Base name (e.g. ubuntu@24.04).
	Name string `json:"name"`
	// Base image identifier, typically a hash.
	Fingerprint string `json:"fingerprint"`
}

type BaseImageManager interface {
	// Lookup the latest image for the given base.
	GetBase(ctx context.Context, base string) (BaseImage, error)
	// Download the given base image.
	DownloadBase(ctx context.Context, image BaseImage, report *progress.Reporter) error
}

type ExecArgs struct {
	Command     []string
	UserId      int
	GroupId     int
	WorkDir     string
	Timeout     time.Duration
	Environment map[string]string
	Interactive bool
	Terminal    bool
	SplitStderr bool
	Width       int
	Height      int
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
	VolumeManager
	BaseImageManager
	Snapshot

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

	// Launch a barebone workshop instance. If the workshop exists, wipe out its
	// rootfs and rebuild it from the workshop file's base image. The
	// configuration and devices of the rebuilt workshop will be reset to the
	// default one. The given base fingerprint is combined with `file.Base` to
	// determine the exact base image to use.
	LaunchOrRebuildWorkshop(ctx context.Context, file *File, image BaseImage) error

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

	// TODO: these methods are too generic and should be wrapped with a proper
	// interface method where required. We should not let the client to change
	// any workshop property arbitrarily.
	AddWorkshopConfig(ctx context.Context, name string, item *WorkshopConfigValue) error
	RemoveWorkshopConfig(ctx context.Context, name string, key string) error

	// Execute a command in a given workshop. The client should differentiate
	// between the errors that occured during the execution but not related to
	// the command (i.e. the workshop does not exist) and the errors that were
	// produced by the command itself (i.e. return code != 0). If the latter, an
	// instance of ErrExec with the status code will be returned.
	// The callback ExecContext.WaitExecution will initite the command execution
	// and redirect its IO using args.ExecControls. ExecContext.Environment will
	// contain full (actual)
	Exec(ctx context.Context, name string, args *Execution) (ExecContext, error)
}

type Snapshot interface {
	Snapshot(ctx context.Context, workshop, sk string) error
	Restore(ctx context.Context, workshop, sk string, file *File) error
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
	if cachedStore := cachedBackend(st); cachedStore != nil {
		return cachedStore
	}
	panic("internal error: needing the store before managers have initialized it")
}
