package render

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

var update = flag.Bool("update", false, "rewrite golden files")

type fakeTerminator string

func (f fakeTerminator) Describe() string                { return string(f) }
func (f fakeTerminator) Terminate(context.Context) error { return nil }

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func dockerReport() report.PortReport {
	return report.PortReport{
		Port: 5432,
		Findings: []report.Finding{{
			Socket: socket.Socket{Port: 5432, Protocol: socket.TCP, Addr: netip.MustParseAddr("0.0.0.0"), State: socket.Listen, PID: 812},
			Process: &proc.Process{
				PID: 812, PPID: 1, Name: "postgres", Cmdline: []string{"postgres", "-D", "/data"},
				User: "root", Started: time.Date(2026, 11, 9, 8, 30, 0, 0, time.UTC),
				Ancestors: []proc.Ancestor{{PID: 1, Name: "init"}},
			},
			Explanation: detect.Explanation{
				Source: detect.SourceDocker,
				Title:  `Docker container "db" (postgres:16)`,
				Details: []detect.Detail{
					{Key: "Mapping", Value: "5432→5432"},
					{Key: "up", Value: "3 days"},
				},
				Hints:      []string{`restart policy "always": it comes back after a restart`},
				Terminator: fakeTerminator("docker stop db"),
			},
		}},
	}
}

func hiddenReport() report.PortReport {
	return report.PortReport{
		Port: 8080,
		Findings: []report.Finding{{
			Socket:      socket.Socket{Port: 8080, Protocol: socket.TCP, Addr: netip.MustParseAddr("::"), State: socket.Listen},
			Explanation: detect.Explanation{Source: detect.SourceProcess, Title: "Process"},
		}},
	}
}

func adviceReport() report.PortReport {
	r := dockerReport()
	r.Findings[0].Advice = []advise.Advice{
		{Level: advise.Warn, Text: "Listening on all interfaces"},
		{Level: advise.Info, Text: "Try 127.0.0.1 for local-only access"},
	}
	return r
}

func renderText(t *testing.T, c Colors, reports ...report.PortReport) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Text(&buf, reports, c); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func renderJSON(t *testing.T, reports ...report.PortReport) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := JSON(&buf, reports); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestText_DockerFinding(t *testing.T) {
	golden(t, "text_docker.golden", renderText(t, Colors{}, dockerReport()))
}

func TestText_FreePort(t *testing.T) {
	golden(t, "text_free.golden", renderText(t, Colors{}, report.PortReport{Port: 3000}))
}

func TestText_TimeWaitOnly(t *testing.T) {
	golden(t, "text_time_wait.golden", renderText(t, Colors{}, report.PortReport{Port: 3000, TimeWaitOnly: true}))
}

func TestText_OwnerNotVisible(t *testing.T) {
	golden(t, "text_owner_hidden.golden", renderText(t, Colors{}, hiddenReport()))
}

func TestText_AdviceMarkers(t *testing.T) {
	golden(t, "text_advice.golden", renderText(t, Colors{}, adviceReport()))
}

func TestText_Color(t *testing.T) {
	golden(t, "text_color.golden", renderText(t, Colors{Enabled: true}, adviceReport()))
}

func TestText_NoColorHasNoEscapes(t *testing.T) {
	out := renderText(t, Colors{}, adviceReport(), hiddenReport())
	if strings.Contains(string(out), "\x1b") {
		t.Errorf("escape sequence in no-color output: %q", out)
	}
}

func TestText_MultipleReports(t *testing.T) {
	golden(t, "text_multi.golden", renderText(t, Colors{}, report.PortReport{Port: 3000}, dockerReport()))
}

func TestColorsFor(t *testing.T) {
	var buf bytes.Buffer
	t.Setenv("NO_COLOR", "")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	if ColorsFor(&buf, false).Enabled {
		t.Error("non-file writer must not get colours")
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if ColorsFor(f, false).Enabled {
		t.Error("regular file is not a TTY")
	}
	if ColorsFor(f, true).Enabled {
		t.Error("flag must disable colours")
	}
	t.Setenv("NO_COLOR", "1")
	if ColorsFor(f, false).Enabled {
		t.Error("NO_COLOR must disable colours")
	}
}

func TestJSON_Schema(t *testing.T) {
	out := renderJSON(t, dockerReport(), report.PortReport{Port: 3000})
	var doc struct {
		Version int `json:"version"`
		Ports   []struct {
			Port     int    `json:"port"`
			State    string `json:"state"`
			Findings []struct {
				Protocol  string `json:"protocol"`
				Address   string `json:"address"`
				PID       int    `json:"pid"`
				Source    string `json:"source"`
				Terminate struct {
					Command string `json:"command"`
				} `json:"terminate"`
				Process struct {
					Started string `json:"started"`
				} `json:"process"`
			} `json:"findings"`
		} `json:"ports"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || len(doc.Ports) != 2 {
		t.Fatalf("unexpected document: %+v", doc)
	}
	p := doc.Ports[0]
	if p.State != "busy" || p.Findings[0].Terminate.Command != "docker stop db" || p.Findings[0].PID != 812 {
		t.Errorf("unexpected busy port: %+v", p)
	}
	if _, err := time.Parse(time.RFC3339, p.Findings[0].Process.Started); err != nil {
		t.Errorf("started is not RFC 3339: %v", err)
	}
	golden(t, "json_docker.golden", out)
}

func TestJSON_FreePort(t *testing.T) {
	golden(t, "json_free.golden", renderJSON(t, report.PortReport{Port: 3000}))
}

func TestJSON_OwnerNotVisible(t *testing.T) {
	golden(t, "json_owner_hidden.golden", renderJSON(t, adviceReport(), hiddenReport()))
}

func TestText_ControlCharactersInExternalStringsAreNeutralised(t *testing.T) {
	const esc = "\x1b[31m\x07evil\r\x9b"
	r := dockerReport()
	f := &r.Findings[0]
	f.Process.Name = "proc" + esc
	f.Explanation.Title = "title" + esc
	f.Explanation.Details = []detect.Detail{{Key: "key" + esc, Value: "value" + esc}}
	f.Explanation.Hints = []string{"hint" + esc}
	f.Explanation.Terminator = fakeTerminator("stop" + esc)
	f.Advice = []advise.Advice{{Level: advise.Warn, Text: "advice" + esc}}

	for _, c := range []Colors{{}, {Enabled: true}} {
		out := renderText(t, c, r)
		stripped := out
		if c.Enabled {
			for _, code := range []string{ansiGreen, ansiYellow, ansiCyan, ansiReset} {
				stripped = bytes.ReplaceAll(stripped, []byte(code), nil)
			}
		}
		for _, bad := range []string{"\x1b", "\x07", "\r", "\x9b"} {
			if bytes.Contains(stripped, []byte(bad)) {
				t.Errorf("colors=%v: output contains control character %q:\n%q", c.Enabled, bad, out)
			}
		}
		if !bytes.Contains(out, []byte("evil")) {
			t.Errorf("sanitising must keep the printable text:\n%q", out)
		}
	}
}

func timeWaitAdviceReport() report.PortReport {
	return report.PortReport{
		Port: 3000, TimeWaitOnly: true,
		Advice: []advise.Advice{{Level: advise.Info, Text: "Nobody is listening; the port is in TIME_WAIT. Wait for it to clear or bind with SO_REUSEADDR."}},
	}
}

func TestText_TimeWaitOnlyWithAdvice(t *testing.T) {
	golden(t, "text_time_wait_advice.golden", renderText(t, Colors{}, timeWaitAdviceReport()))
}

func TestJSON_TimeWaitOnlyWithAdvice(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, []report.PortReport{timeWaitAdviceReport()}); err != nil {
		t.Fatal(err)
	}
	golden(t, "json_time_wait_advice.golden", b.Bytes())
}
