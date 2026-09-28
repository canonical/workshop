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

package cmdutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/juju/ansiterm/tabwriter"
	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

type cmdUtil struct {
}

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&cmdUtil{})

func (m *cmdUtil) TestHomeDirectoryPathContraction(c *check.C) {
	home, _ := os.UserHomeDir()
	r := ContractHome(filepath.Join(home, "test"))
	c.Assert(r, check.Equals, "~/test")
	r = ContractHome(filepath.Join(home, "///test"))
	c.Assert(r, check.Equals, "~/test")
	r = ContractHome(home)
	c.Assert(r, check.Equals, "~")
	r = ContractHome("/sys")
	c.Assert(r, check.Equals, "/sys")
}

func FuzzEscapeYAMLScalar(f *testing.F) {
	// Some examples we expect to see in `workshop info`.
	f.Add("dev", uint32(13111961))
	f.Add("-", uint32(12966890))
	f.Add("–", uint32(13148641))
	f.Add("~/project", uint32(14853889))
	f.Add("[::1]:8080", uint32(2130201))
	f.Add("@socket", uint32(8336348))
	// Tricky cases found by the fuzzer.
	f.Add("\n", uint32(3667180))
	f.Add("\n0", uint32(115407))
	f.Add("0\n\n", uint32(7803534))
	f.Add("0\t\n", uint32(5590977))
	f.Add("\t0\n", uint32(3785866))
	// Prevents using json.Marshal to escape multiline strings.
	f.Add("\n\x7f", uint32(3830174))
	// This one only appears if mixing YAML v3 and v4.
	f.Add("0b+0", uint32(14161959))

	f.Fuzz(func(t *testing.T, s string, lengths uint32) {
		if !utf8.ValidString(s) {
			t.SkipNow()
		}

		// Generate example YAML, e.g.
		// aaa: xxxxx
		// bb: "\x7x\n\n"
		// ccccc: zzz
		// The lengths of surrounding keys and values are determined by
		// the lowest 24 bits of `lengths`, to attempt to approximate
		// a real-world scenario.
		a := strings.Repeat("a", int(lengths&0xf)+1)
		lengths >>= 4
		b := strings.Repeat("b", int(lengths&0xf)+1)
		lengths >>= 4
		c := strings.Repeat("c", int(lengths&0xf)+1)
		lengths >>= 4

		x := strings.Repeat("x", int(lengths&0x3f)+1)
		lengths >>= 6
		z := strings.Repeat("z", int(lengths&0x3f)+1)

		var out bytes.Buffer
		w := tabwriter.NewWriter(&out, 4, 3, 2, ' ', tabwriter.StripEscape)
		tabescape := []byte{tabwriter.Escape}

		fmt.Fprintf(w, "%s:\t%s\n", a, x)
		fmt.Fprintf(w, "%s:\t%s%s%s\n", b, tabescape, EscapeYAMLScalar(s), tabescape)
		fmt.Fprintf(w, "%s:\t%s\n", c, z)

		w.Flush()
		var result any
		if err := yaml.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("cannot unmarshal %q: %v", s, err)
		}

		expected := map[string]any{
			a: x,
			b: s,
			c: z,
		}
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("unexpected YAML round trip for %q", s)
		}
	})
}
