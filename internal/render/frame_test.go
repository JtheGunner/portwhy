package render

import (
	"regexp"
	"strings"
	"testing"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func TestFitWith(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		ell   string
		want  string
	}{
		{"fits", "abc", 5, "...", "abc"},
		{"ascii ellipsis", "abcdefgh", 6, "...", "abc..."},
		{"ascii ellipsis wider than width", "abcdefgh", 2, "...", ".."},
		{"unicode ellipsis", "abcdefgh", 4, "…", "abc…"},
		{"unlimited", "abcdefgh", 0, "...", "abcdefgh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitWith(tt.in, tt.width, tt.ell); got != tt.want {
				t.Errorf("fitWith(%q, %d, %q) = %q, want %q", tt.in, tt.width, tt.ell, got, tt.want)
			}
		})
	}
}

func TestRule(t *testing.T) {
	h := unicodeGlyphs.H
	a := asciiGlyphs.H
	tests := []struct {
		name  string
		title string
		width int
		g     Glyphs
		want  string
	}{
		{"even padding", "node", 20, unicodeGlyphs, strings.Repeat(h, 7) + " node " + strings.Repeat(h, 7)},
		{"odd padding puts the extra line on the right", "abc", 20, unicodeGlyphs, strings.Repeat(h, 7) + " abc " + strings.Repeat(h, 8)},
		{"long title is shortened", "ABCDEFGHIJKLMNOPQRSTUV", 20, unicodeGlyphs, strings.Repeat(h, 3) + " ABCDEFGHIJK… " + strings.Repeat(h, 3)},
		{"multi-byte title", "Überprüfung", 30, unicodeGlyphs, strings.Repeat(h, 8) + " Überprüfung " + strings.Repeat(h, 9)},
		{"ascii glyphs", "node", 20, asciiGlyphs, strings.Repeat(a, 7) + " node " + strings.Repeat(a, 7)},
		{"ascii ellipsis", "ABCDEFGHIJKLMNOPQRSTUV", 20, asciiGlyphs, strings.Repeat(a, 3) + " ABCDEFGHI... " + strings.Repeat(a, 3)},
		{"empty title is a plain line", "", 10, unicodeGlyphs, strings.Repeat(h, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule(tt.title, tt.width, tt.g, Colors{})

			if got != tt.want {
				t.Errorf("rule(%q, %d) =\n%q\nwant\n%q", tt.title, tt.width, got, tt.want)
			}
		})
	}
}

func TestRule_ColoursDoNotChangeTheWidth(t *testing.T) {
	got := rule("node · 3 ports", 60, unicodeGlyphs, Colors{Enabled: true})

	if w := visibleWidth(got); w != 60 {
		t.Errorf("visible width = %d, want 60; line %q", w, got)
	}
	if !strings.Contains(got, ansiBold) {
		t.Errorf("title is not bold: %q", got)
	}
}

func TestBox(t *testing.T) {
	got := box("Summary", []string{"Busy ports  9", "Hidden      2"}, 24, unicodeGlyphs, Colors{})

	want := strings.Join([]string{
		"╭" + strings.Repeat("─", 6) + " Summary " + strings.Repeat("─", 7) + "╮",
		"│ " + "Busy ports  9" + strings.Repeat(" ", 7) + " │",
		"│ " + "Hidden      2" + strings.Repeat(" ", 7) + " │",
		"╰" + strings.Repeat("─", 22) + "╯",
	}, "\n")
	if got != want {
		t.Errorf("box =\n%s\nwant\n%s", got, want)
	}
}

func TestBox_Ascii(t *testing.T) {
	got := box("Result", []string{"ok"}, 20, asciiGlyphs, Colors{})

	want := strings.Join([]string{
		"+" + strings.Repeat("-", 5) + " Result " + strings.Repeat("-", 5) + "+",
		"| ok" + strings.Repeat(" ", 14) + " |",
		"+" + strings.Repeat("-", 18) + "+",
	}, "\n")
	if got != want {
		t.Errorf("box =\n%s\nwant\n%s", got, want)
	}
}

func TestBox_ColouredRowsKeepTheBorderAligned(t *testing.T) {
	c := Colors{Enabled: true}
	rows := []string{c.dim("Busy ports") + "  " + c.port("9"), "plain row"}

	got := box("Summary", rows, 40, unicodeGlyphs, c)

	for i, line := range strings.Split(got, "\n") {
		if w := visibleWidth(line); w != 40 {
			t.Errorf("line %d has visible width %d, want 40: %q", i, w, line)
		}
	}
}

func TestBox_LongTitleIsShortened(t *testing.T) {
	got := box(strings.Repeat("T", 60), []string{"x"}, 30, unicodeGlyphs, Colors{})

	for i, line := range strings.Split(got, "\n") {
		if w := visibleWidth(line); w != 30 {
			t.Errorf("line %d has visible width %d, want 30: %q", i, w, line)
		}
	}
	if !strings.Contains(got, "…") {
		t.Errorf("long title was not shortened: %q", got)
	}
}

func TestGutter(t *testing.T) {
	tests := []struct {
		name string
		kind statusKind
		g    Glyphs
		want string
	}{
		{"info", statusInfo, unicodeGlyphs, "  ℹ  hello"},
		{"ok", statusOK, unicodeGlyphs, "  ✓  hello"},
		{"warn", statusWarn, unicodeGlyphs, "  ⚠  hello"},
		{"go", statusGo, unicodeGlyphs, "  →  hello"},
		{"ascii warn", statusWarn, asciiGlyphs, "  !  hello"},
		{"ascii go", statusGo, asciiGlyphs, "  >  hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gutter(tt.kind, "hello", tt.g, Colors{}); got != tt.want {
				t.Errorf("gutter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGutter_WarnIsYellowWhenColoured(t *testing.T) {
	got := gutter(statusWarn, "hello", unicodeGlyphs, Colors{Enabled: true})

	if !strings.Contains(got, ansiYellow+"⚠") {
		t.Errorf("warn glyph is not yellow: %q", got)
	}
	if stripANSI(got) != "  ⚠  hello" {
		t.Errorf("text changed by colours: %q", stripANSI(got))
	}
}

func TestLabelRows_AlignValues(t *testing.T) {
	rows := []labelRow{{"Process", "php-fpm"}, {"Started by", "launchd"}, {"User", "jeffry"}}

	got := labelRows(rows, 5, Colors{})

	want := []string{
		"     Process     php-fpm",
		"     Started by  launchd",
		"     User        jeffry",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("labelRows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLabelRows_ColoursDoNotBreakAlignment(t *testing.T) {
	rows := []labelRow{{"Process", "1"}, {"Started by", "2"}}

	got := labelRows(rows, 2, Colors{Enabled: true})

	for _, line := range got {
		plain := stripANSI(line)
		if idx := strings.IndexAny(plain, "12"); idx != 2+len("Started by")+2 {
			t.Errorf("value column at %d in %q", idx, plain)
		}
		if !strings.Contains(line, ansiDim) {
			t.Errorf("label is not dim: %q", line)
		}
	}
}

func TestLabelRows_Empty(t *testing.T) {
	if got := labelRows(nil, 5, Colors{}); len(got) != 0 {
		t.Errorf("labelRows(nil) = %q, want none", got)
	}
}
