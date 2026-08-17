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

import (
	"strings"
	"testing"

	"gopkg.in/check.v1"
)

type cloudConfigSuite struct{}

var _ = check.Suite(&cloudConfigSuite{})

func TestCloudConfig(t *testing.T) {
	check.TestingT(t)
}

// TestCloudConfigTemplateParses checks that the embedded cloud-init user-data
// template compiles without error, catching template syntax mistakes before
// release.
func (s *cloudConfigSuite) TestCloudConfigTemplateParses(c *check.C) {
	_, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)
}

// TestCloudConfigTemplateRendersVars checks that the cloud-init user-data
// template parses and that the template variables are interpolated into the
// rendered output.
func (s *cloudConfigSuite) TestCloudConfigTemplateRendersVars(c *check.C) {
	tmpl, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	vars := cloudConfigVars{
		WorkshopCtlPath:          "/wsp/bin/workshopctl",
		WorkshopSecretSocketPath: "/wsp/run/workshop.socket.secret",
		WorkshopStateDir:         "/wsp/state",
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, vars)
	c.Assert(err, check.IsNil)

	out := buf.String()
	c.Check(out, check.Matches, `(?s).*ListenStream=/wsp/run/workshop\.socket\.secret.*`)
	c.Check(out, check.Matches, `(?s).*ExecStart=/wsp/bin/workshopctl get-secret --systemd.*`)
	c.Check(out, check.Matches, `(?s).*- ln -sf /wsp/bin/workshopctl /usr/local/bin/workshopctl.*`)
	c.Check(out, check.Matches, `(?s).*- install --directory --mode=755 /project /usr/local/bin /usr/local/lib/workshop /wsp/state.*`)
}

// TestCloudConfigTemplateCached checks that the template is parsed once and
// the same instance is returned on subsequent calls.
func (s *cloudConfigSuite) TestCloudConfigTemplateCached(c *check.C) {
	first, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	second, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	c.Check(first == second, check.Equals, true)
}
