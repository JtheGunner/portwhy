package execx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// ErrUnexpectedCommand is returned by Fake for a command it was not set up for.
var ErrUnexpectedCommand = errors.New("unexpected command")

var _ Runner = (*Fake)(nil)

// Fake is a Runner for tests. Commands are matched by Key(name, args...):
// a command listed in Errors fails with that error and still returns its
// Responses entry (if any) as output, like a real command that prints before
// failing; any other command fails with ErrUnexpectedCommand. Fake records every call it receives.
type Fake struct {
	Responses map[string][]byte
	Errors    map[string]error

	mu    sync.Mutex
	calls []string
}

// Run implements Runner.
func (f *Fake) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := Key(name, args...)

	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()

	if err, ok := f.Errors[key]; ok {
		return f.Responses[key], err
	}
	if out, ok := f.Responses[key]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnexpectedCommand, key)
}

// Calls returns the command lines run so far, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}
