package lxdbackend

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	storagePool           = "workshop"
	storagePoolDriver     = "zfs"
	storagePoolMinimalGiB = 5

	networkName = "workshopbr0"
	networkType = "bridge"
)

var (
	checkNvidiaRuntime  = checkNvidia
	startCommandTimeout = 1 * time.Minute
)

type volumeGuard struct {
	c       chan struct{}
	counter int32
}

var (
	// However many backend instances are created, downloads are always a single
	// instance map with the LXD backend.
	imageLock        sync.Mutex
	currentDownloads map[string]*downloadOp

	volumeGuardsLock sync.Mutex
	volumeGuards     map[string]*volumeGuard
)

//go:embed start_command.sh
var startCommand string

func init() {
	imageLock.Lock()
	defer imageLock.Unlock()
	if currentDownloads == nil {
		currentDownloads = make(map[string]*downloadOp)
	}

	volumeGuardsLock.Lock()
	defer volumeGuardsLock.Unlock()
	volumeGuards = make(map[string]*volumeGuard)
}

type Backend struct {
}

func InstanceName(name string, project_id string) string {
	return fmt.Sprintf("%s-%s", name, project_id)
}

func instanceStashName(name string, pid string) string {
	return "stash-" + InstanceName(name, pid)
}

func ImageAlias(name string) string {
	return fmt.Sprintf("workshop-%s-%s", name, runtime.GOARCH)
}

func ErrorLxdBackend(err error) error {
	switch {
	case errors.Is(err, unix.ECONNREFUSED):
		return fmt.Errorf(`cannot connect to LXD: %w

Maybe LXD daemon isn't active?
To start the LXD daemon: 'sudo snap start lxd'
To restart the workshop daemon: 'sudo snap restart workshop'`, err)
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf(`cannot connect to LXD: %w

Maybe LXD isn't installed?
To install LXD: 'sudo snap install lxd'
To restart the workshop daemon: 'sudo snap restart workshop'`, err)
	default:
		return err
	}
}

func New() (*Backend, error) {
	server := Backend{}

	if srv := os.Getenv("WORKSHOP_IMAGE_SERVER"); srv != "" {
		imageServer = srv
	}

	// TODO: run this logic for a specific user. The code below implies the
	// default project activated for the connection. As we have seen, every user
	// has to create its own storage pool to avoid issues with id mapping of a
	// volume with the same name (e.g. both users have system-1 volume for the
	// system SDK that cannot be successfully mounted for another user).
	conn, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return nil, ErrorLxdBackend(err)
	}
	defer conn.Disconnect()

	// Create LXD storage pool if it doesn't exist.
	pools, err := conn.GetStoragePools()
	if err != nil {
		return nil, err
	}
	if idx := slices.IndexFunc(pools, func(p api.StoragePool) bool { return p.Name == storagePool }); idx < 0 {
		req := api.StoragePoolsPost{
			Name:   storagePool,
			Driver: storagePoolDriver,
			StoragePoolPut: api.StoragePoolPut{
				Config: map[string]string{
					"volume.zfs.remove_snapshots": "true",
				},
			},
		}
		err := conn.CreateStoragePool(req)
		if err != nil {
			return nil, err
		}

		// Ensure the new pool has enough total space available.
		pool, etag, err := conn.GetStoragePool(storagePool)
		if err != nil {
			return nil, err
		}

		res, err := conn.GetStoragePoolResources(storagePool)
		if err != nil {
			return nil, err
		}

		gibTotal := uint64(res.Space.Total) / (1024 * 1024 * 1024)
		if gibTotal < storagePoolMinimalGiB {
			// Ensure the storage pool is no less than 5GiB, otherwise it makes
			// it running out of space in tests and environments with less than
			// ~14GiB of available space. LXD defaults to 20% in those cases
			// which results in a ~2GiB pool size for workshop.
			pool.Config["size"] = strconv.FormatUint(storagePoolMinimalGiB*1024*1024*1024, 10)
			if err = conn.UpdateStoragePool(storagePool, pool.Writable(), etag); err != nil {
				logger.Noticef("On Backend.New: failed to set storage pool to the minimal size: %dGiB, %s", storagePoolMinimalGiB, err)
				return nil, err
			}
			logger.Noticef("On Backend.New: set storage pool to the minimal size: %dGiB", storagePoolMinimalGiB)
		}
	} else if pools[idx].Driver != storagePoolDriver {
		return nil, fmt.Errorf("storage pool %q already exists with a different driver: %q", storagePool, pools[idx].Driver)
	}

	networks, err := conn.GetNetworks()
	if err != nil {
		return nil, err
	}
	if idx := slices.IndexFunc(networks, func(n api.Network) bool { return n.Name == networkName }); idx < 0 {
		req := api.NetworksPost{
			Name: networkName,
			Type: networkType,
			NetworkPut: api.NetworkPut{
				Config: map[string]string{
					"dns.domain": "workshop",
				},
				Description: "Bridge network for workshops",
			},
		}

		if err := conn.CreateNetwork(req); err != nil {
			return nil, err
		}
	} else if networks[idx].Type != networkType {
		return nil, fmt.Errorf("network %q already exists with a different type: %q", networkName, networks[idx].Type)
	}

	return &server, nil
}

func (s *Backend) LaunchOrRebuildWorkshop(ctx context.Context, file *workshop.File) error {
	var err error
	var image *api.Image

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	info, err := conn.GetConnectionInfo()
	if err != nil {
		return err
	}

	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key user not found")
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	// Check if we have the base image stored locally.
	alias, _, err := conn.GetImageAlias(ImageAlias(file.Base))
	if err != nil {
		return err
	}

	image, _, err = conn.GetImage(alias.Target)
	if err != nil {
		return err
	}

	usr, err := osutil.UserLookup(username)
	if err != nil {
		return err
	}

	config, err := s.workshopConfig(projectId, usr.Uid, usr.Gid, file)
	if err != nil {
		return err
	}
	devices := defaultDevices(projectId, file.Name)

	inst, _, err := conn.GetInstanceFull(InstanceName(file.Name, projectId))
	switch {
	case err != nil && !api.StatusErrorCheck(err, http.StatusNotFound):
		return err
	case err != nil && api.StatusErrorCheck(err, http.StatusNotFound):
		// Create a new workshop.
		req := api.InstancesPost{
			InstancePut: api.InstancePut{
				Devices: devices,
				Config:  config,
			},
			Name: InstanceName(file.Name, projectId),
			Type: api.InstanceType("container"),
			Source: api.InstanceSource{
				Type:        "image",
				Fingerprint: image.Fingerprint,
				Project:     info.Project,
			},
		}
		op, err := conn.CreateInstance(req)
		if err != nil {
			return err
		}

		return op.Wait()
	default:
		// Rebuild the existing workshop.
		for _, snapshot := range inst.Snapshots {
			op, err := conn.DeleteInstanceSnapshot(inst.Name, snapshot.Name)
			if err != nil {
				return err
			}
			if err = op.Wait(); err != nil {
				return err
			}
		}

		rop, err := conn.RebuildInstanceFromImage(conn, *image, inst.Name, api.InstanceRebuildPost{})
		if err != nil {
			return err
		}
		if err = rop.Wait(); err != nil {
			return err
		}

		// Get an updated instance configuration.
		rebuilt, etag, err := conn.GetInstance(inst.Name)
		if err != nil {
			return err
		}

		maps.Copy(rebuilt.Config, config)
		clear(rebuilt.Devices)
		maps.Copy(rebuilt.Devices, devices)

		op, err := conn.UpdateInstance(rebuilt.Name, rebuilt.Writable(), etag)
		if err != nil {
			return err
		}
		if err = op.Wait(); err != nil {
			return err
		}
		return nil
	}
}

func (s *Backend) updateInstanceState(conn lxd.InstanceServer, ctx context.Context, name, action string, force bool) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	// Do nothing if the instance is already in the desired state
	if (inst.StatusCode == api.Running && action == "start") ||
		(inst.StatusCode == api.Stopped && action == "stop") {
		return nil
	}

	req := api.InstanceStatePut{
		Action:  action,
		Timeout: 60,
		Force:   force,
	}

	op, err := conn.UpdateInstanceState(inst.Name, req, etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *Backend) StartWorkshop(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	return s.startWorkshop(conn, ctx, name)
}

func (s *Backend) startWorkshop(conn lxd.InstanceServer, ctx context.Context, name string) error {
	rev := revert.New()
	defer rev.Fail()

	cleanupCtx := context.WithoutCancel(ctx)
	rev.Add(func() {
		cleanupCtxTimeout, cancel := context.WithTimeout(cleanupCtx, 5*time.Second)
		defer cancel()

		if e := s.AddWorkshopConfig(cleanupCtxTimeout, name, &workshop.WorkshopConfigValue{Name: "boot.autostart", Value: "false"}); e != nil {
			logger.Noticef("On StartWorkshop: cannot reset %q workshop autostart config on cleanup: %v", name, e)
		}

		// Stop workshop's timeout is handled by LXD API, so no need to have
		// a context with a timeout.
		if e := s.StopWorkshop(cleanupCtx, name, true); e != nil {
			logger.Noticef("On StartWorkshop: cannot stop %q workshop on cleanup: %v", name, e)
		}
	})

	if err := s.updateInstanceState(conn, ctx, name, "start", false); err != nil {
		return err
	}

	// Workshop started, enable autostart.
	if err := s.addWorkshopConfig(conn, ctx, name, &workshop.WorkshopConfigValue{Name: "boot.autostart", Value: "true"}); err != nil {
		return err
	}

	args := workshop.Execution{
		ExecArgs: workshop.ExecArgs{
			UserId:  0,
			GroupId: 0,
			Command: []string{
				"bash", "-euc", startCommand,
			},
			WorkDir: "/",
			Timeout: startCommandTimeout,
		},
	}

	exectx, err := s.execCommand(conn, ctx, name, &args)
	if err != nil {
		return err
	}

	if err = exectx.WaitExecution(ctx); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (s *Backend) StopWorkshop(ctx context.Context, name string, force bool) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	return s.stopWorkshop(conn, ctx, name, force)
}

func (s *Backend) stopWorkshop(conn lxd.InstanceServer, ctx context.Context, name string, force bool) error {
	// Workshop stopped, disable autostart.
	if err := s.addWorkshopConfig(conn, ctx, name, &workshop.WorkshopConfigValue{Name: "boot.autostart", Value: "false"}); err != nil {
		return err
	}

	return s.updateInstanceState(conn, ctx, name, "stop", force)
}

func (s *Backend) AddWorkshopConfig(ctx context.Context, name string, item *workshop.WorkshopConfigValue) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	return s.addWorkshopConfig(conn, ctx, name, item)
}

func (s *Backend) addWorkshopConfig(conn lxd.InstanceServer, ctx context.Context, name string, item *workshop.WorkshopConfigValue) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	inst.Config[item.Name] = item.Value
	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *Backend) RemoveWorkshopConfig(ctx context.Context, name string, key string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	delete(inst.Config, key)
	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}

	return op.Wait()
}

func (s *Backend) AddWorkshopMount(ctx context.Context, name string, mount workshop.Mount) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	inst.Devices[mount.Name] = mountToLxdDisk(mount)

	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func mountToLxdDisk(mount workshop.Mount) map[string]string {
	device := map[string]string{
		"type":     "disk",
		"source":   mount.What,
		"path":     mount.Where,
		"readonly": fmt.Sprint(mount.ReadOnly),
	}
	if mount.Type == workshop.Volume {
		device["pool"] = storagePool
	}
	return device
}

func (s *Backend) RemoveWorkshopMount(ctx context.Context, name, mount string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	delete(inst.Devices, mount)
	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *Backend) execCommand(conn lxd.InstanceServer, ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return workshop.ExecContext{}, fmt.Errorf("context key project-id not found")
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
		return workshop.ExecContext{}, err
	}

	opmeta := op.Get()
	var env = map[string]string{}
	for k, v := range opmeta.Metadata["environment"].(map[string]any) {
		if value, ok := v.(string); ok {
			env[k] = value
		}
	}

	return workshop.ExecContext{
		Environment: env,
		WaitExecution: func(ctx context.Context) error {
			defer conn.Disconnect()

			if err := op.WaitContext(ctx); err != nil {
				switch err.Error() {
				case "Command not executable", "Command not found":
					// Usually a nonzero exit status is not an error,
					// but LXD translates 126 and 127 into the above messages.
				default:
					return err
				}
			}

			// waiting for any remaining data IO to be flushed LXD closes this channel
			// unconditionally right after the operation has exited, so it will not be
			// blocked if we are here
			<-done
			var status = int(op.Get().Metadata["return"].(float64))
			if status != 0 {
				return &workshop.ErrExec{Status: status}
			}
			return nil
		},
	}, nil
}

func (s *Backend) Exec(ctx context.Context, name string, args *workshop.Execution) (workshop.ExecContext, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return workshop.ExecContext{}, err
	}

	return s.execCommand(conn, ctx, name, args)
}

func (s *Backend) Workshop(ctx context.Context, name string) (*workshop.Workshop, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	projects, err := s.Projects(ctx)
	if err != nil {
		return nil, err
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	idx := slices.IndexFunc(projects[user], func(p workshop.Project) bool { return p.ProjectId == projectId })
	if idx == -1 {
		return nil, fmt.Errorf("project %q not found", projectId)
	}
	p := projects[user][idx]

	inst, _, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil, workshop.ErrWorkshopNotLaunched
		}
		return nil, err
	}

	workshop, err := s.loadWorkshop(conn, inst, p)
	if err != nil {
		return nil, err
	}

	return workshop, nil
}

func workshopFile(lxdConfig map[string]string) (*workshop.File, error) {
	var f workshop.File
	if yml, ok := lxdConfig[workshop.ConfigWorkshopFile]; ok {
		if err := yaml.Unmarshal([]byte(yml), &f); err != nil {
			return nil, err
		}
	}
	return &f, nil
}

func (b *Backend) loadWorkshop(conn lxd.InstanceServer, inst *api.Instance, p workshop.Project) (*workshop.Workshop, error) {
	f, err := workshopFile(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("cannot load workshop: %v", err)
	}

	sdks := map[string]sdk.Setup{}
	if buf, exist := inst.Config[workshop.ConfigWorkshopSdks]; exist {
		if err := json.Unmarshal([]byte(buf), &sdks); err != nil {
			return nil, err
		}
	}

	profs := make(map[string]workshop.SdkProfile, len(sdks))
	for _, s := range sdks {
		sp, err := Profile(conn, p.ProjectId, f.Name, s.Name)
		if err != nil && !errors.Is(err, workshop.ErrSdkProfileNotFound) {
			return nil, err
		}
		if errors.Is(err, workshop.ErrSdkProfileNotFound) {
			continue
		}

		profs[s.Name] = sp
	}

	return &workshop.Workshop{
		Backend:  b,
		Project:  p,
		Name:     f.Name,
		Base:     f.Base,
		Running:  inst.StatusCode == api.Running || inst.StatusCode == api.Ready,
		Sdks:     sdks,
		Profiles: profs,
		File:     f,
	}, nil
}

func (s *Backend) filterLxdInstancesByConfig(conn lxd.InstanceServer, filter workshop.WorkshopConfigFilter) ([]api.Instance, error) {
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

func (s *Backend) ProjectWorkshops(ctx context.Context) ([]*workshop.Workshop, error) {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	projects, err := s.Projects(ctx)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(projects[user], func(p workshop.Project) bool { return p.ProjectId == projectId })
	if idx == -1 {
		return nil, fmt.Errorf("project %q not found", projectId)
	}
	p := projects[user][idx]

	// Get all the running workshops for this project.
	instances, err := conn.GetInstances(api.InstanceTypeContainer)
	if err != nil {
		return nil, err
	}

	var workshops []*workshop.Workshop
	for _, i := range instances {
		if i.Config[workshop.ConfigProjectId] == p.ProjectId {
			ws, err := s.loadWorkshop(conn, &i, p)
			if err != nil {
				logger.Debugf("Workshop Backend on ProjectsWorkshops: %v", err)
				continue
			}
			workshops = append(workshops, ws)
		}
	}

	return workshops, nil
}

func (s *Backend) RemoveWorkshop(ctx context.Context, name string) (err error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	// ignore possible errors (e.g. container is already stopped)
	if err = s.stopWorkshop(conn, ctx, name, true); err != nil {
		logger.Noticef("On RemoveWorkshop: failed to stop %q workshop: %v", name, err)
	}

	op, err := conn.DeleteInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	// DeleteInstance cannot be cancelled in LXD.
	return op.Wait()
}

func (s *Backend) Snapshot(ctx context.Context, name, snapid string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	op, err := conn.CreateInstanceSnapshot(InstanceName(name, projectId), api.InstanceSnapshotsPost{
		Name: snapid,
	})
	if err != nil {
		return err
	}
	return op.Wait()
}

func (s *Backend) Restore(ctx context.Context, name, snapid string, file *workshop.File) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	instPut := inst.Writable()
	instPut.Restore = snapid
	op, err := conn.UpdateInstance(inst.Name, instPut, etag)
	if err != nil {
		return err
	}
	if err = op.Wait(); err != nil {
		return err
	}

	restored, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	f, err := yaml.Marshal(file)
	if err != nil {
		return err
	}

	// The restored snapshot will have an updated file.
	// Similar to how a launched workshop is associated with its definition.
	restored.Config[workshop.ConfigWorkshopFile] = string(f)

	op, err = conn.UpdateInstance(inst.Name, restored.Writable(), etag)
	if err != nil {
		return err
	}
	if err = op.Wait(); err != nil {
		return err
	}
	return nil
}

func (s *Backend) WorkshopFs(ctx context.Context, name string) (workshop.WorkshopFs, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return nil, fmt.Errorf("context key project-id not found")
	}

	sftp, err := conn.GetInstanceFileSFTP(InstanceName(name, projectId))
	if err != nil {
		return nil, err
	}

	return workshop.NewWorkshopFs(sftp), nil
}

func ConnectLxd(ctx context.Context) (lxd.InstanceServer, error) {
	conn, err := lxd.ConnectLXDUnixWithContext(ctx, "", nil)
	if err != nil {
		return nil, ErrorLxdBackend(err)
	}

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	project, err := lxdProjectName(user)
	if err != nil {
		return nil, err
	}

	if err = initLxdProject(conn, project, user); err != nil {
		return nil, err
	}

	return conn.UseProject(project), err
}

func (s *Backend) LxdClient(ctx context.Context) (lxd.InstanceServer, error) {
	return ConnectLxd(ctx)
}

func defaultDevices(pid, w string) map[string]map[string]string {
	devices := map[string]map[string]string{
		"root":             {"type": "disk", "pool": storagePool, "path": "/"},
		"workshop.network": {"type": "nic", "network": networkName, "name": "eth0"},
	}

	mounts, proxies := workshop.DefaultDevices(pid, w)
	for _, mount := range mounts {
		devices[mount.Name] = mountToLxdDisk(mount)
	}

	for _, proxy := range proxies {
		devices[proxy.Name] = proxyToLxdDevice(proxy)
	}

	return devices
}

func proxyToLxdDevice(proxy workshop.ProxyEntry) map[string]string {
	device := map[string]string{
		"type":    "proxy",
		"connect": proxy.Connect.Protocol + ":" + proxy.Connect.Address,
		"listen":  proxy.Listen.Protocol + ":" + proxy.Listen.Address,
		"mode":    "0666",
	}
	switch proxy.Direction {
	case workshop.WorkshopToHost:
		device["bind"] = "instance"
	case workshop.HostToWorkshop:
		device["bind"] = "host"
	}
	return device
}

func checkNvidia() (bool, error) {
	conn, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return false, ErrorLxdBackend(err)
	}
	defer conn.Disconnect()

	resources, err := conn.GetServerResources()
	if err != nil {
		return false, err
	}

	// Check if nvidia card(s) are present as this requires additional
	// configuration for the GPU interfaces runtime passthrough.
	nvidiaRuntime := false
	for _, card := range resources.GPU.Cards {
		if card.Nvidia != nil {
			nvidiaRuntime = true
			break
		}
	}
	return nvidiaRuntime, nil
}

// The following 'write-files' and 'runcmd' sections are for the desktop
// interface. These create a systemd path/service unit pair to copy the
// Xauthority cookie to /tmp when we mount it in the workshop. This is done to
// work around file mount ordering complications with lxc and the requirements
// on the Xauthority cookie for snapd, namely:
//  1. Snapd requires the Xauth cookie to be in a directory visible to snaps,
//     however there is a special case for /tmp in which snapd will migrate
//     the cookie for us, guaranteeing it's visibility.
//  2. Snapd explicitly checks the provided cookie for symlinks, this means
//     that we can only make a copy of the cookie
//  2. Mounts in dynamic filesystems (ie. /tmp) are genreally advised against
//     for LXD
//
// Although these will be present within every workshop, path units utilise
// inotify and as such add effectively zero overhead to a workshop launch/start.
func (s *Backend) workshopConfig(projectId string, userid, groupid string, file *workshop.File) (map[string]string, error) {
	cloudInitConfig := `#cloud-config
users:
  - default
  - name: workshop
    primary_group: workshop
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: adm,cdrom,sudo,dip,plugdev,audio,netdev,lxd,video,render
    shell: /bin/bash
apt:
  conf: |
    # Installed by workshop
    
    # Don't automatically install recommended packages
    APT::Install-Recommends "0";

    # Don't automatically install suggested packages
    APT::Install-Suggests "0";

    # Bypass confirmation prompts
    APT::Get::Assume-Yes "1";
write_files:
- content: |
    # Managed by workshop, do not remove
    [Unit]
    Description=Required for x11 support

    [Path]
    PathChanged=/var/lib/workshop/run/
    Unit=xauth-copy.service

    [Install]
    WantedBy=multi-user.target
  path: /etc/systemd/system/xauth-watch.path
- content: |
    # Managed by workshop, do not remove
    [Unit]
    Description=Required for x11 support; copies Xauthority to /tmp

    [Service]
    Type=simple
    ExecStart=/bin/bash -c 'if [ -f /var/lib/workshop/run/Xauthority/.Xauthority ]; then cp -f /var/lib/workshop/run/Xauthority/.Xauthority /tmp/.Xauthority && chown workshop:workshop /tmp/.Xauthority; fi'

    [Install]
    WantedBy=multi-user.target
  path: /etc/systemd/system/xauth-copy.service
- content: |
    # Workaround for https://bugs.launchpad.net/snapd/+bug/2104066
    [Service]
    Environment=SNAPD_STANDBY_WAIT=1m
  path: /etc/systemd/system/snapd.service.d/override.conf
runcmd:
  # Project directory is required for 'workshop exec'.
  - install --directory --mode=755 /project
  - systemctl daemon-reload
  - systemctl enable xauth-copy.service
  - systemctl enable --now xauth-watch.path
  - systemctl restart snapd.service
`

	f, err := yaml.Marshal(file)
	if err != nil {
		return map[string]string{}, err
	}

	nvidiaRuntime, err := checkNvidiaRuntime()
	if err != nil {
		return nil, err
	}

	// nvidia.* properties must be set at launch as otherwise it requires a
	// container restart to take effect.
	cfgNvidiaDriverCapabilities := ""
	cfgNvidiaRuntime := ""
	if nvidiaRuntime {
		cfgNvidiaDriverCapabilities = "all"
		cfgNvidiaRuntime = "true"
	}

	// Include all options we might change, even those with default values,
	// so that workshops can be rebuilt.
	cfg := map[string]string{
		"boot.autostart":             "false",
		"raw.idmap":                  fmt.Sprintf("uid %s %s\ngid %s %s", userid, workshop.User.Uid, groupid, workshop.User.Gid),
		"security.nesting":           "true",
		"user.workshop.project-id":   projectId,
		"user.user-data":             cloudInitConfig,
		"user.workshop.file":         string(f),
		"user.workshop.sdks":         "{}",
		"nvidia.driver.capabilities": cfgNvidiaDriverCapabilities,
		"nvidia.runtime":             cfgNvidiaRuntime,
		// LXC appears to have a race condition wherein a proxy device mounted in
		// a dynamically created directory has the potential to be 'masked' by this
		// directory. We create an explicit mount for /tmp here (one such dymanic
		// directory) to allow us to mount X11 sockets reliably.
		// See: https://github.com/lxc/lxc/issues/434
		"raw.lxc": "lxc.mount.entry = tmpfs tmp tmpfs defaults",
	}
	return cfg, nil
}

func FakeStartCommand(script string) func() {
	old := startCommand
	startCommand = script
	return func() {
		startCommand = old
	}
}
