package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

// Sources writes the footer line that names the sources found in a listing and
// how to filter by them. On a terminal it wraps to the width unless opts.Wide
// is set. It writes nothing when counts is empty.
func Sources(w io.Writer, counts []report.SourceCount, opts Options) error {
	if len(counts) == 0 {
		return nil
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%s (%d)", textutil.Clean(string(c.Source)), c.Count))
	}
	width := opts.Width
	if opts.Wide {
		width = 0
	}
	var b strings.Builder
	for _, line := range wrapWords(fmt.Sprintf("Sources: %s — filter with --source <name>", strings.Join(parts, " · ")), width) {
		fmt.Fprintln(&b, opts.Colors.dim(line))
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write sources footer: %w", err)
	}
	return nil
}
