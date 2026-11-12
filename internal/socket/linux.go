//go:build linux

package socket

// defaultProcRoot is where Linux mounts the proc filesystem.
const defaultProcRoot = "/proc"

// NewCollector returns a Collector that reads sockets from the proc
// filesystem at root. An empty root means /proc.
func NewCollector(root string) Collector {
	if root == "" {
		root = defaultProcRoot
	}
	return newProcCollector(root)
}
