//go:build linux

package cli

import (
	"os"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/detect/systemd"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const (
	// systemctlQueryTimeout bounds the quick systemctl lookups.
	systemctlQueryTimeout = 5 * time.Second
	// systemctlStopTimeout leaves room for systemd's own stop timeout, which
	// is 90 s by default.
	systemctlStopTimeout = 2 * time.Minute
)

// DefaultDeps wires the real Linux collectors and readers.
func DefaultDeps() Deps {
	procs := proc.NewReader("")
	systemdDetector := systemd.NewDetector(
		execx.NewRunner(systemctlQueryTimeout),
		execx.NewRunner(systemctlStopTimeout),
		os.Getuid(),
	)
	return Deps{
		Sockets:    socket.NewCollector(""),
		Procs:      procs,
		Detector:   newDetector(detect.NewGeneric(procs, os.Getpid()), systemdDetector),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTTY:      isTerminal(os.Stdout),
		Stdin:      os.Stdin,
		StdinIsTTY: isTerminal(os.Stdin),
		Pid:        os.Getpid(),
	}
}
