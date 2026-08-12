package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/canonical/workshop/internal/osutil"
)

// RoundTripperFunc adapts a function to an [http.RoundTripper].
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTripperWrapper decorates an [http.RoundTripper].
type RoundTripperWrapper func(http.RoundTripper) http.RoundTripper

// workshopInstanceIDHeader identifies the Workshop instance in API requests.
const workshopInstanceIDHeader = "workshop-instance-id"

// NewWorkshopInstanceIDRoundTripper returns a wrapper that sends the workshop
// instance identifier with every request. It writes a warning to warnings and
// leaves requests unchanged when the instance identifier is not found. Warnings
// are written sequentially, so warnings does not need to be safe for concurrent
// use.
func NewWorkshopInstanceIDRoundTripper(warnings io.Writer) RoundTripperWrapper {
	return newWorkshopInstanceIDRoundTripper(osutil.WorkshopInstanceID, warnings)
}

// newWorkshopInstanceIDRoundTripper returns a wrapper that gets its instance
// identifier from readWorkshopInstanceID.
func newWorkshopInstanceIDRoundTripper(
	readWorkshopInstanceID func() (string, error),
	warnings io.Writer,
) RoundTripperWrapper {
	instanceID, err := readWorkshopInstanceID()
	if errors.Is(err, osutil.ErrorWorkshopInstanceIDNotFound) {
		fmt.Fprintf(
			warnings,
			"warning: cannot read workshop instance ID: %v; ",
			err,
		)
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	} else if err != nil {
		instanceIDError := fmt.Errorf("cannot read workshop instance ID: %w", err)
		return func(http.RoundTripper) http.RoundTripper {
			return RoundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, instanceIDError
			})
		}
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.Header.Set(workshopInstanceIDHeader, instanceID)
			return next.RoundTrip(req)
		})
	}
}

// RoundTrip implements [http.RoundTripper] by calling fn with req.
func (fn RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
