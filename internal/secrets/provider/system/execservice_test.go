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
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/check.v1"
)

// execServiceSuite tests delegated lookups without D-Bus or account lookups.
type execServiceSuite struct {
	executable string
}

var _ = check.Suite(&execServiceSuite{})

// SetUpSuite locates the test executable once for the suite.
func (s *execServiceSuite) SetUpSuite(c *check.C) {
	executable, err := os.Executable()
	c.Assert(err, check.IsNil)
	s.executable = executable
}

// TestGetCommandFailure excludes stdout while exposing stderr and exit status.
func (s *execServiceSuite) TestGetCommandFailure(c *check.C) {
	command := &fakeExecCommand{
		exitCode: 7,
		path:     s.executable,
		stderr:   "delegated lookup failed",
		stdout:   "private-secret-output",
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(context.Background(), request)

	c.Assert(err, check.NotNil)
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	c.Assert(ok, check.Equals, true)
	c.Check(exitErr.ExitCode(), check.Equals, 7)
	c.Check(strings.Contains(err.Error(), command.stderr), check.Equals, true)
	c.Check(strings.Contains(err.Error(), command.stdout), check.Equals, false)
}

// TestGetCommandFailureWithoutStderr preserves a start failure for errors.Is.
func (s *execServiceSuite) TestGetCommandFailureWithoutStderr(c *check.C) {
	failure := errors.New("command unavailable")
	command := &fakeExecCommand{
		path:     s.executable,
		startErr: failure,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(context.Background(), request)

	c.Check(errors.Is(err, failure), check.Equals, true)
	c.Check(
		err,
		check.ErrorMatches,
		"running secret command: command unavailable",
	)
}

// TestGetCredentialsFailure checks that [ExecService.Get] wraps credential
// resolution failures with operation context while preserving the original
// error for [errors.Is]. No command is constructed when credentials cannot
// be resolved.
func (s *execServiceSuite) TestGetCredentialsFailure(c *check.C) {
	failure := errors.New("credentials unavailable")
	command := &fakeExecCommand{}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return nil, failure
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(context.Background(), request)

	c.Check(errors.Is(err, failure), check.Equals, true)
	c.Check(
		err,
		check.ErrorMatches,
		"resolving secret command credentials: credentials unavailable",
	)
	c.Check(command.cmd, check.IsNil)
}

// TestGetCrossUserCredentials checks that [ExecService.Get] configures a
// lookup for another user with the resolved UID and primary GID, without
// inheriting supplementary groups. An injected start failure prevents any
// actual credential-changing execution, so the test requires no privileges.
func (s *execServiceSuite) TestGetCrossUserCredentials(c *check.C) {
	credentials := &syscall.Credential{
		Gid: 1234,
		Uid: uint32(os.Geteuid()) ^ 1,
	}
	failure := errors.New("prevent credential-changing execution")
	command := &fakeExecCommand{
		path:     s.executable,
		startErr: failure,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return credentials, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        strconv.FormatUint(uint64(credentials.Uid), 10),
	}

	_, err := service.Get(context.Background(), request)

	c.Check(errors.Is(err, failure), check.Equals, true)
	c.Assert(command.cmd, check.NotNil)
	c.Check(command.cmd.Process, check.IsNil)
	c.Assert(command.cmd.SysProcAttr, check.NotNil)
	c.Check(
		command.cmd.SysProcAttr.Credential,
		check.DeepEquals,
		&syscall.Credential{Gid: 1234, Uid: uint32(os.Geteuid()) ^ 1},
	)
}

// TestGetEmptySecret accepts an explicitly empty successful response.
func (s *execServiceSuite) TestGetEmptySecret(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"","secret":""}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	value, err := service.Get(context.Background(), request)
	c.Assert(err, check.IsNil)
	defer value.Close()
	contents, err := io.ReadAll(value)
	c.Check(err, check.IsNil)
	c.Check(len(contents), check.Equals, 0)
}

// TestGetForwardsRequest checks that [ExecService.Get] prepares the configured
// executable with the collection and attributes encoded as JSON on stdin,
// no command-line arguments and an empty environment. The UID is passed to
// credential resolution rather than included in the JSON request. An injected
// start failure allows inspection without launching the command.
func (s *execServiceSuite) TestGetForwardsRequest(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	command := &fakeExecCommand{
		path:     s.executable,
		startErr: errors.New("inspect command without consuming stdin"),
	}
	service := ExecService{
		command:    command.Command,
		executable: filepath.Join(c.MkDir(), secretCommandName),
		resolveCredentials: func(uid string) (*syscall.Credential, error) {
			c.Check(uid, check.Equals, request.UID)
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	_, err := service.Get(context.Background(), request)
	c.Assert(errors.Is(err, command.startErr), check.Equals, true)
	c.Check(command.executable, check.Equals, service.executable)
	c.Check(len(command.args), check.Equals, 0)
	c.Assert(command.cmd.Env, check.NotNil)
	c.Check(len(command.cmd.Env), check.Equals, 0)

	input, err := io.ReadAll(command.cmd.Stdin)
	c.Assert(err, check.IsNil)
	var delegated DelegatedDBusRequest
	err = json.Unmarshal(input, &delegated)
	c.Assert(err, check.IsNil)
	c.Check(
		delegated,
		check.DeepEquals,
		DelegatedDBusRequest{
			Attributes: request.Attributes,
			Collection: request.Collection,
		},
	)

}

// TestGetMalformedResponse preserves the JSON decoding error.
func (s *execServiceSuite) TestGetMalformedResponse(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"secret":`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(context.Background(), request)

	c.Check(errors.Is(err, io.ErrUnexpectedEOF), check.Equals, true)
	c.Check(err, check.ErrorMatches, "decoding secret response: unexpected EOF")
}

// TestGetPreservesCancellation retains cancellation and an independent failure.
func (s *execServiceSuite) TestGetPreservesCancellation(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("independent start failure")
	command := &fakeExecCommand{
		path:     s.executable,
		startErr: failure,
	}
	service := ExecService{
		command: func(
			ctx context.Context,
			name string,
			args ...string,
		) *exec.Cmd {
			cancel()
			return command.Command(ctx, name, args...)
		},
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(ctx, request)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
	c.Check(errors.Is(err, failure), check.Equals, true)
	c.Check(command.cmd.Process, check.IsNil)
}

// TestGetRejectsInvalidRequest validates before resolving or executing.
func (s *execServiceSuite) TestGetRejectsInvalidRequest(c *check.C) {
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: " ",
		UID:        "1000",
	}
	command := &fakeExecCommand{}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			c.Fatal("invalid request must not resolve credentials")
			return nil, nil
		},
	}

	_, err := service.Get(context.Background(), request)

	c.Check(err, check.ErrorMatches, "secret request collection is missing")
	c.Check(command.cmd, check.IsNil)
}

// TestGetRejectsPreCancelledContext stops before resolving or executing.
func (s *execServiceSuite) TestGetRejectsPreCancelledContext(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := &fakeExecCommand{}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			c.Fatal("cancelled request must not resolve credentials")
			return nil, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	_, err := service.Get(ctx, request)

	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
	c.Check(command.cmd, check.IsNil)
}

// TestGetResponseCollectionAmbiguous matches the delegated sentinel error.
func (s *execServiceSuite) TestGetResponseCollectionAmbiguous(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"secret service collection is ambiguous"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	_, err := service.Get(context.Background(), request)
	c.Check(errors.Is(err, ErrorCollectionAmbiguous), check.Equals, true)
}

// TestGetResponseCollectionLocked matches the delegated sentinel error.
func (s *execServiceSuite) TestGetResponseCollectionLocked(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"secret service collection is locked"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	_, err := service.Get(context.Background(), request)
	c.Check(errors.Is(err, ErrorCollectionLocked), check.Equals, true)
}

// TestGetResponseCollectionNotFound matches the delegated sentinel error.
func (s *execServiceSuite) TestGetResponseCollectionNotFound(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"secret service collection not found"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	_, err := service.Get(context.Background(), request)
	c.Check(errors.Is(err, ErrorCollectionNotFound), check.Equals, true)
}

// TestGetResponseMultipleSecrets matches the delegated sentinel error.
func (s *execServiceSuite) TestGetResponseMultipleSecrets(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"multiple secrets match the request"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	_, err := service.Get(context.Background(), request)
	c.Check(errors.Is(err, ErrorMultipleSecrets), check.Equals, true)
}

// TestGetResponseSecretNotFound gives an error precedence over secret bytes.
func (s *execServiceSuite) TestGetResponseSecretNotFound(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"secret not found","secret":"c2VjcmV0"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}
	_, err := service.Get(context.Background(), request)
	c.Check(errors.Is(err, ErrorSecretNotFound), check.Equals, true)
}

// TestGetSameUserInheritsCredentials avoids privileged group changes.
func (s *execServiceSuite) TestGetSameUserInheritsCredentials(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"","secret":"c2VjcmV0"}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        strconv.Itoa(os.Geteuid()),
	}
	value, err := service.Get(context.Background(), request)
	c.Assert(err, check.IsNil)
	defer value.Close()
	c.Check(command.cmd.SysProcAttr, check.IsNil)
}

// TestGetSuccessBinary preserves non-text bytes through the JSON protocol.
func (s *execServiceSuite) TestGetSuccessBinary(c *check.C) {
	command := &fakeExecCommand{
		path:   s.executable,
		stdout: `{"error":"","secret":"AP8KgAA="}`,
	}
	service := ExecService{
		command: command.Command,
		resolveCredentials: func(string) (*syscall.Credential, error) {
			return &syscall.Credential{
				Gid: uint32(os.Getegid()),
				Uid: uint32(os.Geteuid()),
			}, nil
		},
	}
	request := Request{
		Attributes: map[string]string{"service": "example", "account": "test"},
		Collection: "default",
		UID:        "1000",
	}

	value, err := service.Get(context.Background(), request)
	c.Assert(err, check.IsNil)
	defer value.Close()
	contents, err := io.ReadAll(value)
	c.Check(err, check.IsNil)
	c.Check(contents, check.DeepEquals, []byte{0, 255, 10, 128, 0})
}

// TestMakeExecService selects a sibling executable rather than searching PATH.
func (s *execServiceSuite) TestMakeExecService(c *check.C) {
	service, err := MakeExecService()
	c.Assert(err, check.IsNil)
	c.Check(
		service.executable,
		check.Equals,
		filepath.Join(filepath.Dir(s.executable), secretCommandName),
	)
	c.Check(service.command, check.NotNil)
	c.Check(service.resolveCredentials, check.NotNil)
}

// TestParseExecIDInvalid rejects non-numeric credentials with a syntax error.
func (s *execServiceSuite) TestParseExecIDInvalid(c *check.C) {
	_, err := parseExecID("not-a-uid")
	c.Check(errors.Is(err, strconv.ErrSyntax), check.Equals, true)
}

// TestParseExecIDNegative rejects signed negative credentials.
func (s *execServiceSuite) TestParseExecIDNegative(c *check.C) {
	_, err := parseExecID("-1")
	c.Check(errors.Is(err, strconv.ErrSyntax), check.Equals, true)
}

// TestParseExecIDOverflow rejects credentials outside the uint32 range.
func (s *execServiceSuite) TestParseExecIDOverflow(c *check.C) {
	_, err := parseExecID("4294967296")
	c.Check(errors.Is(err, strconv.ErrRange), check.Equals, true)
}

// TestParseExecIDReserved rejects the Unix leave-unchanged credential value.
func (s *execServiceSuite) TestParseExecIDReserved(c *check.C) {
	_, err := parseExecID("4294967295")
	c.Check(err, check.ErrorMatches, "reserved process credential ID")
}

// TestParseExecIDValid accepts the highest non-reserved numeric credential.
func (s *execServiceSuite) TestParseExecIDValid(c *check.C) {
	id, err := parseExecID("4294967294")
	c.Assert(err, check.IsNil)
	c.Check(id, check.Equals, uint32(4294967294))
}

// TestParseExecIDZero accepts root as a numeric ID without looking it up.
func (s *execServiceSuite) TestParseExecIDZero(c *check.C) {
	id, err := parseExecID("0")
	c.Assert(err, check.IsNil)
	c.Check(id, check.Equals, uint32(0))
}
