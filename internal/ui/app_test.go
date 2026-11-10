package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
	"gopkg.in/yaml.v3"
)

// Drives the real screens with key presses, from the files k8s-installer main
// ships to a saved inventory.yml / vars.yml, the way a person at the terminal would.

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends keys and follows the navigation commands they return, as the
// bubbletea runtime would.
func press(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var m tea.Model = a
		_, cmd := m.Update(keyMsg(k))
		for cmd != nil {
			switch msg := cmd().(type) {
			case screens.NavigateNextMsg, screens.NavigatePrevMsg:
				_, cmd = a.Update(msg)
			default:
				cmd = nil
			}
		}
	}
}

func copyFromFileioTestdata(t *testing.T, dir, name, as string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "fileio", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, as)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newAppFromMain(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	inv := copyFromFileioTestdata(t, dir, "k8s-installer-inventory.yml", "inventory.yml")
	vars := copyFromFileioTestdata(t, dir, "k8s-installer-vars.yml", filepath.Join("group_vars", "all", "vars.yml"))
	s := state.DefaultState()
	if err := fileio.LoadInventory(inv, s); err != nil {
		t.Fatal(err)
	}
	if err := fileio.LoadVars(vars, s); err != nil {
		t.Fatal(err)
	}
	// What the person types on screens 3 and 4: main ships ki_cp_ha_mode true with
	// no vip, and there is no ingress vip in main's vars.yml.
	s.KiCpHaModeVIP = "192.168.0.250"
	s.K8sIngressVIP = "192.168.0.100"
	a := NewApp(s, inv, vars, "test")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return a, inv, vars
}

func TestWizardFromMainFilesToSave(t *testing.T) {
	a, invPath, varsPath := newAppFromMain(t)

	if !strings.Contains(a.View(), "node4") {
		t.Fatal("screen 1 does not list the nodes of the inventory")
	}
	press(t, a, "ctrl+n")
	if a.current != 1 {
		t.Fatalf("screen %d, want 2 (groups); errors %v", a.current+1, a.screenErrors)
	}
	view := a.View()
	if !strings.Contains(view, "Control") || !strings.Contains(view, "control_node_ih") {
		t.Fatalf("groups screen has no Control column:\n%s", view)
	}
	if a.appState.ControlNodeIH != "node1" {
		t.Fatalf("control node loaded as %q", a.appState.ControlNodeIH)
	}

	// Row node2, column Control, Space: node2 becomes the control node.
	press(t, a, "down", "right", "space")
	// Row node2, column ki_cp, Space: node2 leaves ki_cp, so the control node
	// falls back to the first ki_cp node.
	press(t, a, "left", "space")
	press(t, a, "ctrl+n", "ctrl+p")
	if got := a.appState.ControlNodeIH; got != "node1" {
		t.Fatalf("after clearing ki_cp of node2 the control node is %q, want node1", got)
	}
	if a.appState.IsKiCpNode("node2") {
		t.Fatal("node2 should have left ki_cp_node")
	}
	// Control on node3 checks ki_cp for it as well.
	press(t, a, "down", "down", "right", "space")
	press(t, a, "ctrl+n")
	if got := a.appState.ControlNodeIH; got != "node3" || !a.appState.IsKiCpNode("node3") {
		t.Fatalf("control node %q, ki_cp %v", got, a.appState.KiCpNodes)
	}

	// Screens 3, 4, 5; screen 6 is skipped with AIPub off.
	press(t, a, "ctrl+n", "ctrl+n", "ctrl+n")
	if a.current != 6 {
		t.Fatalf("on screen %d, want 7 (preview); errors %v", a.current+1, a.screenErrors)
	}
	if v := a.View(); !strings.Contains(v, "검증 통과") {
		t.Fatalf("preview reports errors:\n%s", v)
	}
	press(t, a, "s")
	if v := a.View(); !strings.Contains(v, "저장 완료") || !strings.Contains(v, ".bak.") {
		t.Fatalf("save did not report a backup:\n%s", v)
	}

	var inv map[string]interface{}
	b, _ := os.ReadFile(invPath)
	if err := yaml.Unmarshal(b, &inv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "control_node_ih: node3") {
		t.Errorf("saved inventory:\n%s", b)
	}
	if _, ok := inv["broken_node"]; !ok {
		t.Error("broken_node lost")
	}
	v, _ := os.ReadFile(varsPath)
	for _, want := range []string{`ki_cp_ha_mode_vip: "192.168.0.250"`, "kubelet_max_pods: null", "ki_cp_ha_mode_vip_carrier"} {
		if !strings.Contains(string(v), want) {
			t.Errorf("saved vars.yml lacks %q", want)
		}
	}
	baks, _ := filepath.Glob(varsPath + ".bak.*")
	if len(baks) != 1 {
		t.Errorf("vars backups %v", baks)
	}
}

func TestSmallTerminalDoesNotPanic(t *testing.T) {
	a, _, _ := newAppFromMain(t)
	for _, size := range [][2]int{{40, 12}, {80, 24}, {200, 60}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for i := 0; i < 7; i++ {
			_ = a.View()
			press(t, a, "ctrl+n")
		}
		for i := 0; i < 7; i++ {
			press(t, a, "ctrl+p")
		}
	}
}

// On a Korean locale go-runewidth measures the frame's box characters as two
// columns; the frame must still fit the window.
func TestKoreanLocaleFrameFits(t *testing.T) {
	saved := runewidth.DefaultCondition.EastAsianWidth
	defer func() { runewidth.DefaultCondition.EastAsianWidth = saved }()
	runewidth.DefaultCondition.EastAsianWidth = true
	useNarrowAmbiguousWidth()

	a, _, _ := newAppFromMain(t)
	for i := 0; i < 7; i++ {
		for _, line := range strings.Split(a.View(), "\n") {
			if w := lipgloss.Width(line); w > 120 {
				t.Fatalf("screen %d: a line is %d columns in a 120 column window", a.current+1, w)
			}
			if strings.HasPrefix(strings.TrimLeft(line, " │"), "┓") {
				t.Fatalf("screen %d: the frame wrapped", a.current+1)
			}
		}
		press(t, a, "ctrl+n")
	}
}
