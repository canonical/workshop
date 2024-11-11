package workshopstate

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/x-go/strutil"
	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/conflict"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

type WorkshopManager struct {
	backend workshop.Backend
	state   *state.State
}

func New(st *state.State, runner *state.TaskRunner) *WorkshopManager {
	manager := &WorkshopManager{
		state: st,
	}

	st.Lock()
	manager.backend = workshop.WorkshopBackend(st)
	st.Unlock()

	runner.AddHandler("download-base", OnDo(manager.doDownloadBase), nil)
	runner.AddHandler("create-workshop", OnDo(manager.doCreateWorkshop), manager.undoCreateWorkshop)
	runner.AddHandler("start-workshop", OnDo(manager.doStart), manager.doStop)
	runner.AddHandler("stop-workshop", OnDo(manager.doStop), manager.doStart)
	runner.AddHandler("remove-workshop", OnDo(manager.doRemoveWorkshop), nil)
	runner.AddHandler("mount-project", OnDo(manager.doMountProject), manager.undoMountProject)
	runner.AddHandler("remove-workshop-stash", OnDo(manager.doRemoveWorkshopStash), nil)
	runner.AddHandler("stash-workshop", OnDo(manager.doStashWorkshop), manager.undoStashWorkshop)
	runner.AddHandler("create-state-storage", OnDo(manager.doCreateStateStorage), manager.doRemoveStateStorage)
	runner.AddHandler("remove-state-storage", OnDo(manager.doRemoveStateStorage), nil)

	return manager
}

func (m *WorkshopManager) StartUp() error {
	return nil
}

func (w *WorkshopManager) Ensure() error {
	return nil
}

// Checks all of the provided list of workshops are in the required health status.
func (w *WorkshopManager) CheckStatus(ctx context.Context, names []string, pId string, allowedStatuses []healthstate.Status) error {
	for _, name := range names {
		workshop, err := w.Workshop(ctx, name, pId)
		if err != nil {
			return fmt.Errorf("status check for %q failed (%v)", name, err)
		}

		health := w.WorkshopHealth(workshop)
		allowed := []string{}
		for _, s := range allowedStatuses {
			allowed = append(allowed, s.String())
		}
		if slices.Index(allowedStatuses, health.Status) == -1 {
			return fmt.Errorf("%q status is %q, must be one of: %s", name, health.Status.String(), strutil.Quoted(allowed))
		}
	}
	return nil
}

// Loads a workshop, the state must be locked as it is used to find out the
// workshop state
func (w *WorkshopManager) Workshop(ctx context.Context, name, pId string) (*workshop.Workshop, error) {
	// project-id must be in the context for this query
	pCtx := context.WithValue(ctx, workshop.ContextProjectId, pId)

	workshop, err := w.backend.Workshop(pCtx, name)
	if err != nil {
		return nil, err
	}

	return workshop, nil
}

// Returns all workshops and workshop files for a project, the state must be
// locked as it is used to find out the workshop state.
func (w *WorkshopManager) Workshops(ctx context.Context, pId string) ([]string, []*workshop.Workshop, error) {
	// project-id must be in the context for this query
	pCtx := context.WithValue(ctx, workshop.ContextProjectId, pId)

	files, workshops, err := w.backend.ProjectWorkshops(pCtx)
	if err != nil {
		return nil, nil, err
	}

	return files, workshops, nil
}

// Examine the tasks of the change to fetch possible check-health hook results
// for the workshop's SDKs.
func sdksHealthCheckSummary(chg *state.Change) map[string]healthstate.HealthCheck {
	var SdkChecks = map[string]healthstate.HealthCheck{}
	for _, task := range chg.Tasks() {
		if task.Kind() == "run-hook" {
			var healthCheck healthstate.HealthCheck
			if err := task.Get("health", &healthCheck); err == nil {
				SdkChecks[healthCheck.Sdk] = healthCheck
			}
		}
	}
	return SdkChecks
}

// Infers the state of a workshop based on the container's state and any of the
// operations in progress for the workshop. The state must be locked.
func (w *WorkshopManager) WorkshopHealth(ws *workshop.Workshop) healthstate.HealthState {
	var healthState = healthstate.HealthState{
		Timestamp: time.Now(),
	}

	// Check the project directory exists.
	if !ws.Project.Exists() {
		w.state.Warnf("%q project directory %q not found", ws.Name, ws.Project.Path)

		healthState.Status = healthstate.ErrorStatus
		healthState.Code = "missing-project"
		return healthState
	}

	// Check if the associated workshop file exists. We only check if that file
	// exists in the .workshop directory here; its state (e.g. if it is in sync
	// with the workshop instance or has any errors) is not checked.
	path := ws.Filepath()
	if !osutil.FileExists(path) {
		w.state.Warnf("%q workshop definition %q not found", ws.Name, path)

		healthState.Status = healthstate.ErrorStatus
		healthState.Code = "missing-file"
		return healthState
	}

	err := conflict.CheckChangeConflict(w.state, ws.Project.ProjectId, ws.Name, "")
	if err != nil {
		conflict, ok := err.(*conflict.ChangeConflictError)
		if !ok || conflict.ChangeID == "" {
			healthState.Status = healthstate.ErrorStatus
			return healthState
		}

		change := w.state.Change(conflict.ChangeID)
		if change.Status() == state.WaitStatus {
			healthState.Code = "wait-on-error"
		}

		healthState.SdkHealth = sdksHealthCheckSummary(change)
		healthState.Status = healthstate.PendingStatus
	} else {
		if ws.Running {
			healthState.Status = healthstate.ReadyStatus
		} else {
			healthState.Status = healthstate.StoppedStatus
		}
	}
	return healthState
}
