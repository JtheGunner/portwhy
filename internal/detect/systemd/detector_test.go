package systemd

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const (
	systemCgroup = "/system.slice/postgresql.service"
	userCgroup   = "/user.slice/user-1000.slice/user@1000.service/app.slice/foo.service"
)

var testSocket = socket.Socket{Port: 5432, Protocol: socket.TCP}

func process(cgroup string) *proc.Process {
	return &proc.Process{
		PID:     812,
		Name:    "postgres",
		Cgroup:  cgroup,
		Started: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC),
	}
}

func newTestDetector(query, stop execx.Runner, uid int) *Detector {
	d := NewDetector(query, stop, uid, "")
	d.now = func() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) }
	return d
}

// showRunner answers the show call with the given Restart value and no
// MainPID line.
func showRunner(key, restart string) *execx.Fake {
	return showOutput(key, "Restart="+restart+"\n")
}

func showOutput(key, output string) *execx.Fake {
	return &execx.Fake{Responses: map[string][]byte{key: []byte(output)}}
}

const (
	showSystem = "systemctl show -p Restart -p MainPID postgresql.service"
	showUser   = "systemctl --user show -p Restart -p MainPID foo.service"
	stopHint   = "systemctl stop ends the whole unit, including every process in it"
)

func hasHint(hints []string, want string) bool {
	for _, h := range hints {
		if h == want {
			return true
		}
	}
	return false
}

func detail(details []detect.Detail, key string) string {
	for _, d := range details {
		if d.Key == key {
			return d.Value
		}
	}
	return ""
}

func TestDetector_ExplainsSystemService(t *testing.T) {
	query := showRunner("systemctl show -p Restart -p MainPID postgresql.service", "always")

	got, ok := newTestDetector(query, &execx.Fake{}, 1000).Detect(context.Background(), testSocket, process(systemCgroup))

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if got.Source != detect.SourceSystemd {
		t.Errorf("Source = %q, want systemd", got.Source)
	}
	if got.Title != `systemd service "postgresql.service"` {
		t.Errorf("Title = %q", got.Title)
	}
	if v := detail(got.Details, "Scope"); v != "system" {
		t.Errorf("Scope = %q, want system", v)
	}
	if v := detail(got.Details, "Process"); v != "postgres (PID 812)" {
		t.Errorf("Process = %q, want postgres (PID 812)", v)
	}
	if v := detail(got.Details, "Uptime"); v != "3d 0h" {
		t.Errorf("Uptime = %q, want 3d 0h", v)
	}
	if v := detail(got.Details, "Restart"); v != "always" {
		t.Errorf("Restart = %q, want always", v)
	}
	if got.Terminator == nil || got.Terminator.Describe() != "sudo systemctl stop postgresql.service" {
		t.Errorf("Terminator = %v, want sudo systemctl stop postgresql.service", got.Terminator)
	}
}

func TestDetector_ExplainsUserService(t *testing.T) {
	query := showRunner("systemctl --user show -p Restart -p MainPID foo.service", "no")

	got, ok := newTestDetector(query, &execx.Fake{}, 1000).Detect(context.Background(), testSocket, process(userCgroup))

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if v := detail(got.Details, "Scope"); v != "user" {
		t.Errorf("Scope = %q, want user", v)
	}
	if got.Terminator.Describe() != "systemctl --user stop foo.service" {
		t.Errorf("Describe() = %q", got.Terminator.Describe())
	}
}

func TestDetector_RestartHint(t *testing.T) {
	tests := []struct {
		restart  string
		wantHint bool
		wantRow  bool
	}{
		{"always", true, true},
		{"on-failure", true, true},
		{"no", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		t.Run("Restart="+tt.restart, func(t *testing.T) {
			query := showRunner("systemctl show -p Restart -p MainPID postgresql.service", tt.restart)

			got, _ := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, process(systemCgroup))

			if gotHint := len(got.Hints) > 1; gotHint != tt.wantHint {
				t.Errorf("hint present = %v, want %v (hints %v)", gotHint, tt.wantHint, got.Hints)
			}
			if tt.wantHint && !strings.Contains(got.Hints[0], "Restart="+tt.restart) {
				t.Errorf("hint %q does not name Restart=%s", got.Hints[0], tt.restart)
			}
			if hasRow := detail(got.Details, "Restart") != ""; hasRow != tt.wantRow {
				t.Errorf("restart row present = %v, want %v", hasRow, tt.wantRow)
			}
		})
	}
}

func TestDetector_FallsBackSilently(t *testing.T) {
	missing := &exec.Error{Name: "systemctl", Err: exec.ErrNotFound}
	tests := []struct {
		name  string
		query execx.Runner
		p     *proc.Process
	}{
		{"systemctl missing", &execx.Fake{Errors: map[string]error{"systemctl show -p Restart -p MainPID postgresql.service": missing}}, process(systemCgroup)},
		{"systemd not running", &execx.Fake{Errors: map[string]error{"systemctl show -p Restart -p MainPID postgresql.service": errors.New("System has not been booted with systemd")}}, process(systemCgroup)},
		{"no process", &execx.Fake{}, nil},
		{"no cgroup", &execx.Fake{}, process("")},
		{"scope only", &execx.Fake{}, process("/user.slice/user-1000.slice/session-3.scope")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := newTestDetector(tt.query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, tt.p)
			if ok || got != nil {
				t.Errorf("Detect() = %v, %v, want no match", got, ok)
			}
		})
	}
}

func TestDetector_OtherUsersUserUnitIsNotMatched(t *testing.T) {
	query := &execx.Fake{}

	_, ok := newTestDetector(query, &execx.Fake{}, 1001).Detect(context.Background(), testSocket, process(userCgroup))

	if ok {
		t.Error("Detect() matched a user unit that belongs to another user")
	}
	if calls := query.Calls(); len(calls) != 0 {
		t.Errorf("ran %v, want no systemctl call for a foreign user unit", calls)
	}
}

func TestDetector_CleansExternalStrings(t *testing.T) {
	query := showOutput(showSystem, "Restart=always\x1b[2J\nMainPID=7\x1b[2J\n")
	p := process(systemCgroup)
	p.Name = "evil\x1b[2Jname"

	got, _ := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, p)

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

func TestTerminator_DescribeUserVsSystem(t *testing.T) {
	tests := []struct {
		name string
		unit cgroupUnit
		uid  int
		want string
	}{
		{"system unit as root", cgroupUnit{Name: "a.service"}, 0, "systemctl stop a.service"},
		{"system unit as user", cgroupUnit{Name: "a.service"}, 1000, "sudo systemctl stop a.service"},
		{"user unit", cgroupUnit{Name: "a.service", User: true, UID: 1000}, 1000, "systemctl --user stop a.service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newTerminator(&execx.Fake{}, tt.unit, tt.uid).Describe(); got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTerminator_StopsWithoutSudo(t *testing.T) {
	tests := []struct {
		name string
		unit cgroupUnit
		uid  int
		want string
	}{
		{"system unit as user", cgroupUnit{Name: "a.service"}, 1000, "systemctl stop a.service"},
		{"user unit", cgroupUnit{Name: "a.service", User: true, UID: 1000}, 1000, "systemctl --user stop a.service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stop := &execx.Fake{Responses: map[string][]byte{tt.want: nil}}

			if err := newTerminator(stop, tt.unit, tt.uid).Terminate(context.Background()); err != nil {
				t.Fatalf("Terminate error: %v", err)
			}
			if calls := stop.Calls(); len(calls) != 1 || calls[0] != tt.want {
				t.Errorf("ran %v, want [%s]", calls, tt.want)
			}
		})
	}
}

func TestTerminator_PermissionErrorSuggestsSudo(t *testing.T) {
	for _, msg := range []string{
		"Failed to stop a.service: Interactive authentication required.",
		"Failed to stop a.service: Access denied",
		"Failed to stop a.service: Permission denied",
	} {
		t.Run(msg, func(t *testing.T) {
			cause := errors.New("systemctl stop a.service: exit status 1: " + msg)
			stop := &execx.Fake{Errors: map[string]error{"systemctl stop a.service": cause}}

			err := newTerminator(stop, cgroupUnit{Name: "a.service"}, 1000).Terminate(context.Background())

			if err == nil || !strings.Contains(err.Error(), "sudo systemctl stop a.service") {
				t.Fatalf("error = %v, want the sudo suggestion", err)
			}
			if !errors.Is(err, cause) {
				t.Error("error does not wrap the cause")
			}
		})
	}
}

func TestTerminator_OtherErrorPropagates(t *testing.T) {
	cause := errors.New("systemctl stop a.service: exit status 5: Unit a.service not loaded.")
	stop := &execx.Fake{Errors: map[string]error{"systemctl stop a.service": cause}}

	err := newTerminator(stop, cgroupUnit{Name: "a.service"}, 1000).Terminate(context.Background())

	if err == nil || strings.Contains(err.Error(), "sudo") {
		t.Fatalf("error = %v, want the plain failure without a sudo suggestion", err)
	}
	if !errors.Is(err, cause) {
		t.Error("error does not wrap the cause")
	}
}

func TestTerminator_UserUnitPermissionErrorHasNoSudoHint(t *testing.T) {
	cause := errors.New("Access denied")
	stop := &execx.Fake{Errors: map[string]error{"systemctl --user stop a.service": cause}}

	err := newTerminator(stop, cgroupUnit{Name: "a.service", User: true, UID: 1000}, 1000).Terminate(context.Background())

	if err == nil || strings.Contains(err.Error(), "sudo") {
		t.Fatalf("error = %v, want no sudo suggestion for a user unit", err)
	}
}

func TestTerminator_CleansErrorText(t *testing.T) {
	cause := errors.New("boom\x1b[2J\rspoof")
	stop := &execx.Fake{Errors: map[string]error{"systemctl stop a.service": cause}}

	err := newTerminator(stop, cgroupUnit{Name: "a.service"}, 0).Terminate(context.Background())

	if err == nil || strings.ContainsAny(err.Error(), "\x1b\r") {
		t.Fatalf("error = %q, want it free of control characters", err)
	}
	if !errors.Is(err, cause) {
		t.Error("error does not wrap the cause")
	}
}

func TestDetector_OwnUnitIsNotMatched(t *testing.T) {
	// portwhy itself runs inside postgresql.service (one of its sub cgroups):
	// stopping that unit would end portwhy and everything else in it.
	tests := []struct {
		name      string
		ownCgroup string
		cgroup    string
		wantMatch bool
	}{
		{"same system unit", "/system.slice/postgresql.service", systemCgroup, false},
		{"same system unit, own sub cgroup", "/system.slice/postgresql.service/worker", systemCgroup, false},
		{"same user unit", userCgroup, userCgroup, false},
		{"different system unit", "/system.slice/nginx.service", systemCgroup, true},
		{"system and user unit of the same name differ", "/system.slice/foo.service", userCgroup, true},
		{"own cgroup is a session scope", "/user.slice/user-1000.slice/session-3.scope", systemCgroup, true},
		{"own cgroup unknown", "", systemCgroup, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &execx.Fake{Responses: map[string][]byte{
				"systemctl show -p Restart -p MainPID postgresql.service": []byte("Restart=no\nMainPID=812\n"),
				"systemctl --user show -p Restart -p MainPID foo.service": []byte("Restart=no\nMainPID=812\n"),
			}}

			_, ok := NewDetector(query, &execx.Fake{}, 1000, tt.ownCgroup).Detect(context.Background(), testSocket, process(tt.cgroup))

			if ok != tt.wantMatch {
				t.Errorf("Detect() matched = %v, want %v", ok, tt.wantMatch)
			}
			if !tt.wantMatch && len(query.Calls()) != 0 {
				t.Errorf("ran %v, want no systemctl call for the own unit", query.Calls())
			}
		})
	}
}

func TestDetector_RoleOfHolder(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantRole string
		wantRow  bool
	}{
		{"master is the main process", "MainPID=812\nRestart=no\n", "main process of the unit", true},
		{"worker is a child process", "MainPID=800\nRestart=no\n", "child process of the unit (main PID 800)", true},
		{"property order does not matter", "Restart=no\nMainPID=800\n", "child process of the unit (main PID 800)", true},
		{"MainPID missing", "Restart=no\n", "", false},
		{"MainPID zero", "MainPID=0\nRestart=no\n", "", false},
		{"MainPID empty", "MainPID=\nRestart=no\n", "", false},
		{"MainPID garbage", "MainPID=abc\nRestart=no\n", "", false},
		{"MainPID negative", "MainPID=-5\nRestart=no\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := showOutput(showSystem, tt.output)

			got, ok := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, process(systemCgroup))

			if !ok {
				t.Fatal("Detect() did not match")
			}
			if v := detail(got.Details, "Role"); v != tt.wantRole {
				t.Errorf("Role = %q, want %q", v, tt.wantRole)
			}
			if !hasHint(got.Hints, stopHint) {
				t.Errorf("hints %v lack %q", got.Hints, stopHint)
			}
		})
	}
}

func TestDetector_RoleFollowsProcess(t *testing.T) {
	query := showOutput(showSystem, "MainPID=812\nRestart=no\n")

	got, _ := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, process(systemCgroup))

	var keys []string
	for _, d := range got.Details {
		keys = append(keys, d.Key)
	}
	want := []string{"Scope", "Process", "Role", "Uptime"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("detail keys = %v, want %v", keys, want)
	}
}

func TestDetector_UserUnitQueriesUserManager(t *testing.T) {
	query := showOutput(showUser, "MainPID=900\nRestart=on-failure\n")

	got, ok := newTestDetector(query, &execx.Fake{}, 1000).Detect(context.Background(), testSocket, process(userCgroup))

	if !ok {
		t.Fatal("Detect() did not match")
	}
	if v := detail(got.Details, "Role"); v != "child process of the unit (main PID 900)" {
		t.Errorf("Role = %q", v)
	}
	if !hasHint(got.Hints, stopHint) {
		t.Errorf("hints %v lack %q", got.Hints, stopHint)
	}
}

func TestDetector_ShowFailureFallsBack(t *testing.T) {
	query := &execx.Fake{Errors: map[string]error{showSystem: errors.New("exit status 1")}}

	got, ok := newTestDetector(query, &execx.Fake{}, 0).Detect(context.Background(), testSocket, process(systemCgroup))

	if ok || got != nil {
		t.Errorf("Detect() = %v, %v, want no match", got, ok)
	}
}
