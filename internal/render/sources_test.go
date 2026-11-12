package render

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
)

func renderSources(t *testing.T, counts []report.SourceCount) []byte {
	t.Helper()
	return renderSourcesWith(t, Options{}, counts)
}

func renderSourcesWith(t *testing.T, opts Options, counts []report.SourceCount) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Sources(&b, counts, opts); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSources_Footer(t *testing.T) {
	golden(t, "text_sources.golden", renderSources(t, []report.SourceCount{
		{Source: detect.SourceDocker, Count: 2},
		{Source: detect.SourceLaunchd, Count: 1},
		{Source: detect.SourceProcess, Count: 4},
	}))
}

func TestSources_SingleSource(t *testing.T) {
	golden(t, "text_sources_single.golden", renderSources(t, []report.SourceCount{
		{Source: detect.SourceProcess, Count: 1},
	}))
}

func TestSources_NothingToListWritesNothing(t *testing.T) {
	if got := renderSources(t, nil); len(got) != 0 {
		t.Errorf("output = %q, want empty", got)
	}
}

func manySources() []report.SourceCount {
	return []report.SourceCount{
		{Source: detect.SourceDocker, Count: 12},
		{Source: detect.SourceSystemd, Count: 3},
		{Source: detect.SourceLaunchd, Count: 4},
		{Source: detect.SourceProcess, Count: 20},
	}
}

func TestSources_FooterFitsTheWidthOnATerminal(t *testing.T) {
	out := string(renderSourcesWith(t, Options{Width: 60}, manySources()))

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if n := utf8.RuneCountInString(line); n > 60 {
			t.Errorf("line of %d runes exceeds width 60: %q", n, line)
		}
	}
	if flat := strings.Join(strings.Fields(out), " "); !strings.Contains(flat, "process (20) — filter with --source <name>") {
		t.Errorf("wrapping lost words:\n%s", out)
	}
}

func TestSources_FooterIsNotWrappedWithoutATerminalOrWithWide(t *testing.T) {
	for name, opts := range map[string]Options{"no terminal": {}, "wide": {Width: 60, Wide: true}} {
		if got := strings.Count(string(renderSourcesWith(t, opts, manySources())), "\n"); got != 1 {
			t.Errorf("%s: footer spans %d lines, want 1", name, got)
		}
	}
}

func TestSources_FooterIsDimmedWithColor(t *testing.T) {
	out := string(renderSourcesWith(t, Options{Colors: Colors{Enabled: true}}, manySources()))

	if !strings.HasPrefix(out, ansiDim) {
		t.Errorf("footer = %q, want it dimmed like the other footers", out)
	}
}
