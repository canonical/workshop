package hookstate

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// Handler is the interface a client must satify to handle hooks.
type Handler interface {
	// Before is called right before the hook is to be run.
	Before() error

	// Done is called right after the hook has finished successfully.
	Done() error

	// Error is called if the hook encounters an error while running.
	// The returned bool flag indicates if the original hook error should be
	// ignored by hook manager.
	Error(hookErr error) (ignoreHookErr bool, err error)
}

// HandlerGenerator is the function signature required to register for hooks.
type HandlerGenerator func(*Context) Handler

type HookSetup struct {
	ProjectId   string           `json:"project-id"`
	Workshop    string           `json:"workshop"`
	Sdk         string           `json:"sdk"`
	HookType    WorkshopHookType `json:"type"`
	Timeout     time.Duration    `json:"timeout"`
	IgnoreError bool             `json:"bool"`
}

type WorkshopHookType int

const (
	SetupBase WorkshopHookType = iota
	SetupProject
	SaveState
	RestoreState
	CheckHealth

	fakeHook // tests only
)

func (s WorkshopHookType) String() string {
	return [...]string{"setup-base", "setup-project", "save-state", "restore-state", "check-health", "fake-hook"}[s]
}

func (h *HookSetup) Type() string {
	return h.HookType.String()
}

type HookManager struct {
	state      *state.State
	repository *repository
	backend    workshop.Backend

	contextsMutex sync.RWMutex
	contexts      map[string]*Context
}

func New(s *state.State, runner *state.TaskRunner) *HookManager {
	manager := &HookManager{
		state:      s,
		repository: newRepository(),
		contexts:   make(map[string]*Context),
	}

	s.Lock()
	manager.backend = workshop.WorkshopBackend(s)
	s.Unlock()

	runner.AddHandler("run-hook", handlersetup.OnDo(manager.doRunHook), nil)
	runner.AddCleanup("run-hook", manager.doHookCleanup)

	setupHooks(manager)

	return manager
}

func (w *HookManager) Ensure() error {
	return nil
}

func (m *HookManager) ephemeralContext(cookieID string) (context *Context, err error) {
	var contexts map[string]string
	m.state.Lock()
	defer m.state.Unlock()
	err = m.state.Get("workshop-cookies", &contexts)
	if err != nil {
		return nil, fmt.Errorf("cannot get workshop cookies: %v", err)
	}
	if workshop, ok := contexts[cookieID]; ok {
		// create new ephemeral context
		context, err = NewContext(nil, m.state, &HookSetup{Workshop: workshop}, nil, cookieID)
		return context, err
	}
	return nil, fmt.Errorf("invalid workshop cookie requested")
}

// Context obtains the context for the given cookie ID.
func (m *HookManager) Context(cookieID string) (*Context, error) {
	m.contextsMutex.RLock()
	defer m.contextsMutex.RUnlock()

	var err error
	context, ok := m.contexts[cookieID]
	if !ok {
		context, err = m.ephemeralContext(cookieID)
		if err != nil {
			return nil, err
		}
	}

	return context, nil
}

// Register registers a function to create Handler values whenever hooks
// matching the provided pattern are run.
func (m *HookManager) Register(pattern *regexp.Regexp, generator HandlerGenerator) {
	m.repository.addHandlerGenerator(pattern, generator)
}

type workshopHookHandler struct {
}

func (h *workshopHookHandler) Before() error {
	return nil
}

func (h *workshopHookHandler) Done() error {
	return nil
}

func (h *workshopHookHandler) Error(err error) (bool, error) {
	return false, nil
}

func setupHooks(hookMgr *HookManager) {
	handlerGenerator := func(context *Context) Handler {
		return &workshopHookHandler{}
	}

	hookMgr.Register(regexp.MustCompile("^setup-base$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^setup-project$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^save-state$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^restore-state$"), handlerGenerator)
}
