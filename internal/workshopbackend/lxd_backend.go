package workshopbackend

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/gorilla/websocket"
	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	lxd "github.com/canonical/lxd/client"

	"github.com/canonical/lxd/shared/api"
)

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
	Stdin   io.ReadCloser
	Stdout  io.WriteCloser
	Stderr  io.WriteCloser
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

type LxdBackend struct {
}

const (
	LxdSock = "/var/snap/lxd/common/lxd/unix.socket"
)

var (
	ConnectSimpleStreams = lxd.ConnectSimpleStreams
	LookupUsername       = user.Lookup
	NewProjectId         = allocateProjectId
	defaultDevices       = createDefaultDevices

	ErrWorkshopNotFound = errors.New("workshop not found")
	imageServer         = "https://cloud-images.ubuntu.com/releases/"
)

func New() WorkshopBackend {
	server := LxdBackend{}
	return &server
}

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

func allocateProjectId() (string, error) {
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *LxdBackend) loadProject(client lxd.InstanceServer, ctx context.Context, path string) (*Project, error) {

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
		}
	}
	return nil, ErrProjectNotFound
}

func (s *LxdBackend) untrackProject(client lxd.InstanceServer, ctx context.Context, prj *Project) error {
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
		return fmt.Errorf("project %q at %q not found", prj.ProjectId, prj.Path)
	}

	projects = slices.Delete(projects, idx, idx+1)
	projectsJson, err := saveProjects(projects)
	if err != nil {
		return err
	}
	lxdPrj.ProjectPut.Config["user.workshop.projects"] = projectsJson

	return client.UpdateProject(LxdProjectName(user), lxdPrj.ProjectPut, etag)
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
// went unsuccessful, stop project tracking (it is likely that the directory was
// removed already)
func (s *LxdBackend) checkAndRecoverProjectPaths(client lxd.InstanceServer, ctx context.Context, projects []*Project) ([]*Project, error) {
	for idx, prj := range projects {
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
					s.updateWorkshopsProjectPath(client, ctx, prj)
				}
				continue
			}
			// Could not recover the directory, reconcile the project from the
			// list of projects that we track (only if there are no remaining
			// workshops for this project)
			inst, err := s.filterLxdInstancesByConfig(client, func(config map[string]string) bool {
				return config["user.workshop.project-id"] == prj.ProjectId
			})
			if err == nil && len(inst) == 0 {
				if err = s.untrackProject(client, ctx, prj); err != nil {
					return nil, err
				}
				projects = slices.Delete(projects, idx, idx+1)
			}
		}
	}
	return projects, nil
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

	// see if we have this project already existing
	if existingProject, err := s.loadProject(client, ctx, projectDir); err == nil {
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

func (s *LxdBackend) Projects(ctx context.Context) (map[string][]*Project, error) {
	if user, ok := ctx.Value(ContextUser).(string); ok {
		client, err := s.LxdClient(ctx)
		if err != nil {
			return nil, err
		}
		lxdPrj, _, err := client.GetProject(LxdProjectName(user))
		if err != nil {
			return nil, err
		}

		projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
		if err != nil {
			return nil, err
		}

		checked, err := s.checkAndRecoverProjectPaths(client, ctx, projects)
		if err != nil {
			return nil, err
		}
		return map[string][]*Project{user: checked}, nil
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
		for _, prj := range lxdProjects {
			username := LxdProjectUser(prj.Name)
			_, err := LookupUsername(username)
			// if the project is created by workshop, the key must be present
			if _, ok := prj.Config["user.workshop.projects"]; ok && err == nil {
				prjctx := context.WithValue(ctx, ContextUser, username)
				projects, err := readProjects([]byte(prj.Config["user.workshop.projects"]))
				if err != nil {
					return nil, err
				}

				checked, err := s.checkAndRecoverProjectPaths(client, prjctx, projects)
				if err != nil {
					return nil, err
				}

				allProjects[username] = checked
			}
		}
		return allProjects, nil
	}
}

func (s *LxdBackend) LaunchWorkshop(ctx context.Context, name, base string) error {
	var err error
	var imageSrv lxd.ImageServer
	var image *api.Image

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	userName, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key user not found")
	}

	/* Skip if the instance exists already */
	if _, _, err := conn.GetInstance(InstanceName(name, projectId)); err == nil {
		return fmt.Errorf("workshop \"%s\" already exists", name)
	}

	/* Check if we have the base image stored locally */
	if alias, _, err := conn.GetImageAlias(base); err == nil {
		if image, _, err = conn.GetImage(alias.Target); err != nil {
			return err
		}
		imageSrv = conn
	} else {
		imageSrv, image, err = s.fetchRemoteImage(base)
		if err != nil {
			return err
		}
	}

	usr, err := LookupUsername(userName)
	if err != nil {
		return err
	}

	req := api.InstancesPost{
		InstancePut: api.InstancePut{
			Devices: defaultDevices(),
			Config:  defaultConfig(projectId, usr.Uid, usr.Gid),
		},
		Name: InstanceName(name, projectId),
		Type: api.InstanceType("container"),
		Source: api.InstanceSource{
			Type:        "image",
			Fingerprint: image.Fingerprint,
			Project:     LxdProjectName(userName),
		},
	}
	op, err := conn.CreateInstanceFromImage(imageSrv, *image, req)
	if err != nil {
		return err
	}

	if err = op.Wait(); err != nil {
		return err
	}

	_, _, err = conn.GetImageAlias(base)
	if err != nil && !api.StatusErrorCheck(err, http.StatusNotFound) {
		return err
	} else if api.StatusErrorCheck(err, http.StatusNotFound) {
		if err = conn.CreateImageAlias(api.ImageAliasesPost{ImageAliasesEntry: api.ImageAliasesEntry{
			Name: base,
			ImageAliasesEntryPut: api.ImageAliasesEntryPut{
				Target: image.Fingerprint,
			},
		}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *LxdBackend) updateInstanceState(conn lxd.InstanceServer, ctx context.Context, name, action string, force bool) error {
	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	/* Do nothing if the instance is already in the desired state */
	if (inst.StatusCode == api.Running && action == "start") ||
		(inst.StatusCode == api.Stopped && action == "stop") {
		return nil
	}

	req := api.InstanceStatePut{
		Action:  action,
		Timeout: 45,
		Force:   force,
	}

	op, err := conn.UpdateInstanceState(inst.Name, req, etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *LxdBackend) StartWorkshop(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	if err = s.updateInstanceState(conn, ctx, name, "start", false); err != nil {
		return err
	}

	// Wait until system is up an running before returning
	// see: https://blog.simos.info/how-to-know-when-a-lxd-container-has-finished-starting-up/
	args := Execution{
		ExecArgs: ExecArgs{
			UserId:  0,
			GroupId: 0,
			Command: []string{
				"bash", "-eu", "-c", "while " +
					"[ \"$(systemctl is-system-running 2>/dev/null)\" != \"running\" ] && " +
					"[ \"$(systemctl is-system-running 2>/dev/null)\" != \"degraded\" ]; do :; done",
			},
			WorkDir: "/",
		},
	}

	exectx, err := s.execCommand(conn, ctx, name, &args)
	if err != nil {
		return err
	}

	return exectx.WaitExecution(ctx)
}

func (s *LxdBackend) StopWorkshop(ctx context.Context, name string, force bool) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	return s.updateInstanceState(conn, ctx, name, "stop", force)
}

func (s *LxdBackend) AddWorkshopConfig(ctx context.Context, name string, item *WorkshopConfigValue) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}
	inst.Config[item.Name] = item.Value
	op, err := conn.UpdateInstance(inst.Name, inst.InstancePut, etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *LxdBackend) RemoveWorkshopConfig(ctx context.Context, name string, key string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	delete(inst.Config, key)
	op, err := conn.UpdateInstance(inst.Name, inst.InstancePut, etag)
	if err != nil {
		return err
	}

	return op.Wait()
}

func (s *LxdBackend) AddWorkshopDevice(ctx context.Context, name string, device Device) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}
	inst.Devices[device.Name()] = device.properties
	op, err := conn.UpdateInstance(inst.Name, inst.InstancePut, etag)
	if err != nil {
		return err
	}

	return op.Wait()
}

func (s *LxdBackend) RemoveWorkshopDevice(ctx context.Context, name string, device string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	delete(inst.Devices, device)
	op, _ := conn.UpdateInstance(inst.Name, inst.InstancePut, etag)

	return op.Wait()
}

func (s *LxdBackend) execCommand(conn lxd.InstanceServer, ctx context.Context, name string, args *Execution) (ExecContext, error) {
	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return ExecContext{}, fmt.Errorf("context key project-id not found")
	}

	req := api.InstanceExecPost{
		Command:     args.Command,
		WaitForWS:   true,
		Interactive: args.Interactive,
		Environment: args.Environment,
		Width:       args.Width,
		Height:      args.Height,
		User:        uint32(args.UserId),
		Group:       uint32(args.GroupId),
		Cwd:         args.WorkDir,
	}

	done := make(chan bool)

	op, err := conn.ExecInstance(InstanceName(name, projectId), req, &lxd.InstanceExecArgs{
		Stdin:    args.Stdin,
		Stdout:   args.Stdout,
		Stderr:   args.Stderr,
		Control:  args.Control,
		DataDone: done,
	})
	if err != nil {
		return ExecContext{}, err
	}

	opmeta := op.Get()
	var env = map[string]string{}
	for k, v := range opmeta.Metadata["environment"].(map[string]any) {
		if value, ok := v.(string); ok {
			env[k] = value
		}
	}

	return ExecContext{
		Environment: env,
		WaitExecution: func(ctx context.Context) error {
			if err := op.WaitContext(ctx); err != nil {
				return err
			}

			// waiting for any remaining data IO to be flushed LXD closes this channel
			// unconditionally right after the operation has exited, so it will not be
			// blocked if we are here
			<-done
			var status = int(op.Get().Metadata["return"].(float64))
			if status != 0 {
				return &ErrExec{Status: status}
			}
			return nil
		},
	}, nil
}

func (s *LxdBackend) Exec(ctx context.Context, name string, args *Execution) (ExecContext, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return ExecContext{}, err
	}

	return s.execCommand(conn, ctx, name, args)
}

func (s *LxdBackend) Workshop(ctx context.Context, name string) (*Workshop, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	var p *Project
	projects, err := s.Projects(ctx)
	if err != nil {
		return nil, err
	}

	user, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", ContextUser)
	}

	idx := slices.IndexFunc(projects[user], func(p *Project) bool { return p.ProjectId == projectId })
	if idx == -1 {
		return nil, fmt.Errorf("project %q is not found", projectId)
	}
	p = projects[user][idx]

	inst, _, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil, ErrWorkshopNotFound
		}
		return nil, err
	}

	workshop, err := s.loadWorkshop(inst, p)
	if err != nil {
		return nil, err
	}

	return workshop, nil
}

func (s *LxdBackend) loadWorkshop(inst *api.Instance, p *Project) (*Workshop, error) {
	var err error
	var running bool

	name := WorkshopName(inst.Name)

	if inst.StatusCode == api.Running || inst.StatusCode == api.Ready {
		running = true
	}

	base := inst.Config["image.os"] + "@" + inst.Config["image.version"]
	if base == "" {
		base = "unknown"
	}

	var workshop = &Workshop{
		backend: s,
		project: p,
		Name:    name,
		running: running,
		base:    base,
		devices: inst.Devices,
	}

	// Fetch information about the installed SDKs
	workshop.content, err = InstalledContent(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("cannot load workshop: installed SDK content is not readable: %v", err)
	}
	return workshop, nil
}

func (s *LxdBackend) filterLxdInstancesByConfig(conn lxd.InstanceServer, filter WorkshopConfigFilter) ([]api.Instance, error) {
	instances, err := conn.GetInstances(api.InstanceTypeContainer)
	if err != nil {
		return nil, err
	}

	toReturn := make([]api.Instance, 0, len(instances))
	for _, i := range instances {
		if filter(i.Config) {
			toReturn = append(toReturn, i)
		}
	}

	return toReturn, nil
}

func (s *LxdBackend) ProjectWorkshops(ctx context.Context) ([]*WorkshopFile, []*Workshop, error) {
	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return nil, nil, fmt.Errorf("context key project-id not found")
	}

	user, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return nil, nil, fmt.Errorf("context key %s not found", ContextUser)
	}

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	var p *Project
	projects, err := s.Projects(ctx)
	if err != nil {
		return nil, nil, err
	}

	idx := slices.IndexFunc(projects[user], func(p *Project) bool { return p.ProjectId == projectId })
	if idx == -1 {
		return nil, nil, fmt.Errorf("project %q is not found", projectId)
	}

	p = projects[user][idx]

	files, err := p.EnumWorkshopFiles()
	// if the dir does not exist it does not mean there are no workshops. It
	// could be because the dir was removed with some workshops still operating
	// resulting in a missing-project error
	if err != nil && !osutil.IsDirNotExist(err) {
		return nil, nil, err
	}

	// get all the running workshops for this project
	instances, err := conn.GetInstances(api.InstanceTypeContainer)
	if err != nil {
		return nil, nil, err
	}

	var projectWorkshops []*Workshop
	for _, i := range instances {
		if i.Config[ProjectIdConfig] == p.ProjectId {
			ws, err := s.loadWorkshop(&i, p)
			if err != nil {
				logger.Debugf("error loading workshop: %v", err)
				continue
			}
			projectWorkshops = append(projectWorkshops, ws)
		}
	}

	wsFiles, wsInstances := mergeInstancesAndFiles(files, projectWorkshops)
	return wsFiles, wsInstances, nil
}

// Examine the lists of project's workshop files and workshops. Returns two
// lists. The first has *only* the workshop files that do not have any launched
// workshops yet, the second contains workshops that are launched with or
// without an associated file.
func mergeInstancesAndFiles(f []*WorkshopFile, instances []*Workshop) ([]*WorkshopFile, []*Workshop) {
	files := make([]*WorkshopFile, len(f))
	copy(files, f)
	/* Walk both lists from to build a list of workshops with their states */
	for _, ws := range instances {
		finder := func(p *WorkshopFile) bool { return p.Name == ws.Name }
		idx := slices.IndexFunc(files, finder)
		if idx != -1 {
			/* Both a file and instance exist */
			files = slices.Delete(files, idx, idx+1)
		}
	}

	/* At this point, files contain only inactive workshops and instances
	contain the workshops that have workshop files available */
	return files, instances
}

func (s *LxdBackend) RemoveWorkshop(ctx context.Context, name string) (err error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, _, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	if inst.StatusCode != 0 && inst.StatusCode != api.Stopped {
		if err = s.updateInstanceState(conn, ctx, name, "stop", true); err != nil {
			return err
		}
	}

	op, err := conn.DeleteInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	if err = op.WaitContext(ctx); err != nil {
		return err
	}
	return nil
}

func (s *LxdBackend) WorkshopFs(ctx context.Context, name string) (WorkshopFs, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}

	projectId, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	sftp, err := conn.GetInstanceFileSFTP(InstanceName(name, projectId))
	if err != nil {
		return nil, err
	}

	return NewWorkshopFs(sftp), nil
}

func (s *LxdBackend) CreateStateStorage(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	pid, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", ContextProjectId)
	}

	// Create the storage volume entry
	vol := api.StorageVolumesPost{}
	vol.Name = WorkshopStateVolumeName(name, pid)
	vol.Type = "custom"
	vol.ContentType = "filesystem"
	vol.Config = map[string]string{}

	return conn.CreateStoragePoolVolume("default", vol)
}

func (s *LxdBackend) DeleteStateStorage(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}

	pid, ok := ctx.Value(ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", ContextProjectId)
	}

	return conn.DeleteStoragePoolVolume("default", "custom", WorkshopStateVolumeName(name, pid))
}

func (s *LxdBackend) LxdClient(ctx context.Context) (lxd.InstanceServer, error) {
	user, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", ContextUser)
	}

	if srv, err := lxd.ConnectLXDUnixWithContext(ctx, LxdSock, nil); err != nil {
		return nil, err
	} else {
		if err = InitProject(srv, user); err != nil {
			return nil, err
		}
		return srv.UseProject(LxdProjectName(user)), nil
	}
}

func createDefaultDevices() map[string]map[string]string {
	return map[string]map[string]string{
		"root":                 {"type": "disk", "pool": "default", "path": "/"},
		"workshop.network":     {"type": "nic", "network": "lxdbr0", "name": "eth0"},
		"workshop.socket":      {"type": "disk", "source": dirs.SocketPath + ".untrusted", "path": filepath.Join(dirs.WorkshopBaseDir, ".workshop.socket.untrusted")},
		"workshop.workshopctl": {"type": "disk", "source": filepath.Join(dirs.ExecDir, "workshopctl"), "path": "/usr/bin/workshopctl"},
	}
}

func defaultConfig(projectId string, userid, groupid string) map[string]string {
	return map[string]string{
		"raw.idmap":                fmt.Sprint("uid ", userid, " 1000\ngid ", groupid, " 1000"),
		"security.nesting":         "true",
		"user.workshop.project-id": projectId,
		"user.user-data": `#cloud-config
users:
  - default
  - name: workshop
    primary_group: workshop
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: adm,cdrom,sudo,dip,plugdev,audio,netdev,lxd,video
    shell: /bin/bash
`,
	}
}

func (s *LxdBackend) fetchRemoteImage(base string) (lxd.ImageServer, *api.Image, error) {
	var image *api.Image

	imageServer, err := ConnectSimpleStreams(imageServer, nil)
	if err != nil {
		return nil, nil, err
	}

	names := strings.Split(base, "@")
	if len(names) <= 1 {
		return nil, nil, fmt.Errorf("cannot find a base image for the workshop")
	}

	alias, _, err := imageServer.GetImageAlias(fmt.Sprintf("%s/amd64", names[1]))
	if err != nil {
		return nil, nil, err
	}

	image, _, err = imageServer.GetImage(alias.Target)
	if err != nil {
		return nil, nil, err
	}

	return imageServer, image, nil
}
