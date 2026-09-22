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
	"io"
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/sdk/system/secret"
	"github.com/canonical/workshop/internal/secrets"
)

// commandSuite tests workshopsyssecret request handling and JSON responses.
type commandSuite struct{}

var _ = check.Suite(&commandSuite{})

// Test runs the command suite.
func Test(t *testing.T) {
	check.TestingT(t)
}

// TestRun checks request and UID forwarding without consuming or closing
// the returned secret.
func (s *commandSuite) TestRun(c *check.C) {
	value := secrets.NewSecret([]byte{'a', 0, 0xff, '\n'})
	defer value.Close()

	service := stubService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{
			Attributes: map[string]string{"app": "example"},
			Collection: "default",
			UID:        "1001",
		})
		return value, nil
	})

	request := Request{
		Attributes: map[string]string{"app": "example"},
		Collection: "default",
	}

	got, err := run(context.Background(), "1001", service, request)

	c.Assert(err, check.IsNil)
	c.Check(got.Error, check.Equals, "")
	c.Check(got.Secret.Secret, check.Equals, value)
	data, err := io.ReadAll(got.Secret)
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(data, check.DeepEquals, []byte{'a', 0, 0xff, '\n'})
}

// TestRunCancellation checks a cancelled lookup returns context.Canceled
// without requiring a particular context instance or a service call.
func (s *commandSuite) TestRunCancellation(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := stubService(func(
		gotContext context.Context,
		_ secret.Request,
	) (secrets.Secret, error) {
		return secrets.Secret{}, gotContext.Err()
	})
	request := Request{
		Attributes: map[string]string{"app": "example"},
		Collection: "default",
	}

	value, err := run(ctx, "1001", service, request)

	c.Check(value, check.Equals, Response{})
	c.Check(errors.Is(err, context.Canceled), check.Equals, true)
}

// TestRunInvalidRequest checks validation is left to the service and its
// error remains identifiable through wrapping.
func (s *commandSuite) TestRunInvalidRequest(c *check.C) {
	validationErr := errors.New("secret request collection is missing")
	service := stubService(func(
		_ context.Context,
		request secret.Request,
	) (secrets.Secret, error) {
		c.Check(request, check.DeepEquals, secret.Request{UID: "1001"})
		return secrets.Secret{}, validationErr
	})
	value, err := run(context.Background(), "1001", service, Request{})

	c.Check(value, check.Equals, Response{})
	c.Check(errors.Is(err, validationErr), check.Equals, true)
}

// TestRunServiceError checks unrecognised errors remain identifiable through
// wrapping and return an empty response rather than response error data.
func (s *commandSuite) TestRunServiceError(c *check.C) {
	serviceErr := errors.New("service unavailable")
	service := stubService(func(
		context.Context,
		secret.Request,
	) (secrets.Secret, error) {
		return secrets.Secret{}, serviceErr
	})
	request := Request{
		Attributes: map[string]string{"app": "example"},
		Collection: "default",
	}

	got, err := run(context.Background(), "1001", service, request)

	c.Check(errors.Is(err, serviceErr), check.Equals, true)
	c.Check(got, check.Equals, Response{})
}

// TestResponseError checks an error response uses a lowercase string field
// and omits the zero-value secret.
func (s *commandSuite) TestResponseError(c *check.C) {
	data, err := json.Marshal(Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})

	c.Assert(err, check.IsNil)
	var response map[string]string
	err = json.Unmarshal(data, &response)
	c.Assert(err, check.IsNil)
	c.Check(response, check.DeepEquals, map[string]string{
		"error": "secret not found",
	})
}

// TestResponseSecretBinary checks arbitrary bytes are encoded as base64
// without an error field, consuming the secret.
func (s *commandSuite) TestResponseSecretBinary(c *check.C) {
	value := secrets.NewSecret([]byte{'a', 0, 0xff, '\n'})
	defer value.Close()

	data, err := json.Marshal(Response{
		Secret: SecretResponseValue{value},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	var response map[string]string
	err = json.Unmarshal(data, &response)
	c.Assert(err, check.IsNil)
	c.Check(response, check.DeepEquals, map[string]string{
		"secret": "YQD/Cg==",
	})

	var remaining [1]byte
	n, err := value.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)
}

// TestResponseSecretEmpty checks an empty secret is encoded as an empty
// string, rather than omitted or accompanied by an error field.
func (s *commandSuite) TestResponseSecretEmpty(c *check.C) {
	value := secrets.NewSecret([]byte{})
	defer value.Close()

	data, err := json.Marshal(Response{
		Secret: SecretResponseValue{value},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestResponseSecretNilBytes checks a secret constructed from nil bytes is
// encoded as an empty string, not null or an omitted field.
func (s *commandSuite) TestResponseSecretNilBytes(c *check.C) {
	value := secrets.NewSecret(nil)
	defer value.Close()

	data, err := json.Marshal(Response{
		Secret: SecretResponseValue{value},
	})
	defer clear(data)
	c.Assert(err, check.IsNil)
	c.Check(string(data), check.Equals, `{"secret":""}`)
}

// TestMarshalJSONConsumesSecret checks a copied wrapper shares consumption
// and cannot marshal the original bytes a second time.
func (s *commandSuite) TestMarshalJSONConsumesSecret(c *check.C) {
	value := secrets.NewSecret([]byte("hello"))
	defer value.Close()
	responseValue := SecretResponseValue{value}
	copyValue := responseValue

	first, err := responseValue.MarshalJSON()
	defer clear(first)
	c.Assert(err, check.IsNil)
	c.Check(string(first), check.Equals, `"aGVsbG8="`)

	second, err := copyValue.MarshalJSON()
	defer clear(second)
	c.Assert(err, check.IsNil)
	c.Check(string(second), check.Equals, `""`)
}

// TestMakeResponseFromErrorCollectionAmbiguous checks an ambiguous collection
// becomes a canonical response error rather than a returned error.
func (s *commandSuite) TestMakeResponseFromErrorCollectionAmbiguous(c *check.C) {
	response, err := makeResponseFromError(secret.ErrorCollectionAmbiguous)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorCollectionAmbiguous.Error(),
	})
}

// TestMakeResponseFromErrorCollectionLocked checks a locked collection becomes
// a canonical response error rather than a returned error.
func (s *commandSuite) TestMakeResponseFromErrorCollectionLocked(c *check.C) {
	response, err := makeResponseFromError(secret.ErrorCollectionLocked)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorCollectionLocked.Error(),
	})
}

// TestMakeResponseFromErrorCollectionNotFound checks a missing collection
// becomes a canonical response error rather than a returned error.
func (s *commandSuite) TestMakeResponseFromErrorCollectionNotFound(c *check.C) {
	response, err := makeResponseFromError(secret.ErrorCollectionNotFound)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorCollectionNotFound.Error(),
	})
}

// TestMakeResponseFromErrorMultipleSecrets checks ambiguous secret matches
// become a canonical response error rather than a returned error.
func (s *commandSuite) TestMakeResponseFromErrorMultipleSecrets(c *check.C) {
	response, err := makeResponseFromError(secret.ErrorMultipleSecrets)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorMultipleSecrets.Error(),
	})
}

// TestMakeResponseFromErrorSecretNotFound checks a missing secret becomes a
// canonical response error rather than a returned error.
func (s *commandSuite) TestMakeResponseFromErrorSecretNotFound(c *check.C) {
	response, err := makeResponseFromError(secret.ErrorSecretNotFound)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})
}

// TestMakeResponseFromErrorWrapped checks a wrapped recognised error is
// identified and its wrapping context is excluded from the response.
func (s *commandSuite) TestMakeResponseFromErrorWrapped(c *check.C) {
	lookupErr := fmt.Errorf("lookup: %w", secret.ErrorSecretNotFound)

	response, err := makeResponseFromError(lookupErr)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})
}

// TestRunRecognisedError checks run converts a recognised service error into
// response data with no returned error or secret.
func (s *commandSuite) TestRunRecognisedError(c *check.C) {
	service := stubService(func(
		context.Context,
		secret.Request,
	) (secrets.Secret, error) {
		return secrets.Secret{}, secret.ErrorSecretNotFound
	})
	request := Request{
		Attributes: map[string]string{"app": "example"},
		Collection: "default",
	}

	response, err := run(context.Background(), "1001", service, request)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})
}

// TestMakeResponseFromErrorNil checks nil produces an empty response and no
// returned error.
func (s *commandSuite) TestMakeResponseFromErrorNil(c *check.C) {
	response, err := makeResponseFromError(nil)

	c.Check(err, check.IsNil)
	c.Check(response, check.Equals, Response{})
}

// TestMakeResponseFromErrorUnknown checks an unrecognised error remains
// identifiable with an empty response, even if its text matches a known error.
func (s *commandSuite) TestMakeResponseFromErrorUnknown(c *check.C) {
	unknown := errors.New(secret.ErrorSecretNotFound.Error())

	response, err := makeResponseFromError(unknown)

	c.Check(errors.Is(err, unknown), check.Equals, true)
	c.Check(response, check.Equals, Response{})
}

// TestResponseErrorPartialWrite checks a partial error-response write returns
// the writer error and leaves incomplete JSON.
func (s *commandSuite) TestResponseErrorPartialWrite(c *check.C) {
	writeErr := errors.New("output pipe closed")
	output := &failingWriter{err: writeErr, limit: 5}

	err := json.NewEncoder(output).Encode(Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})

	c.Check(errors.Is(err, writeErr), check.Equals, true)
	c.Check(output.output.Len(), check.Equals, 5)
	c.Check(json.Valid(output.output.Bytes()), check.Equals, false)
}

// TestResponseErrorWriteFailure checks an error-response write can fail
// without producing output and preserves the writer error.
func (s *commandSuite) TestResponseErrorWriteFailure(c *check.C) {
	writeErr := errors.New("output pipe closed")
	output := &failingWriter{err: writeErr, limit: 0}

	err := json.NewEncoder(output).Encode(Response{
		Error: secret.ErrorSecretNotFound.Error(),
	})

	c.Check(errors.Is(err, writeErr), check.Equals, true)
	c.Check(output.output.Len(), check.Equals, 0)
}

// TestResponseSecretPartialWrite checks a partial secret-response write
// preserves the writer error and leaves the secret consumed and JSON incomplete.
func (s *commandSuite) TestResponseSecretPartialWrite(c *check.C) {
	value := secrets.NewSecret([]byte("private value"))
	defer value.Close()
	writeErr := errors.New("output pipe closed")
	output := &failingWriter{err: writeErr, limit: 5}

	err := json.NewEncoder(output).Encode(Response{
		Secret: SecretResponseValue{value},
	})

	c.Check(errors.Is(err, writeErr), check.Equals, true)
	c.Check(output.output.Len(), check.Equals, 5)
	c.Check(json.Valid(output.output.Bytes()), check.Equals, false)
	var remaining [1]byte
	n, err := value.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)
}

// TestResponseSecretWriteFailure checks the secret is consumed even when
// writing its response fails without producing output.
func (s *commandSuite) TestResponseSecretWriteFailure(c *check.C) {
	value := secrets.NewSecret([]byte("private value"))
	defer value.Close()
	writeErr := errors.New("output pipe closed")
	output := &failingWriter{err: writeErr, limit: 0}

	err := json.NewEncoder(output).Encode(Response{
		Secret: SecretResponseValue{value},
	})

	c.Check(errors.Is(err, writeErr), check.Equals, true)
	c.Check(output.output.Len(), check.Equals, 0)
	var remaining [1]byte
	n, err := value.Read(remaining[:])
	c.Check(n, check.Equals, 0)
	c.Check(errors.Is(err, io.EOF), check.Equals, true)
}
