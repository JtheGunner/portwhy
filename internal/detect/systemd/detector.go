package systemd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

// Detector explains a socket whose owner runs inside a systemd service. It
// never matches when systemctl cannot be queried (not installed, systemd not
// running, unit unknown), so the next detector in the chain takes over.
type Detector struct {
	query execx.Runner
	stop  execx.Runner
	uid   int
	// own is the unit portwhy itself runs in, or the zero value when it runs
	// outside any service. Stopping that unit would end portwhy and everything
	// else inside it, so it is never matched.
	own cgroupUnit
	now func() time.Time
}

// NewDetector returns a Detector for the user with the given uid. query runs
// the quick systemctl lookups; stop runs "systemctl stop" and needs a much
// longer timeout, because systemd waits for the service to shut down.
// ownCgroup is the cgroup path of the portwhy process itself.
func NewDetector(query, stop execx.Runner, uid int, ownCgroup string) *Detector {
	own, _ := unitFromCgroup(ownCgroup)
	return &Detector{query: query, stop: stop, uid: uid, own: own, now: time.Now}
}

// Detect implements detect.Detector.
func (d *Detector) Detect(ctx context.Context, _ socket.Socket, p *proc.Process) (*detect.Explanation, bool) {
	if p == nil || p.Cgroup == "" {
		return nil, false
	}
	unit, ok := unitFromCgroup(p.Cgroup)
	if !ok {
		return nil, false
	}
	// "systemctl --user" always talks to the manager of the current user, so a
	// unit of another user's manager can neither be queried nor stopped.
	if unit.User && unit.UID != d.uid {
		return nil, false
	}

	// A port held inside the unit portwhy runs in falls through to the generic
	// detector, which stops just that process instead of the whole unit.
	if unit == d.own {
		return nil, false
	}

	out, err := d.query.Run(ctx, "systemctl", systemctlArgs(unit.User, "show", "-p", "Restart", "--value", unit.Name)...)
	if err != nil {
		return nil, false
	}
	restart := strings.TrimSpace(string(out))

	scope := "system"
	if unit.User {
		scope = "user"
	}
	e := &detect.Explanation{
		Source: detect.SourceSystemd,
		Title:  fmt.Sprintf("systemd service %q", textutil.Clean(unit.Name)),
		Details: []detect.Detail{
			{Key: "Scope", Value: scope},
			{Key: "Process", Value: fmt.Sprintf("%s (PID %d)", textutil.Clean(p.Name), p.PID)},
		},
		Terminator: newTerminator(d.stop, unit, d.uid),
	}
	if !p.Started.IsZero() {
		e.Details = append(e.Details, detect.Detail{Key: "Uptime", Value: detect.FormatUptime(d.now().Sub(p.Started))})
	}
	if restart != "" && restart != "no" {
		clean := textutil.Clean(restart)
		e.Details = append(e.Details, detect.Detail{Key: "Restart", Value: clean})
		e.Hints = append(e.Hints, fmt.Sprintf(
			"Restart=%s: systemd restarts the process after a plain kill; stopping the unit keeps it down", clean))
	}
	return e, true
}

// systemctlArgs builds the argument list of a systemctl call, selecting the
// per-user manager when user is set.
func systemctlArgs(user bool, args ...string) []string {
	if user {
		return append([]string{"--user"}, args...)
	}
	return args
}

// terminator stops a service with systemctl.
type terminator struct {
	run  execx.Runner
	unit cgroupUnit
	uid  int
}

var _ kill.Terminator = (*terminator)(nil)

func newTerminator(run execx.Runner, unit cgroupUnit, uid int) *terminator {
	return &terminator{run: run, unit: unit, uid: uid}
}

// needsSudo reports whether stopping this unit takes root: system units do,
// unless portwhy already runs as root.
func (t *terminator) needsSudo() bool {
	return !t.unit.User && t.uid != 0
}

func (t *terminator) command() string {
	return execx.Key("systemctl", systemctlArgs(t.unit.User, "stop", t.unit.Name)...)
}

// Describe returns the command a user would type. For a system unit and a
// non-root user that includes sudo, although Terminate itself never escalates.
func (t *terminator) Describe() string {
	cmd := textutil.Clean(t.command())
	if t.needsSudo() {
		return "sudo " + cmd
	}
	return cmd
}

// Terminate runs systemctl stop without sudo. When systemctl refuses for lack
// of rights, the error says to rerun portwhy with sudo.
func (t *terminator) Terminate(ctx context.Context) error {
	_, err := t.run.Run(ctx, "systemctl", systemctlArgs(t.unit.User, "stop", t.unit.Name)...)
	if err == nil {
		return nil
	}
	cleaned := cleanedError{err: err}
	if t.needsSudo() && isPermissionError(err) {
		return fmt.Errorf("not permitted to stop %s: rerun portwhy with sudo (%s): %w",
			textutil.Clean(t.unit.Name), t.Describe(), cleaned)
	}
	return fmt.Errorf("stop %s: %w", textutil.Clean(t.unit.Name), cleaned)
}

// isPermissionError recognises systemctl's complaints about missing rights.
func isPermissionError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"access denied", "authentication", "permission denied", "not permitted"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// cleanedError wraps an error whose text comes from an external command, so
// that its message is safe to print while errors.Is and errors.As still see
// the original.
type cleanedError struct{ err error }

func (e cleanedError) Error() string { return textutil.Clean(e.err.Error()) }
func (e cleanedError) Unwrap() error { return e.err }

var _ error = cleanedError{}
