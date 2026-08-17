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
	"cmp"
	"context"
	"embed"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/x-go/strutil/shlex"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/syscheck"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	storagePool           = "workshop"
	storagePoolMinimalGiB = 5

	networkName = "workshopbr0"
	networkType = "bridge"

	// networkDomain is the DNS domain served by the workshop bridge. Workshop
	// instances are reachable as "<instance>.<networkDomain>", and configure-dns
	// registers this domain with the host resolver.
	networkDomain = "wp"

	// NetworkBridgeName is the name of the LXD bridge network used by
	// workshops, exported for use by other packages (e.g. firewall checks).
	NetworkBridgeName = networkName
)

var (
	startCommandTimeout = 1 * time.Minute
	storagePoolDriver   = "zfs"
)

//go:embed start_command.sh
var startCommand string

func init() {
	if osutil.IsWSL() {
		storagePoolDriver = "btrfs"
	}

	// Order matters: capabilities (version and storage) must be validated
	// before ensureBackendReady attempts to create the storage pool and
	// network. Registering it as a check also lets the daemon recover after
	// LXD is installed or refreshed, without a restart.
	syscheck.RegisterCheck(checkServerCapabilities)
	syscheck.RegisterCheck(ensureBackendReady)
	syscheck.RegisterCheck(checkStorageSpace)
}

type Backend struct {
}

var _ workshop.Backend = (*Backend)(nil)

func InstanceName(name string, project_id string) string {
	return fmt.Sprintf("%s-%s", name, project_id)
}

func instanceStashName(name string, pid string) string {
	return "stash-" + InstanceName(name, pid)
}

func workshopName(instance string) string {
	idx := strings.LastIndex(instance, "-")
	if idx == -1 {
		return ""
	}

	// drop the project id from the name
	return instance[:idx]
}

func workshopProjectId(instance string) (string, string) {
	idx := strings.LastIndex(instance, "-")
	if idx == -1 {
		return "", ""
	}
	return instance[:idx], instance[idx+1:]
}

func ErrorLxdBackend(err error) error {
	switch {
	case errors.Is(err, unix.ECONNREFUSED):
		return fmt.Errorf(`cannot connect to LXD: %w

Maybe LXD daemon isn't active?
To start the LXD daemon: 'sudo snap start lxd'`, err)
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf(`cannot connect to LXD: %w

Maybe LXD isn't installed?
To install LXD: 'sudo snap install --channel=6/stable lxd'`, err)
	default:
		return err
	}
}

func checkVersion(version string) error {
	const minimalLXDMajor = 6
	const minimalLXDMinor = 8

	comps := strings.Split(version, ".")

	// LXD non-LTS versions are in the form of X.Y, while LTS versions are
	// in the form of X.Y.Z. Accept both.
	if len(comps) != 2 && len(comps) != 3 {
		return fmt.Errorf("%w: cannot parse LXD server version %q", workshop.ErrIncompatibleBackend, version)
	}

	major, err := strconv.Atoi(comps[0])
	minor, err2 := strconv.Atoi(comps[1])
	if cmp.Or(err, err2) != nil {
		return fmt.Errorf("%w: cannot parse LXD server version %q", workshop.ErrIncompatibleBackend, version)
	}

	if major < minimalLXDMajor || (major == minimalLXDMajor && minor < minimalLXDMinor) {
		return fmt.Errorf("%w: LXD server version %q is not supported; required >= %d.%d.*\nTo refresh LXD: 'sudo snap refresh --channel=6/stable lxd'", workshop.ErrIncompatibleBackend, version, minimalLXDMajor, minimalLXDMinor)
	}
	return nil
}

func checkStorageDriver(drivers []api.ServerStorageDriverInfo) error {
	hasDriver := func(driver api.ServerStorageDriverInfo) bool {
		return driver.Name == storagePoolDriver
	}
	if slices.ContainsFunc(drivers, hasDriver) {
		return nil
	}

	// The LXD error message when creating a pool is:
	//  Error: Error loading "zfs" module: Failed to run: modprobe -b zfs:
	//  exit status 1 (modprobe: FATAL: Module zfs not found ...)
	// We keep the first part for consistency, the rest doesn't add much.
	return fmt.Errorf(`suitable storage backend not found: error loading %q module`, storagePoolDriver)
}

// checkStorageSpace puts the daemon into degraded mode when the workshop
// storage pool is 90% or more full, preventing further launches from failing
// weirdly (e.g. on workshop remove causing troubles with backup yaml files
// removal) due to lack of space.
func checkStorageSpace() error {
	const fullThresholdPct = 90

	conn, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		// LXD is not available; checkServerCapabilities handles this case.
		return nil
	}
	defer conn.Disconnect()

	res, err := conn.GetStoragePoolResources(storagePool)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		// Pool not yet created; ensureBackendReady handles this case.
		return nil
	}
	if err != nil {
		return err
	}

	if res.Space.Total == 0 {
		// Cannot determine usage (e.g. directory-backed pools report zero).
		return nil
	}

	usedPct := float64(res.Space.Used) / float64(res.Space.Total) * 100
	if usedPct >= fullThresholdPct {
		availGiB := float64(res.Space.Total-res.Space.Used) / (1024 * 1024 * 1024)
		return fmt.Errorf("storage pool %q is %.0f%% full (%.1f GiB available); "+
			"free up space or expand the pool with `lxc storage volume set workshop size=<N>GiB`\n"+
			"For details see: https://ubuntu.com/workshop/docs/reference/workshops/#storage-pools-and-drivers",
			storagePool, usedPct, availGiB)
	}

	return nil
}

func checkServerCapabilities() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := lxd.ConnectLXDUnixWithContext(ctx, "", nil)
	if err != nil {
		return ErrorLxdBackend(err)
	}
	defer conn.Disconnect()

	info, _, err := conn.GetServer()
	if err != nil {
		return err
	}

	if err := checkVersion(info.Environment.ServerVersion); err != nil {
		return err
	}

	return checkStorageDriver(info.Environment.StorageSupportedDrivers)
}

// New constructs the LXD backend and attempts to prepare the required LXD
// storage pool and network. It always returns a usable backend; if LXD is not
// yet ready the error is reported by the system check (see init), which puts
// the daemon into degraded mode and retries until LXD becomes available.
func New() (*Backend, error) {
	server := Backend{}

	if srv := os.Getenv("WORKSHOP_IMAGE_SERVER"); srv != "" {
		imageServer = srv
	}

	return &server, ensureBackendReady()
}

// ensureBackendReady creates the LXD storage pool and network the backend
// relies on, it is idempotent.
func ensureBackendReady() error {
	// TODO: consider separating the storage pool or network for different users.
	conn, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return ErrorLxdBackend(err)
	}
	defer conn.Disconnect()

	// Create LXD storage pool if it doesn't exist.
	pools, err := conn.GetStoragePools()
	if err != nil {
		return err
	}
	if idx := slices.IndexFunc(pools, func(p api.StoragePool) bool { return p.Name == storagePool }); idx < 0 {
		req := api.StoragePoolsPost{
			Name:   storagePool,
			Driver: storagePoolDriver,
		}
		op, err := conn.CreateStoragePool(req)
		if err != nil {
			return err
		}
		if err := op.Wait(); err != nil {
			return err
		}

		// Ensure the new pool has enough total space available.
		pool, etag, err := conn.GetStoragePool(storagePool)
		if err != nil {
			return err
		}

		res, err := conn.GetStoragePoolResources(storagePool)
		if err != nil {
			return err
		}

		gibTotal := uint64(res.Space.Total) / (1024 * 1024 * 1024)
		if gibTotal < storagePoolMinimalGiB {
			// Ensure the storage pool is no less than 5GiB, otherwise it makes
			// it running out of space in tests and environments with less than
			// ~14GiB of available space. LXD defaults to 20% in those cases
			// which results in a ~2GiB pool size for workshop.
			pool.Config["size"] = strconv.FormatUint(storagePoolMinimalGiB*1024*1024*1024, 10)
			op, err = conn.UpdateStoragePool(storagePool, pool.Writable(), etag)
			if err != nil {
				return err
			}
			if err := op.Wait(); err != nil {
				logger.Noticef("On ensureBackendReady: failed to set storage pool to the minimal size: %dGiB, %s", storagePoolMinimalGiB, err)
				return err
			}

			logger.Noticef("On ensureBackendReady: set storage pool to the minimal size: %dGiB", storagePoolMinimalGiB)
		}
	} else if pools[idx].Driver != storagePoolDriver {
		return fmt.Errorf("storage pool %q already exists with a different driver: %q (expected %q)", storagePool, pools[idx].Driver, storagePoolDriver)
	}

	network, etag, err := conn.GetNetwork(networkName)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		req := api.NetworksPost{
			Name: networkName,
			Type: networkType,
			NetworkPut: api.NetworkPut{
				Config: map[string]string{
					"dns.domain": networkDomain,
				},
				Description: "Bridge network for workshops",
			},
		}

		op, err := conn.CreateNetwork(req)
		if err != nil {
			return err
		}
		if err := op.Wait(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if network.Type != networkType {
		return fmt.Errorf("network %q already exists with a different type: %q", networkName, network.Type)
	} else if network.Config["dns.domain"] != networkDomain {
		network.Config["dns.domain"] = networkDomain
		op, err := conn.UpdateNetwork(network.Name, network.Writable(), etag)
		if err != nil {
			return err
		}
		if err := op.Wait(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Backend) LaunchOrRebuildWorkshop(ctx context.Context, file *workshop.File, snapshot workshop.Snapshot) error {
	conn, snapshotConn, err := s.snapshotClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key user not found")
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	usr, err := osutil.UserLookup(username)
	if err != nil {
		return err
	}

	config, err := s.workshopConfig(projectId, usr.Uid, usr.Gid, file, snapshot.Format, snapshot.Image.Fingerprint)
	if err != nil {
		return err
	}
	req := api.InstancesPost{
		InstancePut: api.InstancePut{
			Config:  config,
			Devices: defaultDevices(usr, projectId, file.Name),
		},
		Name: InstanceName(file.Name, projectId),
		Type: api.InstanceTypeContainer,
	}

	if !snapshot.IsBase() {
		return s.launchOrRebuildFromSnapshot(conn, snapshotConn, usr, req, snapshot)
	}

	req.Source = api.InstanceSource{
		Type:        api.SourceTypeImage,
		Fingerprint: snapshot.Image.Fingerprint,
	}
	if err := s.launchOrRebuildFromImage(conn, usr, req); err != nil {
		return err
	}

	return s.adjustInstanceTemplates(conn, req.Name)
}

func (s *Backend) launchOrRebuildFromImage(conn lxd.InstanceServer, usr *user.User, req api.InstancesPost) error {
	inst, _, err := conn.GetInstance(req.Name)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		// Create a new workshop.
		config, err := sshConfig(usr, req.Name+"."+networkDomain)
		if err != nil {
			return err
		}
		maps.Copy(req.Config, config)

		op, err := conn.CreateInstance(req)
		if err != nil {
			return err
		}
		return op.Wait()
	}
	if err != nil {
		return err
	}

	// Rebuild the existing workshop.
	op, err := conn.RebuildInstance(inst.Name, api.InstanceRebuildPost{Source: req.Source})
	if err != nil {
		return err
	}
	if err = op.Wait(); err != nil {
		return err
	}

	rebuilt, etag, err := conn.GetInstance(inst.Name)
	if err != nil {
		return err
	}

	// When rebuilding an instance from an image, LXD resets the image
	// properties and volatile.base_image. It also copies volatile.uuid
	// over volatile.uuid.generation. The latter seems to be unused for
	// containers. Finally, it clears volatile.idmap.next and
	// volatile.last_state.idmap. All other options are unchanged. We
	// preserve the LXD-managed options and forget the other ones.
	maps.DeleteFunc(rebuilt.Config, func(k, v string) bool {
		return optionDomain(k) == customOption
	})
	if rebuilt.Config != nil {
		maps.Copy(rebuilt.Config, req.Config)
		req.Config = rebuilt.Config
	}

	req.Architecture = rebuilt.Architecture
	op, err = conn.UpdateInstance(rebuilt.Name, req.InstancePut, etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

//go:embed templates/*.tpl
var instanceTemplates embed.FS

// adjustInstanceTemplates ensures LXD creates certain files before the
// instance starts. The files are:
//   - /etc/cloud/cloud.cfg.d/90_workshop.cfg (configure cloud-init settings
//     before it runs)
//   - /etc/hostname (set to the workshop's name)
//   - /etc/machine-id (set to the LXD UUID, without dashes)
//   - /etc/ssh_* (set workshop-specific SSH keys)
//   - /etc/systemd/network/10-cloud-init-eth0.network.d/workshop.conf (set
//     systemd-networkd options that are workshop-specific or unsupported by
//     clout-init)
//   - /var/lib/workshop/run/workshop.socket.untrusted (empty, to be replaced
//     with a proxy socket)
//
// Also removes cloud-init seed files for NoCloud. The current Ubuntu 20.04
// images contain a recent version of cloud-init that supports LXD, but these
// files force it to use NoCloud instead. See
// https://docs.cloud-init.io/en/latest/development/dir_layout.html#seed.
//
// The seed files contain a hostname and instance-id, both set to the instance
// name. This is OK when launching a new workshop, or rebuilding a workshop
// from an image (although the instance-id is different for 22.04 and up), but
// when rebuilding a workshop from a snapshot, it results in both the hostname
// and instance-id being taken from the snapshot.
func (s *Backend) adjustInstanceTemplates(conn lxd.InstanceServer, name string) error {
	fromImage := []string{"create"}
	fromSnapshot := []string{"create", "copy"}

	templates := map[string]*api.ImageMetadataTemplate{
		"/etc/cloud/cloud.cfg.d/90_workshop.cfg": {
			When:     fromImage,
			Template: "cloud.cfg.tpl",
		},
		"/etc/hostname": {
			When:     fromSnapshot,
			Template: "hostname.tpl",
		},
		"/etc/machine-id": {
			When:     fromSnapshot,
			Template: "machine-id.tpl",
		},
		"/etc/ssh/ssh_host_ed25519_key": {
			When:     fromSnapshot,
			Template: "ssh_host_ed25519_key.tpl",
		},
		"/etc/ssh/ssh_host_ed25519_key.pub": {
			When:     fromSnapshot,
			Template: "ssh_host_ed25519_key.pub.tpl",
		},
		"/etc/ssh/ssh_host_ed25519_key-cert.pub": {
			When:     fromSnapshot,
			Template: "ssh_host_ed25519_key-cert.pub.tpl",
		},
		"/etc/ssh/ssh_ca_ed25519_key.pub": {
			When:     fromSnapshot,
			Template: "ssh_ca_ed25519_key.pub.tpl",
		},
		"/etc/systemd/network/10-cloud-init-eth0.network.d/workshop.conf": {
			When:       fromSnapshot,
			Template:   "eth0.network.tpl",
			Properties: map[string]string{"domain": networkDomain},
		},
		dirs.WorkshopSocketPath + ".untrusted": {
			When:       fromImage,
			CreateOnly: true,
			Template:   "workshop.socket.untrusted.tpl",
		},
	}

	metadata, etag, err := conn.GetInstanceMetadata(name)
	if err != nil {
		return err
	}

	deleted := make([]string, 0, len(metadata.Templates))
	for path, template := range metadata.Templates {
		if strings.HasPrefix(path, "/var/lib/cloud/seed/nocloud-net/") {
			// Remove NoCloud metadata and fall through to remove templates.
			delete(metadata.Templates, path)
		} else if t := templates[path]; t == nil || template == nil || t.Template == template.Template {
			// Skip other templates unless we're about to replace them.
			continue
		}
		if template != nil {
			deleted = append(deleted, template.Template)
		}
	}

	if metadata.Templates == nil {
		metadata.Templates = map[string]*api.ImageMetadataTemplate{}
	}
	maps.Copy(metadata.Templates, templates)

	files, err := instanceTemplates.ReadDir("templates")
	if err != nil {
		return err
	}
	for _, entry := range files {
		if err := createInstanceTemplateFile(conn, name, entry.Name()); err != nil {
			return err
		}
	}

	if err := conn.UpdateInstanceMetadata(name, *metadata, etag); err != nil {
		return err
	}

	for _, template := range deleted {
		if err := conn.DeleteInstanceTemplateFile(name, template); err != nil {
			return err
		}
	}

	return nil
}

func createInstanceTemplateFile(conn lxd.InstanceServer, instance, filename string) error {
	f, err := instanceTemplates.Open(path.Join("templates", filename))
	if err != nil {
		return err
	}
	defer f.Close()
	return conn.CreateInstanceTemplateFile(instance, filename, f.(io.ReadSeeker))
}

func (s *Backend) updateInstanceState(conn lxd.InstanceServer, ctx context.Context, name, action string, timeout int) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	req := api.InstanceStatePut{
		Action:  action,
		Timeout: timeout,
		// Currently force is equivalent to zero timeout, but we might
		// as well set it just in case.
		Force: timeout == 0,
	}

	op, err := conn.UpdateInstanceState(InstanceName(name, projectId), req, "")
	if err == nil {
		err = op.WaitContext(ctx)
	}
	if isAlready(err, action) {
		err = nil
	}
	return err
}

func isAlready(err error, action string) bool {
	if err == nil {
		return false
	}

	if action == "start" && err.Error() == "The instance is already running" {
		return true
	}
	if action == "stop" && err.Error() == "The instance is already stopped" {
		return true
	}
	return false
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
		// Stop workshop's timeout is handled by LXD API, so no need to have
		// a context with a timeout.
		if e := s.stopWorkshop(conn, cleanupCtx, name, true); e != nil {
			logger.Noticef("On StartWorkshop: cannot stop %q workshop on cleanup: %v", name, e)
		}
	})

	if err := s.updateInstanceState(conn, ctx, name, "start", 60); err != nil {
		return err
	}

	for i := range 2 {
		// Workshop started, enable autostart.
		if err := s.setAutoStart(conn, ctx, name, true); err != nil {
			if i == 0 && api.StatusErrorCheck(err, http.StatusPreconditionFailed) && strings.HasPrefix(err.Error(), "ETag does not match: ") {
				// TODO: remove the loop after modifying the logic to wait for
				// LXD's instance ready event. Currently, the instance may or
				// may not set itself to Ready, so we can't rely on it. When it
				// does so, LXD sets volatile.last_state.ready, which can
				// invalidates the ETag; a single retry is enough to fix it.
				continue
			}
			return err
		}
		break
	}

	var stderr strings.Builder
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
		ExecControls: workshop.ExecControls{
			Stderr: &stderr,
		},
	}

	exectx, err := s.execCommand(conn, ctx, name, &args)
	if err != nil {
		return err
	}

	var errExec *workshop.ErrExec
	if err := exectx.WaitExecution(ctx); errors.As(err, &errExec) {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return err
		}
		return errors.New(message)
	} else if err != nil {
		return err
	}

	if err := s.addWorkshopCNAMEs(conn, ctx, name); err != nil {
		logger.Noticef("On StartWorkshop: failed to add %q workshop CNAME records: %v", name, err)
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
	if err := s.setAutoStart(conn, ctx, name, false); err != nil {
		return err
	}

	timeout := 60
	if force {
		timeout = 10
	}
	err := s.updateInstanceState(conn, ctx, name, "stop", timeout)
	if err != nil && force {
		logger.Noticef("On StopWorkshop: failed to stop %q workshop: %v", name, err)
		err = s.updateInstanceState(conn, ctx, name, "stop", 0)
	}
	if err != nil {
		return err
	}

	if err := s.removeWorkshopCNAMEs(conn, ctx, name); err != nil {
		logger.Noticef("On StopWorkshop: failed to remove %q workshop CNAME records: %v", name, err)
	}

	return nil
}

func (s *Backend) setAutoStart(conn lxd.InstanceServer, ctx context.Context, name string, autostart bool) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		return err
	}

	inst.Config["boot.autostart"] = strconv.FormatBool(autostart)

	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}
	return op.WaitContext(ctx)
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

	disk := mountToLxdDisk(mount)
	if maps.Equal(inst.Devices[mount.Name], disk) {
		return nil
	}
	inst.Devices[mount.Name] = disk

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

	if _, ok := inst.Devices[mount]; !ok {
		return nil
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
		Command:     args.EffectiveCommand(),
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
	rev := revert.New()
	defer rev.Fail()

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return workshop.ExecContext{}, err
	}
	rev.Add(conn.Disconnect)

	exectx, err := s.execCommand(conn, ctx, name, args)
	if err != nil {
		return exectx, err
	}

	// Extend the connection until WaitExecution is done.
	waitExecution := exectx.WaitExecution
	exectx.WaitExecution = func(ctx context.Context) error {
		defer conn.Disconnect()
		return waitExecution(ctx)
	}
	rev.Success()
	return exectx, err
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

	var cnames []cname
	network, _, err := conn.GetNetwork(networkName)
	if err != nil {
		if !api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil, err
		}
	} else {
		cnames, _ = unmarshalDnsmasq(network.Config["raw.dnsmasq"])
	}
	if cnames == nil {
		// Tell loadWorkshop to add the hostname-fallback note.
		cnames = []cname{}
	}

	workshop, err := s.loadWorkshop(conn, inst, p, cnames)
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

func (b *Backend) loadWorkshop(conn lxd.InstanceServer, inst *api.Instance, p workshop.Project, cnames []cname) (*workshop.Workshop, error) {
	f, err := workshopFile(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("cannot load workshop: %v", err)
	}

	format, err := sdk.ParseRevision(inst.Config[workshop.ConfigWorkshopSnapshotFormat])
	if err != nil {
		return nil, err
	}

	image := workshop.BaseImage{
		Name:        f.Base,
		Fingerprint: inst.Config[workshop.ConfigWorkshopBaseFingerprint],
	}

	sdks := map[string]workshop.SdkInstallation{}
	for key, device := range inst.Devices {
		s, err := maybeSdkInstallation(key, device)
		if err != nil {
			return nil, err
		}
		if s != nil {
			sdks[s.Name] = *s
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

	running := inst.StatusCode == api.Running || inst.StatusCode == api.Ready
	hostname := b.hostname(f.Name, p, running, cnames)

	return &workshop.Workshop{
		Backend:  b,
		Project:  p,
		Name:     f.Name,
		Format:   format,
		Image:    image,
		Running:  running,
		Sdks:     sdks,
		Profiles: profs,
		File:     f,
		Hostname: hostname,
	}, nil
}

func (s *Backend) hostname(name string, p workshop.Project, running bool, cnames []cname) workshop.Hostname {
	var hostname workshop.Hostname
	if cnames == nil {
		// Skip adding notes if we weren't given the CNAME entries (i.e. when
		// loading multiple workshops).
		return hostname
	}

	idx := slices.IndexFunc(cnames, func(c cname) bool {
		return c.Workshop == name && c.ProjectId == p.ProjectId
	})
	if idx >= 0 {
		hostname.Domain = cnames[idx].friendly()
		hostname.Note = cnames[idx].Note
	} else if running {
		hostname.Domain = InstanceName(name, p.ProjectId) + "." + networkDomain
		hostname.Note = "hostname-fallback"
	}

	return hostname
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
	args := lxd.GetInstancesArgs{
		InstanceType: api.InstanceTypeContainer,
		Filters:      []string{"config.user.workshop.project-id=" + p.ProjectId},
	}
	instances, err := conn.GetInstances(args)
	if err != nil {
		return nil, err
	}

	var workshops []*workshop.Workshop
	for _, i := range instances {
		ws, err := s.loadWorkshop(conn, &i, p, nil)
		if err != nil {
			logger.Debugf("Workshop Backend on ProjectsWorkshops: %v", err)
			continue
		}
		workshops = append(workshops, ws)
	}

	return workshops, nil
}

func (s *Backend) RemoveWorkshop(ctx context.Context, name string) (err error) {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	op, err := conn.DeleteInstance(InstanceName(name, projectId), false)
	if err != nil {
		return err
	}

	// DeleteInstance cannot be cancelled in LXD.
	return op.Wait()
}

func (s *Backend) WorkshopFs(ctx context.Context, name string) (fsutil.Fs, error) {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return fsutil.Fs{}, err
	}
	defer conn.Disconnect()

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fsutil.Fs{}, fmt.Errorf("context key project-id not found")
	}

	return s.instanceFs(conn, InstanceName(name, projectId))
}

func (s *Backend) instanceFs(conn lxd.InstanceServer, name string) (fsutil.Fs, error) {
	sftp, err := conn.GetInstanceFileSFTP(name)
	if err != nil {
		return fsutil.Fs{}, err
	}

	return fsutil.NewSftpFs(sftp, workshop.RootUmask), nil
}

func ConnectLxd(ctx context.Context) (lxd.InstanceServer, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	project, err := lxdProjectName(user)
	if err != nil {
		return nil, err
	}

	conn, err := lxd.ConnectLXDUnixWithContext(ctx, "", nil)
	if err != nil {
		return nil, ErrorLxdBackend(err)
	}

	if err := initLxdProject(conn, project, user); err != nil {
		conn.Disconnect()
		return nil, err
	}

	return conn.UseProject(project), err
}

func (s *Backend) LxdClient(ctx context.Context) (lxd.InstanceServer, error) {
	return ConnectLxd(ctx)
}

func defaultDevices(usr *user.User, pid, w string) map[string]map[string]string {
	devices := map[string]map[string]string{
		"root":             {"type": "disk", "pool": storagePool, "path": "/"},
		"workshop.network": {"type": "nic", "network": networkName, "name": "eth0"},
	}

	mounts, proxies := workshop.DefaultDevices(pid, w)
	for _, mount := range mounts {
		devices[mount.Name] = mountToLxdDisk(mount)
	}

	for _, proxy := range proxies {
		devices[proxy.Name] = proxyToLxdDevice(usr, proxy)
	}

	return devices
}

func proxyToLxdDevice(usr *user.User, proxy workshop.ProxyEntry) map[string]string {
	device := map[string]string{
		"type":    "proxy",
		"connect": proxy.Connect.Protocol + ":" + proxy.Connect.Address,
		"listen":  proxy.Listen.Protocol + ":" + proxy.Listen.Address,
		"mode":    "0666",
	}
	switch proxy.Direction {
	case workshop.WorkshopToHost:
		device["bind"] = "instance"
		device["security.uid"] = usr.Uid
		device["security.gid"] = usr.Gid
	case workshop.HostToWorkshop:
		device["bind"] = "host"
	}
	return device
}

func (s *Backend) workshopConfig(projectId string, userid, groupid string, file *workshop.File, format sdk.Revision, baseFingerprint string) (map[string]string, error) {
	cloudConfigTemplate := `
#cloud-config
users:
  - default
  - name: workshop
    primary_group: workshop
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    create_groups: false
    groups:
    - 'adm'
    - 'cdrom'
    - 'sudo'
    - 'dip'
    - 'plugdev'
    - 'audio'
    - 'netdev'
    - 'lxd'
    - 'video'
    - 'render'
    # Compatibility GIDs for various host systems:
    - '108' # netdev on 26.04
    - '111' # netdev on 24.04
    - '118' # netdev on 20.04
    - '119' # netdev on 22.04
    - '109' # render on 20.04
    - '110' # render on 22.04
    - '990' # render on 26.04
    - '992' # render on 24.04
bootcmd:
- |
  set -e
  maybe_groupadd() {
      # Ignore GID not unique (exit code 4) or group name not unique (exit code 9)
      groupadd -g "$1" -r "$2" || case $? in 4|9) ;; *) return $? ;; esac
  }
  maybe_groupadd 1000 workshop
  maybe_groupadd 108 netdev-compat-108
  maybe_groupadd 111 netdev-compat-111
  maybe_groupadd 118 netdev-compat-118
  maybe_groupadd 119 netdev-compat-119
  maybe_groupadd 109 render-compat-109
  maybe_groupadd 110 render-compat-110
  maybe_groupadd 990 render-compat-990
  maybe_groupadd 992 render-compat-992
- chmod 0600 /etc/ssh/ssh_host_ed25519_key
apt:
  conf: |
    # Installed by workshop

    # Don't automatically install recommended packages
    APT::Install-Recommends "0";

    # Don't automatically install suggested packages
    APT::Install-Suggests "0";

    # Bypass confirmation prompts
    APT::Get::Assume-Yes "1";
grub_dpkg:
  enabled: false
ssh_deletekeys: false
ssh_genkeytypes: [ed25519]
write_files:
  - path: /etc/cloud/cloud-init.disabled
    defer: true
  - path: /etc/ssh/sshd_config.d/90-workshop.conf
    content: |
      HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub
      TrustedUserCAKeys /etc/ssh/ssh_ca_ed25519_key.pub
  - path: /etc/systemd/system/workshop-waitready.service
    content: |
      [Unit]
      Description=Signal workshop readiness to LXD

      [Service]
      Type=notify
      ExecStart=/usr/local/lib/workshop/waitready

      [Install]
      WantedBy=multi-user.target
runcmd:
  # Project directory is required for 'workshop exec'.
  - install --directory --mode=755 /project /usr/local/bin /usr/local/lib/workshop {{shquote .WorkshopStateDir}}
  # Create XDG base directories so SDKs don't need an extra mode=700 step.
  - install --directory --mode=700 --owner=workshop --group=workshop /home/workshop/.cache /home/workshop/.config /home/workshop/.local
  # Create ~/.local/bin so SDKs don't need to source ~/.profile to add it to the PATH.
  - install --directory --mode=755 --owner=workshop --group=workshop /home/workshop/.local/bin
  # Put workshopctl on the PATH.
  - ln -sf {{shquote .WorkshopCtlPath}} /usr/local/bin/workshopctl
  - ln -sf ../../bin/workshopctl /usr/local/lib/workshop/waitready
  - systemctl enable --now workshop-waitready.service
`[1:]

	var cloudConfig strings.Builder
	funcs := map[string]any{
		"shquote": shlex.Quote,
	}
	dot := struct {
		WorkshopCtlPath  string
		WorkshopStateDir string
	}{
		WorkshopCtlPath:  filepath.Join(dirs.WorkshopGuestBinDir, filepath.Base(dirs.WorkshopCtlPath)),
		WorkshopStateDir: dirs.WorkshopStateDir,
	}
	t := template.Must(template.New("cloud-config").Funcs(funcs).Parse(cloudConfigTemplate))
	if err := t.Execute(&cloudConfig, dot); err != nil {
		return nil, err
	}

	f, err := yaml.Marshal(file)
	if err != nil {
		return map[string]string{}, err
	}

	// Include all options we might change, even those with default values,
	// so that workshops can be rebuilt.
	cfg := map[string]string{
		"boot.autostart":                 "false",
		"raw.idmap":                      fmt.Sprintf("uid %s %s\ngid %s %s", userid, workshop.User.Uid, groupid, workshop.User.Gid),
		"security.nesting":               "true",
		"cloud-init.user-data":           cloudConfig.String(),
		"user.workshop.format-revision":  format.String(),
		"user.workshop.project-id":       projectId,
		"user.workshop.name":             file.Name,
		"user.workshop.file":             string(f),
		"user.workshop.base-fingerprint": baseFingerprint,
		// LXC appears to have a race condition wherein a proxy device mounted in
		// a dynamically created directory has the potential to be 'masked' by this
		// directory. We create an explicit mount for /tmp here (one such dynamic
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
