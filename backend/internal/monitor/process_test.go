package monitor

import (
	"os"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

func TestIsClaudeProcess(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"claude binary with flag", []string{"/usr/local/bin/claude", "--help"}, true},
		{"claude binary no args", []string{"/home/user/.local/bin/claude"}, true},
		{"bare claude", []string{"claude"}, true},
		{"node running claude", []string{"node", "/usr/lib/claude/cli.js"}, true},
		{"bash script", []string{"bash", "-c", "ls"}, false},
		{"python", []string{"/usr/bin/python3", "script.py"}, false},
		{"unrelated node", []string{"node", "/usr/lib/something/server.js"}, false},
		{"empty", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClaudeProcess(tt.args)
			if got != tt.expected {
				t.Errorf("isClaudeProcess(%v) = %v, want %v", tt.args, got, tt.expected)
			}
		})
	}
}

func TestIsAgentProcess(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"claude binary", []string{"/usr/local/bin/claude", "--help"}, true},
		{"claude-code binary", []string{"/usr/bin/claude-code"}, true},
		{"codex binary", []string{"/usr/local/bin/codex", "--help"}, true},
		{"gemini binary", []string{"/usr/local/bin/gemini", "run"}, true},
		{"node running claude", []string{"node", "/usr/lib/claude/cli.js"}, true},
		{"node running codex", []string{"node", "/home/user/.npm/codex/main.js"}, true},
		{"node running gemini", []string{"node", "/opt/gemini/server.js"}, true},
		{"bash script", []string{"bash", "-c", "ls"}, false},
		{"python", []string{"/usr/bin/python3", "script.py"}, false},
		{"unrelated node", []string{"node", "/usr/lib/something/server.js"}, false},
		{"node_modules bin", []string{"node", "/project/node_modules/.bin/codex"}, false},
		{"empty", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgentProcess(tt.args)
			if got != tt.expected {
				t.Errorf("isAgentProcess(%v) = %v, want %v", tt.args, got, tt.expected)
			}
		})
	}
}

func TestProcessActivity_IsChurning(t *testing.T) {
	tests := []struct {
		name           string
		cpu            float64
		tcpConns       int
		threshold      float64
		requireNetwork bool
		expected       bool
	}{
		{"high CPU, no network required", 25.0, 0, 15.0, false, true},
		{"high CPU with TCP", 25.0, 3, 15.0, false, true},
		{"high CPU, network required, has TCP", 25.0, 3, 15.0, true, true},
		{"high CPU, network required, no TCP", 25.0, 0, 15.0, true, false},
		{"low CPU", 5.0, 3, 15.0, false, false},
		{"zero CPU", 0.0, 0, 15.0, false, false},
		{"exactly at threshold", 15.0, 0, 15.0, false, true},
		{"just below threshold", 14.9, 0, 15.0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pa := ProcessActivity{
				PID:      1234,
				CPU:      tt.cpu,
				TCPConns: tt.tcpConns,
			}
			got := pa.IsChurning(tt.threshold, tt.requireNetwork)
			if got != tt.expected {
				t.Errorf("IsChurning(%.1f, %v) = %v, want %v", tt.threshold, tt.requireNetwork, got, tt.expected)
			}
		})
	}
}

func TestProcessActivity_IsChurning_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		cpu            float64
		tcpConns       int
		threshold      float64
		requireNetwork bool
		expected       bool
	}{
		// Zero threshold: any non-negative CPU should churn
		{"zero threshold, zero CPU", 0.0, 0, 0.0, false, true},
		{"zero threshold, small CPU", 0.1, 0, 0.0, false, true},
		// Very high CPU
		{"very high CPU", 400.0, 0, 15.0, false, true},
		{"100% CPU with network required and TCP", 100.0, 10, 15.0, true, true},
		// Many TCP connections
		{"many TCP connections, low CPU", 5.0, 100, 15.0, false, false},
		{"many TCP connections, high CPU", 50.0, 100, 15.0, true, true},
		// Single TCP connection edge
		{"network required, exactly 1 TCP", 20.0, 1, 15.0, true, true},
		// Very high threshold
		{"very high threshold", 99.0, 5, 100.0, false, false},
		{"CPU matches very high threshold", 100.0, 5, 100.0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pa := ProcessActivity{
				PID:      9999,
				CPU:      tt.cpu,
				TCPConns: tt.tcpConns,
			}
			got := pa.IsChurning(tt.threshold, tt.requireNetwork)
			if got != tt.expected {
				t.Errorf("IsChurning(%.1f, %v) = %v, want %v", tt.threshold, tt.requireNetwork, got, tt.expected)
			}
		})
	}
}

func TestProcessActivity_IsChurning_ZeroValueActivity(t *testing.T) {
	// Zero-value ProcessActivity should never be churning with any positive threshold.
	pa := ProcessActivity{}
	if pa.IsChurning(15.0, false) {
		t.Error("zero-value ProcessActivity should not be churning")
	}
	if pa.IsChurning(15.0, true) {
		t.Error("zero-value ProcessActivity should not be churning with network required")
	}
}

func TestActivityByDir_HighestCPUWins(t *testing.T) {
	activities := []ProcessActivity{
		{PID: 1, CPU: 10.0, TCPConns: 2, WorkingDir: "/home/user/project"},
		{PID: 2, CPU: 30.0, TCPConns: 1, WorkingDir: "/home/user/project"},
		{PID: 3, CPU: 20.0, TCPConns: 5, WorkingDir: "/home/user/project"},
	}

	activityByDir := make(map[string]ProcessActivity)
	for _, a := range activities {
		if existing, ok := activityByDir[a.WorkingDir]; ok {
			if a.CPU > existing.CPU {
				activityByDir[a.WorkingDir] = a
			}
		} else {
			activityByDir[a.WorkingDir] = a
		}
	}

	winner, ok := activityByDir["/home/user/project"]
	if !ok {
		t.Fatal("expected activity for /home/user/project")
	}
	if winner.PID != 2 {
		t.Errorf("expected PID 2 (highest CPU), got PID %d", winner.PID)
	}
	if winner.CPU != 30.0 {
		t.Errorf("expected CPU 30.0, got %.1f", winner.CPU)
	}
}

func TestActivityByDir_MultipleDirectories(t *testing.T) {
	activities := []ProcessActivity{
		{PID: 1, CPU: 25.0, TCPConns: 1, WorkingDir: "/project/a"},
		{PID: 2, CPU: 50.0, TCPConns: 3, WorkingDir: "/project/b"},
		{PID: 3, CPU: 10.0, TCPConns: 0, WorkingDir: "/project/a"},
	}

	activityByDir := make(map[string]ProcessActivity)
	for _, a := range activities {
		if existing, ok := activityByDir[a.WorkingDir]; ok {
			if a.CPU > existing.CPU {
				activityByDir[a.WorkingDir] = a
			}
		} else {
			activityByDir[a.WorkingDir] = a
		}
	}

	if len(activityByDir) != 2 {
		t.Errorf("expected 2 directories, got %d", len(activityByDir))
	}

	dirA := activityByDir["/project/a"]
	if dirA.PID != 1 {
		t.Errorf("expected PID 1 for /project/a, got PID %d", dirA.PID)
	}

	dirB := activityByDir["/project/b"]
	if dirB.PID != 2 {
		t.Errorf("expected PID 2 for /project/b, got PID %d", dirB.PID)
	}
}

func TestChurningAppliedToActiveSessions(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second)

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	// Active session (Thinking) with matching working directory
	state := &session.SessionState{
		ID:         "claude:session-1",
		Source:     "claude",
		Activity:   session.Thinking,
		WorkingDir: "/home/user/project",
	}
	store.Update(state)

	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 1234, CPU: 25.0, TCPConns: 2, WorkingDir: "/home/user/project"},
	}

	// Simulate the churning logic from poll()
	updates := []*session.SessionState{state}
	for _, s := range updates {
		churning := false
		if !s.IsTerminal() && s.Activity != session.Waiting {
			if pa, ok := activityByDir[s.WorkingDir]; ok {
				churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
				if pa.PID > 0 && s.PID == 0 {
					s.PID = pa.PID
				}
			}
		}
		if s.IsChurning != churning {
			s.IsChurning = churning
			store.Update(s)
		}
	}

	got, _ := store.Get("claude:session-1")
	if !got.IsChurning {
		t.Error("active session with high CPU should be marked as churning")
	}
	if got.PID != 1234 {
		t.Errorf("PID should be populated from process activity, got %d", got.PID)
	}

	_ = broadcaster // keep reference to prevent GC
}

func TestChurningNotAppliedToTerminalSessions(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	completedAt := time.Now()
	terminalStates := []session.Activity{session.Complete, session.Errored, session.Lost}

	for _, activity := range terminalStates {
		state := &session.SessionState{
			ID:          "claude:terminal-" + activity.String(),
			Source:      "claude",
			Activity:    activity,
			WorkingDir:  "/home/user/project",
			CompletedAt: &completedAt,
		}
		store.Update(state)

		activityByDir := map[string]ProcessActivity{
			"/home/user/project": {PID: 5678, CPU: 90.0, TCPConns: 5},
		}

		updates := []*session.SessionState{state}
		for _, s := range updates {
			churning := false
			if !s.IsTerminal() && s.Activity != session.Waiting {
				if pa, ok := activityByDir[s.WorkingDir]; ok {
					churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
				}
			}
			if s.IsChurning != churning {
				s.IsChurning = churning
				store.Update(s)
			}
		}

		got, _ := store.Get(state.ID)
		if got.IsChurning {
			t.Errorf("terminal session (%s) should not be marked as churning", activity)
		}
	}
}

func TestChurningNotAppliedToWaitingSessions(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	state := &session.SessionState{
		ID:         "claude:waiting-1",
		Source:     "claude",
		Activity:   session.Waiting,
		WorkingDir: "/home/user/project",
	}
	store.Update(state)

	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 5678, CPU: 90.0, TCPConns: 5},
	}

	updates := []*session.SessionState{state}
	for _, s := range updates {
		churning := false
		if !s.IsTerminal() && s.Activity != session.Waiting {
			if pa, ok := activityByDir[s.WorkingDir]; ok {
				churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
			}
		}
		if s.IsChurning != churning {
			s.IsChurning = churning
			store.Update(s)
		}
	}

	got, _ := store.Get("claude:waiting-1")
	if got.IsChurning {
		t.Error("waiting session should not be marked as churning")
	}
}

func TestChurningClearedWhenCPUDrops(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	state := &session.SessionState{
		ID:         "claude:session-clear",
		Source:     "claude",
		Activity:   session.Thinking,
		WorkingDir: "/home/user/project",
		IsChurning: true, // previously churning
	}
	store.Update(state)

	// Process now has low CPU
	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 1234, CPU: 5.0, TCPConns: 0},
	}

	updates := []*session.SessionState{state}
	for _, s := range updates {
		churning := false
		if !s.IsTerminal() && s.Activity != session.Waiting {
			if pa, ok := activityByDir[s.WorkingDir]; ok {
				churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
			}
		}
		if s.IsChurning != churning {
			s.IsChurning = churning
			store.Update(s)
		}
	}

	got, _ := store.Get("claude:session-clear")
	if got.IsChurning {
		t.Error("churning should be cleared when CPU drops below threshold")
	}
}

func TestChurningWithNetworkRequirement(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: true, // network required
		},
	}

	tests := []struct {
		name     string
		cpu      float64
		tcpConns int
		want     bool
	}{
		{"high CPU with TCP", 25.0, 3, true},
		{"high CPU without TCP", 25.0, 0, false},
		{"low CPU with TCP", 5.0, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &session.SessionState{
				ID:         "claude:" + tt.name,
				Source:     "claude",
				Activity:   session.ToolUse,
				WorkingDir: "/home/user/project",
			}
			store.Update(state)

			activityByDir := map[string]ProcessActivity{
				"/home/user/project": {PID: 1234, CPU: tt.cpu, TCPConns: tt.tcpConns},
			}

			updates := []*session.SessionState{state}
			for _, s := range updates {
				churning := false
				if !s.IsTerminal() && s.Activity != session.Waiting {
					if pa, ok := activityByDir[s.WorkingDir]; ok {
						churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
					}
				}
				if s.IsChurning != churning {
					s.IsChurning = churning
					store.Update(s)
				}
			}

			got, _ := store.Get(state.ID)
			if got.IsChurning != tt.want {
				t.Errorf("IsChurning = %v, want %v", got.IsChurning, tt.want)
			}
		})
	}
}

func TestChurningNoMatchingWorkingDir(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	state := &session.SessionState{
		ID:         "claude:no-match",
		Source:     "claude",
		Activity:   session.Thinking,
		WorkingDir: "/home/user/other-project",
	}
	store.Update(state)

	// Activity is for a different directory
	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 1234, CPU: 90.0, TCPConns: 5},
	}

	updates := []*session.SessionState{state}
	for _, s := range updates {
		churning := false
		if !s.IsTerminal() && s.Activity != session.Waiting {
			if pa, ok := activityByDir[s.WorkingDir]; ok {
				churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
			}
		}
		if s.IsChurning != churning {
			s.IsChurning = churning
			store.Update(s)
		}
	}

	got, _ := store.Get("claude:no-match")
	if got.IsChurning {
		t.Error("session with unmatched working dir should not be churning")
	}
}

func TestChurningPIDNotOverwrittenWhenAlreadySet(t *testing.T) {
	store := session.NewStore()

	state := &session.SessionState{
		ID:         "claude:has-pid",
		Source:     "claude",
		Activity:   session.Thinking,
		WorkingDir: "/home/user/project",
		PID:        9999, // already set
	}
	store.Update(state)

	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 1234, CPU: 25.0, TCPConns: 2},
	}

	updates := []*session.SessionState{state}
	for _, s := range updates {
		if !s.IsTerminal() && s.Activity != session.Waiting {
			if pa, ok := activityByDir[s.WorkingDir]; ok {
				if pa.PID > 0 && s.PID == 0 {
					s.PID = pa.PID
				}
			}
		}
	}

	if state.PID != 9999 {
		t.Errorf("PID should not be overwritten when already set, got %d", state.PID)
	}
}

func TestChurningAcrossMultipleSessionActivities(t *testing.T) {
	store := session.NewStore()

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
		},
	}

	// Test all non-terminal, non-waiting activities should allow churning
	activeActivities := []session.Activity{
		session.Starting, session.Thinking, session.ToolUse, session.Idle,
	}

	activityByDir := map[string]ProcessActivity{
		"/home/user/project": {PID: 1234, CPU: 50.0, TCPConns: 2},
	}

	for _, activity := range activeActivities {
		state := &session.SessionState{
			ID:         "claude:" + activity.String(),
			Source:     "claude",
			Activity:   activity,
			WorkingDir: "/home/user/project",
		}
		store.Update(state)

		updates := []*session.SessionState{state}
		for _, s := range updates {
			churning := false
			if !s.IsTerminal() && s.Activity != session.Waiting {
				if pa, ok := activityByDir[s.WorkingDir]; ok {
					churning = pa.IsChurning(cfg.Monitor.ChurningCPUThreshold, cfg.Monitor.ChurningRequiresNetwork)
				}
			}
			if s.IsChurning != churning {
				s.IsChurning = churning
				store.Update(s)
			}
		}

		got, _ := store.Get(state.ID)
		if !got.IsChurning {
			t.Errorf("activity %s should allow churning when CPU is high", activity)
		}
	}
}

func TestIsInsideClaudeDir(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact claude dir", homeDir() + "/.claude", true},
		{"subdirectory", homeDir() + "/.claude/projects/test", true},
		{"direct child", homeDir() + "/.claude/sessions", true},
		{"unrelated path", "/home/user/project", false},
		{"partial match", homeDir() + "/.claude-code", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInsideClaudeDir(tt.path)
			if got != tt.want {
				t.Errorf("isInsideClaudeDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func homeDir() string {
	// Use the same logic as the production code for test consistency
	dir, _ := os.UserHomeDir()
	return dir
}
