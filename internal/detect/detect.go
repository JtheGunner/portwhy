// Package detect explains who holds a socket. Detectors are tried in order by
// Chain; the first one that recognises the socket wins.
package detect

import (
	"context"

	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// Source says which mechanism owns a socket.
type Source string

// Known sources.
const (
	SourceDocker  Source = "docker"
	SourceSystemd Source = "systemd"
	SourceLaunchd Source = "launchd"
	SourceProcess Source = "process"
)

// Detail is one labelled line of an explanation.
type Detail struct {
	Key, Value string
}

// Explanation describes the cause of a busy port. Terminator is nil when no
// way to stop the cause is known.
type Explanation struct {
	Source     Source
	Title      string
	Details    []Detail
	Hints      []string
	Terminator kill.Terminator
}

// Detector recognises a socket. The process is nil when its owner is not
// visible. It reports false when the socket is not its responsibility.
type Detector interface {
	Detect(ctx context.Context, s socket.Socket, p *proc.Process) (*Explanation, bool)
}

type chain []Detector

// Chain returns a Detector that asks ds in order and returns the first hit.
func Chain(ds ...Detector) Detector {
	return chain(ds)
}

func (c chain) Detect(ctx context.Context, s socket.Socket, p *proc.Process) (*Explanation, bool) {
	for _, d := range c {
		if e, ok := d.Detect(ctx, s, p); ok {
			return e, true
		}
	}
	return nil, false
}
