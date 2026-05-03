package monitor

import (
	"testing"

	awmonitor "github.com/mrf/agentwatch/monitor"

	"github.com/agent-racer/backend/internal/ws"
)

func TestSanitizeHealthError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"no path", "connection refused", "connection refused"},
		{"os open error", "open /home/user/.claude/projects/foo/session.jsonl: permission denied", "open <path>: permission denied"},
		{"os stat error", "stat /home/user/.gemini/tmp/hash/chats/session-1.json: no such file or directory", "stat <path>: no such file or directory"},
		{"panic", "panic: runtime error: index out of range [5] with length 3", "internal error"},
		{"panic nil pointer", "panic: nil pointer dereference", "internal error"},
		{"path in middle", "no session files found in /home/user/.claude/projects/foo", "no session files found in <path>"},
		{"file size error", "file size 5368709120 exceeds max 5368709120", "file size 5368709120 exceeds max 5368709120"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHealthError(tt.raw)
			if got != tt.want {
				t.Errorf("sanitizeHealthError(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestConvertHealthStatus(t *testing.T) {
	tests := []struct {
		input awmonitor.HealthStatus
		want  ws.SourceHealthStatus
	}{
		{awmonitor.HealthHealthy, ws.StatusHealthy},
		{awmonitor.HealthDegraded, ws.StatusDegraded},
		{awmonitor.HealthFailed, ws.StatusFailed},
	}
	for _, tt := range tests {
		got := convertHealthStatus(tt.input)
		if got != tt.want {
			t.Errorf("convertHealthStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
