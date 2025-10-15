package lxdbackend

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

func (s *Backend) Snapshot(ctx context.Context, name, sk string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, layerConn, err := s.layerClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	// Copy workshop to preserve setup-base results for the SDK.
	instance := InstanceName(name, projectId)
	snapshot, err := sdkLayerName(sk)
	if err != nil {
		return err
	}
	config := map[string]string{
		// The SDK can be deduced from user.workshop.file together with
		// user.workshop.sdks, but we add a standalone option to make
		// it easier to find during Restore.
		"user.workshop.sdk": sk,
		// Mark the copy as an SDK layer to avoid ambiguity.
		"user.workshop.layer-type": "sdk",
	}
	// We could exclude user.workshop.file since Restore will override it,
	// but doing that requires additional LXD API calls, and there's no
	// harm in keeping it around.
	var exclude []string
	return s.copyInstance(conn, layerConn, instance, snapshot, false, config, exclude)
}

// Generates a random suffix for the SDK, to avoid clashing with SDK layers
// from other projects and workshops, while remaining withing the 63 characters
// allowed for LXD instance names.
func sdkLayerName(sk string) (string, error) {
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return sk + "-" + hex.EncodeToString(bytes), nil
}

func (s *Backend) Restore(ctx context.Context, name, sk string, file *workshop.File) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	f, err := yaml.Marshal(file)
	if err != nil {
		return err
	}

	conn, layerConn, err := s.layerClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	// Find the snapshot itself and any newer snapshots.
	snapshot, obstacles, err := s.layerNamesAfter(layerConn, projectId, name, sk)
	if err != nil {
		return err
	}
	// Remove the newer snapshots, as LXD would do. This is not necessary
	// to "restore the snapshot," i.e. replace the volume with a new one,
	// but we want to avoid having two snapshots for the same SDK and
	// workshop until we introduce a way to distinguish them.
	for _, layer := range obstacles {
		if err := s.deleteLayer(layerConn, layer); err != nil {
			return err
		}
	}

	instance := InstanceName(name, projectId)
	// Update the workshop definition, similar to LaunchOrRebuildWorkshop.
	config := map[string]string{workshop.ConfigWorkshopFile: string(f)}
	// Avoid copying the options which we added when taking the snapshot.
	exclude := []string{"user.workshop.sdk", "user.workshop.layer-type"}
	return s.copyInstance(layerConn, conn, snapshot, instance, true, config, exclude)
}

// Returns a clone of the parent context to be used in cleanup operations on
// errors or if the parent context was cancelled. The non-cancelled clone will
// be created even if the parent's context is cancelled.
func revctx(parent context.Context) context.Context {
	revctx := context.Background()
	user, ok := parent.Value(workshop.ContextUser).(string)
	if ok {
		revctx = context.WithValue(revctx, workshop.ContextUser, user)
	}
	pid, ok := parent.Value(workshop.ContextProjectId).(string)
	if ok {
		revctx = context.WithValue(revctx, workshop.ContextProjectId, pid)
	}
	return revctx
}

func (s *Backend) StashWorkshop(ctx context.Context, name string) error {
	rev := revert.New()
	defer rev.Fail()

	pid, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, layerConn, err := s.layerClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := s.stopWorkshop(conn, ctx, name, true); err != nil {
		return err
	}

	rev.Add(func() {
		revctx := revctx(ctx)
		revconn, rerr := s.LxdClient(revctx)
		if rerr != nil {
			logger.Debugf("On StashWorkshop: Cannot connect to LXD server: %v", rerr)
			return
		}
		defer revconn.Disconnect()
		rerr = s.startWorkshop(revconn, revctx, name)
		if rerr != nil {
			logger.Debugf("On StashWorkshop: Cannot restart %q workshop after failed stash operation: %v", name, rerr)
		}
	})

	layers, err := s.layerNames(layerConn, pid, name, "sdk")
	if err != nil {
		return err
	}

	// Backup the workshop's SDK layers, modifying the instance name and
	// layer type to distinguish the backups from the originals.
	for _, layer := range layers {
		newname := "stash-" + layer
		config := map[string]string{"user.workshop.layer-type": "stash-sdk"}
		if err := s.copyInstance(layerConn, layerConn, layer, newname, false, config, nil); err != nil {
			return err
		}
	}

	// Backup the workshop itself.
	instance := InstanceName(name, pid)
	stashed := instanceStashName(name, pid)
	// Mark the copy as a stash to avoid confusing it with an SDK layer.
	config := map[string]string{"user.workshop.layer-type": "stash"}
	if err := s.copyInstance(conn, layerConn, instance, stashed, false, config, nil); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (s *Backend) UnstashWorkshop(ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, layerConn, err := s.layerClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	layers, err := s.layerNames(layerConn, projectId, name, "sdk")
	if err != nil {
		return err
	}
	stashLayers, err := s.layerNames(layerConn, projectId, name, "stash-sdk")
	if err != nil {
		return err
	}

	// Remove the layers that were created after stashing. These are the
	// ones which don't have backups.
	for _, layer := range layers {
		if slices.Contains(stashLayers, "stash-"+layer) {
			continue
		}
		if err := s.deleteLayer(layerConn, layer); err != nil {
			return err
		}
	}

	// Restore the layers that were deleted when restoring an SDK snapshot.
	// These come from the backups whose source no longer exists.
	for _, layer := range stashLayers {
		name := strings.TrimPrefix(layer, "stash-")
		if slices.Contains(layers, name) {
			continue
		}
		// Restore the original layer type (stash-sdk -> sdk).
		config := map[string]string{"user.workshop.layer-type": "sdk"}
		if err := s.copyInstance(layerConn, layerConn, layer, name, false, config, nil); err != nil {
			return err
		}
	}

	// Restore the workshop itself.
	instance := InstanceName(name, projectId)
	stash := instanceStashName(name, projectId)
	// Avoid copying the option which we added when stashing.
	exclude := []string{"user.workshop.layer-type"}
	if err := s.copyInstance(layerConn, conn, stash, instance, true, nil, exclude); err != nil {
		return err
	}

	return s.startWorkshop(conn, ctx, name)
}

// Find layer names for all SDKs in the given workshop or stash.
func (s *Backend) layerNames(layerConn lxd.InstanceServer, pid, w string, kind string) ([]string, error) {
	filters := []string{
		"config.user.workshop.project-id=" + pid,
		"config.user.workshop.name=" + w,
		"config.user.workshop.layer-type=" + kind,
	}
	layers, err := layerConn.GetInstancesWithFilter(api.InstanceTypeContainer, filters)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(layers))
	for _, layer := range layers {
		names = append(names, layer.Name)
	}
	return names, nil
}

// Find layer name for the given SDK and any subsequent SDKs.
func (s *Backend) layerNamesAfter(layerConn lxd.InstanceServer, pid, w, sk string) (string, []string, error) {
	filters := []string{
		"config.user.workshop.project-id=" + pid,
		"config.user.workshop.name=" + w,
		"config.user.workshop.layer-type=sdk",
	}
	layers, err := layerConn.GetInstancesWithFilter(api.InstanceTypeContainer, filters)
	if err != nil {
		return "", nil, err
	}

	idx := slices.IndexFunc(layers, func(inst api.Instance) bool {
		return inst.Config["user.workshop.sdk"] == sk
	})
	if idx < 0 {
		return "", nil, fmt.Errorf("%q SDK snapshot not found in workshop %q", sk, w)
	}

	// Find the installed SDKs at the time of the snapshot.
	sdks := map[string]sdk.Setup{}
	buf, exist := layers[idx].Config[workshop.ConfigWorkshopSdks]
	if exist {
		if err := json.Unmarshal([]byte(buf), &sdks); err != nil {
			return "", nil, err
		}
	}

	// Any SDK which wasn't installed must have been added later.
	obstacles := make([]string, 0, len(layers))
	for _, layer := range layers {
		if _, ok := sdks[layer.Config["user.workshop.sdk"]]; !ok {
			obstacles = append(obstacles, layer.Name)
		}
	}

	return layers[idx].Name, obstacles, nil
}

// Copies an instance from the src server to the dst server. The servers can be
// identical, different, or the same underlying server for different projects.
// If the target instance already exists, set refresh=true to overwrite it.
// Otherwise, refresh should be false to avoid unnecessary API calls. In both
// cases the non-volatile config options, and a couple of volatile ones, are
// copied to the target instance. Additional options can be added or overridden
// using the config argument. To prevent an option from being copied over, add
// it to the exclude list. The boot.autostart option is always set to false in
// the copy: the source instance might be running but the copy won't be.
func (s *Backend) copyInstance(src, dst lxd.InstanceServer, srcName, dstName string, refresh bool, config map[string]string, exclude []string) error {
	if config == nil {
		config = map[string]string{}
	}
	config["boot.autostart"] = "false"

	srcInst, _, err := src.GetInstance(srcName)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return workshop.ErrWorkshopNotLaunched
		}
		return err
	}

	// Set the config and device overrides. LXD will copy most other
	// options from the source instance, but it will omit options which are
	// unique to the source instance, like MAC addresses and UUIDs.
	req := *srcInst
	req.Config = config
	req.Devices = nil

	args := lxd.InstanceCopyArgs{
		Name:         dstName,
		InstanceOnly: true,
		Refresh:      refresh,
	}

	rop, err := dst.CopyInstance(src, req, &args)
	if err != nil {
		return err
	}
	if err = rop.Wait(); err != nil {
		return err
	}

	if !refresh && len(exclude) == 0 {
		// LXD created a new instance with copied config and devices,
		// and we don't need to go back and remove any options.
		return nil
	}

	// We're in one of the following scenarios:
	// - LXD replaced an existing instance's root disk, but it's our job to
	//   copy the config and devices.
	// - LXD created a new instance with copied config and devices, but we
	//   didn't want it to copy some of the config options.
	dstInst, etag, err := dst.GetInstance(dstName)
	if err != nil {
		return err
	}

	// Remove all options that aren't managed by LXD. The ones we do want
	// to set are added below. Ideally, some LXD-managed options should
	// also be removed. See https://github.com/canonical/lxd/issues/16667.
	maps.DeleteFunc(dstInst.Config, func(k, v string) bool { return includeWhenCopying(k) })

	// Add all options from the source, except for those managed by LXD or
	// excluded by the caller. Then add config overrides.
	maps.DeleteFunc(srcInst.Config, func(k, v string) bool { return !includeWhenCopying(k) || slices.Contains(exclude, k) })
	if srcInst.Config == nil {
		srcInst.Config = config
	} else {
		maps.Copy(srcInst.Config, config)
	}
	if dstInst.Config == nil {
		dstInst.Config = srcInst.Config
	} else {
		maps.Copy(dstInst.Config, srcInst.Config)
	}

	dstInst.Devices = srcInst.Devices

	op, err := dst.UpdateInstance(dstName, dstInst.Writable(), etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

// Based on LXD's InstanceIncludeWhenCopying. These are the options which
// CopyInstance adds to a newly created instance by default (i.e. when
// api.Instance.Config is empty).
func includeWhenCopying(key string) bool {
	suffix, found := strings.CutPrefix(key, "volatile.")
	return !found || slices.Contains([]string{"base_image", "last_state.idmap"}, suffix)
}

func (s *Backend) RemoveWorkshopStash(ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	_, conn, err := s.layerClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	layers, err := s.layerNames(conn, projectId, name, "stash-sdk")
	if err == nil {
		for _, layer := range layers {
			err1 := s.deleteLayer(conn, layer)
			err = cmp.Or(err, err1)
		}
	}

	err1 := s.deleteLayer(conn, instanceStashName(name, projectId))
	return cmp.Or(err, err1)
}

func (s *Backend) deleteLayer(layerConn lxd.InstanceServer, layer string) error {
	op, err := layerConn.DeleteInstance(layer)
	if err == nil {
		err = op.Wait()
	}
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return nil
	}
	return err
}

func (s *Backend) layerClients(ctx context.Context) (lxd.InstanceServer, lxd.InstanceServer, error) {
	connctx := revctx(ctx)
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	project, err := lxdLayersProjectName(user)
	if err != nil {
		return nil, nil, err
	}

	conn, err := s.LxdClient(connctx)
	if err != nil {
		return nil, nil, err
	}

	return conn, conn.UseProject(project), nil
}
