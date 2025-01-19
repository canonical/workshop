// Copyright (c) 2014-2020 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package daemon

import (
	"github.com/gorilla/mux"

	"github.com/canonical/workshop/internal/overlord/state"
)

var api = []*Command{{
	// See daemon.go:canAccess for details how the access is controlled.
	Path:    "/v1/projects",
	GuestOK: false,
	UserOK:  true,
	GET:     v1GetProjects,
	POST:    v1PostProjects,
}, {
	Path:    "/v1/projects/{id}/workshops",
	GuestOK: false,
	UserOK:  true,
	GET:     v1GetProjectWorkshops,
	POST:    v1PostProjectWorkshop,
}, {
	Path:    "/v1/projects/{id}/workshops/{name}/exec",
	GuestOK: false,
	UserOK:  true,
	POST:    v1PostWorkshopExec,
}, {
	Path:    "/v1/projects/{id}/workshops/{name}",
	GuestOK: false,
	UserOK:  true,
	GET:     v1GetProjectWorkshop,
}, {
	Path:    "/v1/projects/{id}/workshops/{name}/mounts",
	GuestOK: false,
	UserOK:  true,
	POST:    v1PostWorkshopMount,
}, {
	Path:    "/v1/projects/{id}/workshops/{name}/scripts",
	GuestOK: false,
	UserOK:  true,
	GET:     v1GetProjectWorkshopScripts,
}, {
	Path:    "/v1/connections",
	GuestOK: false,
	UserOK:  true,
	GET:     v1GetConnections,
	POST:    v1PostConnections,
}, {
	Path:   "/v1/changes",
	UserOK: true,
	GET:    v1GetChanges,
}, {
	Path:   "/v1/warnings",
	UserOK: true,
	GET:    v1GetWarnings,
	POST:   v1PostWarnings,
}, {
	Path:   "/v1/changes/{id}",
	UserOK: true,
	GET:    v1GetChange,
	POST:   v1PostChange,
}, {
	Path:   "/v1/changes/{id}/wait",
	UserOK: true,
	GET:    v1GetChangeWait,
}, {
	Path:   "/v1/tasks/{task-id}/websocket/{websocket-id}",
	UserOK: true,
	GET:    v1GetTaskWebsocket,
}, {
	Path:        "/v1/workshopctl",
	UserOK:      true,
	UntrustedOK: true,
	POST:        v1PostWorkshopCtl,
},
}

var (
	stateEnsureBefore = (*state.State).EnsureBefore
	muxVars           = mux.Vars
)
