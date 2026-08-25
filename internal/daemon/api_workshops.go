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

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type actionOpts struct {
	// Supported action modes: "transactional", "wait-on-error", "continue",
	// "abort".
	Mode string `json:"mode"`
	// Supported refresh options: "update", "restore".
	RefreshOption string `json:"refresh-option"`
	Verbose       bool   `json:"verbose"`
}

type changeConflictValue struct {
	ChangeID   string `json:"change-id"`
	ChangeKind string `json:"change-kind"`
	ProjectID  string `json:"project-id"`
	Workshop   string `json:"workshop"`
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
	BuiltAt     *time.Time       `json:"built-at,omitempty"`
	InstalledAt time.Time        `json:"installed-at,omitzero"`
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
	Hostname  string     `json:"hostname,omitempty"`
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

type Action struct {
	Script string `json:"script"`
}

var ensureStateSoon = stateEnsureBefore

// changeInProgressErrorResponse converts a [healthstate.ChangeInProgressError]
// into a change-conflict API error response.
func changeInProgressErrorResponse(
	conflictErr healthstate.ChangeInProgressError,
) Response {
	return &resp{
		Type:   ResponseTypeError,
		Status: http.StatusBadRequest,
		Result: &errorResult{
			Kind:    errorKindChangeConflict,
			Message: conflictErr.Error(),
			Value: changeConflictValue{
				ChangeID:   conflictErr.ChangeID,
				ChangeKind: conflictErr.ChangeKind,
				ProjectID:  conflictErr.ProjectID,
				Workshop:   conflictErr.Workshop,
			},
		},
	}
}

// changeConflictErrorResponse converts a [conflict.ChangeConflictError] into a
// change-conflict API error response.
func changeConflictErrorResponse(conflictErr *conflict.ChangeConflictError) Response {
	return &resp{
		Type:   ResponseTypeError,
		Status: http.StatusBadRequest,
		Result: &errorResult{
			Kind:    errorKindChangeConflict,
			Message: conflictErr.Error(),
			Value: changeConflictValue{
				ChangeID:   conflictErr.ChangeID,
				ChangeKind: conflictErr.ChangeKind,
				ProjectID:  conflictErr.ProjectId,
				Workshop:   conflictErr.Workshop,
			},
		},
	}
}

// noWaitingChangeResponse converts a [conflict.ErrorNoWaitingChange] into a
// no-waiting-change-in-progress API error response, preserving the wrapped
// message.
func noWaitingChangeResponse(err error) Response {
	return &resp{
		Type:   ResponseTypeError,
		Status: http.StatusBadRequest,
		Result: &errorResult{
			Kind:    errorKindNoWaitingChange,
			Message: err.Error(),
		},
	}
}

func workshopFileToInfo(pid string, name string, path string) *WorkshopFileInfo {
	var ws WorkshopFileInfo
	ws.ProjectId = pid
	ws.Name = name
	ws.Path = path
	return &ws
}

// Returns essential workshop properties and its SDKs health statuses (if
// available).
func workshopToInfo(username string, w *workshop.Workshop, health healthstate.HealthState) (*WorkshopInfo, error) {
	var info WorkshopInfo
	info.Name = w.Name
	info.ProjectId = w.Project.ProjectId
	info.Base = w.File.Base

	sdkSetups := w.SdksByInstallOrder()

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return nil, err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	for _, sk := range sdkSetups {
		source := workshop.SdkSourcePath(userDataDir, w.Project, w.Name, sk.Name, sk.Source)

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
			Source:      source,
			Revision:    sk.Revision.String(),
			InstalledAt: sk.InstalledAt,
			Health:      healthInfo,
		})
	}

	if len(health.Code) > 0 {
		info.Notes = append(info.Notes, health.Code)
	}
	info.Status = health.Status.String()

	info.Hostname = w.Hostname.Domain
	if w.Hostname.Note != "" {
		info.Notes = append(info.Notes, w.Hostname.Note)
	}

	return &info, nil
}

// Returns essential workshop properties, SDK health statuses (if available) and
// information about the installed SDKs. This function reads from the workshop's
// filesystem to obtain certain fields, it has to be used with possible latency
// changes in mind.
func workshopToInfoFull(ctx context.Context, username string, w *workshop.Workshop, health healthstate.HealthState) (*WorkshopInfo, error) {
	var info WorkshopInfo
	info.Name = w.Name
	info.ProjectId = w.Project.ProjectId
	info.Base = w.File.Base

	sdks, err := w.SdkInfosByInstallOrder(ctx)
	if err != nil {
		return nil, err
	}

	mnts := w.Mounts(sdks)
	tunnels := w.Tunnels(sdks)

	usr, env, err := osutil.UserAndEnv(username)
	if err != nil {
		return nil, err
	}
	userDataDir := workshop.UserDataRootDir(usr.HomeDir, env)

	for _, sk := range sdks {
		source := workshop.SdkSourcePath(userDataDir, w.Project, w.Name, sk.Name, sk.Source)

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
			Source:      source,
			Revision:    sk.Revision.String(),
			BuiltAt:     sk.BuiltAt,
			InstalledAt: w.Sdks[sk.Name].InstalledAt,
			Health:      healthInfo,
			Mounts:      mntInfos,
			Tunnels:     tunnelInfos,
		})
	}

	if len(health.Code) > 0 {
		info.Notes = append(info.Notes, health.Code)
	}
	info.Status = health.Status.String()

	info.Hostname = w.Hostname.Domain
	if w.Hostname.Note != "" {
		info.Notes = append(info.Notes, w.Hostname.Note)
	}

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

	username, ok := r.Context().Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("internal error: user is not known")
	}

	info := Workshops{}
	info.Workshops = make([]*WorkshopInfo, 0, len(workshops))
	for _, w := range workshops {
		health := healthstate.WorkshopHealth(state, w)
		if ignoreStatus || health.Status == status {
			wi, err := workshopToInfo(username, w, health)
			if err != nil {
				return statusBadRequest("%w", err)
			}
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

func launch(ctx context.Context, mgr *workshopstate.WorkshopManager, reqData *workshopReq, pid string) ([]workshopstate.Manifest, []*state.TaskSet, error) {
	project, err := mgr.Project(ctx, pid)
	if err != nil {
		return nil, nil, err
	}

	manifests, err := mgr.LaunchManifests(ctx, project, reqData.Names)
	if err != nil {
		return nil, nil, err
	}

	tasksets, err := mgr.LaunchMany(ctx, project, manifests)
	return manifests, tasksets, err
}

func refreshOption(reqData *workshopReq) (conflict.RefreshOption, error) {
	if reqData.Options.RefreshOption == "" {
		reqData.Options.RefreshOption = "update"
	}

	return conflict.ParseRefreshSetting(reqData.Options.RefreshOption)
}

func refresh(ctx context.Context, mgr *workshopstate.WorkshopManager, reqData *workshopReq, pid string) (current, latest []workshopstate.Manifest, tasksets []*state.TaskSet, err error) {
	refreshOption, err := refreshOption(reqData)
	if err != nil {
		return nil, nil, nil, err
	}

	project, err := mgr.Project(ctx, pid)
	if err != nil {
		return nil, nil, nil, err
	}

	current, latest, err = mgr.RefreshManifests(ctx, project, reqData.Names, refreshOption)
	if err != nil {
		return nil, nil, nil, err
	}

	tasksets, err = mgr.RefreshMany(ctx, project, current, latest, refreshOption)
	if err != nil {
		return nil, nil, nil, err
	}

	return current, latest, tasksets, nil
}

func remove(ctx context.Context, mgr *workshopstate.WorkshopManager, reqData *workshopReq, pid string) (stashed, current []workshopstate.Manifest, tasksets []*state.TaskSet, err error) {
	project, err := mgr.Project(ctx, pid)
	if err != nil {
		return nil, nil, nil, err
	}

	stashed, current, running, err := mgr.RemoveManifests(ctx, pid, reqData.Names)
	if err != nil {
		return nil, nil, nil, err
	}

	tasksets, err = mgr.RemoveMany(ctx, project, stashed, current, running)
	if err != nil {
		return nil, nil, nil, err
	}

	return stashed, current, tasksets, nil
}

var validActions = []string{
	"launch",
	"refresh",
	"start",
	"stop",
	"remove",
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

	if !slices.Contains(validActions, reqData.Action) {
		return statusBadRequest("unknown action %q", reqData.Action)
	}

	mode, err := actionMode(&reqData)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	if reqData.Options.RefreshOption != "" && reqData.Action != "refresh" {
		return statusBadRequest(`cannot %s: "refresh-option" is only valid for refresh actions`, reqData.Action)
	}

	if reqData.Options.RefreshOption != "" && (mode != conflict.ChangeTransactional && mode != conflict.ChangeWaitOnError) {
		return statusBadRequest(`cannot %s: "refresh-option" is only applicable to "transactional" and "wait-on-error" modes; given: %q`, reqData.Action, mode)
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
	var current, latest, stashed []workshopstate.Manifest
	var tasksets []*state.TaskSet

	if mode.Resume() {
		format := wsmgr.FormatRevision()
		change, err = conflict.ResumeAfterWait(st, format, reqData.Names[0], projectId, mode, reqData.Action)
	} else {
		switch reqData.Action {
		case "launch":
			change = newWorkshopChange(st, "launch", user, projectId, reqData.Action, reqData.Names)
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
			change.Set("verbose", reqData.Options.Verbose)
			latest, tasksets, err = launch(r.Context(), wsmgr, &reqData, projectId)
		case "refresh":
			change = newWorkshopChange(st, "refresh", user, projectId, reqData.Action, reqData.Names)
			change.Set("wait-setup", conflict.ChangeSetup{Mode: mode.String()})
			change.Set("verbose", reqData.Options.Verbose)
			current, latest, tasksets, err = refresh(r.Context(), wsmgr, &reqData, projectId)
		case "start":
			change = newWorkshopChange(st, "start", user, projectId, reqData.Action, reqData.Names)
			tasksets, err = wsmgr.StartMany(r.Context(), reqData.Names, projectId)
		case "stop":
			change = newWorkshopChange(st, "stop", user, projectId, reqData.Action, reqData.Names)
			tasksets, err = wsmgr.StopMany(r.Context(), reqData.Names, projectId)
		case "remove":
			change = newWorkshopChange(st, "remove", user, projectId, reqData.Action, reqData.Names)
			stashed, current, tasksets, err = remove(r.Context(), wsmgr, &reqData, projectId)
		default:
			return statusBadRequest("internal error: action passed validation but was not matched during dispatch")
		}
	}

	if err != nil && change != nil {
		change.SetStatus(state.ErrorStatus)
	}

	var inProgressErr healthstate.ChangeInProgressError
	var changeConflictErr *conflict.ChangeConflictError
	switch {
	case err == nil:
	case errors.As(err, &inProgressErr):
		return changeInProgressErrorResponse(inProgressErr)
	case errors.As(err, &changeConflictErr):
		return changeConflictErrorResponse(changeConflictErr)
	case errors.Is(err, conflict.ErrorNoWaitingChange):
		return noWaitingChangeResponse(err)
	case errors.Is(err, workshop.ErrWorkshopNotLaunched):
		return statusNotFound("%w", err)
	default:
		return statusBadRequest("%w", err)
	}

	manifestsByAge := map[handlersetup.Age][]workshopstate.Manifest{
		handlersetup.OldWorkshop: current,
		handlersetup.NewWorkshop: latest,
		handlersetup.OldStash:    stashed,
	}
	for age, manifests := range manifestsByAge {
		for _, m := range manifests {
			change.Set(handlersetup.WorkshopFormatKey(m.File.Name, age), m.Format)
			change.Set(handlersetup.WorkshopBaseKey(m.File.Name, age), m.Image)
			change.Set(handlersetup.WorkshopSdksKey(m.File.Name, age), m.Sdks)
		}
	}

	for _, taskset := range tasksets {
		change.AddAll(taskset)
	}
	if len(change.Tasks()) == 0 {
		change.SetStatus(state.DoneStatus)
		return workshopUnchanged()
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

	username, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("internal error: user is not known")
	}

	info, err := workshopToInfoFull(ctx, username, w, health)
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

func v1GetProjectWorkshopActions(c *Command, r *http.Request, _ *userState) Response {
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
	compat := make(map[string]Action, len(file.Actions))
	for name, action := range file.Actions {
		compat[name] = Action{Script: action.String()}
	}

	return SyncResponse(compat, http.StatusOK)
}
