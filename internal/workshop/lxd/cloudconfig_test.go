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

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/workshop"
)

// cloudConfigSuite checks the cloud-init user-data template and its variables.
type cloudConfigSuite struct{}

var _ = check.Suite(&cloudConfigSuite{})

// TestCloudConfigTemplateCached checks that the template is parsed once and
// the same instance is returned on subsequent calls.
func (s *cloudConfigSuite) TestCloudConfigTemplateCached(c *check.C) {
	first, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	second, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	c.Check(first == second, check.Equals, true)
}

// TestCloudConfigTemplateParses checks that the cloud-init user-data template
// compiles without error, catching template syntax mistakes before release.
func (s *cloudConfigSuite) TestCloudConfigTemplateParses(c *check.C) {
	_, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)
}

// TestCloudConfigTemplateRendersVars checks that the template variables are
// interpolated into the rendered user-data.
func (s *cloudConfigSuite) TestCloudConfigTemplateRendersVars(c *check.C) {
	tmpl, err := cloudConfigTemplate()
	c.Assert(err, check.IsNil)

	vars := cloudConfigVars{
		FsFreezePath:             "/wsp/bin/fsfreeze",
		HasGRUB:                  true,
		StartTimeout:             123,
		WorkshopCtlPath:          "/wsp/bin/workshopctl",
		WorkshopSecretSocketPath: "/wsp/run/workshop.socket.secret",
		WorkshopStateDir:         "/wsp/state",
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, vars)
	c.Assert(err, check.IsNil)

	out := buf.String()
	c.Check(out, check.Matches,
		`(?s).*Environment=WORKSHOP_WAITREADY_TIMEOUT_NS=123.*`)
	c.Check(out, check.Matches,
		`(?s).*- ln -sf /wsp/bin/workshopctl /usr/local/bin/workshopctl.*`)
	c.Check(out, check.Matches,
		`(?s).*- ln -sf ../../bin/workshopctl /wsp/bin/fsfreeze.*`)
	c.Check(out, check.Matches,
		`(?s).*path: /boot/grub/custom.cfg.*`)
	c.Check(out, check.Matches,
		`(?s).*- update-grub.*`)
	c.Check(out, check.Matches,
		`(?s).*- install --directory --mode=755 /project /usr/local/bin /usr/local/lib/workshop /wsp/state.*`)
	c.Check(out, check.Matches,
		`(?s).*ListenStream=/wsp/run/workshop\.socket\.secret.*`)
	c.Check(out, check.Matches,
		`(?s).*Accept=yes.*`)
	c.Check(out, check.Matches,
		`(?s).*path: /etc/systemd/system/workshop-secret@\.service.*`)
	c.Check(out, check.Matches,
		`(?s).*ExecStart=/wsp/bin/workshopctl get-secret --systemd.*`)
	c.Check(out, check.Matches,
		`(?s).*StandardInput=socket.*`)
	c.Check(out, check.Matches,
		`(?s).*StandardOutput=socket.*`)
	c.Check(out, check.Matches,
		`(?s).*systemctl enable --now workshop-secret\.socket.*`)
}

// TestMakeCloudConfigVarsContainer checks that container variables exclude the
// VM-only fsfreeze helper and GRUB configuration.
func (s *cloudConfigSuite) TestMakeCloudConfigVarsContainer(c *check.C) {
	vars := makeCloudConfigVars(&workshop.File{
		Runtime: workshop.RuntimeLXDContainer,
	})

	c.Check(vars.FsFreezePath, check.Equals, "")
	c.Check(vars.HasGRUB, check.Equals, false)
	c.Check(vars.StartTimeout, check.Equals,
		startTimeoutContainer.Nanoseconds())
	c.Check(vars.WorkshopSecretSocketPath, check.Equals,
		dirs.WorkshopSecretSocketPath)
	c.Check(vars.WorkshopStateDir, check.Equals, dirs.WorkshopStateDir)
}

// TestMakeCloudConfigVarsVM checks that VM variables include the fsfreeze
// helper, GRUB configuration and the longer start timeout.
func (s *cloudConfigSuite) TestMakeCloudConfigVarsVM(c *check.C) {
	vars := makeCloudConfigVars(&workshop.File{
		Runtime: workshop.RuntimeLXDVM,
	})

	c.Check(vars.FsFreezePath, check.Equals, dirs.FsFreezePath)
	c.Check(vars.HasGRUB, check.Equals, true)
	c.Check(vars.StartTimeout, check.Equals, startTimeoutVM.Nanoseconds())
	c.Check(vars.WorkshopSecretSocketPath, check.Equals,
		dirs.WorkshopSecretSocketPath)
	c.Check(vars.WorkshopStateDir, check.Equals, dirs.WorkshopStateDir)
}
