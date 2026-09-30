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

package ctlcmd_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jessevdk/go-flags"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/hookstate/ctlcmd"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/workshop"
)

// getSecretSuite tests command behaviour with synchronous secret retrieval.
type getSecretSuite struct{}

var _ = check.Suite(&getSecretSuite{})

// TestGetSecret checks the request identity, raw output and secret closure.
func (getSecretSuite) TestGetSecret(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)

	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	var stdout bytes.Buffer
	value := secrets.NewSecret([]byte("provider-api-token"))
	defer value.Close()

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", &stdout, nil,
	)
	called := false

	command.GetSecret = func(
		_ context.Context,
		st *state.State,
		project workshop.Project,
		ref sdk.PlugRef,
	) (secrets.Secret, error) {
		called = true
		c.Check(st, check.Equals, testState)
		c.Check(project, check.DeepEquals, workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		})
		c.Check(ref, check.DeepEquals, sdk.PlugRef{
			Name:      "api-key",
			ProjectId: "test-project",
			Sdk:       "my-sdk",
			Workshop:  "test-workshop",
		})
		return value, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.IsNil)
	c.Check(called, check.Equals, true)
	c.Check(stdout.String(), check.Equals, "provider-api-token")
	_, err = value.Read(make([]byte, 1))
	c.Check(err, check.Equals, io.EOF)
}

// TestGetSecretCancelled checks retrieval sees request cancellation and its
// error is preserved by the command.
func (getSecretSuite) TestGetSecretCancelled(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.GetSecret = func(
		ctx context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Check(ctx.Err(), check.Equals, context.Canceled)
		return secrets.Secret{}, ctx.Err()
	}

	err = command.Execute(ctx, nil)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestGetSecretInvalidFormat checks that a missing separator reports the
// expected identifier structure without echoing the supplied value.
func (getSecretSuite) TestGetSecretInvalidFormat(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	identifier := "private-identifier"

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, identifier, nil, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Fatal("invalid identifier must not request a secret")
		return secrets.Secret{}, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		`invalid secret identifier: expected "<SDK>.<secret>" `+
			"with both names present")
	c.Check(strings.Contains(err.Error(), identifier), check.Equals, false)
}

// TestGetSecretInvalidPlug checks that plug validation reports naming rules
// without exposing the rejected plug name.
func (getSecretSuite) TestGetSecretInvalidPlug(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	identifier := "ollama.Private_Key"

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, identifier, nil, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Fatal("invalid identifier must not request a secret")
		return secrets.Secret{}, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"invalid secret plug name: expected a lowercase letter "+
			"followed by lowercase letters or digits, optionally "+
			"separated by single hyphens")
	c.Check(strings.Contains(err.Error(), "Private_Key"), check.Equals, false)
}

// TestGetSecretInvalidSDK checks that SDK validation reports naming rules
// without exposing the rejected SDK or qualified identifier.
func (getSecretSuite) TestGetSecretInvalidSDK(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	identifier := "Private_SDK.api-key"

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, identifier, nil, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Fatal("invalid identifier must not request a secret")
		return secrets.Secret{}, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"invalid SDK name: expected at most 40 characters using "+
			"lowercase letters, digits and single internal hyphens, "+
			"with at least one letter; the name agent and prefixes "+
			"try- and project- are reserved")
	c.Check(strings.Contains(err.Error(), "Private_SDK"), check.Equals, false)
}

// TestGetSecretMissingArg checks that argument parsing requires a secret
// identifier before the command can execute.
func (getSecretSuite) TestGetSecretMissingArg(c *check.C) {
	_, _, err := ctlcmd.Run(
		context.Background(), nil, []string{"get-secret"}, 0,
	)

	c.Check(err, check.ErrorMatches,
		".*the required argument `<SDK>.<secret>` was not provided.*")
}

// TestGetSecretMissingContext checks retrieval requires a hook context.
func (getSecretSuite) TestGetSecretMissingContext(c *check.C) {
	command := ctlcmd.NewGetSecretCommand(
		nil, "my-sdk.api-key", nil, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Fatal("missing context must not request a secret")
		return secrets.Secret{}, nil
	}

	err := command.Execute(context.Background(), nil)

	c.Check(err, check.FitsTypeOf, &ctlcmd.MissingContextError{})
}

// TestGetSecretMissingIdentity checks a taskless context without an identity
// rejects retrieval before requesting a secret.
func (getSecretSuite) TestGetSecretMissingIdentity(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil, testState, &hookstate.HookSetup{}, nil, "",
	)
	c.Assert(err, check.IsNil)
	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Fatal("missing identity must not request a secret")
		return secrets.Secret{}, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Check(err, check.ErrorMatches,
		"resolving secret request identity: .*missing workshop identity.*")
}

// TestGetSecretParseSystemd checks that --systemd selects credential
// semantics while preserving the SDK-qualified secret identifier.
func (getSecretSuite) TestGetSecretParseSystemd(c *check.C) {
	command := ctlcmd.NewGetSecretCommand(nil, "", nil, nil)
	parser := flags.NewParser(command, flags.None)

	_, err := parser.ParseArgs([]string{"--systemd", "my-sdk.api-key"})

	c.Check(err, check.IsNil)
	c.Check(command.Systemd, check.Equals, true)
	c.Check(command.Secret, check.Equals, "my-sdk.api-key")
}

// TestGetSecretSystemd checks successful credentials are written unchanged.
func (getSecretSuite) TestGetSecretSystemd(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	var stdout, stderr bytes.Buffer
	value := secrets.NewSecret([]byte("provider-api-token"))
	defer value.Close()
	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", &stdout, &stderr,
	)
	command.Systemd = true
	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return value, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.IsNil)
	c.Check(stdout.String(), check.Equals, "provider-api-token")
	c.Check(stderr.String(), check.Equals, "")
}

// TestGetSecretTaskBackedContext checks retrieval uses the workshop identity
// from an attached hook task and change, without a stored identity.
func (getSecretSuite) TestGetSecretTaskBackedContext(c *check.C) {
	testState := state.New(nil)
	project := workshop.Project{
		Path:      "/task-project",
		ProjectId: "task-project",
	}
	testState.Lock()
	change := testState.NewChange("run-hook", "Run test hook")
	change.Set("user", "task-user")
	task := testState.NewTask("run-hook", "Run test hook")
	task.Set("project", project)
	task.Set("workshop", "task-workshop")
	change.AddTask(task)
	task.SetStatus(state.DoingStatus)
	testState.Unlock()
	hookCtx, err := hookstate.NewContext(
		task, testState, &hookstate.HookSetup{}, nil, "cookie-id",
	)
	c.Assert(err, check.IsNil)
	var stdout bytes.Buffer
	value := secrets.NewSecret([]byte("task-api-token"))
	defer value.Close()
	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "ollama.ollama-api-key", &stdout, nil,
	)
	command.GetSecret = func(
		_ context.Context,
		st *state.State,
		actualProject workshop.Project,
		ref sdk.PlugRef,
	) (secrets.Secret, error) {
		c.Check(st, check.Equals, testState)
		c.Check(actualProject, check.DeepEquals, project)
		c.Check(ref, check.DeepEquals, sdk.PlugRef{
			Name:      "ollama-api-key",
			ProjectId: "task-project",
			Sdk:       "ollama",
			Workshop:  "task-workshop",
		})
		return value, nil
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.IsNil)
	c.Check(stdout.String(), check.Equals, "task-api-token")
}

// TestSecretRequestErrorMultipleMatches checks that ambiguous lookups provide
// a clean diagnostic and exit code 4.
func (getSecretSuite) TestSecretRequestErrorMultipleMatches(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup failed: %w", secrets.ErrorMultipleSecrets)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"multiple secrets match plug \"my-sdk.api-key\"; "+
			"refine the secret slot")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 4)
}

// TestSecretRequestErrorPlugNotConnected checks that an absent connection
// has a user-facing diagnostic and a distinct ordinary exit code.
func (getSecretSuite) TestSecretRequestErrorPlugNotConnected(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("connections: %w", interfaces.ErrorPlugNotConnected)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"secret plug \"my-sdk.api-key\" is not connected")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 3)
}

// TestSecretRequestErrorProviderLocked checks that locked stores produce
// unlock advice and request exit code 2.
func (getSecretSuite) TestSecretRequestErrorProviderLocked(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup failed: %w", secrets.ErrorProviderLocked)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"cannot retrieve secret for plug \"my-sdk.api-key\": "+
			"unlock the secret provider and try again")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 2)
}

// TestSecretRequestErrorSecretNotFound checks that missing secrets identify
// the requested plug without exposing the wrapped retrieval context.
func (getSecretSuite) TestSecretRequestErrorSecretNotFound(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup failed: %w", secrets.ErrorSecretNotFound)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"no secret found for plug \"my-sdk.api-key\"")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 1)
}

// TestSecretRequestErrorUnknown checks that unrecognised failures retain
// their original error and diagnostic context.
func (getSecretSuite) TestSecretRequestErrorUnknown(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := errors.New("provider unavailable")

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals, cause.Error())
	c.Check(errors.Is(err, cause), check.Equals, true)
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 255)
}

// TestSystemdSecretRequestErrorMultipleMatches checks ambiguous lookups
// produce a clean diagnostic and request exit code 4.
func (getSecretSuite) TestSystemdSecretRequestErrorMultipleMatches(
	c *check.C,
) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup: %w", secrets.ErrorMultipleSecrets)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.Systemd = true

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"multiple secrets match plug \"my-sdk.api-key\"; refine the secret slot")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 4)
}

// TestSystemdSecretRequestErrorPlugNotConnected checks that an unconnected
// plug returns no error while writing a diagnostic to stderr.
func (getSecretSuite) TestSystemdSecretRequestErrorPlugNotConnected(
	c *check.C,
) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("connections: %w", interfaces.ErrorPlugNotConnected)
	var stderr bytes.Buffer

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, &stderr,
	)
	command.Systemd = true

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Check(err, check.IsNil)
	c.Check(stderr.String(), check.Equals,
		"secret plug \"my-sdk.api-key\" is not connected\n")
}

// TestSystemdSecretRequestErrorProviderLocked checks that locked providers
// supply unlock advice and request exit code 2.
func (getSecretSuite) TestSystemdSecretRequestErrorProviderLocked(
	c *check.C,
) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup: %w", secrets.ErrorProviderLocked)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.Systemd = true

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"cannot retrieve secret for plug \"my-sdk.api-key\": "+
			"unlock the secret provider and try again")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 2)
}

// TestSystemdSecretRequestErrorSecretNotFound checks that missing secrets
// identify the plug and request exit code 1.
func (getSecretSuite) TestSystemdSecretRequestErrorSecretNotFound(
	c *check.C,
) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := fmt.Errorf("provider lookup: %w", secrets.ErrorSecretNotFound)

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.Systemd = true

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals,
		"no secret found for plug \"my-sdk.api-key\"")
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 1)
}

// TestSystemdSecretRequestErrorUnknown checks unexpected failures retain
// their original diagnostic and cause, with exit code 255.
func (getSecretSuite) TestSystemdSecretRequestErrorUnknown(c *check.C) {
	testState := state.New(nil)
	hookCtx, err := hookstate.NewContext(
		nil,
		testState,
		&hookstate.HookSetup{},
		nil,
		"",
	)
	c.Assert(err, check.IsNil)
	hookCtx.SetWorkshopIdentity(hookstate.WorkshopIdentity{
		Project: workshop.Project{
			Path:      "/project",
			ProjectId: "test-project",
		},
		User:     "test-user",
		Workshop: "test-workshop",
	})

	cause := errors.New("provider unavailable")

	command := ctlcmd.NewGetSecretCommand(
		hookCtx, "my-sdk.api-key", nil, nil,
	)
	command.Systemd = true

	command.GetSecret = func(
		_ context.Context,
		_ *state.State,
		_ workshop.Project,
		_ sdk.PlugRef,
	) (secrets.Secret, error) {
		return secrets.Secret{}, cause
	}

	err = command.Execute(context.Background(), nil)

	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals, cause.Error())
	c.Check(errors.Is(err, cause), check.Equals, true)
	code, ok := errors.AsType[ctlcmd.CommandExitCodeError](err)
	c.Check(ok, check.Equals, true)
	c.Check(code.ExitCode, check.Equals, 255)
}
