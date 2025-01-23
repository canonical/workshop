package hookstate

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/afero"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

func (h *HookManager) doRunHook(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := handlersetup.UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := handlersetup.BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	var hook HookSetup
	st := task.State()
	st.Lock()
	err = task.Get("hook-setup", &hook)
	st.Unlock()
	if err != nil {
		return err
	}

	if hook.HookType == SaveState || hook.HookType == RestoreState {
		volume := workshop.WorkshopStateVolumeName(w, prj.ProjectId)
		if err := h.backend.AttachVolume(ctx, w, volume, dirs.WorkshopStateDir, false); err != nil {
			return fmt.Errorf("cannot run hook %q for SDK %q: %w", hook.Type(), hook.Sdk, err)
		}

		defer func() {
			if err := h.backend.DetachVolume(ctx, w, volume); err != nil {
				logger.Noticef("RunHook on Do: Cannot detach SDK state storage volume %q", volume)
			}
		}()
	}

	switch hook.HookType {
	case SaveState:
		{
			fs, err := h.backend.WorkshopFs(ctx, w)
			if err != nil {
				return fmt.Errorf("cannot run hook \"save-state\" for %q SDK: %v", hook.Sdk, err)
			}
			err = fs.MkdirAll(hook.Environment["SDK_STATE_DIR"], 0755)
			fs.Close()
			if err != nil {
				return fmt.Errorf("cannot run hook \"save-state\" for %q SDK: %v", hook.Sdk, err)
			}
		}
		return h.executeHook(ctx, task, w, prj.ProjectId, &hook)
	case RestoreState:
		{
			fs, err := h.backend.WorkshopFs(ctx, w)
			if err != nil {
				return fmt.Errorf("cannot run hook \"restore-state\" for %q SDK: %v", hook.Sdk, err)
			}
			info, err := fs.Stat(hook.Environment["SDK_STATE_DIR"])
			fs.Close()
			if err != nil {
				return fmt.Errorf("cannot run hook \"restore-state\" for %q SDK: %v", hook.Sdk, err)
			}

			if !info.IsDir() {
				return fmt.Errorf("cannot run hook \"restore-state\" for %q SDK: state storage path is not a directory", hook.Sdk)
			}
		}
		return h.executeHook(ctx, task, w, prj.ProjectId, &hook)
	default:
		return h.executeHook(ctx, task, w, prj.ProjectId, &hook)
	}
}

func (h *HookManager) executeHook(ctx context.Context, task *state.Task, w, projectId string, hook *HookSetup) error {
	hookPath := sdk.SdkHookPath(hook.Sdk, hook.Type())

	wsFs, err := h.backend.WorkshopFs(ctx, w)
	if err != nil {
		return err
	}

	info, err := wsFs.Stat(hookPath)
	wsFs.Close()
	if errors.Is(err, afero.ErrFileNotFound) || !info.Mode().IsRegular() {
		logger.Debugf("%q SDK does not provide %q hook", hook.Sdk, hook.Type())
		return nil
	}

	hookCtx, err := createHookContext(task, h.repository, hook)
	if err != nil {
		return err
	}

	contextID := hookCtx.ID()
	h.contextsMutex.Lock()
	h.contexts[contextID] = hookCtx
	h.contextsMutex.Unlock()

	defer func() {
		h.contextsMutex.Lock()
		delete(h.contexts, contextID)
		h.contextsMutex.Unlock()
	}()

	if err := hookCtx.Handler().Before(); err != nil {
		return err
	}

	// create a memory out/err to log the hook output into the task's log
	memFs := afero.NewMemMapFs()
	out, err := memFs.Create(fmt.Sprintf("%s-%s", w, projectId))
	if err != nil {
		return err
	}
	defer out.Close()

	hook.Environment["WORKSHOP_COOKIE"] = hookCtx.ID()
	args := workshop.Execution{
		ExecArgs: workshop.ExecArgs{
			UserId:  0,
			GroupId: 0,
			Command: []string{
				"bash",
				"-ue",
				"-o",
				"pipefail",
				hookPath,
			},
			Environment: hook.Environment,
			WorkDir:     sdk.SdkHooksDir(hook.Sdk),
			Timeout:     hook.Timeout,
		},
		ExecControls: workshop.ExecControls{
			Stdin:  nil,
			Stdout: out,
			Stderr: out,
		},
	}

	exectx, err := h.backend.Exec(ctx, w, &args)
	// Handle errors that are unrelated to the command, for example, LXD-related
	// issues. An error here means the execution has not started at all.
	if err != nil {
		return err
	}
	err = exectx.WaitExecution(ctx)

	st := task.State()
	hookLog, _ := afero.ReadFile(memFs, out.Name())
	if len(hookLog) > 0 {
		st.Lock()
		task.Logf("%s", string(hookLog))
		st.Unlock()
	}

	// Handle the command execution errors; all the errors that are related to
	// the backend that was executing the command should have been handled above
	// already.
	if err != nil {
		ignore, handlerError := hookCtx.Handler().Error(err)
		if handlerError != nil {
			return handlerError
		}
		if ignore {
			return nil
		}
		return err
	}

	if err = hookCtx.Handler().Done(); err != nil {
		return err
	}

	hookCtx.Lock()
	defer hookCtx.Unlock()
	if err = hookCtx.Done(); err != nil {
		return err
	}

	return err
}

func createHookContext(task *state.Task, repo *repository, hook *HookSetup) (*Context, error) {
	hookCtx, err := NewContext(task, task.State(), hook, nil, "")
	if err != nil {
		return nil, err
	}

	handlers := repo.generateHandlers(hookCtx)
	handlersCount := len(handlers)
	if handlersCount == 0 {
		return nil, fmt.Errorf("internal error: no registered handlers for hook %q", hook.HookType)
	}
	if handlersCount > 1 {
		return nil, fmt.Errorf("internal error: %d handlers registered for hook %q, expected 1", handlersCount, hook.HookType)
	}
	hookCtx.handler = handlers[0]
	return hookCtx, nil
}
