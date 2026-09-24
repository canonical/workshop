// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/canonical/workshop/internal/secrets"
)

// DelegatedDBusRequest describes a D-Bus secret lookup delegated to a
// user-scoped command. The UID is conveyed through process credentials,
// not the JSON request body.
type DelegatedDBusRequest struct {
	Attributes map[string]string `json:"attributes"`
	Collection string            `json:"collection"`
}

// DelegatedDBusResponse carries the result of a delegated D-Bus secret lookup.
// A non-empty Error indicates failure. Secret contains the decoded secret
// bytes, represented as base64 in JSON, and may be empty on success.
// The recipient owns the secret bytes and must consume or clear them.
type DelegatedDBusResponse struct {
	Error  string `json:"error"`
	Secret []byte `json:"secret"`
}

// ExecService retrieves secrets by running workshop-ss-tool as the requested
// user. Same-user execution inherits the current credentials. Cross-user
// execution requires permission to set the child's user ID and primary group
// ID and clear its supplementary groups.
type ExecService struct {
	command            func(context.Context, string, ...string) *exec.Cmd
	executable         string
	resolveCredentials func(string) (*syscall.Credential, error)
}

// secretCommandName is the executable used for user-scoped secret lookups.
const secretCommandName = "workshop-ss-tool"

// Get runs the secret command for req.UID and translates its response. The
// caller must consume or close the returned secret. Cancellation kills and
// reaps the child process. Command failures include stderr, never stdout.
//
// The following errors may be expected:
//   - [ErrorCollectionAmbiguous]: several collections have the requested label.
//   - [ErrorCollectionLocked]: the selected collection is locked.
//   - [ErrorCollectionNotFound]: no collection has the requested name.
//   - [ErrorMultipleSecrets]: several secrets match the supplied attributes.
//   - [ErrorSecretNotFound]: no secret matches the supplied attributes.

// - [exec.ExitError]: the command exited unsuccessfully.
func (s ExecService) Get(
	ctx context.Context,
	req Request,
) (secrets.Secret, error) {
	err := ctx.Err()
	if err != nil {
		return secrets.Secret{}, err
	}
	err = validateRequest(req)
	if err != nil {
		return secrets.Secret{}, err
	}

	credentials, err := s.resolveCredentials(req.UID)
	if err != nil {
		return secrets.Secret{}, fmt.Errorf(
			"resolving secret command credentials: %w", err,
		)
	}

	input, err := json.Marshal(DelegatedDBusRequest{
		Attributes: req.Attributes,
		Collection: req.Collection,
	})
	if err != nil {
		return secrets.Secret{}, fmt.Errorf("encoding secret request: %w", err)
	}

	var output, errorOutput bytes.Buffer
	// Stderr may contain sensitive diagnostics; clear it on every return path.
	defer func() {
		clear(errorOutput.Bytes())
	}()

	command := s.command(ctx, s.executable)

	// Changing user credentials does not sanitise the inherited environment.
	// Use an empty environment to avoid exposing daemon secrets or inheriting
	// its loader and session settings. The command derives its bus address
	// from its effective UID and needs no inherited session environment.
	command.Env = []string{}
	command.Stdin = bytes.NewReader(input)
	command.Stdout = &output
	command.Stderr = &errorOutput
	// Same-user execution inherits the current credentials, avoiding privileged
	// supplementary-group changes. Cross-user execution sets them explicitly.
	if credentials.Uid != uint32(os.Geteuid()) {
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: credentials,
		}
	}
	err = command.Run()
	if err != nil {
		// Preserve the process failure alongside any observed cancellation;
		// cancellation does not necessarily explain why the process failed.
		err = errors.Join(err, ctx.Err())

		if errorOutput.Len() > 0 {
			return secrets.Secret{}, fmt.Errorf(
				"running secret command: %w: %s", err, &errorOutput,
			)
		}
		return secrets.Secret{}, fmt.Errorf("running secret command: %w", err)
	}

	// Both successful lookups and recognised lookup failures exit with zero.
	return decodeExecResponse(&output)
}

// MakeExecService creates a service using workshop-ss-tool beside the current
// executable, as located by [os.Executable]. It returns an error if the current
// executable's path cannot be determined. It does not search PATH or check
// whether workshop-ss-tool is installed; execution failures are reported by
// [ExecService.Get].
func MakeExecService() (ExecService, error) {
	executable, err := os.Executable()
	if err != nil {
		return ExecService{}, fmt.Errorf(
			"determining current executable path: %w", err,
		)
	}
	return ExecService{
		command: exec.CommandContext,
		executable: filepath.Join(
			filepath.Dir(executable),
			secretCommandName,
		),
		resolveCredentials: resolveExecCredentials,
	}, nil
}

// decodeExecResponse translates the command protocol into a secret or a
// lookup error. Decoded bytes are cleared on failure; successful bytes transfer
// to the returned secret. The caller retains ownership of input.
func decodeExecResponse(input io.Reader) (secrets.Secret, error) {
	var response DelegatedDBusResponse
	err := json.NewDecoder(input).Decode(&response)
	if err != nil {
		clear(response.Secret)
		return secrets.Secret{}, fmt.Errorf("decoding secret response: %w", err)
	}

	if response.Error == "" {
		return secrets.NewSecret(response.Secret), nil
	}
	clear(response.Secret)

	return secrets.Secret{}, constError(response.Error)
}

// parseExecID parses a numeric process credential, rejecting the reserved value
// that Unix credential-changing calls use to mean "leave unchanged".
func parseExecID(value string) (uint32, error) {
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, err
	}
	if id == math.MaxUint32 {
		return 0, errors.New("reserved process credential ID")
	}
	return uint32(id), nil
}

// resolveExecCredentials resolves the user and primary group IDs for uid.
// Leaving Groups empty and NoSetGroups false clears the child's supplementary
// groups rather than inheriting the daemon's group access.
//
// The following errors may be expected:
//   - [user.UnknownUserIdError]: no user account exists for the supplied UID.
func resolveExecCredentials(uid string) (*syscall.Credential, error) {
	userID, err := parseExecID(uid)
	if err != nil {
		return nil, fmt.Errorf("parsing user ID %q: %w", uid, err)
	}
	account, err := user.LookupId(strconv.FormatUint(uint64(userID), 10))
	if err != nil {
		return nil, fmt.Errorf("looking up user ID %q: %w", uid, err)
	}
	groupID, err := parseExecID(account.Gid)
	if err != nil {
		return nil, fmt.Errorf("parsing primary group ID: %w", err)
	}
	return &syscall.Credential{
		Gid: groupID,
		Uid: userID,
	}, nil
}
