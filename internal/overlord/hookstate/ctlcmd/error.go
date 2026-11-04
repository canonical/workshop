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

package ctlcmd

// CommandExitCodeError carries a command's requested process exit code.
// Attach it to a descriptive error rather than returning it alone, as it
// provides no user-facing diagnostic.
type CommandExitCodeError struct {
	// ExitCode is the process exit code to return to the caller.
	ExitCode int
}

// Error returns an empty string because this error carries no diagnostic.
func (e CommandExitCodeError) Error() string {
	return ""
}
