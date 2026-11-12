package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/JtheGunner/portwhy/internal/report"
)

// Options controls the text layout.
type Options struct {
	Colors Colors
	// Width is the terminal width in columns; 0 means unlimited.
	Width int
	// Wide disables shortening of long values.
	Wide bool
	// Overview selects the grouped list layout instead of detail blocks.
	Overview bool
	// ASCII replaces the Unicode symbols and line characters by plain ASCII.
	ASCII bool
	// Sources are the sources found before any --source filter; the overview
	// names them in its filter hint. Empty means the hint omits the filter.
	Sources []report.SourceCount
}

// glyphs returns the symbol set the options ask for.
func (o Options) glyphs() Glyphs {
	if o.ASCII {
		return asciiGlyphs
	}
	return unicodeGlyphs
}

// UseASCII reports whether the environment calls for plain ASCII output: a
// dumb terminal or a locale that is not UTF-8. getenv is os.Getenv in
// production.
func UseASCII(getenv func(string) string) bool {
	return glyphsFromEnv(getenv) == asciiGlyphs
}

// Text writes the reports as an overview (Options.Overview) or as detail
// blocks, one per service or process holding a port.
func Text(w io.Writer, reports []report.PortReport, opts Options) error {
	var b strings.Builder
	if opts.Overview {
		writeOverview(&b, reports, opts)
	} else {
		for _, r := range reports {
			writeDetail(&b, r, opts)
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write text report: %w", err)
	}
	return nil
}
