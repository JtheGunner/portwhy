package render

import (
	"strings"
	"testing"
)

func TestKillLine_UsesAStatusSymbolPerOutcome(t *testing.T) {
	tests := []struct {
		outcome KillOutcome
		want    string
	}{
		{KillStopped, "  ✓  Port 3000 is free."},
		{KillDeclined, "  ℹ  Port 3000 left untouched."},
		{KillFailed, "  ⚠  Port 3000: boom"},
	}
	for _, tt := range tests {
		got := KillLine(tt.outcome, 3000, "boom", Options{})
		if got != tt.want {
			t.Errorf("outcome %d: got %q, want %q", tt.outcome, got, tt.want)
		}
	}
}

func TestKillLine_NeutralisesControlCharacters(t *testing.T) {
	if got := KillLine(KillFailed, 1, "bad\x1b[31m\r", Options{}); strings.ContainsAny(got, "\x1b\r") {
		t.Errorf("control character in %q", got)
	}
}

func TestKillSummary_CountsOutcomesAndSkipsZeroRows(t *testing.T) {
	got := KillSummary(map[KillOutcome]int{KillStopped: 2, KillFailed: 1}, Options{})

	for _, want := range []string{" Result ", "Stopped", "Failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from\n%s", want, got)
		}
	}
	if strings.Contains(got, "Left untouched") {
		t.Errorf("zero row shown:\n%s", got)
	}
}

func TestKillLine_ASCII(t *testing.T) {
	if got := KillLine(KillStopped, 80, "", Options{ASCII: true}); got != "  +  Port 80 is free." {
		t.Errorf("got %q", got)
	}
}
