package report

import (
	"reflect"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func findingFrom(port uint16, source detect.Source) Finding {
	return Finding{
		Socket:      socket.Socket{Port: port},
		Explanation: detect.Explanation{Source: source},
	}
}

func portReport(port uint16, sources ...detect.Source) PortReport {
	r := PortReport{Port: port}
	for _, s := range sources {
		r.Findings = append(r.Findings, findingFrom(port, s))
	}
	return r
}

func TestFilterBySource_KeepsOnlyMatchingFindings(t *testing.T) {
	reports := []PortReport{
		portReport(3000, detect.SourceProcess),
		portReport(5432, detect.SourceDocker),
		portReport(8080, detect.SourceDocker, detect.SourceProcess),
	}

	got := FilterBySource(reports, []detect.Source{detect.SourceDocker})

	if len(got) != 2 || got[0].Port != 5432 || got[1].Port != 8080 {
		t.Fatalf("ports = %v, want 5432 and 8080", got)
	}
	if n := len(got[1].Findings); n != 1 {
		t.Fatalf("port 8080 findings = %d, want 1 (only the docker one)", n)
	}
	if src := got[1].Findings[0].Explanation.Source; src != detect.SourceDocker {
		t.Errorf("port 8080 source = %q, want docker", src)
	}
}

func TestFilterBySource_MultipleSourcesAreOred(t *testing.T) {
	reports := []PortReport{
		portReport(3000, detect.SourceProcess),
		portReport(5432, detect.SourceDocker),
		portReport(9000, detect.SourceLaunchd),
	}

	got := FilterBySource(reports, []detect.Source{detect.SourceDocker, detect.SourceLaunchd})

	if len(got) != 2 || got[0].Port != 5432 || got[1].Port != 9000 {
		t.Errorf("ports = %v, want 5432 and 9000", got)
	}
}

func TestFilterBySource_DropsPortsWithoutFindings(t *testing.T) {
	// A free port has no source, so a filter must never keep it: an explicit
	// port would otherwise show up as "free" next to a filtered list.
	reports := []PortReport{{Port: 3000}, portReport(5432, detect.SourceDocker)}

	got := FilterBySource(reports, []detect.Source{detect.SourceDocker})

	if len(got) != 1 || got[0].Port != 5432 {
		t.Errorf("ports = %v, want only 5432", got)
	}
}

func TestFilterBySource_EmptyFilterReturnsInputUnchanged(t *testing.T) {
	reports := []PortReport{{Port: 3000}, portReport(5432, detect.SourceDocker)}

	got := FilterBySource(reports, nil)

	if !reflect.DeepEqual(got, reports) {
		t.Errorf("got %v, want input unchanged", got)
	}
}

func TestFilterBySource_DoesNotMutateInput(t *testing.T) {
	reports := []PortReport{portReport(8080, detect.SourceDocker, detect.SourceProcess)}

	FilterBySource(reports, []detect.Source{detect.SourceDocker})

	if n := len(reports[0].Findings); n != 2 {
		t.Errorf("input findings = %d, want 2 (unchanged)", n)
	}
}

func TestFilterBySource_NoMatchReturnsEmpty(t *testing.T) {
	got := FilterBySource([]PortReport{portReport(3000, detect.SourceProcess)}, []detect.Source{detect.SourceDocker})

	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestCountSources_CountsFindingsPerSourceSorted(t *testing.T) {
	reports := []PortReport{
		portReport(3000, detect.SourceProcess),
		portReport(3001, detect.SourceProcess),
		portReport(5432, detect.SourceDocker),
		portReport(8080, detect.SourceDocker, detect.SourceProcess),
		{Port: 9999},
	}

	got := CountSources(reports)

	want := []SourceCount{
		{Source: detect.SourceDocker, Count: 2},
		{Source: detect.SourceProcess, Count: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("counts = %v, want %v", got, want)
	}
}

func TestCountSources_NoFindingsIsEmpty(t *testing.T) {
	if got := CountSources([]PortReport{{Port: 3000}}); len(got) != 0 {
		t.Errorf("counts = %v, want none", got)
	}
}
