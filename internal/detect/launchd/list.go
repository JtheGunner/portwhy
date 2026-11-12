// Package launchd explains processes that macOS launchd runs for the current
// user, including Homebrew services, and stops them with launchctl or brew.
// It talks to the system only through execx.Runner, so it needs no build tags
// and its tests run on every OS.
package launchd

import (
	"strconv"
	"strings"
)

// parseLaunchctlList maps process IDs to job labels. The input is the output
// of "launchctl list": a header line, then one tab-separated row per job with
// the columns PID, last exit status and label. Jobs that are not running have
// "-" as PID and are skipped, like the header and any line that does not fit.
func parseLaunchctlList(out []byte) map[int]string {
	labels := make(map[int]string)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || pid <= 0 {
			continue
		}
		label := strings.TrimSpace(fields[2])
		if label == "" {
			continue
		}
		if _, seen := labels[pid]; !seen {
			labels[pid] = label
		}
	}
	return labels
}
