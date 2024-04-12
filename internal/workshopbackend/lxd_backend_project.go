package workshopbackend

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/spf13/afero"
)

func LxdProjectName(user string) string {
	return "workshop." + user
}

func LxdProjectUser(project string) string {
	parts := strings.Split(project, ".")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func LxdSystemProjectName(user string) string {
	return LxdProjectName(user) + ".system"
}

// Initialise the Workshop project namespace.
func InitProject(conn lxd.InstanceServer, username string) error {
	if username == "" {
		return fmt.Errorf("cannot init LXD project: username is empty")
	}
	if err := createOrLoadLxdProject(conn, LxdProjectName(username)); err != nil {
		return err
	}

	if err := createOrLoadLxdProject(conn, LxdSystemProjectName(username)); err != nil {
		return err
	}
	return nil
}

func createOrLoadLxdProject(conn lxd.InstanceServer, projectName string) error {
	if _, _, err := conn.GetProject(projectName); err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return conn.CreateProject(api.ProjectsPost{
				ProjectPut: api.ProjectPut{
					Config: map[string]string{
						"features.images":          "true",
						"features.profiles":        "true",
						"features.storage.volumes": "true",
					},
				},
				Name: projectName,
			})
		} else {
			return err
		}
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

func (s *LxdBackend) loadProjectFromPath(client lxd.InstanceServer, ctx context.Context, path string) (*Project, error) {
	user, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", ContextUser)
	}

	pId, err := projectId(path)
	lockNotFound := false
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if errors.Is(err, os.ErrNotExist) {
		lockNotFound = true
	}

	lxdPrj, _, err := client.GetProject(LxdProjectName(user))
	if err != nil {
		return nil, err
	}

	projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
	if err != nil {
		return nil, err
	}

	// did we find a .workshop.lock in the path?
	if lockNotFound {
		// try to recover .workshop.lock file for this project
		// if it existed before and was accidentally removed
		for _, i := range projects {
			if i.Path == path {
				// save the lock file in the project's location
				if err = i.createProjectLock(); err != nil {
					return nil, err
				}
				return i, nil
			}
		}
	} else {
		idx := slices.IndexFunc(projects, func(p *Project) bool { return p.ProjectId == pId })
		if idx != -1 {
			return projects[idx], nil
		} else {
			// The lock is found, but the project is not tracked. This could happen
			// if LXD was removed and reinstalled. Track the project again.
			var prj = &Project{Path: path, ProjectId: pId}
			return prj, s.trackProject(client, ctx, prj)
		}
	}
	return nil, ErrProjectNotFound
}

func (s *LxdBackend) trackProject(client lxd.InstanceServer, ctx context.Context, prj *Project) error {
	user, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", ContextUser)
	}

	lxdPrj, etag, err := client.GetProject(LxdProjectName(user))
	if err != nil {
		return err
	}

	projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(projects, func(p *Project) bool { return p.ProjectId == prj.ProjectId })
	if idx == -1 {
		projects = append(projects, prj)
	} else {
		projects[idx] = prj
	}

	projectsJson, err := saveProjects(projects)
	if err != nil {
		return err
	}
	lxdPrj.ProjectPut.Config["user.workshop.projects"] = projectsJson

	return client.UpdateProject(LxdProjectName(user), lxdPrj.ProjectPut, etag)
}

func (s *LxdBackend) updateWorkshopsProjectPath(conn lxd.InstanceServer, ctx context.Context, existingProject *Project) error {
	workshops, err := s.filterLxdInstancesByConfig(conn, NewWorkshopConfigFilter(ProjectIdConfig, existingProject.ProjectId))
	if err != nil {
		return err
	}

	for _, i := range workshops {
		project := Mount(ProjectPathDevice, existingProject.Path, WorkshopProjectPath)
		err = s.AddWorkshopDevice(ctx, WorkshopName(i.Name), project)
		if err != nil {
			return fmt.Errorf("cannot update workshop \"%v\" project directory", i.Name)
		}
	}
	return nil
}

func (s *LxdBackend) findProjectPathFromBindMounts(conn lxd.InstanceServer, ctx context.Context, p *Project) (path string, err error) {
	workshops, err := s.filterLxdInstancesByConfig(conn, NewWorkshopConfigFilter(ProjectIdConfig, p.ProjectId))
	if err != nil {
		return "", err
	}

	/* memFs to story temporary results of the commands execution output */
	memFs := afero.NewMemMapFs()
	for _, i := range workshops {
		// attempt to execute the command only in a running instance
		if i.StatusCode != api.Ready && i.StatusCode != api.Running {
			continue
		}

		/* Take the first instance from the group, we need any running
		and ready to execute commands to validate the project directory */
		out, err := memFs.Create(i.Name)
		if err != nil {
			return "", err
		}
		defer out.Close()

		/* Get the mount point device/directory from findmnt and extract the path without a device
		using awk */
		args := Execution{
			ExecArgs: ExecArgs{
				UserId:  0,
				GroupId: 0,
				Command: []string{"bash", "-c",
					"findmnt --mountpoint /project -o source -n | awk -F\"[][]\" '{printf $2}'"},
				WorkDir: "/",
			},
			ExecControls: ExecControls{
				Stdin:  nil,
				Stdout: out,
				Stderr: out,
			},
		}

		execCtx := context.WithValue(ctx, ContextProjectId, p.ProjectId)
		meta, err := s.execCommand(conn, execCtx, WorkshopName(i.Name), &args)
		if err == nil {
			err = meta.WaitExecution(ctx)
			if err != nil {
				outbuf, _ := afero.ReadFile(memFs, out.Name())
				logger.Debugf("cannot check %q bind-mounts: %v, findmnt output: %s", i.Name, err, string(outbuf))
				continue
			}
		} else {
			logger.Debugf("cannot check %q bind-mounts: %v", i.Name, err)
			continue
		}

		/* Process the findmnt results */
		if currentPath, err := afero.ReadFile(memFs, i.Name); err == nil {
			/* check if the path is not //deleted, i.e. the project directory still exists on the host */
			if ok, isDir, err := osutil.ExistsIsDir(string(currentPath)); ok && isDir {
				return string(currentPath), nil
			} else if err != nil && !osutil.IsDirNotExist(err) {
				return "", nil
			}
		}
	}
	return "", nil
}

// Ensures that every project has a valid existing path. If not, tries to
// recover the path from the actual bind mount of the '/project'. If recovery
// went unsuccessful, removes the project from the list.
func (s *LxdBackend) maybeRecoverProjectPaths(client lxd.InstanceServer, ctx context.Context, projects []*Project) []*Project {
	return slices.DeleteFunc(projects, func(prj *Project) bool {
		if !prj.Exists() {
			var err error
			// If got here then there is no project directory for the projectId
			// anymore. It can mean moving or deletion happened in the past. Try
			// to recover the new project path
			newPath, _ := s.findProjectPathFromBindMounts(client, ctx, prj)
			if newPath != "" {
				// start tracking this project under a new path
				prj.Path = newPath
				if err = s.trackProject(client, ctx, prj); err == nil {
					// update the workshops configuration with the new path
					_ = s.updateWorkshopsProjectPath(client, ctx, prj)
				}
				return false
			}
			// Could not recover the directory, reconcile the project from the
			// list of projects that we track (only if there are no remaining
			// workshops for this project)
			inst, err := s.filterLxdInstancesByConfig(client, func(config map[string]string) bool {
				return config["user.workshop.project-id"] == prj.ProjectId
			})
			if err == nil && len(inst) == 0 {
				return true
			}
		}
		return false
	})
}

// projectPath returns a project path for the cwd provided
// if cwd is a sub-directory of the project. Otherwise, cwd
// is returned unchanged
func ProjectPath(cwd string) (string, error) {
	path, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", nil
	}

	for {
		var err error
		var ok, isDir bool
		if ok, isDir, err = osutil.ExistsIsDir(path); err == nil && ok && isDir {
			if _, err := projectId(path); err == nil {
				return filepath.Clean(path), nil
			}
		}
		if err != nil {
			return "", err
		}

		if path == string(os.PathSeparator) {
			break
		}
		path = filepath.Join(path, "..", string(os.PathSeparator))
	}

	if cwd, err = filepath.EvalSymlinks(cwd); err != nil {
		return "", err
	}
	return cwd, nil
}

func (s *LxdBackend) CreateOrLoadProject(ctx context.Context, path string) (*Project, bool, error) {
	var err error

	if !filepath.IsAbs(path) {
		return nil, false, ErrNoRelativePathsAllowed
	}

	projectDir, err := ProjectPath(path)
	if err != nil {
		return nil, false, err
	}

	client, err := s.LxdClient(ctx)
	if err != nil {
		return nil, false, err
	}
	defer client.Disconnect()

	// see if we have this project already existing
	if existingProject, err := s.loadProjectFromPath(client, ctx, projectDir); err == nil {
		// the tracked path and the requested path must be the same
		// otherwise it means that the project directory was moved or copied
		// If that is the case, we must update the project's configuration
		// in the LXD user.* key (i.e. track the project path with the existing id)
		if existingProject.Path != projectDir {
			// Was the project directory moved or copied?
			_, err := os.Stat(existingProject.Path)
			copied := true
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, false, err
			} else if errors.Is(err, os.ErrNotExist) {
				copied = false
			}

			if !copied {
				// the directory was moved, so we:
				// 1. Update the new path to the actual and track it
				// 2. Update all the workshops to the new project mount
				existingProject.Path = projectDir
				err := s.trackProject(client, ctx, existingProject)
				if err != nil {
					return nil, false, err
				}
				// also, update configuration of all the project's workshops
				projectCtx := context.WithValue(ctx, ContextProjectId, existingProject.ProjectId)
				return existingProject, false, s.updateWorkshopsProjectPath(client, projectCtx, existingProject)
			} else {
				// the directory was copied, so we:
				// 1. Generate a new project id for the actual path and update .lock file
				// 2. Start tracking the actual path as a new project
				id, err := NewProjectId()
				if err != nil {
					return nil, false, err
				}
				var newPrj = Project{Path: projectDir, ProjectId: id}

				// rewrite the existing lock file with the new project id.
				if err = newPrj.updateProjectLock(); err != nil {
					return nil, false, err
				}
				if err := s.trackProject(client, ctx, &newPrj); err != nil {
					return nil, false, err
				}
				return &newPrj, true, nil
			}
		}
		return existingProject, false, nil
	} else if !errors.Is(err, ErrProjectNotFound) {
		// if there is some error that is unrelated to the
		// project loadOrCreate logic (e.g. failed to connect to LXD)
		// then return the error immediately
		return nil, false, err
	}

	// no project found, try to create one, note there is no ID yet at this stage
	var project = Project{Path: projectDir}
	workshops, err := project.EnumWorkshopFiles()
	if err != nil {
		return nil, false, err
	}

	// no workshops found in the directory provided
	// it means we won't be creating a project
	if len(workshops) == 0 {
		return nil, false, ErrNotAProject
	}

	// If there is at least one workshop, we consider the path
	// as a project and create a new project id
	if project.ProjectId, err = NewProjectId(); err != nil {
		return nil, false, err
	} else {
		// if we allocated a new project ID successfully,
		// we store it in the lock file immediately
		if err = project.createProjectLock(); err != nil {
			// a possible reason to fail here is to try to create
			// a project in a directory where a different user has
			// a project already. That project will not be visible to
			// anyone but the owner.
			return nil, false, err
		}
	}

	// Now, add the project ID to the tracking map
	// stored in a custom user.* key of the LXD project for this user
	if err = s.trackProject(client, ctx, &project); err != nil {
		return nil, false, err
	}

	return &project, true, nil
}

func (s *LxdBackend) loadUserProjects(ctx context.Context, user string) ([]*Project, error) {
	client, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect()

	lxdPrj, etag, err := client.GetProject(LxdProjectName(user))
	if err != nil {
		return nil, err
	}

	projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
	if err != nil {
		return nil, err
	}

	checked := s.maybeRecoverProjectPaths(client, ctx, projects)

	if !reflect.DeepEqual(projects, checked) {
		projectsJson, err := saveProjects(checked)
		if err != nil {
			return nil, err
		}
		lxdPrj.ProjectPut.Config["user.workshop.projects"] = projectsJson
		if err = client.UpdateProject(LxdProjectName(user), lxdPrj.ProjectPut, etag); err != nil {
			return nil, err
		}
	}
	return checked, nil
}

func (s *LxdBackend) Projects(ctx context.Context) (map[string][]*Project, error) {
	if user, ok := ctx.Value(ContextUser).(string); ok {
		projects, err := s.loadUserProjects(ctx, user)
		if err != nil {
			return nil, err
		}
		return map[string][]*Project{user: projects}, nil
	} else {
		// get a default connection without preseting the LXD project as we are
		// going over all the LXD projects to filter the ones managed by
		// workshop and reload every interface connection for every SDK of
		// every workshop
		client, err := lxd.ConnectLXDUnixWithContext(ctx, LxdSock, nil)
		if err != nil {
			return nil, err
		}
		// list all projects for all users if the user is not provided
		lxdProjects, err := client.GetProjects()
		if err != nil {
			return nil, err
		}
		allProjects := make(map[string][]*Project)
		for _, lxdProject := range lxdProjects {
			username := LxdProjectUser(lxdProject.Name)
			if _, err = LookupUsername(username); err != nil {
				continue
			}
			// if the project is created by workshop, the key must be present
			if _, ok := lxdProject.Config["user.workshop.projects"]; ok {
				prjctx := context.WithValue(ctx, ContextUser, username)

				projects, err := s.loadUserProjects(prjctx, username)
				if err != nil {
					return nil, err
				}

				allProjects[username] = projects
			}
		}
		return allProjects, nil
	}
}
