package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const indent = "       " // 7 spaces: aligns details under the text after ":<port>  ".

// Text writes one block per busy socket and one line per free port.
func Text(w io.Writer, reports []report.PortReport, c Colors) error {
	var b strings.Builder
	for _, r := range reports {
		writeReport(&b, r, c)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write text report: %w", err)
	}
	return nil
}

func writeReport(b *strings.Builder, r report.PortReport, c Colors) {
	if len(r.Findings) == 0 {
		if r.TimeWaitOnly {
			fmt.Fprintf(b, ":%d  free (only lingering time_wait connections)\n", r.Port)
			return
		}
		fmt.Fprintf(b, ":%d  free\n", r.Port)
		return
	}
	for _, f := range r.Findings {
		writeFinding(b, f, c)
	}
}

func writeFinding(b *strings.Builder, f report.Finding, c Colors) {
	fmt.Fprintf(b, ":%d  %s %s  → %s", f.Socket.Port, textutil.Clean(strings.ToUpper(string(f.Socket.Protocol))), f.Socket.Addr, owner(f, c))
	if f.Explanation.Title != "" {
		fmt.Fprintf(b, " · %s", textutil.Clean(f.Explanation.Title))
	}
	b.WriteByte('\n')

	if len(f.Explanation.Details) > 0 {
		parts := make([]string, 0, len(f.Explanation.Details))
		for _, d := range f.Explanation.Details {
			parts = append(parts, strings.TrimSpace(textutil.Clean(d.Key)+" "+textutil.Clean(d.Value)))
		}
		fmt.Fprintf(b, "%s%s\n", indent, strings.Join(parts, " · "))
	}

	hints := ""
	if len(f.Explanation.Hints) > 0 {
		hints = "(" + textutil.Clean(strings.Join(f.Explanation.Hints, "; ")) + ")"
	}
	switch {
	case f.Explanation.Terminator != nil && hints != "":
		fmt.Fprintf(b, "%sStop: %s   %s\n", indent, textutil.Clean(f.Explanation.Terminator.Describe()), hints)
	case f.Explanation.Terminator != nil:
		fmt.Fprintf(b, "%sStop: %s\n", indent, textutil.Clean(f.Explanation.Terminator.Describe()))
	case hints != "":
		fmt.Fprintf(b, "%s%s\n", indent, hints)
	}

	for _, a := range f.Advice {
		marker := c.paint(ansiCyan, "i")
		if a.Level == advise.Warn {
			marker = c.paint(ansiYellow, "!")
		}
		fmt.Fprintf(b, "%s%s %s\n", indent, marker, textutil.Clean(a.Text))
	}
}

// owner is the coloured target of the header arrow.
func owner(f report.Finding, c Colors) string {
	switch {
	case f.Process != nil:
		return c.paint(ansiGreen, fmt.Sprintf("%s (PID %d)", textutil.Clean(f.Process.Name), f.Process.PID))
	case f.Socket.PID != 0:
		return c.paint(ansiGreen, fmt.Sprintf("PID %d", f.Socket.PID))
	default:
		return c.paint(ansiYellow, "owner not visible")
	}
}
