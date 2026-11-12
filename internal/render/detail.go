package render

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	indent = "       " // 7 spaces: aligns details under the text after ":<port>  ".

	// maxFactLength is the longest detail value that is packed into the
	// shared "Facts" row; longer values get a row of their own.
	maxFactLength = 24

	labelGap     = "  "
	labelFacts   = "Facts"
	labelProcess = "Process"
	labelStop    = "Stop"
	labelNote    = "Note"
)

// ownRowKeys are detail keys that always get their own row, however short.
var ownRowKeys = map[string]bool{"Command": true, "Started by": true}

// detailRow is one aligned "label  value" line below a block header.
type detailRow struct {
	label string
	// paintedLabel replaces the dimmed label when set (advice markers).
	paintedLabel string
	value        string
	// shorten marks values that are cut at the terminal width.
	shorten bool
	// wrap marks values that continue on further lines at the terminal width.
	wrap bool
	// tail follows value, on the same line when it fits and on wrapped
	// continuation lines otherwise. value itself is never broken.
	tail string
}

// blockKey identifies the findings that merge into one block: the same
// service (or process name) on the same socket address.
type blockKey struct {
	port   uint16
	proto  socket.Protocol
	addr   netip.Addr
	source detect.Source
	title  string
}

// writeDetail writes one block per service or process holding the port, or a
// single line when the port is free.
func writeDetail(b *strings.Builder, r report.PortReport, opts Options) {
	if len(r.Findings) == 0 {
		if r.TimeWaitOnly {
			fmt.Fprintf(b, ":%d  free (only lingering time_wait connections)\n", r.Port)
			writeAdvice(b, r.Advice, opts)
			return
		}
		fmt.Fprintf(b, ":%d  free\n", r.Port)
		return
	}

	var order []blockKey
	blocks := map[blockKey][]report.Finding{}
	for _, f := range r.Findings {
		k := blockKey{f.Socket.Port, f.Socket.Protocol, f.Socket.Addr, f.Explanation.Source, f.Explanation.Title}
		if _, ok := blocks[k]; !ok {
			order = append(order, k)
		}
		blocks[k] = append(blocks[k], f)
	}
	for _, k := range order {
		writeBlock(b, blocks[k], opts)
	}
}

func writeBlock(b *strings.Builder, findings []report.Finding, opts Options) {
	c := opts.Colors
	findings = slices.Clone(findings)
	slices.SortStableFunc(findings, func(x, y report.Finding) int { return cmp.Compare(pidOf(x), pidOf(y)) })
	primary := findings[0]

	port := fmt.Sprintf(":%d", primary.Socket.Port)
	where := fmt.Sprintf("%s %s", textutil.Clean(strings.ToUpper(string(primary.Socket.Protocol))), primary.Socket.Addr)
	title := headline(primary)
	if opts.Width > 0 && !opts.Wide {
		prefix := utf8.RuneCountInString(port) + len(labelGap) + utf8.RuneCountInString(where) + len(labelGap)
		title = fit(title, max(opts.Width-prefix, minProcessWidth))
	}
	fmt.Fprintf(b, "%s%s%s%s%s\n", c.bold(port), labelGap, where, labelGap, c.bold(title))

	rows := blockRows(findings, c)
	labelWidth := 0
	for _, r := range rows {
		labelWidth = max(labelWidth, utf8.RuneCountInString(r.label))
	}
	valueWidth := 0
	if opts.Width > 0 && !opts.Wide {
		valueWidth = max(opts.Width-len(indent)-labelWidth-len(labelGap), minProcessWidth)
	}
	for _, r := range rows {
		label := r.paintedLabel
		if label == "" && r.label != "" {
			label = c.dim(r.label)
		}
		label += strings.Repeat(" ", labelWidth-utf8.RuneCountInString(r.label))
		lines := rowLines(r, valueWidth)
		fmt.Fprintf(b, "%s%s%s%s\n", indent, label, labelGap, lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(b, "%s%s%s%s\n", indent, strings.Repeat(" ", labelWidth), labelGap, line)
		}
	}
}

// rowLines is the text of a row split for a value column of width runes
// (0 means unlimited).
func rowLines(r detailRow, width int) []string {
	value := r.value
	if r.shorten {
		value = fit(value, width)
	}
	if r.tail != "" {
		joined := value + "   " + r.tail
		if width <= 0 || utf8.RuneCountInString(joined) <= width {
			return []string{joined}
		}
		return append([]string{value}, wrapWords(r.tail, width)...)
	}
	if r.wrap {
		return wrapWords(value, width)
	}
	return []string{value}
}

// pidOf is the PID that identifies a finding; 0 when the owner is not visible.
func pidOf(f report.Finding) int {
	if f.Process != nil {
		return f.Process.PID
	}
	return f.Socket.PID
}

// headline names what holds the socket: the service when there is one,
// otherwise the process. A name is never printed twice.
func headline(f report.Finding) string {
	title := textutil.Clean(f.Explanation.Title)
	switch {
	case f.Explanation.Source != detect.SourceProcess && title != "":
		return title
	case f.Process != nil:
		return fmt.Sprintf("%s (PID %d)", textutil.Clean(f.Process.Name), f.Process.PID)
	case f.Socket.PID != 0:
		return fmt.Sprintf("%s (PID %d)", title, f.Socket.PID)
	default:
		return title
	}
}

// blockRows builds the body of a block from its findings (lowest PID first).
func blockRows(findings []report.Finding, c Colors) []detailRow {
	primary := findings[0]
	var rows []detailRow

	if p := primary.Process; p != nil && (primary.Explanation.Source != detect.SourceProcess || len(findings) > 1) {
		value := fmt.Sprintf("%s (PID %d)", textutil.Clean(p.Name), p.PID)
		if n := len(findings) - 1; n > 0 {
			value += fmt.Sprintf(" +%d more", n)
		}
		rows = append(rows, detailRow{label: labelProcess, value: value, shorten: true})
	}

	rows = append(rows, detailRows(primary.Explanation.Details)...)
	rows = append(rows, stopRows(findings)...)
	rows = append(rows, adviceRows(findings, c)...)
	return rows
}

// detailRows turns detector details into rows: short values share one "Facts"
// row at the position of the first of them, the rest get a row each. A
// "Process" detail is skipped because the block states the owner itself.
func detailRows(details []detect.Detail) []detailRow {
	var rows []detailRow
	facts := -1
	for _, d := range details {
		key, value := textutil.Clean(d.Key), textutil.Clean(d.Value)
		if key == labelProcess {
			continue
		}
		if ownRowKeys[key] || utf8.RuneCountInString(value) > maxFactLength {
			rows = append(rows, detailRow{label: key, value: value, shorten: true})
			continue
		}
		fact := strings.TrimSpace(key + " " + value)
		if facts < 0 {
			facts = len(rows)
			rows = append(rows, detailRow{label: labelFacts, value: fact, wrap: true})
			continue
		}
		rows[facts].value += " · " + fact
	}
	return rows
}

// stopRows lists each distinct way to stop the block once, followed by the
// hints; without a way to stop, the hints stand alone as a note.
func stopRows(findings []report.Finding) []detailRow {
	var stops, hints []string
	for _, f := range findings {
		if t := f.Explanation.Terminator; t != nil {
			if s := textutil.Clean(t.Describe()); !slices.Contains(stops, s) {
				stops = append(stops, s)
			}
		}
		for _, h := range f.Explanation.Hints {
			if h = textutil.Clean(h); !slices.Contains(hints, h) {
				hints = append(hints, h)
			}
		}
	}
	hintText := ""
	if len(hints) > 0 {
		hintText = "(" + strings.Join(hints, "; ") + ")"
	}

	var rows []detailRow
	for i, s := range stops {
		row := detailRow{value: s}
		if i == 0 {
			row.label = labelStop
			row.tail = hintText
		}
		rows = append(rows, row)
	}
	if len(stops) == 0 && hintText != "" {
		rows = append(rows, detailRow{label: labelNote, value: hintText, wrap: true})
	}
	return rows
}

// adviceRows lists the distinct advice of the findings under a level marker.
func adviceRows(findings []report.Finding, c Colors) []detailRow {
	var advice []advise.Advice
	for _, f := range findings {
		for _, a := range f.Advice {
			if !slices.Contains(advice, a) {
				advice = append(advice, a)
			}
		}
	}
	rows := make([]detailRow, 0, len(advice))
	for _, a := range advice {
		marker, code := "i", ansiCyan
		if a.Level == advise.Warn {
			marker, code = "!", ansiYellow
		}
		rows = append(rows, detailRow{label: marker, paintedLabel: c.paint(code, marker), value: textutil.Clean(a.Text), wrap: true})
	}
	return rows
}

// writeAdvice writes the advice of a port without a block of its own, wrapped
// to the terminal width with a hanging indent under the marker.
func writeAdvice(b *strings.Builder, advice []advise.Advice, opts Options) {
	c := opts.Colors
	width := 0
	if opts.Width > 0 && !opts.Wide {
		width = max(opts.Width-len(indent)-2, minProcessWidth)
	}
	for _, a := range advice {
		marker := c.paint(ansiCyan, "i")
		if a.Level == advise.Warn {
			marker = c.paint(ansiYellow, "!")
		}
		for i, line := range wrapWords(textutil.Clean(a.Text), width) {
			lead := marker
			if i > 0 {
				lead = " "
			}
			fmt.Fprintf(b, "%s%s %s\n", indent, lead, line)
		}
	}
}
