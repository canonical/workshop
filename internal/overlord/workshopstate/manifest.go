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

package workshopstate

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/sdk/system"
	"github.com/canonical/workshop/internal/sdkstore/transport"
	"github.com/canonical/workshop/internal/workshop"
)

// Manifest lists the components needed to construct a workshop. It augments
// the workshop definition with specific versions of the base image and SDKs.
type Manifest struct {
	File   *workshop.File
	Format sdk.Revision
	Image  workshop.BaseImage
	Sdks   []sdk.Setup
}

func (m *Manifest) maybeRevision(sk string) sdk.Revision {
	if m == nil {
		return sdk.Revision{}
	}

	idx := slices.IndexFunc(m.Sdks, func(s sdk.Setup) bool {
		return s.Name == sk
	})
	if idx < 0 {
		return sdk.Revision{}
	}

	return m.Sdks[idx].Revision
}

// LaunchManifests reads the definitions of the given workshops and resolves
// the latest base image and SDKs for each of them. This information is
// bundled into the resulting manifests.
func (w *WorkshopManager) LaunchManifests(ctx context.Context, project workshop.Project, names []string) ([]Manifest, error) {
	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key user not found")
	}

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return nil, err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	a := artifactFinder{
		WorkshopManager: w,
		user:            usr,
		userDataDir:     userDataDir,
		project:         project,
	}
	_, manifests, err := a.launchOrRefreshManifests(ctx, names, false)
	return manifests, err
}

// RefreshManifests reconstructs the manifests that were used to create the
// given workshops. For --restore, these manifests are also used to refresh the
// workshops. Otherwise it resolves a new manifest similar to LaunchManifests.
func (w *WorkshopManager) RefreshManifests(ctx context.Context, project workshop.Project, names []string, option conflict.RefreshOption) (current, latest []Manifest, err error) {
	switch option {
	case conflict.RefreshUpdate:
		username, ok := ctx.Value(workshop.ContextUser).(string)
		if !ok {
			return nil, nil, fmt.Errorf("context key user not found")
		}

		usr, env, err := osutil.UserAndEnv(username)
		if err != nil {
			return nil, nil, err
		}
		userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

		a := artifactFinder{
			WorkshopManager: w,
			user:            usr,
			userDataDir:     userDataDir,
			project:         project,
		}
		return a.launchOrRefreshManifests(ctx, names, true)
	case conflict.RefreshRestore:
		format := w.backend.FormatRevision()

		manifests := make([]Manifest, 0, len(names))
		for _, name := range names {
			manifest, err := w.workshopManifest(ctx, project.ProjectId, name)
			if err != nil {
				return nil, nil, fmt.Errorf("cannot refresh %q: %w", name, err)
			}
			if manifest.Format != format {
				return nil, nil, fmt.Errorf("cannot refresh %q: workshop created using incompatible Workshop version", name)
			}
			manifests = append(manifests, *manifest)
		}
		return manifests, manifests, nil
	default:
		return nil, nil, errors.New("cannot refresh: internal error: unknown refresh option")
	}
}

func (w *WorkshopManager) RemoveManifests(ctx context.Context, projectId string, names []string) (stashed, current []Manifest, running map[string]bool, err error) {
	ctx = context.WithValue(ctx, workshop.ContextProjectId, projectId)

	stashed = make([]Manifest, 0, len(names))
	current = make([]Manifest, 0, len(names))
	running = make(map[string]bool, len(names))
	for _, name := range names {
		wp, err := w.backend.Workshop(ctx, name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}

		manifest, err := w.maybeDiscardWaitingRefresh(projectId, wp.File)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}
		if manifest != nil {
			stashed = append(stashed, *manifest)
		}

		allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.ErrorStatus, healthstate.StoppedStatus}
		if err := healthstate.CheckWorkshopHealth(w.state, wp, allowed); err != nil {
			return nil, nil, nil, fmt.Errorf("cannot remove %q: %w", name, err)
		}

		installed := make([]sdk.Setup, 0, len(wp.Sdks))
		for _, sk := range wp.SdksByInstallOrder() {
			installed = append(installed, sk.Setup)
		}
		current = append(current, Manifest{File: wp.File, Format: wp.Format, Image: wp.Image, Sdks: installed})

		running[wp.File.Name] = wp.Running
	}

	return stashed, current, running, nil
}

func (w *WorkshopManager) maybeDiscardWaitingRefresh(projectId string, file *workshop.File) (*Manifest, error) {
	err := conflict.CheckChangeConflict(w.state, projectId, file.Name, []string{"exec"})
	if err == nil {
		return nil, nil
	}
	conflictErr, ok := errors.AsType[*conflict.ChangeConflictError](err)
	if !ok {
		return nil, err
	}
	chg := w.state.Change(conflictErr.ChangeID)
	if chg.Status() != state.WaitStatus || (chg.Kind() != "launch" && chg.Kind() != "refresh") {
		// Let CheckWorkshopHealth handle the error.
		return nil, nil
	}

	var stashed *Manifest
	if chg.Kind() == "refresh" {
		format, err := handlersetup.WorkshopFormat(chg, file.Name, handlersetup.OldWorkshop)
		if err != nil {
			return nil, err
		}
		image, err := handlersetup.WorkshopBase(chg, file.Name, handlersetup.OldWorkshop)
		if err != nil {
			return nil, err
		}
		sdks, err := handlersetup.WorkshopSdks(chg, file.Name, handlersetup.OldWorkshop)
		if err != nil {
			return nil, err
		}
		// The file here is not quite right, but we only use the name field. If
		// that changes in future, we should reconstruct it from chg.
		stashed = &Manifest{File: file, Format: format, Image: image, Sdks: sdks}
	}

	conflict.BackgroundDiscard(chg, file.Name)
	return stashed, nil
}

type artifactFinder struct {
	*WorkshopManager
	user        *user.User
	userDataDir string
	project     workshop.Project
	sdkVolumes  []workshop.SdkVolume
}

// launchOrRefreshManifests constructs new manifests for the given workshops.
// If refresh is true it also reconstructs manifests from existing workshops.
// TODO: query the Store asynchronously, so we can move the launch/refresh
// specific part out of the middle of this function but still share most of the
// code between launch and refresh.
func (a *artifactFinder) launchOrRefreshManifests(ctx context.Context, names []string, refresh bool) (current, latest []Manifest, err error) {
	action := "launch"
	if refresh {
		action = "refresh"
	}

	sto := sdk.StoreService(a.state)

	rev := revert.New()
	defer rev.Fail()

	// Not an error, the state is locked; unlock it to let other requests to be
	// processed while we are getting the store info sorted.
	// This code can be concurrent with other changes,
	// so we avoid interacting with local SDKs.
	a.state.Unlock()
	rev.Add(a.state.Lock)

	systemMeta, err := system.SystemSdkMeta()
	if err != nil {
		return nil, nil, err
	}

	files := make([]*workshop.File, 0, len(names))
	images := make([]workshop.BaseImage, 0, len(names))
	storeSdks := make([][]sdk.Setup, 0, len(names))

	for _, name := range names {
		file, err := a.project.Workshop(name)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		files = append(files, file)

		image, err := a.backend.GetBase(ctx, file.Base, file.Confinement)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		images = append(images, image)

		sdks, err := a.findStoreSdks(sto, ctx, file)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}
		if file.Confinement == workshop.ConfinementContainer {
			sdks = slices.Insert(sdks, 0, systemMeta.Setup)
		}
		storeSdks = append(storeSdks, sdks)
	}

	a.state.Lock()
	rev.Success()

	current = make([]Manifest, 0, len(names))
	latest = make([]Manifest, 0, len(names))

	for i, name := range names {
		// Workshop may change while the state is unlocked so we have
		// to query it after re-locking.
		var cur *Manifest
		if refresh {
			cur, err = a.workshopManifest(ctx, a.project.ProjectId, name)
			if err != nil {
				return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
			}

			if cur.File.Confinement != files[i].Confinement {
				c1, err1 := cur.File.Confinement.MarshalText()
				c2, err2 := files[i].Confinement.MarshalText()
				if err := cmp.Or(err1, err2); err != nil {
					return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
				}
				return nil, nil, fmt.Errorf("cannot %s %q: confinement changed from %q to %q", action, name, c1, c2)
			}

			current = append(current, *cur)
		} else if err := a.checkNotLaunched(ctx, a.project.ProjectId, name); err != nil {
			return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}

		localSdks, err := a.findLocalSdks(ctx, cur, files[i])
		if err != nil {
			return nil, nil, fmt.Errorf("cannot %s %q: %w", action, name, err)
		}

		format := a.backend.FormatRevision()
		installOrder := sdkInstallOrder(files[i])
		sdks := ordered(installOrder, storeSdks[i], localSdks)
		if files[i].Confinement != workshop.ConfinementContainer && len(sdks) > 0 {
			return nil, nil, fmt.Errorf("cannot %s %q: SDKs are currently unavailable for virtual machines", action, name)
		}
		latest = append(latest, Manifest{File: files[i], Format: format, Image: images[i], Sdks: sdks})
	}

	return current, latest, nil
}

func (w *WorkshopManager) checkNotLaunched(ctx context.Context, projectId, name string) error {
	_, err := w.Workshop(ctx, name, projectId)
	if err == nil {
		return errors.New("workshop exists")
	}
	if !errors.Is(err, workshop.ErrWorkshopNotLaunched) {
		return fmt.Errorf("failed to check whether the workshop exists: %w", err)
	}
	if err := conflict.CheckChangeConflict(w.state, projectId, name, nil); err != nil {
		return fmt.Errorf("other changes in progress: %w", err)
	}
	return nil
}

func (w *WorkshopManager) workshopManifest(ctx context.Context, projectId, name string) (*Manifest, error) {
	wp, err := w.Workshop(ctx, name, projectId)
	if err != nil {
		return nil, err
	}

	err = healthstate.CheckWorkshopHealth(
		w.state, wp, []healthstate.Status{healthstate.ReadyStatus})
	if err != nil {
		return nil, err
	}

	installed := make([]sdk.Setup, 0, len(wp.Sdks))
	for _, sk := range wp.SdksByInstallOrder() {
		installed = append(installed, sk.Setup)
	}
	return &Manifest{File: wp.File, Format: wp.Format, Image: wp.Image, Sdks: installed}, nil
}

func (a *artifactFinder) findStoreSdks(sto sdk.Store, ctx context.Context, file *workshop.File) ([]sdk.Setup, error) {
	platforms, err := makePlatforms(file.Base)
	if err != nil {
		return nil, err
	}

	req, err := makeResolveRequest(platforms[0], file.Sdks)
	if err != nil {
		return nil, err
	}
	if len(req.Packages) == 0 {
		return nil, nil
	}

	// We have to request each SDK 4 times because the Store requires SDK
	// platforms to exactly match the request.
	pkgs := make([]transport.ResolvePackage, 0, len(platforms)*len(req.Packages))
	for i, platform := range platforms {
		for _, pkg := range req.Packages {
			pkg.InstanceKey += fmt.Sprint("_", i)
			pkg.Platform = platform
			pkgs = append(pkgs, pkg)
		}
	}
	pkgs, req.Packages = req.Packages, pkgs

	resp, err := sto.Resolve(ctx, req)
	if err != nil {
		return nil, err
	}

	sdks := make([]sdk.Setup, 0, len(req.Packages))
	errs := make([]error, 0, len(req.Packages))
	for _, pkg := range pkgs {
		var setup sdk.Setup
		var errActual, errNotFound error
		// Pick the most recent SDK among the compatible variants. Some
		// platforms are likely to return revision-not-found errors. We ignore
		// all errors if at least one SDK was found; if not we return the first
		// unexpected error. If all errors are revision-not-found, we return
		// the first one.
		for i := range platforms {
			key := fmt.Sprint(pkg.InstanceKey, "_", i)
			idx := slices.IndexFunc(resp.PackageResults, func(p transport.ResolvePackageResponse) bool {
				return p.InstanceKey == key
			})
			if idx < 0 {
				errActual = cmp.Or(errActual, fmt.Errorf("%q SDK: not found in refresh response", pkg.Name))
				continue
			}

			if err := storeError(resp.PackageResults[idx]); err != nil {
				if resp.PackageResults[idx].Error != nil && resp.PackageResults[idx].Error.Code == "revision-not-found" {
					errNotFound = cmp.Or(errNotFound, err)
				} else {
					errActual = cmp.Or(errActual, err)
				}
				continue
			}

			s, err := resolvedSetup(resp.PackageResults[idx])
			if err != nil {
				errActual = cmp.Or(errActual, err)
				continue
			}

			if s.Revision.N > setup.Revision.N {
				setup = s
			}
		}

		if !setup.Revision.Unset() {
			sdks = append(sdks, setup)
		} else if errActual != nil {
			errs = append(errs, errActual)
		} else if errNotFound != nil {
			errs = append(errs, errNotFound)
		}
	}

	if len(errs) == 1 {
		return nil, errs[0]
	}
	if len(errs) > 1 {
		for i := range errs {
			errs[i] = fmt.Errorf("- %w", errs[i])
		}
		return nil, fmt.Errorf("multiple SDK Store errors:\n%w", errors.Join(errs...))
	}

	return sdks, nil
}

func makePlatforms(base string) ([]transport.Platform, error) {
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '@' })
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid base %q (expected <NAME>@<VERSION>)", base)
	}

	return []transport.Platform{{
		Name:         parts[0],
		Channel:      parts[1],
		Architecture: arch.DpkgArchitecture(),
	}, {
		Name:         "all",
		Channel:      "all",
		Architecture: arch.DpkgArchitecture(),
	}, {
		Name:         parts[0],
		Channel:      parts[1],
		Architecture: "all",
	}, {
		Name:         "all",
		Channel:      "all",
		Architecture: "all",
	}}, nil
}

func makeResolveRequest(platform transport.Platform, sdks []workshop.SdkRecord) (transport.ResolveRequest, error) {
	req := transport.ResolveRequest{
		Packages: make([]transport.ResolvePackage, 0, len(sdks)),
	}
	for _, sd := range sdks {
		if sd.Source != sdk.StoreSource {
			continue
		}

		pkg := transport.ResolvePackage{
			Namespace: "sdk",
			Name:      sd.Name,
			Channel:   sd.Channel,
			Platform:  platform,
		}
		if pkg.Channel == "" {
			pkg.Channel = "stable"
		}

		for {
			uuid, err := uuid.NewRandom()
			if err != nil {
				return transport.ResolveRequest{}, err
			}
			pkg.InstanceKey = uuid.String()

			hasKey := func(p transport.ResolvePackage) bool {
				return pkg.InstanceKey == p.InstanceKey
			}
			if !slices.ContainsFunc(req.Packages, hasKey) {
				break
			}
		}

		req.Packages = append(req.Packages, pkg)
	}
	return req, nil
}

func storeError(resp transport.ResolvePackageResponse) error {
	if resp.Status != "error" && resp.Error == nil {
		return nil
	}

	message := "unknown error"
	if resp.Error != nil {
		message = resp.Error.Message
	}

	if resp.Name == "" {
		return fmt.Errorf("unknown SDK: %s", message)
	}
	return fmt.Errorf("%q SDK: %s", resp.Name, message)
}

func resolvedSetup(resp transport.ResolvePackageResponse) (sdk.Setup, error) {
	channel, err := sdk.ParseChannel(resp.Result.Channel.EffectiveChannel)
	if err != nil {
		return sdk.Setup{}, err
	}

	return sdk.Setup{
		Name:      resp.Name,
		PackageID: resp.ID,
		Channel:   channel.Full().Name,
		Revision:  sdk.Revision{N: resp.Result.Revision.Revision},
		Sha3_384:  resp.Result.Revision.Download.Sha3_384,
	}, nil
}

func (a *artifactFinder) findLocalSdks(ctx context.Context, current *Manifest, latest *workshop.File) ([]sdk.Setup, error) {
	sdks := make([]sdk.Meta, 0, len(latest.Sdks))
	for _, sd := range latest.Sdks {
		switch sd.Source {
		case sdk.TrySource:
			meta, err := a.findTrySdk(ctx, latest.Base, sd.Name)
			if err != nil {
				return nil, err
			}
			sdks = append(sdks, *meta)
		case sdk.ProjectSource:
			installed := current.maybeRevision(sd.Name)

			meta, err := a.findInProjectSdk(latest.Name, sd.Name, installed)
			if err != nil {
				return nil, err
			}
			sdks = append(sdks, *meta)
		}
	}

	action := "launch"
	if current != nil {
		action = "refresh"
	}
	installed := current.maybeRevision(sdk.Sketch)

	meta, err := a.maybeFindSketchSdk(action, latest.Name, installed)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		sdks = append(sdks, *meta)
	} else {
		// If there's no sketch SDK, it's usually fine. But in rare
		// cases (e.g. plug binding) the user might add the sketch SDK
		// to the workshop definition. We shouldn't just ignore it,
		// since it makes it harder to reason about the list of SDKs.
		idx := slices.IndexFunc(latest.Sdks, func(s workshop.SdkRecord) bool {
			return s.Source == sdk.SketchSource
		})
		if idx >= 0 {
			return nil, fmt.Errorf("%q SDK not found, but appears in workshop definition", latest.Sdks[idx].Name)
		}
	}

	return validateSdkMeta(a.project.ProjectId, latest, sdks)
}

func (a *artifactFinder) findTrySdk(ctx context.Context, base, sk string) (*sdk.Meta, error) {
	trydir := workshop.TrySdkDir(a.userDataDir, sk)
	root, err := os.OpenRoot(trydir)
	if err != nil {
		return nil, fmt.Errorf("%q SDK not found: %w", "try-"+sk, err)
	}
	defer root.Close()

	file, filename, err := findTrySdkFile(root, sk, arch.DpkgArchitecture(), base)
	if err != nil {
		return nil, fmt.Errorf("%q SDK not found: %w", "try-"+sk, err)
	}
	defer file.Close()

	digest, sdkYaml, err := readTrySdkMetadata(root, filename)
	if err != nil {
		return nil, fmt.Errorf("invalid %q SDK: %w", "try-"+sk, err)
	}

	volumes, err := a.volumes(ctx)
	if err != nil {
		return nil, err
	}

	minRevision := sdk.Revision{}
	for _, volume := range volumes {
		if volume.Name != sk || !volume.Revision.Local() {
			continue
		}
		if volume.Revision.N < minRevision.N {
			minRevision = volume.Revision
		}

		if volume.Sha3_384 == digest {
			meta := volume.Meta
			meta.Source = sdk.TrySource
			return &meta, nil
		}
	}

	revision := sdk.Revision{N: minRevision.N - 1}
	meta := sdk.Meta{
		Setup: sdk.Setup{
			Name:     sk,
			Source:   sdk.TrySource,
			Revision: revision,
			Sha3_384: digest,
		},
		SdkYAML: sdkYaml,
	}
	if err := a.importSdk(ctx, meta, file); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (a *artifactFinder) volumes(ctx context.Context) ([]workshop.SdkVolume, error) {
	if a.sdkVolumes != nil {
		// The state is locked, preventing other launches and refreshes
		// from calling ImportSdk, so it's safe to reuse sdkVolumes.
		return a.sdkVolumes, nil
	}

	sdks, err := a.backend.Sdks(ctx)
	if err != nil {
		return nil, err
	}
	if sdks == nil {
		sdks = []workshop.SdkVolume{}
	}
	a.sdkVolumes = sdks
	return sdks, nil
}

func (a *artifactFinder) importSdk(ctx context.Context, meta sdk.Meta, tarball *os.File) error {
	if err := a.backend.ImportSdk(ctx, meta, tarball); err != nil {
		return err
	}
	a.sdkVolumes = append(a.sdkVolumes, workshop.SdkVolume{Meta: meta, Workshops: make(map[string][]string)})
	return nil
}

func findTrySdkFile(root *os.Root, sk, arch, base string) (*os.File, string, error) {
	filenames := []string{
		fmt.Sprintf("%s_%s_%s.sdk", sk, arch, base),
		fmt.Sprintf("%s_%s.sdk", sk, arch),
		fmt.Sprintf("%s_all_%s.sdk", sk, base),
		fmt.Sprintf("%s_all.sdk", sk),
	}
	var firstErr error
	for _, filename := range filenames {
		file, err := root.Open(filename)
		if err == nil {
			return file, filename, nil
		}
		firstErr = cmp.Or(firstErr, err)
	}
	return nil, "", prependRootPath(root, firstErr)
}

func readTrySdkMetadata(root *os.Root, filename string) (string, string, error) {
	content, err := root.ReadFile(filename + ".sha3-384")
	if err != nil {
		return "", "", prependRootPath(root, err)
	}
	digest := strings.TrimSpace(string(content))

	content, err = root.ReadFile(filename + ".yaml")
	if err != nil {
		return "", "", prependRootPath(root, err)
	}
	meta := string(content)

	return digest, meta, nil
}

func prependRootPath(root *os.Root, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		pathErr.Path = filepath.Join(root.Name(), pathErr.Path)
	}
	return err
}

func (a *artifactFinder) findInProjectSdk(w, sk string, installed sdk.Revision) (*sdk.Meta, error) {
	sdkdir := workshop.ProjectSdkPath(a.project.Path, sk)
	return a.commitRevision(w, sk, sdk.ProjectSource, sdkdir, installed)
}

func (a *artifactFinder) maybeFindSketchSdk(action, w string, installed sdk.Revision) (*sdk.Meta, error) {
	sketchdir := workshop.SketchSdkCurrent(a.userDataDir, a.project.ProjectId, w)

	recs, err := os.ReadDir(sketchdir)
	// no Sketch SDK exists for the workshop and it is not an error.
	if (err == nil && len(recs) == 0) || osutil.IsDirNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if action == "launch" {
		// Old sketches might exist because the snap remove hook doesn't remove them.
		// No workshop exists currently; including the sketch SDK would be unexpected.
		// We remove it (but keep the stash) to prevent future refreshes from including it.
		if err = os.RemoveAll(sketchdir); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return a.commitRevision(w, sdk.Sketch, sdk.SketchSource, sketchdir, installed)
}

func (a *artifactFinder) commitRevision(w, sk string, source sdk.Source, path string, installed sdk.Revision) (*sdk.Meta, error) {
	target := workshop.LocalSdkDir(a.userDataDir, a.project.ProjectId, w, sk)
	revision, digest, err := sdk.CommitRevision(a.user, path, target, installed)
	if err != nil {
		return nil, err
	}

	sdkYaml, err := os.ReadFile(filepath.Join(target, digest, "meta", "sdk.yaml"))
	if err != nil {
		return nil, fmt.Errorf("invalid %q SDK: %w", sk, err)
	}
	if source == sdk.ProjectSource {
		if err := sdk.ValidateYaml(bytes.NewReader(sdkYaml)); err != nil {
			return nil, fmt.Errorf("invalid %q SDK: %w", sk, err)
		}
	}

	setup := sdk.Setup{
		Name:     sk,
		Source:   source,
		Revision: revision,
		Sha3_384: digest,
	}
	return &sdk.Meta{Setup: setup, SdkYAML: string(sdkYaml)}, nil
}

func validateSdkMeta(projectId string, file *workshop.File, sdks []sdk.Meta) ([]sdk.Setup, error) {
	setups := make([]sdk.Setup, 0, len(sdks))
	for _, s := range sdks {
		if err := workshop.ValidateSdkInfo(projectId, file.Name, file.Base, s.Name, s.SdkYAML); err != nil {
			return nil, err
		}
		setups = append(setups, s.Setup)
	}
	return setups, nil
}

func sdkInstallOrder(file *workshop.File) []string {
	installOrder := make([]string, 1, len(file.Sdks)+2)
	installOrder[0] = sdk.System.String()
	for _, sk := range file.Sdks {
		if !workshop.IsImplicitSdk(sk.Name) {
			installOrder = append(installOrder, sk.Name)
		}
	}
	return append(installOrder, sdk.Sketch)
}

func ordered(order []string, setups ...[]sdk.Setup) []sdk.Setup {
	ordered := make([]sdk.Setup, 0, len(order))

	for _, sk := range order {
		for _, setup := range setups {
			contains := func(sp sdk.Setup) bool { return sk == sp.Name }

			idx := slices.IndexFunc(setup, contains)
			if idx != -1 {
				ordered = append(ordered, setup[idx])
				break
			}
		}
	}
	return ordered
}
