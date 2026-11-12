package socket

import (
	"net/netip"
	"slices"
	"testing"
)

func sock(port uint16, proto Protocol, addr string, state State, pid int) Socket {
	return Socket{Port: port, Protocol: proto, Addr: netip.MustParseAddr(addr), State: state, PID: pid}
}

func TestDedupe_DualStackCollapses(t *testing.T) {
	in := []Socket{
		sock(3000, TCP, "::", Listen, 42),
		sock(3000, TCP, "0.0.0.0", Listen, 42),
	}

	got := Dedupe(in)

	want := []Socket{sock(3000, TCP, "0.0.0.0", Listen, 42)}
	if !slices.Equal(got, want) {
		t.Errorf("Dedupe() = %v, want %v", got, want)
	}
}

func TestDedupe_DifferentPIDsKept(t *testing.T) {
	in := []Socket{
		sock(3000, TCP, "0.0.0.0", Listen, 1),
		sock(3000, TCP, "::", Listen, 2),
	}

	got := Dedupe(in)

	if len(got) != 2 {
		t.Fatalf("Dedupe() = %v, want both sockets kept", got)
	}
}

func TestDedupe_DifferentProtocolsKept(t *testing.T) {
	in := []Socket{
		sock(53, TCP, "0.0.0.0", Listen, 7),
		sock(53, UDP, "::", Listen, 7),
	}

	if got := Dedupe(in); len(got) != 2 {
		t.Fatalf("Dedupe() = %v, want TCP and UDP kept", got)
	}
}

func TestDedupe_DifferentStatesKept(t *testing.T) {
	in := []Socket{
		sock(3000, TCP, "0.0.0.0", Listen, 0),
		sock(3000, TCP, "::", TimeWait, 0),
	}

	if got := Dedupe(in); len(got) != 2 {
		t.Fatalf("Dedupe() = %v, want listen and time_wait kept", got)
	}
}

func TestDedupe_LoopbackPairNotCollapsed(t *testing.T) {
	in := []Socket{
		sock(3000, TCP, "127.0.0.1", Listen, 9),
		sock(3000, TCP, "::1", Listen, 9),
	}

	if got := Dedupe(in); len(got) != 2 {
		t.Fatalf("Dedupe() = %v, want loopback IPv4 and IPv6 kept separate", got)
	}
}

func TestDedupe_LoneIPv6WildcardKept(t *testing.T) {
	in := []Socket{sock(3000, TCP, "::", Listen, 5)}

	got := Dedupe(in)

	if !slices.Equal(got, in) {
		t.Errorf("Dedupe() = %v, want %v", got, in)
	}
}

func TestDedupe_SortsByPortProtocolAddress(t *testing.T) {
	in := []Socket{
		sock(8080, TCP, "127.0.0.1", Listen, 3),
		sock(80, UDP, "0.0.0.0", Listen, 2),
		sock(80, TCP, "127.0.0.1", Listen, 1),
		sock(80, TCP, "10.0.0.5", Listen, 4),
	}

	got := Dedupe(in)

	want := []Socket{
		sock(80, TCP, "10.0.0.5", Listen, 4),
		sock(80, TCP, "127.0.0.1", Listen, 1),
		sock(80, UDP, "0.0.0.0", Listen, 2),
		sock(8080, TCP, "127.0.0.1", Listen, 3),
	}
	if !slices.Equal(got, want) {
		t.Errorf("Dedupe() = %v, want %v", got, want)
	}
}

func TestDedupe_DoesNotMutateInput(t *testing.T) {
	in := []Socket{
		sock(3000, TCP, "::", Listen, 42),
		sock(3000, TCP, "0.0.0.0", Listen, 42),
	}
	snapshot := slices.Clone(in)

	_ = Dedupe(in)

	if !slices.Equal(in, snapshot) {
		t.Errorf("input mutated: %v, was %v", in, snapshot)
	}
}

func TestDedupe_EmptyInput(t *testing.T) {
	if got := Dedupe(nil); len(got) != 0 {
		t.Errorf("Dedupe(nil) = %v, want empty", got)
	}
}
