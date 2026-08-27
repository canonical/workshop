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

package lxdbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/workshop"
)

func lxdProjectConfig(username string) map[string]string {
	return map[string]string{
		"features.images":          "false",
		"features.profiles":        "true",
		"features.storage.volumes": "false",
		"user.workshop.username":   username,
	}
}

// Checks if a user name can be used in a LXD project name.
var isValidProjectSuffix = regexp.MustCompile(`^[a-zA-Z][-a-zA-Z0-9.]{0,31}$`).MatchString

func projectName(prefix, username string) (string, error) {
	if isValidProjectSuffix(username) {
		return prefix + username, nil
	}

	u, err := osutil.UserLookup(username)
	if err != nil {
		return "", err
	}
	return prefix + u.Uid, nil
}

func lxdProjectName(user string) (string, error) {
	return projectName("workshop.", user)
}

func lxdSnapshotsProjectName(user string) (string, error) {
	return projectName("workshop-snapshots.", user)
}

// Create LXD projects (storing workshops and snapshots) for the user if they don't exist.
func initLxdProject(conn lxd.InstanceServer, project, username string) error {
	names, err := conn.GetProjectNames()
	if err != nil {
		return err
	}

	if !slices.Contains(names, project) {
		err = conn.CreateProject(api.ProjectsPost{
			ProjectPut: api.ProjectPut{
				Config:      lxdProjectConfig(username),
				Description: fmt.Sprintf(`Workshop project for "%s" user`, username),
			},
			Name: project,
		})
		if err != nil {
			return err
		}
	}

	rev := revert.New()
	defer rev.Fail()
	rev.Add(func() {
		op, err := conn.DeleteProject(project, false)
		if err != nil {
			logger.Noticef("cannot delete project %q: %v", project, err)
		} else {
			err = op.Wait()
			if err != nil {
				logger.Noticef("cannot wait for project %q deletion: %v", project, err)
			}
		}
	})

	snapshots, err := lxdSnapshotsProjectName(username)
	if err != nil {
		return err
	}
	if !slices.Contains(names, snapshots) {
		err = conn.CreateProject(api.ProjectsPost{
			ProjectPut: api.ProjectPut{
				Config:      lxdProjectConfig(username),
				Description: fmt.Sprintf(`Workshop snapshots project for "%s" user`, username),
			},
			Name: snapshots,
		})
		if err != nil {
			return err
		}
	}

	rev.Success()
	return nil
}

func (s *Backend) CreateOrLoadProject(ctx context.Context, path string) (*workshop.Project, bool, error) {
	client, err := s.LxdClient(ctx)
	if err != nil {
		return nil, false, err
	}
	defer client.Disconnect()

	info, err := client.GetConnectionInfo()
	if err != nil {
		return nil, false, err
	}

	lxdPrj, etag, err := client.GetProject(info.Project)
	if err != nil {
		return nil, false, err
	}

	projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
	if err != nil {
		return nil, false, err
	}

	tracker := workshop.ProjectTracker{Projects: projects}
	project, result, err := tracker.Track(path)
	if err != nil {
		return nil, false, err
	}

	if result == workshop.ProjectMoved {
		if err = s.updateProjectMounts(client, ctx, *project); err != nil {
			return nil, false, err
		}
	}

	if result != workshop.ProjectFound {
		projectsJson, err := saveProjects(tracker.Projects)
		if err != nil {
			return nil, false, err
		}
		lxdPrj.Config["user.workshop.projects"] = projectsJson
		if err = client.UpdateProject(lxdPrj.Name, lxdPrj.Writable(), etag); err != nil {
			return nil, false, err
		}
	}

	return project, result == workshop.ProjectAdded, nil
}

func (s *Backend) Projects(ctx context.Context) (map[string][]workshop.Project, error) {
	if user, ok := ctx.Value(workshop.ContextUser).(string); ok {
		projects, err := s.userProjects(ctx)
		if err != nil {
			return nil, err
		}
		return map[string][]workshop.Project{user: projects}, nil
	}

	// Get a default connection without preseting the LXD project as we are
	// going over all the LXD projects to filter the ones managed by workshop
	// and reload every interface connection for every SDK of every workshop.
	client, err := lxd.ConnectLXDUnixWithContext(ctx, "", nil)
	if err != nil {
		return nil, ErrorLxdBackend(err)
	}
	defer client.Disconnect()
	// list all projects for all users if the user is not provided
	lxdProjects, err := client.GetProjects()
	if err != nil {
		return nil, err
	}
	allProjects := make(map[string][]workshop.Project)
	for _, lxdProject := range lxdProjects {
		// If the project is created by workshop, the key must be present.
		username, ok := lxdProject.Config["user.workshop.username"]
		if !ok {
			continue
		}

		if _, err = osutil.UserLookup(username); err != nil {
			logger.Noticef("cannot find user %q: %v", username, err)
			continue
		}

		prjctx := context.WithValue(ctx, workshop.ContextUser, username)
		projects, err := s.userProjects(prjctx)
		if err != nil {
			return nil, err
		}

		allProjects[username] = projects
	}
	return allProjects, nil
}

func (s *Backend) userProjects(ctx context.Context) ([]workshop.Project, error) {
	client, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect()

	info, err := client.GetConnectionInfo()
	if err != nil {
		return nil, err
	}

	lxdPrj, etag, err := client.GetProject(info.Project)
	if err != nil {
		return nil, err
	}

	projects, err := readProjects([]byte(lxdPrj.Config["user.workshop.projects"]))
	if err != nil {
		return nil, err
	}

	projects, modified, err := s.pruneProjects(client, ctx, projects)
	if err != nil {
		return nil, err
	}
	if modified {
		projectsJson, err := saveProjects(projects)
		if err != nil {
			return nil, err
		}
		lxdPrj.Config["user.workshop.projects"] = projectsJson
		if err = client.UpdateProject(lxdPrj.Name, lxdPrj.Writable(), etag); err != nil {
			return nil, err
		}
	}

	return projects, nil
}

// Attempts to ensure that every project has a valid existing path.
// If a path does not exist, recover it from the actual bind mount of the '/project'.
// If recovery fails and no workshops exist for the project,
// remove the project from the list.
func (s *Backend) pruneProjects(client lxd.InstanceServer, ctx context.Context, projects []workshop.Project) ([]workshop.Project, bool, error) {
	pruned := make([]workshop.Project, 0, len(projects))
	removed := make([]workshop.Project, 0, len(projects))
	modified := false

	for _, prj := range projects {
		if prj.Exists() {
			pruned = append(pruned, prj)
			continue
		}

		// If got here then there is no project directory for the projectId
		// anymore. It can mean moving or deletion happened in the past. Try
		// to recover the new project path.
		path, err := s.projectFsRoot(client, ctx, prj.ProjectId)
		if err != nil {
			return nil, false, err
		}
		if path != "" {
			prj.Path = path
			if err = s.updateProjectMounts(client, ctx, prj); err != nil {
				return nil, false, err
			}
			pruned = append(pruned, prj)
			modified = true
			continue
		}

		// Could not recover the directory, reconcile the project from the
		// list of projects that we track (only if there are no remaining
		// workshops for this project)
		args := lxd.GetInstancesArgs{
			InstanceType: api.InstanceTypeContainer,
			Filters:      []string{"config.user.workshop.project-id=" + prj.ProjectId},
		}
		workshops, err := client.GetInstances(args)
		if err != nil {
			return nil, false, err
		}
		if len(workshops) > 0 {
			pruned = append(pruned, prj)
		} else {
			removed = append(removed, prj)
			modified = true
		}
	}

	if err := s.pruneWorkshopCNAMEs(client, ctx, removed); err != nil {
		logger.Noticef("On pruneProjects: failed to prune workshop CNAME records: %v", err)
	}

	return pruned, modified, nil
}

func (s *Backend) projectFsRoot(conn lxd.InstanceServer, ctx context.Context, projectId string) (path string, err error) {
	args := lxd.GetInstancesArgs{
		InstanceType: api.InstanceTypeContainer,
		Filters:      []string{"config.user.workshop.project-id=" + projectId},
	}
	workshops, err := conn.GetInstances(args)
	if err != nil {
		return "", err
	}

	for _, i := range workshops {
		// attempt to execute the command only in a running instance
		if i.StatusCode != api.Ready && i.StatusCode != api.Running {
			continue
		}

		var outbuf bytes.Buffer
		var errbuf strings.Builder

		/* Get the mount point directory from findmnt */
		args := workshop.Execution{
			ExecArgs: workshop.ExecArgs{
				UserId:  0,
				GroupId: 0,
				Command: []string{"findmnt", "--json", "--mountpoint", "/project", "--output", "fsroot"},
				WorkDir: "/",
			},
			ExecControls: workshop.ExecControls{
				Stdin:  nil,
				Stdout: &outbuf,
				Stderr: &errbuf,
			},
		}

		execCtx := context.WithValue(ctx, workshop.ContextProjectId, projectId)
		meta, err := s.execCommand(conn, execCtx, workshopName(i.Name), &args)
		if err != nil {
			logger.Debugf("cannot check %q bind-mounts: %v", i.Name, err)
			continue
		}
		if err = meta.WaitExecution(ctx); err != nil {
			// It's unsafe to access errbuf before the DataDone channel is closed.
			var details string
			if _, ok := errors.AsType[*workshop.ErrExec](err); ok {
				details = ", findmnt output: " + errbuf.String()
			}

			logger.Debugf("cannot check %q bind-mounts: %v%s", i.Name, err, details)
			continue
		}

		output := struct {
			Filesystems []struct {
				Fsroot string `json:"fsroot"`
			} `json:"filesystems"`
		}{}
		if err = json.Unmarshal(outbuf.Bytes(), &output); err != nil {
			return "", err
		}
		if len(output.Filesystems) != 1 {
			logger.Debugf("cannot check %q bind-mounts: exactly one source required", i.Name)
			continue
		}
		currentPath := output.Filesystems[0].Fsroot

		/* check if the path is not deleted, i.e. the project directory still exists on the host */
		if ok, isDir, err := osutil.ExistsIsDir(currentPath); ok && isDir {
			return currentPath, nil
		} else if err != nil && !osutil.IsDirNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

func readProjects(jsonData []byte) ([]workshop.Project, error) {
	var projects = make([]workshop.Project, 0)
	if len(jsonData) == 0 {
		return projects, nil
	}
	if err := json.Unmarshal([]byte(jsonData), &projects); err != nil {
		return nil, fmt.Errorf("invalid projects record: %w", err)
	}
	return projects, nil
}

func saveProjects(projects []workshop.Project) (string, error) {
	buf, err := json.Marshal(projects)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (s *Backend) updateProjectMounts(conn lxd.InstanceServer, ctx context.Context, project workshop.Project) error {
	projectCtx := context.WithValue(ctx, workshop.ContextProjectId, project.ProjectId)

	args := lxd.GetInstancesArgs{
		InstanceType: api.InstanceTypeContainer,
		Filters:      []string{"config.user.workshop.project-id=" + project.ProjectId},
	}
	workshops, err := conn.GetInstances(args)
	if err != nil {
		return err
	}

	for _, i := range workshops {
		mount := workshop.Mount{
			Name:  workshop.ConfigProjectPathDevice,
			Type:  workshop.HostWorkshop,
			What:  project.Path,
			Where: workshop.WorkshopProjectPath,
		}
		err = s.AddWorkshopMount(projectCtx, workshopName(i.Name), mount)
		if err != nil {
			return fmt.Errorf("cannot update workshop %q project directory: %w", i.Name, err)
		}
	}
	return nil
}
