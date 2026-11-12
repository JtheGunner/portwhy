package detect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

const ownerNotVisibleTitle = "owner not visible"

// generic is the fallback detector: it always matches and describes the bare
// process. Its Terminator stays nil until the kill core is wired in.
type generic struct {
	now func() time.Time
}

// Generic returns the fallback detector that always matches.
func Generic() Detector {
	return generic{now: time.Now}
}

func (g generic) Detect(_ context.Context, _ socket.Socket, p *proc.Process) (*Explanation, bool) {
	e := &Explanation{Source: SourceProcess, Title: ownerNotVisibleTitle}
	if p == nil {
		return e, true
	}

	e.Title = p.Name
	add := func(key, value string) {
		if value != "" {
			e.Details = append(e.Details, Detail{Key: key, Value: value})
		}
	}
	add("Command", strings.Join(p.Cmdline, " "))
	add("User", p.User)
	if !p.Started.IsZero() {
		add("Uptime", formatUptime(g.now().Sub(p.Started)))
	}
	add("Cwd", p.Cwd)
	add("Started by", ancestorChain(*p))
	return e, true
}

// ancestorChain renders the process and its parents, e.g. "node ← npm ← zsh".
// It returns "" when there are no ancestors.
func ancestorChain(p proc.Process) string {
	if len(p.Ancestors) == 0 {
		return ""
	}
	names := []string{p.Name}
	for _, a := range p.Ancestors {
		names = append(names, a.Name)
	}
	return strings.Join(names, " ← ")
}

// formatUptime renders a duration using its two most significant units.
func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int(d / time.Minute)
	seconds := int((d - time.Duration(minutes)*time.Minute) / time.Second)

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}
