// Package advise turns what is known about a busy port into short pieces of
// advice. Each rule is a small unexported function returning at most one
// Advice.
package advise

import (
	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// Level is the severity of a piece of advice.
type Level int

// Advice levels.
const (
	Info Level = iota
	Warn
)

// Advice is one hint for the user.
type Advice struct {
	Level Level
	Text  string
}

// Input is everything a rule may look at. Process is nil when the owner is not
// visible; All holds every socket collected in this run. GOOS names the
// operating system the sockets were collected on (runtime.GOOS in production);
// it is an input so OS-specific rules are testable everywhere.
type Input struct {
	GOOS    string
	Socket  socket.Socket
	Process *proc.Process
	Source  detect.Source
	All     []socket.Socket
}

type rule func(Input) *Advice

var (
	listenRules   = []rule{airplay, exposedBind, ownerNotVisible}
	timeWaitRules = []rule{timeWaitOnly}
)

// Advise returns the advice for one socket. A time_wait socket only gets the
// time_wait hint; every other rule applies to listening sockets.
func Advise(in Input) []Advice {
	rules := listenRules
	if in.Socket.State == socket.TimeWait {
		rules = timeWaitRules
	}
	var out []Advice
	for _, r := range rules {
		if a := r(in); a != nil {
			out = append(out, *a)
		}
	}
	return out
}

func airplay(in Input) *Advice {
	if in.GOOS != "darwin" || in.Process == nil || in.Process.Name != "ControlCenter" {
		return nil
	}
	if in.Socket.Port != 5000 && in.Socket.Port != 7000 {
		return nil
	}
	return &Advice{Level: Warn, Text: "macOS AirPlay Receiver holds this port. Turn it off in System Settings → General → AirDrop & Handoff → AirPlay Receiver."}
}

func timeWaitOnly(in Input) *Advice {
	if in.Socket.State != socket.TimeWait {
		return nil
	}
	return &Advice{Level: Info, Text: "Nobody is listening; the port is in TIME_WAIT. Wait for it to clear or bind with SO_REUSEADDR."}
}

func exposedBind(in Input) *Advice {
	if !in.Socket.Addr.IsUnspecified() {
		return nil
	}
	return &Advice{Level: Warn, Text: "Reachable from the network (bound to all interfaces)."}
}

func ownerNotVisible(in Input) *Advice {
	if in.Socket.PID != 0 && in.Process != nil {
		return nil
	}
	return &Advice{Level: Info, Text: "Owner not visible. Run with sudo for details."}
}
