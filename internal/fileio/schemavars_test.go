package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

func schemaState(t *testing.T, varsPath string) *state.AppState {
	t.Helper()
	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "vars.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	values, err := LoadVarsValues(varsPath)
	if err != nil {
		t.Fatal(err)
	}
	st := state.DefaultState()
	sch.Fill(st, values)
	return st
}

func edit(st *state.AppState, name string, v interface{}) {
	st.VarsValues[name] = v
	st.VarsEdited[name] = true
}

func asYAML(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestSchemaModeNoEditIsNoChange(t *testing.T) {
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	before, _ := os.ReadFile(varsPath)
	out, err := BuildVars(varsPath, schemaState(t, varsPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(before) {
		t.Error("a save with nothing changed changed the file")
	}
}

func TestSchemaModeEditsOnlyTheirLines(t *testing.T) {
	varsPath := copyTestdata(t, "k8s-installer-vars.yml")
	before, _ := os.ReadFile(varsPath)
	st := schemaState(t, varsPath)
	edit(st, "ki_cp_ha_mode_vip", "192.168.0.250")
	edit(st, "kubelet_max_pods", 250)
	edit(st, "ki_cp_ha_mode_vip_carrier", "aws-eni")
	edit(st, "k8s_kube_proxy_enabled", false)
	edit(st, "internal_network_subnets", []interface{}{"10.0.0.0/24", "10.0.1.0/24"})
	edit(st, "internal_network_extra_zone", "aipub.test") // shipped commented out

	out, err := BuildVars(varsPath, st)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"ki_cp_ha_mode_vip: \"192.168.0.250\"\n",
		"kubelet_max_pods: 250\n",
		"ki_cp_ha_mode_vip_carrier: \"aws-eni\"\n",
		"k8s_kube_proxy_enabled: false\n",
		"internal_network_subnets:\n  - \"10.0.0.0/24\"\n  - \"10.0.1.0/24\"\n",
		"\ninternal_network_extra_zone: \"aipub.test\"\n#internal_network_extra_zone_a_records:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// Everything else is the file as it was: the lines that are new are those
	// of the six variables and nothing more.
	was := map[string]int{}
	for _, l := range strings.Split(string(before), "\n") {
		was[l]++
	}
	changed := 0
	for _, l := range strings.Split(got, "\n") {
		if was[l] == 0 {
			changed++
		} else {
			was[l]--
		}
	}
	if changed != 7 { // six variables, one of them on two new lines
		t.Errorf("%d new lines, want 7", changed)
	}
	m := mustParse(t, out)
	if m["kubelet_max_pods"] != 250 || m["ki_etcd_backup_retention_count"] != 5 {
		t.Errorf("parsed back: %v %v", m["kubelet_max_pods"], m["ki_etcd_backup_retention_count"])
	}
}

func TestSchemaModeAddsAVariableTheFileLacks(t *testing.T) {
	// A vars.yml from before a variable existed: the schema says the installer
	// has it, so it is added rather than left out.
	varsPath := writeFile(t, "vars.yml", "ki_cp_ha_mode: false\n# kept\nsysctl_extra: {}\n")
	st := schemaState(t, varsPath)
	edit(st, "kubelet_max_pods", 300)
	out, err := BuildVars(varsPath, st)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ki_cp_ha_mode: false\n# kept\nsysctl_extra: {}\n\nkubelet_max_pods: 300\n"; string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestSchemaModeNewFileIsWhatTheInstallerShips(t *testing.T) {
	varsPath := filepath.Join(t.TempDir(), "group_vars", "all", "vars.yml")
	st := schemaState(t, varsPath)
	edit(st, "ki_cp_ha_mode_vip", "192.168.0.250")
	out, err := BuildVars(varsPath, st)
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, out)

	shippedBytes, err := os.ReadFile(filepath.Join("testdata", "k8s-installer-vars.yml"))
	if err != nil {
		t.Fatal(err)
	}
	shipped := mustParse(t, shippedBytes)
	for name, want := range shipped {
		if name == "ki_cp_ha_mode_vip" {
			continue
		}
		if got, ok := m[name]; !ok {
			t.Errorf("%s is missing from the new file", name)
		} else if asYAML(t, got) != asYAML(t, want) {
			t.Errorf("%s = %v, the installer ships %v", name, got, want)
		}
	}
	if len(m) != len(shipped) {
		t.Errorf("%d variables written, the installer ships %d", len(m), len(shipped))
	}
	if m["ki_cp_ha_mode_vip"] != "192.168.0.250" {
		t.Error("the edit is missing")
	}
}
