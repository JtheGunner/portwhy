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
	// Width is the terminal width of Stdout in columns; 0 when Stdout is not a
	// terminal or its size is unknown.
	Width int
	// ASCII selects plain ASCII symbols and line characters instead of
	// Unicode; set from TERM and the locale.
	ASCII bool
	Pid   int
}

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// terminalWidth returns the width of the terminal f is attached to, or 0 when
// f is not a terminal or its size cannot be read.
func terminalWidth(f *os.File) int {
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width < 0 {
		return 0
	}
	return width
}

// newDetector builds the detector chain: Docker first when a daemon socket
// exists, then the platform detectors in order, then fallback. Docker goes
// first because under Docker Desktop, Colima and OrbStack the listening
// process is a VM helper that says nothing about the container.
func newDetector(fallback detect.Detector, platform ...detect.Detector) detect.Detector {
	var chain []detect.Detector
	// A missing home directory only drops the per-user socket candidates.
	home, _ := os.UserHomeDir()
	if path := docker.FindSocket(os.Getenv("DOCKER_HOST"), home, docker.IsSocket); path != "" {
		chain = append(chain, docker.New(docker.NewClient(path)))
	}
	chain = append(chain, platform...)
	return detect.Chain(append(chain, fallback)...)
}
