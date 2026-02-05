package monitor

import (
	"testing"
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
