package cli

import (
	"io"
	"os"

	"golang.org/x/term"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// Deps bundles everything Run needs from the outside world, so tests can
// substitute fakes.
type Deps struct {
	Sockets  socket.Collector
	Procs    proc.Reader
	Detector detect.Detector
	Stdout   io.Writer
	Stderr   io.Writer
	IsTTY    bool
	Pid      int
}

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
