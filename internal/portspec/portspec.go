// Package portspec parses the port arguments given on the command line.
package portspec

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	minPort = 1
	maxPort = 65535
)

// ErrInvalid is wrapped by every error returned from Parse.
var ErrInvalid = errors.New("invalid port argument")

// Parse expands port arguments into a sorted, de-duplicated list of ports.
// Each argument is a single port ("3000") or an inclusive range ("3000-3010").
// No arguments yield a nil slice. The first invalid argument aborts parsing; the
// returned error wraps ErrInvalid and names the offending argument.
func Parse(args []string) ([]uint16, error) {
	var seen [maxPort + 1]bool
	for _, arg := range args {
		first, last, err := parseArg(arg)
		if err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrInvalid, arg, err)
		}
		for port := first; port <= last; port++ {
			seen[port] = true
		}
	}

	var ports []uint16
	for port := minPort; port <= maxPort; port++ {
		if seen[port] {
			ports = append(ports, uint16(port))
		}
	}
	return ports, nil
}

func parseArg(arg string) (first, last int, err error) {
	parts := strings.Split(strings.TrimSpace(arg), "-")
	switch len(parts) {
	case 1:
		first, err = parsePort(parts[0])
		return first, first, err
	case 2:
		if first, err = parsePort(parts[0]); err != nil {
			return 0, 0, err
		}
		if last, err = parsePort(parts[1]); err != nil {
			return 0, 0, err
		}
		if first > last {
			return 0, 0, errors.New("range start is greater than range end")
		}
		return first, last, nil
	default:
		return 0, 0, errors.New("expected a port or a range like 3000-3010")
	}
}

func parsePort(s string) (int, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, errors.New("not a port number")
	}
	if n < minPort || n > maxPort {
		return 0, fmt.Errorf("port must be between %d and %d", minPort, maxPort)
	}
	return int(n), nil
}
