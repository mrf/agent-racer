package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// TmuxPane represents a single tmux pane and its shell PID.
type TmuxPane struct {
	SessionName string // e.g. "main"
	WindowIndex int    // e.g. 2
	PaneIndex   int    // e.g. 0
	PanePID     int    // PID of the shell running inside this pane
	Target      string // Pre-formatted "main:2.0" for tmux commands
}

// TmuxResolver maps process PIDs to their containing tmux pane.
type TmuxResolver struct {
	targetByPID map[int]string // pane shell PID -> target string
}

// NewTmuxResolver queries tmux for all panes. Returns nil resolver
// (not an error) when tmux is not running or not installed.
func NewTmuxResolver() *TmuxResolver {
	panes, err := listTmuxPanes()
	if err != nil || len(panes) == 0 {
		return nil
	}
	targetByPID := make(map[int]string, len(panes))
	for _, p := range panes {
		targetByPID[p.PanePID] = p.Target
	}
	return &TmuxResolver{targetByPID: targetByPID}
}

// Resolve walks the process tree from pid upward (via /proc/<pid>/stat ppid)
// to find a PID that matches a tmux pane's shell PID. Returns the pane target
// string and true, or ("", false) if no match.
func (r *TmuxResolver) Resolve(pid int) (string, bool) {
	if r == nil {
		return "", false
	}

	current := pid
	for i := 0; i < 10; i++ {
		if target, ok := r.targetByPID[current]; ok {
			return target, true
		}
		parent := getParentPID(current)
		if parent <= 1 || parent == current {
			break
		}
		current = parent
	}

	return "", false
}

// listTmuxPanes runs tmux list-panes and parses the output.
func listTmuxPanes() ([]TmuxPane, error) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return nil, err
	}

	out, err := exec.Command(path, "list-panes", "-a", "-F",
		"#{pane_pid}\t#{session_name}\t#{window_index}\t#{pane_index}").Output()
	if err != nil {
		return nil, err
	}

	return parseTmuxPanes(string(out)), nil
}

// parseTmuxPanes parses the tab-separated output of tmux list-panes.
func parseTmuxPanes(output string) []TmuxPane {
	var panes []TmuxPane
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		winIdx, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		paneIdx, err := strconv.Atoi(fields[3])
		if err != nil {
			continue
		}

		sessionName := fields[1]
		panes = append(panes, TmuxPane{
			SessionName: sessionName,
			WindowIndex: winIdx,
			PaneIndex:   paneIdx,
			PanePID:     pid,
			Target:      fmt.Sprintf("%s:%d.%d", sessionName, winIdx, paneIdx),
		})
	}
	return panes
}

// getParentPID reads /proc/<pid>/stat to extract the parent PID (field 4).
func getParentPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	return parseParentPID(string(data))
}

// parseParentPID extracts the ppid from /proc/<pid>/stat content.
// The comm field (field 2) is enclosed in parens and may contain spaces,
// so we find the closing paren first. PPID is the first field after comm.
func parseParentPID(stat string) int {
	idx := strings.LastIndex(stat, ")")
	if idx < 0 || idx+2 >= len(stat) {
		return 0
	}
	rest := strings.TrimSpace(stat[idx+1:])
	fields := strings.Fields(rest)
	// rest starts at field 3 (state), so ppid is field 4 = index 1.
	if len(fields) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}
