package kill

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/proc"
)

var started = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

type fakeReader struct {
	p   proc.Process
	err error
}

func (f fakeReader) Read(context.Context, int) (proc.Process, error) { return f.p, f.err }

type sent struct {
	pid int
	sig syscall.Signal
}

// fakeSignaler records signals. The process stays alive until a signal in
// diesOn arrives.
type fakeSignaler struct {
	sent    []sent
	diesOn  map[syscall.Signal]bool
	dead    bool
	sendErr error
}

func (f *fakeSignaler) Signal(pid int, s syscall.Signal) error {
	f.sent = append(f.sent, sent{pid, s})
	if f.sendErr != nil {
		return f.sendErr
	}
	if f.diesOn[s] {
		f.dead = true
	}
	return nil
}

func (f *fakeSignaler) Alive(int) bool { return !f.dead }

func (f *fakeSignaler) signals() []syscall.Signal {
	var out []syscall.Signal
	for _, s := range f.sent {
		out = append(out, s.sig)
	}
	return out
}

func target() proc.Process {
	return proc.Process{PID: 4242, Name: "node", Started: started}
}

func newTerminator(sig Signaler) Terminator {
	return NewProcessTerminator(target(), fakeReader{p: target()}, 99, sig)
}

func TestProcessTerminator_RefusesPID1AndSelf(t *testing.T) {
	for name, p := range map[string]proc.Process{
		"pid 1": {PID: 1, Name: "init", Started: started},
		"self":  {PID: 99, Name: "portwhy", Started: started},
	} {
		t.Run(name, func(t *testing.T) {
			sig := &fakeSignaler{}
			term := NewProcessTerminator(p, fakeReader{p: p}, 99, sig)
			o, _ := options("", false)
			o.Yes = true

			err := Run(context.Background(), term, 80, o, freePort)

			if !errors.Is(err, ErrProtected) {
				t.Fatalf("err = %v, want ErrProtected", err)
			}
			if len(sig.sent) != 0 {
				t.Errorf("signals sent: %v", sig.sent)
			}
			if err := term.Terminate(context.Background()); !errors.Is(err, ErrProtected) {
				t.Errorf("direct Terminate err = %v, want ErrProtected", err)
			}
			if len(sig.sent) != 0 {
				t.Errorf("signals sent: %v", sig.sent)
			}
		})
	}
}

func TestProcessTerminator_RefusesInvalidPIDs(t *testing.T) {
	for _, pid := range []int{0, -1} {
		sig := &fakeSignaler{}
		p := proc.Process{PID: pid, Started: started}
		term := NewProcessTerminator(p, fakeReader{p: p}, 99, sig)

		if err := term.Terminate(context.Background()); !errors.Is(err, ErrProtected) {
			t.Errorf("pid %d: err = %v, want ErrProtected", pid, err)
		}
		if len(sig.sent) != 0 {
			t.Errorf("pid %d: signals sent: %v", pid, sig.sent)
		}
	}
}

func TestProcessTerminator_StaleStartTimeAborts(t *testing.T) {
	sig := &fakeSignaler{}
	recycled := target()
	recycled.Started = started.Add(time.Hour)
	term := NewProcessTerminator(target(), fakeReader{p: recycled}, 99, sig)
	o, _ := options("", false)
	o.Yes = true

	err := Run(context.Background(), term, 3000, o, freePort)

	if !errors.Is(err, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
}

func TestProcessTerminator_UnknownStartTimeIsRefused(t *testing.T) {
	sig := &fakeSignaler{}
	p := target()
	p.Started = time.Time{}
	term := NewProcessTerminator(p, fakeReader{p: p}, 99, sig)

	if err := term.Terminate(context.Background()); !errors.Is(err, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
}

func TestProcessTerminator_ReadErrorAborts(t *testing.T) {
	sig := &fakeSignaler{}
	boom := errors.New("boom")
	term := NewProcessTerminator(target(), fakeReader{err: boom}, 99, sig)

	if err := term.Terminate(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
}

func TestProcessTerminator_VanishedProcessIsSuccess(t *testing.T) {
	sig := &fakeSignaler{}
	term := NewProcessTerminator(target(), fakeReader{err: proc.ErrNotFound}, 99, sig)

	if err := term.Terminate(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
}

func TestProcessTerminator_TermSufficesWhenProcessExits(t *testing.T) {
	sig := &fakeSignaler{diesOn: map[syscall.Signal]bool{syscall.SIGTERM: true}}
	o, _ := options("", false)

	err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := sig.signals(); len(got) != 1 || got[0] != syscall.SIGTERM {
		t.Errorf("signals = %v, want [SIGTERM]", got)
	}
	if sig.sent[0].pid != 4242 {
		t.Errorf("signalled pid %d, want 4242", sig.sent[0].pid)
	}
}

func withYes(o Options) Options {
	o.Yes = true
	return o
}

func TestProcessTerminator_TermThenAskKill(t *testing.T) {
	sig := &fakeSignaler{diesOn: map[syscall.Signal]bool{syscall.SIGKILL: true}}
	// First answer confirms the stop, second confirms SIGKILL.
	o, out := options("y\ny\n", true)

	err := Run(context.Background(), newTerminator(sig), 3000, o, freePort)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	got := sig.signals()
	if len(got) != 2 || got[0] != syscall.SIGTERM || got[1] != syscall.SIGKILL {
		t.Errorf("signals = %v, want [SIGTERM SIGKILL]", got)
	}
	if !strings.Contains(out.String(), "Process still running — send SIGKILL? [y/N]") {
		t.Errorf("output %q lacks the SIGKILL prompt", out.String())
	}
}

func TestProcessTerminator_DeclinedKillLeavesProcess(t *testing.T) {
	sig := &fakeSignaler{diesOn: map[syscall.Signal]bool{syscall.SIGKILL: true}}
	o, _ := options("y\nn\n", true)

	err := Run(context.Background(), newTerminator(sig), 3000, o, freePort)

	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	if got := sig.signals(); len(got) != 1 || got[0] != syscall.SIGTERM {
		t.Errorf("signals = %v, want [SIGTERM]", got)
	}
}

func TestProcessTerminator_YesKillsWithoutSecondPrompt(t *testing.T) {
	sig := &fakeSignaler{diesOn: map[syscall.Signal]bool{syscall.SIGKILL: true}}
	o, out := options("", false)

	err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := sig.signals(); len(got) != 2 || got[1] != syscall.SIGKILL {
		t.Errorf("signals = %v, want [SIGTERM SIGKILL]", got)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("output %q contains a prompt", out.String())
	}
}

func TestProcessTerminator_StaleBeforeKillAborts(t *testing.T) {
	sig := &fakeSignaler{}
	rd := &switchingReader{first: target()}
	rd.second = target()
	rd.second.Started = started.Add(time.Minute)
	term := NewProcessTerminator(target(), rd, 99, sig)
	o, _ := options("", false)

	err := Run(context.Background(), term, 3000, withYes(o), freePort)

	if !errors.Is(err, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	if got := sig.signals(); len(got) != 1 || got[0] != syscall.SIGTERM {
		t.Errorf("signals = %v, want only [SIGTERM]", got)
	}
}

type switchingReader struct {
	first, second proc.Process
	calls         int
}

func (r *switchingReader) Read(context.Context, int) (proc.Process, error) {
	r.calls++
	if r.calls <= 2 { // Run's preflight and Terminate's own re-check
		return r.first, nil
	}
	return r.second, nil
}

func TestProcessTerminator_ForceSendsKillFirst(t *testing.T) {
	sig := &fakeSignaler{diesOn: map[syscall.Signal]bool{syscall.SIGKILL: true}}
	o, _ := options("y\n", true)
	o.Force = true

	err := Run(context.Background(), newTerminator(sig), 3000, o, freePort)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := sig.signals(); len(got) != 1 || got[0] != syscall.SIGKILL {
		t.Errorf("signals = %v, want [SIGKILL]", got)
	}
}

func TestProcessTerminator_ForceStillNeedsConfirmation(t *testing.T) {
	sig := &fakeSignaler{}
	o, _ := options("", false)
	o.Force = true

	err := Run(context.Background(), newTerminator(sig), 3000, o, freePort)

	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("err = %v, want ErrNotInteractive", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
}

func TestProcessTerminator_DescribeReflectsForce(t *testing.T) {
	o, _ := options("", false)
	o.Force = true
	term := configure(newTerminator(&fakeSignaler{}), o)

	if d := term.Describe(); !strings.Contains(d, "-KILL") || !strings.Contains(d, "4242") {
		t.Errorf("Describe = %q, want a SIGKILL command for pid 4242", d)
	}
	plain := newTerminator(&fakeSignaler{})
	if d := plain.Describe(); !strings.Contains(d, "-TERM") {
		t.Errorf("Describe = %q, want a SIGTERM command", d)
	}
}

func TestProcessTerminator_DryRunNeverSignals(t *testing.T) {
	sig := &fakeSignaler{}
	o, out := options("", false)
	o.DryRun = true

	if err := Run(context.Background(), newTerminator(sig), 3000, o, busyPort); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(sig.sent) != 0 {
		t.Errorf("signals sent: %v", sig.sent)
	}
	if !strings.Contains(out.String(), "4242") {
		t.Errorf("output %q does not describe the command", out.String())
	}
}

func TestProcessTerminator_EPERMSuggestsSudo(t *testing.T) {
	sig := &fakeSignaler{sendErr: syscall.EPERM}
	o, _ := options("", false)

	err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort)

	if err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("err = %v, want a sudo hint", err)
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Errorf("err = %v, want it to wrap EPERM", err)
	}
	if len(sig.sent) != 1 {
		t.Errorf("signals sent = %d, want exactly one attempt (no escalation)", len(sig.sent))
	}
}

func TestProcessTerminator_SignalErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	sig := &fakeSignaler{sendErr: boom}
	o, _ := options("", false)

	if err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestProcessTerminator_SignalToVanishedProcessIsSuccess(t *testing.T) {
	sig := &fakeSignaler{sendErr: syscall.ESRCH}
	o, _ := options("", false)

	if err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestProcessTerminator_SurvivingKillIsAnError(t *testing.T) {
	sig := &fakeSignaler{} // ignores every signal
	o, _ := options("", false)

	err := Run(context.Background(), newTerminator(sig), 3000, withYes(o), freePort)

	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("err = %v, want a still-running error", err)
	}
}

func TestProcessTerminator_ContextCancelStopsWaiting(t *testing.T) {
	sig := &fakeSignaler{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o, _ := options("", false)
	o.Grace = time.Minute

	err := Run(ctx, newTerminator(sig), 3000, withYes(o), freePort)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestProcessTerminator_KillsRealChild terminates a child this test started.
func TestProcessTerminator_KillsRealChild(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait() // reaps the child so it does not linger as a zombie
		close(exited)
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	p := proc.Process{PID: cmd.Process.Pid, Name: "sleep", Started: started}
	term := NewProcessTerminator(p, fakeReader{p: p}, os.Getpid(), OSSignaler{})
	o, _ := options("", false)
	o.Grace = 2 * time.Second

	if err := Run(context.Background(), term, 3000, withYes(o), freePort); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("child still running after Terminate")
	}
}
