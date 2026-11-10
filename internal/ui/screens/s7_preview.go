package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/preflight"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// Focus layout:
//   0  : content (viewport — ↑/↓ scroll, ←/→ 탭 전환)
//   1  : nav slot (←/→: 이전/저장)

const (
	s7FocusContent = iota
	s7FocusNav
	s7FocusMax = s7FocusNav
)

type S7Preview struct {
	inventoryContent string
	varsContent      string
	vp               viewport.Model
	activeTab        int // 0=inventory, 1=vars
	errors           []validator.Error
	saved            bool
	saveErr          string
	backups          []string          // backup files written by the last save
	notSaved         []string          // keys the existing vars.yml does not have (fileio.VarsNotSaved)
	pfRunning        bool              // the preflight playbook is running
	pfReport         *preflight.Report // what it last came back with
	pfErr            string            // or why it could not be run
	appState         *state.AppState
	inventoryPath    string
	varsPath         string

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS7Preview(s *state.AppState, inventoryPath, varsPath string) *S7Preview {
	return &S7Preview{
		vp:            viewport.New(80, 20),
		appState:      s,
		inventoryPath: inventoryPath,
		varsPath:      varsPath,
	}
}

func (s *S7Preview) Title() string { return "미리보기 & 저장" }
func (s *S7Preview) SetSize(w, h int) {
	s.width = w
	s.height = h
	vpH := h - 14
	if vpH < 5 {
		vpH = 5
	}
	s.vp.Width = w
	s.vp.Height = vpH
}

func (s *S7Preview) SyncFromState(st *state.AppState) {
	s.errors = validator.Validate(st)
	s.saved = false
	s.saveErr = ""

	inv, err := fileio.RenderInventoryString(s.inventoryPath, st)
	if err != nil {
		s.inventoryContent = fmt.Sprintf("렌더링 오류: %v", err)
	} else {
		s.inventoryContent = inv
	}

	vars, err := fileio.RenderVarsString(s.varsPath, st)
	if err != nil {
		s.varsContent = fmt.Sprintf("렌더링 오류: %v", err)
	} else {
		s.varsContent = vars
	}
	s.notSaved = fileio.VarsNotSaved(s.varsPath, st)

	s.activeTab = 0
	s.focusIdx = 0
	s.navIdx = 0
	s.refreshViewport()
}

func (s *S7Preview) SyncToState(_ *state.AppState) {}

func (s *S7Preview) Validate() []string {
	errs := validator.Validate(s.appState)
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return msgs
}

func (s *S7Preview) Init() tea.Cmd { return nil }

// PreflightDoneMsg is what the preflight playbook came back with.
type PreflightDoneMsg struct {
	Report *preflight.Report
	Err    error
}

// tabCount is two files, and the preflight result once there is one to show.
func (s *S7Preview) tabCount() int {
	if s.pfRunning || s.pfReport != nil || s.pfErr != "" {
		return 3
	}
	return 2
}

// startPreflight runs playbooks/preflight.yml of the installer beside the
// inventory. It reads the files on disk, so what it checks is what was saved.
func (s *S7Preview) startPreflight() (tea.Model, tea.Cmd) {
	if s.pfRunning || !preflight.Available(s.inventoryPath) {
		return s, nil
	}
	ansible, err := preflight.FindAnsible(s.inventoryPath, s.appState.AnsiblePlaybook)
	if err != nil {
		s.pfReport, s.pfErr = nil, err.Error()
		s.activeTab = 2
		s.refreshViewport()
		return s, nil
	}
	s.pfRunning, s.pfReport, s.pfErr = true, nil, ""
	s.activeTab = 2
	s.refreshViewport()
	inventoryPath := s.inventoryPath
	return s, func() tea.Msg {
		report, err := preflight.Run(inventoryPath, ansible)
		return PreflightDoneMsg{Report: report, Err: err}
	}
}

// preflightContent is the third tab: the rows of the report, the ones that need
// attention first, each with what the installer says to do about it.
func (s *S7Preview) preflightContent() string {
	w := s.width
	switch {
	case s.pfRunning:
		return strings.Join(wrapLines("점검 중입니다… 모든 노드에 접속해 확인합니다. 아무것도 바꾸지 않습니다.", w), "\n")
	case s.pfErr != "":
		return strings.Join(wrapLines("✗ 점검을 실행하지 못했습니다\n"+s.pfErr, w), "\n")
	case s.pfReport == nil:
		return ""
	}
	r := s.pfReport
	var lines []string
	head := fmt.Sprintf("실패 %d · 확인 필요 %d · 통과 %d · 건너뜀 %d   (저장된 파일 기준)",
		r.Count("fail"), r.Count("warn"), r.Count("ok"), r.Count("skip"))
	lines = append(lines, wrapLines(head, w)...)
	lines = append(lines, "")
	marks := map[string]string{"fail": "✗", "warn": "!", "ok": "✓", "skip": "-"}
	for _, status := range []string{"fail", "warn", "ok", "skip"} {
		for _, row := range r.Rows {
			if row.Status != status {
				continue
			}
			text := fmt.Sprintf("%s %s · %s", marks[status], row.Node, preflight.Label(row.Check))
			if row.Detail != "" && (status == "ok" || status == "skip") {
				text += "  " + strings.SplitN(row.Detail, "\n", 2)[0]
			}
			lines = append(lines, wrapLines(text, w)...)
			if status == "fail" || status == "warn" {
				for _, l := range wrapLines(row.Detail, w-4) {
					lines = append(lines, "    "+l)
				}
				for _, l := range wrapLines("→ "+row.Fix, w-4) {
					lines = append(lines, "    "+l)
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (s *S7Preview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PreflightDoneMsg:
		s.pfRunning, s.pfReport, s.pfErr = false, msg.Report, ""
		if msg.Err != nil {
			s.pfErr = msg.Err.Error()
		}
		s.activeTab = 2
		s.refreshViewport()
		return s, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "c":
			return s.startPreflight()
		case "up", "k":
			if s.focusIdx == s7FocusContent {
				s.vp.LineUp(1)
			} else {
				s.focusIdx = s7FocusContent
			}
			return s, nil
		case "down", "j":
			if s.focusIdx == s7FocusContent {
				if s.vp.AtBottom() {
					s.focusIdx = s7FocusNav
				} else {
					s.vp.LineDown(1)
				}
			}
			return s, nil
		case "left", "h":
			switch s.focusIdx {
			case s7FocusContent:
				if s.activeTab > 0 {
					s.activeTab--
					s.refreshViewport()
				}
			case s7FocusNav:
				if s.navIdx > 0 {
					s.navIdx--
				}
			}
		case "right", "l":
			switch s.focusIdx {
			case s7FocusContent:
				if s.activeTab < s.tabCount()-1 {
					s.activeTab++
					s.refreshViewport()
				}
			case s7FocusNav:
				if s.navIdx < 1 {
					s.navIdx++
				}
			}
		case "tab":
			s.focusIdx = s7FocusNav
		case "enter", " ":
			if s.saved {
				return s, tea.Quit
			}
			if s.focusIdx == s7FocusNav {
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s.doSave()
			}
		case "s":
			return s.doSave()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s *S7Preview) doSave() (tea.Model, tea.Cmd) {
	if len(s.errors) > 0 {
		s.saveErr = "검증 오류가 있어 저장할 수 없습니다"
		return s, nil
	}
	s.backups = nil
	invBak, err := fileio.WriteInventory(s.inventoryPath, s.appState)
	if err != nil {
		s.saveErr = fmt.Sprintf("inventory.yml 저장 실패: %v", err)
		return s, nil
	}
	if invBak != "" {
		s.backups = append(s.backups, invBak)
	}
	varsBak, err := fileio.WriteVars(s.varsPath, s.appState)
	if err != nil {
		s.saveErr = fmt.Sprintf("vars.yml 저장 실패: %v", err)
		return s, nil
	}
	if varsBak != "" {
		s.backups = append(s.backups, varsBak)
	}
	s.saved = true
	s.saveErr = ""
	return s, nil
}

func (s *S7Preview) refreshViewport() {
	switch s.activeTab {
	case 0:
		s.vp.SetContent(s.inventoryContent)
	case 1:
		s.vp.SetContent(s.varsContent)
	default:
		s.vp.SetContent(s.preflightContent())
	}
	s.vp.GotoTop()
}

func (s *S7Preview) scrollBar() string {
	hasAbove := s.vp.YOffset > 0
	hasBelow := !s.vp.AtBottom()

	lineLen := s.width
	if lineLen < 4 {
		lineLen = 4
	}

	topLine := strings.Repeat("─", lineLen)
	botLine := strings.Repeat("─", lineLen)

	if hasAbove {
		indicator := " ↑ "
		topLine = indicator + strings.Repeat("─", lineLen-len(indicator))
	}
	if hasBelow {
		indicator := " ↓ "
		botLine = strings.Repeat("─", lineLen-len(indicator)) + indicator
	}

	return topLine + "\n" + s.vp.View() + "\n" + botLine
}

func (s *S7Preview) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("미리보기 & 저장") + "\n\n")

	// Tabs — ←/→ 전환, 콘텐츠 포커스 시 활성 탭 표시
	contentFocused := s.focusIdx == s7FocusContent
	names := []string{"inventory.yml", "vars.yml", "사전 점검"}[:s.tabCount()]
	tabs := ""
	for i, name := range names {
		if i == s.activeTab {
			tabs += styles.StylePrimary.Render("[ " + name + " ]")
		} else {
			tabs += styles.StyleMuted.Render("  " + name + "  ")
		}
	}
	if contentFocused {
		tabs += styles.StyleMuted.Render("  ←/→")
	}
	b.WriteString(tabs + "\n")

	b.WriteString(s.scrollBar() + "\n")

	if preflight.Available(s.inventoryPath) {
		b.WriteString(styles.StyleMuted.Render(fit("c: 사전 점검 — 노드 접속·sudo·OS·디스크·변수를 한 번에 확인 (바꾸는 것 없음)", s.width)) + "\n")
	}

	if len(s.errors) == 0 {
		b.WriteString(styles.StyleSuccess.Render("✓ 검증 통과 — 저장 가능합니다") + "\n")
	} else {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("✗ 검증 오류 %d건:", len(s.errors))) + "\n")
		for _, e := range s.errors {
			b.WriteString(styles.StyleError.Render("  • "+e.Error()) + "\n")
		}
	}
	if len(s.notSaved) > 0 {
		b.WriteString(styles.StyleMuted.Copy().Width(s.width).Render("참고: 지금 vars.yml에 없는 키라 저장하지 않습니다(이 설치기 버전은 읽지 않음): "+
			strings.Join(s.notSaved, ", ")) + "\n")
	}

	if s.saveErr != "" {
		b.WriteString(styles.StyleError.Render("✗ "+s.saveErr) + "\n")
	}
	if s.saved {
		b.WriteString("\n" + styles.StyleSuccess.Render("✓ 저장 완료!") + "\n")
		b.WriteString(styles.StyleMuted.Render(fmt.Sprintf("  %s\n  %s", s.inventoryPath, s.varsPath)) + "\n")
		if len(s.backups) > 0 {
			b.WriteString(styles.StyleMuted.Render("  기존 파일 백업: "+strings.Join(s.backups, ", ")) + "\n")
			b.WriteString(styles.StyleMuted.Render("  TUI가 다루지 않는 키와 주석은 그대로 두었습니다") + "\n")
		}
		b.WriteString("\n" + styles.StyleTitle.Render("종료하시겠습니까?") + "  " +
			styles.StyleMuted.Render("Enter 확인") + "\n")
	}

	prevFocused := s.focusIdx == s7FocusNav && s.navIdx == 0
	saveFocused := s.focusIdx == s7FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "저장 (s)", prevFocused, saveFocused, s.width))

	return b.String()
}
