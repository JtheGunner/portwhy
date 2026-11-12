//go:build linux

package cli

import (
	"os"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// DefaultDeps wires the real Linux collectors and readers.
func DefaultDeps() Deps {
	return Deps{
		Sockets:  socket.NewCollector(""),
		Procs:    proc.NewReader(""),
		Detector: detect.Chain(detect.Generic()),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		IsTTY:    isTerminal(os.Stdout),
		Pid:      os.Getpid(),
	}
}
