// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sdkstore

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/retry"

	"github.com/canonical/workshop/internal/https"
	"github.com/canonical/workshop/internal/sdkstore/path"
	"github.com/canonical/workshop/internal/sdkstore/transport"
)

type resolveClient struct {
	path   path.Path
	client RESTClient
	clock  clock.Clock
}

func newResolveClient(path path.Path, client RESTClient) *resolveClient {
	return &resolveClient{
		path:   path,
		client: client,
		clock:  clock.WallClock,
	}
}

type resolveResponse struct {
	transport.ResolveResponse
	transport.ErrorResponse
}

// Resolve is used to find the latest revisions of a given set of SDKs.
func (c *resolveClient) Resolve(ctx context.Context, req transport.ResolveRequest) (transport.ResolveResponse, error) {
	var resp resolveResponse
	// Resolve is a POST request so it won't be retried automatically.
	// We retry it here because duplicate resolve requests are harmless,
	// and we need to construct the request body from scratch.
	err := retry.Call(retry.CallArgs{
		Clock: c.clock,
		Func: func() error {
			var attempt resolveResponse
			if err := c.resolve(ctx, &attempt, req); err != nil {
				return err
			}
			resp = attempt
			return nil
		},
		IsFatalError: func(err error) bool {
			return !https.IsRetryableNetworkError(err)
		},
		Attempts: defaultRetryAttempts,
		Delay:    defaultRetryDelay,
		Stop:     ctx.Done(),
	})
	if err != nil {
		return transport.ResolveResponse{}, err
	}

	if err := handleBasicAPIErrors(resp.ErrorList); err != nil {
		return resp.ResolveResponse, err
	}

	return resp.ResolveResponse, nil
}

func (c *resolveClient) resolve(ctx context.Context, resp any, req transport.ResolveRequest) error {
	_, err := c.client.Post(ctx, c.path, nil, req, resp)
	return err
}
