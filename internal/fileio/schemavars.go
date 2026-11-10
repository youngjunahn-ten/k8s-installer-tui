package fileio

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// Schema mode: the variables of vars.yml are whatever the schema of the
// installer lists (state.VarsOrder), held as plain values (state.VarsValues),
// and a save writes the ones the person changed (state.VarsEdited).

// LoadVarsValues reads vars.yml as it is: every top-level key and its value.
// A missing file is no values.
func LoadVarsValues(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	values := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return values, nil
}

// buildSchemaVars is buildVars for schema mode.
//
// With a vars.yml there, only the lines of the changed variables are replaced
// (a commented placeholder is replaced where it is, a variable the file lacks
// is added at the end: the schema says the installer has it). With none, every
// variable that has a value is written, in the order of the schema, which is a
// file the validator of the installer accepts.
func buildSchemaVars(path string, s *state.AppState) ([]byte, []string, error) {
	existing, err := readIfExists(path)
	if err != nil {
		return nil, nil, err
	}

	var names []string
	for _, name := range s.VarsOrder {
		_, has := s.VarsValues[name]
		if has && (existing == nil || s.VarsEdited[name]) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return existing, nil, nil
	}
	rendered, err := renderValues(names, s.VarsValues)
	if err != nil {
		return nil, nil, err
	}
	if existing == nil {
		return rendered, nil, nil
	}
	return mergeVars(existing, rendered, names, true)
}

// renderValues writes "name: value" for each name, one top-level key after
// another, strings double quoted the way the installer's vars.yml writes them.
func renderValues(names []string, values map[string]interface{}) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range names {
		root.Content = append(root.Content, strNode(name), valueNode(values[name]))
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return nil, fmt.Errorf("encode vars: %w", err)
	}
	return buf.Bytes(), nil
}

func valueNode(v interface{}) *yaml.Node {
	switch x := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: x, Style: yaml.DoubleQuotedStyle}
	case []interface{}:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if len(x) == 0 {
			n.Style = yaml.FlowStyle // []
		}
		for _, item := range x {
			n.Content = append(n.Content, valueNode(item))
		}
		return n
	case []string:
		items := make([]interface{}, len(x))
		for i, item := range x {
			items[i] = item
		}
		return valueNode(items)
	case map[string]interface{}:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if len(x) == 0 {
			n.Style = yaml.FlowStyle // {}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n.Content = append(n.Content, strNode(k), valueNode(x[k]))
		}
		return n
	default:
		// bool, int, float: what yaml makes of them
		n := &yaml.Node{}
		_ = n.Encode(v)
		return n
	}
}
