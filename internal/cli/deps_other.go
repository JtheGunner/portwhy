//go:build !linux && !darwin

package cli

import (
	"context"
	"errors"
	"os"

	"github.com/JtheGunner/portwhy/internal/socket"
)

var errUnsupportedOS = errors.New("unsupported operating system: portwhy supports Linux and macOS")

type unsupportedCollector struct{}

func (unsupportedCollector) Sockets(context.Context) ([]socket.Socket, error) {
	return nil, errUnsupportedOS
}

// DefaultDeps returns Deps whose collector fails with a clear message, so Run
// exits with code 2 on unsupported operating systems.
func DefaultDeps() Deps {
	return Deps{
		Sockets: unsupportedCollector{},
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Pid:     os.Getpid(),
	}
}
