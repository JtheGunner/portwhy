package render

import (
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
	golden(t, "text_overview_wide.golden", renderOverview(t, Options{Width: 120}, overviewFixture()))
}

func TestText_OverviewNarrow(t *testing.T) {
	golden(t, "text_overview_narrow.golden", renderOverview(t, Options{Width: 50}, overviewFixture()))
}

func TestText_OverviewColor(t *testing.T) {
	golden(t, "text_overview_color.golden", renderOverview(t, Options{Colors: Colors{Enabled: true}, Width: 120}, overviewFixture()))
}

func TestText_OverviewHiddenOnly(t *testing.T) {
	golden(t, "text_overview_hidden_only.golden", renderOverview(t, Options{}, reportOf(
		finding(88, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
	)))
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
