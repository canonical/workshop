package workshopstate

import (
	"context"
	"fmt"

	"golang.org/x/exp/slices"

	. "github.com/canonical/workshop/internal/overlord/handlersetup"
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
	runner.AddHandler("create-workshop", OnDo(manager.doConstructWorkshop), manager.doRemoveWorkshop)
	runner.AddHandler("start-workshop", OnDo(manager.doStart), manager.doStop)
	runner.AddHandler("stop-workshop", OnDo(manager.doStop), manager.doStart)
	runner.AddHandler("remove-workshop", OnDo(manager.doRemoveWorkshop), nil)
	runner.AddHandler("mount-project", OnDo(manager.doMountProject), manager.undoMountProject)
	runner.AddHandler("create-workshop-storage", OnDo(manager.doCreateWorkshopStorage), manager.doRemoveWorkshopStorage)
	runner.AddHandler("remove-workshop-storage", OnDo(manager.doRemoveWorkshopStorage), nil)
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

// Returns latest file for a workshop. The state must be locked, as listing
// projects can update project metadata.
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

// Returns all workshop files for a project. The state must be locked, as
// listing projects can update project metadata.
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
