package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/canonical/x-go/strutil"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

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
	Channel     string           `json:"channel,omitempty"`
	Source      string           `json:"source,omitempty"`
	Revision    string           `json:"revision"`
	BuildTime   *time.Time       `json:"build-time,omitempty"`
	InstallTime *time.Time       `json:"install-time,omitempty"`
	Health      *HealthCheckInfo `json:"health-check,omitempty"`
	Mounts      []*Mount         `json:"mounts,omitempty"`
	Tunnels     []*Tunnel        `json:"tunnels,omitempty"`
}

type Mount struct {
	Plug           sdk.PlugRef `json:"plug"`
	WorkshopSource string      `json:"workshop-source,omitempty"`
	HostSource     string      `json:"host-source,omitempty"`
	WorkshopTarget string      `json:"workshop-target,omitempty"`
}

type Tunnel struct {
	Plug sdk.PlugRef `json:"plug"`
	From Endpoint    `json:"from"`
	To   Endpoint    `json:"to"`
}

type Endpoint struct {
	Protocol string `json:"protocol"`
	Path     string `json:"path,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     uint16 `json:"port,omitempty"`
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

// Returns essential workshop properties and its SDKs health statuses (if
// available).
func workshopToInfo(w *workshop.Workshop, health healthstate.HealthState) *WorkshopInfo {
	var info WorkshopInfo
	info.Name = w.Name
	info.ProjectId = w.Project.ProjectId
	info.Base = w.Base

	sdkSetups := w.SdksByInstallOrder()

	for _, sk := range sdkSetups {
		var healthInfo *HealthCheckInfo
		if sdkHealth, ok := health.SdkHealth[sk.Name]; ok {
			healthInfo = &HealthCheckInfo{
				Timestamp: sdkHealth.Timestamp,
				Message:   sdkHealth.Message,
				Code:      sdkHealth.Code,
			}
		}

		info.Sdks = append(info.Sdks, &SdkInfo{
			Name:        sk.Name,
			Channel:     sk.Channel,
			Source:      workshop.ExpandSdkSource(sk.Source, w.Project.Path),
			Revision:    sk.Revision.String(),
			InstallTime: sk.InstallTime,
			Health:      healthInfo,
		})
	}

	if len(health.Code) > 0 {
		info.Notes = append(info.Notes, health.Code)
	}
	info.Status = health.Status.String()
	return &info
}

// Returns essential workshop properties, SDK health statuses (if available) and
// information about the installed SDKs. This function reads from the workshop's
// filesystem to obtain certain fields, it has to be used with possible latency
// changes in mind.
func workshopToInfoFull(ctx context.Context, w *workshop.Workshop, health healthstate.HealthState) (*WorkshopInfo, error) {
	var info WorkshopInfo
	info.Name = w.Name
	info.ProjectId = w.Project.ProjectId
	info.Base = w.Base

	sdks, err := w.SdkInfosByInstallOrder(ctx)
	if err != nil {
		return nil, err
	}

	mnts := w.Mounts(sdks)
	tunnels := w.Tunnels(sdks)

	for _, sk := range sdks {
		var healthInfo *HealthCheckInfo
		if sdkHealth, ok := health.SdkHealth[sk.Name]; ok {
			healthInfo = &HealthCheckInfo{
				Timestamp: sdkHealth.Timestamp,
				Message:   sdkHealth.Message,
				Code:      sdkHealth.Code,
			}
		}

		ref := sdk.Ref{ProjectId: info.ProjectId, Workshop: info.Name, Sdk: sk.Name}
		mntInfos := mountInfos(ref, mnts[sk.Name])
		tunnelInfos, err := tunnelInfos(ref, tunnels[sk.Name])
		if err != nil {
			return nil, err
		}

		info.Sdks = append(info.Sdks, &SdkInfo{
			Name:        sk.Name,
			Version:     sk.Version,
			Channel:     sk.Channel,
			Source:      workshop.ExpandSdkSource(sk.Source, w.Project.Path),
			Revision:    sk.Revision.String(),
			BuildTime:   sk.BuildTime,
			InstallTime: w.Sdks[sk.Name].InstallTime,
			Health:      healthInfo,
			Mounts:      mntInfos,
			Tunnels:     tunnelInfos,
		})
	}

	if len(health.Code) > 0 {
		info.Notes = append(info.Notes, health.Code)
	}
	info.Status = health.Status.String()

	return &info, nil
}

func mountInfos(sk sdk.Ref, mnts []workshop.Mount) []*Mount {
	if mnts == nil {
		return nil
	}

	infos := make([]*Mount, 0, len(mnts))
	for _, mnt := range mnts {
		pref := sdk.PlugRef{ProjectId: sk.ProjectId, Workshop: sk.Workshop, Sdk: sk.Sdk, Name: mnt.Name}
		switch mnt.Type {
		case workshop.HostWorkshop:
			info := &Mount{
				Plug:           pref,
				HostSource:     mnt.What,
				WorkshopTarget: mnt.Where,
			}
			infos = append(infos, info)
		case workshop.WorkshopWorkshop:
			info := &Mount{
				Plug:           pref,
				WorkshopSource: mnt.What,
				WorkshopTarget: mnt.Where,
			}
			infos = append(infos, info)
		}
	}

	return infos
}

func tunnelInfos(sk sdk.Ref, tunnels []workshop.Tunnel) ([]*Tunnel, error) {
	if tunnels == nil {
		return nil, nil
	}

	infos := make([]*Tunnel, 0, len(tunnels))
	for _, tunnel := range tunnels {
		pref := sdk.PlugRef{ProjectId: sk.ProjectId, Workshop: sk.Workshop, Sdk: sk.Sdk, Name: tunnel.Name}
		listen, err := endpoint(tunnel.Listen)
		if err != nil {
			return nil, err
		}
		connect, err := endpoint(tunnel.Connect)
		if err != nil {
			return nil, err
		}
		info := &Tunnel{
			Plug: pref,
			From: *listen,
			To:   *connect,
		}
		infos = append(infos, info)
	}

	return infos, nil
}

func endpoint(target workshop.ProxyTarget) (*Endpoint, error) {
	if target.Protocol == "unix" {
		return &Endpoint{Protocol: "unix", Path: target.Address}, nil
	}

	host, port, err := net.SplitHostPort(target.Address)
	if err != nil {
		return nil, err
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}

	return &Endpoint{Protocol: target.Protocol, Host: host, Port: uint16(number)}, nil
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
		health := healthstate.WorkshopHealth(state, w)
		if ignoreStatus || health.Status == status {
			wi := workshopToInfo(w, health)
			info.Workshops = append(info.Workshops, wi)
		}
	}

	// If the client queried everything available, we add workshop files to the
	// response. Some of these may only exist as files, not instances. The
	// "available" query is a best-effort version of "all": if something is
	// wrong with the files we still return the instances.
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
	change := newWorkshopChange(st, "refresh", user, pid, reqData.Action, reqData.Names)
	taskset, err := mgr.RefreshMany(ctx, pid, reqData.Names)

	return change, taskset, err
}

func v1PostProjectWorkshop(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]

	var reqData workshopReq
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqData); err != nil {
		return statusBadRequest("cannot decode data from request body: %w", err)
	}

	if len(reqData.Names) == 0 {
		return statusBadRequest("cannot %s: no workshop names provided", reqData.Action)
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
		return statusBadRequest("unknown action %q", reqData.Action)
	}

	mode, err := actionMode(&reqData)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	user, ok := r.Context().Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("cannot %s: user is not known", reqData.Action)
	}

	st := c.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	wsmgr := c.d.overlord.WorkshopManager()

	var change *state.Change
	var taskset []*state.TaskSet

	if mode.Resume() {
		change, err = conflict.ResumeAfterWait(st, reqData.Names[0], projectId, mode, reqData.Action)
	} else {
		switch reqData.Action {
		case "launch":
			change = newWorkshopChange(st, "launch", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.LaunchMany(r.Context(), reqData.Names, projectId)
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
		case "refresh":
			change, taskset, err = refresh(r.Context(), st, wsmgr, &reqData, user, projectId)
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
		case "start":
			change = newWorkshopChange(st, "start", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.StartMany(r.Context(), reqData.Names, projectId)
		case "stop":
			change = newWorkshopChange(st, "stop", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.StopMany(r.Context(), reqData.Names, projectId)
		case "remove":
			change = newWorkshopChange(st, "remove", user, projectId, reqData.Action, reqData.Names)
			taskset, err = wsmgr.RemoveMany(r.Context(), reqData.Names, projectId)
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
		return statusBadRequest("project-id required")
	}

	if name == "" {
		return statusBadRequest("workshop name required")
	}

	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()

	wrkmgr := c.d.overlord.WorkshopManager()
	w, err := wrkmgr.Workshop(r.Context(), name, projectId)
	if err != nil {
		return statusNotFound("%w", err)
	}
	health := healthstate.WorkshopHealth(state, w)

	ctx := context.WithValue(r.Context(), workshop.ContextProjectId, projectId)

	info, err := workshopToInfoFull(ctx, w, health)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	files, err := wrkmgr.WorkshopFiles(ctx, projectId)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	rsp := Workshop{
		WorkshopInfo: *info,
		Path:         files[w.Name],
	}

	return SyncResponse(rsp, http.StatusOK)
}

func v1GetProjectWorkshopScripts(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	name := muxVars(r)["name"]

	if projectId == "" {
		return statusBadRequest("project-id required")
	}

	if name == "" {
		return statusBadRequest("workshop name required")
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
