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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"

	"github.com/canonical/workshop/internal/secrets"
	"github.com/canonical/workshop/internal/secrets/provider/system"
)

// SecretService provides the host secret lookups required by the command.
type SecretService interface {
	// Get retrieves the unique secret matching the request for its user ID.
	// On success, ownership transfers to the caller, which must consume or
	// close the secret. It must honour context cancellation.
	Get(context.Context, system.Request) (secrets.Secret, error)
}

const (
	exitCodeSuccess           = 0
	exitCodeUnstructuredError = 2
)

// makeResponseFromError converts recognised lookup errors, including wrapped
// errors, into a [system.DelegatedDBusResponse] with the canonical error
// message.
// Recognised errors return a nil error; unrecognised errors are returned
// unchanged with an empty response. A nil input returns an empty response and
// a nil error.
func makeResponseFromError(err error) (system.DelegatedDBusResponse, error) {
	switch {
	case errors.Is(err, system.ErrorCollectionAmbiguous):
		return system.DelegatedDBusResponse{
			Error: system.ErrorCollectionAmbiguous.Error(),
		}, nil
	case errors.Is(err, system.ErrorCollectionLocked):
		return system.DelegatedDBusResponse{
			Error: system.ErrorCollectionLocked.Error(),
		}, nil
	case errors.Is(err, system.ErrorCollectionNotFound):
		return system.DelegatedDBusResponse{
			Error: system.ErrorCollectionNotFound.Error(),
		}, nil
	case errors.Is(err, system.ErrorMultipleSecrets):
		return system.DelegatedDBusResponse{
			Error: system.ErrorMultipleSecrets.Error(),
		}, nil
	case errors.Is(err, system.ErrorSecretNotFound):
		return system.DelegatedDBusResponse{
			Error: system.ErrorSecretNotFound.Error(),
		}, nil
	default:
		return system.DelegatedDBusResponse{}, err
	}
}

func main() {
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()

	var request system.DelegatedDBusRequest
	err := decoder.Decode(&request)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "decoding secret request: %v\n", err)
		os.Exit(exitCodeUnstructuredError)
	}

	ctx, ctxStop := signal.NotifyContext(context.Background(), os.Interrupt)

	res, err := run(
		ctx,
		strconv.Itoa(os.Geteuid()),
		system.NewDBusService(),
		request,
	)
	ctxStop()

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitCodeUnstructuredError)
	}

	outputEncoder := json.NewEncoder(os.Stdout)
	err = outputEncoder.Encode(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoding response: %v\n", err)
		os.Exit(exitCodeUnstructuredError)
	}
}

// run resolves request for the supplied effective user ID and returns a
// [system.DelegatedDBusResponse]. A successful lookup transfers ownership of
// the response's secret to the caller, which must consume or close it.
// Recognised lookup errors populate the response's Error field and return a
// nil error. Unrecognised service errors are returned unchanged with an empty
// response.
func run(
	ctx context.Context,
	uid string,
	service SecretService,
	request system.DelegatedDBusRequest,
) (system.DelegatedDBusResponse, error) {
	secretVal, err := service.Get(ctx, system.Request{
		Attributes: request.Attributes,
		Collection: request.Collection,
		UID:        uid,
	})

	if err != nil {
		return makeResponseFromError(err)
	}

	return system.DelegatedDBusResponse{
		Secret: system.SecretResponseValue{Secret: secretVal},
	}, nil
}
