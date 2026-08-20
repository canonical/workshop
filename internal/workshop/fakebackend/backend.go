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

package fakebackend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/x-go/strutil"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

/* Fake backend implementation for tests */

type ExecFunc func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error)

type FakeWorkshop struct {
	*workshop.Workshop
	Devices            map[string]map[string]string
	WorkshopFilesystem fsutil.Fs
	CurrentSnapshot    workshop.Snapshot
}

type FakeVolume struct {
	Kind   string
	What   string
	Mounts []WorkshopVolumeMount
	Size   uint64
}

type FakeSnapshot struct {
	workshop.Snapshot
	Id int
}

type WorkshopVolumeMount struct {
	ProjectId string
	Workshop  string
	Where     string
}

type ExecCall struct {
	Name string
	Args workshop.Execution
}

type FsCall struct {
	Name string
}

type DownloadCall struct {
	Image workshop.BaseImage
}

type AttachVolumeCall struct {
	Workshop string
	Name     string
}

type SnapshotCall struct {
	Workshop string
	Snapshot workshop.Snapshot
}

type LaunchOrRebuildCall struct {
	Workshop string
	File     *workshop.File
	Snapshot workshop.Snapshot
}

type WorkshopFsCallback func(ctx context.Context, name string) (fsutil.Fs, error)

type FakeWorkshopBackend struct {
	workshopLock sync.Mutex
	// the key is a project-id - workshop name
	Workshops map[string]map[string]*FakeWorkshop
	// workshops put to stash (e.g. during refresh)
	StashedWorkshops map[string]map[string]*FakeWorkshop
	// storage volumes, the key is a volume name
	volumeLock sync.Mutex
	Volumes    map[string]FakeVolume
	SdkVolumes map[string]sdk.Meta
	// the key is a username
	projects map[string][]workshop.Project

	execLock     sync.Mutex
	ExecCallback ExecFunc
	ExecCalls    []*ExecCall

	workshopFsLock     sync.Mutex
	WorkshopFsCallback WorkshopFsCallback
	WorkshopFsCalls    []*FsCall

	baseLock             sync.Mutex
	GetBaseCallback      func(ctx context.Context, base string) (workshop.BaseImage, error)
	DownloadBaseCallback func(ctx context.Context, image workshop.BaseImage, report *progress.Reporter) error
	DownloadBaseCalls    []*DownloadCall

	AttachVolumeCalls []AttachVolumeCall

	formatRevision sdk.Revision

	snapshotLock            sync.Mutex
	LaunchOrRebuildCallback func(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error
	LaunchOrRebuildCalls    []LaunchOrRebuildCall
	SnapshotCalls           []SnapshotCall
	SnapshotCallback        func(ctx context.Context, name string, snapshot workshop.Snapshot) error
	Snapshots               []FakeSnapshot
	RemovedSnapshots        []workshop.Snapshot

	BaseDir     string
	SnapshotDir string
}

var _ workshop.Backend = (*FakeWorkshopBackend)(nil)

type FakeNetworkManager struct {
	Addrs []netip.Addr
	Err   error
}

func (f FakeNetworkManager) InterfaceAddrs(context.Context, string) ([]netip.Addr, error) {
	return f.Addrs, f.Err
}

func New(baseDir string) (*FakeWorkshopBackend, error) {
	var be FakeWorkshopBackend
	be.Workshops = make(map[string]map[string]*FakeWorkshop)
	be.StashedWorkshops = make(map[string]map[string]*FakeWorkshop)
	be.Volumes = make(map[string]FakeVolume)
	be.SdkVolumes = make(map[string]sdk.Meta)
	be.projects = make(map[string][]workshop.Project)

	be.ExecCallback = DoExecDefault
	be.BaseDir = baseDir
	be.SnapshotDir = filepath.Join(baseDir, "snapshots")
	if err := os.MkdirAll(be.SnapshotDir, 0755); err != nil {
		return nil, err
	}

	return &be, nil
}

func (s *FakeWorkshopBackend) CreateOrLoadProject(ctx context.Context, path string) (*workshop.Project, bool, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, false, errors.New("user not found")
	}
	if val, ok := s.projects[username]; ok {
		idx := slices.IndexFunc(val, func(p workshop.Project) bool { return p.Path == path })
		if idx != -1 {
			return &val[idx], false, nil
		}
	} else {
		s.projects[username] = make([]workshop.Project, 0)
	}

	prjId, _ := workshop.NewProjectId()
	newPrj := workshop.Project{ProjectId: prjId, Path: path}
	s.projects[username] = append(s.projects[username], newPrj)
	return &newPrj, true, nil
}

func (f *FakeWorkshopBackend) Projects(ctx context.Context) (map[string][]workshop.Project, error) {
	userName, ok := ctx.Value(workshop.ContextUser).(string)
	if ok {
		return map[string][]workshop.Project{userName: f.projects[userName]}, nil
	}
	return maps.Clone(f.projects), nil
}

func (f *FakeWorkshopBackend) project(user, id string) *workshop.Project {
	prjs := f.projects[user]
	idx := slices.IndexFunc(prjs, func(p workshop.Project) bool { return p.ProjectId == id })
	if idx != -1 {
		return &prjs[idx]
	}
	return nil
}

func (s *FakeWorkshopBackend) SetLaunchOrRebuildCallback(c func(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error) func() {
	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	old := s.LaunchOrRebuildCallback
	s.LaunchOrRebuildCallback = c

	return func() {
		s.workshopLock.Lock()
		defer s.workshopLock.Unlock()

		s.LaunchOrRebuildCallback = old
	}
}

func (f *FakeWorkshopBackend) LaunchOrRebuildWorkshop(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	snapId := -1
	if !snapshot.IsBase() {
		f.snapshotLock.Lock()
		idx := f.snapshotIdx(snapshot)
		f.snapshotLock.Unlock()
		if idx < 0 {
			sk := snapshot.Sdks[len(snapshot.Sdks)-1].Name
			return fmt.Errorf("internal error: %q snapshot not found", sk)
		}
		snapId = f.Snapshots[idx].Id
	}

	f.workshopLock.Lock()
	if f.Workshops[projectId] == nil {
		f.Workshops[projectId] = make(map[string]*FakeWorkshop)
	}
	ws, ok := f.Workshops[projectId][file.Name]
	if !ok {
		ws = &FakeWorkshop{}
		f.Workshops[projectId][file.Name] = ws
	}
	f.workshopLock.Unlock()

	if err := f.resetWorkshop(ctx, file, snapshot, ws); err != nil {
		return err
	}

	wfspath, err := os.MkdirTemp(f.BaseDir, "*")
	if err != nil {
		return err
	}
	if snapId >= 0 {
		snapPath := filepath.Join(f.SnapshotDir, fmt.Sprint(snapId))
		if err := copyRegularFilesAndDirs(snapPath, wfspath); err != nil {
			return err
		}
	}
	ws.WorkshopFilesystem = fsutil.NewBasePathFs(wfspath)

	f.snapshotLock.Lock()
	defer f.snapshotLock.Unlock()

	call := LaunchOrRebuildCall{Workshop: ws.Name, File: file, Snapshot: snapshot}
	f.LaunchOrRebuildCalls = append(f.LaunchOrRebuildCalls, call)

	if f.LaunchOrRebuildCallback != nil {
		return f.LaunchOrRebuildCallback(ctx, file, snapshot)
	}
	return nil
}

func (f *FakeWorkshopBackend) resetWorkshop(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot, ws *FakeWorkshop) error {
	if ws.Workshop == nil {
		user, projectId, err := f.userProject(ctx)
		if err != nil {
			return err
		}
		prj := f.project(user, projectId)

		ws.Workshop = &workshop.Workshop{Backend: f,
			Name:     file.Name,
			Running:  false,
			Project:  *prj,
			Format:   f.FormatRevision(),
			Image:    snapshot.Image,
			File:     file,
			Sdks:     map[string]workshop.SdkInstallation{},
			Profiles: map[string]workshop.SdkProfile{},
		}
		ws.Devices = map[string]map[string]string{}
		ws.CurrentSnapshot = snapshot
		return nil
	}

	// Remove project mount
	if _, ok := ws.Devices[workshop.ConfigProjectPathDevice]; ok {
		if err := f.RemoveWorkshopMount(ctx, ws.Name, workshop.ConfigProjectPathDevice); err != nil {
			return err
		}
	}

	// Sanity check for devices and profiles
	if len(ws.Profiles) > 0 {
		names := strutil.Quoted(slices.Collect(maps.Keys(ws.Profiles)))
		return fmt.Errorf("interfaces must be disconnected before building workshop: %s", names)
	}
	devices := slices.Collect(maps.Keys(ws.Devices))
	devices = slices.DeleteFunc(devices, func(name string) bool {
		return strings.HasPrefix(name, "sdk.")
	})
	if len(devices) > 0 {
		names := strutil.Quoted(devices)
		return fmt.Errorf("devices must be detached before rebuilding workshop: %s", names)
	}

	ws.File = file
	ws.Format = f.FormatRevision()
	ws.Image = snapshot.Image
	ws.CurrentSnapshot = snapshot
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshop(ctx context.Context, name string) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	_, ok := f.Workshops[projectId][name]
	delete(f.Workshops[projectId], name)
	f.workshopLock.Unlock()
	if !ok {
		return workshop.ErrWorkshopNotLaunched
	}
	return nil
}

func (s *FakeWorkshopBackend) StartWorkshop(ctx context.Context, name string) error {
	w, err := s.Workshop(ctx, name)
	if err != nil {
		return err
	}
	if w.Running {
		return api.StatusErrorf(http.StatusConflict, "workshop already running")
	}
	w.Running = true
	return nil
}

func (s *FakeWorkshopBackend) StopWorkshop(ctx context.Context, name string, force bool) error {
	w, err := s.Workshop(ctx, name)
	if err != nil {
		return err
	}
	w.Running = false
	return nil
}

func (f *FakeWorkshopBackend) AddWorkshopMount(ctx context.Context, name string, mount workshop.Mount) error {
	if mount.Type != workshop.HostWorkshop {
		return errors.New("fake backend only supports HostWorkshop mounts")
	}

	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	old := f.Workshops[projectId][name].Devices[mount.Name]
	f.workshopLock.Unlock()

	device := map[string]string{"type": "disk", "source": mount.What, "path": mount.Where}
	if maps.Equal(device, old) {
		return nil
	}

	wfs, err := f.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	mnt, err := wfs.FsBackend.(*fsutil.BasePathFs).RealPath(mount.Where)
	if err != nil {
		return err
	}

	if mount.MakeWhere {
		if err := os.MkdirAll(filepath.Dir(mnt), mount.Mode); err != nil {
			return err
		}
	}

	if err := os.Symlink(mount.What, mnt); err != nil {
		return err
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	f.Workshops[projectId][name].Devices[mount.Name] = device
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshopMount(ctx context.Context, name, mount string) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	device, ok := f.Workshops[projectId][name].Devices[mount]
	delete(f.Workshops[projectId][name].Devices, mount)
	f.workshopLock.Unlock()
	if !ok {
		return fmt.Errorf("mount %q not found", mount)
	}

	wfs, err := f.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	where, err := wfs.FsBackend.(*fsutil.BasePathFs).RealPath(device["path"])
	if err != nil {
		return err
	}

	return os.Remove(where)
}

func (f *FakeWorkshopBackend) Workshop(ctx context.Context, name string) (*workshop.Workshop, error) {
	user, projectId, err := f.userProject(ctx)
	if err != nil {
		return nil, err
	}

	project := f.project(user, projectId)
	if project == nil {
		return nil, api.StatusErrorf(404, "project not found")
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	wp := f.Workshops[projectId][name]
	if wp == nil {
		return nil, workshop.ErrWorkshopNotLaunched
	}
	return wp.Workshop, nil
}

func (f *FakeWorkshopBackend) ProjectWorkshops(ctx context.Context) ([]*workshop.Workshop, error) {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return nil, err
	}

	f.workshopLock.Lock()
	var names []string
	for _, i := range f.Workshops[projectId] {
		names = append(names, i.Name)
	}
	f.workshopLock.Unlock()

	var workshops = make([]*workshop.Workshop, 0)
	for _, name := range names {
		ws, _ := f.Workshop(ctx, name)
		workshops = append(workshops, ws)
	}
	return workshops, nil
}

func (s *FakeWorkshopBackend) SetWorkshopFsCallback(c WorkshopFsCallback) func() {
	s.workshopFsLock.Lock()
	defer s.workshopFsLock.Unlock()

	old := s.WorkshopFsCallback
	s.WorkshopFsCallback = c

	return func() {
		s.workshopFsLock.Lock()
		defer s.workshopFsLock.Unlock()

		s.WorkshopFsCallback = old
	}
}

func (s *FakeWorkshopBackend) WorkshopFs(ctx context.Context, name string) (fsutil.Fs, error) {
	s.workshopFsLock.Lock()
	s.WorkshopFsCalls = append(s.WorkshopFsCalls, &FsCall{Name: name})
	if s.WorkshopFsCallback != nil {
		defer s.workshopFsLock.Unlock()
		return s.WorkshopFsCallback(ctx, name)
	}
	s.workshopFsLock.Unlock()

	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return fsutil.Fs{}, err
	}

	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	fs, exists := s.Workshops[projectId][name]
	if !exists {
		return fsutil.Fs{}, fmt.Errorf(`%q filesystem is not available`, name)
	}
	return fs.WorkshopFilesystem, nil
}

func (f *FakeWorkshopBackend) Exec(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
	f.execLock.Lock()
	f.ExecCalls = append(f.ExecCalls, &ExecCall{name, *args})
	f.execLock.Unlock()
	return f.ExecCallback(ctx, name, args)
}

func DoExecDefault(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
	return workshop.ExecContext{
		WaitExecution: func(ctx context.Context) error {
			return nil
		},
	}, nil
}

func (s *FakeWorkshopBackend) RemoveWorkshopStash(ctx context.Context, name string) error {
	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return err
	}

	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	delete(s.StashedWorkshops[projectId], "stash-"+name)
	return nil
}

func (s *FakeWorkshopBackend) UnstashWorkshop(ctx context.Context, name string) error {
	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return err
	}

	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	stash := s.StashedWorkshops[projectId]["stash-"+name]
	if stash == nil {
		return fmt.Errorf("stashed workshop %q not found", name)
	}
	if wp := s.Workshops[projectId][name]; wp != nil && wp.Running {
		return fmt.Errorf("cannot unstash running %q workshop", name)
	}

	wp := copyWorkshop(stash)
	wp.Name = name
	s.Workshops[projectId][name] = wp

	return nil
}

func (s *FakeWorkshopBackend) StashWorkshop(ctx context.Context, name string) error {
	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return err
	}

	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	wp := s.Workshops[projectId][name]
	if wp == nil {
		return workshop.ErrWorkshopNotLaunched
	}
	if wp.Running {
		return fmt.Errorf("cannot stash running %q workshop", name)
	}

	if s.StashedWorkshops[projectId] == nil {
		s.StashedWorkshops[projectId] = make(map[string]*FakeWorkshop)
	}

	stash := copyWorkshop(wp)
	stash.Name = "stash-" + name
	s.StashedWorkshops[projectId][stash.Name] = stash
	return nil
}

func copyWorkshop(wp *FakeWorkshop) *FakeWorkshop {
	wcpy := *wp.Workshop
	wcpy.Sdks = maps.Clone(wcpy.Sdks)
	wcpy.Profiles = maps.Clone(wcpy.Profiles)
	result := *wp
	result.Workshop = &wcpy
	return &result
}

func (s *FakeWorkshopBackend) userProject(ctx context.Context) (string, string, error) {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return "", "", fmt.Errorf("context key project-id not found")
	}

	userName, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return "", "", fmt.Errorf("context key user not found")
	}
	return userName, projectId, nil
}

func (b *FakeWorkshopBackend) GetBase(ctx context.Context, base string) (workshop.BaseImage, error) {
	b.baseLock.Lock()
	defer b.baseLock.Unlock()

	if b.GetBaseCallback != nil {
		return b.GetBaseCallback(ctx, base)
	}
	return workshop.BaseImage{Name: base, Fingerprint: "fakeimage123"}, nil
}

func (b *FakeWorkshopBackend) DownloadBase(ctx context.Context, image workshop.BaseImage, report *progress.Reporter) error {
	b.baseLock.Lock()
	defer b.baseLock.Unlock()

	b.DownloadBaseCalls = append(b.DownloadBaseCalls, &DownloadCall{Image: image})
	if b.DownloadBaseCallback != nil {
		return b.DownloadBaseCallback(ctx, image, report)
	}
	return nil
}

// ImportSdk imports an SDK tarball into the volume. The tarball must be a valid
// directory (unpacked so that the tests could provide a temp directory instead
// of a packed tarball).
func (s *FakeWorkshopBackend) ImportSdk(ctx context.Context, meta sdk.Meta, tarball *os.File) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	name := sdk.VolumeName(meta.Name, meta.Revision)
	if _, ok := s.Volumes[name]; ok {
		return workshop.ErrVolumeAlreadyExists
	}

	info, err := tarball.Stat()
	if err != nil {
		return err
	}

	what := tarball.Name()
	if !info.IsDir() {
		content, err := io.ReadAll(tarball)
		if err != nil {
			return err
		}
		what = string(content)
	}

	s.Volumes[name] = FakeVolume{Kind: "sdk", What: what}
	s.SdkVolumes[name] = meta
	return nil
}

func (b *FakeWorkshopBackend) DeleteSdk(ctx context.Context, setup sdk.Setup) error {
	b.volumeLock.Lock()
	defer b.volumeLock.Unlock()

	what := sdk.VolumeName(setup.Name, setup.Revision)
	volume, ok := b.Volumes[what]
	if !ok {
		return nil
	}

	if len(volume.Mounts) > 0 {
		return workshop.ErrVolumeInUse
	}

	delete(b.Volumes, what)
	delete(b.SdkVolumes, what)
	return nil
}

func (s *FakeWorkshopBackend) Sdks(ctx context.Context) ([]workshop.SdkVolume, error) {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	var sdks []workshop.SdkVolume
	for name, volume := range s.Volumes {
		if volume.Kind != "sdk" {
			continue
		}
		sdks = append(sdks, s.sdkVolume(name, volume))
	}
	return sdks, nil
}

func (s *FakeWorkshopBackend) Sdk(ctx context.Context, setup sdk.Setup) (workshop.SdkVolume, error) {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	name := sdk.VolumeName(setup.Name, setup.Revision)
	volume, ok := s.Volumes[name]
	if !ok {
		return workshop.SdkVolume{}, workshop.ErrVolumeNotFound
	}
	return s.sdkVolume(name, volume), nil
}

func (s *FakeWorkshopBackend) sdkVolume(name string, volume FakeVolume) workshop.SdkVolume {
	workshops := map[string][]string{}
	for _, mount := range volume.Mounts {
		workshops[mount.ProjectId] = append(workshops[mount.ProjectId], mount.Workshop)
	}

	return workshop.SdkVolume{
		Meta:      s.SdkVolumes[name],
		Workshops: workshops,
		Size:      volume.Size,
	}
}

func (b *FakeWorkshopBackend) InstallSdk(ctx context.Context, name string, setup sdk.Setup) error {
	user, projectId, err := b.userProject(ctx)
	if err != nil {
		return err
	}

	b.workshopLock.Lock()
	wp := b.Workshops[projectId][name]
	b.workshopLock.Unlock()
	if _, exist := wp.Sdks[setup.Name]; exist {
		return fmt.Errorf("%q SDK is already installed", setup.Name)
	}

	wp.Sdks[setup.Name] = workshop.SdkInstallation{
		Setup:        setup,
		InstallOrder: len(wp.Sdks) + 1,
		InstalledAt:  workshop.InstallTimeNow().UTC(),
	}

	usr, env, err := osutil.UserAndEnv(user)
	if err != nil {
		return err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	mount := workshop.SdkMount(userDataDir, projectId, name, setup)
	if mount.Type == workshop.Volume {
		return b.attachVolume(ctx, name, mount)
	}
	return b.AddWorkshopMount(ctx, name, mount)
}

func (s *FakeWorkshopBackend) attachVolume(ctx context.Context, name string, mount workshop.Mount) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	s.AttachVolumeCalls = append(s.AttachVolumeCalls, AttachVolumeCall{Workshop: name, Name: mount.What})

	wfs, err := s.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	mnt, err := wfs.FsBackend.(*fsutil.BasePathFs).RealPath(mount.Where)
	if err != nil {
		return err
	}

	volume, ok := s.Volumes[mount.What]
	if !ok {
		return fmt.Errorf("volume %q not found", mount.What)
	}
	entry := WorkshopVolumeMount{ProjectId: projectId, Workshop: name, Where: mount.Where}
	if slices.Contains(volume.Mounts, entry) {
		return fmt.Errorf("volume %q already mounted in %q", mount.What, name)
	}

	if err := os.MkdirAll(filepath.Dir(mnt), 0755); err != nil {
		return err
	}

	if err := os.Symlink(volume.What, mnt); err != nil {
		return err
	}

	volume.Mounts = append(volume.Mounts, entry)
	s.Volumes[mount.What] = volume
	return nil
}

func (b *FakeWorkshopBackend) UninstallSdk(ctx context.Context, name, sk string) error {
	_, projectId, err := b.userProject(ctx)
	if err != nil {
		return err
	}

	b.workshopLock.Lock()
	wp := b.Workshops[projectId][name]
	b.workshopLock.Unlock()
	setup, ok := wp.Sdks[sk]
	if !ok {
		return nil
	}
	delete(wp.Sdks, sk)

	if setup.IsVolume() {
		what := sdk.VolumeName(setup.Name, setup.Revision)
		where := sdk.SdkDir(setup.Name)
		return b.detachVolume(ctx, name, what, where)
	}
	what := workshop.SdkDeviceName(setup.Name)
	return b.RemoveWorkshopMount(ctx, name, what)
}

func (s *FakeWorkshopBackend) detachVolume(ctx context.Context, name, what, where string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	volume, ok := s.Volumes[what]
	if !ok {
		return fmt.Errorf("volume %q not found", what)
	}
	idx := slices.IndexFunc(volume.Mounts, func(m WorkshopVolumeMount) bool {
		return m.ProjectId == projectId && m.Workshop == name && m.Where == where
	})
	if idx < 0 {
		return fmt.Errorf("volume %q not mounted in %q", what, name)
	}

	wfs, err := s.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	err = wfs.Remove(volume.Mounts[idx].Where)
	if err != nil {
		return err
	}

	volume.Mounts = slices.Delete(volume.Mounts, idx, idx+1)
	s.Volumes[what] = volume
	return err
}

func (f *FakeWorkshopBackend) FormatRevision() sdk.Revision {
	if f.formatRevision.Unset() {
		return sdk.R(1)
	}
	return f.formatRevision
}

func (f *FakeWorkshopBackend) SetFormatRevision(format sdk.Revision) func() {
	old := f.formatRevision
	f.formatRevision = format
	return func() { f.formatRevision = old }
}

func (f *FakeWorkshopBackend) Snapshot(ctx context.Context, snapshot workshop.Snapshot) (*workshop.SnapshotInfo, error) {
	f.snapshotLock.Lock()
	idx := f.snapshotIdx(snapshot)
	f.snapshotLock.Unlock()
	if idx < 0 {
		return nil, workshop.ErrSnapshotNotFound
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	workshops := make(map[string][]string)
	for pid, wps := range f.Workshops {
		for name, wp := range wps {
			if wp.CurrentSnapshot.IsBasedOn(snapshot) {
				workshops[pid] = append(workshops[pid], name)
			}
		}
	}
	for pid, wps := range f.StashedWorkshops {
		for name, wp := range wps {
			name = strings.TrimPrefix(name, "stash-")
			if slices.Contains(workshops[pid], name) {
				continue
			}

			if wp.CurrentSnapshot.IsBasedOn(snapshot) {
				workshops[pid] = append(workshops[pid], name)
			}
		}
	}
	return &workshop.SnapshotInfo{Snapshot: snapshot, Workshops: workshops}, nil
}

func (f *FakeWorkshopBackend) TakeSnapshot(ctx context.Context, name string, snapshot workshop.Snapshot) error {
	wfs, err := f.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	wfspath, err := wfs.FsBackend.(*fsutil.BasePathFs).RealPath(".")
	wfs.Close()
	if err != nil {
		return err
	}

	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	wp := f.Workshops[projectId][name]
	f.workshopLock.Unlock()
	if wp == nil {
		return workshop.ErrWorkshopNotLaunched
	}
	wp.CurrentSnapshot = snapshot

	f.snapshotLock.Lock()
	defer f.snapshotLock.Unlock()

	f.SnapshotCalls = append(f.SnapshotCalls, SnapshotCall{Workshop: name, Snapshot: snapshot})
	if f.SnapshotCallback != nil {
		if err := f.SnapshotCallback(ctx, name, snapshot); err != nil {
			return err
		}
	}

	if f.snapshotIdx(snapshot) >= 0 {
		return workshop.ErrSnapshotAlreadyExists
	}

	snapId := 0
	if len(f.Snapshots) > 0 {
		snapId = f.Snapshots[len(f.Snapshots)-1].Id + 1
	}
	f.Snapshots = append(f.Snapshots, FakeSnapshot{Snapshot: snapshot, Id: snapId})

	snapPath := filepath.Join(f.SnapshotDir, fmt.Sprint(snapId))
	return copyRegularFilesAndDirs(wfspath, snapPath)
}

func (f *FakeWorkshopBackend) RemoveSnapshot(ctx context.Context, snapshot workshop.Snapshot) error {
	f.snapshotLock.Lock()
	defer f.snapshotLock.Unlock()

	if idx := f.snapshotIdx(snapshot); idx >= 0 {
		f.RemovedSnapshots = append(f.RemovedSnapshots, snapshot)

		snapId := f.Snapshots[idx].Id
		f.Snapshots = slices.Delete(f.Snapshots, idx, idx+1)
		return os.RemoveAll(filepath.Join(f.SnapshotDir, fmt.Sprint(snapId)))
	}

	return nil
}

func (f *FakeWorkshopBackend) snapshotIdx(snapshot workshop.Snapshot) int {
	return slices.IndexFunc(f.Snapshots, func(s FakeSnapshot) bool {
		return snapshot.Equal(s.Snapshot)
	})
}

func copyRegularFilesAndDirs(source, target string) error {
	return fs.WalkDir(os.DirFS(source), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		local, err := filepath.Localize(path)
		if err != nil {
			return err
		}
		src := filepath.Join(source, local)
		tgt := filepath.Join(target, local)

		if d.Type().IsRegular() {
			return osutil.CopyFile(src, tgt, 0)
		}
		if !d.IsDir() {
			// Ignore symlinks because we use them to simulate mounts.
			return nil
		}
		if err := os.Mkdir(tgt, 0755); !errors.Is(err, os.ErrExist) {
			return err
		}
		return nil
	})
}
