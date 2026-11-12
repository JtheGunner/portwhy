package render

import (
	"strings"
	"unicode/utf8"
)

const ellipsis = "…"

const (
	// defaultLayoutWidth is the rule and box width when there is no terminal.
	defaultLayoutWidth = 78
	// maxLayoutWidth caps rules and boxes on wide terminals.
	maxLayoutWidth = 100
	// minLayoutWidth keeps rules and boxes readable on tiny terminals.
	minLayoutWidth = 40
)

// fit shortens s to at most width runes, ending in an ellipsis when it had to
// cut. A width of 0 or less means unlimited. Wide glyphs count as one rune.
func fit(s string, width int) string {
	return fitWith(s, width, ellipsis)
}

// fitWith is fit with a caller-chosen ellipsis, for example "..." in ASCII
// mode. The result is at most width runes long, ellipsis included.
func fitWith(s string, width int, ell string) string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return s
	}
	keep := width - utf8.RuneCountInString(ell)
	if keep < 0 {
		keep = 0
	}
	runes := []rune(s)
	return string(runes[:keep]) + string([]rune(ell)[:min(width, utf8.RuneCountInString(ell))])
}

// layoutWidth returns the width of rules and boxes. Without a terminal
// (width 0) it is a fixed default; on a terminal it is capped unless wide is
// set. It is never below minLayoutWidth.
func layoutWidth(width int, wide bool) int {
	switch {
	case width <= 0:
		return defaultLayoutWidth
	case !wide && width > maxLayoutWidth:
		return maxLayoutWidth
	case width < minLayoutWidth:
		return minLayoutWidth
	default:
		return width
	}
}

// visibleWidth counts the runes of s that are shown on screen: ANSI colour
// sequences (ESC [ ... m) are skipped.
func visibleWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}

// padVisible pads s with spaces to width visible runes; it never shortens s.
func padVisible(s string, width int) string {
	if n := width - visibleWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
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
