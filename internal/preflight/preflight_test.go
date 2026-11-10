package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests run this test binary in place of ansible-playbook: with
// KI_FAKE_ANSIBLE set it does what the playbook does as far as this package can
// tell - writes the report to the path it was given with -e - and exits the way
// the playbook does when a check failed.
func TestMain(m *testing.M) {
	if mode := os.Getenv("KI_FAKE_ANSIBLE"); mode != "" {
		os.Exit(fakeAnsible(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeAnsible is the stand-in. mode is a report file to hand back, or "broken"
// for a playbook that stops before it writes one.
func fakeAnsible(mode string, args []string) int {
	if mode == "broken" {
		fmt.Println("ERROR! the playbook: playbooks/preflight.yml could not be found")
		return 4
	}
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && strings.HasPrefix(args[i+1], "ki_preflight_report=") {
			data, err := os.ReadFile(mode)
			if err != nil {
				fmt.Println(err)
				return 3
			}
			if err := os.WriteFile(strings.TrimPrefix(args[i+1], "ki_preflight_report="), data, 0600); err != nil {
				fmt.Println(err)
				return 3
			}
			return 2 // what ansible-playbook exits with when a task failed
		}
	}
	fmt.Println("no ki_preflight_report given")
	return 3
}

func installerWithPlaybook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "playbooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "playbooks", "preflight.yml"), []byte("# stand-in\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "inventory.yml")
}

func TestAvailableOnlyBesideAnInstallerThatHasThePlaybook(t *testing.T) {
	if Available(filepath.Join(t.TempDir(), "inventory.yml")) {
		t.Error("an installer without playbooks/preflight.yml has no preflight")
	}
	if !Available(installerWithPlaybook(t)) {
		t.Error("the playbook beside the inventory was not found")
	}
}

// testdata/report-fail.json is what the playbook wrote on a real control node
// with two nodes that pass and one address nothing answers on, with the names
// and the address of the machines replaced by example ones.
func TestRunReadsTheReportOfAPlaybookThatFoundProblems(t *testing.T) {
	report, _ := filepath.Abs(filepath.Join("testdata", "report-fail.json"))
	t.Setenv("KI_FAKE_ANSIBLE", report)

	r, err := Run(installerWithPlaybook(t), os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || r.Count("fail") != 1 || r.Count("ok") != 16 {
		t.Errorf("ok=%v fail=%d ok=%d", r.OK, r.Count("fail"), r.Count("ok"))
	}
	var failed Row
	for _, row := range r.Rows {
		if row.Status == "fail" {
			failed = row
		}
	}
	if failed.Node != "node3" || failed.Check != "reachable" || !strings.Contains(failed.Fix, "inventory.yml") {
		t.Errorf("failed row: %+v", failed)
	}
}

func TestRunSaysWhyWhenThePlaybookWroteNoReport(t *testing.T) {
	t.Setenv("KI_FAKE_ANSIBLE", "broken")

	_, err := Run(installerWithPlaybook(t), os.Args[0])
	if err == nil || !strings.Contains(err.Error(), "could not be found") {
		t.Errorf("the end of what the playbook printed should be in the error: %v", err)
	}
}

func TestFindAnsible(t *testing.T) {
	if p, err := FindAnsible("inventory.yml", "/given/ansible-playbook"); err != nil || p != "/given/ansible-playbook" {
		t.Errorf("the path given should be used: %q %v", p, err)
	}
	// The virtual environment setup.sh puts beside the ansible directory
	root := t.TempDir()
	venv := filepath.Join(root, "venv", "bin")
	os.MkdirAll(venv, 0755)
	os.MkdirAll(filepath.Join(root, "ansible"), 0755)
	os.WriteFile(filepath.Join(venv, "ansible-playbook"), []byte(""), 0755)
	t.Setenv("PATH", t.TempDir())
	p, err := FindAnsible(filepath.Join(root, "ansible", "inventory.yml"), "")
	if err != nil || filepath.Clean(p) != filepath.Join(venv, "ansible-playbook") {
		t.Errorf("found %q %v", p, err)
	}
}
