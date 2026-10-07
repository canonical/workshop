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
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

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

// UnmarshalStrict decodes a struct from the given YAML node, but returns an
// error if the YAML mapping contains fields that aren't in the struct. When
// implementing UnmarshalYAML using this function, T must be distinct from the
// receiver's type (to avoid infinite recursion).
func UnmarshalStrict[T any](t *T, value *yaml.Node) error {
	result := struct {
		T       *T                 `yaml:",inline"`
		Unknown map[string]NodeRef `yaml:",inline"`
	}{
		T: t,
	}

	if err := value.Decode(&result); err != nil || len(result.Unknown) == 0 {
		return err
	}

	fields := make([]UnknownField, 0, len(result.Unknown))
	for name, node := range result.Unknown {
		field := UnknownField{Name: name}
		// The yaml package never calls UnmarshalYAML for `null` scalars.
		if node.Node != nil {
			field.Line = node.Node.Line
			field.Column = node.Node.Column
		}
		fields = append(fields, field)
	}
	slices.SortFunc(fields, func(a, b UnknownField) int {
		return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column), cmp.Compare(a.Name, b.Name))
	})
	return &UnknownFieldsError{Fields: fields}
}

// UnknownFieldsError reports unknown struct fields from decoding a mapping.
type UnknownFieldsError struct {
	Context string
	Fields  []UnknownField
}

type UnknownField struct {
	Name   string
	Line   int
	Column int
}

func (e *UnknownFieldsError) Error() string {
	var builder strings.Builder
	if e.Context == "" {
		builder.WriteString("unknown YAML fields: ")
	} else {
		fmt.Fprintf(&builder, "%s contains unknown fields: ", e.Context)
	}
	_, _ = e.WriteTo(&builder)
	return builder.String()
}

func (e *UnknownFieldsError) WriteTo(w io.Writer) (int64, error) {
	n := int64(0)
	for _, field := range e.Fields {
		if n > 0 {
			m, err := w.Write([]byte("; "))
			n += int64(m)
			if err != nil {
				return n, err
			}
		}
		m, err := field.WriteTo(w)
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (f UnknownField) WriteTo(w io.Writer) (int64, error) {
	m, err := fmt.Fprintf(w, "%q", f.Name)
	n := int64(m)
	if err != nil || f.Line <= 0 {
		return n, err
	}

	m, err = fmt.Fprintf(w, " at line %d", f.Line)
	n += int64(m)
	if err != nil || f.Column <= 0 {
		return n, err
	}

	m, err = fmt.Fprintf(w, ", column %d", f.Column)
	n += int64(m)
	return n, err
}

func AttachContext(document string, err error) error {
	typeErr, ok := errors.AsType[*yaml.TypeError](err)
	if ok {
		return fmt.Errorf("%s:\n%s", document, strings.Join(typeErr.Errors, "\n"))
	}

	unknownErr, ok := errors.AsType[*UnknownFieldsError](err)
	if ok && unknownErr.Context == "" {
		unknownErr.Context = document
	}

	return err
}

// SetString sets the value of a scalar node to the given string, dropping
// any explicit tag (e.g. "!!binary") which would change how the value is
// decoded. The encoder quotes the value if needed.
func SetString(node *yaml.Node, value string) {
	node.Tag = "!!str"
	node.Style &^= yaml.TaggedStyle
	node.Value = value
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
