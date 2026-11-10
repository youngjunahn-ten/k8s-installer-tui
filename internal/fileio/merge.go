package fileio

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// The TUI knows only part of what k8s-installer reads. When a file already exists,
// saving changes only what the TUI owns and keeps everything else as it was:
// other keys, their comments, and (for vars.yml) the layout line for line.

// ownedVarKeys are the top-level keys of vars.yml the TUI writes. One that is
// absent from the rendered file (aipub_* with AIPub off, internal_network_extra_zone
// with the internal DNS server off) is removed from the existing file.
var ownedVarKeys = []string{
	"ki_var_root_path", "containerd_root_path", "docker_root_path",
	"internal_network_subnets", "internal_network_extra_zone", "internal_network_extra_zone_a_records",
	"ki_cp_ha_mode", "ki_cp_ha_mode_vip", "ki_cp_dns_dnssec_validation",
	"ki_cp_dns_server_upstream_servers", "ki_cp_ntp_server_upstream_servers",
	"k8s_certificate_validity_period", "k8s_load_balancers", "k8s_gateways", "k8s_default_ingress_class",
	"aipub_ingress_zone", "aipub_ha_mode", "aipub_ha_mode_storage_class", "aipub_cp_nodes",
	"aipub_harbor_ingress_subdomain", "aipub_harbor_replica_count",
	"aipub_harbor_registry_storage_size", "aipub_harbor_postgresql_storage_size",
	"aipub_harbor_redis_storage_size", "aipub_harbor_trivy_storage_size",
	"ki_cert_mode",
}

// lineSpan is the 0-based, inclusive line range of a top-level key and its value.
// before is the first line after the previous key's span: the blank lines and
// comments that lead up to this key.
type lineSpan struct {
	before, start, end int
}

// topLevelSpans finds where each top-level key of a YAML mapping sits in src.
// A key's value runs until the next top-level key; blank lines and comments in
// column 0 just before the next key belong to that next key.
func topLevelSpans(lines []string, root *yaml.Node) (map[string]lineSpan, []string) {
	spans := map[string]lineSpan{}
	var order []string
	var starts []int
	for i := 0; i+1 < len(root.Content); i += 2 {
		order = append(order, root.Content[i].Value)
		starts = append(starts, root.Content[i].Line-1)
	}
	prevEnd := -1
	for i, key := range order {
		next := len(lines)
		if i+1 < len(starts) {
			next = starts[i+1]
		}
		end := next - 1
		for end > starts[i] && isBlankOrColumn0Comment(lines[end]) {
			end--
		}
		spans[key] = lineSpan{before: prevEnd + 1, start: starts[i], end: end}
		prevEnd = end
	}
	return spans, order
}

func isBlankOrColumn0Comment(line string) bool {
	t := strings.TrimRight(line, " \t\r")
	return t == "" || strings.HasPrefix(t, "#")
}

func splitLines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func parseMapping(b []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	return doc.Content[0], nil
}

// mergeVars writes the TUI's keys into an existing vars.yml in place. Each owned
// key's lines are replaced by the rendered ones, an owned key the TUI no longer
// writes is removed, and a rendered key the file lacks is added at the end with
// the comments that lead up to it. Lines of any other key are left untouched.
func mergeVars(existing, rendered []byte) ([]byte, error) {
	eroot, err := parseMapping(existing)
	if err != nil {
		return nil, fmt.Errorf("parse existing vars.yml: %w", err)
	}
	if eroot == nil {
		return rendered, nil
	}
	froot, err := parseMapping(rendered)
	if err != nil || froot == nil {
		return nil, fmt.Errorf("parse rendered vars.yml: %v", err)
	}

	elines := splitLines(existing)
	flines := splitLines(rendered)
	espans, eorder := topLevelSpans(elines, eroot)
	fspans, forder := topLevelSpans(flines, froot)

	owned := map[string]bool{}
	for _, k := range ownedVarKeys {
		owned[k] = true
	}
	for _, k := range forder {
		owned[k] = true
	}

	var out []string
	cur := 0
	for _, key := range eorder {
		sp := espans[key]
		if !owned[key] {
			continue
		}
		out = append(out, elines[cur:sp.start]...)
		if fsp, ok := fspans[key]; ok {
			out = append(out, flines[fsp.start:fsp.end+1]...)
		}
		cur = sp.end + 1
	}
	out = append(out, elines[cur:]...)

	for _, key := range forder {
		if _, ok := espans[key]; ok {
			continue
		}
		fsp := fspans[key]
		lead := flines[fsp.before:fsp.start]
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" && (len(lead) == 0 || strings.TrimSpace(lead[0]) != "") {
			out = append(out, "")
		}
		out = append(out, lead...)
		out = append(out, flines[fsp.start:fsp.end+1]...)
	}

	return []byte(strings.Join(out, "\n") + "\n"), nil
}

// mergeInventory writes the TUI's part of the inventory into an existing one:
// control_node_ih, the hosts of all / ki_cp_node / k8s_node with the keys the TUI
// sets on them (ansible_host, ansible_port, ansible_ssh_user, k8s_cp, nvidia_gpu).
// Other host variables, other groups (broken_node, ...) and comments are kept.
// A host the TUI no longer lists is removed from these three groups, and a node
// deleted in the TUI from every group.
func mergeInventory(existing, rendered []byte) ([]byte, error) {
	var edoc yaml.Node
	if err := yaml.Unmarshal(existing, &edoc); err != nil {
		return nil, fmt.Errorf("parse existing inventory.yml: %w", err)
	}
	if len(edoc.Content) == 0 || edoc.Content[0].Kind != yaml.MappingNode {
		return rendered, nil
	}
	froot, err := parseMapping(rendered)
	if err != nil || froot == nil {
		return nil, fmt.Errorf("parse rendered inventory.yml: %v", err)
	}
	eroot := edoc.Content[0]

	local := ensureMap(ensureMap(ensureMap(eroot, "control_node"), "hosts"), "localhost")
	if mappingGet(local, "ansible_connection") == nil {
		mappingSet(local, "ansible_connection", strNode("local"))
	}
	fLocal := mappingGet(mappingGet(mappingGet(froot, "control_node"), "hosts"), "localhost")
	if ih := mappingGet(fLocal, "control_node_ih"); ih != nil {
		mappingSet(local, "control_node_ih", ih)
	} else {
		mappingDelete(local, "control_node_ih")
	}

	// A node deleted in the TUI (in all.hosts before, not now) is also dropped from
	// every other group (broken_node, a site's own group): left there, ansible would
	// make it a host with no ansible_host.
	keep := map[string]bool{}
	if fAll := mappingGet(mappingGet(froot, "all"), "hosts"); fAll != nil {
		for i := 0; i+1 < len(fAll.Content); i += 2 {
			keep[fAll.Content[i].Value] = true
		}
	}
	var deleted []string
	if eAll := mappingGet(mappingGet(eroot, "all"), "hosts"); eAll != nil && eAll.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(eAll.Content); i += 2 {
			if name := eAll.Content[i].Value; !keep[name] {
				deleted = append(deleted, name)
			}
		}
	}
	for i := 0; i+1 < len(eroot.Content); i += 2 {
		if hosts := mappingGet(eroot.Content[i+1], "hosts"); hosts != nil {
			for _, name := range deleted {
				mappingDelete(hosts, name)
			}
		}
	}

	groups := []struct {
		name  string
		owned []string
	}{
		{"all", []string{"ansible_host", "ansible_port", "ansible_ssh_user"}},
		{"ki_cp_node", nil},
		{"k8s_node", []string{"k8s_cp", "nvidia_gpu"}},
	}
	for _, g := range groups {
		fHosts := mappingGet(mappingGet(froot, g.name), "hosts")
		eHosts := ensureMap(ensureMap(eroot, g.name), "hosts")
		mergeHosts(eHosts, fHosts, g.owned)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&edoc); err != nil {
		return nil, fmt.Errorf("encode inventory: %w", err)
	}
	return buf.Bytes(), nil
}

// mergeHosts makes eHosts list the hosts of fHosts in their order. An existing
// host keeps its key (and comments) and its variables; only the owned ones are
// set from the rendered entry or removed when the rendered entry lacks them.
func mergeHosts(eHosts, fHosts *yaml.Node, owned []string) {
	var content []*yaml.Node
	if fHosts != nil && fHosts.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(fHosts.Content); i += 2 {
			fKey, fVal := fHosts.Content[i], fHosts.Content[i+1]
			eKey, eVal := mappingGetPair(eHosts, fKey.Value)
			if eKey == nil {
				content = append(content, fKey, fVal)
				continue
			}
			if eVal.Kind != yaml.MappingNode {
				// "node4:" (null) or a scalar: take the rendered entry unless it is empty.
				if len(fVal.Content) > 0 {
					eVal = fVal
				}
				content = append(content, eKey, eVal)
				continue
			}
			for _, k := range owned {
				if v := mappingGet(fVal, k); v != nil {
					mappingSet(eVal, k, v)
				} else {
					mappingDelete(eVal, k)
				}
			}
			content = append(content, eKey, eVal)
		}
	}
	eHosts.Content = content
}

// --- yaml.Node mapping helpers ---

func mappingGetPair(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// mappingSet replaces the value of key, keeping the comments of the old value, or
// appends key: val when the mapping lacks it.
func mappingSet(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			old := m.Content[i+1]
			nv := *val
			if nv.LineComment == "" {
				nv.LineComment = old.LineComment
			}
			if nv.HeadComment == "" {
				nv.HeadComment = old.HeadComment
			}
			m.Content[i+1] = &nv
			return
		}
	}
	m.Content = append(m.Content, strNode(key), val)
}

func mappingDelete(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// ensureMap returns the mapping under key, adding it, or turning a null/scalar
// value ("hosts:" with nothing under it) into an empty mapping.
func ensureMap(m *yaml.Node, key string) *yaml.Node {
	_, v := mappingGetPair(m, key)
	if v == nil {
		v = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, strNode(key), v)
		return v
	}
	if v.Kind != yaml.MappingNode {
		*v = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: v.HeadComment, LineComment: v.LineComment}
	}
	return v
}
