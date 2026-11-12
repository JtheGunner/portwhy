package render

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// maxPortsWidth bounds the ports column on a terminal; --wide and output
	// without a terminal list every port.
	maxPortsWidth = 28
	// maxTitleWidth bounds the service column on a terminal.
	maxTitleWidth = 40
	// minTitleWidth is the narrowest the service column is shrunk to.
	minTitleWidth = 12
	// minOwnerWidth is the room the owner column keeps when the service column
	// is shrunk to fit a narrow terminal.
	minOwnerWidth = 16
	// minProcessWidth is the narrowest the owner column is shortened to.
	minProcessWidth = 8
	// minNameWidth is the fewest runes of a process name the overview keeps.
	minNameWidth = 4
	// protoWidth fits the header word "Proto".
	protoWidth = 5

	tableIndent = "  "
	columnGap   = "  "
	hiddenTitle = "Owner not visible"
	hiddenOwner = "run with sudo"
)

// tableRow is one line of the compact overview.
type tableRow struct {
	exposed bool
	title   string
	ports   string
	proto   string
	addr    string
	// owner renders the owner column within width runes (0 means unlimited).
	owner func(width int) string
}

// writeOverview writes the compact overview, the default list view: one
// aligned table row per service and protocol/address, then a summary and a
// hint line. It stays short on machines with many busy ports; the airy
// layout is a separate view.
func writeOverview(b *strings.Builder, reports []report.PortReport, opts Options) {
	ov := buildOverview(reports)
	if len(ov.Groups) == 0 && len(ov.Hidden) == 0 {
		return
	}
	c, g := opts.Colors, opts.glyphs()
	limit := opts.Width
	if opts.Wide {
		limit = 0
	}
	portsLimit := 0
	if limit > 0 {
		portsLimit = maxPortsWidth
	}

	rows := overviewRows(ov, portsLimit)
	titleW, portsW, addrW := columnWidths(rows, limit)

	// Everything before the owner column.
	prefixLen := len(tableIndent) + 1 + len(columnGap) + titleW + len(columnGap) + portsW + len(columnGap) + protoWidth + len(columnGap) + addrW + len(columnGap)
	ownerW := processWidth(limit, prefixLen)

	line := func(marker, title, ports, proto, addr, owner string) string {
		return tableIndent + marker + columnGap + title + columnGap + ports + columnGap + proto + columnGap + addr + columnGap + owner
	}
	titleHeader := "Service / process"
	if titleW < utf8.RuneCountInString(titleHeader) {
		titleHeader = "Service"
	}
	lines := []string{line(" ",
		c.dim(padRight(titleHeader, titleW)), c.dim(padRight("Ports", portsW)),
		c.dim(padRight("Proto", protoWidth)), c.dim(padRight("Bind", addrW)), c.dim("Owner"))}
	// The separators span the aligned table; a port list that overflows its
	// column does not stretch them.
	ownerMax := utf8.RuneCountInString("Owner")
	for _, r := range rows {
		marker := " "
		if r.exposed {
			marker = c.warn(g.Warn)
		}
		owner := r.owner(ownerW)
		ownerMax = max(ownerMax, utf8.RuneCountInString(owner))
		lines = append(lines, line(marker,
			c.bold(padRight(fitWith(r.title, titleW, g.Ellipsis), titleW)), c.port(padRight(r.ports, portsW)),
			c.dim(padRight(r.proto, protoWidth)), padRight(r.addr, addrW), owner))
	}

	sepW := prefixLen + ownerMax
	if limit > 0 {
		sepW = min(sepW, limit)
	}
	sep := c.dim(strings.Repeat(g.H, sepW))
	fmt.Fprintln(b, lines[0])
	fmt.Fprintln(b, sep)
	for _, l := range lines[1:] {
		fmt.Fprintln(b, l)
	}
	fmt.Fprintln(b, sep)
	room := 0
	if limit > 0 {
		room = max(limit-len(tableIndent), minTitleWidth)
	}
	for _, l := range joinWrapped(summaryParts(ov, c, g), " "+g.Dot+" ", room) {
		fmt.Fprintln(b, tableIndent+l)
	}
	writeHint(b, ov, opts, limit, true)
}

// overviewRows turns the groups and the hidden owners into table rows.
func overviewRows(ov overview, portsLimit int) []tableRow {
	var rows []tableRow
	for _, grp := range ov.Groups {
		title := textutil.Clean(grp.Title)
		for _, r := range grp.Rows {
			rows = append(rows, tableRow{
				exposed: r.Exposed,
				title:   title,
				ports:   formatPortsWithin(r.Ports, portsLimit),
				proto:   string(r.Protocol),
				addr:    r.Addr.String(),
				owner:   func(width int) string { return processLabel(grp, r, width) },
			})
		}
	}
	type hiddenKey struct {
		proto socket.Protocol
		addr  netip.Addr
	}
	hidden := map[hiddenKey][]uint16{}
	for _, f := range ov.Hidden {
		k := hiddenKey{f.Socket.Protocol, f.Socket.Addr}
		if !slices.Contains(hidden[k], f.Socket.Port) {
			hidden[k] = append(hidden[k], f.Socket.Port)
		}
	}
	keys := make([]hiddenKey, 0, len(hidden))
	for k, ports := range hidden {
		slices.Sort(ports)
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b hiddenKey) int {
		return cmp.Or(cmp.Compare(hidden[a][0], hidden[b][0]), cmp.Compare(a.proto, b.proto), a.addr.Compare(b.addr))
	})
	for _, k := range keys {
		rows = append(rows, tableRow{
			exposed: k.addr.IsUnspecified(),
			title:   hiddenTitle,
			ports:   formatPortsWithin(hidden[k], portsLimit),
			proto:   string(k.proto),
			addr:    k.addr.String(),
			owner:   func(int) string { return hiddenOwner },
		})
	}
	return rows
}

// columnWidths sizes the service, ports and bind columns. On a terminal
// (limit > 0) the service column is capped and shrunk so the owner column keeps
// some room.
func columnWidths(rows []tableRow, limit int) (titleW, portsW, addrW int) {
	titleW, addrW = utf8.RuneCountInString("Service / process"), utf8.RuneCountInString("Bind")
	portsW = utf8.RuneCountInString("Ports")
	for _, r := range rows {
		titleW = max(titleW, utf8.RuneCountInString(r.title))
		portsW = max(portsW, utf8.RuneCountInString(r.ports))
		addrW = max(addrW, utf8.RuneCountInString(r.addr))
	}
	// A very long port list may overflow its own row but never widens the
	// column for all the others.
	portsW = min(portsW, maxPortsWidth)
	if limit > 0 {
		titleW = min(titleW, maxTitleWidth)
		others := len(tableIndent) + 1 + 4*len(columnGap) + portsW + protoWidth + addrW + len(columnGap)
		titleW = max(min(titleW, limit-others-minOwnerWidth), minTitleWidth)
	}
	return titleW, portsW, addrW
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

// processWidth is the room left for the owner column; 0 means unlimited.
func processWidth(limit, prefixLen int) int {
	if limit <= 0 {
		return 0
	}
	return max(limit-prefixLen, minProcessWidth)
}

// portKey identifies a busy port for the summary: the same port on several
// addresses counts once.
type portKey struct {
	port  uint16
	proto socket.Protocol
}

// stats counts what an overview lists.
type stats struct {
	ports, services, exposed, hidden int
}

// overviewStats counts the busy ports, services, ports reachable from the
// network and ports with a hidden owner of ov.
func overviewStats(ov overview) stats {
	ports, exposed, hidden := map[portKey]bool{}, map[portKey]bool{}, map[portKey]bool{}
	for _, grp := range ov.Groups {
		for _, r := range grp.Rows {
			for _, p := range r.Ports {
				k := portKey{p, r.Protocol}
				ports[k] = true
				if r.Exposed {
					exposed[k] = true
				}
			}
		}
	}
	for _, f := range ov.Hidden {
		k := portKey{f.Socket.Port, f.Socket.Protocol}
		ports[k], hidden[k] = true, true
		if f.Socket.Addr.IsUnspecified() {
			exposed[k] = true
		}
	}
	return stats{len(ports), len(ov.Groups), len(exposed), len(hidden)}
}

// summaryParts words the stats for the one-line summary of the compact table.
func summaryParts(ov overview, c Colors, g Glyphs) []string {
	st := overviewStats(ov)
	parts := []string{
		c.number(fmt.Sprint(st.ports)) + plural(st.ports, " busy port", " busy ports"),
		c.number(fmt.Sprint(st.services)) + plural(st.services, " service", " services"),
	}
	if st.exposed > 0 {
		parts = append(parts, c.warn(g.Warn)+" "+c.number(fmt.Sprint(st.exposed))+" reachable from the network")
	}
	if st.hidden > 0 {
		parts = append(parts, c.number(fmt.Sprint(st.hidden))+plural(st.hidden, " port with hidden owner", " ports with hidden owner"))
	}
	return parts
}

// plural picks the singular for exactly one, else the plural.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// writeHint writes the closing info line: where to look closer, how to get
// the airy view (airyHint) and, when sources are known, how to filter by them.
func writeHint(b *strings.Builder, ov overview, opts Options, limit int, airyHint bool) {
	c, g := opts.Colors, opts.glyphs()
	text := fmt.Sprintf("Details: portwhy %d", lowestPort(ov))
	if airyHint {
		text += "    Airy view: --expanded"
	}
	if len(opts.Sources) > 0 {
		names := make([]string, len(opts.Sources))
		for i, s := range opts.Sources {
			names[i] = textutil.Clean(string(s.Source))
		}
		text += "    Filter: --source " + strings.Join(names, "|")
	}
	room := 0
	if limit > 0 {
		room = max(limit-5, minTitleWidth)
	}
	for i, l := range wrapWords(text, room) {
		if i == 0 {
			fmt.Fprintln(b, gutter(statusInfo, c.dim(l), g, c))
			continue
		}
		fmt.Fprintln(b, "     "+c.dim(l))
	}
}

// joinWrapped joins parts with sep into lines of at most width visible
// runes (0 means unlimited). A part never splits across lines.
func joinWrapped(parts []string, sep string, width int) []string {
	var lines []string
	line := ""
	for _, part := range parts {
		switch {
		case line == "":
			line = part
		case width <= 0 || visibleWidth(line)+visibleWidth(sep)+visibleWidth(part) <= width:
			line += sep + part
		default:
			lines = append(lines, line)
			line = part
		}
	}
	return append(lines, line)
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
