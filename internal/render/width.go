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

// wrapWords breaks s at spaces into lines of at most width runes. A word
// longer than width stays whole on a line of its own. A width of 0 or less
// means unlimited.
func wrapWords(s string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return []string{s}
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}
