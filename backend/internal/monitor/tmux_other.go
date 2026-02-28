//go:build !linux

package monitor

import (
	"os/exec"
	"strconv"
	"strings"
)

// getParentPID uses the ps command to get the parent PID of the given process.
// This is the cross-platform fallback for non-Linux systems (macOS, FreeBSD, etc.).
func getParentPID(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return ppid
}
