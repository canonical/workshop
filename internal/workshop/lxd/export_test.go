// Copyright (c) 2026 Canonical Ltd
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

package lxdbackend

type CNAME = cname

var (
	DefaultConfig      = (*Backend).workshopConfig
	ReadProjects       = readProjects
	SaveProjects       = saveProjects
	HandleImageUpdate  = handleImageUpdate
	CheckServerVersion = checkVersion
	GenerateCNAME      = generateCNAME
	PreferredDriver    = preferredDriver
	DriverSupported    = driverSupported
)

func MockFirewallChecker(f func(string) string) func() {
	old := firewallChecker
	firewallChecker = f
	return func() {
		firewallChecker = old
	}
}

// Exported for testing.
var (
	AnalyzeNftJSON       = analyzeNftJSON
	BridgeBlockedWarning = bridgeBlockedWarning
	CauseUnknown         = causeUnknown
	CauseDocker          = causeDocker
	CauseUFW             = causeUFW
)
