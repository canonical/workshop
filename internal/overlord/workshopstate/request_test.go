package workshopstate_test

import (
	"bytes"
	"context"
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/fsutil"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/workshop/fakebackend"
)

type requestSuite struct {
	state   *state.State
	project workshop.Project
	backend workshop.Backend
	mgr     *workshopstate.WorkshopManager
	ctx     context.Context
}

var _ = check.Suite(&requestSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *requestSuite) SetUpTest(c *check.C) {
	var err error
	s.state = state.New(nil)
	s.ctx = context.WithValue(context.Background(), workshop.ContextUser, "testuser")

	s.backend, err = fakebackend.New(c.MkDir())
	c.Assert(err, check.IsNil)
	workshop.ReplaceBackend(s.state, s.backend)
	s.mgr = workshopstate.New(s.state, state.NewTaskRunner(s.state))
	project, _, err := s.backend.CreateOrLoadProject(s.ctx, c.MkDir())
	c.Assert(err, check.IsNil)
	s.project = *project
	s.ctx = context.WithValue(s.ctx, workshop.ContextProjectId, s.project.ProjectId)
}

var workshopTemplate = `name: %s
base: ubuntu@20.04
sdks:
  {{- range . }}
  - name: {{ .Name}}
    channel: {{.Channel}}
  {{- end }}
`

var sdkTemplate = `name: %s
base: ubuntu@20.04
`

func (s *requestSuite) writeSDKMetaFile(c *check.C, fs fsutil.Fs, name, yaml string) {
	sdkPath := sdk.SdkMetaDir(name)
	c.Assert(fs.MkdirAll(sdkPath, 0o755), check.IsNil)
	metaPath := filepath.Join(sdkPath, "sdk.yaml")
	c.Assert(fs.WriteFile(metaPath, []byte(yaml), 0o644), check.IsNil)
}

func (s *requestSuite) launchWorkshopWithSDKs(c *check.C, ws string, sdks []workshop.SdkRecord) {
	t, err := template.New("workshop").Parse(fmt.Sprintf(workshopTemplate, ws))
	c.Assert(err, check.IsNil)

	workshopFile := bytes.NewBuffer([]byte{})
	t.Execute(workshopFile, sdks)

	path := workshop.Filepath(s.project.Path, ws)

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	c.Assert(err, check.IsNil)

	err = os.WriteFile(path, workshopFile.Bytes(), 0o644)
	c.Assert(err, check.IsNil)

	wf := workshop.File{Name: ws, Base: "ubuntu@20.04", Sdks: sdks}
	image := workshop.BaseImage{Name: wf.Base, Fingerprint: "fakeimage123"}
	err = s.backend.LaunchOrRebuildWorkshop(s.ctx, &wf, image)
	c.Assert(err, check.IsNil)

	w, err := s.backend.Workshop(s.ctx, ws)
	c.Assert(err, check.IsNil)

	wfs, err := s.backend.WorkshopFs(s.ctx, w.Name)
	c.Assert(err, check.IsNil)
	defer wfs.Close()

	for _, sd := range sdks {
		sdkYaml := fmt.Sprintf(sdkTemplate, sd.Name)
		s.writeSDKMetaFile(c, wfs, sd.Name, sdkYaml)
		digest := sha3.Sum384([]byte(sdkYaml))
		setup := sdk.Setup{
			Name:     sd.Name,
			Channel:  sd.Channel,
			Source:   sdk.StoreSource,
			Revision: sdk.R(1),
			Sha3_384: hex.EncodeToString(digest[:]),
		}
		c.Assert(w.AddSdk(s.ctx, setup), check.IsNil)
	}
}

func (s *requestSuite) TestStartMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts, err := workshopstate.StartManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(err, check.IsNil)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "start-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "start-workshop")
}

func (s *requestSuite) TestStopMany(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	ts, err := workshopstate.StopManyImpl(s.state, []string{"ws-1", "ws-2"}, s.project)
	c.Assert(err, check.IsNil)
	c.Assert(ts, check.HasLen, 2)
	c.Assert(ts[0].Tasks()[0].Kind(), check.Equals, "stop-workshop")
	c.Assert(ts[1].Tasks()[0].Kind(), check.Equals, "stop-workshop")

	var force bool
	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)

	ts[0].Tasks()[0].Get("force", &force)
	c.Assert(force, check.Equals, false)
}

func (s *requestSuite) TestRemountSuccess(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	sdks := []workshop.SdkRecord{
		{Name: "sdk-1", Channel: "latest/stable"},
	}
	source := c.MkDir()

	s.launchWorkshopWithSDKs(c, "ws-1", sdks)

	ts, err := s.mgr.Remount(s.ctx, s.state, plug, source)
	c.Assert(err, check.IsNil)
	c.Assert(ts.Tasks(), check.HasLen, 1)

	var w string
	var p workshop.Project
	task := ts.Tasks()[0]
	c.Assert(task.Get("workshop", &w), check.IsNil)
	c.Assert(task.Get("project", &p), check.IsNil)
	c.Assert(task.Summary(), check.Equals, `Remount "ws-1/sdk-1:plug"`)
	c.Assert(w, check.Equals, "ws-1")
	c.Assert(p, check.DeepEquals, s.project)

	var plugRef sdk.PlugRef
	var src string
	c.Assert(task.Get("plug", &plugRef), check.IsNil)
	c.Assert(plugRef, check.DeepEquals, plug)
	c.Assert(task.Get("host-source", &src), check.IsNil)
	c.Assert(src, check.Equals, source)
}

func (s *requestSuite) TestRemountWorkshopNotReady(c *check.C) {
	s.state.Lock()
	defer s.state.Unlock()

	plug := sdk.PlugRef{ProjectId: s.project.ProjectId, Workshop: "ws-1", Sdk: "sdk-1", Name: "plug"}
	sdks := []workshop.SdkRecord{
		{Name: "sdk-1", Channel: "latest/stable"},
	}

	s.launchWorkshopWithSDKs(c, "ws-1", sdks)

	// pretend there is another change running that would conflict with this one.
	change := s.state.NewChange("refresh", "test")
	task := s.state.NewTask("task", "test")
	task.Set("workshop", "ws-1")
	change.AddTask(task)
	change.Set("project-id", s.project.ProjectId)

	_, err := s.mgr.Remount(s.ctx, s.state, plug, c.MkDir())
	c.Assert(err, check.ErrorMatches, `cannot remount "ws-1/sdk-1:plug": other changes in progress`)
}
