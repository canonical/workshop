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
	"github.com/canonical/workshop/internal/secrets/provider/secretservice"
)

// SecretService provides the host secret lookups required by the command.
type SecretService interface {
	// Get retrieves the unique secret matching the request for its user ID.
	// On success, ownership transfers to the caller, which must consume or
	// close the secret. It must honour context cancellation.
	Get(context.Context, secretservice.Request) (secrets.Secret, error)
}

const (
	exitCodeSuccess           = 0
	exitCodeUnstructuredError = 2
)

// makeResponseFromError converts recognised lookup errors, including wrapped
// errors, into a [secretservice.DelegatedDBusResponse] with the canonical error
// message.
// Recognised errors return a nil error; unrecognised errors are returned
// unchanged with an empty response. A nil input returns an empty response and
// a nil error.
func makeResponseFromError(
	err error,
) (secretservice.DelegatedDBusResponse, error) {
	switch {
	case errors.Is(err, secretservice.ErrorCollectionAmbiguous):
		return secretservice.DelegatedDBusResponse{
			Error: secretservice.ErrorCollectionAmbiguous.Error(),
		}, nil
	case errors.Is(err, secretservice.ErrorCollectionLocked):
		return secretservice.DelegatedDBusResponse{
			Error: secretservice.ErrorCollectionLocked.Error(),
		}, nil
	case errors.Is(err, secretservice.ErrorCollectionNotFound):
		return secretservice.DelegatedDBusResponse{
			Error: secretservice.ErrorCollectionNotFound.Error(),
		}, nil
	case errors.Is(err, secretservice.ErrorMultipleSecrets):
		return secretservice.DelegatedDBusResponse{
			Error: secretservice.ErrorMultipleSecrets.Error(),
		}, nil
	case errors.Is(err, secretservice.ErrorSecretNotFound):
		return secretservice.DelegatedDBusResponse{
			Error: secretservice.ErrorSecretNotFound.Error(),
		}, nil
	default:
		return secretservice.DelegatedDBusResponse{}, err
	}
}

func main() {
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()

	var request secretservice.DelegatedDBusRequest
	err := decoder.Decode(&request)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "decoding secret request: %v\n", err)
		os.Exit(exitCodeUnstructuredError)
	}

	ctx, ctxStop := signal.NotifyContext(context.Background(), os.Interrupt)

	res, err := run(
		ctx,
		strconv.Itoa(os.Geteuid()),
		secretservice.NewDBusClient(),
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
// [secretservice.DelegatedDBusResponse]. A successful lookup transfers ownership
// of the response's secret to the caller, which must consume or close it.
// Recognised lookup errors populate the response's Error field and return a
// nil error. Unrecognised service errors are returned unchanged with an empty
// response.
func run(
	ctx context.Context,
	uid string,
	service SecretService,
	request secretservice.DelegatedDBusRequest,
) (secretservice.DelegatedDBusResponse, error) {
	secretVal, err := service.Get(ctx, secretservice.Request{
		Attributes: request.Attributes,
		Collection: request.Collection,
		UID:        uid,
	})

	if err != nil {
		return makeResponseFromError(err)
	}

	return secretservice.DelegatedDBusResponse{
		Secret: secretservice.SecretResponseValue{Secret: secretVal},
	}, nil
}
