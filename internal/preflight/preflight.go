// Package preflight runs playbooks/preflight.yml of k8s-installer and reads its
// report: every node and the variables, checked before the installer is on a
// node, without changing anything.
//
// What is checked and what each failure means is the installer's to say. This
// package starts the playbook the way an operator would and shows the rows it
// wrote (-e ki_preflight_report=<file>); it holds no check of its own.
package preflight

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Row is one check of one node, as report-preflight.py writes it.
type Row struct {
	Node   string `json:"node"`
	Check  string `json:"check"`
	Status string `json:"status"` // ok, warn, fail, skip
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

// Report is the json the playbook writes.
type Report struct {
	OK   bool  `json:"ok"`
	Rows []Row `json:"rows"`
}

// Count returns how many rows have the status.
func (r *Report) Count(status string) int {
	n := 0
	for _, row := range r.Rows {
		if row.Status == status {
			n++
		}
	}
	return n
}

// PlaybookPath is where the playbook is when the installer has one: beside
// inventory.yml, where every playbook of the installer is.
func PlaybookPath(inventoryPath string) string {
	return filepath.Join(filepath.Dir(inventoryPath), "playbooks", "preflight.yml")
}

// Available reports whether the installer beside the inventory has the playbook.
func Available(inventoryPath string) bool {
	_, err := os.Stat(PlaybookPath(inventoryPath))
	return err == nil
}

// FindAnsible returns the ansible-playbook to run: the one given, the one on
// PATH (an operator who ran "source venv/bin/activate"), or the one of the
// installer's own virtual environment, which setup.sh puts beside the ansible
// directory.
func FindAnsible(inventoryPath, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if p, err := exec.LookPath("ansible-playbook"); err == nil {
		return p, nil
	}
	venv := filepath.Join(filepath.Dir(inventoryPath), "..", "venv", "bin", "ansible-playbook")
	if _, err := os.Stat(venv); err == nil {
		return venv, nil
	}
	return "", fmt.Errorf("ansible-playbook을 찾지 못했습니다. \"source <설치 경로>/venv/bin/activate\" 후 다시 실행하거나 --ansible-playbook으로 경로를 지정해주세요")
}

// Run runs the playbook from the directory of the inventory and returns its
// report. The playbook exits non-zero when a check failed, which is a report
// and not an error; an error is the playbook not getting as far as writing one,
// and then the end of what it printed is returned with it.
func Run(inventoryPath, ansible string) (*Report, error) {
	tmp, err := os.CreateTemp("", "ki-preflight-report-*.json")
	if err != nil {
		return nil, err
	}
	reportPath := tmp.Name()
	tmp.Close()
	os.Remove(reportPath)
	defer os.Remove(reportPath)

	cmd := exec.Command(ansible, "-i", inventoryPath, PlaybookPath(inventoryPath),
		"-e", "ki_preflight_report="+reportPath)
	cmd.Dir = filepath.Dir(inventoryPath)
	out, runErr := cmd.CombinedOutput()

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("점검이 결과를 쓰기 전에 끝났습니다 (%v)\n%s", runErr, tail(string(out), 15))
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("점검 결과를 읽지 못했습니다: %v", err)
	}
	return &report, nil
}

func tail(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Label is the name a check is shown under.
func Label(check string) string {
	switch check {
	case "variables":
		return "변수 검증"
	case "control_node":
		return "Control 노드"
	case "reachable":
		return "SSH 접속"
	case "sudo":
		return "sudo (비밀번호 없이)"
	case "sudo_shell":
		return "sudo 셸 실행"
	case "os":
		return "OS·아키텍처"
	case "package_manager":
		return "패키지 관리자"
	case "disk":
		return "디스크 여유"
	case "hostname":
		return "호스트 이름 중복"
	case "vip":
		return "VIP"
	}
	return check
}
