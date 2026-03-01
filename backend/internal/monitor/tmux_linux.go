//go:build linux

package monitor

import (
	"fmt"
	"os"
)

// getParentPID reads /proc/<pid>/stat to extract the parent PID (field 4).
func getParentPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	return parseParentPID(string(data))
}
