package workshop_test

import (
	"os"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/arch"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
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
	c.Assert(os.MkdirAll(filepath.Dir(path), 0o755), check.IsNil)
	c.Assert(os.WriteFile(path, []byte(content), 0o644), check.IsNil)
}

func (f *workshopSuite) TestValidateSdkSyntax(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `incorrect yaml: -
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `invalid "test-sdk-1" SDK definition: yaml: block sequence entries are not allowed in this context`)
}

func (f *workshopSuite) TestValidateSdkName(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `name: sdk-1
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `SDK must be named "test-sdk-1" \(now: "sdk-1"\)`)
}

func (f *workshopSuite) TestValidateSdkBase(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()

	wpath := filepath.Join(f.project.Path, "workshop.yaml")
	writeFile(c, wpath, string(workshopyaml))
	file, err := workshop.ReadWorkshop(wpath)
	c.Assert(err, check.IsNil)

	sdkYaml := `name: test-sdk-1
base: ubuntu@24.04
`
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file, "test-sdk-1", sdkYaml)
	c.Check(err, check.ErrorMatches, `"test-sdk-1" SDK has "ubuntu@24.04" base; required: "ubuntu@22.04"`)
}

func (f *workshopSuite) TestValidateSdkArchitecture(c *check.C) {
	defer sdk.MockSanitizePlugsSlots(func(sdkInfo *sdk.Info) {})()
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
	err = workshop.ValidateSdkInfo(f.project.ProjectId, file, "test-sdk-1", sdkYaml)
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
				Name:     "test-sdk-1",
				Channel:  "latest/stable",
				Revision: sdk.R(1),
				Sha3_384: "84fa7f3d2e556fe410132260dfacb67d4cbbfb36ecfc26dfcef3f247524122d58c992902def9b52b88da0d6ec0efad05",
			},
		},
		"test-sdk-2": {
			Setup: sdk.Setup{
				Name:     "test-sdk-2",
				Channel:  "latest/edge",
				Revision: sdk.R(1),
				Sha3_384: "d4089378c26310627268153caa216240311f2a3193c778e96ed6dd895dc10c82db50f4f39676b29d23d9813b21e14b9b",
			},
		},
		"system": {
			Setup: sdk.Setup{
				Name:     "system",
				Source:   sdk.SystemSource,
				Revision: sdk.R(1),
				Sha3_384: "6b499970ebf370d4dbc4e9a005c042dee003c19a9420a78944bcbf32653d257f80f7c56bad55b4c967dca68a1ea92be7",
			},
		},
		"sketch": {
			Setup: sdk.Setup{
				Name:     "sketch",
				Source:   sdk.SketchSource,
				Revision: sdk.R(-3),
				Sha3_384: "dd4b5a4cba8539e858e5fdcc318e46d9a2940439b0d8e7bd9c6bfc8b474f410d91aee43f5d4e18cb2c1b7dbaaba06fc3",
			},
		},
	}

	sdks := w.SdksByInstallOrder()
	c.Assert(sdks, check.DeepEquals, []workshop.SdkInstallation{w.Sdks["system"], w.Sdks["test-sdk-1"], w.Sdks["test-sdk-2"], w.Sdks["sketch"]})
}
