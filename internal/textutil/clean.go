// Package textutil holds helpers for printing externally sourced text safely.
package textutil

import (
	"strings"
	"unicode"
)

// Clean makes an externally sourced string safe to print on a terminal. Process
// names, command lines and container metadata are controlled by whoever started
// the process, so control characters (ESC, C1 controls such as CSI, BEL, CR,
// newlines, tabs) are replaced with '?' and invalid UTF-8 with U+FFFD. Without
// this a hostile name could inject escape sequences or rewrite a confirmation
// prompt.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return '?'
		}
		return r
	}, s)
}
