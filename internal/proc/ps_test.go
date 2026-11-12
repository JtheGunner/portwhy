package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/execx"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "darwin", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// exitError returns a real *exec.ExitError for a command that exits with code,
// wrapped the way execx.Runner wraps it.
func exitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected an exit error, got %v", err)
	}
	return fmt.Errorf("sh -c: %w", err)
}

func psKey(pid int) string {
	return execx.Key("ps", "-o", "pid=,ppid=,user=,lstart=,command=", "-p", fmt.Sprint(pid))
}

func commKey(pid int) string {
	return execx.Key("ps", "-o", "comm=", "-p", fmt.Sprint(pid))
}

func lsofKey(pid int) string {
	return execx.Key("lsof", "-a", "-p", fmt.Sprint(pid), "-d", "cwd", "-Fn")
}

func psLine(pid, ppid int, command string) []byte {
	return []byte(fmt.Sprintf("%5d %5d devuser Thu Nov 12 02:36:39 2026 %s\n", pid, ppid, command))
}

func TestParsePS_ParsesFields(t *testing.T) {
	p, err := parsePS(fixture(t, "ps_one.txt"), "")
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}

	if p.PID != 37550 || p.PPID != 12887 {
		t.Errorf("PID/PPID = %d/%d, want 37550/12887", p.PID, p.PPID)
	}
	if p.User != "devuser" {
		t.Errorf("User = %q, want devuser", p.User)
	}
	if p.Name != "node" {
		t.Errorf("Name = %q, want node", p.Name)
	}
	wantCmd := []string{"/usr/local/bin/node", "/home/devuser/app/server.js", "--port", "3000"}
	if !slices.Equal(p.Cmdline, wantCmd) {
		t.Errorf("Cmdline = %v, want %v", p.Cmdline, wantCmd)
	}
	wantStart := time.Date(2026, time.November, 12, 2, 36, 39, 0, time.Local)
	if !p.Started.Equal(wantStart) {
		t.Errorf("Started = %v, want %v", p.Started, wantStart)
	}
}

func TestParsePS_CommandWithSpaces(t *testing.T) {
	p, err := parsePS(fixture(t, "ps_spaces.txt"), "")
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}

	// Space-padded day of month ("Nov  7") must parse.
	wantStart := time.Date(2026, time.November, 7, 9, 5, 1, 0, time.Local)
	if !p.Started.Equal(wantStart) {
		t.Errorf("Started = %v, want %v", p.Started, wantStart)
	}
	// The split is lossy: a path containing spaces falls apart.
	wantCmd := []string{"/Applications/My", "App.app/Contents/MacOS/My", "App", "--flag"}
	if !slices.Equal(p.Cmdline, wantCmd) {
		t.Errorf("Cmdline = %v, want %v", p.Cmdline, wantCmd)
	}
	if p.Name != "My" {
		t.Errorf("Name = %q, want My", p.Name)
	}
}

func TestParsePS_NameWithSpaces(t *testing.T) {
	tests := map[string]struct {
		fixture  string
		comm     string
		wantName string
		wantCmd  []string
	}{
		"spaces": {
			"ps_spaces.txt", "/Applications/My App.app/Contents/MacOS/My App", "My App",
			[]string{"/Applications/My App.app/Contents/MacOS/My App", "--flag"},
		},
		"special characters": {
			"ps_special.txt",
			"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)",
			"Google Chrome Helper (Renderer)",
			[]string{"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)", "--type=renderer"},
		},
		"no spaces unchanged": {
			"ps_one.txt", "/usr/local/bin/node", "node",
			[]string{"/usr/local/bin/node", "/home/devuser/app/server.js", "--port", "3000"},
		},
		"comm not a prefix falls back": {
			"ps_one.txt", "/other/path", "node",
			[]string{"/usr/local/bin/node", "/home/devuser/app/server.js", "--port", "3000"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := parsePS(fixture(t, tt.fixture), tt.comm)
			if err != nil {
				t.Fatalf("parsePS: %v", err)
			}
			if p.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", p.Name, tt.wantName)
			}
			if !slices.Equal(p.Cmdline, tt.wantCmd) {
				t.Errorf("Cmdline = %v, want %v", p.Cmdline, tt.wantCmd)
			}
		})
	}
}

func TestParsePSComm(t *testing.T) {
	if got := parsePSComm([]byte("/Applications/LM Studio.app/Contents/MacOS/LM Studio\n")); got != "/Applications/LM Studio.app/Contents/MacOS/LM Studio" {
		t.Errorf("comm = %q", got)
	}
	if got := parsePSComm(nil); got != "" {
		t.Errorf("comm of empty output = %q, want empty", got)
	}
}

func TestDarwinReader_NameWithSpaces(t *testing.T) {
	comm := "/Applications/LM Studio.app/Contents/MacOS/LM Studio"
	f := &execx.Fake{Responses: map[string][]byte{
		psKey(70):   psLine(70, 1, comm+" --port 1234"),
		commKey(70): []byte(comm + "\n"),
		lsofKey(70): nil,
		psKey(1):    psLine(1, 0, "/sbin/launchd"),
		lsofKey(1):  nil,
	}}
	p, err := newPSReader(f).Read(context.Background(), 70)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if p.Name != "LM Studio" {
		t.Errorf("Name = %q, want %q", p.Name, "LM Studio")
	}
}

func TestParsePS_Malformed(t *testing.T) {
	tests := map[string]string{
		"too few fields": "12 1 devuser Thu Nov 12\n",
		"bad pid":        "abc 1 devuser Thu Nov 12 02:36:39 2026 /bin/sh\n",
		"bad ppid":       "12 x devuser Thu Nov 12 02:36:39 2026 /bin/sh\n",
		"bad lstart":     "12 1 devuser Thu Foo 12 02:36:39 2026 /bin/sh\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parsePS([]byte(in), "")
			if err == nil {
				t.Fatal("parsePS returned nil error")
			}
			if errors.Is(err, ErrNotFound) {
				t.Errorf("error %v must not be ErrNotFound", err)
			}
		})
	}
}

func TestParsePS_EmptyIsNotFound(t *testing.T) {
	if _, err := parsePS([]byte("  \n"), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestParseLsofCwd(t *testing.T) {
	if got := parseLsofCwd(fixture(t, "lsof_cwd.txt")); got != "/Users/devuser/app" {
		t.Errorf("cwd = %q, want /Users/devuser/app", got)
	}
	if got := parseLsofCwd(nil); got != "" {
		t.Errorf("cwd of empty output = %q, want empty", got)
	}
}

func TestDarwinReader_Read(t *testing.T) {
	f := &execx.Fake{Responses: map[string][]byte{
		psKey(37550):   fixture(t, "ps_one.txt"),
		lsofKey(37550): fixture(t, "lsof_cwd.txt"),
		psKey(12887):   psLine(12887, 1, "/bin/zsh -l"),
		lsofKey(12887): nil,
		psKey(1):       psLine(1, 0, "/sbin/launchd"),
	}}

	p, err := newPSReader(f).Read(context.Background(), 37550)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if p.Cwd != "/Users/devuser/app" {
		t.Errorf("Cwd = %q", p.Cwd)
	}
	want := []Ancestor{{PID: 12887, Name: "zsh"}, {PID: 1, Name: "launchd"}}
	if !slices.Equal(p.Ancestors, want) {
		t.Errorf("Ancestors = %v, want %v", p.Ancestors, want)
	}
}

func TestDarwinReader_NotFound(t *testing.T) {
	tests := map[string]*execx.Fake{
		"exit 0 with empty output": {Responses: map[string][]byte{psKey(99): nil}},
		"exit 1":                   {Errors: map[string]error{psKey(99): exitError(t, 1)}},
	}
	for name, f := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newPSReader(f).Read(context.Background(), 99)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestDarwinReader_OtherPSFailureIsNotNotFound(t *testing.T) {
	tests := map[string]error{
		"exit 2":  exitError(t, 2),
		"missing": &exec.Error{Name: "ps", Err: exec.ErrNotFound},
		"timeout": fmt.Errorf("ps: %w", context.DeadlineExceeded),
	}
	for name, runErr := range tests {
		t.Run(name, func(t *testing.T) {
			f := &execx.Fake{Errors: map[string]error{psKey(99): runErr}}
			_, err := newPSReader(f).Read(context.Background(), 99)
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want a non-ErrNotFound error", err)
			}
			if !errors.Is(err, runErr) {
				t.Errorf("error %v does not wrap the cause", err)
			}
		})
	}
}

func TestDarwinReader_MalformedLstartIsNotNotFound(t *testing.T) {
	f := &execx.Fake{Responses: map[string][]byte{
		psKey(5): []byte("5 1 devuser Thu Foo 12 02:36:39 2026 /bin/sh\n"),
	}}
	_, err := newPSReader(f).Read(context.Background(), 5)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want a non-ErrNotFound error", err)
	}
}

func TestDarwinReader_LsofFailureKeepsCwdEmpty(t *testing.T) {
	f := &execx.Fake{
		Responses: map[string][]byte{psKey(1): psLine(1, 0, "/sbin/launchd")},
		Errors:    map[string]error{lsofKey(1): exitError(t, 1)},
	}
	p, err := newPSReader(f).Read(context.Background(), 1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if p.Cwd != "" {
		t.Errorf("Cwd = %q, want empty", p.Cwd)
	}
}

func TestDarwinReader_Ancestors(t *testing.T) {
	f := &execx.Fake{Responses: map[string][]byte{
		psKey(300): psLine(300, 200, "/usr/local/bin/node app.js"),
		psKey(200): psLine(200, 100, "npm run dev"),
		psKey(100): psLine(100, 1, "-zsh"),
		psKey(1):   psLine(1, 0, "/sbin/launchd"),
	}}
	for _, pid := range []int{300, 200, 100, 1} {
		f.Responses[lsofKey(pid)] = nil
	}

	p, err := newPSReader(f).Read(context.Background(), 300)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []Ancestor{{200, "npm"}, {100, "-zsh"}, {1, "launchd"}}
	if !slices.Equal(p.Ancestors, want) {
		t.Errorf("Ancestors = %v, want %v", p.Ancestors, want)
	}
}

func TestDarwinReader_MissingAncestorEndsWalk(t *testing.T) {
	f := &execx.Fake{
		Responses: map[string][]byte{
			psKey(300): psLine(300, 200, "node"),
			psKey(200): psLine(200, 100, "npm"),
		},
		Errors: map[string]error{psKey(100): exitError(t, 1)},
	}
	p, err := newPSReader(f).Read(context.Background(), 300)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := []Ancestor{{200, "npm"}}; !slices.Equal(p.Ancestors, want) {
		t.Errorf("Ancestors = %v, want %v", p.Ancestors, want)
	}
}

func TestDarwinReader_AncestorCycleTerminates(t *testing.T) {
	f := &execx.Fake{Responses: map[string][]byte{
		psKey(10): psLine(10, 20, "a"),
		psKey(20): psLine(20, 30, "b"),
		psKey(30): psLine(30, 20, "c"),
	}}
	p, err := newPSReader(f).Read(context.Background(), 10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := []Ancestor{{20, "b"}, {30, "c"}}; !slices.Equal(p.Ancestors, want) {
		t.Errorf("Ancestors = %v, want %v", p.Ancestors, want)
	}
}

func TestDarwinReader_AncestorDepthLimit(t *testing.T) {
	f := &execx.Fake{Responses: map[string][]byte{}}
	const chain = 100
	for pid := 1000; pid < 1000+chain; pid++ {
		f.Responses[psKey(pid)] = psLine(pid, pid+1, "p")
	}
	p, err := newPSReader(f).Read(context.Background(), 1000)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(p.Ancestors) != maxPSAncestorDepth || maxPSAncestorDepth != 32 {
		t.Errorf("len(Ancestors) = %d, want 32", len(p.Ancestors))
	}
}
