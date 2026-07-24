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

// workshopMachineIDHeader identifies the Workshop machine in API requests.
const workshopMachineIDHeader = "workshop-machine-id"

// NewMachineIDRoundTripper returns a wrapper that sends the workshop machine
// identifier with every request. It writes a warning to warnings and leaves
// requests unchanged when the machine identifier is not found. Warnings are
// written sequentially, so warnings does not need to be safe for concurrent
// use.
func NewMachineIDRoundTripper(warnings io.Writer) RoundTripperWrapper {
	return newMachineIDRoundTripper(osutil.MachineID, warnings)
}

// newMachineIDRoundTripper returns a wrapper that gets its machine identifier
// from readMachineID.
func newMachineIDRoundTripper(
	readMachineID func() (string, error),
	warnings io.Writer,
) RoundTripperWrapper {
	machineID, err := readMachineID()
	if errors.Is(err, osutil.ErrorMachineIDNotFound) {
		fmt.Fprintf(
			warnings,
			"warning: cannot read workshop machine ID: %v; ",
			err,
		)
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	} else if err != nil {
		machineIDError := fmt.Errorf("cannot read machine ID: %w", err)
		return func(http.RoundTripper) http.RoundTripper {
			return RoundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, machineIDError
			})
		}
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.Header.Set(workshopMachineIDHeader, machineID)
			return next.RoundTrip(req)
		})
	}
}

// RoundTrip implements [http.RoundTripper] by calling fn with req.
func (fn RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
