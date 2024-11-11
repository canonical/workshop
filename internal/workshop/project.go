package workshop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
)

var (
	ErrProjectNotFound        = errors.New("project not found")
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
	path := Filepath(w.Path, workshop)

	buf, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("workshop definition %q not found", path)
		}
		return nil, err
	}

	file, err := readWorkshop(buf)
	if err != nil {
		return nil, err
	}

	fname := filepath.Base(path)
	if Filename(file.Name) != fname {
		return nil, fmt.Errorf("%q workshop file must be named as %q (now: %s)", file.Name, Filename(file.Name), fname)
	}
	return file, nil
}

func (w *Project) ReadWorkshops() ([]string, error) {
	// *.yaml is the only supported extension for workshop files as the only
	// recommended "official" extension: https://yaml.org/faq.html. Also, having a
	// single way of naming workshop files avoids unneccesary inconsistencies.
	files, err := filepath.Glob(filepath.Join(w.Path, Directory, "workshop.*.yaml"))
	if err != nil {
		return nil, err
	}

	var workshops = make([]string, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			logger.Noticef("On ReadWorkshops: Cannot stat a workshop file %q: %v", f, err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		var name = strings.TrimSuffix(strings.TrimPrefix(info.Name(), "workshop."), ".yaml")
		workshops = append(workshops, name)
	}
	return workshops, nil
}

func (w *Project) UpdateProjectLock() error {
	return w.createLock()
}

func (w *Project) CreateProjectLock() error {
	if osutil.FileExists(LockPath(w.Path)) {
		return ErrProjectAlreadyExists
	}
	return w.createLock()
}

func (w *Project) createLock() error {
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

	uid, gid := 0, 0
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = int(stat.Uid)
		gid = int(stat.Gid)
	}

	if err = os.Chown(LockPath(w.Path), uid, gid); err != nil {
		return err
	}

	_, err = lock.Write([]byte(w.ProjectId))
	if err != nil {
		return err
	}

	return nil
}

// Read a project id from projectDir (.workshop.lock)
func ProjectId(projectDir string) (string, error) {
	buf, err := os.ReadFile(LockPath(projectDir))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func allocateProjectId() (string, error) {
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
