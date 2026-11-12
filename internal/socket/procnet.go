package socket

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Kernel TCP state codes in /proc/net/tcp{,6} that portwhy reports.
const (
	procStateListen   = "0A"
	procStateTimeWait = "06"
)

// rawSocket is a socket row from /proc/net before its owner is resolved.
type rawSocket struct {
	Addr  netip.Addr
	Port  uint16
	State State
	Inode uint64
}

// parseProcNet parses the contents of /proc/net/{tcp,tcp6,udp,udp6}. The first
// (header) line is skipped. For TCP, only listening and time_wait rows are
// returned. For UDP, rows without a remote endpoint (0:0) count as listening.
// v6 selects the 128-bit address format. A malformed row makes the whole parse
// fail with an error naming the line; blank lines are ignored.
func parseProcNet(r io.Reader, proto Protocol, v6 bool) ([]rawSocket, error) {
	var out []rawSocket
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo == 1 {
			continue
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		sock, keep, err := parseProcNetLine(line, proto, v6)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if keep {
			out = append(out, sock)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/net table: %w", err)
	}
	return out, nil
}

func parseProcNetLine(line string, proto Protocol, v6 bool) (rawSocket, bool, error) {
	// Columns: sl local remote st tx:rx tr:when retrnsmt uid timeout inode ...
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return rawSocket{}, false, fmt.Errorf("expected at least 10 fields, got %d", len(fields))
	}
	addr, port, err := parseProcEndpoint(fields[1], v6)
	if err != nil {
		return rawSocket{}, false, fmt.Errorf("local address: %w", err)
	}
	remoteAddr, remotePort, err := parseProcEndpoint(fields[2], v6)
	if err != nil {
		return rawSocket{}, false, fmt.Errorf("remote address: %w", err)
	}
	inode, err := strconv.ParseUint(fields[9], 10, 64)
	if err != nil {
		return rawSocket{}, false, fmt.Errorf("inode: %w", err)
	}

	var state State
	switch proto {
	case TCP:
		switch strings.ToUpper(fields[3]) {
		case procStateListen:
			state = Listen
		case procStateTimeWait:
			state = TimeWait
		default:
			return rawSocket{}, false, nil
		}
	case UDP:
		if !remoteAddr.IsUnspecified() || remotePort != 0 {
			return rawSocket{}, false, nil
		}
		state = Listen
	default:
		return rawSocket{}, false, fmt.Errorf("unsupported protocol %q", proto)
	}
	return rawSocket{Addr: addr, Port: port, State: state, Inode: inode}, true, nil
}

// parseProcEndpoint decodes "ADDR:PORT" where ADDR is hex in host byte order
// (little-endian 32-bit words) and PORT is big-endian hex.
func parseProcEndpoint(s string, v6 bool) (netip.Addr, uint16, error) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	if !ok {
		return netip.Addr{}, 0, fmt.Errorf("missing port separator in %q", s)
	}
	wantLen := 8
	if v6 {
		wantLen = 32
	}
	if len(hexAddr) != wantLen {
		return netip.Addr{}, 0, fmt.Errorf("address %q has %d hex digits, want %d", hexAddr, len(hexAddr), wantLen)
	}
	raw, err := hex.DecodeString(hexAddr)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("address %q: %w", hexAddr, err)
	}
	for i := 0; i+4 <= len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("port %q: %w", hexPort, err)
	}
	var addr netip.Addr
	if v6 {
		addr = netip.AddrFrom16([16]byte(raw)).Unmap()
	} else {
		addr = netip.AddrFrom4([4]byte(raw))
	}
	return addr, uint16(port), nil
}

// scanInodeOwners maps socket inodes to the PID that holds them open by
// reading <root>/<pid>/fd/*. Processes whose fd directory cannot be read (other
// users, exited meanwhile) are skipped silently; their sockets stay unmapped.
func scanInodeOwners(ctx context.Context, root string) (map[uint64]int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	owners := make(map[uint64]int)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fdDir := filepath.Join(root, entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if inode, ok := parseSocketLink(target); ok {
				if _, seen := owners[inode]; !seen {
					owners[inode] = pid
				}
			}
		}
	}
	return owners, nil
}

// parseSocketLink extracts the inode from a "socket:[12345]" link target.
func parseSocketLink(target string) (uint64, bool) {
	rest, ok := strings.CutPrefix(target, "socket:[")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, "]")
	if !ok {
		return 0, false
	}
	inode, err := strconv.ParseUint(digits, 10, 64)
	return inode, err == nil
}

// procCollector reads sockets from a /proc-shaped directory tree.
type procCollector struct {
	root string
}

func newProcCollector(root string) *procCollector {
	return &procCollector{root: root}
}

type procTable struct {
	file  string
	proto Protocol
	v6    bool
	// optional tables may be absent, e.g. tcp6 when IPv6 is disabled.
	optional bool
}

var procTables = []procTable{
	{"tcp", TCP, false, false},
	{"tcp6", TCP, true, true},
	{"udp", UDP, false, false},
	{"udp6", UDP, true, true},
}

// Sockets implements Collector. Sockets whose owner cannot be determined are
// returned with PID 0 rather than dropped.
func (c *procCollector) Sockets(ctx context.Context) ([]Socket, error) {
	type tableRows struct {
		proto Protocol
		rows  []rawSocket
	}
	var tables []tableRows
	for _, table := range procTables {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := c.readTable(table)
		if err != nil {
			return nil, err
		}
		tables = append(tables, tableRows{table.proto, rows})
	}

	owners, err := scanInodeOwners(ctx, c.root)
	if err != nil {
		return nil, err
	}

	var out []Socket
	for _, t := range tables {
		for _, row := range t.rows {
			out = append(out, Socket{
				Port:     row.Port,
				Protocol: t.proto,
				Addr:     row.Addr,
				State:    row.State,
				PID:      owners[row.Inode],
			})
		}
	}
	return Dedupe(out), nil
}

func (c *procCollector) readTable(table procTable) ([]rawSocket, error) {
	path := filepath.Join(c.root, "net", table.file)
	f, err := os.Open(path)
	if err != nil {
		if table.optional && errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	rows, err := parseProcNet(f, table.proto, table.v6)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return rows, nil
}
