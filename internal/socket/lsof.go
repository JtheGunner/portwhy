package socket

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// parseLsof parses the output of "lsof -nP -iTCP -sTCP:LISTEN -iUDP -F pPn".
//
// The field output is line oriented: a "p" line starts a process, a "P" line
// names the protocol of the following file and an "n" line carries its name.
// A wildcard address (*) is reported as 0.0.0.0, because lsof does not say
// which family it belongs to; the collector's merge treats all wildcards as
// one. Entries that are not bound to a port (*:*) and connected UDP sockets
// (local->remote) are skipped.
func parseLsof(out []byte) ([]Socket, error) {
	var (
		sockets  []Socket
		pid      int
		havePID  bool
		protocol Protocol
	)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()
		if line == "" {
			continue
		}
		value := line[1:]

		switch line[0] {
		case 'p':
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return nil, fmt.Errorf("lsof line %d: invalid pid %q", lineNumber, value)
			}
			pid, havePID, protocol = parsed, true, ""
		case 'P':
			switch value {
			case "TCP":
				protocol = TCP
			case "UDP":
				protocol = UDP
			default:
				return nil, fmt.Errorf("lsof line %d: unsupported protocol %q", lineNumber, value)
			}
		case 'n':
			if !havePID || protocol == "" {
				return nil, fmt.Errorf("lsof line %d: address %q without process or protocol", lineNumber, value)
			}
			if strings.Contains(value, "->") || value == "*:*" {
				continue
			}
			addr, port, err := parseLsofName(value)
			if err != nil {
				return nil, fmt.Errorf("lsof line %d: %w", lineNumber, err)
			}
			sockets = append(sockets, Socket{Port: port, Protocol: protocol, Addr: addr, State: Listen, PID: pid})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read lsof output: %w", err)
	}
	return sockets, nil
}

// parseLsofName parses a local lsof name: "*:3000", "127.0.0.1:3000" or
// "[::1]:3000".
func parseLsofName(name string) (netip.Addr, uint16, error) {
	separator := strings.LastIndexByte(name, ':')
	if separator < 0 {
		return netip.Addr{}, 0, fmt.Errorf("invalid address %q", name)
	}
	host, portText := name[:separator], name[separator+1:]

	port, err := parsePort(portText)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("address %q: %w", name, err)
	}

	if host == "*" {
		return netip.IPv4Unspecified(), port, nil
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("address %q: %w", name, err)
	}
	return addr, port, nil
}

func parsePort(text string) (uint16, error) {
	port, err := strconv.ParseUint(text, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", text)
	}
	return uint16(port), nil
}
