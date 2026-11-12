// Package docker explains ports that a Docker container publishes and stops
// the container through the Docker Engine API on its Unix socket. It never
// uses the docker CLI or an SDK.
package docker

import (
	"os"
	"path/filepath"
	"strings"
)

const unixScheme = "unix://"

// FindSocket returns the first Docker API socket that exists, or "" when there
// is none. The search order is $DOCKER_HOST (unix scheme only), the system
// socket, then the per-user sockets of Docker Desktop, Colima and OrbStack.
// An empty home skips the per-user candidates. isSocket reports whether a path
// is a usable socket.
func FindSocket(dockerHost, home string, isSocket func(string) bool) string {
	var candidates []string
	if path, ok := strings.CutPrefix(dockerHost, unixScheme); ok && path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "/var/run/docker.sock")
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".docker", "run", "docker.sock"),
			filepath.Join(home, ".colima", "default", "docker.sock"),
			filepath.Join(home, ".orbstack", "run", "docker.sock"),
		)
	}
	for _, c := range candidates {
		if isSocket(c) {
			return c
		}
	}
	return ""
}

// IsSocket reports whether path exists and is a Unix socket.
func IsSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}
