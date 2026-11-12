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
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
)

// Colors decides whether ANSI colour sequences are written.
type Colors struct {
	Enabled bool
}

// NewColors returns the colour setting for a stream. Colours are disabled
// when the stream is not a terminal, when noColorFlag is set, when the
// NO_COLOR environment variable is set to a non-empty value, or when TERM is
// "dumb".
func NewColors(isTTY, noColorFlag bool) Colors {
	if !isTTY || noColorFlag || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return Colors{}
	}
	return Colors{Enabled: true}
}

// ColorsFor returns the colour setting for w; see NewColors.
func ColorsFor(w io.Writer, noColorFlag bool) Colors {
	return NewColors(isTerminal(w), noColorFlag)
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

func (c Colors) bold(s string) string { return c.paint(ansiBold, s) }

func (c Colors) dim(s string) string { return c.paint(ansiDim, s) }
