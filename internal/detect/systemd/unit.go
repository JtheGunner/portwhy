// Package systemd explains processes that run inside a systemd service and
// stops the service with systemctl. It talks to systemd only through
// execx.Runner, so it needs no build tags and its tests run on every OS.
package systemd

import (
	"regexp"
	"strconv"
	"strings"
)

const serviceSuffix = ".service"

// userManagerRE matches the cgroup of a per-user systemd instance. Services
// of that instance live below it and belong to the user manager, not to the
// system manager.
var userManagerRE = regexp.MustCompile(`^user@([0-9]+)\.service$`)

// unitNameRE accepts the characters systemd allows in unit names. It also
// guarantees the name cannot start with '-', so it can never be read by
// systemctl as an option.
var unitNameRE = regexp.MustCompile(`^[A-Za-z0-9:_.\\@][A-Za-z0-9:_.\\@-]*\.service$`)

// cgroupUnit is the service a control group path belongs to.
type cgroupUnit struct {
	Name string
	// User is true for services of a per-user systemd instance; UID is then
	// the owner of that instance.
	User bool
	UID  int
}

// unitFromCgroup finds the service unit in a cgroup path such as
// "/system.slice/postgresql.service". Only *.service units match: scopes
// (login sessions, Docker containers) and slices do not. For a path below
// "user@<uid>.service" the first service below the user manager is returned
// with User set. The name is validated, so a hostile directory name is not
// matched.
func unitFromCgroup(cg string) (cgroupUnit, bool) {
	parts := strings.Split(strings.Trim(cg, "/"), "/")

	var result cgroupUnit
	for _, part := range parts {
		if !result.User {
			if m := userManagerRE.FindStringSubmatch(part); m != nil {
				uid, err := strconv.Atoi(m[1])
				if err != nil {
					return cgroupUnit{}, false
				}
				result = cgroupUnit{User: true, UID: uid}
				continue
			}
		}
		if strings.HasSuffix(part, serviceSuffix) {
			if !unitNameRE.MatchString(part) {
				return cgroupUnit{}, false
			}
			result.Name = part
			return result, true
		}
	}
	return cgroupUnit{}, false
}
