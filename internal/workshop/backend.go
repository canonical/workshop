package workshop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/user"
	"time"

	"github.com/gorilla/websocket"

	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/progress"
)

type ContextKeyProjectId string
type ContextKeyUser string

type WorkshopConfigFilter func(config map[string]string) bool

const (
	ContextProjectId = ContextKeyProjectId("project-id")
	ContextUser      = ContextKeyUser("user")
)

var (
	ErrWorkshopNotLaunched = errors.New("workshop not launched")
	ErrVolumeNotFound      = errors.New("volume not found")
	ErrVolumeAlreadyExists = errors.New("volume already exists")
	ErrSdkProfileNotFound  = errors.New("sdk profile not found")

	LookupUsername = user.Lookup
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

var StashNamePrefix string = "stash-"

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

type VolumeInfo struct {
	Name   string
	Config map[string]string
}

type VolumeManager interface {
	// Create a temporary storage volume for the workshop. It does not
	// mount the device to the workshop, it must be mounted to the required
	// workshop as a separate operation.
	CreateVolume(ctx context.Context, name string) error

	// Import a tarball into the volume. The tarball must be a valid tarball filepath.
	ImportVolume(ctx context.Context, name string, tarball string) error

	// Attach the volume to the workshop. The volume must be created before.
	AttachVolume(ctx context.Context, wp, name, what string) error

	// Detach the volume from the workshop.
	DetachVolume(ctx context.Context, wp, name string) error

	// Delete a temporary storage volume for the workshop. It does not
	// unmount the volume from the workshop if mounted.
	DeleteVolume(ctx context.Context, name string) error

	// Get the volume information.
	Volume(ctx context.Context, name string) (VolumeInfo, error)
}

type BaseImageManager interface {
	Download(ctx context.Context, base string, report *progress.Reporter) error
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
	WorkshopFs(ctx context.Context, name string) (WorkshopFs, error)

	// Returns a list of workshops for the project in context.
	ProjectWorkshops(ctx context.Context) ([]*Workshop, error)

	// Launch a barebone workshop instance using the base provided.
	LaunchWorkshop(ctx context.Context, file *File) error

	// Delete workshop. Stop the workshop forcefully if not in Stopped before deleting
	RemoveWorkshop(ctx context.Context, name string) error

	// Starts a workshop and waits until it is ready
	// to accept commands
	StartWorkshop(ctx context.Context, name string) error

	// Stops workshop gracefully (i.e. waits for the graceful instance and all
	// its running services termination) unless force is used.
	StopWorkshop(ctx context.Context, name string, force bool) error

	// Adds a workshop mount described by the properties.
	AddWorkshopMount(ctx context.Context, name string, device Mount) error

	// Removes a workshop mount.
	RemoveWorkshopMount(ctx context.Context, name string, device string) error

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

func FakeUserLookup(f func(name string) (*user.User, error)) func() {
	oldUserLookup := LookupUsername
	LookupUsername = f
	return func() { LookupUsername = oldUserLookup }
}
