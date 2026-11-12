package socket

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "proc", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestParseProcNet_TCPListen(t *testing.T) {
	got, err := parseProcNet(openFixture(t, "net_tcp"), TCP, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []rawSocket{
		{Addr: netip.MustParseAddr("127.0.0.1"), Port: 3000, State: Listen, Inode: 12345},
		{Addr: netip.MustParseAddr("0.0.0.0"), Port: 80, State: Listen, Inode: 23456},
		{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8080, State: TimeWait, Inode: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseProcNet() = %v, want %v (ESTABLISHED row must be ignored)", got, want)
	}
}

func TestParseProcNet_IPv6Wildcard(t *testing.T) {
	got, err := parseProcNet(openFixture(t, "net_tcp6"), TCP, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []rawSocket{
		{Addr: netip.MustParseAddr("::"), Port: 3000, State: Listen, Inode: 12345},
		{Addr: netip.MustParseAddr("::1"), Port: 8080, State: Listen, Inode: 45678},
		{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8081, State: Listen, Inode: 56789},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseProcNet() = %v, want %v", got, want)
	}
}

func TestParseProcNet_TimeWait(t *testing.T) {
	got, err := parseProcNet(openFixture(t, "net_tcp"), TCP, false)
	if err != nil {
		t.Fatal(err)
	}
	if last := got[len(got)-1]; last.State != TimeWait || last.Port != 8080 {
		t.Errorf("last socket = %+v, want time_wait on port 8080", last)
	}
}

func TestParseProcNet_UDPListen(t *testing.T) {
	got, err := parseProcNet(openFixture(t, "net_udp"), UDP, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []rawSocket{{Addr: netip.MustParseAddr("0.0.0.0"), Port: 53, State: Listen, Inode: 78901}}
	if !slices.Equal(got, want) {
		t.Errorf("parseProcNet() = %v, want %v (connected UDP row must be ignored)", got, want)
	}
}

func TestParseProcNet_MalformedLines(t *testing.T) {
	const header = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	tests := map[string]string{
		"truncated line": "   0: 0100007F:0BB8 00000000:0000 0A\n",
		"bad address":    "   0: ZZ00007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1\n",
		"bad port":       "   0: 0100007F:XYZ 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1\n",
		"bad inode":      "   0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 abc 1\n",
		"short ipv4":     "   0: 0100:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1\n",
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseProcNet(strings.NewReader(header+line), TCP, false); err == nil {
				t.Error("parseProcNet() error = nil, want error for malformed line")
			}
		})
	}
}

func TestParseProcNet_IPv6AddressLengthMismatch(t *testing.T) {
	line := "   0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1\n"
	if _, err := parseProcNet(strings.NewReader("header\n"+line), TCP, true); err == nil {
		t.Error("parseProcNet() error = nil, want error for IPv4-length address in tcp6")
	}
}

// writeProcTree builds a fake /proc under a temp dir with the net fixtures and
// returns its root. Existing fixture files are copied; tcp6/udp6 are optional.
func writeProcTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join("testdata", "proc", "net_"+name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "net", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func addSocketFD(t *testing.T, root string, pid int, fd int, inode uint64) string {
	t.Helper()
	fdDir := filepath.Join(root, strconv.Itoa(pid), "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := "socket:[" + strconv.FormatUint(inode, 10) + "]"
	if err := os.Symlink(target, filepath.Join(fdDir, strconv.Itoa(fd))); err != nil {
		t.Fatal(err)
	}
	return fdDir
}

func findSocket(socks []Socket, proto Protocol, port uint16, state State) (Socket, bool) {
	for _, s := range socks {
		if s.Protocol == proto && s.Port == port && s.State == state {
			return s, true
		}
	}
	return Socket{}, false
}

func TestCollector_MapsInodeToPID(t *testing.T) {
	root := writeProcTree(t, "tcp", "udp")
	addSocketFD(t, root, 4242, 3, 12345)
	// Non-socket fds and non-numeric entries are ignored.
	if err := os.Symlink("/dev/null", filepath.Join(root, "4242", "fd", "0")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := newProcCollector(root).Sockets(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	s, ok := findSocket(got, TCP, 3000, Listen)
	if !ok || s.PID != 4242 {
		t.Errorf("port 3000 socket = %+v (found %v), want PID 4242", s, ok)
	}
	if s, ok := findSocket(got, TCP, 80, Listen); !ok || s.PID != 0 {
		t.Errorf("port 80 socket = %+v (found %v), want PID 0 for unowned inode", s, ok)
	}
	if _, ok := findSocket(got, TCP, 8080, TimeWait); !ok {
		t.Error("time_wait socket missing")
	}
}

func TestCollector_UnreadableFDDirGivesPIDZero(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read every fd directory")
	}
	root := writeProcTree(t, "tcp", "udp")
	fdDir := addSocketFD(t, root, 4242, 3, 12345)
	if err := os.Chmod(fdDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(fdDir, 0o755) })

	got, err := newProcCollector(root).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets() error = %v, want nil for unreadable fd dir", err)
	}
	s, ok := findSocket(got, TCP, 3000, Listen)
	if !ok || s.PID != 0 {
		t.Errorf("port 3000 socket = %+v (found %v), want visible with PID 0", s, ok)
	}
}

func TestCollector_DualStackIsOneSocket(t *testing.T) {
	root := writeProcTree(t, "tcp", "tcp6", "udp")
	// Inode 12345 is 127.0.0.1:3000 in tcp and :::3000 in tcp6; use the
	// wildcard pair instead by rewriting tcp so both families are wildcards.
	v4 := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1\n"
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(v4), 0o644); err != nil {
		t.Fatal(err)
	}
	addSocketFD(t, root, 4242, 3, 12345)

	got, err := newProcCollector(root).Sockets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var onPort3000 []Socket
	for _, s := range got {
		if s.Port == 3000 {
			onPort3000 = append(onPort3000, s)
		}
	}
	if len(onPort3000) != 1 || onPort3000[0].Addr != netip.MustParseAddr("0.0.0.0") || onPort3000[0].PID != 4242 {
		t.Errorf("port 3000 sockets = %v, want one 0.0.0.0 socket with PID 4242", onPort3000)
	}
}

func TestCollector_MissingIPv6FilesAreNotAnError(t *testing.T) {
	root := writeProcTree(t, "tcp", "udp")

	got, err := newProcCollector(root).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets() error = %v, want nil when tcp6/udp6 are absent", err)
	}
	if _, ok := findSocket(got, UDP, 53, Listen); !ok {
		t.Error("udp listener missing")
	}
}

func TestCollector_MissingIPv4FileIsAnError(t *testing.T) {
	root := writeProcTree(t)
	if _, err := newProcCollector(root).Sockets(context.Background()); err == nil {
		t.Error("Sockets() error = nil, want error when /proc/net/tcp is missing")
	}
}

func TestCollector_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newProcCollector(writeProcTree(t, "tcp", "udp")).Sockets(ctx); err == nil {
		t.Error("Sockets() error = nil, want context error")
	}
}
