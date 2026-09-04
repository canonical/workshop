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
	"encoding/json"
	"net/http"

	"github.com/jessevdk/go-flags"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
	"github.com/canonical/workshop/internal/workshop"
)

// workshopCtlOptions holds the various options with which workshopctl is invoked.
type workshopCtlOptions struct {
	// ContextID is a string used to determine the context of this call (e.g.
	// which context and handler should be used, etc.)
	ContextID string `json:"context-id"`

	// Args contains a list of parameters to use for this invocation.
	Args []string `json:"args"`
}

// workshopCtlPostData is the data posted to the daemon /v2/workshopctl endpoint
// TODO: this can be removed once we no longer need to pass stdin data
// but instead use a real stdin stream
type workshopCtlPostData struct {
	workshopCtlOptions

	Stdin []byte `json:"stdin,omitempty"`
}

type workshopctlOutput struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func v1PostWorkshopCtl(c *Command, r *http.Request, _ *userState) Response {
	var reqData workshopCtlPostData

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqData); err != nil {
		return statusBadRequest("cannot decode data from request body: %w", err)
	}

	_, uid, _, err := ucrednetGet(r.RemoteAddr)
	if err != nil {
		return statusForbidden("cannot get remote user: %w", err)
	}

	hookContext, response := workshopctlHookContext(c, r, reqData.ContextID)
	if response != nil {
		return response
	}

	if reqData.Stdin != nil {
		hookContext.Lock()
		hookContext.Set("stdin", reqData.Stdin)
		hookContext.Unlock()
	}

	stdout, stderr, err := ctlcmd.Run(r.Context(), hookContext, reqData.Args, uid)
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			stdout = []byte(e.Error())
		} else {
			return statusBadRequest("%w", err)
		}
	}

	result := workshopctlOutput{
		Stdout: string(stdout),
		Stderr: string(stderr),
	}

	return SyncResponse(result, http.StatusOK)
}

// workshopctlHookContext returns the hook context used to execute a
// workshopctl command. A supplied cookie selects an existing context;
// otherwise the workshop instance ID is validated before creating an
// ephemeral context.
func workshopctlHookContext(
	c *Command,
	r *http.Request,
	contextID string,
) (*hookstate.Context, Response) {
	if contextID != "" {
		return workshopctlHookContextFromCookie(c, contextID)
	}
	return workshopctlHookContextFromInstanceID(c, r)
}

// workshopctlHookContextFromCookie returns the active or long-lived context
// identified by the supplied workshop cookie. An invalid cookie is rejected
// rather than falling back to instance ID authentication.
func workshopctlHookContextFromCookie(
	c *Command,
	contextID string,
) (*hookstate.Context, Response) {
	hookContext, err := c.d.overlord.HookManager().Context(contextID)
	if err != nil {
		return nil, statusBadRequest("cannot get workshop context: %w", err)
	}
	return hookContext, nil
}

// workshopctlHookContextFromInstanceID validates that the requesting user owns
// the workshop instance identified by the request context, then creates a
// taskless context for this individual workshopctl invocation.
func workshopctlHookContextFromInstanceID(
	c *Command,
	r *http.Request,
) (*hookstate.Context, Response) {
	instanceID, _ := r.Context().
		Value(workshop.ContextWorkshopInstanceID).(string)
	if instanceID == "" {
		return nil, statusBadRequest("workshop instance ID not supplied")
	}

	valid, err := c.d.overlord.WorkshopManager().
		OwnsWorkshopInstanceID(r.Context(), instanceID)
	if err != nil {
		logger.Noticef("cannot validate workshop instance ID: %v", err)
		return nil, statusInternalError("internal error occurred validating workshop instance id")
	}
	if !valid {
		return nil, statusForbidden("invalid workshop instance ID")
	}

	hookContext, err := c.d.overlord.HookManager().NewEphemeralContext()
	if err != nil {
		logger.Noticef("cannot create workshop context: %v", err)
		return nil, statusInternalError("internal error occurred")
	}
	return hookContext, nil
}
