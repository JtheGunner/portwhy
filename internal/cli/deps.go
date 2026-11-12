package cli

import (
	"io"
	"os"

	"golang.org/x/term"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/detect/docker"
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
	Stdin    io.Reader
	// IsTTY reports whether Stdout is a terminal; StdinIsTTY the same for
	// Stdin. Prompts need both.
	IsTTY      bool
	StdinIsTTY bool
	Pid        int
}

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// newDetector builds the detector chain: Docker first when a daemon socket
// exists, then fallback. Docker goes first because under Docker Desktop,
// Colima and OrbStack the listening process is a VM helper that says nothing
// about the container.
func newDetector(fallback detect.Detector) detect.Detector {
	// A missing home directory only drops the per-user socket candidates.
	home, _ := os.UserHomeDir()
	path := docker.FindSocket(os.Getenv("DOCKER_HOST"), home, docker.IsSocket)
	if path == "" {
		return fallback
	}
	return detect.Chain(docker.New(docker.NewClient(path)), fallback)
}
