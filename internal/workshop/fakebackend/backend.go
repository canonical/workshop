package fakebackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

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

type WorkshopFsCallback func(ctx context.Context, name string) (workshop.WorkshopFs, error)

type FakeWorkshopBackend struct {
	// the key is a project-id - workshop name
	Workshops map[string]map[string]*FakeWorkshop
	// workshops put to stash (e.g. during refresh)
	StashedWorkshops map[string]map[string]*FakeWorkshop
	// storage volumes, the key is a volume name
	WorkshopVolumes           map[string]bool
	WorkshopVolumeContents    map[string]map[string]bool
	WorkshopVolumeMountPoints map[string]string
	// the key is a username
	projects map[string][]workshop.Project

	ExecCallback ExecFunc
	ExecCalls    []*ExecCall

	WorkshopFsCallback WorkshopFsCallback
	WorkshopFsCalls    []*FsCall

	DownloadBaseCallback func(ctx context.Context, base string, report *progress.Reporter) error
	DownloadBaseCalls    []*DownloadCall

	BaseDir string
}

func New(baseDir string) (*FakeWorkshopBackend, error) {
	var be FakeWorkshopBackend
	be.Workshops = make(map[string]map[string]*FakeWorkshop)
	be.StashedWorkshops = make(map[string]map[string]*FakeWorkshop)
	be.WorkshopVolumes = make(map[string]bool)
	be.WorkshopVolumeContents = make(map[string]map[string]bool)
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

func (f *FakeWorkshopBackend) LaunchWorkshop(ctx context.Context, file *workshop.File) error {
	user, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	prj := f.project(user, projectId)

	if f.Workshops[projectId] == nil {
		f.Workshops[projectId] = make(map[string]*FakeWorkshop)
	}
	if _, ok := f.Workshops[projectId][file.Name]; ok {
		return errors.New("workshop exists")
	}

	ws := &FakeWorkshop{}
	ws.WorkshopFilesystem, err = NewWorkshopFs(f.BaseDir)
	if err != nil {
		return err
	}
	ws.Workshop = &workshop.Workshop{Backend: f,
		Name:    file.Name,
		Running: false,
		Project: *prj,
		Base:    file.Base,
		File:    file,
	}

	ws.Config = make(map[string]string)
	ws.Config[workshop.ConfigWorkshopSdks] = `{}`
	ws.Devices = make(map[string]map[string]string)

	ws.Sdks = make(map[string]sdk.Setup)
	ws.Profiles = make(map[string]workshop.SdkProfile, 0)

	f.Workshops[projectId][file.Name] = ws
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshop(ctx context.Context, name string) error {
	user, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}

	prj := f.project(user, projectId)

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

func (f *FakeWorkshopBackend) AddWorkshopMount(ctx context.Context, name string, props workshop.Mount) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}
	f.Workshops[projectId][name].Devices[props.Name] = map[string]string{"type": "disk", "source": props.What,
		"path": props.Where}
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshopMount(ctx context.Context, name string, device string) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}
	delete(f.Workshops[projectId][name].Devices, device)
	return nil
}

func (f *FakeWorkshopBackend) AddWorkshopConfig(ctx context.Context, name string, item *workshop.WorkshopConfigValue) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}
	f.Workshops[projectId][name].Config[item.Name] = item.Value
	return nil
}

func (f *FakeWorkshopBackend) RemoveWorkshopConfig(ctx context.Context, name string, key string) error {
	_, projectId, err := f.userProject(ctx)
	if err != nil {
		return err
	}
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

	var workshops = make([]*workshop.Workshop, 0)
	for _, i := range f.Workshops[projectId] {
		ws, _ := f.Workshop(ctx, i.Name)
		workshops = append(workshops, ws)
	}
	return workshops, nil
}

func (f *FakeWorkshopBackend) GetWorkshopsByConfig(ctx context.Context, filter workshop.WorkshopConfigFilter) ([]*workshop.Workshop, error) {
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
	old := s.WorkshopFsCallback
	s.WorkshopFsCallback = c
	return func() {
		s.WorkshopFsCallback = old
	}
}

func (s *FakeWorkshopBackend) WorkshopFs(ctx context.Context, name string) (workshop.WorkshopFs, error) {
	s.WorkshopFsCalls = append(s.WorkshopFsCalls, &FsCall{Name: name})
	if s.WorkshopFsCallback != nil {
		return s.WorkshopFsCallback(ctx, name)
	}

	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return nil, err
	}
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
	return nil
}

func (s *FakeWorkshopBackend) UnstashWorkshop(ctx context.Context, name string) error {
	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return err
	}

	wp := s.StashedWorkshops[projectId][workshop.StashNamePrefix+name]
	if wp == nil {
		return fmt.Errorf("stashed workshop %q not found", name)
	}
	delete(s.StashedWorkshops[projectId], workshop.StashNamePrefix+name)
	wp.Name = name
	s.Workshops[projectId][name] = wp
	return nil
}

func (s *FakeWorkshopBackend) StashWorkshop(ctx context.Context, name string) error {
	_, projectId, err := s.userProject(ctx)
	if err != nil {
		return err
	}

	wp := s.Workshops[projectId][name]
	if wp == nil {
		return workshop.ErrWorkshopNotLaunched
	}

	if s.StashedWorkshops[projectId] == nil {
		s.StashedWorkshops[projectId] = make(map[string]*FakeWorkshop)
	}
	s.StashedWorkshops[projectId][workshop.StashNamePrefix+name] = wp
	wp.Name = workshop.StashNamePrefix + name
	delete(s.Workshops[projectId], name)
	return nil
}

func (s *FakeWorkshopBackend) AttachVolume(ctx context.Context, wp, name, what string) error {
	s.WorkshopVolumeMountPoints[name] = what

	paths := s.WorkshopVolumeContents[name]
	if paths == nil {
		s.WorkshopVolumeContents[name] = map[string]bool{}
		return nil
	}
	wfs, err := s.WorkshopFs(ctx, wp)
	if err != nil {
		return err
	}
	defer wfs.Close()
	for path := range paths {
		if err = wfs.MkdirAll(path, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (s *FakeWorkshopBackend) DetachVolume(ctx context.Context, wp, name string) error {
	target := s.WorkshopVolumeMountPoints[name]

	wfs, err := s.WorkshopFs(ctx, wp)
	if err != nil {
		return err
	}
	defer wfs.Close()

	afero.Walk(wfs, target, func(path string, info fs.FileInfo, err error) error {
		s.WorkshopVolumeContents[name][path] = true
		return nil
	})

	err = wfs.RemoveAll(target)
	delete(s.WorkshopVolumeMountPoints, name)
	return err
}

func (s *FakeWorkshopBackend) CreateVolume(ctx context.Context, name string) error {
	s.WorkshopVolumes[name] = true
	return nil
}

func (s *FakeWorkshopBackend) DeleteVolume(ctx context.Context, name string) error {
	delete(s.WorkshopVolumes, name)
	return nil
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
	b.DownloadBaseCalls = append(b.DownloadBaseCalls, &DownloadCall{Base: base})
	if b.DownloadBaseCallback != nil {
		return b.DownloadBaseCallback(ctx, base, report)
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
