package kill

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/JtheGunner/portwhy/internal/proc"
)

const alivePollInterval = 20 * time.Millisecond

var (
	errGone         = errors.New("process is gone")
	errStillRunning = errors.New("still running")
)

// Signaler sends signals to processes and tells whether they are alive. It is
// an interface so tests never signal real processes.
type Signaler interface {
	Signal(pid int, s syscall.Signal) error
	Alive(pid int) bool
}

// OSSignaler is the Signaler backed by the operating system.
type OSSignaler struct{}

// Signal sends s to pid.
func (OSSignaler) Signal(pid int, s syscall.Signal) error { return syscall.Kill(pid, s) }

// Alive reports whether pid exists. A process owned by someone else (EPERM)
// still counts as alive.
func (OSSignaler) Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

type processTerminator struct {
	target proc.Process
	reader proc.Reader
	self   int
	sig    Signaler
	opts   Options
}

// NewProcessTerminator returns a Terminator that stops process p with signals.
// It refuses PID 1 and self, and re-reads the process through r before every
// signal to make sure the PID was not reused.
func NewProcessTerminator(p proc.Process, r proc.Reader, self int, sig Signaler) Terminator {
	return &processTerminator{target: p, reader: r, self: self, sig: sig}
}

func (t *processTerminator) withOptions(o Options) Terminator {
	c := *t
	c.opts = o
	return &c
}

func (t *processTerminator) Describe() string {
	flag := "-TERM"
	if t.opts.Force {
		flag = "-KILL"
	}
	return fmt.Sprintf("kill %s %d (%s)", flag, t.target.PID, t.target.Name)
}

// preflight refuses protected or changed targets before anything is asked.
// A process that is already gone is not an error: there is nothing to stop.
func (t *processTerminator) preflight(ctx context.Context) error {
	if err := t.check(ctx); err != nil && !errors.Is(err, errGone) {
		return err
	}
	return nil
}

// check refuses PID 1, self and invalid PIDs, then verifies that the PID still
// belongs to the inspected process by comparing start times. It returns
// errGone when the process no longer exists.
func (t *processTerminator) check(ctx context.Context) error {
	pid := t.target.PID
	if pid <= 1 || pid == t.self {
		return fmt.Errorf("PID %d (%s): %w", pid, t.target.Name, ErrProtected)
	}
	if t.target.Started.IsZero() {
		return fmt.Errorf("PID %d: start time unknown: %w", pid, ErrStale)
	}
	current, err := t.reader.Read(ctx, pid)
	if errors.Is(err, proc.ErrNotFound) {
		return errGone
	}
	if err != nil {
		return fmt.Errorf("re-reading PID %d: %w", pid, err)
	}
	if !current.Started.Equal(t.target.Started) {
		return fmt.Errorf("PID %d: %w", pid, ErrStale)
	}
	return nil
}

func (t *processTerminator) Terminate(ctx context.Context) error {
	if err := t.check(ctx); err != nil {
		return ignoreGone(err)
	}

	if t.opts.Force {
		return t.sendAndWait(ctx, syscall.SIGKILL)
	}
	err := t.sendAndWait(ctx, syscall.SIGTERM)
	if err == nil || !errors.Is(err, errStillRunning) {
		return err
	}

	if !t.opts.Yes {
		if !t.opts.Interactive {
			return fmt.Errorf("PID %d: %w", t.target.PID, ErrNotInteractive)
		}
		ok, err := confirm(t.opts.In, t.opts.Out, "Process still running — send SIGKILL? [y/N] ")
		if err != nil {
			return err
		}
		if !ok {
			return ErrDeclined
		}
	}

	// Time has passed since the first check: make sure the PID still belongs
	// to the same process before escalating.
	if err := t.check(ctx); err != nil {
		return ignoreGone(err)
	}
	return t.sendAndWait(ctx, syscall.SIGKILL)
}

func ignoreGone(err error) error {
	if errors.Is(err, errGone) {
		return nil
	}
	return err
}

// sendAndWait sends s and waits up to the grace period for the process to
// exit. It wraps errStillRunning when the process survives.
func (t *processTerminator) sendAndWait(ctx context.Context, s syscall.Signal) error {
	pid := t.target.PID
	if err := t.sig.Signal(pid, s); err != nil {
		switch {
		case errors.Is(err, syscall.ESRCH):
			return nil
		case errors.Is(err, syscall.EPERM):
			return fmt.Errorf("not permitted to signal PID %d (%s): rerun portwhy with sudo to stop it: %w",
				pid, t.target.Name, err)
		default:
			return fmt.Errorf("signalling PID %d: %w", pid, err)
		}
	}

	deadline := time.Now().Add(t.opts.grace())
	for {
		if !t.sig.Alive(pid) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("PID %d (%s) %w after %s", pid, t.target.Name, errStillRunning, s)
		}
		if err := sleep(ctx, alivePollInterval); err != nil {
			return err
		}
	}
}
