//go:build linux

package proc

import (
	"os"
	"os/user"
)

// linuxClockTicks is USER_HZ, the unit of the start time in /proc/<pid>/stat.
// It is 100 on every Linux architecture Go supports.
const linuxClockTicks = 100

// NewReader returns a Reader backed by the procfs mounted at root, normally
// "/proc". An empty root defaults to "/proc".
func NewReader(root string) Reader {
	if root == "" {
		root = "/proc"
	}
	return &procfsReader{
		root:       root,
		clockTicks: linuxClockTicks,
		lookupUser: lookupUserName,
		readlink:   os.Readlink,
	}
}

func lookupUserName(uid string) (string, error) {
	u, err := user.LookupId(uid)
	if err != nil {
		return "", err
	}
	return u.Username, nil
}
