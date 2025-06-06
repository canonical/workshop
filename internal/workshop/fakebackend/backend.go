package fakebackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/x-go/randutil"
	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

/* Fake backend implementation for tests */

type ExecFunc func(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error)

type FakeWorkshop struct {
	*workshop.Workshop
	Config             map[string]string
	Devices            map[string]map[string]string
	WorkshopFilesystem *FakeInstanceFs
}

type ExecCall struct {
	Name string
	Args *workshop.Execution
}

type FsCall struct {
	Name string
}

type DownloadCall struct {
	Base string
}

type AttachVolumeCall struct {
	Workshop string
	Name     string
}

type SnapshotCall struct {
	Workshop string
	Snapid   string
}

type RestoreCall struct {
	Workshop string
	Snapid   string
	File     *workshop.File
}

type WorkshopFsCallback func(ctx context.Context, name string) (workshop.WorkshopFs, error)

type FakeWorkshopBackend struct {
	workshopLock sync.Mutex
	// the key is a project-id - workshop name
	Workshops map[string]map[string]*FakeWorkshop
	// workshops put to stash (e.g. during refresh)
	StashedWorkshops map[string]map[string]*FakeWorkshop
	// storage volumes, the key is a volume name
	volumeLock                sync.Mutex
	WorkshopVolumes           map[string]bool
	WorkshopVolumeContents    map[string]string
	WorkshopVolumeMountPoints map[string]string
	// the key is a username
	projects map[string][]workshop.Project

	ExecCallback ExecFunc
	ExecCalls    []*ExecCall

	workshopFsLock     sync.Mutex
	WorkshopFsCallback WorkshopFsCallback
	WorkshopFsCalls    []*FsCall

	downloadLock         sync.Mutex
	DownloadBaseCallback func(ctx context.Context, base string, report *progress.Reporter) error
	DownloadBaseCalls    []*DownloadCall

	AttachVolumeCalls []AttachVolumeCall

	snapshotLock     sync.Mutex
	SnapshotCalls    []SnapshotCall
	SnapshotCallback func(ctx context.Context, workshop string, snapid string) error
	RestoreCalls     []RestoreCall
	RestoreCallback  func(ctx context.Context, workshop string, snapid string, File *workshop.File) error

	BaseDir string
}

func New(baseDir string) (*FakeWorkshopBackend, error) {
	var be FakeWorkshopBackend
	be.Workshops = make(map[string]map[string]*FakeWorkshop)
	be.StashedWorkshops = make(map[string]map[string]*FakeWorkshop)
	be.WorkshopVolumes = make(map[string]bool)
	be.WorkshopVolumeContents = make(map[string]string)
	be.WorkshopVolumeMountPoints = make(map[string]string)
	be.projects = make(map[string][]workshop.Project)

	be.ExecCallback = DoExecDefault
	be.BaseDir = baseDir

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
	all := map[string][]workshop.Project{}
	for name, prjs := range f.projects {
		all[name] = prjs
	}
	return all, nil
}

func (f *FakeWorkshopBackend) project(user, id string) *workshop.Project {
	prjs := f.projects[user]
	idx := slices.IndexFunc(prjs, func(p workshop.Project) bool { return p.ProjectId == id })
	if idx != -1 {
		return &prjs[idx]
	}
	return nil
}

func (f *FakeWorkshopBackend) LaunchOrRebuildWorkshop(ctx context.Context, file *workshop.File) error {
	user, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	prj := f.project(user, projectId)

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	if f.Workshops[projectId] == nil {
		f.Workshops[projectId] = make(map[string]*FakeWorkshop)
	}

	ws := &FakeWorkshop{}

	if wpe, ok := f.Workshops[projectId][file.Name]; ok {
		// rebuild the workshop
		ws = wpe
		ws.File = file
		ws.Base = file.Base
	} else {
		ws.Workshop = &workshop.Workshop{Backend: f,
			Name:    file.Name,
			Running: false,
			Project: *prj,
			Base:    file.Base,
			File:    file,
		}
		f.Workshops[projectId][file.Name] = ws
	}

	ws.WorkshopFilesystem, err = NewWorkshopFs(f.BaseDir)
	if err != nil {
		return err
	}

	ws.Config = make(map[string]string)
	ws.Config[workshop.ConfigWorkshopSdks] = `{}`
	ws.Devices = make(map[string]map[string]string)

	ws.Sdks = make(map[string]sdk.Setup)
	ws.Profiles = make(map[string]workshop.SdkProfile, 0)

	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshop(ctx context.Context, name string) error {
	user, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	prj := f.project(user, projectId)

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	if _, ok := f.Workshops[prj.ProjectId][name]; !ok {
		return workshop.ErrWorkshopNotLaunched
	}

	delete(f.Workshops[prj.ProjectId], name)
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

	wfs, err := f.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	mnt, err := wfs.(*FakeInstanceFs).Fs.(*afero.BasePathFs).RealPath(mount.Where)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(mnt), 0755); err != nil {
		return err
	}

	if err := os.Symlink(mount.What, mnt); err != nil {
		return err
	}

	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	f.Workshops[projectId][name].Devices[mount.Name] = map[string]string{"type": "disk", "source": mount.What,
		"path": mount.Where}
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

	where, err := wfs.(*FakeInstanceFs).Fs.(*afero.BasePathFs).RealPath(device["path"])
	if err != nil {
		return err
	}

	return os.Remove(where)
}

func (f *FakeWorkshopBackend) AddWorkshopConfig(ctx context.Context, name string, item *workshop.WorkshopConfigValue) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	f.Workshops[projectId][name].Config[item.Name] = item.Value
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshopConfig(ctx context.Context, name string, key string) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	delete(f.Workshops[projectId][name].Config, key)
	return nil
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

	var c map[string]sdk.Setup
	if err := json.Unmarshal([]byte(f.Workshops[projectId][name].Config[workshop.ConfigWorkshopSdks]), &c); err != nil {
		return nil, err
	}
	wp.Sdks = c
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

func (f *FakeWorkshopBackend) GetWorkshopsByConfig(ctx context.Context, filter workshop.WorkshopConfigFilter) ([]*workshop.Workshop, error) {
	f.workshopLock.Lock()
	defer f.workshopLock.Unlock()

	res := make([]*workshop.Workshop, 0)
	for _, i := range f.Workshops {
		for _, j := range i {
			if filter(j.Config) {
				res = append(res, j.Workshop)
			}
		}
	}
	return res, nil
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

func (s *FakeWorkshopBackend) WorkshopFs(ctx context.Context, name string) (workshop.WorkshopFs, error) {
	s.workshopFsLock.Lock()
	s.WorkshopFsCalls = append(s.WorkshopFsCalls, &FsCall{Name: name})
	if s.WorkshopFsCallback != nil {
		defer s.workshopFsLock.Unlock()
		return s.WorkshopFsCallback(ctx, name)
	}
	s.workshopFsLock.Unlock()

	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return nil, err
	}

	s.workshopLock.Lock()
	defer s.workshopLock.Unlock()

	fs, exists := s.Workshops[projectId][name]
	if !exists {
		return nil, fmt.Errorf(`%q filesystem is not available`, name)
	}
	return fs.WorkshopFilesystem, nil
}

func (f *FakeWorkshopBackend) Exec(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
	f.ExecCalls = append(f.ExecCalls, &ExecCall{name, args})
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

	if s.StashedWorkshops[projectId]["stash-"+name] == nil {
		return fmt.Errorf("stashed workshop %q not found", name)
	}
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

	wp := s.StashedWorkshops[projectId]["stash-"+name]
	if wp == nil {
		return fmt.Errorf("stashed workshop %q not found", name)
	}
	wp.Name = name
	s.Workshops[projectId][name] = wp
	wp.Running = true

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

	if s.StashedWorkshops[projectId] == nil {
		s.StashedWorkshops[projectId] = make(map[string]*FakeWorkshop)
	}
	wp.Running = false

	wcpy := *wp.Workshop
	stashed := *wp
	stashed.Workshop = &wcpy
	stashed.Name = "stash-" + name

	s.StashedWorkshops[projectId][stashed.Name] = &stashed
	return nil
}

func (s *FakeWorkshopBackend) CreateVolume(ctx context.Context, name string) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	if s.WorkshopVolumes[name] {
		return workshop.ErrVolumeAlreadyExists
	}

	vfs := filepath.Join(s.BaseDir, "volumes", name)
	if err := os.MkdirAll(vfs, 0755); err != nil {
		return err
	}

	s.WorkshopVolumeContents[name] = vfs
	s.WorkshopVolumes[name] = true
	return nil
}

func (s *FakeWorkshopBackend) AttachVolume(ctx context.Context, wp, name, where string, ro bool) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	s.AttachVolumeCalls = append(s.AttachVolumeCalls, AttachVolumeCall{Workshop: wp, Name: name})

	wfs, err := s.WorkshopFs(ctx, wp)
	if err != nil {
		return err
	}
	defer wfs.Close()

	mnt, err := wfs.(*FakeInstanceFs).Fs.(*afero.BasePathFs).RealPath(where)
	if err != nil {
		return err
	}

	volumeFs := s.WorkshopVolumeContents[name]
	if volumeFs == "" {
		return fmt.Errorf("volume %q not found", name)
	}

	if err := os.MkdirAll(filepath.Dir(mnt), 0755); err != nil {
		return err
	}

	if err := os.Symlink(volumeFs, mnt); err != nil {
		return err
	}

	s.WorkshopVolumeMountPoints[name] = where
	return nil
}

func (s *FakeWorkshopBackend) DetachVolume(ctx context.Context, wp, name string) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	target := s.WorkshopVolumeMountPoints[name]

	wfs, err := s.WorkshopFs(ctx, wp)
	if err != nil {
		return err
	}
	defer wfs.Close()

	err = wfs.Remove(target)
	delete(s.WorkshopVolumeMountPoints, name)
	return err
}

// ImportVolume imports a tarball into the volume. The tarball must be a valid
// directory (unpacked so that the tests could provide a temp directory instead
// of a packed tarball).
func (s *FakeWorkshopBackend) ImportVolume(ctx context.Context, name string, tarball string) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	if s.WorkshopVolumes[name] {
		return workshop.ErrVolumeAlreadyExists
	}
	s.WorkshopVolumeContents[name] = tarball
	s.WorkshopVolumes[name] = true
	return nil
}

func (s *FakeWorkshopBackend) DeleteVolume(ctx context.Context, name string) error {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	delete(s.WorkshopVolumes, name)
	delete(s.WorkshopVolumeContents, name)
	return nil
}

func (s *FakeWorkshopBackend) Volume(ctx context.Context, name string) (workshop.VolumeInfo, error) {
	s.volumeLock.Lock()
	defer s.volumeLock.Unlock()

	if !s.WorkshopVolumes[name] {
		return workshop.VolumeInfo{}, workshop.ErrVolumeNotFound
	}

	meta, err := os.ReadFile(filepath.Join(s.WorkshopVolumeContents[name], "meta", "sdk.yaml"))
	if err != nil {
		return workshop.VolumeInfo{}, err
	}

	return workshop.VolumeInfo{Name: name, Config: map[string]string{workshop.ConfigVolumeMeta: string(meta)}}, nil
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

func (b *FakeWorkshopBackend) Download(ctx context.Context, base string, report *progress.Reporter) error {
	b.downloadLock.Lock()
	defer b.downloadLock.Unlock()

	b.DownloadBaseCalls = append(b.DownloadBaseCalls, &DownloadCall{Base: base})
	if b.DownloadBaseCallback != nil {
		return b.DownloadBaseCallback(ctx, base, report)
	}
	return nil
}

func (s *FakeWorkshopBackend) Snapshot(ctx context.Context, name, snapid string) error {
	s.snapshotLock.Lock()
	defer s.snapshotLock.Unlock()

	s.SnapshotCalls = append(s.SnapshotCalls, SnapshotCall{Workshop: name, Snapid: snapid})
	if s.SnapshotCallback != nil {
		return s.SnapshotCallback(ctx, name, snapid)
	}
	return nil
}

func (s *FakeWorkshopBackend) Restore(ctx context.Context, name, snapid string, file *workshop.File) error {
	wp, err := s.Workshop(ctx, name)
	if err != nil {
		return err
	}

	sdks := wp.SdksByInstallOrder()
	lastIntact := slices.IndexFunc(sdks, func(s sdk.Setup) bool { return workshop.SnapshotId(name, s.Name) == snapid })
	if lastIntact < 0 {
		return fmt.Errorf("invalid snapshot %q", snapid)
	}
	unwantedSdks := sdks[lastIntact+1:]

	fs, err := s.WorkshopFs(ctx, name)
	if err != nil {
		return err
	}
	defer fs.Close()

	// Remove project mount
	if err := fs.Remove(workshop.WorkshopProjectPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Remove SDKs from after the snapshot.
	for _, sk := range unwantedSdks {
		if err = fs.RemoveAll(sdk.SdkDir(sk.Name)); err != nil {
			return err
		}
		delete(wp.Sdks, sk.Name)
	}
	value, err := json.Marshal(wp.Sdks)
	if err != nil {
		return err
	}
	item := &workshop.WorkshopConfigValue{Name: workshop.ConfigWorkshopSdks, Value: string(value)}
	if err := s.AddWorkshopConfig(ctx, name, item); err != nil {
		return err
	}

	wp.File = file

	s.snapshotLock.Lock()
	defer s.snapshotLock.Unlock()

	s.RestoreCalls = append(s.RestoreCalls, RestoreCall{Workshop: name, Snapid: snapid, File: file})
	if s.RestoreCallback != nil {
		return s.RestoreCallback(ctx, name, snapid, file)
	}
	return nil
}

/* Fake workshop fs implementation for tests */

type FakeInstanceFs struct {
	afero.Fs
}

func NewWorkshopFs(baseDir string) (*FakeInstanceFs, error) {
	var fs FakeInstanceFs
	osfs := afero.NewOsFs()
	rndstring := randutil.RandomString(10)
	wfspath := filepath.Join(baseDir, rndstring)
	err := os.MkdirAll(wfspath, 0700)
	if err != nil {
		return nil, err
	}
	fs.Fs = afero.NewBasePathFs(osfs, wfspath)
	return &fs, nil
}

func (w *FakeInstanceFs) Symlink(source, target string) error {
	return w.Fs.(*afero.BasePathFs).SymlinkIfPossible(source, target)
}

func (w *FakeInstanceFs) ReadLink(p string) (string, error) {
	return w.Fs.(*afero.BasePathFs).ReadlinkIfPossible(p)
}

func (w *FakeInstanceFs) Close() {
}
