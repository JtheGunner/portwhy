package render

import (
	"slices"
	"testing"
	"unicode/utf8"
)

func TestFit(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"unlimited", "abcdef", 0, "abcdef"},
		{"negative is unlimited", "abcdef", -1, "abcdef"},
		{"fits exactly", "abc", 3, "abc"},
		{"one over", "abcd", 3, "ab…"},
		{"width one", "abc", 1, "…"},
		{"width two", "abc", 2, "a…"},
		{"empty", "", 5, ""},
		{"multi-byte", "Überprüfung", 5, "Über…"},
		{"emoji", "🔌🔌🔌", 2, "🔌…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fit(tt.in, tt.width)

			if got != tt.want {
				t.Errorf("fit(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("fit(%q, %d) = %q is not valid UTF-8", tt.in, tt.width, got)
			}
		})
	}
}

func TestPadRight_CountsRunes(t *testing.T) {
	if got := padRight("Äb", 4); got != "Äb  " {
		t.Errorf("padRight = %q", got)
	}
	if got := padRight("abcd", 2); got != "abcd" {
		t.Errorf("padRight must not truncate, got %q", got)
	}
}

func TestWrapWords(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"unlimited", "a b c", 0, []string{"a b c"}},
		{"fits", "a b c", 5, []string{"a b c"}},
		{"wraps", "aaa bbb ccc", 7, []string{"aaa bbb", "ccc"}},
		{"long word stays whole", "a bbbbbbbbbb c", 4, []string{"a", "bbbbbbbbbb", "c"}},
		{"empty", "", 5, []string{""}},
		{"counts runes", "äää ööö", 3, []string{"äää", "ööö"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrapWords(tt.in, tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("wrapWords(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
		})
	}
}

func TestLayoutWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		wide  bool
		want  int
	}{
		{"piped output uses the default", 0, false, 78},
		{"wide terminal is capped", 120, false, 100},
		{"wide flag keeps the full width", 120, true, 120},
		{"wide flag without a terminal", 0, true, 78},
		{"tiny terminal is raised", 30, false, 40},
		{"normal terminal", 80, false, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutWidth(tt.width, tt.wide); got != tt.want {
				t.Errorf("layoutWidth(%d, %v) = %d, want %d", tt.width, tt.wide, got, tt.want)
			}
		})
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"plain", "abc", 3},
		{"empty", "", 0},
		{"coloured", "\x1b[1m\x1b[32mabc\x1b[0m", 3},
		{"multi-byte", "Überprüfung", 11},
		{"colour inside text", "a\x1b[2mb\x1b[0mc", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibleWidth(tt.in); got != tt.want {
				t.Errorf("visibleWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestPadVisible_IgnoresColourCodes(t *testing.T) {
	got := padVisible("\x1b[1mab\x1b[0m", 5)

	if want := "\x1b[1mab\x1b[0m   "; got != want {
		t.Errorf("padVisible = %q, want %q", got, want)
	}
	if got := padVisible("abcd", 2); got != "abcd" {
		t.Errorf("padVisible must not shorten, got %q", got)
	}
}
