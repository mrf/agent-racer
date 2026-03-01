//go:build linux

package monitor

import (
	"os"
	"testing"
)

func TestGetParentPID_Linux_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	ppid := getParentPID(pid)
	if ppid <= 0 {
		t.Errorf("getParentPID(%d) = %d, want > 0", pid, ppid)
	}
}

func TestGetParentPID_Linux_InvalidPID(t *testing.T) {
	ppid := getParentPID(-1)
	if ppid != 0 {
		t.Errorf("getParentPID(-1) = %d, want 0", ppid)
	}
}
