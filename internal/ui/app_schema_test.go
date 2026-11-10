package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Schema mode, driven with key presses from the files k8s-installer main ships
// and the schema its export writes, the way main.go starts it.

func newSchemaAppFromMain(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	inv := copyFromFileioTestdata(t, dir, "k8s-installer-inventory.yml", "inventory.yml")
	vars := copyFromFileioTestdata(t, dir, "k8s-installer-vars.yml", filepath.Join("group_vars", "all", "vars.yml"))
	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "vars.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := state.DefaultState()
	if err := fileio.LoadInventory(inv, s); err != nil {
		t.Fatal(err)
	}
	values, err := fileio.LoadVarsValues(vars)
	if err != nil {
		t.Fatal(err)
	}
	sch.Fill(s, values)
	a := NewSchemaApp(s, inv, vars, "test", sch)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return a, inv, vars
}

// find types a name into the filter of the vars screen and confirms it.
func find(t *testing.T, a *App, name string) {
	t.Helper()
	press(t, a, "esc", "/")
	for _, r := range name {
		press(t, a, string(r))
	}
	press(t, a, "enter")
}

// typeValue opens the current row, replaces its text and confirms.
func typeValue(t *testing.T, a *App, text string) {
	t.Helper()
	press(t, a, "enter", "ctrl+u")
	for _, r := range text {
		press(t, a, string(r))
	}
	press(t, a, "enter")
}

func TestSchemaWizardFromMainFilesToSave(t *testing.T) {
	a, invPath, varsPath := newSchemaAppFromMain(t)
	before, _ := os.ReadFile(varsPath)

	if v := a.View(); !strings.Contains(v, "3.설정") || strings.Contains(v, "AIPub") {
		t.Fatalf("the steps should be nodes, groups, settings, save:\n%s", v)
	}
	press(t, a, "ctrl+n", "ctrl+n")
	if a.current != 2 {
		t.Fatalf("on screen %d, want 3 (settings); errors %v", a.current+1, a.screenErrors)
	}
	view := a.View()
	for _, want := range []string{"ansible_user", "변수 63개", "ki_var_root_path"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings screen lacks %q:\n%s", want, view)
		}
	}

	// A text value: found by name, typed, and a wrong one refused with a reason.
	find(t, a, "ha_mode_vip")
	if v := a.View(); !strings.Contains(v, "변수 63개 중 2개 표시") {
		t.Fatalf("the filter should leave the vip and its carrier:\n%s", v)
	}
	typeValue(t, a, "not-an-address")
	if v := a.View(); !strings.Contains(v, "IPv4 주소가 아닙니다") {
		t.Fatalf("a wrong address should be refused where it is typed:\n%s", v)
	}
	press(t, a, "esc")
	typeValue(t, a, "192.168.0.250")

	// An enum: Enter moves to the next choice.
	press(t, a, "down", "enter")
	// A bool: Enter flips it.
	find(t, a, "kube_proxy")
	press(t, a, "enter")
	// A number, then back to what it was: not a change.
	find(t, a, "max_pods")
	typeValue(t, a, "250")
	press(t, a, "r")
	// A variable that can not be null says so.
	find(t, a, "k8s_pod_subnet")
	press(t, a, "x")
	if v := a.View(); !strings.Contains(v, "null로 둘 수 없는") {
		t.Fatalf("x on a required variable should be refused:\n%s", v)
	}
	// An object is not edited here.
	find(t, a, "apiserver_extra_args")
	press(t, a, "enter")
	if v := a.View(); !strings.Contains(v, "vars.yml에서 직접") {
		t.Fatalf("an object should point at the file:\n%s", v)
	}

	press(t, a, "esc")
	if v := a.View(); !strings.Contains(v, "바꾼 것 3개") {
		t.Fatalf("three variables were changed:\n%s", v)
	}
	press(t, a, "ctrl+n")
	if a.current != 3 {
		t.Fatalf("on screen %d, want 4 (preview); errors %v", a.current+1, a.screenErrors)
	}
	if v := a.View(); !strings.Contains(v, "검증 통과") {
		t.Fatalf("preview reports errors:\n%s", v)
	}
	press(t, a, "s")
	if v := a.View(); !strings.Contains(v, "저장 완료") {
		t.Fatalf("not saved:\n%s", v)
	}

	after, _ := os.ReadFile(varsPath)
	for _, want := range []string{
		"ki_cp_ha_mode_vip: \"192.168.0.250\"\n",
		"ki_cp_ha_mode_vip_carrier: \"aws-eni\"\n",
		"k8s_kube_proxy_enabled: false\n",
		"kubelet_max_pods: null\n",
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("saved vars.yml lacks %q", want)
		}
	}
	if len(strings.Split(string(after), "\n")) != len(strings.Split(string(before), "\n")) {
		t.Error("three values on one line each changed, so the line count should not")
	}
	inv, _ := os.ReadFile(invPath)
	if !strings.Contains(string(inv), "control_node_ih: node1") {
		t.Error("the inventory lost control_node_ih")
	}
}

func TestSchemaScreensFitTheirWidth(t *testing.T) {
	for _, w := range []int{80, 120, 200} {
		a, _, _ := newSchemaAppFromMain(t)
		a.Update(tea.WindowSizeMsg{Width: w, Height: 34})
		_, contentW, _ := a.dims()
		for i := 0; i < len(a.screenList); i++ {
			if a.current == 2 {
				// Every row scrolled through, and the edit line open
				for j := 0; j < 70; j++ {
					press(t, a, "down")
					checkWidth(t, a, w, contentW-2)
				}
				press(t, a, "up", "up", "enter")
			}
			checkWidth(t, a, w, contentW-2)
			press(t, a, "esc", "ctrl+n")
		}
	}
}

func checkWidth(t *testing.T, a *App, window, room int) {
	t.Helper()
	for _, line := range strings.Split(a.screenList[a.current].View(), "\n") {
		if lw := lipgloss.Width(line); lw > room {
			t.Fatalf("window %d, screen %d: line of %d columns, room for %d: %q", window, a.current+1, lw, room, line)
		}
	}
}

func TestSchemaScreenKeepsItsHeight(t *testing.T) {
	a, _, _ := newSchemaAppFromMain(t)
	press(t, a, "ctrl+n", "ctrl+n")
	lines := func() int { return strings.Count(a.screenList[a.current].View(), "\n") }
	want := lines()
	// Scrolling, filtering down to nothing, editing and an error must not make
	// the screen taller or shorter, or the frame jumps.
	for _, keys := range [][]string{
		{"down", "down", "down"}, {"/", "z", "z", "z", "enter"}, {"esc"},
		{"/", "v", "i", "p", "enter"}, {"enter"}, {"x", "enter"}, {"esc", "esc"},
	} {
		press(t, a, keys...)
		if got := lines(); got != want {
			t.Fatalf("after %v the screen is %d lines, was %d:\n%s", keys, got, want, a.screenList[a.current].View())
		}
	}
}
