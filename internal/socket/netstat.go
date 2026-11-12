package socket

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"strings"
)

// parseNetstat parses the output of "netstat -an -p tcp" or "-p udp". It
// returns TCP listeners and TIME_WAIT sockets plus unconnected UDP sockets
// (reported as listeners). netstat does not name the owning process, so every
// returned socket has PID 0. Header lines and other TCP states are skipped.
//
// netstat truncates long IPv6 addresses in its default column layout. An
// address that cannot be parsed falls back to the wildcard of its family, so
// the port is still reported as taken.
func parseNetstat(out []byte) ([]Socket, error) {
	var sockets []Socket

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		protocol, wildcard, ok := netstatProtocol(fields[0])
		if !ok {
			continue
		}

		state := Listen
		switch {
		case protocol == TCP && len(fields) >= 6 && fields[5] == "LISTEN":
		case protocol == TCP && len(fields) >= 6 && fields[5] == "TIME_WAIT":
			state = TimeWait
		case protocol == UDP && fields[4] == "*.*":
		default:
			continue
		}

		addr, port, err := parseNetstatName(fields[3], wildcard)
		if err != nil {
			return nil, fmt.Errorf("netstat line %d: %w", lineNumber, err)
		}
		sockets = append(sockets, Socket{Port: port, Protocol: protocol, Addr: addr, State: state})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read netstat output: %w", err)
	}
	return sockets, nil
}

// netstatProtocol maps a netstat protocol column (tcp4, tcp6, tcp46, udp4, ...)
// to the protocol and the wildcard address of its family. The dual-stack
// variants (tcp46, udp46) use the IPv4 wildcard, like lsof does.
func netstatProtocol(column string) (Protocol, netip.Addr, bool) {
	switch column {
	case "tcp4", "tcp46":
		return TCP, netip.IPv4Unspecified(), true
	case "tcp6":
		return TCP, netip.IPv6Unspecified(), true
	case "udp4", "udp46":
		return UDP, netip.IPv4Unspecified(), true
	case "udp6":
		return UDP, netip.IPv6Unspecified(), true
	}
	return "", netip.Addr{}, false
}

// parseNetstatName parses a netstat local address such as "*.3000",
// "127.0.0.1.3000" or "::1.3000", where the port follows the last dot.
func parseNetstatName(name string, wildcard netip.Addr) (netip.Addr, uint16, error) {
	separator := strings.LastIndexByte(name, '.')
	if separator < 0 {
		return netip.Addr{}, 0, fmt.Errorf("invalid address %q", name)
	}
	host, portText := name[:separator], name[separator+1:]

	port, err := parsePort(portText)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("address %q: %w", name, err)
	}

	if host == "*" {
		return wildcard, port, nil
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return wildcard, port, nil
	}
	return addr, port, nil
}
