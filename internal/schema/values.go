package schema

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// wholeNumbers turns the float64 that encoding/json makes of every number back
// into an int where it is one, so that 5 is written to vars.yml as 5.
func wholeNumbers(v interface{}) interface{} {
	switch x := v.(type) {
	case float64:
		if x == float64(int(x)) {
			return int(x)
		}
	case []interface{}:
		for i := range x {
			x[i] = wholeNumbers(x[i])
		}
	case map[string]interface{}:
		for k := range x {
			x[k] = wholeNumbers(x[k])
		}
	}
	return v
}

// Fill puts the state into schema mode: the variables are the ones of the
// schema, each with the value vars.yml has for it (fileValues) or, where the
// file says nothing, the value the installer ships. A variable the installer
// ships commented out stays unset until the person sets it.
func (s *Schema) Fill(st *state.AppState, fileValues map[string]interface{}) {
	st.SchemaMode = true
	st.VarsOrder = nil
	st.VarsValues = map[string]interface{}{}
	st.VarsEdited = map[string]bool{}
	for _, f := range s.Fields {
		st.VarsOrder = append(st.VarsOrder, f.Name)
		if v, ok := fileValues[f.Name]; ok {
			st.VarsValues[f.Name] = v
		} else if !f.Commented {
			st.VarsValues[f.Name] = f.Default
		}
	}
}

// Show is a value as one line of text, the form ParseInput reads back.
func (f Field) Show(v interface{}, set bool) string {
	if !set {
		return "(설정 안 함)"
	}
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case []interface{}:
		if f.Kind == KindObject {
			return fmt.Sprintf("(항목 %d개)", len(x))
		}
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ", ")
	case map[string]interface{}:
		return fmt.Sprintf("(키 %d개)", len(x))
	}
	return fmt.Sprint(v)
}

// ParseInput turns what was typed into the value to store, or says why not.
// Empty text is null where the variable allows null. What is checked is what
// the schema states about the one variable (number, address, subnet, pattern);
// what several variables have to add up to is the installer's validator to say.
func (f Field) ParseInput(text string) (interface{}, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "null" {
		if f.Nullable {
			return nil, nil
		}
		if f.Kind == KindList {
			return []interface{}{}, nil
		}
		return nil, fmt.Errorf("값을 입력해주세요 (비울 수 없는 변수입니다)")
	}
	switch f.Kind {
	case KindBool:
		switch strings.ToLower(text) {
		case "true", "yes", "y":
			return true, nil
		case "false", "no", "n":
			return false, nil
		}
		return nil, fmt.Errorf("true 또는 false를 입력해주세요")
	case KindInt:
		n, err := strconv.Atoi(text)
		if err != nil {
			return nil, fmt.Errorf("숫자를 입력해주세요")
		}
		return n, nil
	case KindEnum:
		for _, e := range f.Enum {
			if e == text {
				return text, nil
			}
		}
		return nil, fmt.Errorf("%s 중 하나여야 합니다", strings.Join(f.Enum, ", "))
	case KindList:
		var items []interface{}
		for _, part := range strings.Split(text, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if err := f.checkText(part); err != nil {
				return nil, fmt.Errorf("%q: %v", part, err)
			}
			items = append(items, part)
		}
		return items, nil
	case KindString:
		if err := f.checkText(text); err != nil {
			return nil, err
		}
		return text, nil
	}
	return nil, fmt.Errorf("이 변수는 vars.yml에서 직접 고쳐주세요")
}

func (f Field) checkText(text string) error {
	switch f.Format {
	case "ipv4":
		if ip := net.ParseIP(text); ip == nil || ip.To4() == nil {
			return fmt.Errorf("IPv4 주소가 아닙니다 (예: 192.168.0.10)")
		}
	case "ipv4network":
		if _, _, err := net.ParseCIDR(text); err != nil {
			return fmt.Errorf("서브넷 형식이 아닙니다 (예: 192.168.0.0/24)")
		}
	}
	if f.Pattern != "" {
		// The patterns are written for python. Most are plain enough for Go to
		// read too; one that is not is left to the installer's validator
		if re, err := regexp.Compile(f.Pattern); err == nil && !re.MatchString(text) {
			return fmt.Errorf("형식이 맞지 않습니다 (%s)", f.Pattern)
		}
	}
	return nil
}
