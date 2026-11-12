package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JtheGunner/portwhy/internal/execx"
)

const (
	// maxPSAncestorDepth bounds how far up the parent chain a reader walks.
	maxPSAncestorDepth = 32

	// psLstartLayout is the layout of the ps "lstart" column. The "_2" verb
	// accepts the space-padded day of month ("Nov  7").
	psLstartLayout = "Mon Jan _2 15:04:05 2006"

	// psLstartFields is the number of whitespace-separated tokens in lstart.
	psLstartFields = 5

	// psLeadingFields counts the fields before the command: pid, ppid, user
	// and the lstart tokens.
	psLeadingFields = 3 + psLstartFields
)

// psReader reads processes through ps and lsof. It is untagged so that its
// logic is testable on every OS; NewDarwinReader wires it up for macOS.
type psReader struct {
	runner execx.Runner
}

func newPSReader(r execx.Runner) *psReader {
	return &psReader{runner: r}
}

// Read implements Reader.
func (r *psReader) Read(ctx context.Context, pid int) (Process, error) {
	p, err := r.readPS(ctx, pid)
	if err != nil {
		return Process{}, err
	}
	p.Cwd = r.readCwd(ctx, pid)
	p.Ancestors = r.readAncestors(ctx, p.PPID, pid)
	return p, nil
}

func (r *psReader) readPS(ctx context.Context, pid int) (Process, error) {
	out, err := r.runner.Run(ctx, "ps", "-o", "pid=,ppid=,user=,lstart=,command=", "-p", strconv.Itoa(pid))
	if err != nil {
		return Process{}, classifyPSError(pid, err)
	}
	p, err := parsePS(out, r.readComm(ctx, pid))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Process{}, err
		}
		return Process{}, fmt.Errorf("read process %d: %w", pid, err)
	}
	return p, nil
}

// readComm returns the executable path of pid as reported by "ps -o comm=",
// which is not split on spaces. It returns "" when ps fails, so the caller
// falls back to the lossy whitespace split.
func (r *psReader) readComm(ctx context.Context, pid int) string {
	out, err := r.runner.Run(ctx, "ps", "-o", "comm=", "-p", strconv.Itoa(pid))
	if err != nil {
		return ""
	}
	return parsePSComm(out)
}

// classifyPSError maps a failed ps run to a Reader error. ps exits 1 with no
// output when the PID does not exist; any other failure (ps missing, timeout,
// other exit codes) is not "not found" and keeps its cause.
func classifyPSError(pid int, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return ErrNotFound
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("read process %d: %w: %w", pid, ErrPermission, err)
	}
	return fmt.Errorf("read process %d: %w", pid, err)
}

// readCwd returns the working directory of pid, or "" when lsof fails or has
// nothing to report (for example for another user's process).
func (r *psReader) readCwd(ctx context.Context, pid int) string {
	out, err := r.runner.Run(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn")
	if err != nil {
		return ""
	}
	return parseLsofCwd(out)
}

// readAncestors walks the parent chain starting at ppid: parent first, then
// grandparent, and so on. The walk includes PID 1 and ends there, after maxPSAncestorDepth
// entries, on a cycle, or when an ancestor cannot be read; it never fails.
func (r *psReader) readAncestors(ctx context.Context, ppid, self int) []Ancestor {
	var ancestors []Ancestor
	seen := map[int]bool{self: true}
	for pid := ppid; pid > 0 && !seen[pid] && len(ancestors) < maxPSAncestorDepth; {
		seen[pid] = true
		p, err := r.readPS(ctx, pid)
		if err != nil {
			break
		}
		ancestors = append(ancestors, Ancestor{PID: p.PID, Name: p.Name})
		pid = p.PPID
	}
	return ancestors
}

// parsePS parses the output of
// "ps -o pid=,ppid=,user=,lstart=,command= -p <pid>". Empty output yields
// ErrNotFound.
//
// comm is the executable path from "ps -o comm=" (see parsePSComm). When the
// command column starts with it, comm is kept as the first Cmdline element and
// Name is its base name, so executables whose path contains spaces survive.
// With comm empty or not a prefix of the command, the first whitespace token
// is used instead, which cuts such paths off.
//
// The remaining Cmdline is the command column split on whitespace. This is
// lossy: ps joins the arguments with single spaces, so arguments that contain
// spaces cannot be told apart from several arguments.
func parsePS(out []byte, comm string) (Process, error) {
	line := strings.TrimSpace(string(out))
	if line == "" {
		return Process{}, ErrNotFound
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}

	fields := strings.Fields(line)
	if len(fields) <= psLeadingFields {
		return Process{}, fmt.Errorf("parse ps output %q: want at least %d fields, got %d", line, psLeadingFields+1, len(fields))
	}

	command := commandColumn(line)

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return Process{}, fmt.Errorf("parse ps pid %q: %w", fields[0], err)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return Process{}, fmt.Errorf("parse ps ppid %q: %w", fields[1], err)
	}
	lstart := strings.Join(fields[3:3+psLstartFields], " ")
	started, err := time.ParseInLocation(psLstartLayout, lstart, time.Local)
	if err != nil {
		return Process{}, fmt.Errorf("parse ps lstart %q: %w", lstart, err)
	}

	cmdline := fields[psLeadingFields:]
	if rest, ok := strings.CutPrefix(command, comm); ok && comm != "" && (rest == "" || rest[0] == ' ') {
		cmdline = append([]string{comm}, strings.Fields(rest)...)
	}
	return Process{
		PID:     pid,
		PPID:    ppid,
		User:    fields[2],
		Started: started,
		Name:    filepath.Base(cmdline[0]),
		Cmdline: cmdline,
	}, nil
}

// commandColumn returns line without its leading psLeadingFields
// whitespace-separated fields, with the command's own spacing intact.
func commandColumn(line string) string {
	rest := line
	for range psLeadingFields {
		rest = strings.TrimLeft(rest, " \t")
		i := strings.IndexAny(rest, " \t")
		if i < 0 {
			return ""
		}
		rest = rest[i:]
	}
	return strings.TrimSpace(rest)
}

// parsePSComm parses "ps -o comm= -p <pid>" output: the first line, trimmed.
func parsePSComm(out []byte) string {
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

// parseLsofCwd extracts the path from "lsof -Fn" output: the first line that
// starts with "n". It returns "" when there is none.
func parseLsofCwd(out []byte) string {
	for line := range strings.SplitSeq(string(out), "\n") {
		if path, ok := strings.CutPrefix(line, "n"); ok && path != "" {
			return path
		}
	}
	return ""
}
