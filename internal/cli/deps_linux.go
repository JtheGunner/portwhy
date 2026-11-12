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
	procs := proc.NewReader("")
	return Deps{
		Sockets:    socket.NewCollector(""),
		Procs:      procs,
		Detector:   newDetector(detect.NewGeneric(procs, os.Getpid())),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTTY:      isTerminal(os.Stdout),
		Stdin:      os.Stdin,
		StdinIsTTY: isTerminal(os.Stdin),
		Pid:        os.Getpid(),
	}
}
