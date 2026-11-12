package kill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Sentinel errors returned by Run and by the terminators.
var (
	// ErrDeclined means the user answered no at a prompt.
	ErrDeclined = errors.New("declined")
	// ErrProtected means the target is PID 1 or portwhy itself and is never signalled.
	ErrProtected = errors.New("refusing to stop a protected process")
	// ErrStale means the process behind the PID is not the one that was
	// explained (the PID was reused, or its start time could not be verified).
	ErrStale = errors.New("process changed since it was inspected")
	// ErrStillBusy means the port is still taken after the cause was stopped.
	ErrStillBusy = errors.New("port is still busy")
	// ErrNotInteractive means confirmation is needed but there is no terminal
	// and --yes was not given.
	ErrNotInteractive = errors.New("confirmation required but not running on a terminal (use --yes)")
)

// Default timings.
const (
	DefaultGrace         = 5 * time.Second
	DefaultVerifyTimeout = time.Second

	verifyInterval = 50 * time.Millisecond
)

// Options control how Run stops the cause of a busy port.
type Options struct {
	// Yes skips every confirmation prompt.
	Yes bool
	// Force sends SIGKILL immediately instead of SIGTERM plus a grace period.
	Force bool
	// DryRun prints what would happen and never signals.
	DryRun bool
	// Grace is how long to wait after SIGTERM before offering SIGKILL.
	// Zero means DefaultGrace.
	Grace time.Duration
	// VerifyTimeout is how long to wait for the port to become free after a
	// successful stop. Zero means DefaultVerifyTimeout.
	VerifyTimeout time.Duration
	// Interactive reports that In and Out are connected to a terminal.
	Interactive bool
	In          io.Reader
	Out         io.Writer
}

func (o Options) grace() time.Duration {
	if o.Grace <= 0 {
		return DefaultGrace
	}
	return o.Grace
}

func (o Options) verifyTimeout() time.Duration {
	if o.VerifyTimeout <= 0 {
		return DefaultVerifyTimeout
	}
	return o.VerifyTimeout
}

// PortChecker reports whether nothing is listening on port any more.
type PortChecker func(ctx context.Context, port uint16) (free bool, err error)

// optionAware is implemented by terminators that need the run options, for
// example to ask before escalating to SIGKILL.
type optionAware interface {
	withOptions(o Options) Terminator
}

// preflighter is implemented by terminators that can refuse a target before
// anything is asked or signalled.
type preflighter interface {
	preflight(ctx context.Context) error
}

func configure(t Terminator, o Options) Terminator {
	if a, ok := t.(optionAware); ok {
		return a.withOptions(o)
	}
	return t
}

// Run stops the cause of a busy port with t after confirmation and verifies
// afterwards that check reports the port as free. A nil check skips the
// verification.
func Run(ctx context.Context, t Terminator, port uint16, o Options, check PortChecker) error {
	t = configure(t, o)

	if o.DryRun {
		return dryRun(ctx, t, o)
	}
	if p, ok := t.(preflighter); ok {
		if err := p.preflight(ctx); err != nil {
			return err
		}
	}
	if !o.Yes {
		if !o.Interactive {
			return ErrNotInteractive
		}
		ok, err := confirm(o.In, o.Out, fmt.Sprintf("Run '%s'? [y/N] ", t.Describe()))
		if err != nil {
			return err
		}
		if !ok {
			return ErrDeclined
		}
	}

	if err := t.Terminate(ctx); err != nil {
		return err
	}
	return verifyFree(ctx, port, o, check)
}

// dryRun reports what a real run would do without signalling. A protected
// target is named as such instead of "would execute", and is not an error:
// nothing is executed either way.
func dryRun(ctx context.Context, t Terminator, o Options) error {
	if p, ok := t.(preflighter); ok {
		if err := p.preflight(ctx); errors.Is(err, ErrProtected) {
			_, werr := fmt.Fprintf(o.Out, "Dry run, would not execute: %v\n", err)
			return werr
		}
	}
	_, err := fmt.Fprintf(o.Out, "Dry run, would execute: %s\n", t.Describe())
	return err
}

// verifyFree polls check until the port is free or the verify timeout passes.
func verifyFree(ctx context.Context, port uint16, o Options, check PortChecker) error {
	if check == nil {
		return nil
	}
	deadline := time.Now().Add(o.verifyTimeout())
	for {
		free, err := check(ctx, port)
		if err != nil {
			return fmt.Errorf("checking port %d afterwards: %w", port, err)
		}
		if free {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("port %d: %w", port, ErrStillBusy)
		}
		if err := sleep(ctx, verifyInterval); err != nil {
			return err
		}
	}
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
