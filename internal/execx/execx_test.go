package execx

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunner_ReturnsStdout(t *testing.T) {
	r := NewRunner(5 * time.Second)

	out, err := r.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got, want := string(out), "hello\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunner_TimesOut(t *testing.T) {
	r := NewRunner(50 * time.Millisecond)

	start := time.Now()
	_, err := r.Run(context.Background(), "sleep", "5")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want one wrapping context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v, want it to stop shortly after the 50ms timeout", elapsed)
	}
}

func TestRunner_NonZeroExitIncludesStderr(t *testing.T) {
	r := NewRunner(5 * time.Second)

	_, err := r.Run(context.Background(), "sh", "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("Run returned nil error for a failing command")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not contain the command's stderr", err)
	}
}

func TestRunner_NonZeroExitStillReturnsStdout(t *testing.T) {
	r := NewRunner(5 * time.Second)

	out, err := r.Run(context.Background(), "sh", "-c", "echo partial; exit 1")
	if err == nil {
		t.Fatal("Run returned nil error for a failing command")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want one wrapping *exec.ExitError", err)
	}
	if got, want := string(out), "partial\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunner_CommandNotFound(t *testing.T) {
	r := NewRunner(5 * time.Second)

	if _, err := r.Run(context.Background(), "portwhy-no-such-binary"); err == nil {
		t.Fatal("Run returned nil error for a missing binary")
	}
}

func TestRunner_NonPositiveTimeoutMeansNoLimit(t *testing.T) {
	r := NewRunner(0)

	out, err := r.Run(context.Background(), "echo", "ok")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(out) != "ok\n" {
		t.Errorf("stdout = %q, want %q", out, "ok\n")
	}
}

func TestRunner_ParentContextCancellationStopsCommand(t *testing.T) {
	r := NewRunner(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Run(ctx, "sleep", "5")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want one wrapping context.Canceled", err)
	}
}

func TestFake_ReturnsConfiguredResponse(t *testing.T) {
	f := &Fake{Responses: map[string][]byte{"lsof -nP -i :3000": []byte("data")}}

	out, err := f.Run(context.Background(), "lsof", "-nP", "-i", ":3000")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(out) != "data" {
		t.Errorf("stdout = %q, want %q", out, "data")
	}
}

func TestFake_UnexpectedCommandReturnsError(t *testing.T) {
	f := &Fake{}

	_, err := f.Run(context.Background(), "ps", "-p", "1")
	if !errors.Is(err, ErrUnexpectedCommand) {
		t.Fatalf("error = %v, want one wrapping ErrUnexpectedCommand", err)
	}
	if !strings.Contains(err.Error(), "ps -p 1") {
		t.Errorf("error %q does not name the command", err)
	}
}

func TestFake_ConfiguredErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	f := &Fake{Errors: map[string]error{"docker ps": boom}}

	if _, err := f.Run(context.Background(), "docker", "ps"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestFake_ConfiguredErrorKeepsConfiguredOutput(t *testing.T) {
	boom := errors.New("boom")
	f := &Fake{
		Responses: map[string][]byte{"lsof -i": []byte("partial")},
		Errors:    map[string]error{"lsof -i": boom},
	}

	out, err := f.Run(context.Background(), "lsof", "-i")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	if string(out) != "partial" {
		t.Errorf("stdout = %q, want %q", out, "partial")
	}
}

func TestFake_RecordsCallsInOrder(t *testing.T) {
	f := &Fake{Responses: map[string][]byte{"a": nil, "b x": nil}}

	_, _ = f.Run(context.Background(), "a")
	_, _ = f.Run(context.Background(), "b", "x")

	if want := []string{"a", "b x"}; !slices.Equal(f.Calls(), want) {
		t.Errorf("Calls() = %v, want %v", f.Calls(), want)
	}
}
