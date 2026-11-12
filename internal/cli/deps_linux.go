//go:build linux

package cli

import (
	"context"
	"os"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/detect/systemd"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/render"
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
	return Deps{
		Sockets:     socket.NewCollector(""),
		Procs:       procs,
		Detector:    newDetector(detect.NewGeneric(procs, os.Getpid()), systemdDetectors(procs)...),
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		IsTTY:       isTerminal(os.Stdout),
		Width:       terminalWidth(os.Stdout),
		ASCII:       render.UseASCII(os.Getenv),
		Stdin:       os.Stdin,
		StdinIsTTY:  isTerminal(os.Stdin),
		StderrIsTTY: isTerminal(os.Stderr),
		Pid:         os.Getpid(),
	}
}

// systemdDetectors returns the systemd detector. It needs portwhy's own
// cgroup to avoid ever stopping the unit portwhy runs in; when that cannot be
// read, the detector is left out rather than risk stopping the wrong unit.
func systemdDetectors(procs proc.Reader) []detect.Detector {
	self, err := procs.Read(context.Background(), os.Getpid())
	if err != nil {
		return nil
	}
	return []detect.Detector{systemd.NewDetector(
		execx.NewRunner(systemctlQueryTimeout),
		execx.NewRunner(systemctlStopTimeout),
		os.Getuid(),
		self.Cgroup,
	)}
}
