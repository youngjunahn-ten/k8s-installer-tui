package screens

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// SVars is the vars.yml screen of schema mode: one row per variable the schema
// of the installer lists, in its order, with the input its kind calls for. No
// variable is named in this file, so a variable the installer adds shows up
// here when its schema does.
//
// Focus layout:
//
//	list    ↑/↓ rows, Enter/Space edit (bool: flip, enum: next, others: type),
//	        x null, r back to what it was, / find by name
//	nav     ←/→ 이전/다음, reached with ↓ past the last row
type SVars struct {
	sch    *schema.Schema
	values map[string]interface{}
	orig   map[string]interface{} // as first shown: what "r" goes back to
	edited map[string]bool

	cur      int // index into rows()
	top      int // first row shown
	focusNav bool
	navIdx   int

	editing   bool
	input     textinput.Model
	filtering bool
	filter    string
	message   string // why the last input was refused

	width, height int
}

func NewSVars(sch *schema.Schema) *SVars {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 512
	return &SVars{sch: sch, input: in}
}

func (s *SVars) Title() string { return "설정" }
func (s *SVars) SetSize(w, h int) {
	s.width, s.height = w, h
	s.input.Width = w - 12
}

func (s *SVars) SyncFromState(st *state.AppState) {
	s.values = map[string]interface{}{}
	for k, v := range st.VarsValues {
		s.values[k] = v
	}
	s.edited = map[string]bool{}
	for k, v := range st.VarsEdited {
		s.edited[k] = v
	}
	if s.orig == nil {
		s.orig = map[string]interface{}{}
		for k, v := range st.VarsValues {
			s.orig[k] = v
		}
	}
	s.editing, s.filtering, s.focusNav, s.message = false, false, false, ""
}

func (s *SVars) SyncToState(st *state.AppState) {
	st.VarsValues, st.VarsEdited = s.values, s.edited
}

func (s *SVars) Validate() []string { return nil }
func (s *SVars) Init() tea.Cmd      { return nil }

// rows are the fields the filter lets through.
func (s *SVars) rows() []schema.Field {
	if s.filter == "" {
		return s.sch.Fields
	}
	needle := strings.ToLower(s.filter)
	var out []schema.Field
	for _, f := range s.sch.Fields {
		if strings.Contains(f.Name, needle) {
			out = append(out, f)
		}
	}
	return out
}

func (s *SVars) visibleRows() int {
	n := s.height - 13
	if n < 3 {
		n = 3
	}
	return n
}

func (s *SVars) clamp() {
	n := len(s.rows())
	if s.cur >= n {
		s.cur = n - 1
	}
	if s.cur < 0 {
		s.cur = 0
	}
	if s.cur < s.top {
		s.top = s.cur
	}
	if s.cur >= s.top+s.visibleRows() {
		s.top = s.cur - s.visibleRows() + 1
	}
	if s.top < 0 {
		s.top = 0
	}
}

func (s *SVars) current() (schema.Field, bool) {
	rows := s.rows()
	if s.cur < 0 || s.cur >= len(rows) {
		return schema.Field{}, false
	}
	return rows[s.cur], true
}

// set stores a value and records whether it now differs from what was shown first.
func (s *SVars) set(name string, v interface{}) {
	s.values[name] = v
	was, had := s.orig[name]
	s.edited[name] = !had || !reflect.DeepEqual(was, v)
}

func (s *SVars) revert(name string) {
	if was, had := s.orig[name]; had {
		s.values[name] = was
	} else {
		delete(s.values, name)
	}
	delete(s.edited, name)
}

func (s *SVars) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch {
	case s.editing:
		return s.updateEditing(key)
	case s.filtering:
		return s.updateFiltering(key)
	case s.focusNav:
		return s.updateNav(key)
	}
	return s.updateList(key)
}

func (s *SVars) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s.message = ""
	switch key.String() {
	case "up", "k":
		s.cur--
	case "down", "j":
		if s.cur >= len(s.rows())-1 {
			s.focusNav, s.navIdx = true, 1
		} else {
			s.cur++
		}
	case "home", "g":
		s.cur = 0
	case "end", "G":
		s.cur = len(s.rows()) - 1
	case "/":
		s.filtering = true
	case "esc":
		s.filter = ""
	case "enter", " ":
		s.activate()
	case "x":
		if f, ok := s.current(); ok {
			if f.Nullable {
				s.set(f.Name, nil)
			} else {
				s.message = "null로 둘 수 없는 변수입니다"
			}
		}
	case "r":
		if f, ok := s.current(); ok {
			s.revert(f.Name)
		}
	case "ctrl+n":
		return s, Next()
	case "ctrl+p":
		return s, Prev()
	}
	s.clamp()
	return s, nil
}

// activate edits the current row the way its kind is edited.
func (s *SVars) activate() {
	f, ok := s.current()
	if !ok {
		return
	}
	v, set := s.values[f.Name]
	switch f.Kind {
	case schema.KindBool:
		b, _ := v.(bool)
		s.set(f.Name, !b)
	case schema.KindEnum:
		next := 0
		for i, e := range f.Enum {
			if e == fmt.Sprint(v) {
				next = (i + 1) % len(f.Enum)
			}
		}
		s.set(f.Name, f.Enum[next])
	case schema.KindObject:
		s.message = "목록·구조 값은 여기서 고치지 않습니다. vars.yml에서 직접 고쳐주세요"
	default:
		text := ""
		if set && v != nil {
			text = f.Show(v, true)
		}
		s.input.SetValue(text)
		s.input.CursorEnd()
		s.input.Focus()
		s.editing = true
	}
}

func (s *SVars) updateEditing(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.editing, s.message = false, ""
		s.input.Blur()
		return s, nil
	case "enter":
		f, _ := s.current()
		v, err := f.ParseInput(s.input.Value())
		if err != nil {
			s.message = err.Error()
			return s, nil
		}
		s.set(f.Name, v)
		s.editing, s.message = false, ""
		s.input.Blur()
		return s, nil
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(key)
	return s, cmd
}

func (s *SVars) updateFiltering(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEnter:
		s.filtering = false
	case tea.KeyEsc:
		s.filtering, s.filter = false, ""
	case tea.KeyBackspace:
		if r := []rune(s.filter); len(r) > 0 {
			s.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		s.filter += string(key.Runes)
	}
	s.cur, s.top = 0, 0
	s.clamp()
	return s, nil
}

func (s *SVars) updateNav(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		s.focusNav = false
	case "left", "h":
		s.navIdx = 0
	case "right", "l":
		s.navIdx = 1
	case "enter", " ":
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	case "ctrl+n":
		return s, Next()
	case "ctrl+p":
		return s, Prev()
	}
	return s, nil
}

// fit pads or cuts text to exactly w columns.
func fit(text string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) > w {
		text = runewidth.Truncate(text, w, "…")
	}
	return runewidth.FillRight(text, w)
}

func (s *SVars) View() string {
	var b strings.Builder
	rows := s.rows()
	changed := 0
	for _, e := range s.edited {
		if e {
			changed++
		}
	}

	b.WriteString(styles.StyleTitle.Render("설정") + styles.StyleMuted.Render(fit(
		fmt.Sprintf("  vars.yml · 변수 %d개 중 %d개 표시 · 바꾼 것 %d개", len(s.sch.Fields), len(rows), changed), s.width-4)) + "\n")
	b.WriteString(styles.StyleMuted.Render(fit("Enter: 고치기  x: null  r: 되돌리기  /: 이름으로 찾기", s.width)) + "\n")
	find := ""
	if s.filtering {
		find = "찾기: " + s.filter + "_   (Enter: 확정, Esc: 지우기)"
	} else if s.filter != "" {
		find = "찾기: " + s.filter + "   (Esc: 지우기)"
	}
	b.WriteString(fit(find, s.width) + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	nameW := 42
	if nameW > s.width*6/10 {
		nameW = s.width * 6 / 10
	}
	valW := s.width - 2 - nameW - 1
	n := s.visibleRows()
	for i := s.top; i < s.top+n; i++ {
		if i >= len(rows) {
			b.WriteString("\n")
			continue
		}
		f := rows[i]
		v, set := s.values[f.Name]
		mark := "  "
		if i == s.cur && !s.focusNav {
			mark = "▶ "
		}
		value := f.Show(v, set)
		if s.edited[f.Name] {
			value = "* " + value
		}
		line := mark + fit(f.Name, nameW) + " " + fit(value, valW)
		b.WriteString(RenderRow(line, i == s.cur && !s.focusNav, s.width) + "\n")
	}
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	// Detail of the current variable: what the installer says about it
	if f, ok := s.current(); ok {
		head := f.Name
		if f.Required {
			head += "  [필수]"
		}
		if f.Class != "" {
			head += "  적용: " + f.Class
		}
		if len(f.Enum) > 0 {
			head += "  선택: " + strings.Join(f.Enum, " | ")
		}
		b.WriteString(styles.StylePrimary.Render(fit(head, s.width)) + "\n")
		for _, line := range wrap(f.Summary(), s.width, 3) {
			b.WriteString(styles.StyleMuted.Render(line) + "\n")
		}
		shipped := "기본값: " + f.Show(f.Default, !f.Commented)
		if f.Commented {
			shipped = "예: " + f.Show(f.Example, true)
		}
		b.WriteString(styles.StyleMuted.Render(fit(shipped, s.width)) + "\n")
	} else {
		b.WriteString(fit("찾는 이름의 변수가 없습니다", s.width) + "\n\n\n\n\n")
	}

	switch {
	case s.editing:
		b.WriteString("새 값: " + s.input.View() + "\n")
	default:
		b.WriteString("\n")
	}
	if s.message != "" {
		b.WriteString(styles.StyleError.Render(fit("✗ "+s.message, s.width)) + "\n")
	} else {
		b.WriteString("\n")
	}

	b.WriteString(RenderNavButtons("이전", "다음", s.focusNav && s.navIdx == 0, s.focusNav && s.navIdx == 1, s.width))
	return b.String()
}

// wrap breaks text into exactly n lines of at most w columns; what does not fit
// ends in an ellipsis.
func wrap(text string, w, n int) []string {
	lines := make([]string, 0, n)
	line := ""
	words := strings.Fields(text)
	for i := 0; i < len(words); i++ {
		word := words[i]
		if line != "" && runewidth.StringWidth(line)+1+runewidth.StringWidth(word) > w {
			lines = append(lines, line)
			line = ""
			if len(lines) == n {
				lines[n-1] = fit(runewidth.Truncate(lines[n-1]+" …", w, "…"), w)
				break
			}
		}
		if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if len(lines) < n && line != "" {
		lines = append(lines, line)
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = fit(lines[i], w)
	}
	return lines
}
