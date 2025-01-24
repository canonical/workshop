package sdkstate

import (
	"github.com/canonical/workshop/internal/interfaces"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
	backend "github.com/canonical/workshop/internal/workshop"
)

type SdkManager struct {
	backend backend.Backend
	repo    *interfaces.Repository
}

func New(s *state.State, runner *state.TaskRunner, repo *interfaces.Repository) *SdkManager {
	manager := &SdkManager{repo: repo}

	s.Lock()
	manager.backend = workshop.WorkshopBackend(s)
	s.Unlock()

	runner.AddHandler("retrieve-sdk", OnDo(manager.doRetrieveSdk), nil)
	runner.AddHandler("install-sdk", OnDo(manager.doInstallSdk), manager.undoInstallSdk)
	runner.AddHandler("install-local-sdk", OnDo(manager.doInstallLocalSdk), manager.undoInstallLocalSdk)
	runner.AddHandler("link-sdk", OnDo(manager.doLinkSdk), manager.undoLinkSdk)

	return manager
}

func (w *SdkManager) Ensure() error {
	return nil
}
