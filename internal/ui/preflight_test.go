package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/preflight"
)

// With KI_FAKE_ANSIBLE set this test binary stands in for ansible-playbook, as
// in the tests of internal/preflight.
func TestMain(m *testing.M) {
	if mode := os.Getenv("KI_FAKE_ANSIBLE"); mode != "" {
		os.Exit(fakeAnsible(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeAnsible(mode string, args []string) int {
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && strings.HasPrefix(args[i+1], "ki_preflight_report=") && mode != "broken" {
			data, _ := os.ReadFile(mode)
			os.WriteFile(strings.TrimPrefix(args[i+1], "ki_preflight_report="), data, 0600)
			return 2
		}
	}
	os.Stdout.WriteString("ERROR! the playbook could not be run\n")
	return 4
}

// pressAndRun sends a key and, as the bubbletea runtime would, runs the command
// it started and delivers what that came back with.
func pressAndRun(t *testing.T, a *App, key string) {
	t.Helper()
	_, cmd := a.Update(keyMsg(key))
	if cmd == nil {
		t.Fatalf("%q started nothing", key)
	}
	a.Update(cmd())
}

func schemaAppAtPreview(t *testing.T) *App {
	t.Helper()
	a, inv, _ := newSchemaAppFromMain(t)
	os.MkdirAll(filepath.Join(filepath.Dir(inv), "playbooks"), 0755)
	os.WriteFile(preflight.PlaybookPath(inv), []byte("# stand-in\n"), 0644)
	a.appState.AnsiblePlaybook = os.Args[0]
	press(t, a, "ctrl+n", "ctrl+n", "ctrl+n")
	if a.current != 3 {
		t.Fatalf("on screen %d, want the preview", a.current+1)
	}
	return a
}

func TestPreflightFromThePreview(t *testing.T) {
	report, _ := filepath.Abs(filepath.Join("..", "preflight", "testdata", "report-fail.json"))
	t.Setenv("KI_FAKE_ANSIBLE", report)
	a := schemaAppAtPreview(t)

	if v := a.View(); !strings.Contains(v, "c: 사전 점검") || strings.Contains(v, "[ 사전 점검 ]") {
		t.Fatalf("the preview should offer the check and not show a result yet:\n%s", v)
	}
	pressAndRun(t, a, "c")
	v := a.View()
	for _, want := range []string{"[ 사전 점검 ]", "실패 1", "통과 16", "✗ node3 · SSH 접속", "connect to host 192.0.2.250", "→ Check ansible_host"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the result lacks %q:\n%s", want, v)
		}
	}
	// What failed comes before what passed
	if strings.Index(v, "node3") > strings.Index(v, "✓ vars") {
		t.Error("the failed row should be listed first")
	}
	_, contentW, _ := a.dims()
	checkWidth(t, a, 120, contentW-2)
	// The files are still there to look at
	press(t, a, "left")
	if v := a.View(); !strings.Contains(v, "[ vars.yml ]") {
		t.Errorf("left of the result is vars.yml:\n%s", v)
	}
}

func TestPreflightThatCouldNotRunSaysWhy(t *testing.T) {
	t.Setenv("KI_FAKE_ANSIBLE", "broken")
	a := schemaAppAtPreview(t)

	pressAndRun(t, a, "c")
	if v := a.View(); !strings.Contains(v, "점검을 실행하지 못했습니다") || !strings.Contains(v, "could not be run") {
		t.Fatalf("the reason should be shown:\n%s", v)
	}
}

func TestNoPreflightBesideAnInstallerWithoutThePlaybook(t *testing.T) {
	a, _, _ := newSchemaAppFromMain(t)
	press(t, a, "ctrl+n", "ctrl+n", "ctrl+n")
	if v := a.View(); strings.Contains(v, "사전 점검") {
		t.Fatalf("an installer without the playbook should not be offered a check:\n%s", v)
	}
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}); cmd != nil {
		t.Error("c should do nothing there")
	}
}
