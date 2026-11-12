//go:build darwin

package cli

import (
	"os"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/detect/launchd"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/render"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const (
	// commandTimeout bounds every external query (lsof, netstat, ps, launchctl).
	commandTimeout = 10 * time.Second
	// stopTimeout leaves room for "launchctl bootout" and "brew services
	// stop", which wait until the service has ended.
	stopTimeout = 2 * time.Minute
)

// DefaultDeps wires the real macOS collectors and readers.
func DefaultDeps() Deps {
	runner := execx.NewRunner(commandTimeout)
	procs := proc.NewDarwinReader(runner)
	launchdDetector := launchd.NewDetector(runner, execx.NewRunner(stopTimeout), os.Getuid())
	return Deps{
		Sockets:    socket.NewDarwinCollector(runner),
		Procs:      procs,
		Detector:   newDetector(detect.NewGeneric(procs, os.Getpid()), launchdDetector),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTTY:      isTerminal(os.Stdout),
		Width:      terminalWidth(os.Stdout),
		ASCII:      render.UseASCII(os.Getenv),
		Stdin:      os.Stdin,
		StdinIsTTY: isTerminal(os.Stdin),
		Pid:        os.Getpid(),
	}
}
