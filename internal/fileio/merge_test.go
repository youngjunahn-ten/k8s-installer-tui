package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// testdata/k8s-installer-*.yml are ansible/inventory.yml and
// ansible/group_vars/all/vars.yml of k8s-installer main (b8a1bd6, 2026-11).

func copyTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadBoth(t *testing.T, invPath, varsPath string) *state.AppState {
	t.Helper()
	s := state.DefaultState()
	if err := LoadInventory(invPath, s); err != nil {
		t.Fatal(err)
	}
	if err := LoadVars(varsPath, s); err != nil {
		t.Fatal(err)
	}
	return s
}

// isSubsequence reports whether every line of want appears in got, in order.
func isSubsequence(want, got []string) (bool, string) {
	j := 0
	for _, w := range want {
		for j < len(got) && got[j] != w {
			j++
		}
		if j == len(got) {
			return false, w
		}
		j++
	}
	return true, ""
}

func TestVarsKeepsWhatTheTUIDoesNotOwn(t *testing.T) {
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	invPath := copyTestdata(t, "k8s-installer-inventory.yml")
	s := loadBoth(t, invPath, varsPath)
	s.KiCpHaModeVIP = "10.0.0.10"

	before, _ := os.ReadFile(varsPath)
	out, err := BuildVars(varsPath, s)
	if err != nil {
		t.Fatal(err)
	}

	// Every line outside the keys the TUI owns is still there, in order.
	root, _ := parseMapping(before)
	lines := splitLines(before)
	spans, order := topLevelSpans(lines, root)
	owned := map[string]bool{}
	for _, k := range ownedVarKeys {
		owned[k] = true
	}
	var kept []string
	cur := 0
	for _, k := range order {
		if owned[k] {
			kept = append(kept, lines[cur:spans[k].start]...)
			cur = spans[k].end + 1
		}
	}
	kept = append(kept, lines[cur:]...)
	if ok, missing := isSubsequence(kept, splitLines(out)); !ok {
		t.Fatalf("line lost on save: %q", missing)
	}

	for _, want := range []string{
		`ki_cp_ha_mode_vip: "10.0.0.10"`,
		`kubelet_max_pods: null`,
		`ki_cp_ha_mode_vip_carrier: "arp"`,
		`# Who tells the network where the vip is.`,
	} {
		if !strings.Contains(string(out), want+"\n") {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(string(out), "ki_cp_ha_mode_vip: null") {
		t.Error("old ki_cp_ha_mode_vip is still there")
	}
}

func TestSaveTwiceIsStableAndKeepsBothBackups(t *testing.T) {
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	invPath := copyTestdata(t, "k8s-installer-inventory.yml")
	s := loadBoth(t, invPath, varsPath)

	t0 := time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)
	for i, at := range []time.Time{t0, t0.Add(time.Minute)} {
		for _, f := range []struct {
			path  string
			build func(string, *state.AppState) ([]byte, error)
		}{{invPath, BuildInventory}, {varsPath, BuildVars}} {
			b, err := f.build(f.path, s)
			if err != nil {
				t.Fatal(err)
			}
			bak, err := writeWithBackup(f.path, b, at)
			if err != nil {
				t.Fatal(err)
			}
			if want := f.path + ".bak." + at.Format("20060102-150405"); bak != want {
				t.Fatalf("save %d: backup %q, want %q", i, bak, want)
			}
		}
		s = loadBoth(t, invPath, varsPath)
	}

	for _, p := range []string{invPath, varsPath} {
		first, _ := os.ReadFile(p + ".bak." + t0.Format("20060102-150405"))
		second, _ := os.ReadFile(p + ".bak." + t0.Add(time.Minute).Format("20060102-150405"))
		now, _ := os.ReadFile(p)
		if len(first) == 0 || len(second) == 0 {
			t.Fatalf("%s: a backup is missing", p)
		}
		if string(second) != string(now) {
			t.Errorf("%s: a second save with no change changed the file", filepath.Base(p))
		}
	}
}

func TestInventoryControlNodeIH(t *testing.T) {
	invPath := copyTestdata(t, "k8s-installer-inventory.yml")
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	s := loadBoth(t, invPath, varsPath)
	if s.ControlNodeIH != "node1" {
		t.Fatalf("loaded control_node_ih %q, want node1", s.ControlNodeIH)
	}

	s.ControlNodeIH = "node2"
	out, err := BuildInventory(invPath, s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invPath, out, 0644); err != nil {
		t.Fatal(err)
	}
	s2 := state.DefaultState()
	if err := LoadInventory(invPath, s2); err != nil {
		t.Fatal(err)
	}
	if s2.ControlNodeIH != "node2" {
		t.Errorf("saved control_node_ih %q, want node2", s2.ControlNodeIH)
	}
	for _, want := range []string{"broken_node:", "Which node of the inventory this control node is", "ansible_connection: local"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("inventory lost %q", want)
		}
	}
}

func TestInventoryKeepsOtherHostVars(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yml")
	existing := `control_node:
  hosts:
    localhost:
      ansible_connection: local
all:
  hosts:
    node1:
      ansible_host: 192.168.0.1
      ib_interface: ib0 # site variable the TUI does not know
    node2:
      ansible_host: 192.168.0.2
ki_cp_node:
  hosts:
    node1: {}
k8s_node:
  hosts:
    node1:
      k8s_cp: true
    node2:
my_group:
  hosts:
    node2: {}
`
	if err := os.WriteFile(invPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	s := state.DefaultState()
	if err := LoadInventory(invPath, s); err != nil {
		t.Fatal(err)
	}
	s.Nodes[0].AnsibleHost = "10.0.0.1"
	s.NvidiaGPUNodes = []string{"node2"}
	s.ControlNodeIH = "node1"

	out, err := BuildInventory(invPath, s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"ansible_host: 10.0.0.1",
		"ib_interface: ib0 # site variable the TUI does not know",
		"my_group:",
		"control_node_ih: node1",
		"nvidia_gpu: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inventory lacks %q:\n%s", want, got)
		}
	}
}

func TestFreshFilesWithoutExisting(t *testing.T) {
	dir := t.TempDir()
	s := state.DefaultState()
	s.Nodes = []state.NodeConfig{{Name: "n1", AnsibleHost: "192.168.0.1"}}
	s.KiCpNodes = []string{"n1"}
	s.K8sNodes = []string{"n1"}
	s.ControlNodeIH = "n1"

	inv, err := BuildInventory(filepath.Join(dir, "inventory.yml"), s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(inv), "control_node_ih: n1") {
		t.Errorf("new inventory lacks control_node_ih:\n%s", inv)
	}
	vars, err := BuildVars(filepath.Join(dir, "vars.yml"), s)
	if err != nil {
		t.Fatal(err)
	}
	rendered, _ := renderVars(s)
	if string(vars) != string(rendered) {
		t.Error("with no existing vars.yml the output should be the rendered file as before")
	}
}

func TestValidateControlNode(t *testing.T) {
	base := func() *state.AppState {
		s := state.DefaultState()
		s.Nodes = []state.NodeConfig{{Name: "n1", AnsibleHost: "192.168.0.1"}, {Name: "n2", AnsibleHost: "192.168.0.2"}}
		s.KiCpNodes = []string{"n1"}
		s.K8sNodes = []string{"n1", "n2"}
		return s
	}
	has := func(errs []validator.Error) bool {
		for _, e := range errs {
			if e.Field == "control_node_ih" {
				return true
			}
		}
		return false
	}

	s := base()
	if !has(validator.Validate(s)) {
		t.Error("an empty control_node_ih should be an error")
	}
	s.ControlNodeIH = "n2"
	if !has(validator.Validate(s)) {
		t.Error("a control node outside ki_cp_node should be an error")
	}
	s.ControlNodeIH = "n1"
	if has(validator.Validate(s)) {
		t.Error("n1 is a ki_cp node and should pass")
	}
}
