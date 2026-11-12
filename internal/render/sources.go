package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

// Sources writes the footer line that names the sources found in a listing and
// how to filter by them. It writes nothing when counts is empty.
func Sources(w io.Writer, counts []report.SourceCount) error {
	if len(counts) == 0 {
		return nil
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%s (%d)", textutil.Clean(string(c.Source)), c.Count))
	}
	if _, err := fmt.Fprintf(w, "Sources: %s — filter with --source <name>\n", strings.Join(parts, " · ")); err != nil {
		return fmt.Errorf("write sources footer: %w", err)
	}
	return nil
}
