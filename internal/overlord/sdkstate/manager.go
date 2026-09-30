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

package sdkstate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/interfaces"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdkstore"
	"github.com/canonical/workshop/internal/timeutil"
	"github.com/canonical/workshop/internal/workshop"
)

type StoreAccount struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display-name"`
	Validation  string `json:"validation,omitempty"`
}

type SdkRevision struct {
	Channel      string     `json:"channel"`
	Track        string     `json:"track"`
	Risk         string     `json:"risk"`
	Revision     string     `json:"revision"`
	BuiltAt      *time.Time `json:"built-at,omitempty"`
	UploadedAt   *time.Time `json:"uploaded-at,omitempty"`
	ReleasedAt   *time.Time `json:"released-at,omitempty"`
	Version      string     `json:"version,omitempty"`
	Base         string     `json:"base,omitempty"`
	Arch         string     `json:"arch,omitempty"`
	DownloadSize uint64     `json:"download-size,omitzero"`
}

type SdkVolume struct {
	Name     string     `json:"name"`
	Version  string     `json:"version,omitempty"`
	Revision string     `json:"revision"`
	BuiltAt  *time.Time `json:"built-at,omitempty"`
	Size     uint64     `json:"size,omitempty"`
}

type SdkInstalled struct {
	ProjectPath string `json:"project-path"`
	Workshop    string `json:"workshop"`
	Channel     string `json:"channel,omitempty"`
	Base        string `json:"base,omitempty"`
	Arch        string `json:"architecture,omitempty"`
	SdkVolume
}

// This struct maintains information merged from
// the local volumes infos and the store.
type SdkFullInfo struct {
	Name        string         `json:"name"`
	PackageID   string         `json:"package-id,omitempty"`
	Title       string         `json:"title,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	License     string         `json:"license,omitempty"`
	Website     string         `json:"website,omitempty"`
	Publisher   *StoreAccount  `json:"publisher,omitempty"`
	Channels    []SdkRevision  `json:"channels,omitempty"`
	Installed   []SdkInstalled `json:"installed,omitempty"`
}

type SdkSummary struct {
	Name        string        `json:"name"`
	PackageID   string        `json:"package-id,omitempty"`
	Summary     string        `json:"summary,omitempty"`
	Description string        `json:"description,omitempty"`
	License     string        `json:"license,omitempty"`
	Publisher   *StoreAccount `json:"publisher,omitempty"`
	Channel     string        `json:"channel"`
	Track       string        `json:"track"`
	Risk        string        `json:"risk"`
	Revision    string        `json:"revision"`
	ReleasedAt  *time.Time    `json:"released-at,omitempty"`
	Version     string        `json:"version,omitempty"`
	Base        string        `json:"base,omitempty"`
	Arch        string        `json:"arch,omitempty"`
}

type SdkManager struct {
	state   *state.State
	store   sdk.Store
	backend workshop.Backend
	repo    *interfaces.Repository
}

var (
	sdkVolumeCooldownTime = 1 * time.Hour // Time to wait before deleting unused SDK volumes.

	timeNow = time.Now
)

func New(s *state.State, runner *state.TaskRunner, repo *interfaces.Repository) *SdkManager {
	manager := &SdkManager{state: s, repo: repo}

	runner.AddHandler("retrieve-sdk", OnDo(manager.doRetrieveSdk), nil)
	runner.AddHandler("install-sdk", OnDo(manager.doInstallSdk), OnUndo(manager.doUninstallSdk))
	runner.AddHandler("uninstall-sdk", OnDo(manager.doUninstallSdk), OnUndo(manager.doInstallSdk))
	runner.AddHandler("snapshot-sdk", OnDo(manager.doSnapshotSdk), nil)

	runner.AddCleanup("install-sdk", manager.doDeleteUnusedSdkVolumes)
	runner.AddCleanup("uninstall-sdk", manager.doDeleteUnusedSdkVolumes)

	return manager
}

func (w *SdkManager) StartUp() error {
	w.state.Lock()
	w.store = sdk.StoreService(w.state)
	w.backend = workshop.WorkshopBackend(w.state)
	w.state.Unlock()
	return nil
}

func (w *SdkManager) SdkVolumes(ctx context.Context) ([]SdkVolume, error) {
	sdks, err := w.backend.Sdks(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]SdkVolume, 0, len(sdks))
	for _, s := range sdks {
		var file sdk.File
		if err := yaml.Unmarshal([]byte(s.SdkYAML), &file); err != nil {
			return nil, err
		}

		entries = append(entries, SdkVolume{
			Name:     s.Name,
			Version:  file.Version,
			Revision: s.Revision.String(),
			BuiltAt:  file.BuiltAt,
			Size:     s.Size,
		})
	}
	return entries, nil
}

func (w *SdkManager) FindSdks(ctx context.Context, query string) ([]SdkSummary, error) {
	response, err := w.store.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	result := make([]SdkSummary, 0, len(response))
	for _, entry := range response {
		var publisher *StoreAccount
		if entry.Metadata.Publisher.ID != "" {
			publisher = &StoreAccount{
				ID:          entry.Metadata.Publisher.ID,
				Username:    entry.Metadata.Publisher.Username,
				DisplayName: entry.Metadata.Publisher.DisplayName,
				Validation:  entry.Metadata.Publisher.Validation,
			}
		}

		base := entry.DefaultRelease.Channel.Platform.Name + "@" + entry.DefaultRelease.Channel.Platform.Channel
		if base == "all@all" {
			base = ""
		}

		channel := sdk.Channel{
			Name:  entry.DefaultRelease.Channel.Name,
			Track: entry.DefaultRelease.Channel.Track,
			Risk:  entry.DefaultRelease.Channel.Risk,
		}
		channel = channel.Full()

		summary := SdkSummary{
			Name:        entry.Name,
			PackageID:   entry.PackageID,
			Summary:     entry.Metadata.Summary,
			Description: entry.Metadata.Description,
			License:     entry.Metadata.License,
			Publisher:   publisher,
			Channel:     channel.Name,
			Track:       channel.Track,
			Risk:        channel.Risk,
			Revision:    sdk.Revision{N: entry.DefaultRelease.Revision}.String(),
			ReleasedAt:  (*time.Time)(entry.DefaultRelease.Channel.ReleasedAt),
			Version:     entry.DefaultRelease.Version,
			Base:        base,
			Arch:        entry.DefaultRelease.Channel.Platform.Architecture,
		}
		result = append(result, summary)
	}
	return result, nil
}

func (w *SdkManager) Sdk(ctx context.Context, name string) (*SdkFullInfo, error) {
	full := &SdkFullInfo{Name: name}

	var sdkErr *sdkstore.SdkNotFoundError
	if err := w.fillChannels(ctx, name, full); err != nil && !errors.As(err, &sdkErr) {
		return nil, err
	}

	if err := w.fillInstalled(ctx, name, full); err != nil {
		return nil, err
	}

	if sdkErr != nil && len(full.Installed) == 0 {
		return nil, sdkErr
	}
	return full, nil
}

func (w *SdkManager) fillChannels(ctx context.Context, name string, full *SdkFullInfo) error {
	response, err := w.store.Info(ctx, name)
	if err != nil {
		return err
	}

	full.PackageID = response.PackageID
	full.Title = response.Metadata.Title
	full.Summary = response.Metadata.Summary
	full.Description = response.Metadata.Description
	full.License = response.Metadata.License
	if response.Metadata.Publisher.ID != "" {
		full.Publisher = &StoreAccount{
			ID:          response.Metadata.Publisher.ID,
			Username:    response.Metadata.Publisher.Username,
			DisplayName: response.Metadata.Publisher.DisplayName,
			Validation:  response.Metadata.Publisher.Validation,
		}
	}

	full.Channels = make([]SdkRevision, 0, len(response.ChannelMap))
	for _, entry := range response.ChannelMap {
		var sdkYaml struct {
			BuiltAt *timeutil.TimeUTC `yaml:"sdkcraft-started-at,omitempty"`
			Website string            `yaml:"website,omitempty"`
		}
		if err := yaml.Unmarshal(entry.Revision.SdkYAML, &sdkYaml); err != nil {
			return fmt.Errorf("invalid %q SDK (%v) metadata: %w", response.Name, entry.Revision.Revision, err)
		}

		base := entry.Channel.Platform.Name + "@" + entry.Channel.Platform.Channel
		if base == "all@all" {
			base = ""
		}

		channel := sdk.Channel{
			Name:  entry.Channel.Name,
			Track: entry.Channel.Track,
			Risk:  entry.Channel.Risk,
		}
		channel = channel.Full()

		// TODO: replace with fetching the website URL from
		// the InfoMetadata when the store supports it.
		if full.Website == "" && sdkYaml.Website != "" {
			full.Website = sdkYaml.Website
		}

		revision := SdkRevision{
			Channel:      channel.Name,
			Track:        channel.Track,
			Risk:         channel.Risk,
			Revision:     sdk.Revision{N: entry.Revision.Revision}.String(),
			BuiltAt:      (*time.Time)(sdkYaml.BuiltAt),
			UploadedAt:   (*time.Time)(entry.Revision.CreatedAt),
			ReleasedAt:   (*time.Time)(entry.Channel.ReleasedAt),
			Version:      entry.Revision.Version,
			Base:         base,
			Arch:         entry.Channel.Platform.Architecture,
			DownloadSize: entry.Revision.Download.Size,
		}
		full.Channels = append(full.Channels, revision)
	}
	return nil
}

func (w *SdkManager) fillInstalled(ctx context.Context, name string, full *SdkFullInfo) error {
	sdks, err := w.backend.Sdks(ctx)
	if err != nil {
		return err
	}
	sdks = slices.DeleteFunc(sdks, func(s workshop.SdkVolume) bool { return s.Name != name })

	full.Installed = make([]SdkInstalled, 0, len(sdks))
	for _, s := range sdks {
		var file sdk.File
		if err := yaml.Unmarshal([]byte(s.SdkYAML), &file); err != nil {
			return err
		}

		// Needed if the SDK isn't in the Store (e.g. try SDKs).
		if full.Title == "" {
			full.Title = file.Title
		}
		if full.Summary == "" {
			full.Summary = file.Summary
		}
		if full.Description == "" {
			full.Description = file.Description
		}
		if full.License == "" {
			full.License = file.License
		}

		for pid, wps := range s.Workshops {
			pctx := context.WithValue(ctx, workshop.ContextProjectId, pid)
			for _, wp := range wps {
				winfo, err := w.backend.Workshop(pctx, wp)
				if err != nil {
					return err
				}

				channel := ""
				sk, ok := winfo.Sdks[name]
				if ok {
					channel = sk.Channel
				}

				arch := file.Arch
				if arch == "" {
					// SDKcraft always sets architecture,
					// but we probably shouldn't rely on it.
					arch = "all"
				}

				full.Installed = append(full.Installed, SdkInstalled{
					SdkVolume: SdkVolume{
						Name:     s.Name,
						Version:  file.Version,
						Revision: s.Revision.String(),
						BuiltAt:  file.BuiltAt,
						Size:     s.Size,
					},
					Workshop:    winfo.Name,
					ProjectPath: winfo.Project.Path,
					Channel:     channel,
					Base:        file.Base,
					Arch:        arch,
				})
			}
		}
	}
	return nil
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

func MockTime(now time.Time) (restore func()) {
	old := timeNow
	timeNow = func() time.Time { return now }
	return func() { timeNow = old }
}
