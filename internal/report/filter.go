package report

import (
	"slices"

	"github.com/JtheGunner/portwhy/internal/detect"
)

// SourceCount is the number of findings that one source accounts for.
type SourceCount struct {
	Source detect.Source
	Count  int
}

// FilterBySource keeps only the findings whose source is in sources and drops
// every port left without a finding, free ports included: a free port has no
// source, so keeping it would show it as "free" next to a filtered list. An
// empty sources list returns reports unchanged. The input is not modified.
func FilterBySource(reports []PortReport, sources []detect.Source) []PortReport {
	if len(sources) == 0 {
		return reports
	}
	out := make([]PortReport, 0, len(reports))
	for _, r := range reports {
		var kept []Finding
		for _, f := range r.Findings {
			if slices.Contains(sources, f.Explanation.Source) {
				kept = append(kept, f)
			}
		}
		if len(kept) == 0 {
			continue
		}
		r.Findings = kept
		out = append(out, r)
	}
	return out
}

// CountSources counts the findings per source, sorted by source name.
func CountSources(reports []PortReport) []SourceCount {
	counts := make(map[detect.Source]int)
	for _, r := range reports {
		for _, f := range r.Findings {
			counts[f.Explanation.Source]++
		}
	}
	out := make([]SourceCount, 0, len(counts))
	for s, n := range counts {
		out = append(out, SourceCount{Source: s, Count: n})
	}
	slices.SortFunc(out, func(a, b SourceCount) int {
		switch {
		case a.Source < b.Source:
			return -1
		case a.Source > b.Source:
			return 1
		default:
			return 0
		}
	})
	return out
}
