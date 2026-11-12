// Package advise turns what is known about a busy port into short pieces of
// advice. The rules are added in a later change.
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
// visible; All holds every socket collected in this run.
type Input struct {
	Socket  socket.Socket
	Process *proc.Process
	Source  detect.Source
	All     []socket.Socket
}

// Advise returns the advice for one socket. It returns nil while no rules exist.
func Advise(in Input) []Advice {
	return nil
}
