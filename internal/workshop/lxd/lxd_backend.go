package lxdbackend

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	LxdSock     = "/var/snap/lxd/common/lxd/unix.socket"
	storagePool = "default"
)

var (
	defaultDevices     = createDefaultDevices
	checkNvidiaRuntime = checkNvidia
)

var (
	// However many backend instances are created, downloads are always a single
	// instance map with the LXD backend.
	imageLock        sync.Mutex
	currentDownloads map[string]*downloadOp
)

//go:embed start_command.sh
var startCommand string

func init() {
	imageLock.Lock()
	defer imageLock.Unlock()
	if currentDownloads == nil {
		currentDownloads = make(map[string]*downloadOp)
	}
}

type Backend struct {
}

func InstanceName(name string, project_id string) string {
	return fmt.Sprintf("%s-%s", name, project_id)
}

func ImageAlias(name string) string {
	return fmt.Sprintf("workshop-%s-%s", name, runtime.GOARCH)
}

func New() (*Backend, error) {
	server := Backend{}

	if srv := os.Getenv("WORKSHOP_IMAGE_SERVER"); srv != "" {
		imageServer = srv
	}

	return &server, nil
}

func (s *Backend) LaunchWorkshop(ctx context.Context, file *workshop.File) error {
	var err error
	var image *api.Image

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	userName, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key user not found")
	}

	// Check if we have the base image stored locally
	alias, _, err := conn.GetImageAlias(ImageAlias(file.Base))
	if err != nil {
		return err
	}

	image, _, err = conn.GetImage(alias.Target)
	if err != nil {
		return err
	}

	usr, err := workshop.LookupUsername(userName)
	if err != nil {
		return err
	}

	config, err := s.workshopConfig(projectId, usr.Uid, usr.Gid, file)
	if err != nil {
		return err
	}
	req := api.InstancesPost{
		InstancePut: api.InstancePut{
			Devices: defaultDevices(),
			Config:  config,
		},
		Name: InstanceName(file.Name, projectId),
		Type: api.InstanceType("container"),
		Source: api.InstanceSource{
			Type:        "image",
			Fingerprint: image.Fingerprint,
			Project:     LxdProjectName(userName),
		},
	}

	op, err := conn.CreateInstance(req)
	if err != nil {
		return err
	}

	// CreateInstance cannot be cancelled in LXD.
	return op.Wait()
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
		Timeout: 45,
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

	if err = s.updateInstanceState(conn, ctx, name, "start", false); err != nil {
		return err
	}

	// Workshop started, enable autostart.
	if err = s.addWorkshopConfig(conn, ctx, name, &workshop.WorkshopConfigValue{Name: "boot.autostart", Value: "true"}); err != nil {
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

	// Workshop stopped, disable autostart
	if err = s.addWorkshopConfig(conn, ctx, name, &workshop.WorkshopConfigValue{Name: "boot.autostart", Value: "false"}); err != nil {
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

func (s *Backend) AddWorkshopMount(ctx context.Context, name string, device workshop.Mount) error {
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
	if device.Type == workshop.Volume {
		inst.Devices[device.Name] = map[string]string{"type": "disk",
			"pool":   "default",
			"path":   device.What,
			"source": device.Where}
	} else {
		inst.Devices[device.Name] = map[string]string{"type": "disk", "source": device.What,
			"path": device.Where}
	}
	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}

	return op.WaitContext(ctx)
}

func (s *Backend) RemoveWorkshopMount(ctx context.Context, name string, device string) error {
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

	delete(inst.Devices, device)
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
				return err
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
	if err = s.updateInstanceState(conn, ctx, name, "stop", true); err != nil {
		logger.Noticef("On RemoveWorkshop: failed to stop %q workshop: %v", name, err)
	}

	op, err := conn.DeleteInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	// DeleteInstance cannot be cancelled in LXD.
	return op.Wait()
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

func (s *Backend) CreateVolume(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	// Create the storage volume entry
	vol := api.StorageVolumesPost{}
	vol.Name = name
	vol.Type = "custom"
	vol.ContentType = "filesystem"
	vol.Config = map[string]string{}

	err = conn.CreateStoragePoolVolume(storagePool, vol)
	if api.StatusErrorCheck(err, http.StatusConflict) {
		return workshop.ErrVolumeAlreadyExists
	}
	return err
}

func (s *Backend) AttachVolume(ctx context.Context, wp, name, what string) error {
	return s.AddWorkshopMount(ctx, wp, workshop.Mount{Name: name, What: what, Where: name, Type: workshop.Volume})
}

func (s *Backend) DetachVolume(ctx context.Context, wp, name string) error {
	return s.RemoveWorkshopMount(ctx, wp, name)
}

func (s *Backend) DeleteVolume(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	return conn.DeleteStoragePoolVolume(storagePool, "custom", name)
}

func (s *Backend) LxdClient(ctx context.Context) (lxd.InstanceServer, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	if srv, err := lxd.ConnectLXDUnixWithContext(ctx, LxdSock, nil); err != nil {
		return nil, err
	} else {
		if err = InitLxdProject(srv, user); err != nil {
			return nil, err
		}
		return srv.UseProject(LxdProjectName(user)), nil
	}
}

func createDefaultDevices() map[string]map[string]string {
	// configure .untrusted socket mount
	shostpath := dirs.SocketPath + ".untrusted"
	swspath := filepath.Join(dirs.WorkshopRunDir, filepath.Base(shostpath))
	return map[string]map[string]string{
		"root":                 {"type": "disk", "pool": storagePool, "path": "/"},
		"workshop.network":     {"type": "nic", "network": "lxdbr0", "name": "eth0"},
		"workshop.socket":      {"type": "proxy", "connect": "unix:" + shostpath, "listen": "unix:" + swspath, "bind": "instance", "mode": "0666"},
		"workshop.workshopctl": {"type": "disk", "source": filepath.Join(dirs.ExecDir, "workshopctl"), "path": "/usr/bin/workshopctl"},
	}
}

func checkNvidia() (bool, error) {
	conn, err := lxd.ConnectLXDUnixWithContext(context.Background(), LxdSock, nil)
	if err != nil {
		return false, err
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
runcmd:
  - systemctl daemon-reload
  - systemctl enable xauth-copy.service
  - systemctl enable --now xauth-watch.path
`

	f, err := yaml.Marshal(file)
	if err != nil {
		return map[string]string{}, err
	}

	cfg := map[string]string{
		"raw.idmap":                fmt.Sprint("uid ", userid, " 1000\ngid ", groupid, " 1000"),
		"security.nesting":         "true",
		"user.workshop.project-id": projectId,
		"user.user-data":           cloudInitConfig,
		"user.workshop.file":       string(f),
		// LXC appears to have a race condition wherein a proxy device mounted in
		// a dynamically created directory has the potential to be 'masked' by this
		// directory. We create an explicit mount for /tmp here (one such dymanic
		// directory) to allow us to mount X11 sockets reliably.
		// See: https://github.com/lxc/lxc/issues/434
		"raw.lxc": "lxc.mount.entry = tmpfs tmp tmpfs defaults",
	}

	nvidiaRuntime, err := checkNvidiaRuntime()
	if err != nil {
		return nil, err
	}

	if nvidiaRuntime {
		// nvidia.* properties must be set at launch as otherwise it requires a
		// container restart to take effect.
		cfg["nvidia.driver.capabilities"] = "all"
		cfg["nvidia.runtime"] = "true"
	}
	return cfg, nil
}

func FakeDefaultDevices(f func() map[string]map[string]string) func() {
	oldDefault := defaultDevices
	defaultDevices = f
	return func() { defaultDevices = oldDefault }
}

func FakeStartCommand(script string) func() {
	old := startCommand
	startCommand = script
	return func() {
		startCommand = old
	}
}
