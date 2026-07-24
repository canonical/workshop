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
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os/user"
	"slices"
	"strings"
	"sync"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/lxd/shared/entity"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

var (
	snapshotGuardsLock sync.Mutex
	snapshotGuards     = map[string]*snapshotGuard{}
)

type snapshotGuard struct {
	c       chan struct{}
	counter int32
}

func lockSnapshot(ctx context.Context, name string) error {
	snapshotGuardsLock.Lock()
	guard, ok := snapshotGuards[name]
	if !ok {
		guard = &snapshotGuard{}
		guard.counter = 0
		guard.c = make(chan struct{}, 1)
		snapshotGuards[name] = guard

		guard.c <- struct{}{}
	}
	guard.counter += 1
	snapshotGuardsLock.Unlock()

	select {
	case <-guard.c:
		return nil
	case <-ctx.Done():
		snapshotGuardsLock.Lock()
		guard.counter -= 1
		if guard.counter == 0 {
			close(guard.c)
			delete(snapshotGuards, name)
		}
		snapshotGuardsLock.Unlock()
		return ctx.Err()
	}
}

func unlockSnapshot(name string) {
	snapshotGuardsLock.Lock()
	defer snapshotGuardsLock.Unlock()

	guard, ok := snapshotGuards[name]
	if !ok {
		panic(fmt.Errorf("%q snapshot is not locked", name))
	}
	guard.c <- struct{}{}

	guard.counter -= 1
	if guard.counter == 0 {
		close(guard.c)
		delete(snapshotGuards, name)
	}
}

// Classifies LXD config options into different domains which describe who is
// responsible for them and how they (should) behave when copying an instance.
type configDomain int

const (
	// Properties unique to a single instance. These are managed by LXD and
	// not shared with instance copies, e.g. volatile.uuid.
	uniqueProperty configDomain = iota
	// Properties managed by LXD and shared with copies, e.g. image.os.
	sharedProperty
	// Options managed by Workshop. Includes instance config options like
	// boot.autostart and workshop metadata like user.workshop.project-id.
	customOption
)

// Classifies options by key. Based on LXD's InstanceIncludeWhenCopying.
func optionDomain(key string) configDomain {
	switch {
	case strings.HasPrefix(key, "user.ed25519-key."):
		return uniqueProperty
	case strings.HasPrefix(key, "image."):
		return sharedProperty
	case !strings.HasPrefix(key, "volatile."):
		return customOption
	case slices.Contains(api.InstanceRemoteCopyConfigKeyPolicy.Immutable, key):
		return sharedProperty
	default:
		return uniqueProperty
	}
}

func (s *Backend) Snapshot(ctx context.Context, snapshot workshop.Snapshot) (*workshop.SnapshotInfo, error) {
	if snapshot.IsBase() {
		return nil, errors.New("internal error: snapshots require at least one SDK")
	}

	conn, snapshotConn, err := s.snapshotClients(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	digest, err := s.HashSnapshot(snapshot)
	if err != nil {
		return nil, fmt.Errorf("internal error: hashing snapshot info: %w", err)
	}

	name := sdkSnapshotName(snapshot, digest)
	inst, _, err := snapshotConn.GetInstance(name)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return nil, workshop.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}

	// Check for in-progress snapshots. In this case we could wait for it to
	// finish and return true, but this optimization would be better suited for
	// the changes and tasks system; i.e. all concurrent launches and refreshes
	// should coordinate their snapshots, not just those which happen to call
	// HasSnapshot and TakeSnapshot at the same time.
	if inst.Config[workshop.ConfigWorkshopSnapshotType] != "sdk" {
		return nil, workshop.ErrSnapshotNotFound
	}

	other, err := detectHashCollision(inst, snapshot)
	if err != nil {
		if other == nil {
			return nil, err
		}
		return &workshop.SnapshotInfo{Snapshot: *other}, err
	}

	workshops := map[string][]string{}

	usedBy, err := conn.GetInstances(lxd.GetInstancesArgs{
		InstanceType: api.InstanceTypeContainer,
		Filters:      []string{fmt.Sprintf("config.user.workshop.snapshot-%v=%s", len(snapshot.Sdks), name)},
	})
	if err != nil {
		return nil, err
	}
	for _, inst := range usedBy {
		projectId := inst.Config[workshop.ConfigProjectId]
		name := inst.Config[workshop.ConfigWorkshopName]
		workshops[projectId] = append(workshops[projectId], name)
	}

	// Check for stashed workshops as well.
	usedBy, err = snapshotConn.GetInstances(lxd.GetInstancesArgs{
		InstanceType: api.InstanceTypeContainer,
		Filters:      []string{fmt.Sprintf("config.user.workshop.snapshot-%v=%s", len(snapshot.Sdks), name)},
	})
	if err != nil {
		return nil, err
	}
	for _, inst := range usedBy {
		projectId := inst.Config[workshop.ConfigProjectId]
		name := inst.Config[workshop.ConfigWorkshopName]
		if !slices.Contains(workshops[projectId], name) {
			workshops[projectId] = append(workshops[projectId], name)
		}
	}

	return &workshop.SnapshotInfo{Snapshot: snapshot, Workshops: workshops}, nil
}

func detectHashCollision(inst *api.Instance, snapshot workshop.Snapshot) (*workshop.Snapshot, error) {
	saved, err := identifySnapshot(inst)
	if err != nil {
		return nil, err
	}
	if err := compareSnapshots(inst.Name, *saved, snapshot); err != nil {
		return saved, fmt.Errorf("hash collision detected: %w", err)
	}
	return saved, nil
}

func identifySnapshot(inst *api.Instance) (*workshop.Snapshot, error) {
	format, err := sdk.ParseRevision(inst.Config[workshop.ConfigWorkshopSnapshotFormat])
	if err != nil {
		return nil, err
	}

	sdks := make([]sdk.ContentID, len(inst.Devices))
	length := 0
	maxInstallOrder := 0
	for key, device := range inst.Devices {
		installOrder, s, err := maybeSdkId(key, device)
		if err != nil {
			return nil, fmt.Errorf("invalid %q snapshot: %w", inst.Name, err)
		}
		if s == nil {
			continue
		}

		if installOrder <= 0 || len(sdks) < installOrder {
			return nil, fmt.Errorf("invalid %q snapshot: install-order for %q SDK out of bounds", inst.Name, s.Name)
		}
		if sdks[installOrder-1].Name != "" {
			return nil, fmt.Errorf("invalid %q snapshot: %q and %q SDKs have same install-order", inst.Name, sdks[installOrder-1].Name, s.Name)
		}
		sdks[installOrder-1] = *s
		length += 1
		maxInstallOrder = max(maxInstallOrder, installOrder)
	}

	if maxInstallOrder > length {
		return nil, fmt.Errorf("invalid %q snapshot: install-order for %q SDK out of bounds", inst.Name, sdks[maxInstallOrder-1].Name)
	}

	return &workshop.Snapshot{
		Format: format,
		Image: workshop.BaseImage{
			Name:        inst.Config[workshop.ConfigWorkshopBase],
			Fingerprint: inst.Config[workshop.ConfigWorkshopBaseFingerprint],
		},
		Sdks: sdks[:length],
	}, nil
}

// compareSnapshots is like reflect.DeepEqual with specific error messages.
func compareSnapshots(name string, actual, expected workshop.Snapshot) error {
	if actual.Format != expected.Format {
		return fmt.Errorf("%q snapshot has format revision %s; required: %s", name, actual.Format, expected.Format)
	}
	if actual.Image.Name != expected.Image.Name {
		return fmt.Errorf("%q snapshot has %q base; required: %q", name, actual.Image.Name, expected.Image.Name)
	}
	if actual.Image.Fingerprint != expected.Image.Fingerprint {
		return fmt.Errorf("%q snapshot has %q base fingerprint; required: %q", name, actual.Image.Fingerprint, expected.Image.Fingerprint)
	}

	for i, sk := range expected.Sdks {
		if len(actual.Sdks) <= i {
			return fmt.Errorf("%q snapshot is missing %q SDK", name, sk.Name)
		}
		if actual.Sdks[i].Name != sk.Name {
			return fmt.Errorf("%q snapshot has %q SDK; required: %q", name, actual.Sdks[i].Name, sk.Name)
		}
		if actual.Sdks[i] != sk {
			return fmt.Errorf("%q snapshot has unexpected revision of %q SDK", name, sk.Name)
		}
	}
	if len(actual.Sdks) > len(expected.Sdks) {
		return fmt.Errorf("%q snapshot has unexpected %q SDK", name, actual.Sdks[len(expected.Sdks)].Name)
	}

	return nil
}

func (s *Backend) TakeSnapshot(ctx context.Context, name string, snapshot workshop.Snapshot) error {
	if snapshot.IsBase() {
		return errors.New("internal error: snapshots require at least one SDK")
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	digest, err := s.HashSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("internal error: hashing snapshot info: %w", err)
	}
	snapshotName := sdkSnapshotName(snapshot, digest)

	// Disable cancellation, because the LXD operation will plow on regardless,
	// and the lock is supposed to prevent concurrent import operations.
	lockedCtx := context.WithoutCancel(ctx)
	conn, snapshotConn, err := s.snapshotClients(lockedCtx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	inst, etag, err := conn.GetInstance(InstanceName(name, projectId))
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return workshop.ErrWorkshopNotLaunched
		}
		return err
	}

	// Mark the snapshot as "in use" by this workshop. This can be done when
	// installing the SDK to avoid an extra UpdateInstance, but is easier to do
	// here since we already know the snapshot name.
	inst.Config[fmt.Sprintf("user.workshop.snapshot-%v", len(snapshot.Sdks))] = snapshotName
	op, err := conn.UpdateInstance(inst.Name, inst.Writable(), etag)
	if err != nil {
		return err
	}
	if err := op.Wait(); err != nil {
		return err
	}

	if inst.Config == nil {
		inst.Config = map[string]string{}
	}
	config := map[string]string{
		"security.protection.start":            "true",
		workshop.ConfigWorkshopSnapshotFormat:  snapshot.Format.String(),
		workshop.ConfigWorkshopBase:            snapshot.Image.Name,
		workshop.ConfigWorkshopBaseFingerprint: snapshot.Image.Fingerprint,
		workshop.ConfigWorkshopSha3_384:        digest,
	}
	mergeConfig(inst.Config, nil, config)

	if inst.Devices == nil {
		inst.Devices = map[string]map[string]string{}
	}
	if err := mergeDevices(inst.Devices, snapshot.Sdks, name); err != nil {
		return err
	}

	// Reset everything that isn't explicitly specified.
	inst.SetWritable(api.InstancePut{
		Architecture: inst.Architecture,
		Config:       inst.Config,
		Devices:      inst.Devices,
		Profiles:     []string{},
	})
	args := lxd.InstanceCopyArgs{Name: snapshotName, InstanceOnly: true}

	// LXD already prevents concurrent instance copies, but without a lock it's
	// hard to tell if a conflict is caused by a concurrent copy or an aborted
	// snapshot. After locking, any conflict we observe must be due to Workshop
	// or LXD dying abruptly.
	if err := lockSnapshot(ctx, snapshotName); err != nil {
		return err
	}
	defer unlockSnapshot(snapshotName)

	rev := revert.New()
	defer rev.Fail()

	for i := range 2 {
		rop, err := snapshotConn.CopyInstance(conn, *inst, &args)
		if err == nil {
			err = rop.Wait()
		}
		if err == nil {
			break
		}

		if i > 0 || !IsInstanceConflict(err, snapshotName) {
			return err
		}
		if err := s.resolveSnapshotConflict(snapshotConn, snapshot, snapshotName); err != nil {
			return err
		}
	}
	rev.Add(func() {
		// Once a snapshot is complete, it can't be deleted safely, so we check
		// that case first before cleaning up.
		if reverr := s.checkPartialSnapshot(snapshotConn, snapshot, snapshotName); reverr != nil {
			logger.Noticef("On TakeSnapshot: %v", reverr)
		}
		if reverr := s.deleteSnapshot(snapshotConn, snapshotName); reverr != nil {
			logger.Noticef("On TakeSnapshot: %v", reverr)
		}
	})

	if err := s.commitPartialSnapshot(snapshotConn, snapshotName); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func IsInstanceConflict(err error, name string) bool {
	if err == nil {
		return false
	}
	if api.StatusErrorCheck(err, http.StatusConflict) {
		return true
	}

	suffixes := []string{
		fmt.Sprintf(": Instance %q already exists", name),
		`: Instance is busy running a "create" operation`,
	}
	return slices.ContainsFunc(suffixes, func(s string) bool {
		return strings.HasSuffix(err.Error(), s)
	})
}

// resolveSnapshotConflict returns ErrSnapshotAlreadyExists (or a hash
// collision error) if a complete snapshot already exists. Otherwise, it waits
// for outstanding operations on the snapshot and checks if it's complete
// again. If not, the partial snapshot is deleted.
func (s *Backend) resolveSnapshotConflict(snapshotConn lxd.InstanceServer, snapshot workshop.Snapshot, name string) error {
	if err := s.checkPartialSnapshot(snapshotConn, snapshot, name); err != nil {
		return err
	}

	info, err := snapshotConn.GetConnectionInfo()
	if err != nil {
		return err
	}

	ops, err := snapshotConn.GetOperations()
	if err != nil {
		return err
	}
	for _, op := range ops {
		isInstance, err := IsInstanceOperation(op, info.Project, name)
		if err != nil {
			return err
		}
		if !isInstance {
			continue
		}

		if _, _, err := snapshotConn.GetOperationWait(op.ID, -1); err != nil && !api.StatusErrorCheck(err) {
			return err
		}
	}

	if err := s.checkPartialSnapshot(snapshotConn, snapshot, name); err != nil {
		return err
	}

	return s.deleteSnapshot(snapshotConn, name)
}

func IsInstanceOperation(op api.Operation, lxdProject, name string) (bool, error) {
	entityUrl, ok := op.Metadata[api.MetadataEntityURL].(string)
	if !ok {
		return false, nil
	}

	u, err := url.Parse(entityUrl)
	if err != nil {
		return false, err
	}
	entityType, project, _, args, err := entity.ParseURL(*u)
	if err != nil {
		return false, err
	}

	matches := entityType == entity.TypeInstance && project == lxdProject && slices.Equal(args, []string{name})
	return matches, nil
}

func (s *Backend) checkPartialSnapshot(snapshotConn lxd.InstanceServer, snapshot workshop.Snapshot, name string) error {
	inst, _, err := snapshotConn.GetInstance(name)
	if api.StatusErrorCheck(err, http.StatusNotFound) || (err == nil && inst.Config[workshop.ConfigWorkshopSnapshotType] != "sdk") {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := detectHashCollision(inst, snapshot); err != nil {
		return err
	}
	return workshop.ErrSnapshotAlreadyExists
}

func (s *Backend) commitPartialSnapshot(snapshotConn lxd.InstanceServer, name string) error {
	copy, etag, err := snapshotConn.GetInstance(name)
	if err != nil {
		return err
	}
	copy.Config[workshop.ConfigWorkshopSnapshotType] = "sdk"
	op, err := snapshotConn.UpdateInstance(name, copy.Writable(), etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (s *Backend) launchOrRebuildFromSnapshot(conn, snapshotConn lxd.InstanceServer, usr *user.User, req api.InstancesPost, snapshot workshop.Snapshot) error {
	digest, err := s.HashSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("internal error: hashing snapshot info: %w", err)
	}

	source, _, err := snapshotConn.GetInstance(sdkSnapshotName(snapshot, digest))
	if err != nil {
		return err
	}

	inst, _, err := conn.GetInstance(req.Name)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		config, err := sshConfig(usr, req.Name+"."+networkDomain)
		if err != nil {
			return err
		}

		inst = &api.Instance{Config: config}
	} else if err != nil {
		return err
	}

	if req.Config == nil {
		req.Config = make(map[string]string, len(snapshot.Sdks))
	} else {
		req.Config = maps.Clone(req.Config)
	}
	for i := range snapshot.Sdks {
		partial := snapshot
		partial.Sdks = partial.Sdks[:i+1]

		digest, err := s.HashSnapshot(partial)
		if err != nil {
			return err
		}
		req.Config[fmt.Sprintf("user.workshop.snapshot-%v", i+1)] = sdkSnapshotName(partial, digest)
	}

	if source.Config == nil {
		source.Config = map[string]string{}
	}
	mergeConfig(source.Config, inst.Config, req.Config)

	req.Architecture = source.Architecture
	req.Config = source.Config
	// Ensure LXD doesn't copy profiles from the snapshot.
	if req.Profiles == nil {
		req.Profiles = []string{}
	}
	source.SetWritable(req.InstancePut)

	args := lxd.InstanceCopyArgs{
		Name:         req.Name,
		InstanceOnly: true,
		Refresh:      true,
	}
	rop, err := conn.CopyInstance(snapshotConn, *source, &args)
	if err != nil {
		return err
	}
	if err := rop.Wait(); err != nil {
		return err
	}

	return nil
}

// Hash is truncated to 16 hex digits to remain within the 63 characters
// allowed for LXD instance names.
func sdkSnapshotName(snapshot workshop.Snapshot, digest string) string {
	sk := snapshot.Sdks[len(snapshot.Sdks)-1].Name
	return sk + "-" + digest[:16]
}

func mergeConfig(source, target, config map[string]string) {
	maps.DeleteFunc(source, func(k, v string) bool {
		return optionDomain(k) != sharedProperty
	})

	for k, v := range target {
		if optionDomain(k) == uniqueProperty {
			source[k] = v
		}
	}

	maps.Copy(source, config)
}

func mergeDevices(source map[string]map[string]string, sdks []sdk.ContentID, w string) error {
	maps.DeleteFunc(source, func(k string, v map[string]string) bool {
		return k != "root"
	})

	if storagePoolDriver == "zfs" {
		root := maps.Clone(source["root"])
		if source == nil || root == nil {
			return fmt.Errorf("internal error: %q workshop has no rootfs", w)
		}
		source["root"] = root
		root["initial.zfs.promote"] = "true"
	}

	for i, sk := range sdks {
		source[workshop.SdkDeviceName(sk.Name)] = sdkToSnapshotDevice(i+1, sk)
	}

	return nil
}

func (s *Backend) RemoveSnapshot(ctx context.Context, snapshot workshop.Snapshot) error {
	if snapshot.IsBase() {
		return errors.New("internal error: snapshots require at least one SDK")
	}

	conn, snapshotConn, err := s.snapshotClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	digest, err := s.HashSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("internal error: hashing snapshot info: %w", err)
	}

	// For completeness we should probably check for collisions here. But if
	// the given snapshot is being removed, it was recently part of a workshop
	// and we already checked for collisions on launch.
	return s.deleteSnapshot(snapshotConn, sdkSnapshotName(snapshot, digest))
}

func (s *Backend) StashWorkshop(ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, snapshotConn, err := s.snapshotClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	instance := InstanceName(name, projectId)
	stashed := instanceStashName(name, projectId)
	// Mark the copy as a stash to avoid confusing it with an SDK snapshot.
	config := map[string]string{workshop.ConfigWorkshopSnapshotType: "stash"}
	if err := s.copyInstance(conn, snapshotConn, instance, stashed, false, config); err != nil {
		return err
	}

	return nil
}

func (s *Backend) UnstashWorkshop(ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn, snapshotConn, err := s.snapshotClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	instance := InstanceName(name, projectId)
	stash := instanceStashName(name, projectId)
	// Avoid restoring the option which we added when stashing.
	config := map[string]string{workshop.ConfigWorkshopSnapshotType: ""}
	return s.copyInstance(snapshotConn, conn, stash, instance, true, config)
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
func (s *Backend) copyInstance(src, dst lxd.InstanceServer, srcName, dstName string, refresh bool, config map[string]string) error {
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

	dstInst, _, err := dst.GetInstance(dstName)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		dstInst = &api.Instance{}
	} else if err != nil {
		return err
	}

	if srcInst.Config == nil {
		srcInst.Config = map[string]string{}
	}
	maps.DeleteFunc(srcInst.Config, func(k, v string) bool {
		return optionDomain(k) == uniqueProperty
	})
	for k, v := range dstInst.Config {
		if optionDomain(k) == uniqueProperty {
			srcInst.Config[k] = v
		}
	}
	for k, v := range config {
		if v == "" {
			delete(srcInst.Config, k)
		} else {
			srcInst.Config[k] = v
		}
	}

	req := *srcInst

	if storagePoolDriver == "zfs" {
		req.Devices = maps.Clone(req.Devices)
		root := maps.Clone(req.Devices["root"])
		if req.Devices == nil || root == nil {
			return fmt.Errorf("internal error: %q has no rootfs", srcName)
		}
		req.Devices["root"] = root
		root["initial.zfs.promote"] = "true"
	}

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

	return nil
}

func (s *Backend) RemoveWorkshopStash(ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	_, conn, err := s.snapshotClients(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	err1 := s.deleteSnapshot(conn, instanceStashName(name, projectId))
	return cmp.Or(err, err1)
}

func (s *Backend) deleteSnapshot(snapshotConn lxd.InstanceServer, snapshot string) error {
	op, err := snapshotConn.DeleteInstance(snapshot, false)
	if err == nil {
		err = op.Wait()
	}
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return nil
	}
	return err
}

func (s *Backend) snapshotClients(ctx context.Context) (lxd.InstanceServer, lxd.InstanceServer, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	project, err := lxdSnapshotsProjectName(user)
	if err != nil {
		return nil, nil, err
	}

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	return conn, conn.UseProject(project), nil
}

// Hard coded number which represents the current "snapshot format," i.e. the
// contents of the rootfs after installing a certain sequence of SDKs. This can
// be influenced by a number of factors, including:
// - Default workshop config (e.g. cloud-config)
// - Default workshop devices (e.g. apt cache)
// - SDK config and devices (e.g. volume mounts)
// - Direct modifications (e.g. mkdir /var/lib/workshop/run)
// If something like this changes, bump the revision number so that workshops
// constructed using the next release of Workshop are updated on refresh.
//
// Some care needs to be taken with workshops using the old format. If a launch
// or refresh is in progress, it should either continue in a manner consistent
// with the old format, or fail if that isn't possible for some reason. For
// example, after changing the format of installed SDKs, the install-sdk task
// should either use a format consistent with the target workshop, or fail for
// workshops stuck on the old format.
//
// Another issue is the restore command. In most cases this simply rebuilds
// from the last snapshot, but if that snapshot doesn't exist then it will
// replay some of the install-sdk and setup-base tasks. These can be handled in
// the same way as in-progress launches and refreshes.
func (s *Backend) FormatRevision() sdk.Revision {
	return sdk.R(11)
}

func (s *Backend) HashSnapshot(snapshot workshop.Snapshot) (string, error) {
	digest, err := hex.DecodeString(snapshot.Image.Fingerprint)
	if err != nil {
		return "", err
	}

	hash := sha3.New384()
	if _, err := fmt.Fprintf(hash, "%s %s\x00%s", snapshot.Format, snapshot.Image.Name, digest); err != nil {
		return "", err
	}

	for _, sk := range snapshot.Sdks {
		// Different types of SDKs result in different workshops. There
		// are really only two types: those which use SDK volumes and
		// those which use local directories.
		kind := "local"
		if sk.IsVolume {
			kind = "volume"
		}

		digest, err := hex.DecodeString(sk.Sha3_384)
		if err != nil {
			return "", err
		}

		if _, err := fmt.Fprintf(hash, "%s %s\x00%s", kind, sk.Name, digest); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
