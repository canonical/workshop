package lxdbackend

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/workshop"
)

var volumeIndex = `name: %s
backend: %s
type: custom
config:
  volume:
    name: %s
    description: "SDK Volume"
    type: custom
    content_type: filesystem
`

func volumeIndexContent(name string) string {
	return fmt.Sprintf(volumeIndex, name, storagePool, name)
}

func lockVolume(volume string) chan struct{} {
	volumes.Lock()
	vchan, ok := volumeLocks[volume]
	if !ok {
		vchan = make(chan struct{}, 1)
		vchan <- struct{}{}
		volumeLocks[volume] = vchan
	}
	volumes.Unlock()
	return vchan
}

func unlockVolume(volume string) {
	volumes.Lock()
	vchan, ok := volumeLocks[volume]
	if ok {
		close(vchan)
		delete(volumeLocks, volume)
	}
	volumes.Unlock()
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

func (s *Backend) ImportVolume(ctx context.Context, name string, tarball string) error {
	// There could be multiple launches that require the same volume. We don't
	// want to unpack and import the volume multiple times.
	vchan := lockVolume(name)
	defer unlockVolume(name)

	select {
	case <-vchan:
	case <-ctx.Done():
		return ctx.Err()
	}

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	_, _, err = conn.GetStoragePoolVolume(storagePool, "custom", name)
	if err == nil {
		return workshop.ErrVolumeAlreadyExists
	}
	if !api.StatusErrorCheck(err, http.StatusNotFound) {
		return err
	}

	// The tarballs will be transformed into a LXD-compatible backup format to
	// create them directly as a custom volume. The LXD's tar archive has the
	// following format:
	//
	// backup/
	//	volume/
	//  index.yaml

	dir, err := os.MkdirTemp("", name)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	unpack := exec.CommandContext(ctx, "tar",
		"--extract",
		"--file="+tarball,
		"--transform",
		"s,^,volume/,",
		"--directory="+dir,
	)

	if _, err := unpack.Output(); err != nil {
		logger.Debugf("Failed to unpack volume tarball: %v", err)
		return err
	}

	// Generate index.yaml for the volume.
	if err = os.WriteFile(filepath.Join(dir, "index.yaml"), []byte(volumeIndexContent(name)), 0644); err != nil {
		return err
	}

	newtar := filepath.Join(dir, filepath.Base(tarball))

	// Read the metadata to store it as a volume's property. This is not ideal
	// when the backend knows the name of the file with metadata as the volume
	// manager should be able to import any tarball as a volume. But given that
	// it is only applicable to SDKs in the nearest future, it should be acceptable
	// as the alternative would be to change the interface to accept the metadata.
	meta, err := os.ReadFile(filepath.Join(dir, "volume", "meta", "sdk.yaml"))
	if err != nil {
		return err
	}

	repack := exec.CommandContext(ctx, "tar",
		"--remove-files",
		"--create",
		"--file",
		newtar,
		"--transform",
		"s,^,backup/,",
		"--directory="+dir,
		"--no-same-owner",
		"volume/",
		"index.yaml",
	)

	if _, err := repack.Output(); err != nil {
		logger.Debugf("Failed to repack volume tarball: %v", err)
		return err
	}

	f, err := os.Open(newtar)
	if err != nil {
		return err
	}
	defer f.Close()

	vol := lxd.StoragePoolVolumeBackupArgs{
		BackupFile: f,
		Name:       name,
	}

	op, err := conn.CreateStoragePoolVolumeFromBackup(storagePool, vol)
	if err != nil {
		return err
	}

	if err = op.WaitContext(ctx); err != nil {
		return err
	}

	return conn.UpdateStoragePoolVolume(storagePool, "custom", name, api.StorageVolumePut{
		Config: map[string]string{
			workshop.ConfigVolumeMeta: string(meta),
		}}, "")
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

func (s *Backend) Volume(ctx context.Context, name string) (workshop.VolumeInfo, error) {
	var info workshop.VolumeInfo
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return info, err
	}
	defer conn.Disconnect()

	vol, _, err := conn.GetStoragePoolVolume(storagePool, "custom", name)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return info, workshop.ErrVolumeNotFound
	}

	if err != nil {
		return info, err
	}

	info.Name = vol.Name
	info.Config = vol.Config
	return info, nil
}
