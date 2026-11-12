package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/socket"
)

type fakeAPI struct {
	containers []Container
	listErr    error
	inspect    Inspect
	inspectErr error
	stopErr    error
	listCalls  int
	inspectIDs []string
	stoppedIDs []string
}

func (f *fakeAPI) ListContainers(context.Context) ([]Container, error) {
	f.listCalls++
	return f.containers, f.listErr
}

func (f *fakeAPI) InspectContainer(_ context.Context, id string) (Inspect, error) {
	f.inspectIDs = append(f.inspectIDs, id)
	return f.inspect, f.inspectErr
}

func (f *fakeAPI) StopContainer(_ context.Context, id string) error {
	f.stoppedIDs = append(f.stoppedIDs, id)
	return f.stopErr
}

func fixtureContainers(t *testing.T) []Container {
	t.Helper()
	var cs []Container
	if err := json.Unmarshal(fixture(t, "containers.json"), &cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

func fixtureInspect(t *testing.T) Inspect {
	t.Helper()
	var in Inspect
	if err := json.Unmarshal(fixture(t, "inspect.json"), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func newTestDetector(api API) *Detector {
	d := New(api)
	d.now = func() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) }
	return d
}

func detail(details []detect.Detail, key string) string {
	for _, d := range details {
		if d.Key == key {
			return d.Value
		}
	}
	return ""
}

func TestDetector_MatchesPublicPortAndProtocol(t *testing.T) {
	tests := []struct {
		name  string
		sock  socket.Socket
		match bool
		title string
	}{
		{"tcp published", socket.Socket{Port: 8125, Protocol: socket.TCP}, true, `Docker container "pw-test" (nginx:1.27)`},
		{"udp published", socket.Socket{Port: 5353, Protocol: socket.UDP}, true, `Docker container "pw-test" (nginx:1.27)`},
		{"wrong protocol", socket.Socket{Port: 8125, Protocol: socket.UDP}, false, ""},
		{"private port only", socket.Socket{Port: 80, Protocol: socket.TCP}, false, ""},
		{"unrelated port", socket.Socket{Port: 9999, Protocol: socket.TCP}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{containers: fixtureContainers(t), inspect: fixtureInspect(t)}

			got, ok := newTestDetector(api).Detect(context.Background(), tt.sock, nil)

			if ok != tt.match {
				t.Fatalf("Detect() ok = %v, want %v", ok, tt.match)
			}
			if ok && got.Title != tt.title {
				t.Errorf("Title = %q, want %q", got.Title, tt.title)
			}
		})
	}
}

func TestDetector_ExplanationDetails(t *testing.T) {
	api := &fakeAPI{containers: fixtureContainers(t), inspect: fixtureInspect(t)}

	got, ok := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if got.Source != "docker" {
		t.Errorf("Source = %q, want docker", got.Source)
	}
	if m := detail(got.Details, "Mapping"); m != "8125→80" {
		t.Errorf("Mapping = %q, want 8125→80", m)
	}
	if u := detail(got.Details, "Uptime"); u != "2d 23h" {
		t.Errorf("Uptime = %q, want 2d 23h", u)
	}
	if p := detail(got.Details, "Restart policy"); p != "unless-stopped" {
		t.Errorf("Restart policy = %q, want unless-stopped", p)
	}
	if len(got.Hints) != 1 || !strings.Contains(got.Hints[0], "unless-stopped") {
		t.Errorf("Hints = %v, want one hint naming the restart policy", got.Hints)
	}
	if got.Terminator == nil || got.Terminator.Describe() != "docker stop pw-test" {
		t.Errorf("Terminator = %v, want docker stop pw-test", got.Terminator)
	}
}

func TestDetector_RestartPolicyHint(t *testing.T) {
	tests := []struct {
		policy   string
		wantHint bool
		wantRow  bool
	}{
		{"always", true, true},
		{"unless-stopped", true, true},
		{"on-failure", false, true},
		{"no", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		t.Run("policy "+tt.policy, func(t *testing.T) {
			in := fixtureInspect(t)
			in.HostConfig.RestartPolicy.Name = tt.policy
			api := &fakeAPI{containers: fixtureContainers(t), inspect: in}

			got, _ := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

			if hasHint := len(got.Hints) > 0; hasHint != tt.wantHint {
				t.Errorf("hint present = %v, want %v", hasHint, tt.wantHint)
			}
			if hasRow := detail(got.Details, "Restart policy") != ""; hasRow != tt.wantRow {
				t.Errorf("restart policy row present = %v, want %v", hasRow, tt.wantRow)
			}
		})
	}
}

func TestDetector_InspectFailureStillExplains(t *testing.T) {
	api := &fakeAPI{containers: fixtureContainers(t), inspectErr: errors.New("boom")}

	got, ok := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	if !ok || got.Terminator == nil {
		t.Fatalf("Detect() = %v, %v, want a match with a Terminator despite the failed inspect", got, ok)
	}
	if detail(got.Details, "Mapping") == "" {
		t.Error("Mapping is missing")
	}
	if len(got.Hints) != 0 {
		t.Errorf("Hints = %v, want none when the restart policy is unknown", got.Hints)
	}
}

func TestDetector_FetchesContainerListOnce(t *testing.T) {
	api := &fakeAPI{containers: fixtureContainers(t), inspect: fixtureInspect(t)}
	d := newTestDetector(api)

	for _, port := range []uint16{8125, 5432, 9999, 8125} {
		d.Detect(context.Background(), socket.Socket{Port: port, Protocol: socket.TCP}, nil)
	}

	if api.listCalls != 1 {
		t.Errorf("container list fetched %d times, want 1", api.listCalls)
	}
}

func TestDetector_FailedListIsNotRetried(t *testing.T) {
	api := &fakeAPI{listErr: errors.New("daemon down")}
	d := newTestDetector(api)

	for range 3 {
		if _, ok := d.Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil); ok {
			t.Fatal("Detect() matched although the daemon is unreachable")
		}
	}

	if api.listCalls != 1 {
		t.Errorf("container list fetched %d times after a failure, want 1", api.listCalls)
	}
}

func TestDetector_SilentFallbackOnHangingDaemon(t *testing.T) {
	release := make(chan struct{})
	sock := startDaemon(t, httpBlock(release))
	t.Cleanup(func() { close(release) })
	c := NewClient(sock)
	c.readTimeout = 100 * time.Millisecond

	start := time.Now()
	_, ok := New(c).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	if ok {
		t.Error("Detect() matched against a hanging daemon")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Detect() did not give up promptly")
	}
}

func TestDetector_CleansExternalStrings(t *testing.T) {
	cs := fixtureContainers(t)
	cs[0].Names = []string{"/evil\x1b[2Jname"}
	cs[0].Image = "img\x07\r\nspoof"
	in := fixtureInspect(t)
	in.HostConfig.RestartPolicy.Name = "always\x1b[0m"
	api := &fakeAPI{containers: cs, inspect: in}

	got, _ := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	var all []string
	all = append(all, got.Title, got.Terminator.Describe())
	all = append(all, got.Hints...)
	for _, d := range got.Details {
		all = append(all, d.Key, d.Value)
	}
	for _, s := range all {
		if strings.ContainsAny(s, "\x1b\x07\r\n") {
			t.Errorf("output %q still contains a control character", s)
		}
	}
}

func TestDetector_NameFallsBackToShortID(t *testing.T) {
	cs := fixtureContainers(t)
	cs[0].Names = nil
	api := &fakeAPI{containers: cs, inspect: fixtureInspect(t)}

	got, _ := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	if !strings.Contains(got.Title, "a1b2c3d4e5f6") {
		t.Errorf("Title = %q, want the short container id", got.Title)
	}
}

func TestTerminator_PostsStop(t *testing.T) {
	api := &fakeAPI{containers: fixtureContainers(t), inspect: fixtureInspect(t)}
	got, _ := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	if err := got.Terminator.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate error: %v", err)
	}

	want := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	if len(api.stoppedIDs) != 1 || api.stoppedIDs[0] != want {
		t.Errorf("stopped %v, want [%s]", api.stoppedIDs, want)
	}
}

func TestTerminator_PropagatesError(t *testing.T) {
	api := &fakeAPI{containers: fixtureContainers(t), inspect: fixtureInspect(t), stopErr: errors.New("denied")}
	got, _ := newTestDetector(api).Detect(context.Background(), socket.Socket{Port: 8125, Protocol: socket.TCP}, nil)

	err := got.Terminator.Terminate(context.Background())

	if err == nil || !strings.Contains(err.Error(), "denied") || !strings.Contains(err.Error(), "pw-test") {
		t.Errorf("Terminate error = %v, want it to wrap the cause and name the container", err)
	}
}

// httpBlock returns a handler that never answers until release is closed.
func httpBlock(release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release })
}
