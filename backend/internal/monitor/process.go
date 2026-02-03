package monitor

import (
	"bufio"
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

// ProcessActivity holds CPU and network metrics for a running agent process.
type ProcessActivity struct {
	PID        int
	CPU        float64 // percent since last sample
	TCPConns   int     // ESTABLISHED TCP connections
	WorkingDir string
}

// IsChurning reports whether this process shows signs of active work:
// CPU above the threshold, and optionally at least one TCP connection.
func (pa ProcessActivity) IsChurning(cpuThreshold float64, requireNetwork bool) bool {
	if pa.CPU < cpuThreshold {
		return false
	}
	if requireNetwork && pa.TCPConns == 0 {
		return false
	}
	return true
}

// cpuSample stores a previous CPU tick reading for delta computation.
type cpuSample struct {
	ticks uint64
	when  time.Time
}

// isAgentProcess returns true if the command line belongs to a known agent
// process (claude, codex, gemini).
func isAgentProcess(cmdline string) bool {
	parts := strings.Split(cmdline, "\x00")
	if len(parts) == 0 {
		return false
	}

	exe := filepath.Base(parts[0])

	// Direct agent binaries
	switch exe {
	case "claude", "claude-code", "codex", "gemini":
		return true
	}

	// Node-based agent processes
	if exe == "node" {
		for _, part := range parts[1:] {
			lower := strings.ToLower(part)
			if strings.Contains(part, "node_modules/.bin") {
				continue
			}
			if strings.Contains(lower, "claude") ||
				strings.Contains(lower, "codex") ||
				strings.Contains(lower, "gemini") {
				return true
			}
		}
	}

	return false
}

// DiscoverProcessActivity scans /proc for running agent processes and
// computes CPU deltas from the previous sample. It returns a slice of
// ProcessActivity and an updated prevCPU map.
func DiscoverProcessActivity(prevCPU map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, prevCPU
	}

	now := time.Now()
	clkTck := uint64(100) // standard Linux USER_HZ

	homeDir, _ := os.UserHomeDir()
	claudeDir := filepath.Join(homeDir, ".claude")

	newCPU := make(map[int]cpuSample)
	var results []ProcessActivity

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

		if !isAgentProcess(cmdline) {
			continue
		}

		cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
		if err != nil {
			continue
		}

		// Skip agent internal processes
		if cwd == claudeDir || strings.HasPrefix(cwd, claudeDir+"/") {
			continue
		}

		// Read CPU ticks from /proc/<pid>/stat
		statData, err := readProcFile(pid, "stat")
		if err != nil {
			continue
		}
		ticks := parseCPUTicks(statData)

		var cpuPct float64
		if prev, ok := prevCPU[pid]; ok && elapsed > 0 {
			dticks := ticks - prev.ticks
			elapsedSec := elapsed.Seconds()
			if elapsedSec > 0 {
				cpuPct = (float64(dticks) / float64(clkTck)) / elapsedSec * 100.0
			}
		}

		newCPU[pid] = cpuSample{ticks: ticks, when: now}

		// Count TCP connections
		tcpConns := countEstablishedTCP(pid)

		results = append(results, ProcessActivity{
			PID:        pid,
			CPU:        cpuPct,
			TCPConns:   tcpConns,
			WorkingDir: cwd,
		})
	}

	return results, newCPU
}

// parseCPUTicks extracts utime + stime (fields 14 and 15, 1-indexed) from
// /proc/<pid>/stat. The comm field (field 2) is enclosed in parens and may
// contain spaces, so we find the closing paren first.
func parseCPUTicks(stat string) uint64 {
	// Find end of comm field: last occurrence of ')'
	idx := strings.LastIndex(stat, ")")
	if idx < 0 || idx+2 >= len(stat) {
		return 0
	}
	// Fields after comm start at index 3 (1-indexed). We need fields 14 and 15,
	// which are indices 11 and 12 in the remaining space-separated fields.
	rest := strings.TrimSpace(stat[idx+1:])
	fields := strings.Fields(rest)
	// rest starts at field 3, so field 14 = index 11, field 15 = index 12
	if len(fields) < 13 {
		return 0
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	return utime + stime
}

// countEstablishedTCP counts ESTABLISHED (state "01") TCP connections for
// the given PID by reading /proc/<pid>/net/tcp and /proc/<pid>/net/tcp6.
func countEstablishedTCP(pid int) int {
	count := 0
	for _, proto := range []string{"net/tcp", "net/tcp6"} {
		data, err := readProcFile(pid, proto)
		if err != nil {
			continue
		}
		count += countEstablishedInNetTCP(data)
	}
	return count
}

// countEstablishedInNetTCP parses the content of /proc/net/tcp or tcp6 and
// counts lines where the connection state (field 4, 0-indexed) is "01"
// (ESTABLISHED).
func countEstablishedInNetTCP(data string) int {
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(data))
	first := true
	for scanner.Scan() {
		if first {
			first = false // skip header
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		if fields[3] == "01" {
			count++
		}
	}
	return count
}
