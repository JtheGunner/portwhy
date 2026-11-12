//go:build darwin

package socket

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"slices"

	"github.com/JtheGunner/portwhy/internal/execx"
)

// lsofNothingFoundExitCode is the exit status lsof uses when nothing matched.
const lsofNothingFoundExitCode = 1

type darwinCollector struct {
	runner execx.Runner
}

// NewDarwinCollector returns a Collector for macOS. It combines two sources:
// lsof names the owning PID but, without root, only for the current user's
// processes; netstat lists every listener and TIME_WAIT socket but not their
// owner. A socket seen only by netstat keeps PID 0, so a port held by another
// user's process is never reported as free.
//
// Spike result: this own lsof/netstat parser is used instead of gopsutil; see
// "Decisions on record" in CLAUDE.md.
func NewDarwinCollector(r execx.Runner) Collector {
	return darwinCollector{runner: r}
}

// Sockets implements Collector.
func (c darwinCollector) Sockets(ctx context.Context) ([]Socket, error) {
	owned, err := c.lsofSockets(ctx)
	if err != nil {
		return nil, err
	}
	visible, err := c.netstatSockets(ctx)
	if err != nil {
		return nil, err
	}
	return mergeSockets(owned, visible), nil
}

func (c darwinCollector) lsofSockets(ctx context.Context) ([]Socket, error) {
	out, err := c.runner.Run(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP", "-F", "pPn")
	if err != nil && !isLsofNothingFound(err) {
		return nil, fmt.Errorf("list sockets with lsof: %w", err)
	}
	sockets, err := parseLsof(out)
	if err != nil {
		return nil, fmt.Errorf("parse lsof output: %w", err)
	}
	return sockets, nil
}

func (c darwinCollector) netstatSockets(ctx context.Context) ([]Socket, error) {
	var sockets []Socket
	for _, protocol := range []string{"tcp", "udp"} {
		out, err := c.runner.Run(ctx, "netstat", "-an", "-p", protocol)
		if err != nil {
			return nil, fmt.Errorf("list %s sockets with netstat: %w", protocol, err)
		}
		parsed, err := parseNetstat(out)
		if err != nil {
			return nil, fmt.Errorf("parse netstat %s output: %w", protocol, err)
		}
		sockets = append(sockets, parsed...)
	}
	return sockets, nil
}

// isLsofNothingFound reports whether err is lsof's regular "no match" exit.
// A missing binary, a timeout or any other failure is a real error.
func isLsofNothingFound(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == lsofNothingFoundExitCode
}

// mergeSockets prefers lsof entries: a netstat listener is dropped when lsof
// already reported a listener on the same protocol, port and address (all
// wildcards count as one address). Remaining netstat entries keep PID 0.
func mergeSockets(owned, visible []Socket) []Socket {
	type listenerKey struct {
		protocol Protocol
		port     uint16
		addr     netip.Addr
	}
	keyOf := func(s Socket) listenerKey {
		addr := s.Addr
		if addr.IsUnspecified() {
			addr = netip.IPv4Unspecified()
		}
		return listenerKey{s.Protocol, s.Port, addr}
	}

	seen := make(map[listenerKey]bool, len(owned))
	for _, s := range owned {
		seen[keyOf(s)] = true
	}

	merged := slices.Clone(owned)
	for _, s := range visible {
		if s.State == Listen && seen[keyOf(s)] {
			continue
		}
		merged = append(merged, s)
	}
	return slices.Compact(Dedupe(merged))
}
