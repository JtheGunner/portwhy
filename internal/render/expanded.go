package render

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// summaryLabelWidth fits the longest label of the summary box.
	summaryLabelWidth = 14
	// summaryValueWidth keeps the descriptions of the summary box in a column.
	summaryValueWidth = 6
)

// socketRow is one port of the expanded overview.
type socketRow struct {
	port    uint16
	proto   socket.Protocol
	addr    netip.Addr
	exposed bool
	// owner renders the owner column within width runes (0 means unlimited).
	owner func(width int) string
}

// section is one titled rule and the rows below it.
type section struct {
	title string
	rows  []socketRow
}

// writeExpanded writes the airy overview: a titled rule per service, one row
// per port, a summary box and a hint line.
func writeExpanded(b *strings.Builder, reports []report.PortReport, opts Options) {
	ov := buildOverview(reports)
	if len(ov.Groups) == 0 && len(ov.Hidden) == 0 {
		return
	}
	c, g := opts.Colors, opts.glyphs()
	limit := opts.Width
	if opts.Wide {
		limit = 0
	}
	width := layoutWidth(opts.Width, opts.Wide)

	sections := expandedSections(ov)
	portW, addrW := 0, 0
	for _, s := range sections {
		for _, r := range s.rows {
			portW = max(portW, len(strconv.Itoa(int(r.port))))
			addrW = max(addrW, len(r.addr.String()))
		}
	}
	prefixLen := len(tableIndent) + 1 + len(columnGap) + portW + len(columnGap) + 3 + len(columnGap) + addrW + len(columnGap)
	ownerW := processWidth(limit, prefixLen)

	for _, s := range sections {
		suffix := fmt.Sprintf(" %s %d%s", g.Dot, len(s.rows), plural(len(s.rows), " port", " ports"))
		title := fitWith(s.title, max(width-titleMargin-utf8.RuneCountInString(suffix), 1), g.Ellipsis) + suffix
		fmt.Fprintln(b, rule(title, width, g, c))
		fmt.Fprintln(b)
		for _, r := range s.rows {
			marker := " "
			if r.exposed {
				marker = c.warn(g.Warn)
			}
			fmt.Fprintln(b, tableIndent+marker+columnGap+c.port(fmt.Sprintf("%*d", portW, r.port))+columnGap+
				c.dim(padRight(string(r.proto), 3))+columnGap+padRight(r.addr.String(), addrW)+columnGap+r.owner(ownerW))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b, box("Summary", summaryRows(ov, c, g), width, g, c))
	writeHint(b, ov, opts, limit, false)
}

// expandedSections turns the groups, then the hidden owners, into sections
// with one row per port. Rows are ordered by port, protocol and address.
func expandedSections(ov overview) []section {
	var sections []section
	for _, grp := range ov.Groups {
		s := section{title: textutil.Clean(grp.Title)}
		for _, r := range grp.Rows {
			for _, p := range r.Ports {
				s.rows = append(s.rows, socketRow{
					port: p, proto: r.Protocol, addr: r.Addr, exposed: r.Exposed,
					owner: func(width int) string { return processLabel(grp, r, width) },
				})
			}
		}
		sections = append(sections, sortedSection(s))
	}
	if len(ov.Hidden) > 0 {
		s := section{title: hiddenTitle}
		for _, f := range ov.Hidden {
			s.rows = append(s.rows, socketRow{
				port: f.Socket.Port, proto: f.Socket.Protocol, addr: f.Socket.Addr, exposed: f.Socket.Addr.IsUnspecified(),
				owner: func(int) string { return hiddenOwner },
			})
		}
		sections = append(sections, sortedSection(s))
	}
	return sections
}

func sortedSection(s section) section {
	slices.SortStableFunc(s.rows, func(a, b socketRow) int {
		return cmp.Or(cmp.Compare(a.port, b.port), cmp.Compare(a.proto, b.proto), a.addr.Compare(b.addr))
	})
	return s
}

// summaryRows are the rows of the summary box; rows with nothing to report
// are left out.
func summaryRows(ov overview, c Colors, g Glyphs) []string {
	st := overviewStats(ov)
	row := func(label string, count int, note string) string {
		line := c.dim(padRight(label, summaryLabelWidth)) + padVisible(c.number(strconv.Itoa(count)), summaryValueWidth)
		return strings.TrimRight(line+note, " ")
	}
	rows := []string{row("Busy ports", st.ports, "")}
	if st.services > 0 {
		rows = append(rows, row("Services", st.services, sourceBreakdown(ov, g)))
	}
	if st.exposed > 0 {
		rows = append(rows, row("Reachable", st.exposed, c.warn(g.Warn)+" bound to all interfaces"))
	}
	if st.hidden > 0 {
		rows = append(rows, row("Hidden owner", st.hidden, "run with sudo"))
	}
	return rows
}

// sourceBreakdown counts the services per source, for example
// "docker 1 · launchd 1 · process 3".
func sourceBreakdown(ov overview, g Glyphs) string {
	counts := map[detect.Source]int{}
	for _, grp := range ov.Groups {
		counts[grp.Source]++
	}
	sources := make([]detect.Source, 0, len(counts))
	for s := range counts {
		sources = append(sources, s)
	}
	slices.Sort(sources)
	parts := make([]string, len(sources))
	for i, s := range sources {
		parts[i] = fmt.Sprintf("%s %d", textutil.Clean(string(s)), counts[s])
	}
	return strings.Join(parts, " "+g.Dot+" ")
}
