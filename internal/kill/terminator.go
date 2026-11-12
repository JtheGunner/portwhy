// Package kill stops the cause of a busy port. This file holds only the
// interface that detectors return; the guarded implementation follows later.
package kill

import "context"

// Terminator stops whatever holds a port: a process, a container or a service.
type Terminator interface {
	// Describe returns the human-readable command that Terminate performs.
	Describe() string
	// Terminate stops the cause of the busy port.
	Terminate(ctx context.Context) error
}
