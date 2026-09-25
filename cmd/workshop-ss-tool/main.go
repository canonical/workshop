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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"

	"github.com/canonical/workshop/internal/sdk/system/secret"
	"github.com/canonical/workshop/internal/secrets"
)

// Request defines the workshop-ss-tool command's JSON input for a lookup
// against a user's host Secret Service.
type Request struct {
	Attributes map[string]string `json:"attributes"`
	Collection string            `json:"collection"`
}

// Response carries either a secret on success or a recognised lookup error.
// Error contains the canonical service error message, without wrapping context.
// A zero-value Secret is omitted; a retrieved secret is encoded as a base64
// JSON string, including an empty string for a successfully retrieved empty
// value.
type Response struct {
	Error  string              `json:"error,omitempty"`
	Secret SecretResponseValue `json:"secret,omitzero"`
}

// SecretResponseValue exposes a [secrets.Secret] as a base64 JSON string.
// Marshalling consumes the secret, including through copies of this value.
// Callers must close the embedded secret if they abandon the response without
// marshalling it or if marshalling fails before the secret is fully consumed.
type SecretResponseValue struct {
	secrets.Secret
}

// SecretService provides the host secret lookups required by the command.
type SecretService interface {
	// Get retrieves the unique secret matching the request for its user ID.
	// On success, ownership transfers to the caller, which must consume or
	// close the secret. It must honour context cancellation.
	Get(context.Context, secret.Request) (secrets.Secret, error)
}

const (
	exitCodeSuccess           = 0
	exitCodeUnstructuredError = 2
)

// makeResponseFromError converts recognised lookup errors, including wrapped
// errors, into a [Response] containing the canonical service error message.
// Recognised errors return a nil error; unrecognised errors are returned
// unchanged with an empty response. A nil input returns an empty response and
// a nil error.
func makeResponseFromError(err error) (Response, error) {
	switch {
	case errors.Is(err, secret.ErrorCollectionAmbiguous):
		return Response{
			Error: secret.ErrorCollectionAmbiguous.Error(),
		}, nil
	case errors.Is(err, secret.ErrorCollectionLocked):
		return Response{
			Error: secret.ErrorCollectionLocked.Error(),
		}, nil
	case errors.Is(err, secret.ErrorCollectionNotFound):
		return Response{
			Error: secret.ErrorCollectionNotFound.Error(),
		}, nil
	case errors.Is(err, secret.ErrorMultipleSecrets):
		return Response{
			Error: secret.ErrorMultipleSecrets.Error(),
		}, nil
	case errors.Is(err, secret.ErrorSecretNotFound):
		return Response{
			Error: secret.ErrorSecretNotFound.Error(),
		}, nil
	default:
		return Response{}, err
	}
}

func main() {
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()

	var request Request
	err := decoder.Decode(&request)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "decoding secret request: %v\n", err)
		os.Exit(exitCodeUnstructuredError)
	}

	ctx, ctxStop := signal.NotifyContext(context.Background(), os.Interrupt)

	res, err := run(
		ctx,
		strconv.Itoa(os.Geteuid()),
		secret.NewDBusService(),
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

// MarshalJSON implements [json.Marshaler] by consuming the secret and encoding
// its bytes as a base64 JSON string. Reads clear consumed bytes, and the
// temporary buffer is cleared on return. The encoded result remains sensitive.
// Repeated calls cannot reproduce the original value after it is consumed.
// This method does not close the secret; callers must close any unread remainder
// if marshalling fails.
func (s SecretResponseValue) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	defer func() { clear(buf.Bytes()) }()

	_, err := io.Copy(&buf, s.Secret)
	if err != nil {
		return nil, err
	}
	return json.Marshal(buf.Bytes())
}

// run resolves request for the supplied effective user ID and returns a
// [Response]. A successful lookup transfers ownership of the response's secret
// to the caller, which must consume or close it. Recognised lookup errors
// populate the response's Error field and return a nil error. Unrecognised
// service errors are returned unchanged with an empty response.
func run(
	ctx context.Context,
	uid string,
	service SecretService,
	request Request,
) (Response, error) {
	secretVal, err := service.Get(ctx, secret.Request{
		Attributes: request.Attributes,
		Collection: request.Collection,
		UID:        uid,
	})

	if err != nil {
		return makeResponseFromError(err)
	}

	return Response{
		Secret: SecretResponseValue{secretVal},
	}, nil
}
