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
	if !strings.Contains(stdout.String(), ":3000  free") {
		t.Errorf("stdout = %q, want explicit port shown as free", stdout.String())
	}
}

func TestRun_MixedBusyAndFreeExit0(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000", "3001"}, Flags{})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if !strings.Contains(stdout.String(), ":3001  free") {
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
	if i, j := strings.Index(out, ":3000"), strings.Index(out, ":8080"); i < 0 || j < 0 || i > j {
		t.Errorf("stdout = %q, want :3000 then :8080", out)
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
	if !strings.Contains(out, ":3002") {
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
	if !strings.Contains(out, ":22  free") || strings.Contains(out, ":3000") {
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
