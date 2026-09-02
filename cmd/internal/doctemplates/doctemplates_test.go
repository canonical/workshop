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

package doctemplates

import (
	"testing"

	"github.com/canonical/gencodo"
)

// TestTemplatesRender fails when an index or command template references a
// field or function that gencodo does not provide, so that a template change
// cannot break the generate-ref-docs workflow unnoticed.
func TestTemplatesRender(t *testing.T) {
	command, err := ReadFile("command.rst")
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"workshop.rst", "sdk.rst"} {
		indexTemplate, err := ReadFile(index)
		if err != nil {
			t.Fatal(err)
		}
		templates := gencodo.TemplateInfo{
			IndexFileName:         index,
			IndexTemplate:         string(indexTemplate),
			SingleCommandTemplate: string(command),
		}
		if err := gencodo.ValidateTemplates(templates); err != nil {
			t.Errorf("%s + command.rst: %v", index, err)
		}
	}
}
