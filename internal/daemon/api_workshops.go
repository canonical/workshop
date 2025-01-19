package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/canonical/x-go/strutil"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type actionOpts struct {
	Mode string `json:"mode"`
}

type workshopReq struct {
	Names   []string   `json:"names"`
	Action  string     `json:"action"`
	Options actionOpts `json:"options"`
}

type HealthCheckInfo struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
	Code      string    `json:"code,omitempty"`
}

type SdkInfo struct {
	Name        string           `json:"name"`
	Version     string           `json:"version,omitempty"`
	Channel     string           `json:"channel"`
	Revision    string           `json:"revision"`
	BuildTime   *time.Time       `json:"build-time,omitempty"`
	InstallTime *time.Time       `json:"install-time,omitempty"`
	Health      *HealthCheckInfo `json:"health-check,omitempty"`
	Mounts      []*Mount         `json:"mounts,omitempty"`
}

type Mount struct {
	Plug           interfaces.PlugRef `json:"plug"`
	WorkshopSource string             `json:"workshop-source,omitempty"`
	HostSource     string             `json:"host-source,omitempty"`
	WorkshopTarget string             `json:"workshop-target,omitempty"`
}

type Workshops struct {
	Workshops []*WorkshopInfo     `json:"workshops"`
	Files     []*WorkshopFileInfo `json:"files"`
}

type WorkshopInfo struct {
	ProjectId string     `json:"project-id"`
	Name      string     `json:"name"`
	Base      string     `json:"base"`
	Status    string     `json:"status"`
	Sdks      []*SdkInfo `json:"sdks,omitempty"`
	Notes     []string   `json:"notes,omitempty"`
}

type WorkshopFileInfo struct {
	ProjectId string `json:"project-id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

type Workshop struct {
	WorkshopInfo
	Path string `json:"path"`
}

type Script struct {
	Script string `json:"script"`
}

var ensureStateSoon = stateEnsureBefore

func workshopFileToInfo(pid string, name string, path string) *WorkshopFileInfo {
	var ws WorkshopFileInfo
	ws.ProjectId = pid
	ws.Name = name
	ws.Path = path
	return &ws
}

func workshopToInfo(w *workshop.Workshop, sdks map[string]*sdk.Info, health healthstate.HealthState, mounts map[string][]*Mount) *WorkshopInfo {
	var info WorkshopInfo
	info.Name = w.Name
	info.ProjectId = w.Project.ProjectId
	info.Base = w.Base

	for _, sk := range w.Sdks {
		sdkInfo := sdks[sk.Name]
		if sdkInfo == nil {
			sdkInfo = &sdk.Info{}
		}

		var healthInfo *HealthCheckInfo
		if sdkHealth, ok := health.SdkHealth[sk.Name]; ok {
			healthInfo = &HealthCheckInfo{
				Timestamp: sdkHealth.Timestamp,
				Message:   sdkHealth.Message,
				Code:      sdkHealth.Code,
			}
		}

		sdkMounts := mounts[sk.Name]

		info.Sdks = append(info.Sdks, &SdkInfo{
			Name:        sk.Name,
			Version:     sdkInfo.Version,
			Channel:     sk.Channel,
			Revision:    sk.Revision.String(),
			BuildTime:   sdkInfo.BuildTime,
			InstallTime: sk.InstallTime,
			Health:      healthInfo,
			Mounts:      sdkMounts,
		})
	}

	if len(health.Code) > 0 {
		info.Notes = append(info.Notes, health.Code)
	}
	info.Status = health.Status.String()

	return &info
}

func mounts(w *workshop.Workshop, sdks map[string]*sdk.Info) (map[string][]*Mount, error) {
	var mnts = map[string][]*Mount{}

	masters := map[interfaces.PlugRef][]interfaces.PlugRef{}
	for _, sk := range sdks {
		for s, m := range sk.PlugBinds {
			ref := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: m.Sdk, Name: m.Name}
			sref := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: sk.Name, Name: s}
			masters[ref] = append(masters[ref], sref)
		}
	}

	for name, prof := range w.Profiles {
		for _, dev := range prof.Mounts {
			if dev.Type == workshop.HostWorkshop {
				pref := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: prof.Sdk, Name: dev.Name}
				mnt := &Mount{
					Plug:           pref,
					HostSource:     dev.What,
					WorkshopTarget: dev.Where,
				}
				mnts[name] = append(mnts[name], mnt)
				if slaves, ok := masters[pref]; ok {
					for _, slave := range slaves {
						mnt := &Mount{
							Plug:           slave,
							HostSource:     dev.What,
							WorkshopTarget: dev.Where,
						}
						mnts[slave.Sdk] = append(mnts[slave.Sdk], mnt)
					}
				}
			}
			if dev.Type == workshop.WorkshopWorkshop {
				pref := interfaces.PlugRef{ProjectId: w.Project.ProjectId, Workshop: w.Name, Sdk: name, Name: dev.Name}
				mnt := &Mount{
					Plug:           pref,
					WorkshopSource: dev.What,
					WorkshopTarget: dev.Where,
				}
				mnts[name] = append(mnts[name], mnt)
			}
		}
	}

	return mnts, nil
}

func newWorkshopChange(st *state.State, kind string, user, projectId, action string, names []string) *state.Change {
	var summary string
	switch len(names) {
	case 1:
		summary = fmt.Sprintf("%s %q workshop", cases.Title(language.BritishEnglish).String(action), names[0])
	default:
		summary = fmt.Sprintf("%s %s workshops", cases.Title(language.BritishEnglish).String(action), strutil.Quoted(names))
	}

	change := st.NewChange(kind, summary)
	change.Set("user", user)
	change.Set("project-id", projectId)
	return change
}

func newWorkshopSdkChange(st *state.State, kind string, user, projectId, action string, wp, sk string) *state.Change {
	sdkRef := sdk.Ref{ProjectId: projectId, Workshop: wp, Sdk: sk}
	summary := fmt.Sprintf(`%s %q SDK`, cases.Title(language.BritishEnglish).String(action), sdkRef.ShortRef())
	change := st.NewChange(kind, summary)
	change.Set("user", user)
	change.Set("project-id", projectId)
	return change
}

func v1GetProjectWorkshops(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()

	query := r.URL.Query()
	wstate := query.Get("state")
	status := healthstate.UnknownStatus
	ignoreStatus := false
	var err error
	if wstate == "" || wstate == "all" || wstate == "available" {
		ignoreStatus = true
	} else {
		status, err = healthstate.StatusLookup(wstate)
		if err != nil {
			return statusBadRequest(`%w, "all", "available"`, err)
		}
	}

	wrkmgr := c.d.overlord.WorkshopManager()
	workshops, err := wrkmgr.Workshops(r.Context(), projectId)
	if err != nil {
		return statusInternalError("%w", err)
	}

	info := Workshops{}
	info.Workshops = make([]*WorkshopInfo, 0, len(workshops))
	for _, w := range workshops {
		health := wrkmgr.WorkshopHealth(w)
		if ignoreStatus || health.Status == status {
			wi := workshopToInfo(w, nil, health, nil)
			info.Workshops = append(info.Workshops, wi)
		}
	}

	// If the client queried everything available,
	// we add workshop files to the response.
	// Some of these may only exist as files, not instances.
	// The "available" query is a best-effort version of "all":
	// if something is wrong with the files we still return the instances.
	if ignoreStatus {
		files, err := wrkmgr.WorkshopFiles(r.Context(), projectId)
		var fileErr *workshopstate.WorkshopFileError
		if wstate == "available" && errors.As(err, &fileErr) {
			state.Warnf("%v", err)
		} else if err != nil {
			return statusInternalError("%w", err)
		} else {
			info.Files = make([]*WorkshopFileInfo, 0, len(files))
			for name, path := range files {
				info.Files = append(info.Files, workshopFileToInfo(projectId, name, path))
			}
		}
	}

	return SyncResponse(info, http.StatusOK)
}

func maybeSdkRefresh(names []string) (wp string, sk string, partial bool) {
	if len(names) != 1 {
		return "", "", false
	}

	parts := strings.FieldsFunc(names[0], func(r rune) bool { return r == '/' })
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func actionMode(reqData *workshopReq) (conflict.Mode, error) {
	var mode conflict.Mode

	if reqData.Options.Mode == "" {
		reqData.Options.Mode = "transactional"
	}

	switch reqData.Options.Mode {
	case "transactional":
	case "wait-on-error", "continue", "abort":
		if reqData.Action != "refresh" && reqData.Action != "launch" {
			return mode, fmt.Errorf("cannot %s: mode %q is not valid with the %q command", reqData.Action, reqData.Options.Mode, reqData.Action)
		}
	default:
		return mode, fmt.Errorf("cannot %s: %q is not a valid mode", reqData.Action, reqData.Options.Mode)
	}

	mode, err := conflict.ParseMode(reqData.Options.Mode)
	if err != nil {
		return mode, fmt.Errorf("cannot %s: %v", reqData.Action, err)
	}

	if len(reqData.Names) > 1 && mode != conflict.ChangeTransactional {
		return mode, fmt.Errorf("wait-on-error is not supported for multiple workshops")
	}

	return mode, nil
}

func refresh(ctx context.Context, st *state.State, mgr *workshopstate.WorkshopManager, reqData *workshopReq, user, pid string) (*state.Change, []*state.TaskSet, error) {
	var taskset []*state.TaskSet
	var change *state.Change
	var err error

	if wp, sk, ok := maybeSdkRefresh(reqData.Names); ok {
		change = newWorkshopSdkChange(st, "refresh", user, pid, reqData.Action, wp, sk)
		if sk != sdk.Sketch {
			return change, taskset, fmt.Errorf(`partial refresh is supported only for "sketch" SDK`)
		}
		taskset, err = mgr.RefreshLocalSdk(ctx, pid, wp, sk)
	} else {
		change = newWorkshopChange(st, "refresh", user, pid, reqData.Action, reqData.Names)
		taskset, err = mgr.RefreshMany(ctx, reqData.Names, pid)
	}
	return change, taskset, err
}

func v1PostProjectWorkshop(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	st := c.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	wsmgr := c.d.overlord.WorkshopManager()

	var reqData workshopReq
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqData); err != nil {
		return statusBadRequest("cannot decode data from request body: %w", err)
	}

	if len(reqData.Names) == 0 {
		return statusBadRequest("cannot %s: at least one workshop name must be provided", reqData.Action)
	}

	reqData.Names = strutil.Deduplicate(reqData.Names)

	validActions := []string{
		"launch",
		"refresh",
		"start",
		"stop",
		"remove",
	}

	if !slices.Contains(validActions, reqData.Action) {
		return statusBadRequest(fmt.Sprintf("unknown action %q", reqData.Action))
	}

	mode, err := actionMode(&reqData)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	user, ok := r.Context().Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("cannot %s: user is not known", reqData.Action)
	}

	var change *state.Change
	var taskset []*state.TaskSet

	if mode.Resume() {
		change, err = conflict.ResumeAfterWait(st, reqData.Names[0], projectId, mode, reqData.Action)
	} else {
		switch reqData.Action {
		case "launch":
			change = newWorkshopChange(st, "launch", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.LaunchMany(r.Context(), reqData.Names, projectId, change.ID())
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
		case "refresh":
			change, taskset, err = refresh(r.Context(), st, wsmgr, &reqData, user, projectId)
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
		case "start":
			change = newWorkshopChange(st, "start", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.StartMany(r.Context(), reqData.Names, projectId, change.ID())
		case "stop":
			change = newWorkshopChange(st, "stop", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.StopMany(r.Context(), reqData.Names, projectId, change.ID())
		case "remove":
			change = newWorkshopChange(st, "remove", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.RemoveMany(r.Context(), reqData.Names, projectId, change.ID())
		default:
			return statusBadRequest("internal error: action passed validation but was not matched during dispatch")
		}
	}

	if err != nil {
		return statusBadRequest("%w", err)
	}

	for _, tset := range taskset {
		change.AddAll(tset)
	}
	if len(change.Tasks()) == 0 {
		change.SetStatus(state.DoneStatus)
	}

	ensureStateSoon(st, 0)

	return AsyncResponse(nil, change.ID())
}

func v1GetProjectWorkshop(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	name := muxVars(r)["name"]

	if projectId == "" {
		return statusBadRequest("project-id must be provided")
	}

	if name == "" {
		return statusBadRequest("workshop name must be provided")
	}

	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()

	wrkmgr := c.d.overlord.WorkshopManager()
	w, err := wrkmgr.Workshop(r.Context(), name, projectId)
	if err != nil {
		return statusNotFound("%w", err)
	}
	health := wrkmgr.WorkshopHealth(w)

	ctx := context.WithValue(r.Context(), workshop.ContextProjectId, projectId)
	sdks, err := w.SdkInfos(ctx)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	ms, err := mounts(w, sdks)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	files, err := wrkmgr.WorkshopFiles(ctx, projectId)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	rsp := Workshop{
		WorkshopInfo: *workshopToInfo(w, sdks, health, ms),
		Path:         files[w.Name],
	}

	return SyncResponse(rsp, http.StatusOK)
}

func v1GetProjectWorkshopScripts(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	name := muxVars(r)["name"]

	if projectId == "" {
		return statusBadRequest("project-id must be provided")
	}

	if name == "" {
		return statusBadRequest("workshop name must be provided")
	}

	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()

	wrkmgr := c.d.overlord.WorkshopManager()
	file, err := wrkmgr.WorkshopFile(r.Context(), name, projectId)
	if err != nil {
		return statusNotFound("%w", err)
	}

	// Convert strings to objects to allow extra fields in future.
	compat := make(map[string]Script, len(file.Scripts))
	for name, script := range file.Scripts {
		compat[name] = Script{Script: script.String()}
	}

	return SyncResponse(compat, http.StatusOK)
}
