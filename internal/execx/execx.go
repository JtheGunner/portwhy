// Package execx runs external commands behind an interface so that code which
// shells out stays testable. All command execution in portwhy goes through it.
package execx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// waitDelay bounds how long Run waits for output pipes to close after a
// command was killed.
const waitDelay = time.Second

// Runner executes an external command and returns its standard output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type commandRunner struct {
	timeout time.Duration
}

// NewRunner returns a Runner that stops every command after timeout. A
// non-positive timeout means no limit beyond the caller's context.
func NewRunner(timeout time.Duration) Runner {
	return commandRunner{timeout: timeout}
}

// Run executes name with args. A failing command yields an error that includes
// the command line and its standard error output; a command stopped by the
// timeout or by ctx yields an error wrapping the context's error.
func (r commandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = waitDelay

	if err := cmd.Run(); err != nil {
		commandLine := Key(name, args...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s: %w", commandLine, ctxErr)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", commandLine, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", commandLine, err)
	}
	return stdout.Bytes(), nil
}

// Key renders a command line the way Fake looks it up: the name and the
// arguments joined by single spaces.
func Key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
