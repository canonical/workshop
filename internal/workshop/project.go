package workshop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/osutil/sys"
)

var (
	ErrProjectLockNotFound    = errors.New("project lock file not found")
	ErrProjectAlreadyExists   = errors.New("project already exists")
	ErrNotProject             = errors.New("not a project (no workshop files found)")
	ErrNoRelativePathsAllowed = errors.New("absolute project path must be used")

	NewProjectId = allocateProjectId
)

const (
	ProjectLock = ".workshop.lock"
	// path used in workshop to mount the project directory
	WorkshopProjectPath = "/project"
)

func LockPath(path string) string {
	return filepath.Join(path, ProjectLock)
}

type Project struct {
	Path      string `json:"path"`
	ProjectId string `json:"id"`
}

func (p *Project) Exists() bool {
	exists, dir, _ := osutil.ExistsIsDir(p.Path)
	return exists && dir
}

func (w *Project) Workshop(workshop string) (*File, error) {
	path, err := w.maybeSingleWorkshop()
	if err != nil {
		return nil, err
	}
	if path != "" {
		file, err := readWorkshop(path)
		if err != nil {
			return nil, fmt.Errorf("invalid file %q: %w", path, err)
		}
		if file.Name != workshop {
			return nil, fmt.Errorf("workshop %q not found (only found %q)",
				workshop, file.Name)
		}
		return file, nil
	}

	path = Filepath(w.Path, workshop)
	file, err := readWorkshop(path)
	if err != nil {
		return nil, err
	}

	if file.Name != workshop {
		return nil, fmt.Errorf("%q workshop file must be named %q (now: %q)",
			file.Name, filename(file.Name), filepath.Base(path))
	}
	return file, nil
}

func (w *Project) ReadWorkshops() (map[string]string, error) {
	path, err := w.maybeSingleWorkshop()
	if err != nil {
		return nil, err
	}

	if path != "" {
		file, err := readWorkshop(path)
		if err != nil {
			return nil, fmt.Errorf("invalid file %q: %w", path, err)
		}
		return map[string]string{file.Name: path}, nil
	}

	// *.yaml is the only supported extension for workshop files as the only
	// recommended "official" extension: https://yaml.org/faq.html. Also, having a
	// single way of naming workshop files avoids unneccesary inconsistencies.
	files, err := filepath.Glob(filepath.Join(w.Path, Directory, "*.yaml"))
	if err != nil {
		return nil, err
	}

	workshops := make(map[string]string, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			logger.Noticef("On ReadWorkshops: Cannot stat a workshop file %q: %v", f, err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		name := strings.TrimSuffix(info.Name(), ".yaml")
		workshops[name] = f
	}
	return workshops, nil
}

// Detects a single workshop file path if it exists and is unique.
func (w *Project) maybeSingleWorkshop() (string, error) {
	var path string

	for _, name := range Filenames {
		_, err := os.Stat(filepath.Join(w.Path, name))
		if err == nil {
			if path != "" {
				return "", fmt.Errorf("ambiguous file %q (directory also contains %q)", path, name)
			}

			path = filepath.Join(w.Path, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	if path == "" {
		return "", nil
	}

	files, err := filepath.Glob(filepath.Join(w.Path, Directory, "*.yaml"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	} else if len(files) > 0 {
		return "", fmt.Errorf("multiple workshops found, but %q not in %q subdirectory", path, Directory)
	}

	return path, nil
}

type ProjectTracker struct {
	Projects []Project
}

type TrackResult int

const (
	ProjectError TrackResult = iota
	ProjectFound
	ProjectMoved
	ProjectAdded
)

// Track attempts to locate a known project that contains the given path.
// If unsuccessful, it creates a new project and begins tracking it.
// Moved projects will be updated with the new path,
// whereas copied projects will receive a new project ID.
func (t *ProjectTracker) Track(path string) (*Project, TrackResult, error) {
	if !filepath.IsAbs(path) {
		return nil, ProjectError, ErrNoRelativePathsAllowed
	}

	path, err := ancestorProject(path)
	if err != nil {
		return nil, ProjectError, err
	}

	id, err := readProjectId(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return t.writeProjectId(path)
		}
		return nil, ProjectError, err
	}

	project, result, err := t.maybeFindProject(path, id)
	if err == nil && project == nil {
		return t.createProjectWithId(path, id)
	}
	return project, result, err
}

// ancestorProject returns an existing project which contains the given path,
// or the path itself if there is no such project.
func ancestorProject(child string) (string, error) {
	child, err := filepath.EvalSymlinks(child)
	if err != nil {
		return "", err
	}

	path := child
	for {
		ok, isDir, err := osutil.ExistsIsDir(path)
		if err != nil {
			return "", err
		}
		if ok && isDir {
			_, err := readProjectId(path)
			if err == nil || isProject(path) {
				return filepath.Clean(path), nil
			}
		}

		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}

	return child, nil
}

// Read a project id from projectDir (.workshop.lock)
func readProjectId(projectDir string) (string, error) {
	buf, err := os.ReadFile(LockPath(projectDir))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (t *ProjectTracker) writeProjectId(path string) (*Project, TrackResult, error) {
	// Try to recover .lock file for this project
	// if it existed before and was accidentally removed.
	idx := slices.IndexFunc(t.Projects, func(p Project) bool { return p.Path == path })
	if idx >= 0 {
		if err := t.Projects[idx].updateLock(); err != nil {
			return nil, ProjectError, err
		}
		return &t.Projects[idx], ProjectFound, nil
	}

	// No project found. If there is at least one workshop definition,
	// we consider the path as a project and create a project ID.
	if !isProject(path) {
		return nil, ProjectError, ErrNotProject
	}
	return t.createProject(path)
}

func (t *ProjectTracker) maybeFindProject(path, id string) (*Project, TrackResult, error) {
	idx := slices.IndexFunc(t.Projects, func(p Project) bool { return p.ProjectId == id })
	if idx < 0 {
		return nil, ProjectError, nil
	}
	if t.Projects[idx].Path == path {
		return &t.Projects[idx], ProjectFound, nil
	}

	// Existing project was moved or copied.
	_, err := os.Stat(t.Projects[idx].Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Moved: keep ID but update path.
			t.Projects[idx].Path = path
			return &t.Projects[idx], ProjectMoved, nil
		}
		return nil, ProjectError, err
	}

	// Copied: generate a new project ID and overwrite the copied .lock file.
	return t.createProject(path)
}

func (t *ProjectTracker) createProject(path string) (*Project, TrackResult, error) {
	id, err := NewProjectId()
	if err != nil {
		return nil, ProjectError, err
	}

	project := Project{Path: path, ProjectId: id}
	if err = project.updateLock(); err != nil {
		return nil, ProjectError, err
	}

	t.Projects = append(t.Projects, project)
	return &project, ProjectAdded, nil
}

func (t *ProjectTracker) createProjectWithId(path, id string) (*Project, TrackResult, error) {
	// If there is at least one workshop definition,
	// we consider the path as a project and use the given ID.
	if !isProject(path) {
		return nil, ProjectError, ErrNotProject
	}

	project := Project{ProjectId: id, Path: path}
	t.Projects = append(t.Projects, project)
	return &project, ProjectAdded, nil
}

// A directory is a project if it has at least one workshop definition.
func isProject(dir string) bool {
	files, err := filepath.Glob(filepath.Join(dir, Directory, "*.yaml"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Noticef("On IsProject: %v", err)
		}
		files = nil
	}

	// Special case for <PROJECT>/.workshop/workshop.yaml.
	if len(files) == 0 && filepath.Base(dir) == Directory && isProject(filepath.Dir(dir)) {
		return false
	}
	for _, name := range Filenames {
		files = append(files, filepath.Join(dir, name))
	}

	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logger.Noticef("On IsProject: %v", err)
			}
		} else if info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func (w *Project) updateLock() error {
	lock, err := os.Create(LockPath(w.Path))
	if err != nil {
		return err
	}
	defer lock.Close()

	// get the desired ownership
	info, err := os.Stat(w.Path)
	if err != nil {
		return err
	}

	uid, gid, err := sys.FileOwner(info)
	if err != nil {
		return err
	}

	if err = sys.Chown(lock, uid, gid); err != nil {
		return err
	}

	_, err = lock.Write([]byte(w.ProjectId))
	if err != nil {
		return err
	}

	return nil
}

func allocateProjectId() (string, error) {
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
