package render

import (
	"strings"
	"unicode/utf8"
)

const ellipsis = "…"

// fit shortens s to at most width runes, ending in an ellipsis when it had to
// cut. A width of 0 or less means unlimited. Wide glyphs count as one rune.
func fit(s string, width int) string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return s
	}
	runes := []rune(s)
	return string(runes[:width-1]) + ellipsis
}

// padRight pads s with spaces to width runes; it never shortens s.
func padRight(s string, width int) string {
	if n := width - utf8.RuneCountInString(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
