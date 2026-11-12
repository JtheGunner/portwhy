package render

import (
	"bytes"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
)

func renderSources(t *testing.T, counts []report.SourceCount) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Sources(&b, counts); err != nil {
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
