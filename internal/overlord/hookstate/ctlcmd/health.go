/*
 * Copyright (C) 2019 Canonical Ltd
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

package ctlcmd

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
)

var (
	shortHealthHelp = "Report the health status of an SDK"
	longHealthHelp  = `
 The set-health command is called from within a workshop to inform the system of the
 SDK's overall health.
 
 It can be called from any hook. An SDK can
 optionally provide a 'check-health' hook to manage these calls, which is
 then called periodically and with increased frequency while the SDK is
 "waiting". Any health regression will issue a warning to the user.
 
 - status: One of okay, waiting, error.
 
 - error-code: An optional note matching regex '[a-z](?:-?[a-z0-9])+', e.g. missing-cuda; up to 20 symbols.
 
 - message: A user-friendly message expanding the status, 7-70 lines long. Required if the status is 'waiting' or 'error'.
 `
)

func init() {
	addCommand("set-health", shortHealthHelp, longHealthHelp, func() command { return &healthCommand{} })
}

type healthPositional struct {
	Status  string `positional-arg-name:"<status>" required:"yes" description:"a valid health status; required."`
	Message string `positional-arg-name:"<message>" description:"a short human-readable explanation of the status (when not okay). Must be longer than 7 characters, and will be truncated if over 70. Message cannot be provided if status is okay, but is required otherwise."`
}

type healthCommand struct {
	baseCommand
	healthPositional `positional-args:"yes"`
	Code             string `long:"code" value-name:"<code>" description:"optional tool-friendly value representing the problem that makes the SDK unhealthy.  Not a number, but a word with 3-30 characters matching [a-z](-?[a-z0-9])+"`
}

var validCode = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`).MatchString

func (c *healthCommand) Execute([]string) error {
	if c.Status == "okay" && (len(c.Message) > 0 || len(c.Code) > 0) {
		return fmt.Errorf(`when status is "okay", message and code must be empty`)
	}

	status, err := healthstate.SetHealthLookup(c.Status)
	if err != nil {
		return err
	}
	if status == healthstate.CheckUnknown {
		return fmt.Errorf(`status cannot be manually set to "unknown"`)
	}

	if len(c.Code) > 0 {
		if len(c.Code) < 3 || len(c.Code) > 30 {
			return fmt.Errorf("code must have between 3 and 30 characters, got %d", len(c.Code))
		}
		if !validCode(c.Code) {
			return fmt.Errorf("invalid code %q (code must start with lowercase ASCII letters, and contain only ASCII letters and numbers, optionally separated by single dashes)", c.Code) // technically not dashes but hyphen-minuses
		}
	}

	if status != healthstate.CheckOkay {
		if len(c.Message) == 0 {
			return fmt.Errorf(`when status is not "okay", message is required`)
		}

		rmsg := []rune(c.Message)
		if len(rmsg) < 7 {
			return fmt.Errorf(`message must be at least 7 characters long (got %d)`, len(rmsg))
		}
		if len(rmsg) > 70 {
			c.Message = string(rmsg[:69]) + "…"
		}
	}

	ctx, err := c.ensureContext()
	if err != nil {
		return err
	}
	ctx.Lock()
	defer ctx.Unlock()

	if ctx.HookName() != hookstate.CheckHealth.String() {
		return errors.New(`"set-health" is only allowed from a "check-health" hook`)
	}

	health := &healthstate.HealthCheck{
		Sdk:         ctx.Sdk(),
		Timestamp:   time.Now(),
		CheckResult: status,
		Message:     c.Message,
		Code:        c.Code,
	}

	ctx.Set("health", health)

	return nil
}
