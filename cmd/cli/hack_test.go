package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/testutil"
)

type workshopHack struct {
	BaseWorkshopSuite
	prjDir string
	prjId  string
	user   *user.User
}

var _ = check.Suite(&workshopHack{})

var simpleHackMeta = `name: hack
base: ubuntu@22.04
`

func (m *workshopHack) SetUpTest(c *check.C) {
	m.BaseWorkshopSuite.SetUpTest(c)

	m.prjDir = c.MkDir()
	m.prjId = "42424242"
	var err error
	m.user, err = osutil.UserMaybeSudoUser()
	c.Assert(err, check.IsNil)
}

func (m *workshopHack) TearDownTest(c *check.C) {
	hackdir := sdk.ProjectHackSdkDir(m.user.HomeDir, m.prjId)
	err := os.RemoveAll(hackdir)
	c.Assert(err, check.IsNil)
}

func (m *workshopHack) mockMinimalHackSdk(c *check.C, current bool, meta []byte) (metapath string, hookspath string) {
	var rootdir string
	if current {
		rootdir = sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	} else {
		rootdir = sdk.WorkshopHackSdkStored(m.user.HomeDir, m.prjId, "ws")
	}
	metadir := filepath.Join(rootdir, "meta")
	err := os.MkdirAll(metadir, 0755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), meta, 0644)
	c.Assert(err, check.IsNil)

	hooksdir := filepath.Join(rootdir, "hooks")
	return metadir, hooksdir
}

func (m *workshopHack) mockHackHappyRefreshPath(c *check.C, refreshname string, refreshMode string) {
	n := 0
	workshop := "ws"
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithContent)
		case 4:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "refresh",
				"names": []interface{}{refreshname}, "options": map[string]interface{}{"refresh-mode": refreshMode}})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 5:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockReadyChangeJSON)
		default:
			c.Errorf("expected 5 calls, now on %d", n)
		}
	})
}

func (m *workshopHack) TestHackSuccess(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")

	hackContent := `name: hack
base: ubuntu@22.04
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		return []byte(inContent), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	current := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "meta", "sdk.yaml"), testutil.FileEquals, hackContent)
}

func (m *workshopHack) TestHackEditExistingMeta(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")

	metadir := filepath.Join(sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws"), "meta")
	err := os.MkdirAll(metadir, 0755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(simpleHackMeta), 0644)
	c.Assert(err, check.IsNil)

	hackContent := `name: hack
base: ubuntu@22.04
plugs:
  gpu:
    interface: gpu
`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		err := os.WriteFile(inPath, []byte(hackContent), 0644)
		c.Assert(err, check.IsNil)
		return []byte(hackContent), nil
	})
	defer restore()

	err = cmd.Run(cmd.Command(), []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(filepath.Join(metadir, "sdk.yaml"), testutil.FileEquals, hackContent)
}

func (m *workshopHack) TestHackCreateHookMinimalMetaCreated(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")
	metadir := filepath.Join(sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws"), "meta")
	err := os.MkdirAll(metadir, 0755)
	c.Assert(err, check.IsNil)
	err = os.WriteFile(filepath.Join(metadir, "sdk.yaml"), []byte(simpleHackMeta), 0644)
	c.Assert(err, check.IsNil)

	hookcontent := `apt-get update`
	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		return []byte(hookcontent), nil
	})
	defer restore()

	err = cmd.Run(cmd.Command(), []string{"ws", "setup-base"})
	c.Assert(err, check.IsNil)

	current := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "hooks", "setup-base"), testutil.FileEquals, hookcontent)
	c.Assert(filepath.Join(current, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}

func (m *workshopHack) TestHackCreateHookMetaExists(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")

	restore := MockTextEditor(func(inPath string, inContent []byte) ([]byte, error) {
		return []byte(inContent), nil
	})
	defer restore()

	err := cmd.Run(cmd.Command(), []string{"ws", "setup-base"})
	c.Assert(err, check.IsNil)

	current := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "hooks", "setup-base"), testutil.FileEquals, "")
	c.Assert(filepath.Join(current, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}

func (m *workshopHack) TestHackEditHookUnknown(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}}

	n := 0
	workshop := "ws"
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithContent)
		default:
			c.Errorf("expected 2 calls, now on %d", n)
		}
	})

	err := cmd.Run(cmd.Command(), []string{"ws", "unknown-hook"})
	c.Assert(err, check.ErrorMatches, `cannot hack: unknown SDK hook "unknown-hook"; valid names are setup-base, save-state, restore-state, check-health`)
}

func (m *workshopHack) TestHackDropRestoreIncompatible(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, drop: true, restore: true}

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot hack: '--drop' incompatible with '--replace'`)
}

func (m *workshopHack) TestHackDropRestoreSingleWorkshop(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, drop: true, restore: false}

	err := cmd.Run(nil, []string{"ws", "ws-1"})
	c.Assert(err, check.ErrorMatches, `cannot hack: '--drop' and '--replace' require a single workshop name`)
}

func (m *workshopHack) TestHackDrop(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, drop: true}
	restore := sdk.WorkshopHackSdkStored(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(restore, "meta", "sdk.yaml"), testutil.FileAbsent)

	m.mockHackHappyRefreshPath(c, "ws", "transactional")
	metadir, _ := m.mockMinimalHackSdk(c, true, []byte(simpleHackMeta))

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)

	c.Assert(metadir, testutil.FileAbsent)
	c.Assert(filepath.Join(restore, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}

func (m *workshopHack) TestHackDropRevertOnFail(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, drop: true}

	n := 0
	workshop := "ws"
	m.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1, 3:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, "/v1/projects")
			r := fmt.Sprintf(`{"type": "sync", "result": {"id":"%s","path":"%s"}}`, m.prjId, m.prjDir)
			fmt.Fprintln(w, r)
		case 2:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops/%s", m.prjId, workshop))
			w.WriteHeader(200)
			fmt.Fprintln(w, mockWorkshopWithContent)
		case 4:
			c.Check(r.Method, check.Equals, "POST")
			c.Assert(r.URL.Path, check.Equals, fmt.Sprintf("/v1/projects/%s/workshops", m.prjId))
			c.Check(DecodedRequestBody(c, r), check.DeepEquals, map[string]interface{}{"action": "refresh",
				"names": []interface{}{workshop}, "options": map[string]interface{}{"refresh-mode": "transactional"}})
			w.WriteHeader(202)
			fmt.Fprintln(w, `{"type":"async", "change": "42", "status-code": 202}`)
		case 5:
			c.Check(r.Method, check.Equals, "GET")
			c.Assert(r.URL.Path, check.Equals, "/v1/changes/42")
			fmt.Fprintln(w, mockChangeWithError)
		default:
			c.Errorf("expected 5 calls, now on %d", n)
		}
	})

	metadir, _ := m.mockMinimalHackSdk(c, true, []byte(simpleHackMeta))

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.NotNil)

	c.Assert(filepath.Join(metadir, "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
	restore := sdk.WorkshopHackSdkStored(m.user.HomeDir, m.prjId, "ws")
	recs, err := os.ReadDir(restore)
	c.Assert(recs, check.HasLen, 0)
	c.Assert(err, check.IsNil)
}

func (m *workshopHack) TestHackRestoreOK(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, restore: true}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")
	m.mockMinimalHackSdk(c, false, []byte(simpleHackMeta))

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)

	current := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}

func (m *workshopHack) TestHackRestoreNoStoredHack(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, restore: true}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.ErrorMatches, `cannot restore: no stored 'hack' SDK found`)
}

func (m *workshopHack) TestHackRestoreSwaps(c *check.C) {
	cmd := &CmdHack{root: &CmdRoot{}, restore: true}

	m.mockHackHappyRefreshPath(c, "ws/hack", "wait-on-error")
	stored := `name: hack
base: ubuntu@22.04
plugs:
  gpu:
    interface: gpu
`
	// current
	m.mockMinimalHackSdk(c, true, []byte(simpleHackMeta))
	// stored
	m.mockMinimalHackSdk(c, false, []byte(stored))

	err := cmd.Run(nil, []string{"ws"})
	c.Assert(err, check.IsNil)

	current := sdk.WorkshopHackSdkCurrent(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(current, "meta", "sdk.yaml"), testutil.FileEquals, stored)

	restore := sdk.WorkshopHackSdkStored(m.user.HomeDir, m.prjId, "ws")
	c.Assert(filepath.Join(restore, "meta", "sdk.yaml"), testutil.FileEquals, simpleHackMeta)
}
