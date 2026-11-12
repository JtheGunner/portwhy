package render

import (
	"fmt"
	"strconv"

	"github.com/JtheGunner/portwhy/internal/textutil"
)

// KillOutcome is how the attempt to stop one port ended.
type KillOutcome int

// The outcomes of a stop attempt.
const (
	KillStopped KillOutcome = iota
	KillDeclined
	KillFailed
)

const killLabelWidth = 16

// KillLine is the status line for one stopped port. detail is the error text
// of a failed attempt; it is external text and cleaned here.
func KillLine(outcome KillOutcome, port uint16, detail string, opts Options) string {
	g, c := opts.glyphs(), opts.Colors
	switch outcome {
	case KillStopped:
		return gutter(statusOK, c.ok(fmt.Sprintf("Port %d is free.", port)), g, c)
	case KillDeclined:
		return gutter(statusInfo, fmt.Sprintf("Port %d left untouched.", port), g, c)
	default:
		return gutter(statusWarn, fmt.Sprintf("Port %d: %s", port, textutil.Clean(detail)), g, c)
	}
}

// KillSummary is the closing box of a --kill run; outcomes without any port
// are left out.
func KillSummary(counts map[KillOutcome]int, opts Options) string {
	g, c := opts.glyphs(), opts.Colors
	labels := []struct {
		outcome KillOutcome
		label   string
	}{{KillStopped, "Stopped"}, {KillDeclined, "Left untouched"}, {KillFailed, "Failed"}}

	var rows []string
	for _, l := range labels {
		if n := counts[l.outcome]; n > 0 {
			rows = append(rows, c.dim(padRight(l.label, killLabelWidth))+c.number(strconv.Itoa(n)))
		}
	}
	return box("Result", rows, layoutWidth(opts.Width, opts.Wide), g, c)
}
