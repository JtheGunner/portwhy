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
