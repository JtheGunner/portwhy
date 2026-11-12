package render

import (
	"strings"
	"unicode/utf8"
)

// titleMargin is the space a rule or box title leaves for line characters and
// its own padding: two corner or edge cells on each side plus a space on each
// side of the title.
const titleMargin = 8

// rule draws a horizontal line of width columns with title centred in it. A
// title that does not fit is shortened. An empty title gives a plain line.
func rule(title string, width int, g Glyphs, c Colors) string {
	if title == "" {
		return c.accent(strings.Repeat(g.H, width))
	}
	padded := " " + fitWith(title, max(width-titleMargin, 1), g.Ellipsis) + " "
	left, right := splitPadding(width - utf8.RuneCountInString(padded))
	return c.accent(strings.Repeat(g.H, left)) + c.bold(padded) + c.accent(strings.Repeat(g.H, right))
}

// box draws a rounded box of width columns with title centred in the top
// border. Rows are padded to the inner width; a row wider than the box is not
// shortened, so callers cut their cells before colouring them.
func box(title string, rows []string, width int, g Glyphs, c Colors) string {
	inner := max(width-2, 0)
	top := c.boxBorder(g.TL + strings.Repeat(g.H, inner) + g.TR)
	if title != "" {
		padded := " " + fitWith(title, max(width-titleMargin, 1), g.Ellipsis) + " "
		left, right := splitPadding(inner - utf8.RuneCountInString(padded))
		top = c.boxBorder(g.TL+strings.Repeat(g.H, left)) + c.bold(padded) + c.boxBorder(strings.Repeat(g.H, right)+g.TR)
	}
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, top)
	for _, row := range rows {
		lines = append(lines, c.boxBorder(g.V)+" "+padVisible(row, max(inner-2, 0))+" "+c.boxBorder(g.V))
	}
	lines = append(lines, c.boxBorder(g.BL+strings.Repeat(g.H, inner)+g.BR))
	return strings.Join(lines, "\n")
}

// splitPadding divides total padding into a left and a right part; the odd
// cell goes to the right. Negative totals give no padding.
func splitPadding(total int) (left, right int) {
	if total <= 0 {
		return 0, 0
	}
	left = total / 2
	return left, total - left
}

// statusKind selects the symbol and colour of a gutter line.
type statusKind int

const (
	statusInfo statusKind = iota
	statusOK
	statusWarn
	statusGo
)

// gutter prefixes text with a coloured status symbol in a fixed two-cell
// column.
func gutter(kind statusKind, text string, g Glyphs, c Colors) string {
	var symbol string
	switch kind {
	case statusOK:
		symbol = c.ok(g.OK)
	case statusWarn:
		symbol = c.warn(g.Warn)
	case statusGo:
		symbol = c.accent(g.Go)
	default:
		symbol = c.accent(g.Info)
	}
	return "  " + symbol + "  " + text
}

// labelRow is one "Label  value" line of a detail block.
type labelRow struct {
	Label string
	Value string
}

// labelRows renders rows with dim labels padded to a common width, indented
// by indent spaces. Values are written as given.
func labelRows(rows []labelRow, indent int, c Colors) []string {
	longest := 0
	for _, row := range rows {
		longest = max(longest, utf8.RuneCountInString(row.Label))
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, strings.Repeat(" ", indent)+padVisible(c.dim(row.Label), longest+2)+row.Value)
	}
	return lines
}
