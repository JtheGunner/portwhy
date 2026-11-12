package render

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// minProcessWidth is the narrowest the process column is shortened to.
	minProcessWidth = 8
	// maxHiddenPorts bounds the ports named in the hidden-owner summary.
	maxHiddenPorts = 8

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

	portsW, addrW := 0, 0
	exposed := false
	for _, g := range ov.Groups {
		for _, r := range g.Rows {
			portsW = max(portsW, len(formatPorts(r.Ports)))
			addrW = max(addrW, len(r.Addr.String()))
			exposed = exposed || r.Exposed
		}
	}

	for _, g := range ov.Groups {
		fmt.Fprintln(b, c.bold(fit(textutil.Clean(g.Title), limit)))
		for _, r := range g.Rows {
			ports := padRight(formatPorts(r.Ports), portsW)
			prefix := rowIndent + ports + columnGap + string(r.Protocol) + columnGap + padRight(r.Addr.String(), addrW) + columnGap + " " + columnGap
			marker := " "
			if r.Exposed {
				marker = c.paint(ansiYellow, exposedTag)
			}
			proc := fit(processLabel(g, r), processWidth(limit, len(prefix)))
			fmt.Fprintf(b, "%s%s%s%s%s%s%s%s%s\n", rowIndent, c.bold(ports), columnGap, r.Protocol, columnGap, padRight(r.Addr.String(), addrW), columnGap, marker, columnGap+proc)
		}
	}

	if len(ov.Hidden) > 0 {
		fmt.Fprintln(b, c.dim(hiddenSummary(ov.Hidden)))
	}
	if exposed {
		fmt.Fprintln(b, c.dim(exposedTag+" reachable from the network (bound to all interfaces)"))
	}
	fmt.Fprintln(b, c.dim(fmt.Sprintf("Details: portwhy %d", lowestPort(ov))))
}

// processLabel names the owner of a row: "name (PID n)" below a service title,
// "PID n" below a plain process name, plus a count of further PIDs.
func processLabel(g group, r row) string {
	label := fmt.Sprintf("PID %d", r.PIDs[0])
	if g.Source != detect.SourceProcess {
		label = fmt.Sprintf("%s (PID %d)", textutil.Clean(r.Name), r.PIDs[0])
	}
	if n := len(r.PIDs) - 1; n > 0 {
		label += fmt.Sprintf(" +%d more", n)
	}
	return label
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
