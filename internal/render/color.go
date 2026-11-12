// Package render prints port reports as human-readable text or as JSON.
package render

import (
	"io"
	"os"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

// Colors decides whether ANSI colour sequences are written.
type Colors struct {
	Enabled bool
}

// ColorsFor returns the colour setting for w. Colours are disabled when
// noColorFlag is set, when the NO_COLOR environment variable is set to a
// non-empty value, or when w is not a terminal.
func ColorsFor(w io.Writer, noColorFlag bool) Colors {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return Colors{}
	}
	return Colors{Enabled: isTerminal(w)}
}

// isTerminal reports whether w is a character device such as a TTY.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (c Colors) paint(code, s string) string {
	if !c.Enabled {
		return s
	}
	return code + s + ansiReset
}
