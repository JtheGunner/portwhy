package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxAncestorDepth bounds the parent chain so a corrupt tree cannot make the
// walk run away.
const maxAncestorDepth = 32

// procfsReader reads processes from a procfs-shaped directory tree. The root,
// clock tick rate, user lookup and readlink are injectable so the logic runs
// against a fake tree on any OS.
type procfsReader struct {
	root       string
	clockTicks int64
	lookupUser func(uid string) (string, error)
	readlink   func(path string) (string, error)
}

// Read implements Reader.
func (r *procfsReader) Read(ctx context.Context, pid int) (Process, error) {
	if err := ctx.Err(); err != nil {
		return Process{}, err
	}
	dir := filepath.Join(r.root, strconv.Itoa(pid))

	statBytes, err := readProcFile(filepath.Join(dir, "stat"))
	if err != nil {
		return Process{}, err
	}
	name, ppid, startTicks, err := parseStat(statBytes)
	if err != nil {
		return Process{}, fmt.Errorf("parse stat of pid %d: %w", pid, err)
	}
	statusBytes, err := readProcFile(filepath.Join(dir, "status"))
	if err != nil {
		return Process{}, err
	}
	uid, err := parseUID(statusBytes)
	if err != nil {
		return Process{}, fmt.Errorf("parse status of pid %d: %w", pid, err)
	}
	cmdlineBytes, err := readProcFile(filepath.Join(dir, "cmdline"))
	if err != nil {
		return Process{}, err
	}
	cgroupBytes, err := readProcFile(filepath.Join(dir, "cgroup"))
	if err != nil {
		return Process{}, err
	}
	started, err := r.startTime(startTicks)
	if err != nil {
		return Process{}, err
	}

	cwd, err := r.readlink(filepath.Join(dir, "cwd"))
	if err != nil {
		cwd = "" // Unreadable for other users' processes; not an error.
	}

	return Process{
		PID:       pid,
		PPID:      ppid,
		Name:      name,
		Cmdline:   parseCmdline(cmdlineBytes),
		User:      r.userName(uid),
		Started:   started,
		Cwd:       cwd,
		Cgroup:    parseCgroup(cgroupBytes),
		Ancestors: r.ancestors(pid, ppid),
	}, nil
}

// ancestors walks the parent chain starting at ppid, parent first, up to and
// including PID 1. A read error, a cycle or the depth limit ends the walk.
func (r *procfsReader) ancestors(pid, ppid int) []Ancestor {
	var chain []Ancestor
	seen := map[int]bool{pid: true}
	for cur := ppid; cur > 0 && !seen[cur] && len(chain) < maxAncestorDepth; {
		seen[cur] = true
		b, err := readProcFile(filepath.Join(r.root, strconv.Itoa(cur), "stat"))
		if err != nil {
			break
		}
		name, next, _, err := parseStat(b)
		if err != nil {
			break
		}
		chain = append(chain, Ancestor{PID: cur, Name: name})
		if cur == 1 {
			break
		}
		cur = next
	}
	return chain
}

func (r *procfsReader) startTime(startTicks uint64) (time.Time, error) {
	b, err := readProcFile(filepath.Join(r.root, "stat"))
	if err != nil {
		return time.Time{}, fmt.Errorf("read boot time: %w", err)
	}
	btime, err := parseBootTime(b)
	if err != nil {
		return time.Time{}, err
	}
	seconds := int64(startTicks) / r.clockTicks
	nanos := (int64(startTicks) % r.clockTicks) * int64(time.Second) / r.clockTicks
	return time.Unix(btime+seconds, nanos), nil
}

func (r *procfsReader) userName(uid string) string {
	if r.lookupUser == nil {
		return uid
	}
	name, err := r.lookupUser(uid)
	if err != nil || name == "" {
		return uid
	}
	return name
}

// readProcFile reads a procfs file and maps filesystem errors to the package
// errors: a missing file means the process is gone, EACCES/EPERM means we may
// not inspect it.
func readProcFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return b, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("%s: %w", path, ErrNotFound)
	case errors.Is(err, fs.ErrPermission):
		return nil, fmt.Errorf("%s: %w", path, ErrPermission)
	default:
		return nil, err
	}
}

// parseStat extracts the command name, parent PID and start time (in clock
// ticks since boot) from /proc/<pid>/stat. The name is delimited by the first
// "(" and the last ")" because it may itself contain spaces and parentheses.
func parseStat(b []byte) (name string, ppid int, startTicks uint64, err error) {
	s := string(b)
	open := strings.IndexByte(s, '(')
	closeIdx := strings.LastIndexByte(s, ')')
	if open < 0 || closeIdx < open {
		return "", 0, 0, errors.New("stat: missing command name")
	}
	name = s[open+1 : closeIdx]
	// Fields after the name: state(0) ppid(1) ... starttime(19).
	fields := strings.Fields(s[closeIdx+1:])
	if len(fields) < 20 {
		return "", 0, 0, fmt.Errorf("stat: want at least 20 fields after name, got %d", len(fields))
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("stat: parent pid: %w", err)
	}
	startTicks, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return "", 0, 0, fmt.Errorf("stat: start time: %w", err)
	}
	return name, ppid, startTicks, nil
}

// parseCmdline splits the NUL-separated /proc/<pid>/cmdline. Kernel threads
// have an empty cmdline and yield nil.
func parseCmdline(b []byte) []string {
	b = bytes.TrimSuffix(b, []byte{0})
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}

// parseCgroup returns the cgroup v2 path (the "0::" entry) or, on cgroup v1,
// the path of the first line.
func parseCgroup(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for _, line := range lines {
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return path
		}
	}
	// Format: hierarchy-ID:controllers:path (the path may contain colons).
	parts := strings.SplitN(lines[0], ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

// parseUID returns the real UID from /proc/<pid>/status.
func parseUID(b []byte) (string, error) {
	for _, line := range strings.Split(string(b), "\n") {
		rest, ok := strings.CutPrefix(line, "Uid:")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			break
		}
		return fields[0], nil
	}
	return "", errors.New("status: no Uid line")
}

// parseBootTime returns the btime (Unix seconds) from /proc/stat.
func parseBootTime(b []byte) (int64, error) {
	for _, line := range strings.Split(string(b), "\n") {
		rest, ok := strings.CutPrefix(line, "btime ")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("stat: btime: %w", err)
		}
		return v, nil
	}
	return 0, errors.New("stat: no btime line")
}
