package detect

import (
	"context"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

type stubDetector struct {
	explanation *Explanation
	hit         bool
	calls       int
}

func (d *stubDetector) Detect(context.Context, socket.Socket, *proc.Process) (*Explanation, bool) {
	d.calls++
	return d.explanation, d.hit
}

func TestChain_FirstHitWins(t *testing.T) {
	miss := &stubDetector{}
	first := &stubDetector{explanation: &Explanation{Title: "first"}, hit: true}
	second := &stubDetector{explanation: &Explanation{Title: "second"}, hit: true}

	got, ok := Chain(miss, first, second).Detect(context.Background(), socket.Socket{}, nil)

	if !ok || got.Title != "first" {
		t.Fatalf("Detect() = %v, %v, want first hit", got, ok)
	}
	if miss.calls != 1 || second.calls != 0 {
		t.Errorf("calls: miss=%d second=%d, want 1 and 0", miss.calls, second.calls)
	}
}

func TestChain_NoHit(t *testing.T) {
	got, ok := Chain(&stubDetector{}, &stubDetector{}).Detect(context.Background(), socket.Socket{}, nil)

	if ok || got != nil {
		t.Errorf("Detect() = %v, %v, want no hit", got, ok)
	}
}

func TestChain_Empty(t *testing.T) {
	if _, ok := Chain().Detect(context.Background(), socket.Socket{}, nil); ok {
		t.Error("empty chain matched")
	}
}

func details(e *Explanation) map[string]string {
	m := make(map[string]string)
	for _, d := range e.Details {
		m[d.Key] = d.Value
	}
	return m
}

func TestGeneric_AlwaysMatchesAndListsAncestors(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	d := generic{now: func() time.Time { return now }}
	p := &proc.Process{
		PID:     42,
		Name:    "node",
		Cmdline: []string{"node", "server.js", "--port", "3000"},
		User:    "jeffry",
		Started: now.Add(-(2*time.Hour + 5*time.Minute)),
		Cwd:     "/home/jeffry/app",
		Ancestors: []proc.Ancestor{
			{PID: 40, Name: "npm"},
			{PID: 30, Name: "zsh"},
		},
	}

	got, ok := d.Detect(context.Background(), socket.Socket{Port: 3000}, p)

	if !ok {
		t.Fatal("generic did not match")
	}
	if got.Source != SourceProcess || got.Title != "node" {
		t.Errorf("Source/Title = %q/%q, want process/node", got.Source, got.Title)
	}
	if got.Terminator != nil {
		t.Errorf("Terminator = %v, want nil", got.Terminator)
	}
	want := map[string]string{
		"Command":    "node server.js --port 3000",
		"User":       "jeffry",
		"Uptime":     "2h 5m",
		"Cwd":        "/home/jeffry/app",
		"Started by": "node ← npm ← zsh",
	}
	gotDetails := details(got)
	for k, v := range want {
		if gotDetails[k] != v {
			t.Errorf("detail %q = %q, want %q", k, gotDetails[k], v)
		}
	}
}

func TestGeneric_OmitsUnknownDetails(t *testing.T) {
	d := Generic()

	got, ok := d.Detect(context.Background(), socket.Socket{}, &proc.Process{Name: "sshd"})

	if !ok || got.Title != "sshd" {
		t.Fatalf("Detect() = %v, %v", got, ok)
	}
	if len(got.Details) != 0 {
		t.Errorf("Details = %v, want none", got.Details)
	}
}

func TestGeneric_NilProcessIsOwnerNotVisible(t *testing.T) {
	got, ok := Generic().Detect(context.Background(), socket.Socket{Port: 22}, nil)

	if !ok {
		t.Fatal("generic did not match")
	}
	if got.Title != "owner not visible" || len(got.Details) != 0 {
		t.Errorf("Explanation = %+v", got)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{-time.Second, "0s"},
		{45 * time.Second, "45s"},
		{3*time.Minute + 10*time.Second, "3m 10s"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
		{50*time.Hour + 30*time.Minute, "2d 2h"},
	}
	for _, tt := range tests {
		if got := formatUptime(tt.in); got != tt.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
