// Copyright (c) 2014-2020 Canonical Ltd
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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/state"
)

type changeInfo struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	Summary string      `json:"summary"`
	Status  string      `json:"status"`
	Tasks   []*taskInfo `json:"tasks,omitempty"`
	Ready   bool        `json:"ready"`
	Err     string      `json:"err,omitempty"`

	SpawnTime time.Time  `json:"spawn-time,omitempty"`
	ReadyTime *time.Time `json:"ready-time,omitempty"`

	ProjectId string `json:"project-id,omitempty"`

	Data map[string]*json.RawMessage `json:"data,omitempty"`
}

type taskInfo struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"`
	Summary  string           `json:"summary"`
	Status   string           `json:"status"`
	Log      []string         `json:"log,omitempty"`
	Progress taskInfoProgress `json:"progress"`

	SpawnTime time.Time  `json:"spawn-time,omitempty"`
	ReadyTime *time.Time `json:"ready-time,omitempty"`

	Data map[string]*json.RawMessage `json:"data,omitempty"`
}

type taskInfoProgress struct {
	Label string `json:"label"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

func change2changeInfo(chg *state.Change, verbose bool) *changeInfo {
	status := chg.Status()
	chgInfo := &changeInfo{
		ID:      chg.ID(),
		Kind:    chg.Kind(),
		Summary: chg.Summary(),
		Status:  status.String(),
		Ready:   status.Ready(),

		SpawnTime: chg.SpawnTime(),
	}
	readyTime := chg.ReadyTime()
	if !readyTime.IsZero() {
		chgInfo.ReadyTime = &readyTime
	}
	if err := chg.Err(); err != nil {
		chgInfo.Err = err.Error()
	}

	tasks := chg.Tasks()
	taskInfos := make([]*taskInfo, len(tasks))
	for j, t := range tasks {
		label, done, total := t.Progress()

		var taskLog []string
		// Only hooks support verbose output.
		if verbose && t.Kind() == "run-hook" {
			cached := chg.State().Cached(hookstate.HookLogKey(t.ID()))
			hookLog, ok := cached.(*hookstate.HookLog)
			if ok && hookLog != nil {
				rawout := hookLog.Output()
				for {
					line, err := rawout.ReadString('\n')
					if err != nil {
						break
					}
					line = strings.TrimRight(line, "\n")
					taskLog = append(taskLog, line)
				}
			}
		} else {
			taskLog = t.Log()
		}

		taskInfo := &taskInfo{
			ID:      t.ID(),
			Kind:    t.Kind(),
			Summary: t.Summary(),
			Status:  t.Status().String(),
			Log:     taskLog,
			Progress: taskInfoProgress{
				Label: label,
				Done:  done,
				Total: total,
			},
			SpawnTime: t.SpawnTime(),
		}
		readyTime := t.ReadyTime()
		if !readyTime.IsZero() {
			taskInfo.ReadyTime = &readyTime
		}
		var data map[string]*json.RawMessage
		if t.Get("api-data", &data) == nil {
			taskInfo.Data = data
		}
		taskInfos[j] = taskInfo
	}
	chgInfo.Tasks = taskInfos

	var prjId string
	if chg.Get("project-id", &prjId) == nil {
		chgInfo.ProjectId = prjId
	}

	var data map[string]*json.RawMessage
	if chg.Get("api-data", &data) == nil {
		chgInfo.Data = data
	}

	return chgInfo
}

func v1GetChanges(c *Command, r *http.Request, _ *userState) Response {
	query := r.URL.Query()

	if query.Has("workshops") && !query.Has("project-id") {
		return statusBadRequest("project-id required if workshops are specified")
	}

	qselect := query.Get("select")
	if qselect == "" {
		qselect = "all"
	}
	var filter func(*state.Change) bool
	switch qselect {
	case "all":
		filter = func(*state.Change) bool { return true }
	case "in-progress":
		filter = func(chg *state.Change) bool { return !chg.Status().Ready() }
	case "ready":
		filter = func(chg *state.Change) bool { return chg.Status().Ready() }
	default:
		return statusBadRequest("select should be one of: all,in-progress,ready")
	}

	projectId := query.Get("project-id")
	if projectId != "" {
		outerFilter := filter
		filter = func(chg *state.Change) bool {
			if !outerFilter(chg) {
				return false
			}

			var id string
			if err := chg.Get("project-id", &id); err != nil {
				return false
			}
			if id != projectId {
				return false
			}

			if query.Has("workshops") {
				workshops := strings.Split(query.Get("workshops"), ",")
				var workshop string
				if err := chg.Get("workshop", &workshop); err != nil {
					return false
				}
				if !slices.Contains(workshops, workshop) {
					return false
				}
			}
			return true
		}
	}

	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()
	chgs := state.Changes()
	chgInfos := make([]*changeInfo, 0, len(chgs))
	for _, chg := range chgs {
		if !filter(chg) {
			continue
		}
		chgInfos = append(chgInfos, change2changeInfo(chg, false))
	}
	return SyncResponse(chgInfos, http.StatusOK)
}

func v1GetChange(c *Command, r *http.Request, _ *userState) Response {
	query := r.URL.Query()

	var err error
	var verbose bool
	if val := query.Get("verbose"); len(val) > 0 {
		verbose, err = strconv.ParseBool(val)
		if err != nil {
			return statusBadRequest(`invalid "verbose" parameter: %v`, err)
		}
	}

	changeID := muxVars(r)["id"]
	st := c.d.overlord.State()
	st.Lock()
	defer st.Unlock()
	chg := st.Change(changeID)
	if chg == nil {
		return statusNotFound("cannot find change with id %q", changeID)
	}

	return SyncResponse(change2changeInfo(chg, verbose), http.StatusOK)
}

func v1PostChange(c *Command, r *http.Request, _ *userState) Response {
	chID := muxVars(r)["id"]

	state := c.d.overlord.State()
	state.Lock()
	defer state.Unlock()
	chg := state.Change(chID)
	if chg == nil {
		return statusNotFound("cannot find change with id %q", chID)
	}

	var reqData struct {
		Action string `json:"action"`
	}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqData); err != nil {
		return statusBadRequest("cannot decode data from request body: %w", err)
	}

	if reqData.Action != "abort" {
		return statusBadRequest("change action %q is unsupported", reqData.Action)
	}

	if chg.Status().Ready() {
		return statusBadRequest("cannot abort change %s with nothing pending", chID)
	}

	// flag the change
	chg.Abort()

	// actually ask to proceed with the abort
	stateEnsureBefore(state, 0)

	return SyncResponse(change2changeInfo(chg, false), http.StatusOK)
}

func v1GetChangeWait(c *Command, r *http.Request, _ *userState) Response {
	changeID := muxVars(r)["id"]
	st := c.d.overlord.State()
	st.Lock()
	change := st.Change(changeID)
	st.Unlock()
	if change == nil {
		return statusNotFound("cannot find change with id %q", changeID)
	}

	timeoutStr := r.URL.Query().Get("timeout")
	if timeoutStr != "" {
		// Timeout specified, wait till change is ready or timeout occurs,
		// whichever is first.
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return statusBadRequest("invalid timeout %q: %w", timeoutStr, err)
		}
		timer := time.NewTimer(timeout)
		select {
		case <-change.Ready():
			timer.Stop() // change ready, release timer resources
		case <-timer.C:
			return statusGatewayTimeout("timed out waiting for change after %s", timeout)
		case <-r.Context().Done():
			return statusInternalError("request cancelled")
		}
	} else {
		// No timeout, wait indefinitely for change to be ready.
		select {
		case <-change.Ready():
		case <-r.Context().Done():
			return statusInternalError("request cancelled")
		}
	}

	st.Lock()
	defer st.Unlock()
	return SyncResponse(change2changeInfo(change, false), http.StatusOK)
}
