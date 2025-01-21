package workshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
)

var (
	ConfigProjectId         = "user.workshop.project-id"
	ConfigWorkshopFile      = "user.workshop.file"
	ConfigWorkshopSdks      = "user.workshop.sdks"
	ConfigVolumeMeta        = "user.sdk.meta"
	ConfigProjectPathDevice = "workshop.project"
)

var InstallTimeNow = time.Now

type Workshop struct {
	Backend Backend
	Project Project
	// Workshop file that was used to launch it; it may be out of sync with the
	// file in the project directory due to user's edits, etc.
	File    *File
	Name    string
	Base    string
	Running bool
	// Installed SDKs.
	Sdks map[string]sdk.Setup
	// Workshop devices installed.
	Profiles map[string]SdkProfile
}

// Associate an SDK with the workshop by creating a 'current' symlink and adding
// the SDK to the Sdks field.
func (w *Workshop) LinkSdk(ctx context.Context, s sdk.Setup) error {
	fs, err := w.Backend.WorkshopFs(ctx, w.Name)
	if err != nil {
		return err
	}
	defer fs.Close()

	// Update the current link to point out to the newly installed SDK.
	sdkrev := sdk.SdkRevPath(s.Name, s.Revision.String())
	current := sdk.SdkCurrentPath(s.Name)

	rev := revert.New()
	defer rev.Fail()

	oldcur, err := fs.ReadLink(current)
	if err != nil && !osutil.IsDirNotExist(err) {
		return err
	}
	if err == nil {
		// The link could already be existing (e.g. there is a previous
		// revision).
		if err = fs.Remove(current); err != nil {
			return err
		}
		rev.Add(func() { _ = fs.Symlink(oldcur, current) })
	}

	if err = fs.Symlink(sdkrev, current); err != nil {
		return err
	}
	rev.Add(func() { _ = fs.Remove(current) })

	// We do not track the system SDK in the Sdks field; this is only for the
	// user-defined SDKs as the system SDK is a special case (e.g. cannot be
	// removed from the workshop).
	if s.Name != sdk.System.String() {
		now := InstallTimeNow()
		s.InstallTime = &now

		sdksetup, exist := w.Sdks[s.Name]
		if exist {
			sdksetup.RevisionSequence = append(sdksetup.RevisionSequence, sdksetup.Revision)
			sdksetup.Revision = s.Revision
			w.Sdks[s.Name] = sdksetup
		} else {
			w.Sdks[s.Name] = s
		}

		sequenceValue, err := json.Marshal(w.Sdks)
		if err != nil {
			return err
		}

		err = w.Backend.AddWorkshopConfig(ctx, w.Name,
			&WorkshopConfigValue{
				Name:  ConfigWorkshopSdks,
				Value: string(sequenceValue),
			})

		if err != nil {
			return err
		}
	}

	rev.Success()
	return nil
}

// Stops associating an SDK with the workshop by removing a 'current' symlink and
// removing the SDK from the workshop "installed" SDKs if there are no more
// revisions left.
func (w *Workshop) UnlinkSdk(ctx context.Context, name string) error {
	if name != sdk.System.String() {
		setup := w.Sdks[name]
		if len(setup.RevisionSequence) > 0 {
			setup.Revision = setup.RevisionSequence[len(setup.RevisionSequence)-1]
			setup.RevisionSequence = setup.RevisionSequence[:len(setup.RevisionSequence)-1]
			w.Sdks[name] = setup
		} else {
			delete(w.Sdks, name)
		}
		newSequence, err := json.Marshal(w.Sdks)
		if err != nil {
			return err
		}

		err = w.Backend.AddWorkshopConfig(ctx, w.Name,
			&WorkshopConfigValue{
				Name:  ConfigWorkshopSdks,
				Value: string(newSequence),
			})
		if err != nil {
			return err
		}
	}

	// Update the 'current' link
	fs, err := w.Backend.WorkshopFs(ctx, w.Name)
	if err != nil {
		return err
	}
	defer fs.Close()

	if setup, exist := w.Sdks[name]; exist {
		if err = fs.Remove(sdk.SdkCurrentPath(name)); err != nil {
			return err
		}
		if err = fs.Symlink(sdk.SdkRevPath(name, setup.Revision.String()), sdk.SdkCurrentPath(name)); err != nil {
			return err
		}
		return nil
	}

	// No revisions left in the sequence, remove the 'current' link.
	// This will be the case during a launch operation that fails, therefore it's
	// possible for there to be no current revision to remove.
	if err = fs.Remove(sdk.SdkCurrentPath(name)); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func WorkshopName(instance string) string {
	idx := strings.LastIndex(instance, "-")
	if idx == -1 {
		return ""
	}

	// drop the project id from the name
	return instance[:idx]
}

func WorkshopStateVolumeName(ws, pid string) string {
	return fmt.Sprintf("%s-%s-state-volume", ws, pid)
}

func AptCacheVolumeName(ws, pid string) string {
	return fmt.Sprintf("%s-%s-cache-apt", ws, pid)
}

func (w *Workshop) metaFromVolume(ctx context.Context, setup sdk.Setup) (string, error) {
	vinfo, err := w.Backend.Volume(ctx, sdk.VolumeName(setup.Name, setup.Revision.String()))
	if err != nil {
		return "", err
	}

	meta, ok := vinfo.Config[ConfigVolumeMeta]
	if !ok {
		return "", fmt.Errorf("cannot find %q SDK metadata", setup.Name)
	}
	return meta, nil
}

func (w *Workshop) metaFromFs(ctx context.Context, setup sdk.Setup) (string, error) {
	fs, err := w.Backend.WorkshopFs(ctx, w.Name)
	if err != nil {
		return "", err
	}
	defer fs.Close()

	metapath := sdk.SdkMetaPath(setup.Name)
	meta, err := afero.ReadFile(fs, metapath)
	return string(meta), err
}

// Reads information about the installed SDK from its meta file.
func (w *Workshop) SdkInfo(ctx context.Context, sdkName string) (*sdk.Info, error) {
	// TODO: isLocal should rely on the SDK source and not the name of the SDK.
	// Currently, a sketch or system SDK are both considered local and these are
	// the only SDKs of that source. This requires rework, once an arbitrary SDK
	// can be installed from a local source, e.g. to support workshop try.
	isLocal := func(n string) bool {
		return n == sdk.Sketch || n == sdk.System.String()
	}

	setup, ok := w.Sdks[sdkName]
	if sdkName != sdk.System.String() && !ok {
		return nil, fmt.Errorf("SDK %q is not installed in %q workshop", sdkName, w.Name)
	}
	if sdkName == sdk.System.String() {
		setup = sdk.Setup{Name: sdk.System.String(), Revision: sdk.Revision{N: 1}}
	}

	var err error
	var meta string
	if isLocal(sdkName) {
		meta, err = w.metaFromFs(ctx, setup)
	} else {
		meta, err = w.metaFromVolume(ctx, setup)
	}

	if err != nil {
		return nil, err
	}

	info, err := sdk.ReadSdkInfo([]byte(meta), w.Project.ProjectId, w.Name)
	if err != nil {
		return nil, err
	}

	// system SDK will always have its workshop's base.
	if info.Type == sdk.System {
		info.Base = w.Base
	}
	info.Revision = setup.Revision
	info.Channel = setup.Channel

	// Now add changes defined for this SDK in the workshop file (e.g. plug
	// binds, slots).
	idx := slices.IndexFunc(w.File.Sdks, func(sr SdkRecord) bool { return sr.Name == info.Name })

	// system and sketch SDK is an optional entry in a workshop file, so it's not an error
	// scenario.
	if idx == -1 && isLocal(sdkName) {
		return info, nil
	}

	if idx == -1 {
		return nil, fmt.Errorf("internal error: %q SDK is installed but not declared in the workshop file", info.Name)
	}

	binds := map[string]*sdk.PlugBind{}
	plugs := map[string]interface{}{}
	for name, m := range w.File.Sdks[idx].Plugs {
		if m.Bind == nil {
			plugs[name] = m.Attributes
		} else {
			binds[name] = &sdk.PlugBind{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: m.Bind.Sdk, Name: m.Bind.Name}
		}
	}

	if err = info.SetupWorkshopPlugs(plugs); err != nil {
		return nil, err
	}

	if err = info.SetupPlugBinds(binds); err != nil {
		return nil, err
	}

	if err = info.SetupWorkshopSlots(w.File.Sdks[idx].Slots); err != nil {
		return nil, err
	}

	return info, nil
}

// Returns a map of SDK info for installed SDKs. The info includes SDK details
// parsed from its sdk.yaml, such as base, plugs, slots, etc.
func (w *Workshop) SdkInfos(ctx context.Context) (map[string]*sdk.Info, error) {
	var infos = make(map[string]*sdk.Info, len(w.Sdks))
	for _, sdk := range w.Sdks {
		info, err := w.SdkInfo(ctx, sdk.Name)
		if err != nil {
			return nil, err
		}
		infos[info.Name] = info
	}
	return infos, nil
}

func install(wfs WorkshopFs, srcfs fs.FS, src, dst string, perm fs.FileMode) error {
	dstdir := filepath.Dir(dst)
	if err := wfs.MkdirAll(dstdir, 0755); err != nil {
		return err
	}

	filesrc, err := srcfs.Open(src)
	if err != nil {
		return err
	}
	defer filesrc.Close()

	filedst, err := wfs.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer filedst.Close()

	if _, err = io.Copy(filedst, filesrc); err != nil {
		return err
	}
	return nil
}

func (w *Workshop) InstallLocalSdk(ctx context.Context, name string, rev string, src fs.FS) error {
	wfs, err := w.Backend.WorkshopFs(ctx, w.Name)
	if err != nil {
		return err
	}
	defer wfs.Close()

	reverter := revert.New()
	defer reverter.Fail()

	// meta: /var/lib/workshop/sdk/<name>/<rev>/meta
	metasrc := filepath.Join("meta", "sdk.yaml")
	metadst := filepath.Join(sdk.SdkRevPath(name, rev), "meta", "sdk.yaml")
	reverter.Add(func() { _ = wfs.RemoveAll(filepath.Dir(metadst)) })

	if err = install(wfs, src, metasrc, metadst, 0644); err != nil {
		return err
	}

	// hooks: /var/lib/workshop/sdk/<name>/<rev>/sdk/hooks
	hooksdir := filepath.Join(sdk.SdkRevPath(name, rev), "sdk", "hooks")
	reverter.Add(func() { _ = wfs.RemoveAll(hooksdir) })

	for _, hook := range []string{"setup-base", "save-state", "restore-state", "check-health"} {
		hooksrc := filepath.Join("hooks", hook)
		hookdst := filepath.Join(hooksdir, hook)

		// Hooks are optional.
		if _, err := src.Open(hooksrc); err != nil {
			if !osutil.IsDirNotExist(err) {
				return err
			}
			continue
		}

		if err = install(wfs, src, hooksrc, hookdst, 0755); err != nil {
			return err
		}
	}

	reverter.Success()
	return nil
}
