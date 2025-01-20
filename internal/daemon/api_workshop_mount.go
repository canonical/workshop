package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

type mountRequest struct {
	Action     string             `json:"action"`
	Plug       interfaces.PlugRef `json:"plug"`
	HostSource string             `json:"host-source"`
}

func newMountChange(st *state.State, user string, reqData *mountRequest) *state.Change {
	summary := fmt.Sprintf("%s %s", cases.Title(language.BritishEnglish).String(reqData.Action), fmt.Sprintf("%s/%s:%s", reqData.Plug.Workshop, reqData.Plug.Sdk, reqData.Plug.Name))

	change := st.NewChange(reqData.Action, summary)
	change.Set("user", user)
	change.Set("project-id", reqData.Plug.ProjectId)
	return change
}

func v1PostWorkshopMount(c *Command, r *http.Request, _ *userState) Response {
	projectId := muxVars(r)["id"]
	w := muxVars(r)["name"]

	if projectId == "" {
		return statusBadRequest("project-id required")
	}

	if w == "" {
		return statusBadRequest("workshop name required")
	}

	user, ok := r.Context().Value(workshop.ContextUser).(string)
	if !ok {
		return statusBadRequest("internal error: no user associated with the request")
	}

	st := c.d.state
	st.Lock()
	defer st.Unlock()

	o := c.d.overlord

	var reqData mountRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqData); err != nil {
		return statusBadRequest("cannot decode data from request body: %w", err)
	}
	if reqData.Action != "remount" {
		return statusBadRequest("unknown action %q", reqData.Action)
	}
	reqData.Plug.Workshop = w
	reqData.Plug.ProjectId = projectId

	if err := checkWorkshopExists(r.Context(), o.WorkshopManager(), projectId, w); err != nil {
		return statusNotFound("cannot access workshop %q: %w", w, err)
	}

	change := newMountChange(st, user, &reqData)
	defer func() {
		if len(change.Tasks()) == 0 {
			change.SetStatus(state.DoneStatus)
		}
	}()

	repo := o.InterfaceManager().Repository()
	connRef, err := repo.Connected(reqData.Plug.ProjectId, reqData.Plug.Workshop, reqData.Plug.Sdk, reqData.Plug.Name)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	if len(connRef) == 0 {
		return statusBadRequest("cannot remount %q: plug is disconnected", reqData.Plug.ShortRef())
	}

	conn, err := repo.Connection(connRef[0])
	if err != nil {
		return statusBadRequest("%w", err)
	}

	if conn.Plug.Interface() != "mount" {
		return statusBadRequest(`cannot remount %q: interface type should be "mount" (now: %q)`, reqData.Plug.ShortRef(), conn.Plug.Interface())
	}

	taskset, err := o.WorkshopManager().Remount(r.Context(), st, reqData.Plug, reqData.HostSource, projectId)
	if err != nil {
		return statusBadRequest("%w", err)
	}

	change.AddAll(taskset)

	ensureStateSoon(st, 0)

	return AsyncResponse(nil, change.ID())
}
