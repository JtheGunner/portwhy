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
