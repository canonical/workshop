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

package workshop_test

import (
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/workshop"
)

// apparmorSuite covers the raw.apparmor renderer and the File accessor that
// feeds it, independent of YAML parsing (see workshop_file_test.go for the
// validateApparmor tests exercised through the full parse path).
type apparmorSuite struct{}

var _ = check.Suite(&apparmorSuite{})

func (s *apparmorSuite) TestRenderAppArmorDenyWriteEmpty(c *check.C) {
	c.Check(workshop.RenderAppArmorDenyWrite(nil), check.Equals, "")
	c.Check(workshop.RenderAppArmorDenyWrite([]string{}), check.Equals, "")
}

func (s *apparmorSuite) TestRenderAppArmorDenyWriteSinglePath(c *check.C) {
	got := workshop.RenderAppArmorDenyWrite([]string{"/project/.git/hooks"})
	c.Check(got, check.Equals, "deny /project/.git/hooks/ w,\ndeny /project/.git/hooks/** w,")
}

func (s *apparmorSuite) TestRenderAppArmorDenyWriteMultiplePaths(c *check.C) {
	got := workshop.RenderAppArmorDenyWrite([]string{"/project/.git/hooks", "/project/.github/workflows"})
	c.Check(got, check.Equals, ""+
		"deny /project/.git/hooks/ w,\n"+
		"deny /project/.git/hooks/** w,\n"+
		"deny /project/.github/workflows/ w,\n"+
		"deny /project/.github/workflows/** w,")
}

func (s *apparmorSuite) TestFileApparmorDenyWriteNoSystemEntry(c *check.C) {
	file := &workshop.File{Name: "dev", Base: "ubuntu@24.04"}
	c.Check(file.ApparmorDenyWrite(), check.IsNil)
}

func (s *apparmorSuite) TestFileApparmorDenyWriteNoApparmorBlock(c *check.C) {
	file := &workshop.File{
		Name: "dev",
		Base: "ubuntu@24.04",
		Sdks: []workshop.SdkRecord{{Name: "system"}},
	}
	c.Check(file.ApparmorDenyWrite(), check.IsNil)
}

func (s *apparmorSuite) TestFileApparmorDenyWrite(c *check.C) {
	file := &workshop.File{
		Name: "dev",
		Base: "ubuntu@24.04",
		Sdks: []workshop.SdkRecord{{
			Name:     "system",
			Apparmor: &workshop.ApparmorPolicy{DenyWrite: []string{"/project/.git/hooks"}},
		}},
	}
	c.Check(file.ApparmorDenyWrite(), check.DeepEquals, []string{"/project/.git/hooks"})
}
