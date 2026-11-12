package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

type fakeSockets struct {
	sockets []socket.Socket
	err     error
}

func (f fakeSockets) Sockets(context.Context) ([]socket.Socket, error) { return f.sockets, f.err }

type fakeProcs struct{}

func (fakeProcs) Read(_ context.Context, pid int) (proc.Process, error) {
	return proc.Process{PID: pid, Name: "node"}, nil
}

func listener(port uint16, pid int) socket.Socket {
	return socket.Socket{Port: port, Protocol: socket.TCP, Addr: netip.MustParseAddr("127.0.0.1"), State: socket.Listen, PID: pid}
}

func testDeps(c fakeSockets) (Deps, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return Deps{
		Sockets:  c,
		Procs:    fakeProcs{},
		Detector: detect.Generic(),
		Stdout:   &stdout,
		Stderr:   &stderr,
		Pid:      1234,
	}, &stdout, &stderr
}

func TestRun_BusyPortExit0(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000"}, Flags{})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if !strings.Contains(stdout.String(), ":3000") || !strings.Contains(stdout.String(), "node") {
		t.Errorf("stdout = %q, want an explanation of port 3000", stdout.String())
	}
}

func TestRun_FreePortExit1(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{})

	got := Run(context.Background(), d, []string{"3000"}, Flags{})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if !strings.Contains(stdout.String(), ":3000 ") || !strings.Contains(stdout.String(), "free") {
		t.Errorf("stdout = %q, want explicit port shown as free", stdout.String())
	}
}

func TestRun_MixedBusyAndFreeExit0(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000", "3001"}, Flags{})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if !strings.Contains(stdout.String(), ":3001 ") || !strings.Contains(stdout.String(), "free") {
		t.Errorf("stdout = %q, want free port 3001 listed", stdout.String())
	}
}

func TestRun_InvalidPortExit2(t *testing.T) {
	for _, arg := range []string{"abc", "70000", "0", "10-5"} {
		t.Run(arg, func(t *testing.T) {
			d, stdout, stderr := testDeps(fakeSockets{})

			got := Run(context.Background(), d, []string{"3000", arg}, Flags{})

			if got != 2 {
				t.Errorf("exit = %d, want 2", got)
			}
			if !strings.Contains(stderr.String(), arg) {
				t.Errorf("stderr = %q, want it to name %q", stderr.String(), arg)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestRun_NoArgsListsAllListening(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(8080, 7), listener(3000, 42)}})

	got := Run(context.Background(), d, nil, Flags{})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if i, j := strings.Index(out, "3000"), strings.Index(out, "8080"); i < 0 || j < 0 || i > j {
		t.Errorf("stdout = %q, want 3000 then 8080", out)
	}
}

func TestRun_NoArgsNothingListeningExit1(t *testing.T) {
	d, _, _ := testDeps(fakeSockets{})

	if got := Run(context.Background(), d, nil, Flags{}); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
}

func TestRun_RangeHidesFreePorts(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3002, 42)}})

	got := Run(context.Background(), d, []string{"3000-3005"}, Flags{})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "3002") {
		t.Errorf("stdout = %q, want busy port 3002", out)
	}
	if strings.Contains(out, "free") {
		t.Errorf("stdout = %q, want free ports hidden", out)
	}
}

func TestRun_RangeAllFreeExit1(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{})

	got := Run(context.Background(), d, []string{"3000-3005"}, Flags{})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if strings.Contains(stdout.String(), ":30") {
		t.Errorf("stdout = %q, want no per-port lines", stdout.String())
	}
}

func TestRun_ExplicitPortNextToRangeStaysVisible(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{})

	Run(context.Background(), d, []string{"22", "3000-3002"}, Flags{})

	out := stdout.String()
	if !strings.Contains(out, ":22 ") || !strings.Contains(out, "free") || strings.Contains(out, ":3000") {
		t.Errorf("stdout = %q, want only explicit port 22 shown", out)
	}
}

func TestRun_JSONFlag(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000", "3001"}, Flags{JSON: true})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	var doc struct {
		Version int `json:"version"`
		Ports   []struct {
			Port  int    `json:"port"`
			State string `json:"state"`
		} `json:"ports"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if doc.Version != 1 || len(doc.Ports) != 2 || doc.Ports[0].State != "busy" || doc.Ports[1].State != "free" {
		t.Errorf("doc = %+v, want version 1 with busy 3000 and free 3001", doc)
	}
}

func TestRun_JSONRangeHidesFreePorts(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{})

	got := Run(context.Background(), d, []string{"3000-3002"}, Flags{JSON: true})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	var doc struct {
		Version int   `json:"version"`
		Ports   []any `json:"ports"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if doc.Version != 1 || len(doc.Ports) != 0 {
		t.Errorf("doc = %+v, want version 1 and no ports", doc)
	}
}

func TestRun_CollectorErrorExit2(t *testing.T) {
	d, stdout, stderr := testDeps(fakeSockets{err: errors.New("lsof exploded")})

	got := Run(context.Background(), d, []string{"3000"}, Flags{})

	if got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "lsof exploded") {
		t.Errorf("stderr = %q, want collector error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRun_YesWithoutKillExit2(t *testing.T) {
	d, _, stderr := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000"}, Flags{Yes: true})

	if got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("stderr = %q, want it to mention --yes", stderr.String())
	}
}

func TestRun_NoColorWhenNotTTY(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	Run(context.Background(), d, []string{"3000"}, Flags{})

	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("stdout = %q, want no colour on a non-TTY", stdout.String())
	}
}

func TestRun_ColorOnTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	d.IsTTY = true

	Run(context.Background(), d, []string{"3000"}, Flags{})
	if !strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("stdout = %q, want colour on a TTY", stdout.String())
	}

	stdout.Reset()
	Run(context.Background(), d, []string{"3000"}, Flags{NoColor: true})
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("stdout = %q, want no colour with --no-color", stdout.String())
	}
}

func TestRootCmd_ParsesFlagsAndReturnsExitCode(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(22, 1)}})
	cmd, exit := newRootCmd(d)
	cmd.SetArgs([]string{"--json", "22"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if *exit != 0 || !strings.Contains(stdout.String(), "\"version\": 1") {
		t.Errorf("exit = %d, stdout = %q, want 0 and JSON", *exit, stdout.String())
	}
}

func TestRootCmd_KillFlagsAreParsed(t *testing.T) {
	d, _, stderr := testDeps(fakeSockets{})
	cmd, exit := newRootCmd(d)
	cmd.SetArgs([]string{"-k", "-y", "--force", "--dry-run", "3000"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if *exit != 1 {
		t.Errorf("exit = %d, want 1; stderr = %q", *exit, stderr.String())
	}
}

func TestRootCmd_YesWithoutKillExit2(t *testing.T) {
	d, _, _ := testDeps(fakeSockets{})
	cmd, exit := newRootCmd(d)
	cmd.SetArgs([]string{"--yes", "3000"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if *exit != 2 {
		t.Errorf("exit = %d, want 2", *exit)
	}
}

type fakeTerminator struct {
	err      error
	calls    int
	onTerm   func()
	describe string
}

func (f *fakeTerminator) Describe() string { return f.describe }
func (f *fakeTerminator) Terminate(context.Context) error {
	f.calls++
	if f.onTerm != nil {
		f.onTerm()
	}
	return f.err
}

// fixedDetector explains every socket with the given terminator.
type fixedDetector struct{ t kill.Terminator }

func (f fixedDetector) Detect(context.Context, socket.Socket, *proc.Process) (*detect.Explanation, bool) {
	return &detect.Explanation{Source: detect.SourceProcess, Title: "node", Terminator: f.t}, true
}

// freeingSockets reports a listener until free() is called.
type freeingSockets struct{ freed bool }

func (f *freeingSockets) Sockets(context.Context) ([]socket.Socket, error) {
	if f.freed {
		return nil, nil
	}
	return []socket.Socket{listener(3000, 42)}, nil
}

func killDeps(t kill.Terminator, c socket.Collector) (Deps, *bytes.Buffer) {
	d, _, stderr := testDeps(fakeSockets{})
	d.Sockets = c
	d.Detector = fixedDetector{t}
	return d, stderr
}

func TestRun_KillYesStopsAndExit0(t *testing.T) {
	c := &freeingSockets{}
	ft := &fakeTerminator{describe: "kill -TERM 42", onTerm: func() { c.freed = true }}
	d, _ := killDeps(ft, c)

	got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if ft.calls != 1 {
		t.Errorf("Terminate calls = %d, want 1", ft.calls)
	}
}

func TestRun_KillStillBusyExit3(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	d, stderr := killDeps(ft, &freeingSockets{})

	got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true})

	if got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if !strings.Contains(stderr.String(), "still busy") {
		t.Errorf("stderr = %q, want the still-busy error", stderr.String())
	}
}

func TestRun_KillTerminateErrorExit3(t *testing.T) {
	ft := &fakeTerminator{describe: "x", err: errors.New("boom")}
	d, _ := killDeps(ft, &freeingSockets{})

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true}); got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
}

func TestRun_KillWithoutTTYAndWithoutYesExit3(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	d, stderr := killDeps(ft, &freeingSockets{})

	got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true})

	if got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate calls = %d, want 0", ft.calls)
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("stderr = %q, want a hint about --yes", stderr.String())
	}
}

func TestRun_KillNeedsStdinTTYToo(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	d, _ := killDeps(ft, &freeingSockets{})
	d.IsTTY = true
	d.StdinIsTTY = false
	d.Stdin = strings.NewReader("y\n")

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true}); got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate calls = %d, want 0", ft.calls)
	}
}

func TestRun_KillPromptAccepted(t *testing.T) {
	c := &freeingSockets{}
	ft := &fakeTerminator{describe: "x", onTerm: func() { c.freed = true }}
	d, _ := killDeps(ft, c)
	d.IsTTY, d.StdinIsTTY = true, true
	d.Stdin = strings.NewReader("y\n")

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true}); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if ft.calls != 1 {
		t.Errorf("Terminate calls = %d, want 1", ft.calls)
	}
}

func TestRun_KillDeclinedIsNotAFailure(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	d, _ := killDeps(ft, &freeingSockets{})
	d.IsTTY, d.StdinIsTTY = true, true
	d.Stdin = strings.NewReader("n\n")

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true}); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate calls = %d, want 0", ft.calls)
	}
}

func TestRun_KillDryRunNeverTerminates(t *testing.T) {
	ft := &fakeTerminator{describe: "kill -TERM 42"}
	d, stderr := killDeps(ft, &freeingSockets{})

	got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, DryRun: true})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate calls = %d, want 0", ft.calls)
	}
	if !strings.Contains(stderr.String(), "kill -TERM 42") {
		t.Errorf("stderr = %q, want the command", stderr.String())
	}
}

func TestRun_KillWithoutTerminatorExit3(t *testing.T) {
	d, stderr := killDeps(nil, &freeingSockets{})

	got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true})

	if got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Errorf("stderr = %q, want a sudo hint", stderr.String())
	}
}

func TestRun_KillSkipsFreePorts(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	d, _ := killDeps(ft, &freeingSockets{freed: true})

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true}); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate calls = %d, want 0", ft.calls)
	}
}

func TestRun_KillTerminatesSharedCauseOnce(t *testing.T) {
	c := &freeingSockets{}
	ft := &fakeTerminator{describe: "kill -TERM 42", onTerm: func() { c.freed = true }}
	d, _ := killDeps(ft, c)
	d.Sockets = twoSockets{c}

	if got := Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true}); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if ft.calls != 1 {
		t.Errorf("Terminate calls = %d, want 1", ft.calls)
	}
}

// twoSockets lists the same process twice (IPv4 and IPv6) until freed.
type twoSockets struct{ c *freeingSockets }

func (s twoSockets) Sockets(ctx context.Context) ([]socket.Socket, error) {
	all, err := s.c.Sockets(ctx)
	if len(all) == 0 {
		return all, err
	}
	v6 := all[0]
	v6.Addr = netip.MustParseAddr("::1")
	return append(all, v6), err
}

func TestRun_NoArgsUsesOverview(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(8080, 7), listener(3000, 42)}})

	Run(context.Background(), d, nil, Flags{})

	if out := stdout.String(); !strings.Contains(out, "Details: portwhy 3000") || strings.Contains(out, ":3000") {
		t.Errorf("stdout = %q, want the overview with a details hint", out)
	}
}

func TestRun_RangeUsesOverview(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3002, 42)}})

	Run(context.Background(), d, []string{"3000-3005"}, Flags{})

	if !strings.Contains(stdout.String(), "Details: portwhy 3002") {
		t.Errorf("stdout = %q, want the overview", stdout.String())
	}
}

func TestRun_ExplicitPortUsesDetail(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	Run(context.Background(), d, []string{"3000"}, Flags{})

	if out := stdout.String(); !strings.Contains(out, ":3000") || strings.Contains(out, "Details:") {
		t.Errorf("stdout = %q, want the detail view", out)
	}
}

func TestRun_MixedArgsUseDetail(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42), listener(4005, 43)}})

	Run(context.Background(), d, []string{"3000", "4000-4010"}, Flags{})

	if out := stdout.String(); !strings.Contains(out, ":3000") || strings.Contains(out, "Details:") {
		t.Errorf("stdout = %q, want the detail view", out)
	}
}

func TestRun_KillWithoutPortsUsesDetail(t *testing.T) {
	d, stdout, stderr := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	Run(context.Background(), d, nil, Flags{Kill: true, DryRun: true})

	if out := stdout.String(); !strings.Contains(out, ":3000") || strings.Contains(out, "Details:") {
		t.Errorf("stdout = %q, want the detail view with --kill", out)
	}
	if n := strings.Count(stderr.String(), "kill 42"); n > 1 {
		t.Errorf("stderr = %q, want the stop announced once", stderr.String())
	}
}

type longCommandProcs struct{}

func (longCommandProcs) Read(_ context.Context, pid int) (proc.Process, error) {
	return proc.Process{PID: pid, Name: "node", Cmdline: []string{"node", strings.Repeat("x", 300)}}, nil
}

func TestRun_WidthShortensCommandUnlessWide(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	d.Procs = longCommandProcs{}
	d.Width = 80

	Run(context.Background(), d, []string{"3000"}, Flags{})
	short := stdout.String()
	stdout.Reset()
	Run(context.Background(), d, []string{"3000"}, Flags{Wide: true})

	if strings.Contains(short, strings.Repeat("x", 300)) || !strings.Contains(short, "…") {
		t.Errorf("stdout = %q, want the command shortened", short)
	}
	if !strings.Contains(stdout.String(), strings.Repeat("x", 300)) {
		t.Errorf("stdout = %q, want the full command with --wide", stdout.String())
	}
}

func TestRun_TermDumbDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	d.IsTTY = true

	Run(context.Background(), d, []string{"3000"}, Flags{})

	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("stdout = %q, want no colour for TERM=dumb", stdout.String())
	}
}

func TestRootCmd_WideFlagIsParsed(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	d.Procs = longCommandProcs{}
	d.Width = 80
	cmd, _ := newRootCmd(d)
	cmd.SetArgs([]string{"--wide", "3000"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout.String(), strings.Repeat("x", 300)) {
		t.Errorf("stdout = %q, want the full command with --wide", stdout.String())
	}
}

func TestRun_KillReportsStatusLinesAndAResultBox(t *testing.T) {
	c := &freeingSockets{}
	ft := &fakeTerminator{describe: "kill -TERM 42", onTerm: func() { c.freed = true }}
	d, stderr := killDeps(ft, c)

	Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, Yes: true})

	for _, want := range []string{"✓  Port 3000 is free.", " Result ", "Stopped"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("%q missing from stderr:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "\x1b") {
		t.Errorf("escape sequence although stderr is not a terminal: %q", stderr.String())
	}
}

func TestRun_KillDeclinedShowsLeftUntouched(t *testing.T) {
	d, stderr := killDeps(&fakeTerminator{describe: "x"}, &freeingSockets{})
	d.IsTTY, d.StdinIsTTY = true, true
	d.Stdin = strings.NewReader("n\n")

	Run(context.Background(), d, []string{"3000"}, Flags{Kill: true})

	if !strings.Contains(stderr.String(), "Port 3000 left untouched.") || !strings.Contains(stderr.String(), "Left untouched") {
		t.Errorf("declined run not reported:\n%s", stderr.String())
	}
}

func TestRun_KillDryRunPrintsNoResultBox(t *testing.T) {
	d, stderr := killDeps(&fakeTerminator{describe: "kill -TERM 42"}, &freeingSockets{})

	Run(context.Background(), d, []string{"3000"}, Flags{Kill: true, DryRun: true})

	if strings.Contains(stderr.String(), "Result") {
		t.Errorf("a dry run must not claim a result:\n%s", stderr.String())
	}
}

func TestRun_ExpandedSwitchesTheOverviewLayout(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	Run(context.Background(), d, nil, Flags{})
	compact := stdout.String()

	d, stdout, _ = testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	Run(context.Background(), d, nil, Flags{Expanded: true})

	if !strings.Contains(stdout.String(), " Summary ") || strings.Contains(compact, " Summary ") {
		t.Errorf("--expanded should add the summary box only in the airy layout\ncompact:\n%s\nexpanded:\n%s", compact, stdout.String())
	}
}

func TestRootCmd_ExpandedFlagIsParsed(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})
	cmd, _ := newRootCmd(d)
	cmd.SetArgs([]string{"--expanded"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), " Summary ") {
		t.Errorf("--expanded not applied:\n%s", stdout.String())
	}
}
