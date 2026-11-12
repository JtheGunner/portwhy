package launchd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const testUID = 501

var testSocket = socket.Socket{Port: 9000, Protocol: socket.TCP}

func listRunner(t *testing.T) *execx.Fake {
	t.Helper()
	return &execx.Fake{Responses: map[string][]byte{"launchctl list": fixture(t, "launchctl_list.txt")}}
}

func process(pid, ppid int, name string) *proc.Process {
	return &proc.Process{
		PID:     pid,
		PPID:    ppid,
		Name:    name,
		Started: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC),
	}
}

func newTestDetector(query, stop execx.Runner, uid int) *Detector {
	d := NewDetector(query, stop, uid)
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

func TestDetector_HomebrewService(t *testing.T) {
	got, ok := newTestDetector(listRunner(t), &execx.Fake{}, testUID).
		Detect(context.Background(), testSocket, process(3884, 1, "php-fpm"))

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if got.Source != detect.SourceLaunchd {
		t.Errorf("Source = %q, want launchd", got.Source)
	}
	if got.Title != `Homebrew service "php@8.4"` {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Terminator == nil || got.Terminator.Describe() != "brew services stop php@8.4" {
		t.Errorf("Terminator = %v, want brew services stop php@8.4", got.Terminator)
	}
	if v := detail(got.Details, "Label"); v != "homebrew.mxcl.php@8.4" {
		t.Errorf("Label = %q", v)
	}
	if v := detail(got.Details, "Process"); v != "php-fpm (PID 3884)" {
		t.Errorf("Process = %q", v)
	}
	if v := detail(got.Details, "Uptime"); v != "3d 0h" {
		t.Errorf("Uptime = %q, want 3d 0h", v)
	}
	if len(got.Hints) != 0 {
		t.Errorf("Hints = %v, want none for a Homebrew service", got.Hints)
	}
}

func TestDetector_PlainLaunchdJob(t *testing.T) {
	got, ok := newTestDetector(listRunner(t), &execx.Fake{}, testUID).
		Detect(context.Background(), testSocket, process(3863, 1, "node"))

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if got.Title != `launchd job "ai.openclaw.gateway"` {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Terminator == nil || got.Terminator.Describe() != "launchctl bootout gui/501/ai.openclaw.gateway" {
		t.Errorf("Terminator = %v, want launchctl bootout gui/501/ai.openclaw.gateway", got.Terminator)
	}
	if v := detail(got.Details, "Domain"); v != "gui/501" {
		t.Errorf("Domain = %q, want gui/501", v)
	}
	if len(got.Hints) != 1 || !strings.Contains(got.Hints[0], "KeepAlive") {
		t.Errorf("Hints = %v, want one hint about KeepAlive", got.Hints)
	}
}

func TestDetector_FallsBack(t *testing.T) {
	tests := []struct {
		name string
		p    *proc.Process
	}{
		{"no process", nil},
		{"child of a shell is not a launchd job", process(3884, 4242, "php-fpm")},
		{"process without a label", process(99999, 1, "stray")},
		{"apple system agent", process(1113, 1, "rapportd")},
		{"gui application instance", process(13250, 1, "Player")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := newTestDetector(listRunner(t), &execx.Fake{}, testUID).Detect(context.Background(), testSocket, tt.p)

			if ok || got != nil {
				t.Errorf("Detect() = %v, %v, want no match", got, ok)
			}
		})
	}
}

func TestDetector_RootIsNotMatched(t *testing.T) {
	query := listRunner(t)

	_, ok := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, process(3884, 1, "php-fpm"))

	if ok {
		t.Error("Detect() matched as root")
	}
	if calls := query.Calls(); len(calls) != 0 {
		t.Errorf("ran %v, want no launchctl call as root", calls)
	}
}

func TestDetector_LaunchctlFailureFallsBack(t *testing.T) {
	query := &execx.Fake{Errors: map[string]error{"launchctl list": errors.New("launchctl list: exit status 1")}}

	got, ok := newTestDetector(query, &execx.Fake{}, testUID).Detect(context.Background(), testSocket, process(3884, 1, "php-fpm"))

	if ok || got != nil {
		t.Errorf("Detect() = %v, %v, want no match when launchctl fails", got, ok)
	}
}

func TestDetector_ListFetchedOnce(t *testing.T) {
	query := listRunner(t)
	d := newTestDetector(query, &execx.Fake{}, testUID)

	for _, pid := range []int{3884, 3863, 99999, 3884} {
		d.Detect(context.Background(), testSocket, process(pid, 1, "x"))
	}

	if calls := query.Calls(); len(calls) != 1 {
		t.Errorf("launchctl list ran %d times (%v), want 1", len(calls), calls)
	}
}

func TestDetector_FailedListIsNotRetried(t *testing.T) {
	query := &execx.Fake{Errors: map[string]error{"launchctl list": errors.New("boom")}}
	d := newTestDetector(query, &execx.Fake{}, testUID)

	for range 3 {
		d.Detect(context.Background(), testSocket, process(3884, 1, "php-fpm"))
	}

	if calls := query.Calls(); len(calls) != 1 {
		t.Errorf("launchctl list ran %d times after a failure, want 1", len(calls))
	}
}

func TestDetector_InvalidBrewNameFallsBackToLaunchdJob(t *testing.T) {
	for _, label := range []string{"homebrew.mxcl.--evil", "homebrew.mxcl.", "homebrew.mxcl.a b"} {
		t.Run(label, func(t *testing.T) {
			query := &execx.Fake{Responses: map[string][]byte{
				"launchctl list": []byte("PID\tStatus\tLabel\n700\t0\t" + label + "\n"),
			}}

			got, ok := newTestDetector(query, &execx.Fake{}, testUID).Detect(context.Background(), testSocket, process(700, 1, "x"))

			if !ok {
				t.Fatal("Detect() did not match")
			}
			if !strings.HasPrefix(got.Title, "launchd job") {
				t.Errorf("Title = %q, want a plain launchd job", got.Title)
			}
			if !strings.HasPrefix(got.Terminator.Describe(), "launchctl bootout gui/501/") {
				t.Errorf("Describe() = %q, want launchctl bootout", got.Terminator.Describe())
			}
		})
	}
}

func TestDetector_CleansExternalStrings(t *testing.T) {
	query := &execx.Fake{Responses: map[string][]byte{
		"launchctl list": []byte("PID\tStatus\tLabel\n700\t0\tcom.example.evil\x1b[2Jlabel\n701\t0\thomebrew.mxcl.ok\n"),
	}}
	p := process(700, 1, "evil\x1b[2Jname")

	got, ok := newTestDetector(query, &execx.Fake{}, testUID).Detect(context.Background(), testSocket, p)

	if !ok {
		t.Fatal("Detect() did not match")
	}
	all := []string{got.Title, got.Terminator.Describe()}
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

func TestTerminator_RunsCommand(t *testing.T) {
	tests := []struct {
		name string
		pid  int
		want string
	}{
		{"homebrew service", 3884, "brew services stop php@8.4"},
		{"launchd job", 3863, "launchctl bootout gui/501/ai.openclaw.gateway"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stop := &execx.Fake{Responses: map[string][]byte{tt.want: nil}}
			got, _ := newTestDetector(listRunner(t), stop, testUID).Detect(context.Background(), testSocket, process(tt.pid, 1, "x"))

			if err := got.Terminator.Terminate(context.Background()); err != nil {
				t.Fatalf("Terminate error: %v", err)
			}
			if calls := stop.Calls(); len(calls) != 1 || calls[0] != tt.want {
				t.Errorf("ran %v, want [%s]", calls, tt.want)
			}
		})
	}
}

func TestTerminator_ErrorIsCleanedAndWrapped(t *testing.T) {
	cause := errors.New("brew: boom\x1b[2J\rspoof")
	stop := &execx.Fake{Errors: map[string]error{"brew services stop php@8.4": cause}}
	got, _ := newTestDetector(listRunner(t), stop, testUID).Detect(context.Background(), testSocket, process(3884, 1, "php-fpm"))

	err := got.Terminator.Terminate(context.Background())

	if err == nil || strings.ContainsAny(err.Error(), "\x1b\r") {
		t.Fatalf("error = %q, want it free of control characters", err)
	}
	if !strings.Contains(err.Error(), "php@8.4") {
		t.Errorf("error %q does not name the service", err)
	}
	if !errors.Is(err, cause) {
		t.Error("error does not wrap the cause")
	}
}
