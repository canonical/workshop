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

// Command workshopsyssecret retrieves one secret from the host Secret Service.
// It reads the first JSON-encoded [Request] from stdin:
//
//	{"collection":"default","attributes":{"app":"example"}}
//
// Stdout contains a JSON [Response] with either a base64-encoded "secret" or
// an "error" string. A successful empty secret is encoded as {"secret":""}.
// Recognised lookup failures use the canonical service error message:
//
//	{"error":"secret service collection is locked"}
//
// Recognised failures include locked or missing collections, ambiguous
// collection labels, and missing or multiple matching secrets. Error strings
// contain neither wrapping context nor numeric codes.
//
// The current implementation exits with status 0 after writing either kind of
// response. Callers must inspect "error" to distinguish a recognised lookup
// failure from a successful lookup. Unstructured errors exit with status 2
// and report a diagnostic on stderr instead.
//
// Malformed or invalid requests, cancellation, unexpected service errors and
// response-write failures produce unstructured errors. For exit status 2,
// callers must discard any partial stdout. Never log secret responses.
//
// The caller must launch workshopsyssecret as the intended user. The command
// uses its effective UID and does not switch credentials. The user's session
// bus and Secret Service must already be available. The command resolves the
// collection, checks its lock state and retrieves the unique matching secret
// over D-Bus.
package main
