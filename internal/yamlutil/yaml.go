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

package yamlutil

import (
	"slices"

	"gopkg.in/yaml.v3"
)

// NodeRef can be used to extract a pointer to the node which
// corresponds to a struct field or slice element.
type NodeRef struct {
	Node *yaml.Node
}

func (n *NodeRef) UnmarshalYAML(value *yaml.Node) error {
	n.Node = value
	return nil
}

// RemoveNodes removes the given nodes from the document.
func RemoveNodes(root *yaml.Node, nodes ...*yaml.Node) {
	r := &nodeRemover{nodes}
	for len(r.nodes) > 0 {
		last := len(r.nodes) - 1
		node := r.nodes[last]
		r.nodes = r.nodes[:last]
		r.remove(root, node)
	}
}

type nodeRemover struct {
	nodes []*yaml.Node
}

func (r *nodeRemover) remove(root, node *yaml.Node) {
	switch root.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		root.Content = slices.DeleteFunc(root.Content, func(n *yaml.Node) bool { return n == node })
	case yaml.MappingNode:
		if idx := slices.Index(root.Content, node); idx >= 0 {
			if idx&1 == 0 {
				r.nodes = append(r.nodes, root.Content[idx+1])
			} else {
				idx -= 1
				r.nodes = append(r.nodes, root.Content[idx])
			}
			root.Content = slices.Delete(root.Content, idx, idx+2)
		}
	case yaml.AliasNode:
		if root.Alias == node {
			r.nodes = append(r.nodes, root)
		}
		return
	default:
		return
	}

	for _, child := range root.Content {
		r.remove(child, node)
	}
}
