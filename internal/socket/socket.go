// Package socket describes the sockets bound on the local machine and defines
// how they are collected. OS-specific collectors live in their own files.
package socket

import (
	"cmp"
	"context"
	"net/netip"
	"slices"
)

// Protocol is the transport protocol of a socket.
type Protocol string

// Supported transport protocols.
const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// State is the part of a socket's lifecycle that portwhy reports.
type State string

// Reported socket states.
const (
	Listen   State = "listen"
	TimeWait State = "time_wait"
)

// Socket is one bound socket. PID is 0 when the owning process is not visible
// to the current user.
type Socket struct {
	Port     uint16
	Protocol Protocol
	Addr     netip.Addr
	State    State
	PID      int
}

// Collector lists the sockets currently bound on the machine.
type Collector interface {
	Sockets(ctx context.Context) ([]Socket, error)
}

// Dedupe collapses dual-stack listeners and returns the sockets in a stable
// order (port, protocol, address, state, PID). An IPv6 wildcard socket (::) is
// dropped when an IPv4 wildcard socket (0.0.0.0) with the same port, protocol,
// state and PID exists, because the kernel reports one dual-stack listener as
// both. The input slice is not modified.
func Dedupe(in []Socket) []Socket {
	type identity struct {
		port     uint16
		protocol Protocol
		state    State
		pid      int
	}
	identityOf := func(s Socket) identity {
		return identity{s.Port, s.Protocol, s.State, s.PID}
	}

	hasIPv4Wildcard := make(map[identity]bool)
	for _, s := range in {
		if s.Addr.Is4() && s.Addr.IsUnspecified() {
			hasIPv4Wildcard[identityOf(s)] = true
		}
	}

	out := make([]Socket, 0, len(in))
	for _, s := range in {
		isIPv6Wildcard := s.Addr.Is6() && !s.Addr.Is4In6() && s.Addr.IsUnspecified()
		if isIPv6Wildcard && hasIPv4Wildcard[identityOf(s)] {
			continue
		}
		out = append(out, s)
	}

	slices.SortStableFunc(out, func(a, b Socket) int {
		return cmp.Or(
			cmp.Compare(a.Port, b.Port),
			cmp.Compare(a.Protocol, b.Protocol),
			a.Addr.Compare(b.Addr),
			cmp.Compare(a.State, b.State),
			cmp.Compare(a.PID, b.PID),
		)
	})
	return out
}
