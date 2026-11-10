// Package schema reads ansible/schema/vars.schema.json of k8s-installer: what
// vars.yml of that installer accepts, written by the installer itself
// (tools/export-vars-schema.py) from its validator, its vars.yml and its
// ki_var_classes.
//
// The screens of this TUI name the variables they edit, and those names are the
// ones of the installer before it was split. A form drawn from this file instead
// follows the installer it is run beside: a variable added there appears here
// without a change to this repository, and one it does not have is never written.
package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Kind is what a form needs to know to pick an input for a variable.
type Kind string

const (
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
	KindString Kind = "string"
	KindEnum   Kind = "enum"   // one of Field.Enum
	KindList   Kind = "list"   // of strings (addresses, subnets, names)
	KindObject Kind = "object" // lists of objects and mappings: edited as YAML
)

// Field is one variable of vars.yml.
type Field struct {
	Name        string
	Kind        Kind
	Nullable    bool // null is a value of its own (usually "work it out for me")
	Required    bool
	Enum        []string
	Format      string // ipv4, ipv4network, ... when the schema says
	Pattern     string
	Default     interface{}
	Commented   bool        // vars.yml ships it commented out; Example is what it shows
	Example     interface{} // set when Commented
	Description string      // the comment above it in vars.yml
	Class       string      // x-ki-class: what applying a change takes
	ClusterWide bool        // inventory.yml may not set it on a node
}

// Summary is the first paragraph of the description, on one line.
func (f Field) Summary() string {
	first := strings.SplitN(f.Description, "\n\n", 2)[0]
	return strings.Join(strings.Fields(first), " ")
}

// Schema is the variables of vars.yml in the order vars.yml lists them.
type Schema struct {
	Fields []Field
	byName map[string]int
}

// Has reports whether the installer this schema came from has the variable.
func (s *Schema) Has(name string) bool {
	if strings.HasPrefix(name, "ansible_") {
		return true // patternProperties of the schema: connection variables pass
	}
	_, ok := s.byName[name]
	return ok
}

// Field returns the variable by name.
func (s *Schema) Field(name string) (Field, bool) {
	i, ok := s.byName[name]
	if !ok {
		return Field{}, false
	}
	return s.Fields[i], true
}

// Unknown returns the names of names the installer does not have, in order.
func (s *Schema) Unknown(names []string) []string {
	var out []string
	for _, n := range names {
		if !s.Has(n) {
			out = append(out, n)
		}
	}
	return out
}

type rawSchema struct {
	Order      []string                   `json:"x-ki-order"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type rawProp struct {
	Type        string            `json:"type"`
	Enum        []interface{}     `json:"enum"`
	AnyOf       []json.RawMessage `json:"anyOf"`
	Ref         string            `json:"$ref"`
	Items       json.RawMessage   `json:"items"`
	Format      string            `json:"format"`
	Pattern     string            `json:"pattern"`
	Default     interface{}       `json:"default"`
	Examples    []interface{}     `json:"examples"`
	Description string            `json:"description"`
	Class       string            `json:"x-ki-class"`
	ClusterWide bool              `json:"x-ki-cluster-wide"`
}

// Load reads a vars.schema.json.
func Load(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse reads the bytes of a vars.schema.json.
func Parse(data []byte) (*Schema, error) {
	var raw rawSchema
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	if len(raw.Order) == 0 || len(raw.Properties) == 0 {
		return nil, fmt.Errorf("not a vars schema of k8s-installer: no x-ki-order or properties")
	}
	required := map[string]bool{}
	for _, n := range raw.Required {
		required[n] = true
	}

	s := &Schema{byName: map[string]int{}}
	for _, name := range raw.Order {
		body, ok := raw.Properties[name]
		if !ok {
			return nil, fmt.Errorf("x-ki-order names %q, which is not a property", name)
		}
		var p rawProp
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		f := Field{
			Name:        name,
			Required:    required[name],
			Default:     p.Default,
			Description: p.Description,
			Class:       p.Class,
			ClusterWide: p.ClusterWide,
		}
		if len(p.Examples) > 0 {
			f.Commented = true
			f.Example = p.Examples[0]
		}
		if err := classify(&f, p); err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		s.byName[name] = len(s.Fields)
		s.Fields = append(s.Fields, f)
	}
	return s, nil
}

// classify turns the JSON Schema of a property into the Kind of input it needs.
// "anyOf: [X, null]" is X made nullable. Anything this does not recognise is
// KindObject, which a form can always offer as raw YAML, so a shape the
// installer adds later is editable rather than an error.
func classify(f *Field, p rawProp) error {
	if len(p.AnyOf) > 0 {
		var inner []rawProp
		for _, alt := range p.AnyOf {
			var a rawProp
			if err := json.Unmarshal(alt, &a); err != nil {
				return err
			}
			if a.Type == "null" {
				f.Nullable = true
				continue
			}
			inner = append(inner, a)
		}
		if len(inner) == 1 {
			return classify(f, inner[0])
		}
		// Several real alternatives (an address or a name): typed as text
		f.Kind = KindString
		return nil
	}

	f.Format, f.Pattern = p.Format, p.Pattern
	switch {
	case len(p.Enum) > 0:
		f.Kind = KindEnum
		for _, v := range p.Enum {
			f.Enum = append(f.Enum, fmt.Sprint(v))
		}
	case p.Type == "boolean":
		f.Kind = KindBool
	case p.Type == "integer":
		f.Kind = KindInt
	case p.Type == "string":
		f.Kind = KindString
	case p.Type == "array":
		var item rawProp
		if len(p.Items) > 0 {
			if err := json.Unmarshal(p.Items, &item); err != nil {
				return err
			}
		}
		if item.Ref != "" || item.Type == "object" || item.Type == "array" {
			f.Kind = KindObject
		} else {
			f.Kind = KindList
		}
	default:
		f.Kind = KindObject
	}
	return nil
}
