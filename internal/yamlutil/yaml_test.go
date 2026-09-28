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

package yamlutil_test

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/yamlutil"
)

type yamlSuite struct{}

var _ = check.Suite(&yamlSuite{})

func Test(t *testing.T) { check.TestingT(t) }

func (y *yamlSuite) TestNodeRefIsUnmarshaler(c *check.C) {
	var ref yamlutil.NodeRef
	var u yaml.Unmarshaler = &ref
	var node yaml.Node
	c.Assert(u.UnmarshalYAML(&node), check.IsNil)
	c.Check(ref.Node, check.Equals, &node)
}

func (y *yamlSuite) TestNodeRefField(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}

	document := unmarshalAndDecode(c, &nodes, `field: data
`)
	c.Check(nodes.Field.Node.Value, check.Equals, "data")
	c.Check(contains(document, nodes.Field.Node), check.Equals, true)
}

func (y *yamlSuite) TestNodeRefAlias(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}

	document := unmarshalAndDecode(c, &nodes, `x-field: &f data
field: *f
`)
	c.Check(nodes.Field.Node.Value, check.Equals, "data")
	c.Check(contains(document, nodes.Field.Node), check.Equals, true)
}

func (y *yamlSuite) TestNodeRefSlice(c *check.C) {
	var nodes struct {
		Slice []yamlutil.NodeRef
	}

	document := unmarshalAndDecode(c, &nodes, `slice: [1, 2, 3]
`)
	c.Check(nodes.Slice, check.HasLen, 3)
	c.Check(contains(document, nodes.Slice[0].Node), check.Equals, true)
	c.Check(contains(document, nodes.Slice[1].Node), check.Equals, true)
	c.Check(contains(document, nodes.Slice[2].Node), check.Equals, true)
}

func (y *yamlSuite) TestNodeRefNested(c *check.C) {
	var nodes struct {
		One struct {
			Nest struct {
				A yamlutil.NodeRef
				B yamlutil.NodeRef
				C yamlutil.NodeRef
			}
		}
		Two struct {
			A yamlutil.NodeRef
			B yamlutil.NodeRef
		}
	}

	document := unmarshalAndDecode(c, &nodes, `two: &t
  a: aa
  b: bb
one:
  nest:
    <<: *t
    c: cc
`)
	c.Check(nodes.One.Nest.A.Node.Value, check.Equals, "aa")
	c.Check(nodes.One.Nest.B.Node.Value, check.Equals, "bb")
	c.Check(nodes.One.Nest.C.Node.Value, check.Equals, "cc")
	c.Check(nodes.One.Nest.A.Node, check.Equals, nodes.Two.A.Node)
	c.Check(nodes.One.Nest.B.Node, check.Equals, nodes.Two.B.Node)
	c.Check(contains(document, nodes.One.Nest.A.Node), check.Equals, true)
	c.Check(contains(document, nodes.One.Nest.B.Node), check.Equals, true)
	c.Check(contains(document, nodes.One.Nest.C.Node), check.Equals, true)
}

func unmarshalAndDecode(c *check.C, v any, content string) *yaml.Node {
	var document yaml.Node
	c.Assert(yaml.Unmarshal([]byte(content), &document), check.IsNil)
	c.Assert(document.Decode(v), check.IsNil)
	return &document
}

func contains(root, node *yaml.Node) bool {
	if root == node {
		return true
	}

	switch root.Kind {
	case yaml.DocumentNode, yaml.SequenceNode, yaml.MappingNode:
		return slices.ContainsFunc(root.Content, func(n *yaml.Node) bool { return contains(n, node) })
	default:
		return false
	}
}

func (y *yamlSuite) TestRemoveNodesMapping(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}

	before := `field: f
`
	after := `{}
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `field: f
after: a
`
	after = `after: a
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `before: b
field: f
`
	after = `before: b
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `before: b
field: f
after: a
`
	after = `before: b
after: a
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)
}

func (y *yamlSuite) TestRemoveNodesSequence(c *check.C) {
	var one [1]yamlutil.NodeRef
	before := `- b
`
	after := `[]
`
	checkRemoveNodes(c, before, after, &one, &one[0])

	var two [2]yamlutil.NodeRef
	before = `- b
- c
`
	after = `- c
`
	checkRemoveNodes(c, before, after, &two, &two[0])

	before = `- a
- b
`
	after = `- a
`
	checkRemoveNodes(c, before, after, &two, &two[1])

	var three [3]yamlutil.NodeRef
	before = `- a
- b
- c
`
	after = `- a
- c
`
	checkRemoveNodes(c, before, after, &three, &three[1])
}

func (y *yamlSuite) TestRemoveNodesFlowStyle(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}
	before := `{"before": "b", "field": "f", "after": "a"}
`
	after := `{"before": "b", "after": "a"}
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	var items [3]yamlutil.NodeRef
	before = `["a", "b", "c"]
`
	after = `["a", "c"]
`
	checkRemoveNodes(c, before, after, &items, &items[1])
}

func (y *yamlSuite) TestRemoveNodesWithComments(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}

	before := `# document head

# field head
field: f # f line
# field foot

# document foot
`
	after := `# document head

{}

# document foot
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `# document head

# field head
field: f # f line
# field foot

# after head
after: a # a line
# after foot

# document foot
`
	after = `# document head

# after head
after: a # a line
# after foot

# document foot
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `# document head

# before head
before: b # b line
# before foot

# field head
field: f # f line
# field foot

# document foot
`
	after = `# document head

# before head
before: b # b line
# before foot

# document foot
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	before = `# document head

# before head
before: b # b line
# before foot

# field head
field: f # f line
# field foot

# after head
after: a # a line
# after foot

# document foot
`
	after = `# document head

# before head
before: b # b line
# before foot

# after head
after: a # a line
# after foot

# document foot
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)

	var nested struct {
		Nodes struct {
			Field yamlutil.NodeRef
		}
	}
	before = `# document head

# nodes head
nodes:
  # field head
  field: f # f line
  # field foot
# nodes foot

# document foot
`
	after = `# document head

# nodes head
nodes: {}
# nodes foot

# document foot
`
	checkRemoveNodes(c, before, after, &nested, &nested.Nodes.Field)
}

func (y *yamlSuite) TestRemoveNodesAliases(c *check.C) {
	var nodes struct {
		Field yamlutil.NodeRef
	}

	before := `x-field: &f data
field: *f
`
	after := `{}
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.Field)
}

func (y *yamlSuite) TestRemoveNodesMultiple(c *check.C) {
	var nodes struct {
		One yamlutil.NodeRef
		Two yamlutil.NodeRef
	}

	before := `zero: 0
one: 1
two: 2
three: 3
`
	after := `zero: 0
three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.One, &nodes.Two)

	before = `zero: 0
one: 1
three: 3
two: 2
`
	after = `zero: 0
three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.One, &nodes.Two)

	before = `one: 1
zero: 0
two: 2
three: 3
`
	after = `zero: 0
three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.One, &nodes.Two)

	before = `one: 1
zero: 0
three: 3
two: 2
`
	after = `zero: 0
three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.One, &nodes.Two)
}

func (y *yamlSuite) TestRemoveNodesMerged(c *check.C) {
	var nodes struct {
		A struct {
			One yamlutil.NodeRef
			Two yamlutil.NodeRef
		}
		B struct {
			One   yamlutil.NodeRef
			Two   yamlutil.NodeRef
			Three yamlutil.NodeRef
		}
	}

	before := `a: &a
  one: 1
  two: 2
b:
  <<: *a
  three: 3
`
	after := `a: &a
  two: 2
b:
  !!merge <<: *a
  three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.A.One)

	after = `a: &a
  one: 1
b:
  !!merge <<: *a
  three: 3
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.A.Two)

	after = `a: &a
  one: 1
  two: 2
b:
  !!merge <<: *a
`
	checkRemoveNodes(c, before, after, &nodes, &nodes.B.Three)

	var maps struct {
		A yamlutil.NodeRef
		B yamlutil.NodeRef
	}

	after = `b:
  three: 3
`
	checkRemoveNodes(c, before, after, &maps, &maps.A)

	after = `a: &a
  one: 1
  two: 2
`
	checkRemoveNodes(c, before, after, &maps, &maps.B)
}

func checkRemoveNodes(c *check.C, before, after string, v any, nodeRefs ...*yamlutil.NodeRef) {
	document := unmarshalAndDecode(c, v, before)

	nodes := make([]*yaml.Node, 0, len(nodeRefs))
	for _, n := range nodeRefs {
		nodes = append(nodes, n.Node)
	}
	yamlutil.RemoveNodes(document, nodes...)

	var builder strings.Builder
	e := yaml.NewEncoder(&builder)
	e.SetIndent(2)
	c.Assert(e.Encode(document), check.IsNil)

	c.Check(builder.String(), check.Equals, after)
}
