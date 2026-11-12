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
	// detailIndent is as wide as the gutter ("  ⚠  "), so label rows line up
	// with the text next to the status symbols.
	detailIndent = "     "
	// maxCommandLines bounds how many lines a long command may take on a
	// terminal before it ends in an ellipsis.
	maxCommandLines = 3
	// labelGapWidth separates a label from its value.
	labelGapWidth = 2

	labelProcess = "Process"
	labelStop    = "Stop"
	labelCommand = "Command"
	labelCwd     = "Cwd"
)

// labelOrder ranks the labels that have a fixed place in a block. Everything
// else keeps the order the detector gave it and sits between "Started by" and
// the command.
var labelOrder = map[string]int{labelProcess: 0, "User": 1, "Uptime": 2, "Started by": 3, labelCommand: 5, labelCwd: 6}

// displayLabels renames detector keys for the screen.
var displayLabels = map[string]string{labelCwd: "Directory"}

// fact is one "Label  value" line of a block before it is wrapped.
type fact struct {
	label string
	value string
}

// note is a hint or an advice line with its status symbol.
type note struct {
	kind statusKind
	text string
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

// textLimit is the width long values are shortened or wrapped to; 0 means
// unlimited, which is the case without a terminal and with --wide.
func textLimit(opts Options) int {
	if opts.Width <= 0 || opts.Wide {
		return 0
	}
	return layoutWidth(opts.Width, false)
}

// writeDetail writes one block per service or process holding the port, or a
// short block when the port is free.
func writeDetail(b *strings.Builder, r report.PortReport, opts Options) {
	g, c := opts.glyphs(), opts.Colors
	width := layoutWidth(opts.Width, opts.Wide)
	if len(r.Findings) == 0 {
		fmt.Fprintln(b, rule(fmt.Sprintf(":%d", r.Port), width, g, c))
		fmt.Fprintln(b)
		text := "free"
		if r.TimeWaitOnly {
			text = "free (only lingering time_wait connections)"
		}
		fmt.Fprintln(b, gutter(statusOK, c.ok(text), g, c))
		writeNotes(b, adviceNotes(r.Advice), textLimit(opts), g, c)
		fmt.Fprintln(b)
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
	g, c := opts.glyphs(), opts.Colors
	limit := textLimit(opts)
	findings = slices.Clone(findings)
	slices.SortStableFunc(findings, func(x, y report.Finding) int { return cmp.Compare(pidOf(x), pidOf(y)) })
	primary := findings[0]

	title := fmt.Sprintf(":%d %s %s", primary.Socket.Port, g.Dot, headline(primary))
	fmt.Fprintln(b, rule(title, layoutWidth(opts.Width, opts.Wide), g, c))
	fmt.Fprintln(b)

	scope := fmt.Sprintf("%s %s", textutil.Clean(string(primary.Socket.Protocol)), primary.Socket.Addr)
	groups := [][]string{{gutter(statusInfo, scope, g, c)}}

	facts := blockFacts(findings)
	stops := stopCommands(findings)
	labelWidth := utf8.RuneCountInString(labelStop)
	for _, f := range facts {
		labelWidth = max(labelWidth, utf8.RuneCountInString(f.label))
	}
	valueWidth := 0
	if limit > 0 {
		valueWidth = max(limit-len(detailIndent)-labelWidth-labelGapWidth, minProcessWidth)
	}

	var factRows []labelRow
	for _, f := range facts {
		for i, line := range factLines(f, valueWidth, g) {
			label := f.label
			if i > 0 {
				label = ""
			}
			factRows = append(factRows, labelRow{Label: label, Value: line})
		}
	}
	// Facts and stops share one label column so their values line up.
	var stopRows []labelRow
	for i, s := range stops {
		label := ""
		if i == 0 {
			label = labelStop
		}
		stopRows = append(stopRows, labelRow{Label: label, Value: c.bold(s)})
	}
	lines := labelRows(append(slices.Clone(factRows), stopRows...), len(detailIndent), c)
	if len(factRows) > 0 {
		groups = append(groups, lines[:len(factRows)])
	}
	if len(stopRows) > 0 {
		stopLines := lines[len(factRows):]
		stopLines[0] = gutter(statusGo, strings.TrimPrefix(stopLines[0], detailIndent), g, c)
		groups = append(groups, stopLines)
	}
	if notes := blockNotes(findings); len(notes) > 0 {
		var sb strings.Builder
		writeNotes(&sb, notes, limit, g, c)
		groups = append(groups, strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n"))
	}

	for _, group := range groups {
		for _, line := range group {
			fmt.Fprintln(b, line)
		}
		fmt.Fprintln(b)
	}
}

// factLines is the text of a fact split for a value column of width runes
// (0 means unlimited). A command wraps over a few lines; everything else is
// shortened to one.
func factLines(f fact, width int, g Glyphs) []string {
	if width <= 0 {
		return []string{f.value}
	}
	if f.label != displayLabel(labelCommand) {
		return []string{fitWith(f.value, width, g.Ellipsis)}
	}
	lines := wrapWords(f.value, width)
	if len(lines) > maxCommandLines {
		lines = append(lines[:maxCommandLines-1], strings.Join(lines[maxCommandLines-1:], " "))
	}
	for i, line := range lines {
		lines[i] = fitWith(line, width, g.Ellipsis)
	}
	return lines
}

func displayLabel(key string) string {
	if label, ok := displayLabels[key]; ok {
		return label
	}
	return key
}

// writeNotes writes hints and advice as status lines, wrapped to limit with a
// hanging indent under the text.
func writeNotes(b *strings.Builder, notes []note, limit int, g Glyphs, c Colors) {
	room := 0
	if limit > 0 {
		room = max(limit-len(detailIndent), minProcessWidth)
	}
	for _, n := range notes {
		for i, line := range wrapWords(n.text, room) {
			if i == 0 {
				fmt.Fprintln(b, gutter(n.kind, line, g, c))
				continue
			}
			fmt.Fprintln(b, detailIndent+line)
		}
	}
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

// blockFacts builds the label rows of a block from its findings (lowest PID
// first): the owner, then the detector details in a fixed order.
func blockFacts(findings []report.Finding) []fact {
	primary := findings[0]
	var facts []fact

	if p := primary.Process; p != nil && (primary.Explanation.Source != detect.SourceProcess || len(findings) > 1) {
		value := fmt.Sprintf("%s (PID %d)", textutil.Clean(p.Name), p.PID)
		if n := len(findings) - 1; n > 0 {
			value += fmt.Sprintf(" +%d more", n)
		}
		facts = append(facts, fact{labelProcess, value})
	}
	for _, d := range primary.Explanation.Details {
		key, value := textutil.Clean(d.Key), textutil.Clean(d.Value)
		// The block states the owner itself.
		if key == labelProcess {
			continue
		}
		facts = append(facts, fact{key, value})
	}
	slices.SortStableFunc(facts, func(a, b fact) int { return cmp.Compare(rankOf(a.label), rankOf(b.label)) })
	for i := range facts {
		facts[i].label = displayLabel(facts[i].label)
	}
	return facts
}

// rankOf orders a label: known labels have a fixed place, others go between
// "Started by" and the command in detector order.
func rankOf(label string) int {
	if r, ok := labelOrder[label]; ok {
		return r
	}
	return 4
}

// stopCommands lists each distinct way to stop the block once.
func stopCommands(findings []report.Finding) []string {
	var stops []string
	for _, f := range findings {
		if t := f.Explanation.Terminator; t != nil {
			if s := textutil.Clean(t.Describe()); !slices.Contains(stops, s) {
				stops = append(stops, s)
			}
		}
	}
	return stops
}

// blockNotes lists the distinct hints of the findings as warnings, followed by
// the distinct advice under its own level.
func blockNotes(findings []report.Finding) []note {
	var notes []note
	for _, f := range findings {
		for _, h := range f.Explanation.Hints {
			n := note{statusWarn, textutil.Clean(h)}
			if !slices.Contains(notes, n) {
				notes = append(notes, n)
			}
		}
	}
	var advice []advise.Advice
	for _, f := range findings {
		for _, a := range f.Advice {
			if !slices.Contains(advice, a) {
				advice = append(advice, a)
			}
		}
	}
	for _, n := range adviceNotes(advice) {
		if !slices.Contains(notes, n) {
			notes = append(notes, n)
		}
	}
	return notes
}

// adviceNotes maps advice to status lines.
func adviceNotes(advice []advise.Advice) []note {
	notes := make([]note, 0, len(advice))
	for _, a := range advice {
		kind := statusInfo
		if a.Level == advise.Warn {
			kind = statusWarn
		}
		notes = append(notes, note{kind, textutil.Clean(a.Text)})
	}
	return notes
}
