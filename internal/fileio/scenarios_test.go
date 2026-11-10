package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

var fixedTime = time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)

// Situations a site meets with an existing inventory.yml / vars.yml.

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustParse(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, b)
	}
	return m
}

func hostsOf(t *testing.T, m map[string]interface{}, group string) map[string]interface{} {
	t.Helper()
	g, _ := m[group].(map[string]interface{})
	h, _ := g["hosts"].(map[string]interface{})
	return h
}

// The vars.yml of the integrated installer before the split (aipub-installer-archive),
// the shape the TUI was written for: AIPub keys and load balancers in one file.
const archiveStyleVars = `ki_var_root_path: "/var/lib/k8s-installer"
containerd_root_path: "/var/lib/containerd"
docker_root_path: "/var/lib/docker"

internal_network_subnets:
  - "192.168.0.0/24"

ki_cp_ha_mode: true
ki_cp_ha_mode_vip: "192.168.0.250"
ki_cp_dns_dnssec_validation: false
ki_cp_dns_server_upstream_servers:
  - "8.8.8.8"
ki_cp_ntp_server_upstream_servers:
  - "time1.google.com"

k8s_certificate_validity_period: "26280h" # 26280h = 3 year
k8s_load_balancers:
  - name: "lb1"
    vip: "192.168.0.251"
    nodes:
      - "node1"
k8s_gateways:
  - name: "lb1"
    vip: "192.168.0.251"
    nodes:
      - "node1"
k8s_default_ingress_class:
  load_balancer: "lb1"
  host_network_port: 443

# site note kept by hand
my_site_key: "keep me"

aipub_ingress_zone: "aipub.example.com"
aipub_ha_mode: false
aipub_ha_mode_storage_class: null
aipub_cp_nodes:
  - "node1"

aipub_harbor_ingress_subdomain: "aipub-harbor"
aipub_harbor_replica_count: 1
aipub_harbor_registry_storage_size: 512Gi
aipub_harbor_postgresql_storage_size: 128Gi
aipub_harbor_redis_storage_size: 32Gi
aipub_harbor_trivy_storage_size: 5Gi

ki_cert_mode: "self_signed"
`

func TestArchiveStyleVarsAIPubOnOff(t *testing.T) {
	varsPath := writeFile(t, "vars.yml", archiveStyleVars)
	s := state.DefaultState()
	if err := LoadVars(varsPath, s); err != nil {
		t.Fatal(err)
	}
	if !s.AipubEnabled || s.AipubIngressZone != "aipub.example.com" {
		t.Fatalf("AIPub not loaded: %+v", s)
	}

	// AIPub off: every aipub_* key goes, the site key and its comment stay.
	s.AipubEnabled = false
	out, err := BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "aipub_") {
		t.Errorf("aipub keys left with AIPub off:\n%s", got)
	}
	for _, want := range []string{"# site note kept by hand\n", `my_site_key: "keep me"` + "\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q", want)
		}
	}
	mustParse(t, out)

	// AIPub back on with a new zone: the keys come back.
	if err := os.WriteFile(varsPath, out, 0644); err != nil {
		t.Fatal(err)
	}
	s.AipubEnabled = true
	s.AipubIngressZone = "new.example.com"
	out, err = BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, out)
	if m["aipub_ingress_zone"] != "new.example.com" || m["my_site_key"] != "keep me" {
		t.Errorf("aipub_ingress_zone=%v my_site_key=%v", m["aipub_ingress_zone"], m["my_site_key"])
	}
}

func TestInternalDNSOnOff(t *testing.T) {
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	invPath := copyTestdata(t, "k8s-installer-inventory.yml")
	s := loadBoth(t, invPath, varsPath)
	if s.InternalDNSServerEnabled {
		t.Fatal("main ships the extra zone commented out")
	}
	s.InternalDNSServerEnabled = true
	s.AipubIngressZone = "aipub.test"
	s.K8sIngressVIP = "192.168.0.100"
	out, err := BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, out)
	if m["internal_network_extra_zone"] != "aipub.test" {
		t.Fatalf("zone %v", m["internal_network_extra_zone"])
	}
	recs, _ := m["internal_network_extra_zone_a_records"].([]interface{})
	if len(recs) != 2 {
		t.Fatalf("a records %v", recs)
	}

	// Off again: the real keys go away.
	if err := os.WriteFile(varsPath, out, 0644); err != nil {
		t.Fatal(err)
	}
	s.InternalDNSServerEnabled = false
	out, err = BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}
	m = mustParse(t, out)
	if _, ok := m["internal_network_extra_zone"]; ok {
		t.Error("internal_network_extra_zone left with the DNS server off")
	}
	if _, ok := m["kubelet_max_pods"]; !ok {
		t.Error("kubelet_max_pods lost")
	}
}

func TestIngressModeSwitch(t *testing.T) {
	varsPath := writeFile(t, "vars.yml", archiveStyleVars)
	s := state.DefaultState()
	if err := LoadVars(varsPath, s); err != nil {
		t.Fatal(err)
	}
	s.K8sIngressMode = 1
	s.K8sGatewayLBName = "lb2"
	s.K8sGatewayVIP = "192.168.0.252"
	out, err := BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, out)
	gws := m["k8s_gateways"].([]interface{})
	if gw := gws[0].(map[string]interface{}); gw["name"] != "lb2" || gw["vip"] != "192.168.0.252" {
		t.Errorf("gateway %v", gw)
	}
	if strings.Count(string(out), "k8s_gateways:") != 1 {
		t.Error("k8s_gateways written twice")
	}
}

func TestDeletingANodeDropsItEverywhere(t *testing.T) {
	invPath := copyTestdata(t, "k8s-installer-inventory.yml")
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	// Put node4 in broken_node and in a site group first.
	b, _ := os.ReadFile(invPath)
	b = []byte(strings.Replace(string(b), "broken_node:\n  hosts: {}", "broken_node:\n  hosts:\n    node4: {}\nsite_group:\n  hosts:\n    node3: {}\n    node4: {}", 1))
	if err := os.WriteFile(invPath, b, 0644); err != nil {
		t.Fatal(err)
	}
	s := loadBoth(t, invPath, varsPath)
	s.Nodes = s.Nodes[:3] // node4 deleted in screen 1
	s.K8sNodes = []string{"node1", "node2", "node3"}

	out, err := BuildInventory(invPath, s)
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, out)
	for _, g := range []string{"all", "k8s_node", "broken_node", "site_group"} {
		if _, ok := hostsOf(t, m, g)["node4"]; ok {
			t.Errorf("node4 still in %s", g)
		}
	}
	if _, ok := hostsOf(t, m, "site_group")["node3"]; !ok {
		t.Error("node3 dropped from site_group")
	}
}

func TestClearingPortAndUser(t *testing.T) {
	invPath := writeFile(t, "inventory.yml", `all:
  hosts:
    node1:
      ansible_host: 10.0.0.1
      ansible_port: 2222
      ansible_ssh_user: admin
ki_cp_node:
  hosts:
    node1: {}
k8s_node:
  hosts:
    node1:
`)
	s := state.DefaultState()
	if err := LoadInventory(invPath, s); err != nil {
		t.Fatal(err)
	}
	if s.Nodes[0].AnsiblePort != "2222" || s.Nodes[0].AnsibleUser != "admin" {
		t.Fatalf("loaded %+v", s.Nodes[0])
	}
	s.Nodes[0].AnsiblePort = ""
	s.Nodes[0].AnsibleUser = ""
	s.ControlNodeIH = "node1"
	out, err := BuildInventory(invPath, s)
	if err != nil {
		t.Fatal(err)
	}
	node1 := hostsOf(t, mustParse(t, out), "all")["node1"].(map[string]interface{})
	if _, ok := node1["ansible_port"]; ok {
		t.Error("cleared ansible_port is still written")
	}
	if _, ok := node1["ansible_ssh_user"]; ok {
		t.Error("cleared ansible_ssh_user is still written")
	}
	// An inventory with no control_node section gets one.
	if !strings.Contains(string(out), "control_node_ih: node1") {
		t.Errorf("control_node not added:\n%s", out)
	}
}

func TestOddExistingFiles(t *testing.T) {
	s := state.DefaultState()
	s.Nodes = []state.NodeConfig{{Name: "n1", AnsibleHost: "10.0.0.1"}}
	s.KiCpNodes = []string{"n1"}
	s.K8sNodes = []string{"n1"}
	s.ControlNodeIH = "n1"
	rendered, _ := renderVars(s)

	cases := map[string]string{
		"empty":          "",
		"comments only":  "# nothing here yet\n",
		"document start": "---\nkubelet_max_pods: 200\nki_cp_ha_mode: false\n",
		"crlf":           "kubelet_max_pods: 200\r\nki_cp_ha_mode: false\r\nsysctl_extra: {}\r\n",
		"no final eol":   "kubelet_max_pods: 200\nki_cp_ha_mode: false",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			p := writeFile(t, "vars.yml", content)
			out, err := BuildVars(p, s)
			if err != nil {
				t.Fatal(err)
			}
			m := mustParse(t, out)
			if m["ki_cp_ha_mode"] != true {
				t.Errorf("ki_cp_ha_mode=%v", m["ki_cp_ha_mode"])
			}
			if strings.Contains(content, "kubelet_max_pods") && m["kubelet_max_pods"] != 200 {
				t.Errorf("kubelet_max_pods lost:\n%s", out)
			}
			if strings.TrimSpace(strings.ReplaceAll(content, "# nothing here yet", "")) == "" && string(out) != string(rendered) {
				t.Errorf("an empty file should give the rendered one")
			}
			if strings.Count(string(out), "ki_cp_ha_mode:") != 1 {
				t.Errorf("ki_cp_ha_mode written more than once:\n%s", out)
			}
		})
	}
}

func TestWriteCreatesDirAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "group_vars", "all", "vars.yml")
	bak, err := writeWithBackup(p, []byte("a: 1\n"), fixedTime)
	if err != nil || bak != "" {
		t.Fatalf("new file: bak=%q err=%v", bak, err)
	}
	if err := os.Chmod(p, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeWithBackup(p, []byte("a: 2\n"), fixedTime); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	// Windows has no group/other bits to keep; check where it means something.
	if os.PathSeparator == '/' && fi.Mode().Perm() != 0600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left: %s", e.Name())
		}
	}
}
