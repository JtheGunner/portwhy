package socket

import (
	"slices"
	"testing"
)

func TestParseNetstat_ListenAndTimeWait(t *testing.T) {
	got, err := parseNetstat(readFixture(t, "netstat_tcp.txt"))
	if err != nil {
		t.Fatalf("parseNetstat: %v", err)
	}

	for _, want := range []Socket{
		sock(3000, TCP, "0.0.0.0", Listen, 0),
		sock(22, TCP, "0.0.0.0", Listen, 0),
		sock(63295, TCP, "127.0.0.1", Listen, 0),
		sock(7679, TCP, "::1", Listen, 0),
		sock(54067, TCP, "0.0.0.0", Listen, 0),
		sock(65066, TCP, "127.0.0.1", TimeWait, 0),
		sock(62627, TCP, "192.0.2.44", TimeWait, 0),
	} {
		if !containsSocket(got, want) {
			t.Errorf("missing %v in %v", want, got)
		}
	}
}

func TestParseNetstat_IgnoresOtherStates(t *testing.T) {
	got, err := parseNetstat(readFixture(t, "netstat_tcp.txt"))
	if err != nil {
		t.Fatalf("parseNetstat: %v", err)
	}

	for _, s := range got {
		if s.Port == 65093 || s.Port == 65091 {
			t.Errorf("established connection reported as %v", s)
		}
	}
}

func TestParseNetstat_TruncatedIPv6AddressFallsBackToWildcard(t *testing.T) {
	got, err := parseNetstat(readFixture(t, "netstat_tcp.txt"))
	if err != nil {
		t.Fatalf("parseNetstat: %v", err)
	}

	if want := sock(6504, TCP, "::", TimeWait, 0); !containsSocket(got, want) {
		t.Errorf("missing %v in %v", want, got)
	}
}

func TestParseNetstat_UDPUnconnectedIsListen(t *testing.T) {
	got, err := parseNetstat(readFixture(t, "netstat_udp.txt"))
	if err != nil {
		t.Fatalf("parseNetstat: %v", err)
	}

	want := []Socket{
		sock(63102, UDP, "192.0.2.45", Listen, 0),
		sock(59332, UDP, "0.0.0.0", Listen, 0),
		sock(5353, UDP, "0.0.0.0", Listen, 0),
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseNetstat() = %v, want %v", got, want)
	}
}

func TestParseNetstat_EmptyInput(t *testing.T) {
	got, err := parseNetstat(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("parseNetstat(nil) = %v, %v; want no sockets and no error", got, err)
	}
}

func TestParseNetstat_MalformedPort(t *testing.T) {
	in := "tcp4       0      0  *.http                 *.*                    LISTEN\n"
	if got, err := parseNetstat([]byte(in)); err == nil {
		t.Errorf("parseNetstat() = %v, want an error", got)
	}
}
