package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type ProcessInfo struct {
	PID        int
	WorkingDir string
	StartTime  time.Time
	CmdLine    string
	TmuxTarget string
}

func DiscoverSessions() ([]ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("listing processes: %w", err)
	}

	var results []ProcessInfo

	for _, p := range procs {
		args, err := p.CmdlineSlice()
		if err != nil || len(args) == 0 {
			continue
		}

		if !isClaudeProcess(args) {
			continue
		}

		cwd, err := p.Cwd()
		if err != nil || isInsideClaudeDir(cwd) {
			continue
		}

		startTime := processStartTime(p)
		cmdline, _ := p.Cmdline()

		results = append(results, ProcessInfo{
			PID:        int(p.Pid),
			WorkingDir: cwd,
			StartTime:  startTime,
			CmdLine:    cmdline,
		})
	}

	return results, nil
}

func isClaudeProcess(args []string) bool {
	if len(args) == 0 {
		return false
	}

	exe := filepath.Base(args[0])

	// Match the main claude process, not subprocesses it spawns
	if exe == "claude" || exe == "claude-code" {
		return true
	}

	// Also match node running claude
	if exe == "node" {
		for _, arg := range args[1:] {
			if strings.Contains(arg, "claude") && !strings.Contains(arg, "node_modules/.bin") {
				return true
			}
		}
	}

	return false
}

func processStartTime(p *process.Process) time.Time {
	ct, err := p.CreateTime()
	if err != nil {
		return time.Now()
	}
	return time.UnixMilli(ct)
}

// isInsideClaudeDir reports whether the given path is at or under ~/.claude.
// Agent internal processes with CWD inside this directory are filtered out.
func isInsideClaudeDir(path string) bool {
	homeDir, _ := os.UserHomeDir()
	claudeDir := filepath.Join(homeDir, ".claude")
	return path == claudeDir || strings.HasPrefix(path, claudeDir+"/")
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
	return pa.CPU >= cpuThreshold && (!requireNetwork || pa.TCPConns > 0)
}

// cpuSample stores a previous CPU time reading for delta computation.
type cpuSample struct {
	cpuTime float64 // User + System CPU time in seconds
	when    time.Time
}

// isAgentProcess returns true if the command args belong to a known agent
// process (claude, codex, gemini).
func isAgentProcess(args []string) bool {
	if len(args) == 0 {
		return false
	}

	exe := filepath.Base(args[0])

	// Direct agent binaries
	switch exe {
	case "claude", "claude-code", "codex", "gemini":
		return true
	}

	// Node-based agent processes
	if exe == "node" {
		for _, arg := range args[1:] {
			lower := strings.ToLower(arg)
			if strings.Contains(arg, "node_modules/.bin") {
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

// DiscoverProcessActivity scans running processes for agent processes and
// computes CPU deltas from the previous sample. It returns a slice of
// ProcessActivity and an updated prevCPU map.
func DiscoverProcessActivity(prevCPU map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
	procs, err := process.Processes()
	if err != nil {
		return nil, prevCPU
	}

	now := time.Now()
	newCPU := make(map[int]cpuSample)
	var results []ProcessActivity

	for _, p := range procs {
		pid := int(p.Pid)

		args, err := p.CmdlineSlice()
		if err != nil || len(args) == 0 {
			continue
		}

		if !isAgentProcess(args) {
			continue
		}

		cwd, err := p.Cwd()
		if err != nil || isInsideClaudeDir(cwd) {
			continue
		}

		times, err := p.Times()
		if err != nil {
			continue
		}
		cpuTime := times.User + times.System

		var cpuPct float64
		if prev, ok := prevCPU[pid]; ok && elapsed > 0 {
			cpuPct = ((cpuTime - prev.cpuTime) / elapsed.Seconds()) * 100.0
		}

		newCPU[pid] = cpuSample{cpuTime: cpuTime, when: now}

		tcpConns := countEstablishedTCP(int32(pid))

		results = append(results, ProcessActivity{
			PID:        pid,
			CPU:        cpuPct,
			TCPConns:   tcpConns,
			WorkingDir: cwd,
		})
	}

	return results, newCPU
}

// countEstablishedTCP counts ESTABLISHED TCP connections for the given PID
// using gopsutil for cross-platform support.
func countEstablishedTCP(pid int32) int {
	conns, err := gnet.ConnectionsPid("tcp", pid)
	if err != nil {
		return 0
	}
	count := 0
	for _, c := range conns {
		if c.Status == "ESTABLISHED" {
			count++
		}
	}
	return count
}
