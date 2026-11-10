package ui

import (
	"os"

	"github.com/mattn/go-runewidth"
)

// go-runewidth counts the "ambiguous" characters as two columns when the locale
// is Chinese, Japanese or Korean (LANG=ko_KR.UTF-8 on a server, code page 949 on
// Windows). The box drawing characters of the frame (┏ ━ ┃) are ambiguous, so on
// a Korean locale every bordered line was measured too wide and wrapped, and the
// screen fell apart. Terminals draw them one column wide, so measure them that
// way. Hangul is wide either way and is not affected. RUNEWIDTH_EASTASIAN=1
// still forces the old behaviour for a terminal that really draws them wide.
func init() { useNarrowAmbiguousWidth() }

func useNarrowAmbiguousWidth() {
	if os.Getenv("RUNEWIDTH_EASTASIAN") != "" {
		return
	}
	runewidth.DefaultCondition.EastAsianWidth = false
}
