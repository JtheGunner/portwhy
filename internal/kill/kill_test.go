package kill

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeTerminator struct {
	describe string
	err      error
	calls    int
}

func (f *fakeTerminator) Describe() string { return f.describe }
func (f *fakeTerminator) Terminate(context.Context) error {
	f.calls++
	return f.err
}

func freePort(context.Context, uint16) (bool, error) { return true, nil }
func busyPort(context.Context, uint16) (bool, error) { return false, nil }

func options(in string, interactive bool) (Options, *bytes.Buffer) {
	var out bytes.Buffer
	return Options{
		Interactive:   interactive,
		In:            strings.NewReader(in),
		Out:           &out,
		Grace:         10 * time.Millisecond,
		VerifyTimeout: 10 * time.Millisecond,
	}, &out
}

func TestRun_DeclinedPromptDoesNothing(t *testing.T) {
	ft := &fakeTerminator{describe: "kill -TERM 42"}
	o, out := options("n\n", true)

	err := Run(context.Background(), ft, 3000, o, freePort)

	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate called %d times, want 0", ft.calls)
	}
	if !strings.Contains(out.String(), "kill -TERM 42") {
		t.Errorf("prompt %q does not show the command", out.String())
	}
}

func TestRun_EmptyAnswerDeclines(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("\n", true)

	if err := Run(context.Background(), ft, 3000, o, freePort); !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
}

func TestRun_AcceptedPromptTerminates(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("y\n", true)

	if err := Run(context.Background(), ft, 3000, o, freePort); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if ft.calls != 1 {
		t.Errorf("Terminate called %d times, want 1", ft.calls)
	}
}

func TestRun_YesSkipsPrompt(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, out := options("", false)
	o.Yes = true

	if err := Run(context.Background(), ft, 3000, o, freePort); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if ft.calls != 1 {
		t.Errorf("Terminate called %d times, want 1", ft.calls)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("output %q contains a prompt", out.String())
	}
}

func TestRun_DryRunNeverSignals(t *testing.T) {
	ft := &fakeTerminator{describe: "kill -TERM 42"}
	o, out := options("", false)
	o.DryRun = true
	o.Yes = true

	if err := Run(context.Background(), ft, 3000, o, busyPort); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate called %d times, want 0", ft.calls)
	}
	if !strings.Contains(out.String(), "kill -TERM 42") {
		t.Errorf("output %q does not show the command", out.String())
	}
}

func TestRun_NotInteractiveWithoutYesFails(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("y\n", false)

	err := Run(context.Background(), ft, 3000, o, freePort)

	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("err = %v, want ErrNotInteractive", err)
	}
	if ft.calls != 0 {
		t.Errorf("Terminate called %d times, want 0", ft.calls)
	}
}

func TestRun_PortStillBusyAfterwards(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("", false)
	o.Yes = true

	err := Run(context.Background(), ft, 3000, o, busyPort)

	if !errors.Is(err, ErrStillBusy) {
		t.Fatalf("err = %v, want ErrStillBusy", err)
	}
}

func TestRun_PortFreeAfterSettling(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("", false)
	o.Yes = true
	o.VerifyTimeout = time.Second
	checks := 0
	check := func(context.Context, uint16) (bool, error) {
		checks++
		return checks >= 3, nil
	}

	if err := Run(context.Background(), ft, 3000, o, check); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestRun_CheckErrorIsReturned(t *testing.T) {
	ft := &fakeTerminator{describe: "x"}
	o, _ := options("", false)
	o.Yes = true
	boom := errors.New("boom")

	err := Run(context.Background(), ft, 3000, o, func(context.Context, uint16) (bool, error) { return false, boom })

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestRun_TerminateErrorSkipsPortCheck(t *testing.T) {
	boom := errors.New("boom")
	ft := &fakeTerminator{describe: "x", err: boom}
	o, _ := options("", false)
	o.Yes = true
	checked := false

	err := Run(context.Background(), ft, 3000, o, func(context.Context, uint16) (bool, error) {
		checked = true
		return true, nil
	})

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if checked {
		t.Error("port was checked after a failed termination")
	}
}
