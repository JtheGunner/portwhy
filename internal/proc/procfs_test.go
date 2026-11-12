package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "proc", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseStat_Fixture(t *testing.T) {
	name, ppid, ticks, err := parseStat(readFixture(t, "stat"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "node" || ppid != 4100 || ticks != 987654 {
		t.Fatalf("got %q %d %d", name, ppid, ticks)
	}
}

func TestParseStat_CommWithSpacesAndParens(t *testing.T) {
	line := "1 (a b) c) S 0 1 1 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 555 0 0 0\n"
	name, ppid, ticks, err := parseStat([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if name != "a b) c" || ppid != 0 || ticks != 555 {
		t.Fatalf("got %q %d %d", name, ppid, ticks)
	}
}

func TestParseStat_Malformed(t *testing.T) {
	for _, in := range []string{"", "1 node S 0", "1 (node) S 0 1", "1 (node) S x 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 5 0", "1 (node) S 0 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 zz 0"} {
		_, _, _, err := parseStat([]byte(in))
		if err == nil {
			t.Errorf("parseStat(%q) returned no error", in)
		}
		if errors.Is(err, ErrNotFound) {
			t.Errorf("parseStat(%q) must not return ErrNotFound", in)
		}
	}
}

func TestParseCmdline_NULSeparated(t *testing.T) {
	got := parseCmdline(readFixture(t, "cmdline"))
	want := []string{"node", "/home/dev/app/server.js", "--port", "3000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseCmdline_EmptyForKernelThreads(t *testing.T) {
	if got := parseCmdline(nil); len(got) != 0 {
		t.Fatalf("got %q, want none", got)
	}
}

func TestParseCgroup_V2(t *testing.T) {
	got := parseCgroup(readFixture(t, "cgroup"))
	want := "/user.slice/user-1000.slice/user@1000.service/app.slice/app-node.service"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseCgroup_V1FallsBackToFirstLine(t *testing.T) {
	got := parseCgroup(readFixture(t, "cgroup_v1"))
	if got != "/user.slice/user-1000.slice" {
		t.Fatalf("got %q", got)
	}
	if got := parseCgroup(nil); got != "" {
		t.Fatalf("empty input: got %q", got)
	}
}

func TestParseUID(t *testing.T) {
	uid, err := parseUID(readFixture(t, "status"))
	if err != nil || uid != "1000" {
		t.Fatalf("got %q, %v", uid, err)
	}
	if _, err := parseUID([]byte("Name:\tx\n")); err == nil {
		t.Fatal("expected error for missing Uid line")
	}
}

func TestParseBootTime(t *testing.T) {
	got, err := parseBootTime(readFixture(t, "stat_system"))
	if err != nil || got != 1700000000 {
		t.Fatalf("got %d, %v", got, err)
	}
	if _, err := parseBootTime([]byte("cpu 1\n")); err == nil {
		t.Fatal("expected error for missing btime")
	}
}

// fakeProc writes a minimal /proc tree and returns its root.
type fakeProc struct {
	root string
	t    *testing.T
}

func newFakeProc(t *testing.T) fakeProc {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "stat"), readFixture(t, "stat_system"), 0o644); err != nil {
		t.Fatal(err)
	}
	return fakeProc{root: root, t: t}
}

func (f fakeProc) add(pid, ppid int, name, uid string, ticks uint64) {
	f.t.Helper()
	dir := filepath.Join(f.root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	files := map[string]string{
		"stat":    fmt.Sprintf("%d (%s) S %d %d %d 0 -1 4194560 0 0 0 0 1 1 0 0 20 0 1 0 %d 1000 100 0\n", pid, name, ppid, pid, pid, ticks),
		"status":  fmt.Sprintf("Name:\t%s\nUid:\t%s\t%s\t%s\t%s\n", name, uid, uid, uid, uid),
		"cmdline": name + "\x00--flag\x00",
		"cgroup":  "0::/system.slice/" + name + ".service\n",
	}
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := os.Symlink("/work/"+name, filepath.Join(dir, "cwd")); err != nil {
		f.t.Fatal(err)
	}
}

func testUsers(uid string) (string, error) {
	if uid == "1000" {
		return "dev", nil
	}
	return "", errors.New("unknown user")
}

func newTestReader(root string) *procfsReader {
	return &procfsReader{root: root, clockTicks: 100, lookupUser: testUsers, readlink: os.Readlink}
}

func TestReader_ReadsAncestors(t *testing.T) {
	f := newFakeProc(t)
	f.add(1, 0, "init", "0", 1)
	f.add(10, 1, "zsh", "1000", 100)
	f.add(20, 10, "npm", "1000", 200)
	f.add(30, 20, "node", "1000", 31000)

	p, err := newTestReader(f.root).Read(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if p.PID != 30 || p.PPID != 20 || p.Name != "node" {
		t.Fatalf("basic fields: %+v", p)
	}
	if !reflect.DeepEqual(p.Cmdline, []string{"node", "--flag"}) {
		t.Errorf("cmdline: %q", p.Cmdline)
	}
	if p.User != "dev" {
		t.Errorf("user: %q", p.User)
	}
	if p.Cwd != "/work/node" {
		t.Errorf("cwd: %q", p.Cwd)
	}
	if p.Cgroup != "/system.slice/node.service" {
		t.Errorf("cgroup: %q", p.Cgroup)
	}
	if want := time.Unix(1700000000+310, 0); !p.Started.Equal(want) {
		t.Errorf("started: %v, want %v", p.Started, want)
	}
	want := []Ancestor{{20, "npm"}, {10, "zsh"}, {1, "init"}}
	if !reflect.DeepEqual(p.Ancestors, want) {
		t.Errorf("ancestors: %v, want %v", p.Ancestors, want)
	}
}

func TestReader_NotFound(t *testing.T) {
	f := newFakeProc(t)
	_, err := newTestReader(f.root).Read(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestReader_PermissionDeniedCwdIsEmptyNotError(t *testing.T) {
	f := newFakeProc(t)
	f.add(1, 0, "init", "0", 1)
	f.add(10, 1, "zsh", "1000", 100)
	r := newTestReader(f.root)
	r.readlink = func(string) (string, error) {
		return "", &os.PathError{Op: "readlink", Path: "cwd", Err: os.ErrPermission}
	}
	p, err := r.Read(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if p.Cwd != "" {
		t.Fatalf("cwd: %q, want empty", p.Cwd)
	}
}

func TestReader_PermissionDeniedOnStat(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}
	f := newFakeProc(t)
	f.add(10, 1, "zsh", "1000", 100)
	path := filepath.Join(f.root, "10", "stat")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	_, err := newTestReader(f.root).Read(context.Background(), 10)
	if !errors.Is(err, ErrPermission) {
		t.Fatalf("got %v, want ErrPermission", err)
	}
}

func TestReader_EmptyCmdlineKeepsNameFromStat(t *testing.T) {
	f := newFakeProc(t)
	f.add(1, 0, "init", "0", 1)
	f.add(2, 0, "kthreadd", "0", 1)
	if err := os.WriteFile(filepath.Join(f.root, "2", "cmdline"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := newTestReader(f.root).Read(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "kthreadd" || len(p.Cmdline) != 0 {
		t.Fatalf("name %q cmdline %q", p.Name, p.Cmdline)
	}
}

func TestReader_NumericUIDFallback(t *testing.T) {
	f := newFakeProc(t)
	f.add(5, 1, "svc", "4711", 10)
	p, err := newTestReader(f.root).Read(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if p.User != "4711" {
		t.Fatalf("user: %q", p.User)
	}
}

func TestReader_AncestorDepthLimit(t *testing.T) {
	f := newFakeProc(t)
	f.add(1, 0, "init", "0", 1)
	const chain = 100
	for pid := 2; pid <= chain; pid++ {
		f.add(pid, pid-1, "p", "1000", 10)
	}
	p, err := newTestReader(f.root).Read(context.Background(), chain)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ancestors) != maxAncestorDepth {
		t.Fatalf("got %d ancestors, want %d", len(p.Ancestors), maxAncestorDepth)
	}
	if p.Ancestors[0].PID != chain-1 {
		t.Errorf("first ancestor: %d", p.Ancestors[0].PID)
	}
}

func TestReader_AncestorCycleTerminates(t *testing.T) {
	f := newFakeProc(t)
	f.add(10, 20, "a", "1000", 10)
	f.add(20, 10, "b", "1000", 10)
	p, err := newTestReader(f.root).Read(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []Ancestor{{20, "b"}}
	if !reflect.DeepEqual(p.Ancestors, want) {
		t.Fatalf("ancestors: %v, want %v", p.Ancestors, want)
	}
}

func TestReader_MissingAncestorEndsWalk(t *testing.T) {
	f := newFakeProc(t)
	f.add(10, 99, "a", "1000", 10)
	p, err := newTestReader(f.root).Read(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ancestors) != 0 {
		t.Fatalf("ancestors: %v", p.Ancestors)
	}
}

func TestReader_MalformedStatIsNotNotFound(t *testing.T) {
	f := newFakeProc(t)
	f.add(10, 1, "a", "1000", 10)
	if err := os.WriteFile(filepath.Join(f.root, "10", "stat"), []byte("10 (a"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := newTestReader(f.root).Read(context.Background(), 10)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}
