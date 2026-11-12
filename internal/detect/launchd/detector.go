package launchd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/execx"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// launchdPID is the parent PID of every process that launchd starts itself.
	launchdPID = 1

	homebrewPrefix = "homebrew.mxcl."
	// Labels that are never matched. GUI application instances are best ended
	// with a plain signal, and unloading an Apple system agent can disturb the
	// login session. Both fall through to the generic detector.
	applePrefix       = "com.apple."
	applicationPrefix = "application."
)

// formulaNameRE accepts Homebrew formula names such as "php@8.4". It cannot
// match a name that starts with '-', so a name is never read as an option.
var formulaNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._+-]*$`)

// Detector explains a socket whose owner is a launchd job of the current
// user. It only knows the user's own domain, so system daemons and every job
// when running as root fall through to the next detector, as does everything
// when launchctl cannot be queried.
//
// The job list is read at most once per Detector.
type Detector struct {
	query execx.Runner
	stop  execx.Runner
	uid   int
	now   func() time.Time

	once   sync.Once
	labels map[int]string
}

// NewDetector returns a Detector for the user with the given uid. query runs
// the quick launchctl listing; stop runs "launchctl bootout" and "brew
// services stop", which wait for the process to end and need a longer timeout.
func NewDetector(query, stop execx.Runner, uid int) *Detector {
	return &Detector{query: query, stop: stop, uid: uid, now: time.Now}
}

// Detect implements detect.Detector.
func (d *Detector) Detect(ctx context.Context, _ socket.Socket, p *proc.Process) (*detect.Explanation, bool) {
	// The system domain (root) is out of scope, and "gui/0" does not exist.
	if d.uid == 0 || p == nil || p.PPID != launchdPID {
		return nil, false
	}
	label := d.labelOf(ctx, p.PID)
	if label == "" || strings.HasPrefix(label, applePrefix) || strings.HasPrefix(label, applicationPrefix) {
		return nil, false
	}

	e := &detect.Explanation{Source: detect.SourceLaunchd}
	if name, ok := homebrewName(label); ok {
		e.Title = fmt.Sprintf("Homebrew service %q", textutil.Clean(name))
		e.Details = append(e.Details, detect.Detail{Key: "Label", Value: textutil.Clean(label)})
		e.Terminator = newTerminator(d.stop, label, "brew", "services", "stop", name)
	} else {
		domain := "gui/" + strconv.Itoa(d.uid)
		e.Title = fmt.Sprintf("launchd job %q", textutil.Clean(label))
		e.Details = append(e.Details, detect.Detail{Key: "Domain", Value: domain})
		e.Hints = []string{"launchd restarts it after a plain kill if KeepAlive is set; unloading the job keeps it down until the next login"}
		e.Terminator = newTerminator(d.stop, label, "launchctl", "bootout", domain+"/"+label)
	}
	e.Details = append(e.Details, detect.Detail{Key: "Process", Value: fmt.Sprintf("%s (PID %d)", textutil.Clean(p.Name), p.PID)})
	if !p.Started.IsZero() {
		e.Details = append(e.Details, detect.Detail{Key: "Uptime", Value: detect.FormatUptime(d.now().Sub(p.Started))})
	}
	return e, true
}

// labelOf returns the launchd label of pid, or "" when pid is not a job.
func (d *Detector) labelOf(ctx context.Context, pid int) string {
	d.once.Do(func() {
		// A failed listing leaves the map empty and is not retried: every port
		// of the run then falls through to the generic detector at once.
		if out, err := d.query.Run(ctx, "launchctl", "list"); err == nil {
			d.labels = parseLaunchctlList(out)
		}
	})
	return d.labels[pid]
}

// homebrewName returns the formula name of a Homebrew service label.
func homebrewName(label string) (string, bool) {
	name, ok := strings.CutPrefix(label, homebrewPrefix)
	if !ok || !formulaNameRE.MatchString(name) {
		return "", false
	}
	return name, true
}

// terminator stops a job with an external command.
type terminator struct {
	run   execx.Runner
	label string
	name  string
	args  []string
}

var _ kill.Terminator = (*terminator)(nil)

func newTerminator(run execx.Runner, label, name string, args ...string) *terminator {
	return &terminator{run: run, label: label, name: name, args: args}
}

func (t *terminator) Describe() string {
	return textutil.Clean(execx.Key(t.name, t.args...))
}

func (t *terminator) Terminate(ctx context.Context) error {
	if _, err := t.run.Run(ctx, t.name, t.args...); err != nil {
		return fmt.Errorf("stop %s: %w", textutil.Clean(t.label), textutil.CleanError(err))
	}
	return nil
}
