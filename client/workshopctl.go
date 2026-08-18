/*
 * Copyright (C) 2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// WorkshopCtlOptions holds the various options with which workshopctl is invoked.
type WorkshopCtlOptions struct {
	// ContextID is a string used to determine the context of this call (e.g.
	// which context and handler should be used, etc.)
	ContextID string `json:"context-id"`

	// Args contains a list of parameters to use for this invocation.
	Args []string `json:"args"`
}

// WorkshopCtlPostData is the data posted to the daemon /v2/workshopctl endpoint
// TODO: this can be removed again once we no longer need to pass stdin data
// but instead use a real stdin stream
type WorkshopCtlPostData struct {
	WorkshopCtlOptions

	Stdin []byte `json:"stdin,omitempty"`
}

type workshopctlOutput struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// stdinReadLimit is the maximum number of bytes read from stdin when
// forwarding it to the daemon. Larger input is rejected to bound the size
// of the request body, as stdin is currently buffered in full rather than
// streamed.
const stdinReadLimit = 4_000_000

// RunWorkshopctl requests a workshopctl run for the given options.
func (client *Client) RunWorkshopctl(
	options *WorkshopCtlOptions,
	stdin io.Reader,
) (stdout, stderr []byte, err error) {
	// TODO: instead of reading all of stdin here we need to forward it to
	//       the daemon eventually
	var stdinData []byte
	if stdin != nil {
		limitedStdin := &io.LimitedReader{R: stdin, N: stdinReadLimit + 1}
		stdinData, err = io.ReadAll(limitedStdin)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot read stdin: %v", err)
		}
		if limitedStdin.N <= 0 {
			return nil, nil, fmt.Errorf("cannot read more than %v bytes of data from stdin", stdinReadLimit)
		}
	}

	b, err := json.Marshal(WorkshopCtlPostData{
		WorkshopCtlOptions: *options,
		Stdin:              stdinData,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("cannot marshal options: %s", err)
	}

	var output workshopctlOutput
	_, err = client.doSync("POST", "/v1/workshopctl", nil, nil, bytes.NewReader(b), &output)
	if err != nil {
		return nil, nil, err
	}

	return []byte(output.Stdout), []byte(output.Stderr), nil
}
