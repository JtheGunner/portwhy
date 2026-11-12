package render

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// minProcessWidth is the narrowest the process column is shortened to.
	minProcessWidth = 8
	// minNameWidth is the fewest runes of a process name the overview keeps.
	minNameWidth = 4
	// maxHiddenPorts bounds the ports named in the hidden-owner summary.
	maxHiddenPorts = 8
	// maxPortsWidth bounds the ports column on a terminal; --wide and output
	// without a terminal list every port.
	maxPortsWidth = 28

	rowIndent  = "  "
	columnGap  = "  "
	exposedTag = "!"
)

// writeOverview writes the grouped list view: one titled group per service,
// one row per set of ports that share protocol, address and owning PIDs.
func writeOverview(b *strings.Builder, reports []report.PortReport, opts Options) {
	ov := buildOverview(reports)
	if len(ov.Groups) == 0 && len(ov.Hidden) == 0 {
		return
	}
	c := opts.Colors
	limit := opts.Width
	if opts.Wide {
		limit = 0
	}

	portsLimit := 0
	if limit > 0 {
		portsLimit = maxPortsWidth
	}
	portsW, addrW := 0, 0
	exposed := false
	for _, g := range ov.Groups {
		for _, r := range g.Rows {
			portsW = max(portsW, utf8.RuneCountInString(formatPortsWithin(r.Ports, portsLimit)))
			addrW = max(addrW, len(r.Addr.String()))
			exposed = exposed || r.Exposed
		}
	}

	for _, g := range ov.Groups {
		fmt.Fprintln(b, c.bold(fit(textutil.Clean(g.Title), limit)))
		for _, r := range g.Rows {
			ports := padRight(formatPortsWithin(r.Ports, portsLimit), portsW)
			prefix := rowIndent + ports + columnGap + string(r.Protocol) + columnGap + padRight(r.Addr.String(), addrW) + columnGap + " " + columnGap
			marker := " "
			if r.Exposed {
				marker = c.paint(ansiYellow, exposedTag)
			}
			proc := processLabel(g, r, processWidth(limit, len(prefix)))
			fmt.Fprintf(b, "%s%s%s%s%s%s%s%s%s\n", rowIndent, c.bold(ports), columnGap, r.Protocol, columnGap, padRight(r.Addr.String(), addrW), columnGap, marker, columnGap+proc)
		}
	}

	footer := func(text string) {
		for _, line := range wrapWords(text, limit) {
			fmt.Fprintln(b, c.dim(line))
		}
	}
	if len(ov.Hidden) > 0 {
		footer(hiddenSummary(ov.Hidden))
	}
	if exposed {
		footer(exposedTag + " reachable from the network (bound to all interfaces)")
	}
	footer(fmt.Sprintf("Details: portwhy %d", lowestPort(ov)))
}

// processLabel names the owner of a row: "name (PID n)" below a service title,
// "PID n" below a plain process name, plus a count of further PIDs. When the
// label is wider than width (0 means unlimited) the name is shortened and the
// PID is kept.
func processLabel(g group, r row, width int) string {
	more := ""
	if n := len(r.PIDs) - 1; n > 0 {
		more = fmt.Sprintf(" +%d more", n)
	}
	if g.Source == detect.SourceProcess {
		pid := fmt.Sprintf("PID %d", r.PIDs[0])
		if width > 0 {
			width = max(width, utf8.RuneCountInString(pid))
		}
		return fit(pid+more, width)
	}
	name := textutil.Clean(r.Name)
	suffix := fmt.Sprintf(" (PID %d)", r.PIDs[0]) + more
	if width <= 0 {
		return name + suffix
	}
	// The PID is never cut; the name keeps at least minNameWidth runes.
	room := max(width-utf8.RuneCountInString(suffix), minNameWidth)
	return fit(name, room) + suffix
}

// processWidth is the room left for the process column; 0 means unlimited.
func processWidth(limit, prefixLen int) int {
	if limit <= 0 {
		return 0
	}
	return max(limit-prefixLen, minProcessWidth)
}

func hiddenSummary(hidden []report.Finding) string {
	var ports []uint16
	for _, f := range hidden {
		if !slices.Contains(ports, f.Socket.Port) {
			ports = append(ports, f.Socket.Port)
		}
	}
	list := formatPorts(ports)
	if len(ports) > maxHiddenPorts {
		list = formatPorts(ports[:maxHiddenPorts]) + ", …"
	}
	noun := "sockets"
	if len(hidden) == 1 {
		noun = "socket"
	}
	return fmt.Sprintf("+ %d %s with hidden owner (%s). Run with sudo.", len(hidden), noun, list)
}

// lowestPort is the port the closing hint points at: the lowest port of a
// visible owner, since a hidden owner has no details to show. Only an overview
// of hidden owners alone falls back to the lowest hidden port.
func lowestPort(ov overview) uint16 {
	var ports []uint16
	for _, g := range ov.Groups {
		for _, r := range g.Rows {
			ports = append(ports, r.Ports[0])
		}
	}
	if len(ports) == 0 {
		for _, f := range ov.Hidden {
			ports = append(ports, f.Socket.Port)
		}
	}
	return slices.Min(ports)
}
