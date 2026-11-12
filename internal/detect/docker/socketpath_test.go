package docker

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestFindSocket_Order(t *testing.T) {
	const home = "/home/dev"
	all := map[string]bool{
		"/run/custom.sock":                      true,
		"/var/run/docker.sock":                  true,
		"/home/dev/.docker/run/docker.sock":     true,
		"/home/dev/.colima/default/docker.sock": true,
		"/home/dev/.orbstack/run/docker.sock":   true,
	}
	tests := []struct {
		name       string
		dockerHost string
		present    []string
		want       string
	}{
		{"DOCKER_HOST wins", "unix:///run/custom.sock", []string{"/run/custom.sock", "/var/run/docker.sock"}, "/run/custom.sock"},
		{"system socket first", "", []string{"/var/run/docker.sock", "/home/dev/.docker/run/docker.sock"}, "/var/run/docker.sock"},
		{"docker desktop", "", []string{"/home/dev/.docker/run/docker.sock", "/home/dev/.colima/default/docker.sock"}, "/home/dev/.docker/run/docker.sock"},
		{"colima", "", []string{"/home/dev/.colima/default/docker.sock", "/home/dev/.orbstack/run/docker.sock"}, "/home/dev/.colima/default/docker.sock"},
		{"orbstack", "", []string{"/home/dev/.orbstack/run/docker.sock"}, "/home/dev/.orbstack/run/docker.sock"},
		{"none", "", nil, ""},
		{"missing DOCKER_HOST socket falls through", "unix:///run/custom.sock", []string{"/var/run/docker.sock"}, "/var/run/docker.sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present := map[string]bool{}
			for _, p := range tt.present {
				present[p] = all[p]
			}
			got := FindSocket(tt.dockerHost, home, func(p string) bool { return present[p] })
			if got != tt.want {
				t.Errorf("FindSocket() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindSocket_IgnoresTCPDockerHost(t *testing.T) {
	got := FindSocket("tcp://127.0.0.1:2375", "/home/dev", func(p string) bool { return p == "/var/run/docker.sock" })
	if got != "/var/run/docker.sock" {
		t.Errorf("FindSocket() = %q, want the default socket (tcp DOCKER_HOST is ignored)", got)
	}
}

func TestFindSocket_EmptyHomeSkipsHomeCandidates(t *testing.T) {
	seen := []string{}
	FindSocket("", "", func(p string) bool { seen = append(seen, p); return false })
	if len(seen) != 1 || seen[0] != "/var/run/docker.sock" {
		t.Errorf("probed %v, want only /var/run/docker.sock", seen)
	}
}

func TestIsSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "pw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	regular := filepath.Join(dir, "file")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if !IsSocket(sock) {
		t.Error("IsSocket(socket) = false, want true")
	}
	if IsSocket(regular) {
		t.Error("IsSocket(regular file) = true, want false")
	}
	if IsSocket(filepath.Join(dir, "missing")) {
		t.Error("IsSocket(missing) = true, want false")
	}
}
