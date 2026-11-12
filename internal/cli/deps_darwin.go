//go:build darwin

package cli

import (
	"os"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// commandTimeout bounds every external command (lsof, netstat, ps).
const commandTimeout = 10 * time.Second

// DefaultDeps wires the real macOS collectors and readers.
func DefaultDeps() Deps {
	runner := execx.NewRunner(commandTimeout)
	procs := proc.NewDarwinReader(runner)
	return Deps{
		Sockets:    socket.NewDarwinCollector(runner),
		Procs:      procs,
		Detector:   detect.Chain(detect.NewGeneric(procs, os.Getpid())),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTTY:      isTerminal(os.Stdout),
		Stdin:      os.Stdin,
		StdinIsTTY: isTerminal(os.Stdin),
		Pid:        os.Getpid(),
	}
}
