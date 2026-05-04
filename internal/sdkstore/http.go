// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3.

package sdkstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"gopkg.in/httprequest.v1"

	"github.com/canonical/workshop/internal/https"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/sdkstore/path"
	"github.com/canonical/workshop/internal/version"
)

const (
	jsonContentType = "application/json"

	userAgentKey = "User-Agent"

	// defaultRetryAttempts defines the number of attempts that a default
	// HTTPClient will retry before giving up.
	// Retries are only performed on certain status codes, nothing in the 200 to
	// 400 range and a select few from the 500 range (deemed retryable):
	//
	// - http.StatusBadGateway
	// - http.StatusGatewayTimeout
	// - http.StatusServiceUnavailable
	// - http.StatusTooManyRequests
	//
	// See: internal/https package.
	defaultRetryAttempts = 3

	// defaultRetryDelay holds the amount of time after a try, a new attempt
	// will wait before another attempt.
	defaultRetryDelay = time.Second * 10

	// defaultRetryMaxDelay holds the amount of time before a giving up on a
	// request. This values includes any server response from the header
	// Retry-After.
	defaultRetryMaxDelay = time.Minute * 10
)

var userAgentValue = "Workshop/" + version.Version

// DefaultHTTPClient creates a new https.Client with the default configuration.
func DefaultHTTPClient() https.HTTPClient {
	return https.NewClient(
		https.WithRequestRecorder(loggingRequestRecorder{}),
		https.WithRequestRetrier(defaultRetryPolicy()),
	)
}

// defaultRetryPolicy returns a retry policy with sane defaults for most
// requests.
func defaultRetryPolicy() https.RetryPolicy {
	return https.RetryPolicy{
		Attempts: defaultRetryAttempts,
		Delay:    defaultRetryDelay,
		MaxDelay: defaultRetryMaxDelay,
	}
}

type loggingRequestRecorder struct{}

// Record an outgoing request which produced an http.Response.
func (loggingRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	logger.Debugf("request (method: %q, host: %q, path: %q, status: %q, duration: %s)", method, url.Host, url.Path, res.Status, rtt)
}

// RecordError records an outgoing request which returned an error.
func (loggingRequestRecorder) RecordError(method string, url *url.URL, err error) {
	logger.Debugf("request error (method: %q, host: %q, path: %q, err: %s)", method, url.Host, url.Path, err)
}

// apiRequester creates a wrapper around the https.HTTPClient to allow for better
// error handling.
type apiRequester struct {
	httpClient https.HTTPClient
	retryDelay time.Duration
}

// newAPIRequester creates a new https.HTTPClient for making requests to a server.
func newAPIRequester(httpClient https.HTTPClient) *apiRequester {
	return &apiRequester{
		httpClient: httpClient,
		retryDelay: 3 * time.Second,
	}
}

// Do performs the *http.Request and returns a *http.Response or an error.
//
// Handle empty response (io.EOF) errors specially and retry. The reason for
// this is we get these errors from the Store fairly regularly (they're not
// valid HTTP responses as there are no headers; they're empty responses).
func (t *apiRequester) Do(req *http.Request) (*http.Response, error) {
	// To retry requests with a body, we need to read the entire body in
	// up-front, otherwise it'll be empty on retries.
	var body []byte
	if req.Body != nil && req.GetBody == nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		err = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("closing request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}

	// Try a fixed number of attempts with a doubling delay in between.
	var resp *http.Response
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			var err error
			resp, err = t.doOnce(req) //nolint:bodyclose // resp.Body is closed by the caller.
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, io.EOF)
		},
		NotifyFunc: func(lastError error, attempt int) {
			logger.Noticef("SDK Store API error (attempt %d): %v", attempt, lastError)
		},
		Attempts: 2,
		Delay:    t.retryDelay,
		Clock:    clock.WallClock,
		Stop:     req.Context().Done(),
	})
	return resp, err
}

func (t *apiRequester) doOnce(req *http.Request) (*http.Response, error) {
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusNoContent {
		return resp, nil
	}

	var potentialInvalidURL bool
	if resp.StatusCode == http.StatusNotFound {
		potentialInvalidURL = true
	} else if resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode <= http.StatusNetworkAuthenticationRequired {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
		return nil, fmt.Errorf("server error: %s", http.StatusText(resp.StatusCode))
	}

	// We expect that we always have a valid content-type from the server, once
	// we've checked that we don't get a 5xx error. Given that we send Accept
	// header of application/json, I would only ever expect to see that.
	// Everything will be incorrectly formatted.
	if contentType := resp.Header.Get("Content-Type"); !isJSON(contentType) {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()

		if potentialInvalidURL {
			return nil, fmt.Errorf(`unexpected SDK Store URL %q when parsing headers`, req.URL.String())
		}
		return nil, fmt.Errorf(`unexpected content-type from server %q`, contentType)
	}

	return resp, nil
}

// Based on isJSONMediaType from gopkg.in/httprequest.v1.
func isJSON(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	suffix, found := strings.CutPrefix(mediaType, "application/")
	if !found {
		return false
	}
	for {
		before, after, found := strings.Cut(suffix, "+")
		if before == "json" {
			return true
		}
		if !found {
			return false
		}
		suffix = after
	}
}

// restResponse abstracts away the underlying response from the implementation.
type restResponse struct {
	StatusCode int
}

// RESTClient defines a type for making requests to a server.
type RESTClient interface {
	// Get performs GET requests to a given Path.
	Get(context.Context, path.Path, any) (restResponse, error)
	// Post performs POST requests to a given Path.
	Post(context.Context, path.Path, http.Header, any, any) (restResponse, error)
}

// httpRESTClient represents a RESTClient that expects to interact with an
// https.HTTPClient.
type httpRESTClient struct {
	httpClient https.HTTPClient
}

// newHTTPRESTClient creates a new httpRESTClient
func newHTTPRESTClient(httpClient https.HTTPClient) *httpRESTClient {
	return &httpRESTClient{
		httpClient: httpClient,
	}
}

// Get makes a GET request to the given path in the SDK Store (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *httpRESTClient) Get(ctx context.Context, path path.Path, result any) (restResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", path.String(), nil)
	if err != nil {
		return restResponse{}, fmt.Errorf("cannot make new request: %w", err)
	}

	// Compose the request headers.
	req.Header = make(http.Header)
	req.Header.Set("Accept", jsonContentType)
	req.Header.Set("Content-Type", jsonContentType)
	req.Header.Set(userAgentKey, userAgentValue)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return restResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return restResponse{}, fmt.Errorf("SDK Store client get: %w", err)
	}

	return restResponse{
		StatusCode: resp.StatusCode,
	}, nil
}

// Post makes a POST request to the given path in the SDK Store (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *httpRESTClient) Post(ctx context.Context, path path.Path, headers http.Header, body, result any) (restResponse, error) {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(body); err != nil {
		return restResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path.String(), buffer)
	if err != nil {
		return restResponse{}, fmt.Errorf("cannot make new request: %w", err)
	}

	// Compose the request headers.
	req.Header = make(http.Header)
	req.Header.Set("Accept", jsonContentType)
	req.Header.Set("Content-Type", jsonContentType)
	req.Header.Set(userAgentKey, userAgentValue)

	// Add any headers specific to this request (in sorted order).
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range headers[k] {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return restResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return restResponse{}, fmt.Errorf("SDK Store client post: %w", err)
	}
	return restResponse{
		StatusCode: resp.StatusCode,
	}, nil
}
