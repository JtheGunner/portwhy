package render

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func finding(port uint16, proto socket.Protocol, addr string, pid int, name string, src detect.Source, title string) report.Finding {
	f := report.Finding{
		Socket:      socket.Socket{Port: port, Protocol: proto, Addr: netip.MustParseAddr(addr), State: socket.Listen, PID: pid},
		Explanation: detect.Explanation{Source: src, Title: title},
	}
	if pid != 0 {
		f.Process = &proc.Process{PID: pid, Name: name}
	}
	return f
}

func reportOf(findings ...report.Finding) []report.PortReport {
	byPort := map[uint16]*report.PortReport{}
	var order []uint16
	for _, f := range findings {
		r, ok := byPort[f.Socket.Port]
		if !ok {
			r = &report.PortReport{Port: f.Socket.Port}
			byPort[f.Socket.Port] = r
			order = append(order, f.Socket.Port)
		}
		r.Findings = append(r.Findings, f)
	}
	out := make([]report.PortReport, 0, len(order))
	for _, p := range order {
		out = append(out, *byPort[p])
	}
	return out
}

func TestFormatPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []uint16
		want  string
	}{
		{"empty", nil, ""},
		{"single", []uint16{80}, "80"},
		{"gaps", []uint16{3000, 3100, 3306, 3890}, "3000, 3100, 3306, 3890"},
		{"run of four", []uint16{8000, 8001, 8002, 8003}, "8000-8003"},
		{"pair stays", []uint16{8000, 8001}, "8000, 8001"},
		{"run of three", []uint16{1, 2, 3, 9}, "1-3, 9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPorts(tt.ports); got != tt.want {
				t.Errorf("formatPorts(%v) = %q, want %q", tt.ports, got, tt.want)
			}
		})
	}
}

func TestBuildOverview_SamePIDManyPortsOneRow(t *testing.T) {
	var fs []report.Finding
	for _, p := range []uint16{3890, 3000, 3306, 3100} {
		fs = append(fs, finding(p, socket.TCP, "127.0.0.1", 3983, "Antigravity", detect.SourceProcess, "Antigravity"))
	}

	ov := buildOverview(reportOf(fs...))

	if len(ov.Groups) != 1 || len(ov.Groups[0].Rows) != 1 {
		t.Fatalf("groups = %+v, want one group with one row", ov.Groups)
	}
	got := ov.Groups[0].Rows[0]
	if !slices.Equal(got.Ports, []uint16{3000, 3100, 3306, 3890}) || got.Name != "Antigravity" || !slices.Equal(got.PIDs, []int{3983}) {
		t.Errorf("row = %+v", got)
	}
	if got.Exposed {
		t.Error("127.0.0.1 must not be exposed")
	}
}

func TestBuildOverview_WorkersCollapseIntoServiceRow(t *testing.T) {
	title := `Homebrew service "php@8.4"`
	ov := buildOverview(reportOf(
		finding(9000, socket.TCP, "127.0.0.1", 5312, "php-fpm", detect.SourceLaunchd, title),
		finding(9000, socket.TCP, "127.0.0.1", 3884, "php-fpm", detect.SourceLaunchd, title),
		finding(9000, socket.TCP, "127.0.0.1", 5313, "php-fpm", detect.SourceLaunchd, title),
	))

	if len(ov.Groups) != 1 || len(ov.Groups[0].Rows) != 1 {
		t.Fatalf("groups = %+v", ov.Groups)
	}
	row := ov.Groups[0].Rows[0]
	if !slices.Equal(row.PIDs, []int{3884, 5312, 5313}) || row.Name != "php-fpm" {
		t.Errorf("row = %+v", row)
	}
	if ov.Groups[0].Title != title || ov.Groups[0].Source != detect.SourceLaunchd {
		t.Errorf("group = %+v", ov.Groups[0])
	}
}

func TestBuildOverview_ProtocolAndAddressStaySeparate(t *testing.T) {
	var fs []report.Finding
	for _, proto := range []socket.Protocol{socket.TCP, socket.UDP} {
		for _, addr := range []string{"127.0.0.1", "::1"} {
			fs = append(fs, finding(5353, proto, addr, 10, "mdns", detect.SourceProcess, "mdns"))
		}
	}

	ov := buildOverview(reportOf(fs...))

	if len(ov.Groups) != 1 || len(ov.Groups[0].Rows) != 4 {
		t.Fatalf("rows = %+v, want 4", ov.Groups)
	}
}

func TestBuildOverview_ExposedWhenUnspecified(t *testing.T) {
	ov := buildOverview(reportOf(
		finding(80, socket.TCP, "0.0.0.0", 1, "a", detect.SourceProcess, "a"),
		finding(81, socket.TCP, "::", 2, "b", detect.SourceProcess, "b"),
	))

	for _, g := range ov.Groups {
		if !g.Rows[0].Exposed {
			t.Errorf("row %+v should be exposed", g.Rows[0])
		}
	}
}

func TestBuildOverview_HiddenOwnersCollected(t *testing.T) {
	ov := buildOverview(reportOf(
		finding(88, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
		finding(22, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
	))

	if len(ov.Groups) != 0 || len(ov.Hidden) != 2 {
		t.Fatalf("overview = %+v, want 2 hidden and no group", ov)
	}
}

func TestBuildOverview_OrderIsStable(t *testing.T) {
	a := finding(9000, socket.TCP, "127.0.0.1", 1, "a", detect.SourceProcess, "a")
	b := finding(100, socket.TCP, "127.0.0.1", 2, "b", detect.SourceProcess, "b")
	c := finding(5000, socket.TCP, "127.0.0.1", 3, "c", detect.SourceProcess, "c")

	ov := buildOverview(reportOf(a, b, c))

	var titles []string
	for _, g := range ov.Groups {
		titles = append(titles, g.Title)
	}
	if !slices.Equal(titles, []string{"b", "c", "a"}) {
		t.Errorf("group order = %v, want by lowest port", titles)
	}
}

func TestBuildOverview_SameNameDifferentSourceStaysSeparate(t *testing.T) {
	ov := buildOverview(reportOf(
		finding(1, socket.TCP, "127.0.0.1", 1, "x", detect.SourceProcess, "x"),
		finding(2, socket.TCP, "127.0.0.1", 2, "x", detect.SourceLaunchd, "x"),
	))

	if len(ov.Groups) != 2 {
		t.Errorf("groups = %d, want 2", len(ov.Groups))
	}
}
