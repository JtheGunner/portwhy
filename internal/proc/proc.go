// Package proc describes the process that owns a socket and defines how
// processes are read. OS-specific readers live in their own files.
package proc

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned by Reader.Read when the process does not exist
	// (any more).
	ErrNotFound = errors.New("process not found")
	// ErrPermission is returned by Reader.Read when the process exists but the
	// current user may not inspect it.
	ErrPermission = errors.New("permission denied")
)

// Ancestor is one entry of a process's parent chain.
type Ancestor struct {
	PID  int
	Name string
}

// Process holds what portwhy knows about one process. Fields that cannot be
// read (for example Cwd for another user's process) are left empty.
type Process struct {
	PID       int
	PPID      int
	Name      string
	Cmdline   []string
	User      string
	Started   time.Time
	Cwd       string
	Cgroup    string // Linux only: the control group path, used to find the systemd unit.
	Ancestors []Ancestor
}

// Reader reads a process by PID.
type Reader interface {
	Read(ctx context.Context, pid int) (Process, error)
}
