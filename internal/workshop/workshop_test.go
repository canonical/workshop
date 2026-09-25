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

package workshop_test

import (
	"context"
	"os"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type workshopSuite struct {
	project workshop.Project
}

var _ = check.Suite(&workshopSuite{})

var workshopyaml = []byte(`name: test-workshop
base: ubuntu@22.04
sdks:
  - name: test-sdk-1
    channel: latest/stable
  - name: test-sdk-2
    channel: latest/stable
  - name: system
`)

func (f *workshopSuite) SetUpTest(c *check.C) {
	f.project = workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "b8639dea",
	}
}

func writeFile(c *check.C, path string, content string) {
	c.Assert(os.MkdirAll(filepath.Dir(path), 0755), check.IsNil)
	c.Assert(os.WriteFile(path, []byte(content), 0644), check.IsNil)
}

// TestExecArgsEffectiveCommand prepends Workshop-managed command prefixes
// without mutating the stored user command.
func (f *workshopSuite) TestExecArgsEffectiveCommand(c *check.C) {
	args := workshop.ExecArgs{
		Command:       []string{"echo", "foo"},
		CommandPrefix: []string{"sudo", "--"},
	}

	command := args.EffectiveCommand()

	c.Check(command, check.DeepEquals, []string{"sudo", "--", "echo", "foo"})
	command[0] = "changed"
	c.Check(args.CommandPrefix, check.DeepEquals, []string{"sudo", "--"})
	c.Check(args.Command, check.DeepEquals, []string{"echo", "foo"})
}

// TestExecArgsEffectiveCommandNoPrefix returns the user command unchanged when
// no Workshop-managed command prefix is supplied.
func (f *workshopSuite) TestExecArgsEffectiveCommandNoPrefix(c *check.C) {
	args := workshop.ExecArgs{
		Command: []string{"echo", "foo"},
	}

	command := args.EffectiveCommand()

	c.Check(command, check.DeepEquals, []string{"echo", "foo"})
	command[0] = "changed"
	c.Check(args.Command, check.DeepEquals, []string{"echo", "foo"})
}

func (f *workshopSuite) TestValidateSdkSyntax(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `incorrect yaml: -
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file.Name, file.Base, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `invalid "test-sdk-1" SDK definition: yaml: block sequence entries are not allowed in this context`)
}

func (f *workshopSuite) TestValidateSdkName(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `name: sdk-1
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file.Name, file.Base, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `SDK must be named "test-sdk-1" \(now: "sdk-1"\)`)
}

func (f *workshopSuite) TestValidateSdkBase(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `name: test-sdk-1
base: ubuntu@24.04
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file.Name, file.Base, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `"test-sdk-1" SDK has "ubuntu@24.04" base; required: "ubuntu@22.04"`)
}

func (f *workshopSuite) TestValidateSdkArchitecture(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()
	architecture := arch.ArchitectureType(arch.DpkgArchitecture())
	arch.SetArchitecture("mock32")
	defer arch.SetArchitecture(architecture)

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `name: test-sdk-1
architecture: mock64
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file.Name, file.Base, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `"test-sdk-1" SDK has "mock64" architecture; required: "mock32" or "all"`)
}

func (f *workshopSuite) TestSdkSetupsByInstallOrder(c *check.C) {
	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	w := workshop.Workshop{File: file, Name: "test-workshop"}
	w.Sdks = map[string]workshop.SdkInstallation{
		"test-sdk-1": {
			Setup: sdk.Setup{
				Name:      "test-sdk-1",
				PackageID: "Q03jfNEDoolt4eMu3ouCzZZG8IO3fNUO",
				Channel:   "latest/stable",
				Revision:  sdk.R(1),
				Sha3_384:  "84fa7f3d2e556fe410132260dfacb67d4cbbfb36ecfc26dfcef3f247524122d58c992902def9b52b88da0d6ec0efad05",
			},
			InstallOrder: 2,
		},
		"test-sdk-2": {
			Setup: sdk.Setup{
				Name:      "test-sdk-2",
				PackageID: "iCJybjjWd2n48hKoMdjGEIWwA3i2TmX7",
				Channel:   "latest/edge",
				Revision:  sdk.R(1),
				Sha3_384:  "d4089378c26310627268153caa216240311f2a3193c778e96ed6dd895dc10c82db50f4f39676b29d23d9813b21e14b9b",
			},
			InstallOrder: 3,
		},
		"system": {
			Setup: sdk.Setup{
				Name:     "system",
				Source:   sdk.SystemSource,
				Revision: sdk.R(1),
				Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			},
			InstallOrder: 1,
		},
		"sketch": {
			Setup: sdk.Setup{
				Name:     "sketch",
				Source:   sdk.SketchSource,
				Revision: sdk.R(-3),
				Sha3_384: "dd4b5a4cba8539e858e5fdcc318e46d9a2940439b0d8e7bd9c6bfc8b474f410d91aee43f5d4e18cb2c1b7dbaaba06fc3",
			},
			InstallOrder: 4,
		},
	}

	sdks := w.SdksByInstallOrder()
	c.Assert(sdks, check.DeepEquals, []workshop.SdkInstallation{w.Sdks["system"], w.Sdks["test-sdk-1"], w.Sdks["test-sdk-2"], w.Sdks["sketch"]})
}

// newTrySdkWorkshop builds a minimal installed *workshop.Workshop with a
// single try-source SDK, for exercising Workshop.SdkPlugsAndSlots without
// going through a full install flow.
func (f *workshopSuite) newTrySdkWorkshop(c *check.C, sdkYaml string, sdkRecord workshop.SdkRecord) *workshop.Workshop {
	backend, err := fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)

	setup := sdk.Setup{Name: sdkRecord.Name, Source: sdk.TrySource, Revision: sdk.R(1)}
	volumeName := sdk.VolumeName(setup.Name, setup.Revision)
	backend.Volumes[volumeName] = fakebackend.FakeVolume{Kind: "sdk"}
	backend.SdkVolumes[volumeName] = sdk.Meta{Setup: setup, SdkYAML: sdkYaml}

	return &workshop.Workshop{
		Backend: backend,
		Project: f.project,
		Name:    "ws",
		File: &workshop.File{
			Name: "ws",
			Base: "ubuntu@22.04",
			Sdks: []workshop.SdkRecord{sdkRecord},
		},
		Sdks: map[string]workshop.SdkInstallation{
			sdkRecord.Name: {Setup: setup},
		},
	}
}

// TestSdkPlugsAndSlotsAddsWorkshopSlot verifies that a slot declared for an
// SDK in the workshop file is merged alongside the SDK's own declared slots.
func (f *workshopSuite) TestSdkPlugsAndSlotsAddsWorkshopSlot(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	sdkYaml := `name: sdk
base: ubuntu@22.04
slots:
  training:
    interface: mount
    workshop-source: /project
`
	w := f.newTrySdkWorkshop(c, sdkYaml, workshop.SdkRecord{
		Name: "sdk",
		Slots: map[string]any{
			"cache": map[string]any{
				"interface":       "mount",
				"workshop-source": "/var/cache",
			},
		},
	})

	info, plugs, slots, badInterfaces, err := w.SdkPlugsAndSlots(context.Background(), "sdk")
	c.Assert(err, check.IsNil)
	c.Assert(badInterfaces, check.HasLen, 0)
	c.Assert(plugs, check.HasLen, 0)
	c.Assert(slots, check.HasLen, 2)
	c.Assert(*slots["training"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "training",
		Interface: "mount",
		Attrs:     map[string]any{"workshop-source": "/project"},
	})
	c.Assert(*slots["cache"], check.DeepEquals, sdk.SlotInfo{
		Sdk:       info.Ref(),
		Name:      "cache",
		Interface: "mount",
		Attrs:     map[string]any{"workshop-source": "/var/cache"},
	})
}

// TestSdkPlugsAndSlotsAlreadyExistingSlotFails verifies that a workshop-file
// slot override colliding with a name the SDK already declares itself fails.
func (f *workshopSuite) TestSdkPlugsAndSlotsAlreadyExistingSlotFails(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	sdkYaml := `name: sdk
base: ubuntu@22.04
slots:
  training:
    interface: mount
    workshop-source: /project
`
	w := f.newTrySdkWorkshop(c, sdkYaml, workshop.SdkRecord{
		Name: "sdk",
		Slots: map[string]any{
			"training": map[string]any{
				"workshop-source": "/data",
			},
		},
	})

	_, _, _, _, err := w.SdkPlugsAndSlots(context.Background(), "sdk")
	c.Assert(err, check.ErrorMatches, `cannot add slot "training" to "sdk" SDK: already exists`)
}

// TestSdkPlugsAndSlotsAlreadyExistingPlugFails verifies that a workshop-file
// plug override colliding with a name the SDK already declares itself fails.
func (f *workshopSuite) TestSdkPlugsAndSlotsAlreadyExistingPlugFails(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(plugs map[string]*sdk.PlugInfo, slots map[string]*sdk.SlotInfo) map[string]string { return nil })()

	sdkYaml := `name: sdk
base: ubuntu@22.04
plugs:
  training:
    interface: mount
    workshop-target: /project
`
	w := f.newTrySdkWorkshop(c, sdkYaml, workshop.SdkRecord{
		Name: "sdk",
		Plugs: map[string]workshop.PlugOrBind{
			"training": {Plug: map[string]any{
				"workshop-target": "/data",
			}},
		},
	})

	_, _, _, _, err := w.SdkPlugsAndSlots(context.Background(), "sdk")
	c.Assert(err, check.ErrorMatches, `cannot add plug "training" to "sdk" SDK: already exists`)
}
