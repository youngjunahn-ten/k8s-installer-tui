package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0..N-1  : table rows  (←/→: ki_cp(0) / Control(1) / Master(2) / GPU Worker(3))
//   N       : nav slot    (←/→: 이전/다음)
//
// CPU Worker column is derived (read-only): node with neither Master nor GPU Worker.
// All nodes are automatically k8s_node — that column is not shown.

const (
	colKiCp    = iota // ki_cp_node
	colControl        // control_node_ih → "Control" (one node, must be ki_cp)
	colMaster         // k8s_cp → "Master Node"  (exclusive with colGPU)
	colGPU            // nvidia_gpu → "GPU Worker" (exclusive with colMaster)
	colCount          // number of interactive columns (CPU Worker is derived)
)

type groupRow struct {
	nodeName string
	checks   [colCount]bool
}

type S2Groups struct {
	rows     []groupRow
	curRow   int
	curCol   int // 0=ki_cp, 1=control, 2=master, 3=gpu  (CPU Worker col is read-only)
	focusNav bool
	navIdx   int // 0=이전, 1=다음
	width    int
	height   int
}

func NewS2Groups() *S2Groups { return &S2Groups{} }

func (s *S2Groups) Title() string { return "그룹 할당" }
func (s *S2Groups) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S2Groups) SyncFromState(st *state.AppState) {
	s.rows = make([]groupRow, len(st.Nodes))
	for i, n := range st.Nodes {
		s.rows[i] = groupRow{
			nodeName: n.Name,
			checks: [colCount]bool{
				colKiCp:    st.IsKiCpNode(n.Name),
				colControl: st.IsControlNode(n.Name),
				colMaster:  st.IsK8sCpNode(n.Name),
				colGPU:     st.IsNvidiaGPUNode(n.Name),
			},
		}
	}
	s.ensureControl()
	s.curRow = 0
	s.focusNav = false
}

func (s *S2Groups) SyncToState(st *state.AppState) {
	// All nodes are k8s_node members automatically.
	st.K8sNodes = make([]string, len(s.rows))
	for i, r := range s.rows {
		st.K8sNodes[i] = r.nodeName
	}
	st.KiCpNodes = nil
	st.K8sCpNodes = nil
	st.NvidiaGPUNodes = nil
	st.ControlNodeIH = ""
	for _, r := range s.rows {
		if r.checks[colControl] {
			st.ControlNodeIH = r.nodeName
		}
		if r.checks[colKiCp] {
			st.KiCpNodes = append(st.KiCpNodes, r.nodeName)
		}
		if r.checks[colMaster] {
			st.K8sCpNodes = append(st.K8sCpNodes, r.nodeName)
		}
		if r.checks[colGPU] {
			st.NvidiaGPUNodes = append(st.NvidiaGPUNodes, r.nodeName)
		}
	}
}

func (s *S2Groups) Validate() []string { return nil }

func (s *S2Groups) Init() tea.Cmd { return nil }

func (s *S2Groups) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusNav {
				s.focusNav = false
			} else if s.curRow > 0 {
				s.curRow--
			}
		case "down", "j":
			if !s.focusNav {
				if s.curRow < len(s.rows)-1 {
					s.curRow++
				} else {
					s.focusNav = true
					s.navIdx = 0
				}
			}
		case "left", "h":
			if s.focusNav {
				if s.navIdx > 0 {
					s.navIdx--
				}
			} else if s.curCol > 0 {
				s.curCol--
			}
		case "right", "l":
			if s.focusNav {
				if s.navIdx < 1 {
					s.navIdx++
				}
			} else if s.curCol < colCount-1 {
				s.curCol++
			}
		case " ", "enter":
			if s.focusNav {
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			}
			if len(s.rows) > 0 {
				s.toggle(s.curRow, s.curCol)
				s.ensureControl()
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

// toggle applies mutual exclusion between Master and GPU Worker.
func (s *S2Groups) toggle(row, col int) {
	r := &s.rows[row]
	switch col {
	case colControl:
		// One control node, and it must be a ki_cp node: choosing it checks ki_cp too.
		for i := range s.rows {
			s.rows[i].checks[colControl] = false
		}
		r.checks[colControl] = true
		r.checks[colKiCp] = true
	case colKiCp:
		r.checks[colKiCp] = !r.checks[colKiCp]
		if !r.checks[colKiCp] {
			r.checks[colControl] = false
		}
	case colMaster:
		if r.checks[colMaster] {
			r.checks[colMaster] = false
		} else {
			r.checks[colMaster] = true
			r.checks[colGPU] = false
		}
	case colGPU:
		if r.checks[colGPU] {
			r.checks[colGPU] = false
		} else {
			r.checks[colGPU] = true
			r.checks[colMaster] = false
		}
	default:
		r.checks[col] = !r.checks[col]
	}
}

func (s *S2Groups) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("그룹 할당") + "\n")
	b.WriteString(styles.StyleMuted.Render("각 노드의 역할을 선택합니다.  ←/→: 열 이동  Space: 선택") + "\n")
	b.WriteString(styles.StyleMuted.Copy().Width(s.width).Render("Control: 설치를 실행하는 노드(control_node_ih). ki_cp_node 중 하나여야 합니다") + "\n\n")

	// 5 display columns: ki_cp | Control | Master | GPU Worker | CPU Worker(derived)
	colLabels := []string{"ki_cp_node", "Control", "Master Node", "GPU Worker", "CPU Worker"}
	// 66 columns in all, as before the Control column: the room an 80 column window gives
	colW := []int{11, 12, 9, 12, 11, 11}

	header := lipgloss.NewStyle().Width(colW[0]).Bold(true).Render("노드")
	for i, l := range colLabels {
		header += lipgloss.NewStyle().Width(colW[i+1]).Bold(true).Foreground(styles.ColorPrimary).Render(l)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	for rowIdx, r := range s.rows {
		isSelectedRow := !s.focusNav && rowIdx == s.curRow
		b.WriteString(renderGroupRow(r, colW, isSelectedRow, s.curCol) + "\n")
	}

	prevFocused := s.focusNav && s.navIdx == 0
	nextFocused := s.focusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

func renderGroupRow(r groupRow, colW []int, selected bool, curCol int) string {
	isCPU := !r.checks[colMaster] && !r.checks[colGPU]

	// One mark per interactive column, then the derived CPU Worker column.
	var marks [colCount + 1]string
	for c := 0; c < colCount; c++ {
		marks[c] = styles.CheckOff()
		if r.checks[c] {
			marks[c] = styles.CheckOn()
		}
	}
	marks[colCount] = styles.StyleMuted.Render("-")
	if isCPU {
		marks[colCount] = styles.CheckOn()
	}

	if selected {
		rs := focusedStyle(0)
		cs := lipgloss.NewStyle().Background(styles.AccentBg).Foreground(styles.FocusFg).Bold(true)

		row := rs.Width(2).Render("▶") + rs.Width(colW[0]-2).Render(r.nodeName)
		for i := 0; i < len(marks); i++ {
			st := rs
			if i < colCount && i == curCol {
				st = cs
			}
			row += st.Width(colW[i+1]).Align(lipgloss.Center).Render(marks[i])
		}
		return row
	}

	row := "  " + lipgloss.NewStyle().Width(colW[0]-2).Render(r.nodeName)
	for i := 0; i < len(marks); i++ {
		row += lipgloss.NewStyle().Width(colW[i+1]).Align(lipgloss.Center).Render(marks[i])
	}
	return row
}

// ensureControl keeps exactly one control node among the ki_cp nodes: when none is
// chosen (an inventory without control_node_ih, or its ki_cp box was cleared), the
// first ki_cp node becomes the control node, as in the k8s-installer example.
func (s *S2Groups) ensureControl() {
	for _, r := range s.rows {
		if r.checks[colControl] && r.checks[colKiCp] {
			return
		}
	}
	for i := range s.rows {
		s.rows[i].checks[colControl] = false
	}
	for i := range s.rows {
		if s.rows[i].checks[colKiCp] {
			s.rows[i].checks[colControl] = true
			return
		}
	}
}
