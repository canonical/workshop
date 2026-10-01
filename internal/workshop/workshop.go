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

package workshop

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
)

var (
	ConfigProjectId               = "user.workshop.project-id"
	ConfigWorkshopName            = "user.workshop.name"
	ConfigWorkshopFile            = "user.workshop.file"
	ConfigWorkshopBase            = "user.workshop.base"
	ConfigWorkshopBaseFingerprint = "user.workshop.base-fingerprint"
	ConfigWorkshopSnapshotType    = "user.workshop.snapshot-type"
	ConfigWorkshopSnapshotFormat  = "user.workshop.format-revision"
	ConfigWorkshopSha3_384        = "user.workshop.sha3-384"
	ConfigProjectPathDevice       = "workshop.project"
	ConfigStateStorageDevice      = "workshop.state-storage"
)

var InstallTimeNow = time.Now

type Workshop struct {
	Backend Backend
	Project Project
	// Workshop file that was used to launch it; it may be out of sync with the
	// file in the project directory due to user's edits, etc.
	File    *File
	Name    string
	Format  sdk.Revision
	Image   BaseImage
	Running bool
	// Installed SDKs.
	Sdks map[string]SdkInstallation
	// Workshop devices installed.
	Profiles map[string]SdkProfile
	Hostname Hostname
}

type Hostname struct {
	// Domain is the workshop's DNS name (e.g. "ws.myapp.wp" or "ws.42424242.wp"),
	// if available.
	Domain string
	// Note explains why Domain uses the project ID instead of basename.
	Note string
}

type SdkInstallation struct {
	sdk.Setup
	// 1-based index of SDK installation (0 is reserved for the base).
	InstallOrder int       `json:"install-order"`
	InstalledAt  time.Time `json:"installed-at"`
}

func SdkDeviceName(sk string) string {
	return "sdk." + sk
}

func (w *Workshop) meta(ctx context.Context, sk sdk.Setup) (string, error) {
	if sk.IsVolume() {
		return w.metaFromVolume(ctx, sk)
	} else {
		return w.metaFromFile(ctx, sk)
	}
}

func (w *Workshop) metaFromVolume(ctx context.Context, setup sdk.Setup) (string, error) {
	vinfo, err := w.Backend.Sdk(ctx, setup)
	if err != nil {
		return "", err
	}

	if vinfo.SdkYAML == "" {
		return "", fmt.Errorf("cannot find %q SDK metadata", setup.Name)
	}
	return vinfo.SdkYAML, nil
}

func (w *Workshop) metaFromFile(ctx context.Context, setup sdk.Setup) (string, error) {
	username, ok := ctx.Value(ContextUser).(string)
	if !ok {
		return "", fmt.Errorf("context key %s not found", ContextUser)
	}

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return "", err
	}
	userDataDir := UserDataRootDir(usr.HomeDir, env)

	sdkDir := LocalSdkDir(userDataDir, w.Project.ProjectId, w.Name, setup.Name)
	metapath := filepath.Join(sdkDir, setup.Sha3_384, "meta", "sdk.yaml")

	meta, err := os.ReadFile(metapath)
	return string(meta), err
}

type Sanitizer = func(info *sdk.Info, runtime Runtime) error

func sdkSanitizer(sanitize Sanitizer, runtime Runtime) sdk.Sanitizer {
	if sanitize == nil {
		return nil
	}
	return func(i *sdk.Info) error { return sanitize(i, runtime) }
}

func ValidateSdkInfo(pid string, file *File, sdkName, sdkYaml string, sanitize Sanitizer) error {
	additions, err := sdkAdditions(pid, file, sdkName)
	if err != nil {
		return err
	}

	return validateSdkInfo(pid, file.Name, file.Base, sdkName, sdkYaml, additions, sdkSanitizer(sanitize, file.Runtime))
}

func ValidateSketch(pid, w, base, runtime string, sdkYaml []byte, sanitize Sanitizer) error {
	var r Runtime
	if err := r.UnmarshalText([]byte(runtime)); err != nil {
		return err
	}
	return validateSdkInfo(pid, w, base, sdk.Sketch, string(sdkYaml), nil, sdkSanitizer(sanitize, r))
}

func validateSdkInfo(pid, w, base, sk, sdkYaml string, additions []sdk.Additions, sanitize sdk.Sanitizer) error {
	info, err := sdk.ReadSdkInfo([]byte(sdkYaml), pid, w, additions, sanitize)
	if err != nil {
		return fmt.Errorf("invalid %q SDK: %w", sk, err)
	}

	if err := sdk.Validate(info); err != nil {
		return err
	}

	if info.Name != sk {
		return fmt.Errorf("SDK must be named %q (now: %q)", sk, info.Name)
	}
	if !slices.Contains([]string{"", base}, info.Base) {
		return fmt.Errorf("%q SDK has %q base; required: %q", sk, info.Base, base)
	}
	if !slices.Contains([]string{"", "all", arch.DpkgArchitecture()}, info.Arch) {
		return fmt.Errorf(`%q SDK has %q architecture; required: %q or "all"`, sk, info.Arch, arch.DpkgArchitecture())
	}

	return nil
}

// Reads information about the installed SDK from its meta file.
func (w *Workshop) SdkFile(ctx context.Context, sdkName string) (*sdk.File, error) {
	sk, ok := w.Sdks[sdkName]
	if !ok {
		return nil, fmt.Errorf("SDK %q is not installed in %q workshop", sdkName, w.Name)
	}

	meta, err := w.meta(ctx, sk.Setup)
	if err != nil {
		return nil, err
	}

	var file sdk.File
	if err := yaml.Unmarshal([]byte(meta), &file); err != nil {
		return nil, err
	}

	return &file, nil
}

// Reads information about the installed SDK from its meta file and merges it
// with the workshop definition.
func (w *Workshop) SdkInfo(ctx context.Context, sdkName string, sanitize Sanitizer) (*sdk.Info, error) {
	sk, ok := w.Sdks[sdkName]
	if !ok {
		return nil, fmt.Errorf("SDK %q is not installed in %q workshop", sdkName, w.Name)
	}

	meta, err := w.meta(ctx, sk.Setup)
	if err != nil {
		return nil, err
	}

	additions, err := sdkAdditions(w.Project.ProjectId, w.File, sdkName)
	if err != nil {
		return nil, err
	}

	info, err := sdk.ReadSdkInfo([]byte(meta), w.Project.ProjectId, w.Name, additions, sdkSanitizer(sanitize, w.File.Runtime))
	if err != nil {
		return nil, err
	}
	if info.Name != sdkName {
		return nil, fmt.Errorf("SDK must be named %q (now: %q)", sdkName, info.Name)
	}

	info.Revision = sk.Revision
	info.Channel = sk.Channel
	info.Source = sk.Source
	info.PackageID = sk.PackageID

	return info, nil
}

func sdkAdditions(pid string, file *File, sdkName string) ([]sdk.Additions, error) {
	idx := slices.IndexFunc(file.Sdks, func(sr SdkRecord) bool { return sr.Name == sdkName })
	if idx < 0 {
		// system and sketch SDK is an optional entry in a workshop file, so it's not an error
		// scenario.
		if IsImplicitSdk(sdkName) {
			return nil, nil
		}
		return nil, fmt.Errorf("internal error: %q SDK is installed but not declared in the workshop file", sdkName)
	}

	plugs := make(map[string]any, len(file.Sdks[idx].Plugs))
	binds := map[string]sdk.PlugRef{}
	for name, m := range file.Sdks[idx].Plugs {
		if m.Bind == nil {
			plugs[name] = m.Plug
		} else {
			binds[name] = sdk.PlugRef{ProjectId: pid, Workshop: file.Name, Sdk: m.Bind.Sdk, Name: m.Bind.Name}
		}
	}

	slots := file.Sdks[idx].Slots
	return []sdk.Additions{{Plugs: plugs, Slots: slots, Binds: binds}}, nil
}

// Returns a map of SDK files for installed SDKs.
func (w *Workshop) SdkFilesByInstallOrder(ctx context.Context) ([]*sdk.File, error) {
	var files = make([]*sdk.File, 0, len(w.Sdks))
	for _, sdk := range w.SdksByInstallOrder() {
		file, err := w.SdkFile(ctx, sdk.Name)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// Returns a map of SDK info for installed SDKs. The info includes SDK details
// parsed from its sdk.yaml, such as base, plugs, slots, etc.
func (w *Workshop) SdkInfosByInstallOrder(ctx context.Context, sanitize Sanitizer) ([]*sdk.Info, error) {
	var infos = make([]*sdk.Info, 0, len(w.Sdks))
	for _, sdk := range w.SdksByInstallOrder() {
		info, err := w.SdkInfo(ctx, sdk.Name, sanitize)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// Returns the list of SDKs of the workshop sorted by installation order.
func (w *Workshop) SdksByInstallOrder() []SdkInstallation {
	return slices.SortedFunc(maps.Values(w.Sdks), func(a, b SdkInstallation) int {
		return cmp.Compare(a.InstallOrder, b.InstallOrder)
	})
}

func (w *Workshop) Bound() (map[sdk.PlugRef][]sdk.PlugRef, map[sdk.PlugRef]sdk.PlugRef) {
	masters := make(map[sdk.PlugRef][]sdk.PlugRef)
	slaves := make(map[sdk.PlugRef]sdk.PlugRef)
	for _, s := range w.File.Sdks {
		for name, pl := range s.Plugs {
			if pl.Bind == nil {
				continue
			}
			sk, plug := pl.Bind.Sdk, pl.Bind.Name
			mkey := sdk.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: sk, Name: plug}
			skey := sdk.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: s.Name, Name: name}
			masters[mkey] = append(masters[mkey], skey)
			slaves[skey] = mkey
		}
	}
	return masters, slaves
}

// Mounts returns a map of active SDK mounts for the workshop.
func (w *Workshop) Mounts() map[string][]Mount {
	masters, _ := w.Bound()

	mnts := map[string][]Mount{}
	for _, prof := range w.Profiles {
		for _, mnt := range prof.Mounts {
			mnts[prof.Sdk] = append(mnts[prof.Sdk], mnt)
			if mnt.Type != HostWorkshop {
				continue
			}

			pref := sdk.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: prof.Sdk, Name: mnt.Name}
			for _, slave := range masters[pref] {
				mnt.Name = slave.Name
				mnts[slave.Sdk] = append(mnts[slave.Sdk], mnt)
			}
		}
	}

	return mnts
}

// Tunnels returns a map of active SDK tunnels for the workshop.
func (w *Workshop) Tunnels() map[string][]Tunnel {
	masters, _ := w.Bound()

	tunnels := map[string][]Tunnel{}
	for _, prof := range w.Profiles {
		for _, tunnel := range prof.Tunnels {
			tunnels[prof.Sdk] = append(tunnels[prof.Sdk], tunnel)

			pref := sdk.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: prof.Sdk, Name: tunnel.Name}
			for _, slave := range masters[pref] {
				tunnel.Name = slave.Name
				tunnels[slave.Sdk] = append(tunnels[slave.Sdk], tunnel)
			}
		}
	}

	return tunnels
}
