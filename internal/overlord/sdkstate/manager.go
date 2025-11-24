package sdkstate

import (
	"context"
	"slices"
	"time"

	"github.com/canonical/workshop/internal/interfaces"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type SdkVolume struct {
	Name      string     `json:"name"`
	Version   string     `json:"version,omitempty"`
	Revision  string     `json:"revision"`
	BuildTime *time.Time `json:"build-time,omitempty"`
	Size      uint64     `json:"size,omitempty"`
}

type SdkInstalled struct {
	ProjectPath string `json:"project-path"`
	Workshop    string `json:"workshop"`
	Channel     string `json:"channel,omitempty"`
	SdkVolume
}

// This struct maintains information merged from
// the local volumes infos and the store.
// TODO: obtain Description and Summary from the store.
type SdkFullInfo struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Installed   []SdkInstalled `json:"installed,omitempty"`
}

type SdkManager struct {
	backend workshop.Backend
	repo    *interfaces.Repository
}

var sdkVolumeCooldownTime = 1 * time.Hour // Time to wait before deleting unused SDK volumes.

func New(s *state.State, runner *state.TaskRunner, repo *interfaces.Repository) *SdkManager {
	manager := &SdkManager{repo: repo}

	s.Lock()
	manager.backend = workshop.WorkshopBackend(s)
	s.Unlock()

	runner.AddHandler("retrieve-sdk", OnDo(manager.doRetrieveSdk), nil)
	runner.AddHandler("install-sdk", OnDo(manager.doInstallSdk), OnUndo(manager.doUninstallSdk))
	runner.AddHandler("register-sdk", OnDo(manager.doRegisterSdk), OnUndo(manager.doUnregisterSdk))
	runner.AddHandler("unregister-sdk", OnDo(manager.doUnregisterSdk), OnUndo(manager.doRegisterSdk))
	runner.AddHandler("snapshot-sdk", OnDo(manager.doSnapshotSdk), nil)

	runner.AddCleanup("unregister-sdk", manager.doDeleteUnusedSdkVolumes)
	runner.AddCleanup("install-sdk", manager.doDeleteUnusedSdkVolumes)

	return manager
}

func (w *SdkManager) SdkVolumes(ctx context.Context) ([]SdkVolume, error) {
	volumes, err := w.backend.Volumes(ctx, "sdk")
	if err != nil {
		return nil, err
	}

	entries := make([]SdkVolume, 0, len(volumes))
	for _, vol := range volumes {
		info, err := sdk.ReadSdkInfo([]byte(vol.Metadata), "", "")
		if err != nil {
			return nil, err
		}

		entries = append(entries, SdkVolume{
			Name:      info.Name,
			Version:   info.Version,
			Revision:  vol.Revision.String(),
			BuildTime: info.BuildTime,
			Size:      vol.Size,
		})
	}
	return entries, nil
}

func (w *SdkManager) Sdk(ctx context.Context, name string) (*SdkFullInfo, error) {
	volumes, err := w.backend.Volumes(ctx, "sdk")
	if err != nil {
		return nil, err
	}

	volumes = slices.DeleteFunc(volumes, func(a workshop.VolumeInfo) bool { return a.Sdk != name })
	if len(volumes) == 0 {
		return nil, workshop.ErrVolumeNotFound
	}

	full := SdkFullInfo{
		Name:      name,
		Installed: make([]SdkInstalled, 0, len(volumes)),
	}
	for _, vol := range volumes {
		info, err := sdk.ReadSdkInfo([]byte(vol.Metadata), "", "")
		if err != nil {
			return nil, err
		}

		// TODO: obtain summary, description, license from the actual store
		// when it becomes available.
		if full.Summary == "" {
			full.Summary = info.Summary
		}
		if full.Description == "" {
			full.Description = info.Description
		}

		for pid, wps := range vol.Workshops {
			pctx := context.WithValue(ctx, workshop.ContextProjectId, pid)
			for _, wp := range wps {
				winfo, err := w.backend.Workshop(pctx, wp)
				if err != nil {
					return nil, err
				}

				channel := ""
				sk, ok := winfo.Sdks[name]
				if ok {
					channel = sk.Channel
				}

				full.Installed = append(full.Installed, SdkInstalled{
					SdkVolume: SdkVolume{
						Name:      info.Name,
						Version:   info.Version,
						Revision:  vol.Revision.String(),
						BuildTime: info.BuildTime,
						Size:      vol.Size,
					},
					Workshop:    winfo.Name,
					ProjectPath: winfo.Project.Path,
					Channel:     channel,
				})
			}
		}
	}

	return &full, nil
}

func (w *SdkManager) Ensure() error {
	return nil
}

func FakeSdkVolumeCooldownTime(t time.Duration) (restore func()) {
	old := sdkVolumeCooldownTime
	sdkVolumeCooldownTime = t
	return func() {
		sdkVolumeCooldownTime = old
	}
}
