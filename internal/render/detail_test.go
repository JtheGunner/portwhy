package render

import (
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const longCommand = "php-fpm: master process (/opt/homebrew/etc/php/8.4/php-fpm.conf) --nodaemonize --fpm-config /opt/homebrew/etc/php/8.4/php-fpm.conf"

func serviceWorkersReport() report.PortReport {
	title := `Homebrew service "php@8.4"`
	mk := func(pid int, name string) report.Finding {
		return report.Finding{
			Socket:  socket.Socket{Port: 9000, Protocol: socket.TCP, Addr: netip.MustParseAddr("0.0.0.0"), State: socket.Listen, PID: pid},
			Process: &proc.Process{PID: pid, Name: name},
			Explanation: detect.Explanation{
				Source: detect.SourceLaunchd,
				Title:  title,
				Details: []detect.Detail{
					{Key: "Label", Value: "homebrew.mxcl.php@8.4"},
					{Key: "Process", Value: name + " (PID " + strconv.Itoa(pid) + ")"},
					{Key: "Uptime", Value: "3d 0h"},
				},
				Terminator: fakeTerminator("brew services stop php@8.4"),
			},
			Advice: []advise.Advice{{Level: advise.Warn, Text: "Reachable from the network (bound to all interfaces)."}},
		}
	}
	return report.PortReport{Port: 9000, Findings: []report.Finding{mk(5312, "php-fpm"), mk(3884, "php-fpm"), mk(5313, "php-fpm")}}
}

func processReport() report.PortReport {
	return report.PortReport{Port: 3000, Findings: []report.Finding{{
		Socket:  socket.Socket{Port: 3000, Protocol: socket.TCP, Addr: netip.MustParseAddr("127.0.0.1"), State: socket.Listen, PID: 3983},
		Process: &proc.Process{PID: 3983, Name: "Antigravity", Started: time.Date(2026, 11, 9, 8, 30, 0, 0, time.UTC)},
		Explanation: detect.Explanation{
			Source: detect.SourceProcess,
			Title:  "Antigravity",
			Details: []detect.Detail{
				{Key: "Command", Value: longCommand},
				{Key: "User", Value: "jeffry"},
				{Key: "Uptime", Value: "6d 1h"},
				{Key: "Cwd", Value: "/Users/jeffry/Projects/some/deeply/nested/project/directory"},
				{Key: "Started by", Value: "Antigravity ← launchd"},
			},
			Terminator: fakeTerminator("kill 3983"),
		},
	}}}
}

func TestText_DetailServiceWithWorkersMergesIntoOneBlock(t *testing.T) {
	out := string(renderText(t, Colors{}, serviceWorkersReport()))

	if n := strings.Count(out, ":9000"); n != 1 {
		t.Errorf("header appears %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, "brew services stop php@8.4"); n != 1 {
		t.Errorf("stop line appears %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "php-fpm (PID 3884) +2 more") {
		t.Errorf("want the lowest PID and the worker count:\n%s", out)
	}
	golden(t, "text_detail_service_workers.golden", []byte(out))
}

func TestText_DetailProcessLayout(t *testing.T) {
	golden(t, "text_detail_process.golden", renderText(t, Colors{}, processReport()))
}

func TestText_DetailCommandWrapsAtWidth(t *testing.T) {
	var buf strings.Builder
	if err := Text(&buf, []report.PortReport{processReport()}, Options{Width: 80}); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line of %d runes exceeds width 80: %q", n, line)
		}
	}
	if flat := strings.Join(strings.Fields(buf.String()), " "); !strings.Contains(flat, longCommand) {
		t.Errorf("a command that fits in three lines must be shown whole:\n%s", buf.String())
	}
	golden(t, "text_detail_wrapped_command.golden", []byte(buf.String()))
}

func TestText_DetailVeryLongCommandEndsInAnEllipsisAfterThreeLines(t *testing.T) {
	r := processReport()
	r.Findings[0].Explanation.Details[0].Value = strings.Repeat("--flag=value ", 30)
	var buf strings.Builder
	if err := Text(&buf, []report.PortReport{r}, Options{Width: 80}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "…") {
		t.Errorf("an over-long command should end in an ellipsis:\n%s", out)
	}
	if n := strings.Count(out, "--flag=value"); n > 3*4 {
		t.Errorf("command takes more than %d lines:\n%s", maxCommandLines, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line of %d runes exceeds width 80: %q", n, line)
		}
	}
}

func TestText_DetailWideKeepsFullCommand(t *testing.T) {
	var buf strings.Builder
	if err := Text(&buf, []report.PortReport{processReport()}, Options{Width: 80, Wide: true}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), longCommand) {
		t.Errorf("--wide must keep the full command:\n%s", buf.String())
	}
}

func TestText_DetailNoTruncationWithoutWidth(t *testing.T) {
	if out := renderText(t, Colors{}, processReport()); !strings.Contains(string(out), longCommand) {
		t.Errorf("no width means no truncation:\n%s", out)
	}
}

func TestText_DetailNoDuplicateName(t *testing.T) {
	out := string(renderText(t, Colors{}, processReport()))

	header := strings.SplitN(out, "\n", 2)[0]
	if n := strings.Count(header, "Antigravity"); n != 1 {
		t.Errorf("header %q names the process %d times, want 1", header, n)
	}
}

func TestText_DetailTinyWidthDoesNotPanic(t *testing.T) {
	for _, w := range []int{1, 2, 10} {
		var buf strings.Builder
		if err := Text(&buf, []report.PortReport{processReport(), serviceWorkersReport(), hiddenReport()}, Options{Width: w}); err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(buf.String()) {
			t.Errorf("width %d: invalid UTF-8", w)
		}
	}
}

func TestText_DetailDifferentPortsStaySeparate(t *testing.T) {
	a, b := processReport(), processReport()
	b.Port = 3100
	b.Findings[0].Socket.Port = 3100

	out := string(renderText(t, Colors{}, a, b))

	if strings.Count(out, ":3000") != 1 || strings.Count(out, ":3100") != 1 {
		t.Errorf("each requested port keeps its own block:\n%s", out)
	}
}

func launchdJobReport() report.PortReport {
	return report.PortReport{Port: 18789, Findings: []report.Finding{{
		Socket:  socket.Socket{Port: 18789, Protocol: socket.TCP, Addr: netip.MustParseAddr("0.0.0.0"), State: socket.Listen, PID: 3863},
		Process: &proc.Process{PID: 3863, Name: "node"},
		Explanation: detect.Explanation{
			Source:     detect.SourceLaunchd,
			Title:      `launchd job "ai.openclaw.gateway"`,
			Details:    []detect.Detail{{Key: "Domain", Value: "gui/501"}, {Key: "Uptime", Value: "6d 2h"}},
			Hints:      []string{"launchd restarts it after a plain kill if KeepAlive is set; unloading the job keeps it down until the next login"},
			Terminator: fakeTerminator("launchctl bootout gui/501/ai.openclaw.gateway"),
		},
		Advice: []advise.Advice{{Level: advise.Warn, Text: "Reachable from the network (bound to all interfaces)."}},
	}}}
}

func TestText_DetailEveryLineFitsTheWidth(t *testing.T) {
	var buf strings.Builder
	reports := []report.PortReport{launchdJobReport(), processReport(), timeWaitAdviceReport()}
	if err := Text(&buf, reports, Options{Width: 80}); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line of %d runes exceeds width 80: %q", n, line)
		}
	}
	golden(t, "text_detail_wrapped.golden", []byte(buf.String()))
}

func TestText_DetailWrappingKeepsTheWholeHint(t *testing.T) {
	var buf strings.Builder
	if err := Text(&buf, []report.PortReport{launchdJobReport()}, Options{Width: 60}); err != nil {
		t.Fatal(err)
	}

	flat := strings.Join(strings.Fields(buf.String()), " ")
	if !strings.Contains(flat, "KeepAlive is set; unloading the job keeps it down until the next login") {
		t.Errorf("hint lost in wrapping:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "launchctl bootout gui/501/ai.openclaw.gateway\n") {
		t.Errorf("the stop command must stay on one line:\n%s", buf.String())
	}
}

func TestText_DetailLongHeadlineAndProcessNameFitTheWidth(t *testing.T) {
	long := strings.Repeat("very-long-name-", 8)
	r := serviceWorkersReport()
	for i := range r.Findings {
		r.Findings[i].Explanation.Title = `Docker container "` + long + `" (registry.example.com/team/image:latest)`
		r.Findings[i].Process.Name = long
	}
	var buf strings.Builder

	if err := Text(&buf, []report.PortReport{r, processReport()}, Options{Width: 80}); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line of %d runes exceeds width 80: %q", n, line)
		}
	}
}
