package docker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startDaemon serves handler on a temporary Unix socket and returns its path.
func startDaemon(t *testing.T, handler http.Handler) string {
	t.Helper()
	// Unix socket paths are length-limited, so avoid the long t.TempDir() path.
	dir, err := os.MkdirTemp("", "pw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return path
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestClient_ListContainers(t *testing.T) {
	sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/containers/json" {
			t.Errorf("request = %s %s, want GET /containers/json", r.Method, r.URL.Path)
		}
		_, _ = w.Write(fixture(t, "containers.json"))
	}))

	got, err := NewClient(sock).ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers error: %v", err)
	}
	if len(got) != 2 || got[0].Image != "nginx:1.27" || got[0].Ports[0].PublicPort != 8125 {
		t.Errorf("ListContainers = %+v, want the two fixture containers", got)
	}
}

func TestClient_InspectContainer(t *testing.T) {
	sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/abc/json" {
			t.Errorf("path = %s, want /containers/abc/json", r.URL.Path)
		}
		_, _ = w.Write(fixture(t, "inspect.json"))
	}))

	got, err := NewClient(sock).InspectContainer(context.Background(), "abc")
	if err != nil {
		t.Fatalf("InspectContainer error: %v", err)
	}
	if got.HostConfig.RestartPolicy.Name != "unless-stopped" || got.State.StartedAt.IsZero() {
		t.Errorf("InspectContainer = %+v, want restart policy and start time", got)
	}
}

func TestClient_HangingDaemonTimesOut(t *testing.T) {
	release := make(chan struct{})
	sock := startDaemon(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(func() { close(release) })
	c := NewClient(sock)
	c.readTimeout = 100 * time.Millisecond

	start := time.Now()
	_, err := c.ListContainers(context.Background())
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want one wrapping context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("ListContainers took %v, want it to stop shortly after the timeout", elapsed)
	}
}

func TestClient_UnreachableSocket(t *testing.T) {
	_, err := NewClient(filepath.Join(t.TempDir(), "missing.sock")).ListContainers(context.Background())
	if err == nil {
		t.Fatal("ListContainers on a missing socket returned nil error")
	}
}

func TestClient_BrokenJSON(t *testing.T) {
	sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not": "a list"`))
	}))

	if _, err := NewClient(sock).ListContainers(context.Background()); err == nil {
		t.Fatal("ListContainers accepted a broken JSON body")
	}
}

func TestClient_Non200IncludesDaemonMessage(t *testing.T) {
	sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom\u001b[2J"}`))
	}))

	_, err := NewClient(sock).ListContainers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want status 500 and the daemon message", err)
	}
	if strings.ContainsRune(err.Error(), 0x1b) {
		t.Errorf("error %q still contains an escape character", err)
	}
}

func TestClient_StopContainer(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"stopped", http.StatusNoContent, false},
		{"already stopped", http.StatusNotModified, false},
		{"no such container", http.StatusNotFound, true},
		{"server error", http.StatusInternalServerError, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.WriteHeader(tt.status)
			}))

			err := NewClient(sock).StopContainer(context.Background(), "abc123")

			if gotMethod != http.MethodPost || gotPath != "/containers/abc123/stop" {
				t.Errorf("request = %s %s, want POST /containers/abc123/stop", gotMethod, gotPath)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("StopContainer error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_StopContainerEscapesID(t *testing.T) {
	var gotPath string
	sock := startDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := NewClient(sock).StopContainer(context.Background(), "a/../b"); err != nil {
		t.Fatalf("StopContainer error: %v", err)
	}
	if gotPath != "/containers/a%2F..%2Fb/stop" {
		t.Errorf("escaped path = %q, want the id escaped into one segment", gotPath)
	}
}
