package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func renderPlain(t *testing.T, reports []report.PortReport) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Plain(&buf, reports); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPlain_Golden(t *testing.T) {
	golden(t, "plain.golden", []byte(renderPlain(t, overviewFixture())))
}

func TestPlain_OneLineWithSixTabSeparatedColumnsPerSocket(t *testing.T) {
	out := renderPlain(t, overviewFixture())

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 15 {
		t.Errorf("got %d lines, want one per socket (15):\n%s", len(lines), out)
	}
	for _, l := range lines {
		if n := len(strings.Split(l, "\t")); n != 6 {
			t.Errorf("line %q has %d columns, want 6", l, n)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("output contains colour codes:\n%s", out)
	}
}

func TestPlain_HiddenOwnerGetsItsOwnLine(t *testing.T) {
	out := renderPlain(t, reportOf(
		finding(22, socket.TCP, "0.0.0.0", 0, "", detect.SourceProcess, "owner not visible"),
		finding(22, socket.TCP, "::", 0, "", detect.SourceProcess, "owner not visible"),
	))

	want := "22\ttcp\t0.0.0.0\t0\tprocess\t-\n22\ttcp\t::\t0\tprocess\t-\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPlain_OrdersByPortProtocolAddress(t *testing.T) {
	out := renderPlain(t, reportOf(
		finding(80, socket.UDP, "0.0.0.0", 1, "b", detect.SourceProcess, "b"),
		finding(80, socket.TCP, "127.0.0.1", 2, "a", detect.SourceProcess, "a"),
		finding(80, socket.TCP, "0.0.0.0", 3, "c", detect.SourceProcess, "c"),
		finding(9, socket.TCP, "0.0.0.0", 4, "d", detect.SourceProcess, "d"),
	))

	want := "9\ttcp\t0.0.0.0\t4\tprocess\td\n" +
		"80\ttcp\t0.0.0.0\t3\tprocess\tc\n" +
		"80\ttcp\t127.0.0.1\t2\tprocess\ta\n" +
		"80\tudp\t0.0.0.0\t1\tprocess\tb\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestPlain_KeepsLongNamesAndNeutralisesTabs(t *testing.T) {
	long := strings.Repeat("x", 200)
	out := renderPlain(t, reportOf(finding(80, socket.TCP, "0.0.0.0", 1, long+"\tsplit\nname", detect.SourceProcess, "t")))

	want := "80\ttcp\t0.0.0.0\t1\tprocess\t" + long + " split name\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPlain_NoSocketsWritesNothing(t *testing.T) {
	if out := renderPlain(t, nil); out != "" {
		t.Errorf("got %q, want empty output", out)
	}
}
