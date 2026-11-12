package render

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func overviewFixture() []report.PortReport {
	php := `Homebrew service "php@8.4"`
	docker := `Docker container "db" (postgres:16)`
	fs := []report.Finding{
		finding(9000, socket.TCP, "127.0.0.1", 5312, "php-fpm", detect.SourceLaunchd, php),
		finding(9000, socket.TCP, "127.0.0.1", 3884, "php-fpm", detect.SourceLaunchd, php),
		finding(9000, socket.TCP, "127.0.0.1", 5313, "php-fpm", detect.SourceLaunchd, php),
		finding(5432, socket.TCP, "0.0.0.0", 812, "postgres", detect.SourceDocker, docker),
		finding(1900, socket.UDP, "0.0.0.0", 13250, "Spotify", detect.SourceProcess, "Spotify"),
		finding(5353, socket.UDP, "0.0.0.0", 13250, "Spotify", detect.SourceProcess, "Spotify"),
	}
	for _, p := range []uint16{3000, 3100, 3306, 3890} {
		fs = append(fs, finding(p, socket.TCP, "127.0.0.1", 3983, "Antigravity", detect.SourceProcess, "Antigravity"))
	}
	for _, p := range []uint16{22, 88, 137, 138, 445} {
		fs = append(fs, finding(p, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"))
	}
	return reportOf(fs...)
}

func fixtureSources() []report.SourceCount {
	return []report.SourceCount{
		{Source: detect.SourceDocker, Count: 1},
		{Source: detect.SourceLaunchd, Count: 3},
		{Source: detect.SourceProcess, Count: 11},
	}
}

func renderOverview(t *testing.T, opts Options, reports []report.PortReport) []byte {
	t.Helper()
	opts.Overview = true
	var buf strings.Builder
	if err := Text(&buf, reports, opts); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

func TestText_OverviewWide(t *testing.T) {
	golden(t, "text_overview_wide.golden", renderOverview(t, Options{Width: 120, Sources: fixtureSources()}, overviewFixture()))
}

func TestText_OverviewNarrow(t *testing.T) {
	golden(t, "text_overview_narrow.golden", renderOverview(t, Options{Width: 60}, overviewFixture()))
}

func TestText_OverviewColor(t *testing.T) {
	golden(t, "text_overview_color.golden", renderOverview(t, Options{Colors: Colors{Enabled: true}, Width: 120, Sources: fixtureSources()}, overviewFixture()))
}

func TestText_OverviewAscii(t *testing.T) {
	golden(t, "text_overview_ascii.golden", renderOverview(t, Options{ASCII: true, Sources: fixtureSources()}, overviewFixture()))
}

func TestText_OverviewHiddenOnly(t *testing.T) {
	golden(t, "text_overview_hidden_only.golden", renderOverview(t, Options{}, reportOf(
		finding(88, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
	)))
}

func TestText_OverviewSummaryCountsPortsServicesAndExposure(t *testing.T) {
	out := string(renderOverview(t, Options{}, overviewFixture()))

	want := "13 busy ports · 4 services · ⚠ 8 reachable from the network · 5 ports with hidden owner"
	if !strings.Contains(out, want) {
		t.Errorf("summary %q missing from\n%s", want, out)
	}
}

func TestText_OverviewHintNamesTheSourcesInUse(t *testing.T) {
	with := string(renderOverview(t, Options{Sources: fixtureSources()}, overviewFixture()))
	without := string(renderOverview(t, Options{}, overviewFixture()))

	if !strings.Contains(with, "--source docker|launchd|process") {
		t.Errorf("hint lacks the sources in use:\n%s", with)
	}
	if strings.Contains(without, "--source") {
		t.Errorf("hint names a filter although no sources are known:\n%s", without)
	}
	if !strings.Contains(with, "Details: portwhy 1900") || !strings.Contains(with, "--expanded") {
		t.Errorf("hint lacks the details command or the --expanded switch:\n%s", with)
	}
}

func TestText_OverviewShowsHiddenOwnersAsRowsNotJustACount(t *testing.T) {
	out := string(renderOverview(t, Options{}, overviewFixture()))

	for _, want := range []string{"Owner not visible", "22, 88, 137, 138, 445", "run with sudo"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from\n%s", want, out)
		}
	}
}

func TestText_OverviewSplitsHiddenOwnersByProtocolAndAddress(t *testing.T) {
	out := string(renderOverview(t, Options{}, reportOf(
		finding(22, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
		finding(137, socket.UDP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
		finding(631, socket.TCP, "127.0.0.1", 0, "", detect.SourceProcess, "owner not visible"),
	)))

	if n := strings.Count(out, "Owner not visible"); n != 3 {
		t.Errorf("want 3 hidden-owner rows (tcp/0.0.0.0, udp/0.0.0.0, tcp/127.0.0.1), got %d:\n%s", n, out)
	}
}

func TestText_OverviewSummaryUsesSingularForOne(t *testing.T) {
	out := string(renderOverview(t, Options{}, reportOf(
		finding(80, socket.TCP, "127.0.0.1", 5, "nginx", detect.SourceProcess, "nginx"),
	)))

	if want := "1 busy port " + "·" + " 1 service\n"; !strings.Contains(out, want) {
		t.Errorf("%q missing from\n%s", want, out)
	}
}

func TestText_OverviewTinyWidthDoesNotPanic(t *testing.T) {
	for _, w := range []int{1, 2, 10} {
		out := renderOverview(t, Options{Width: w}, overviewFixture())
		if len(out) == 0 || !utf8.Valid(out) {
			t.Errorf("width %d: output empty or invalid UTF-8", w)
		}
	}
}

func TestText_OverviewWideFlagIgnoresWidth(t *testing.T) {
	narrow := renderOverview(t, Options{Width: 40, Wide: true}, overviewFixture())
	unlimited := renderOverview(t, Options{}, overviewFixture())

	if string(narrow) != string(unlimited) {
		t.Errorf("--wide output differs from unlimited\n%s\nvs\n%s", narrow, unlimited)
	}
}

func TestText_OverviewNoColorHasNoEscapes(t *testing.T) {
	if out := renderOverview(t, Options{}, overviewFixture()); strings.Contains(string(out), "\x1b") {
		t.Errorf("escape sequence in %q", out)
	}
}

func TestText_OverviewOmitsFreePorts(t *testing.T) {
	out := renderOverview(t, Options{}, append(overviewFixture(), report.PortReport{Port: 7777}, report.PortReport{Port: 7778, TimeWaitOnly: true}))

	if strings.Contains(string(out), "7777") || strings.Contains(string(out), "7778") {
		t.Errorf("free ports must not appear:\n%s", out)
	}
}

func TestText_OverviewNeutralisesControlCharacters(t *testing.T) {
	out := renderOverview(t, Options{}, reportOf(
		finding(80, socket.TCP, "0.0.0.0", 1, "na\x1b[31mme\r", detect.SourceLaunchd, "ti\x1b[2Jtle\r"),
	))

	if strings.ContainsAny(string(out), "\x1b\r") {
		t.Errorf("control character in %q", out)
	}
}

func TestText_OverviewCapsLongPortListsOnlyOnATerminal(t *testing.T) {
	var fs []report.Finding
	for p := uint16(3000); p < 3300; p += 2 {
		fs = append(fs, finding(p, socket.TCP, "127.0.0.1", 3983, "Antigravity", detect.SourceProcess, "Antigravity"))
	}
	fs = append(fs, finding(80, socket.TCP, "127.0.0.1", 5, "nginx", detect.SourceProcess, "nginx"))
	reports := reportOf(fs...)

	capped := string(renderOverview(t, Options{Width: 80}, reports))
	piped := string(renderOverview(t, Options{}, reports))
	wide := string(renderOverview(t, Options{Width: 80, Wide: true}, reports))

	for _, line := range strings.Split(capped, "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line of %d runes in the capped overview: %q", n, line)
		}
	}
	if !strings.Contains(capped, "… (+") {
		t.Errorf("a cut port list must say how many ports are left out:\n%s", capped)
	}
	for name, out := range map[string]string{"without a terminal": piped, "--wide": wide} {
		if !strings.Contains(out, "3298") || strings.Contains(out, "… (+") {
			t.Errorf("%s must list every port:\n%s", name, out)
		}
	}
}

func TestText_OverviewLongTitlesAreShortenedOnlyOnATerminal(t *testing.T) {
	title := `Docker container "a-very-long-container-name" (registry.example.com/team/image:latest)`
	reports := reportOf(finding(5432, socket.TCP, "127.0.0.1", 812, "postgres", detect.SourceDocker, title))

	terminal := string(renderOverview(t, Options{Width: 100}, reports))
	piped := string(renderOverview(t, Options{}, reports))

	if strings.Contains(terminal, title) || !strings.Contains(terminal, "…") {
		t.Errorf("long title not shortened on a terminal:\n%s", terminal)
	}
	if !strings.Contains(piped, title) {
		t.Errorf("piped output must keep the full title:\n%s", piped)
	}
}

// manyPortsFixture is a busy machine: 25 owners with four ports each and 17
// ports whose owner is hidden, 117 ports in all.
func manyPortsFixture() []report.PortReport {
	var fs []report.Finding
	for i := range 25 {
		name := fmt.Sprintf("service%02d", i)
		for j := range 4 {
			fs = append(fs, finding(uint16(10000+i*10+j*2), socket.TCP, "127.0.0.1", 1000+i, name, detect.SourceProcess, name))
		}
	}
	for i := range 17 {
		fs = append(fs, finding(uint16(100+i*3), socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"))
	}
	return reportOf(fs...)
}

func TestText_OverviewOfAHundredPortsStaysCompact(t *testing.T) {
	out := string(renderOverview(t, Options{}, manyPortsFixture()))

	lines := strings.Count(out, "\n")
	if lines > 40 {
		t.Errorf("overview of 117 ports has %d lines, want at most 40\n%s", lines, out)
	}
	if !strings.Contains(out, "117 busy ports") {
		t.Errorf("summary does not count 117 ports:\n%s", out)
	}
}

func TestText_OverviewOneHugePortListDoesNotWidenTheColumnsOfOtherRows(t *testing.T) {
	var fs []report.Finding
	for p := uint16(40000); p < 40300; p += 2 {
		fs = append(fs, finding(p, socket.TCP, "127.0.0.1", 3983, "Helper", detect.SourceProcess, "Helper"))
	}
	fs = append(fs, finding(80, socket.TCP, "127.0.0.1", 5, "nginx", detect.SourceProcess, "nginx"))
	out := string(renderOverview(t, Options{}, reportOf(fs...)))

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "nginx") && strings.Index(line, "tcp") > 60 {
			t.Errorf("a short row is pushed to column %d by one long port list:\n%s", strings.Index(line, "tcp"), line)
		}
	}
	if !strings.Contains(out, "40298") {
		t.Errorf("piped output must still list every port")
	}
	if n := utf8.RuneCountInString(strings.Split(out, "\n")[1]); n > 100 {
		t.Errorf("the separator is %d columns wide, it must not follow the long port list", n)
	}
}
