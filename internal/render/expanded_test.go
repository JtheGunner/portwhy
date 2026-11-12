package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func renderExpanded(t *testing.T, opts Options, reports []report.PortReport) string {
	t.Helper()
	opts.Overview, opts.Expanded = true, true
	var buf strings.Builder
	if err := Text(&buf, reports, opts); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestText_ExpandedWide(t *testing.T) {
	golden(t, "text_expanded_wide.golden", []byte(renderExpanded(t, Options{Width: 120, Sources: fixtureSources()}, overviewFixture())))
}

func TestText_ExpandedNarrow(t *testing.T) {
	golden(t, "text_expanded_narrow.golden", []byte(renderExpanded(t, Options{Width: 60}, overviewFixture())))
}

func TestText_ExpandedColor(t *testing.T) {
	golden(t, "text_expanded_color.golden", []byte(renderExpanded(t, Options{Colors: Colors{Enabled: true}, Width: 120}, overviewFixture())))
}

func TestText_ExpandedAscii(t *testing.T) {
	golden(t, "text_expanded_ascii.golden", []byte(renderExpanded(t, Options{ASCII: true}, overviewFixture())))
}

func TestText_ExpandedHiddenOnly(t *testing.T) {
	golden(t, "text_expanded_hidden_only.golden", []byte(renderExpanded(t, Options{}, reportOf(
		finding(88, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
	))))
}

func TestText_ExpandedHasOneRowPerPortUnderATitledRule(t *testing.T) {
	out := renderExpanded(t, Options{}, overviewFixture())

	for _, want := range []string{
		" Antigravity · 4 ports ",
		` Homebrew service "php@8.4" · 1 port `,
		" Spotify · 2 ports ",
		" Owner not visible · 5 ports ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rule title %q missing from\n%s", want, out)
		}
	}
	for _, port := range []string{"3000", "3100", "3306", "3890", "1900", "5353", "22", "445"} {
		if n := countRowsWithPort(out, port); n != 1 {
			t.Errorf("port %s is on %d rows, want exactly one:\n%s", port, n, out)
		}
	}
}

// countRowsWithPort counts table rows (lines with a protocol) that carry port
// as their port column.
func countRowsWithPort(out, port string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.NewReplacer("⚠", " ").Replace(line))
		if len(fields) >= 3 && fields[0] == port && (fields[1] == "tcp" || fields[1] == "udp") {
			n++
		}
	}
	return n
}

func TestText_ExpandedKeepsRowsOfAGroupInPortOrder(t *testing.T) {
	out := renderExpanded(t, Options{}, reportOf(
		finding(8384, socket.TCP, "0.0.0.0", 7, "syncthing", detect.SourceProcess, "syncthing"),
		finding(22000, socket.UDP, "0.0.0.0", 7, "syncthing", detect.SourceProcess, "syncthing"),
		finding(22000, socket.TCP, "0.0.0.0", 7, "syncthing", detect.SourceProcess, "syncthing"),
	))

	first, mid, last := strings.Index(out, "8384"), strings.Index(out, "22000  tcp"), strings.Index(out, "22000  udp")
	if first < 0 || mid < first || last < mid {
		t.Errorf("rows not ordered by port, protocol (8384 %d, 22000 tcp %d, 22000 udp %d):\n%s", first, mid, last, out)
	}
}

func TestText_ExpandedEndsWithASummaryBoxAndAHintWithoutTheExpandedSwitch(t *testing.T) {
	out := renderExpanded(t, Options{Sources: fixtureSources()}, overviewFixture())

	for _, want := range []string{" Summary ", "Busy ports", "13", "Services", "4", "docker 1", "launchd 1", "process 2",
		"Reachable", "8", "bound to all interfaces", "Hidden owner", "5", "run with sudo",
		"Details: portwhy 1900", "Filter: --source docker|launchd|process"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from\n%s", want, out)
		}
	}
	if strings.Contains(out, "--expanded") {
		t.Errorf("the airy view must not advertise itself:\n%s", out)
	}
}

func TestText_ExpandedSummaryOmitsEmptyLines(t *testing.T) {
	out := renderExpanded(t, Options{}, reportOf(
		finding(80, socket.TCP, "127.0.0.1", 5, "nginx", detect.SourceProcess, "nginx"),
	))

	hiddenOnly := renderExpanded(t, Options{}, reportOf(
		finding(88, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
	))
	if strings.Contains(hiddenOnly, "Services") {
		t.Errorf("a zero services row must not appear:\n%s", hiddenOnly)
	}
	for _, unwanted := range []string{"Reachable", "Hidden owner"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%q must not appear when there is nothing to report:\n%s", unwanted, out)
		}
	}
}

func TestText_ExpandedRulesAndBoxFollowTheLayoutWidth(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int
	}{
		{"piped", Options{}, 78},
		{"wide terminal is capped", Options{Width: 140}, 100},
		{"--wide uses the full width", Options{Width: 140, Wide: true}, 140},
		{"normal terminal", Options{Width: 90}, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderExpanded(t, tt.opts, overviewFixture())

			for _, line := range strings.Split(out, "\n") {
				if strings.ContainsAny(line, "─╭╰│") && utf8.RuneCountInString(line) != tt.want {
					t.Errorf("rule or box line is %d columns, want %d: %q", utf8.RuneCountInString(line), tt.want, line)
				}
			}
		})
	}
}

func TestText_ExpandedNoColorHasNoEscapes(t *testing.T) {
	if out := renderExpanded(t, Options{}, overviewFixture()); strings.Contains(out, "\x1b") {
		t.Errorf("escape sequence in %q", out)
	}
}

func TestText_ExpandedNeutralisesControlCharacters(t *testing.T) {
	out := renderExpanded(t, Options{}, reportOf(
		finding(80, socket.TCP, "0.0.0.0", 1, "na\x1b[31mme\r", detect.SourceLaunchd, "ti\x1b[2Jtle\r"),
	))

	if strings.ContainsAny(out, "\x1b\r") {
		t.Errorf("control character in %q", out)
	}
}

func TestText_ExpandedTinyWidthDoesNotPanic(t *testing.T) {
	for _, w := range []int{1, 2, 10, 39} {
		if out := renderExpanded(t, Options{Width: w}, overviewFixture()); out == "" || !utf8.ValidString(out) {
			t.Errorf("width %d: output empty or invalid UTF-8", w)
		}
	}
}

func TestText_ExpandedListsEveryPortOfABusyMachine(t *testing.T) {
	out := renderExpanded(t, Options{}, manyPortsFixture())

	rows := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "  tcp  ") {
			rows++
		}
	}
	if rows != 117 {
		t.Errorf("expanded view has %d port rows, want 117", rows)
	}
}

func TestText_ExpandedOmitsFreePorts(t *testing.T) {
	out := renderExpanded(t, Options{}, append(overviewFixture(), report.PortReport{Port: 7777}, report.PortReport{Port: 7778, TimeWaitOnly: true}))

	if strings.Contains(out, "7777") || strings.Contains(out, "7778") {
		t.Errorf("free ports must not appear:\n%s", out)
	}
}
