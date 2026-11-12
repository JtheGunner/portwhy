//go:build darwin

package proc

import "github.com/JtheGunner/portwhy/internal/execx"

// NewDarwinReader returns a Reader for macOS that reads process details from
// ps and lsof through r.
func NewDarwinReader(r execx.Runner) Reader {
	return newPSReader(r)
}
