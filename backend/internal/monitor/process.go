package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ProcessInfo struct {
	PID        int
	WorkingDir string
	StartTime  time.Time
	CmdLine    string
}

func DiscoverSessions() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	claudeDir := filepath.Join(homeDir, ".claude")

	var results []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdline, err := readProcFile(pid, "cmdline")
		if err != nil {
			continue
		}

		if !isClaudeProcess(cmdline) {
			continue
		}

		cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
		if err != nil {
			continue
		}

		// Skip Claude's own internal processes (CWD inside ~/.claude)
		if cwd == claudeDir || strings.HasPrefix(cwd, claudeDir+"/") {
			continue
		}

		startTime := getProcessStartTime(pid)

		results = append(results, ProcessInfo{
			PID:        pid,
			WorkingDir: cwd,
			StartTime:  startTime,
			CmdLine:    cleanCmdline(cmdline),
		})
	}

	return results, nil
}

func isClaudeProcess(cmdline string) bool {
	// cmdline has null bytes between args
	parts := strings.Split(cmdline, "\x00")
	if len(parts) == 0 {
		return false
	}

	exe := filepath.Base(parts[0])

	// Match the main claude process, not subprocesses it spawns
	if exe == "claude" || exe == "claude-code" {
		return true
	}

	// Also match node running claude
	if exe == "node" {
		for _, part := range parts[1:] {
			if strings.Contains(part, "claude") && !strings.Contains(part, "node_modules/.bin") {
				return true
			}
		}
	}

	return false
}

func readProcFile(pid int, name string) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cleanCmdline(cmdline string) string {
	parts := strings.Split(cmdline, "\x00")
	var cleaned []string
	for _, p := range parts {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return strings.Join(cleaned, " ")
}

func getProcessStartTime(pid int) time.Time {
	info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}
