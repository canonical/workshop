package workshop_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/afero"
	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

type workshopFile struct {
	fs      afero.Fs
	project workshop.Project
}

var _ = check.Suite(&workshopFile{})

func TestWorkshop(t *testing.T) { check.TestingT(t) }

func TestMain(m *testing.M) {
	// Ensure consistent file permissions for workshopSuite, workshopFsSuite and localSdk.
	syscall.Umask(0002)
	m.Run()
}

func (f *workshopFile) SetUpTest(c *check.C) {
	f.fs = afero.NewMemMapFs()

	f.project = workshop.Project{
		Path:      c.MkDir(),
		ProjectId: "b8639dea",
	}
}

func (f *workshopFile) createWFile(c *check.C, name, yaml string, checkArgs ...interface{}) {
	path := workshop.Filepath(f.project.Path, name)

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil, checkArgs...)

	err = os.WriteFile(path, []byte(yaml), 0644)
	c.Assert(err, check.IsNil, checkArgs...)
}

func (f *workshopFile) createSingleWFile(c *check.C, filename, yaml string) {
	err := os.MkdirAll(f.project.Path, os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(filepath.Join(f.project.Path, filename), []byte(yaml), 0644)
	c.Assert(err, check.IsNil)
}

func (f *workshopFile) TestWorkshopFileParse(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: huggingface
    channel: latest/stable
  - name: cuda
    channel: latest/edge
  - name: zookeeper
    channel: latest/candidate
  - name: automotive
    channel: latest/beta
scripts:
  oneline: echo one line
  multiline: |
    echo multi
    echo line
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.Equals, nil)
	c.Assert(file.Name, check.Equals, "xbert-gpu")
	c.Assert(file.Base, check.Equals, "ubuntu@20.04")
	c.Assert(file.Sdks[0].Name, check.Equals, "huggingface")
	c.Assert(file.Sdks[0].Channel, check.Equals, "latest/stable")
	c.Assert(file.Sdks[1].Name, check.Equals, "cuda")
	c.Assert(file.Sdks[1].Channel, check.Equals, "latest/edge")
	c.Assert(file.Sdks[2].Name, check.Equals, "zookeeper")
	c.Assert(file.Sdks[2].Channel, check.Equals, "latest/candidate")
	c.Assert(file.Sdks[3].Name, check.Equals, "automotive")
	c.Assert(file.Sdks[3].Channel, check.Equals, "latest/beta")
	lines := len(strings.Split(yaml, "\n"))
	skip := strings.Repeat("\n", lines-5)
	c.Assert(string(file.Scripts["oneline"]), check.Equals, skip+"echo one line\n")
	skip = strings.Repeat("\n", lines-3)
	c.Assert(string(file.Scripts["multiline"]), check.Equals, skip+"echo multi\necho line\n")
}

func (f *workshopFile) TestWorkshopFileSave(c *check.C) {
	fl := &workshop.File{
		Name: "test-workshop",
		Base: "ubuntu@22.04",
		Sdks: []workshop.SdkRecord{
			{Name: "one", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"plug": {Bind: &workshop.PlugRef{Sdk: "two", Name: "plug"}}}},
			{Name: "two", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"plug": {Bind: &workshop.PlugRef{Sdk: "one", Name: "plug"}}}},
		},
		Scripts: map[string]workshop.Script{
			"oneline":   "\n\n\necho one line\n",
			"multiline": "\n\n\n\n\necho multi\necho line\n",
		},
	}
	out, err := yaml.Marshal(fl)
	c.Assert(err, check.IsNil)
	c.Assert(string(out), check.Equals, `name: test-workshop
base: ubuntu@22.04
sdks:
    - name: one
      channel: latest/stable
      plugs:
        plug:
            bind: two:plug
    - name: two
      channel: latest/stable
      plugs:
        plug:
            bind: one:plug
scripts:
    multiline: |
        echo multi
        echo line
    oneline: echo one line
`)
}

func (f *workshopFile) TestSingleWorkshopFile(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
`
	f.createSingleWFile(c, "workshop.yaml", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file, check.DeepEquals, &workshop.File{Name: "xbert-gpu", Base: "ubuntu@20.04"})
}

func (f *workshopFile) TestSingleWorkshopFileAmbiguous(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
`
	f.createSingleWFile(c, "workshop.yaml", yaml)
	f.createSingleWFile(c, ".workshop.yaml", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	path := filepath.Join(f.project.Path, "workshop.yaml")
	message := fmt.Sprintf(`ambiguous file %q \(directory also contains ".workshop.yaml"\)`, path)
	c.Assert(err, check.ErrorMatches, message)
}

func (f *workshopFile) TestSingleWorkshopFileWrongName(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
`
	f.createSingleWFile(c, "workshop.yaml", yaml)
	file, err := f.project.Workshop("xbert")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `workshop "xbert" not found \(only found "xbert-gpu"\)`)
}

func (f *workshopFile) TestSingleWorkshopFileError(c *check.C) {
	path := filepath.Join(f.project.Path, "workshop.yaml")
	c.Assert(os.Mkdir(path, os.ModePerm), check.IsNil)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, ".*is a directory")
}

func (f *workshopFile) TestWorkshopFileDuplicate(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@22.04
`
	f.createSingleWFile(c, "workshop.yaml", yaml)
	f.createWFile(c, "xbert-gpu", yaml)

	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	path := filepath.Join(f.project.Path, "workshop.yaml")
	message := fmt.Sprintf(`multiple workshops found, but %q not in ".workshop" subdirectory`, path)
	c.Assert(err, check.ErrorMatches, message)
}

func (f *workshopFile) TestWorkshopNamesDifferent(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
`
	f.createWFile(c, "xbert", yaml)
	file, err := f.project.Workshop("xbert")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `"xbert-gpu" workshop file must be named "xbert-gpu.yaml" \(now: "xbert.yaml"\)`)
}

func (f *workshopFile) TestWorkshopInvalidName(c *check.C) {
	yaml := `name: 99-xbert
base: ubuntu@20.04
`
	f.createWFile(c, "99-xbert", yaml)
	file, err := f.project.Workshop("99-xbert")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `a workshop's name must: \(1\) start with a letter, \(2\) only include digits, lowercase letters, and hyphens joining them`)
}

func (f *workshopFile) TestWorkshopUnsupportedBase(c *check.C) {
	yaml := `name: xbert-gpu
base: foo@20.04
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `base "foo@20.04" not supported`)
}

func (f *workshopFile) TestWorkshopFileDuplicateSdks(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: cuda
    channel: latest/stable
  - name: cuda
    channel: latest/edge
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `"cuda" SDK must only be included once`)
}

func (f *workshopFile) TestWorkshopFileReservedNames(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: agent
    channel: latest/stable
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `"agent" is a reserved SDK name`)
	c.Assert(file, check.IsNil)
}

func (f *workshopFile) TestWorkshopUnsupportedChannel(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: cuda
    channel: latest/foo
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(file, check.IsNil)
	c.Assert(err, check.ErrorMatches, `unsupported channel "latest/foo" for "cuda" SDK`)
}

func (f *workshopFile) TestShortcuts(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      db: tunnel
      tunnel:
  - name: etl-sdk
    channel: latest/stable
    slots:
      dashboard: tunnel
      tunnel:
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file.Sdks, check.DeepEquals, []workshop.SdkRecord{
		{Name: "data-sdk", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"db": {Plug: "tunnel"}, "tunnel": {}}},
		{Name: "etl-sdk", Channel: "latest/stable", Slots: map[string]interface{}{"dashboard": "tunnel", "tunnel": nil}},
	})
}

func (f *workshopFile) TestBindPlug(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      cache:
        bind: etl-sdk:cache
  - name: etl-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: data-sdk:aux
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file.Sdks, check.DeepEquals, []workshop.SdkRecord{
		{Name: "data-sdk", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"cache": {Bind: &workshop.PlugRef{Sdk: "etl-sdk", Name: "cache"}}}},
		{Name: "etl-sdk", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"data": {Bind: &workshop.PlugRef{Sdk: "data-sdk", Name: "aux"}}}},
	})
}

func (f *workshopFile) TestPlugDefinedButNotBound(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      cache:
        attr1: val
  - name: etl-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: data-sdk:aux
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file.Sdks, check.DeepEquals, []workshop.SdkRecord{
		{Name: "data-sdk", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"cache": {Plug: map[string]interface{}{"attr1": "val"}}}},
		{Name: "etl-sdk", Channel: "latest/stable", Plugs: map[string]workshop.PlugOrBind{"data": {Bind: &workshop.PlugRef{Sdk: "data-sdk", Name: "aux"}}}},
	})
}

func (f *workshopFile) TestPlugDefinedAndBoundFails(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: data-sdk:aux
        attr2: val
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `plug is bound to "data-sdk:aux" and must not define other attributes`)
}

func (f *workshopFile) TestBindPlugNoSdk(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      cache:
        bind: no-sdk:cache
  - name: etl-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: data-sdk:cache
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot bind plug "no-sdk:cache": SDK "no-sdk" not found`)
}

func (f *workshopFile) TestBindPlugSystemSdk(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: system
  - name: etl-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: system:cache
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Check(err, check.ErrorMatches, `cannot bind to system SDK plug "system:cache"`)

	yaml = `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: system
    plugs:
      cache: 
        bind: etl-sdk:data
  - name: etl-sdk
    channel: latest/stable
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err = f.project.Workshop("xbert-gpu")
	c.Check(err, check.ErrorMatches, `cannot bind system SDK plug "system:cache"`)
}

func (f *workshopFile) TestBindPlugToItself(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      cache:
        bind: data-sdk:cache
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot bind plug "data-sdk:cache" to itself`)
}

func (f *workshopFile) TestBindPlugToBoundPlug(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: one
    channel: latest/stable
  - name: two
    channel: latest/stable
    plugs:
      data:
        bind: one:data
  - name: three
    channel: latest/stable
    plugs:
      data:
        bind: two:data
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot bind "two:data" to "one:data": plug "two:data" is already bound`)
}

func (f *workshopFile) TestBindPlugInvalidPlugRef(c *check.C) {
	templ := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: etl-sdk
    channel: latest/stable
    plugs:
      %s
`
	invalids := []string{
		`data:		
           bind: cache`,
		`data:
           bind: workshop/no-sdk:cache`,
		`data:
           bind: workshop:etl-sdk:cache`,
		`data:
           bind: etl-sdk`,
	}

	for _, ref := range invalids {
		f.createWFile(c, "xbert-gpu", fmt.Sprintf(templ, ref), check.Commentf(ref))
		_, err := f.project.Workshop("xbert-gpu")
		c.Assert(err, check.ErrorMatches, `.* is not a valid plug or slot reference.*`, check.Commentf(ref))
	}
}

func (f *workshopFile) TestBindToAlreadyBoundPlug(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      cache:
        bind: etl-sdk:cache
      aux:
        bind: etl-sdk:data
  - name: etl-sdk
    channel: latest/stable
    plugs:
      cache: 
        bind: etl-sdk:data
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot bind "etl-sdk:cache" to "etl-sdk:data": plug "etl-sdk:cache" is already bound`)
}

func (f *workshopFile) TestIndirectBindToAlreadyBoundPlug(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      data:
        bind: data-sdk:aux
      aux:
        bind: etl-sdk:cache
  - name: etl-sdk
    channel: latest/stable
    plugs:
      cache:
        bind: data-sdk:data
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot bind "data-sdk:aux" to "etl-sdk:cache": plug "data-sdk:aux" is already bound`)
}

func (f *workshopFile) TestHostSdkSlot(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: system
    slots:
      training-data:
        workshop-source: relative/path
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file.Sdks, check.DeepEquals, []workshop.SdkRecord{
		{Name: sdk.System.String(), Slots: map[string]interface{}{"training-data": map[string]interface{}{"workshop-source": "relative/path"}}}})
}

func (f *workshopFile) TestWorkshopConnectionsOK(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: data-sdk:data
    slot: system:mount
  - plug: etl-sdk:data
    slot: data-sdk:data-slot
`
	f.createWFile(c, "xbert-gpu", yaml)
	file, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
	c.Assert(file.Connections, testutil.DeepUnsortedMatches, []workshop.Connection{
		{PlugRef: workshop.PlugRef{Sdk: "data-sdk", Name: "data"}, SlotRef: workshop.SlotRef{Sdk: sdk.System.String(), Name: "mount"}},
		{PlugRef: workshop.PlugRef{Sdk: "etl-sdk", Name: "data"}, SlotRef: workshop.SlotRef{Sdk: "data-sdk", Name: "data-slot"}},
	})
}

func (f *workshopFile) TestWorkshopConnectionsInvalidRefs(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: data-sdk
    slot: system:mount
  - plug: etl-sdk:data
    slot: data-sdk:data-slot
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `"data-sdk" is not a valid plug or slot reference \(use <sdk>:<plug or slot>\)`)
}

func (f *workshopFile) TestWorkshopConnectionsSlotSdkNotInTheList(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: data-sdk:data
    slot: lost-sdk:mount
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot connect plug "data-sdk:data" to slot "lost-sdk:mount": workshop "xbert-gpu" has no SDK named "lost-sdk"`)
}

func (f *workshopFile) TestWorkshopConnectionsPlugSdkNotInTheList(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: lost-sdk:data
    slot: data-sdk:mount
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot connect plug "lost-sdk:data" to slot "data-sdk:mount": workshop "xbert-gpu" has no SDK named "lost-sdk"`)
}

func (f *workshopFile) TestWorkshopConnectionsImplicitHostSdkPlugSlot(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: system:data
    slot: system:mount
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.IsNil)
}

func (f *workshopFile) TestWorkshopConnectionsBoundPlugCannotBeConnected(c *check.C) {
	yaml := `name: xbert-gpu
base: ubuntu@20.04
sdks:
  - name: data-sdk
    channel: latest/stable
    plugs:
      data: 
        bind: etl-sdk:data
  - name: etl-sdk
    channel: latest/stable
connections:
  - plug: data-sdk:data
    slot: system:mount
  - plug: etl-sdk:data
    slot: data-sdk:data-slot
`
	f.createWFile(c, "xbert-gpu", yaml)
	_, err := f.project.Workshop("xbert-gpu")
	c.Assert(err, check.ErrorMatches, `cannot connect plug "data-sdk:data" to slot "system:mount": plug is bound`)
}
