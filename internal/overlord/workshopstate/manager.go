package workshopstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/exp/slices"

	"github.com/canonical/workshop/internal/overlord/conflict"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

var ErrWaitingOnError = errors.New("waiting on error")

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
	runner.AddHandler("create-apt-cache", OnDo(manager.doCreateAptCache), manager.doRemoveAptCache)
	runner.AddHandler("remove-apt-cache", OnDo(manager.doRemoveAptCache), nil)
	runner.AddHandler("mount-apt-cache", OnDo(manager.doMountAptCache), manager.undoMountAptCache)
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

// Checks the provided workshop has one of the allowed health statuses.
func (w *WorkshopManager) CheckStatus(ctx context.Context, name, pId string, allowedStatuses []healthstate.Status) error {
	wp, err := w.Workshop(ctx, name, pId)
	if err != nil {
		return err
	}

	health := w.WorkshopHealth(wp)

	if !slices.Contains(allowedStatuses, health.Status) {
		switch health.Status {
		case healthstate.ReadyStatus:
			return fmt.Errorf("workshop already running")
		case healthstate.PendingStatus:
			if health.Code == "wait-on-error" {
				return ErrWaitingOnError
			}
			return fmt.Errorf("other changes in progress")
		case healthstate.ErrorStatus:
			return fmt.Errorf("workshop is unhealthy")
		case healthstate.StoppedStatus:
			return fmt.Errorf("workshop not running")
		default:
			return fmt.Errorf("workshop health is unknown")
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

// Returns latest file for a workshop. The state must be locked,
// as listing projects can update project metadata.
func (w *WorkshopManager) WorkshopFile(ctx context.Context, name, pId string) (*workshop.File, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projects, err := w.backend.Projects(ctx)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(projects[user], func(p workshop.Project) bool { return p.ProjectId == pId })
	if idx == -1 {
		return nil, fmt.Errorf("project %q not found", pId)
	}
	p := projects[user][idx]

	return p.Workshop(name)
}

// Returns all workshop files for a project. The state must be locked,
// as listing projects can update project metadata.
func (w *WorkshopManager) WorkshopFiles(ctx context.Context, pId string) (map[string]string, error) {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return nil, fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projects, err := w.backend.Projects(ctx)
	if err != nil {
		return nil, err
	}

	idx := slices.IndexFunc(projects[user], func(p workshop.Project) bool { return p.ProjectId == pId })
	if idx == -1 {
		return nil, fmt.Errorf("project %q not found", pId)
	}
	p := projects[user][idx]

	files, err := p.ReadWorkshops()
	if err != nil {
		return files, &WorkshopFileError{err}
	}
	return files, nil
}

// WorkshopFileError wraps errors related to invalid workshop definitions or file locations.
type WorkshopFileError struct {
	err error
}

func (e *WorkshopFileError) Error() string {
	return e.err.Error()
}

func (e *WorkshopFileError) Unwrap() error {
	return e.err
}

// Returns all existing workshops for a project, the state must be
// locked as it is used to find out the workshop state.
func (w *WorkshopManager) Workshops(ctx context.Context, pId string) ([]*workshop.Workshop, error) {
	// project-id must be in the context for this query
	pCtx := context.WithValue(ctx, workshop.ContextProjectId, pId)

	workshops, err := w.backend.ProjectWorkshops(pCtx)
	if err != nil {
		return nil, err
	}

	return workshops, nil
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
		w.state.Warnf("cannot find project directory %q for workshop %q", ws.Project.Path, ws.Name)

		healthState.Status = healthstate.ErrorStatus
		healthState.Code = "missing-project"
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
