package monitor

import (
	"testing"
)

func TestIsClaudeProcess(t *testing.T) {
	tests := []struct {
		cmdline  string
		expected bool
	}{
		{"/usr/local/bin/claude\x00--help", true},
		{"/home/user/.local/bin/claude\x00", true},
		{"claude\x00", true},
		{"node\x00/usr/lib/claude/cli.js\x00", true},
		{"bash\x00-c\x00ls", false},
		{"/usr/bin/python3\x00script.py", false},
		{"node\x00/usr/lib/something/server.js", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isClaudeProcess(tt.cmdline)
		if got != tt.expected {
			t.Errorf("isClaudeProcess(%q) = %v, want %v", tt.cmdline, got, tt.expected)
		}
	}
}

func TestCleanCmdline(t *testing.T) {
	input := "/usr/bin/claude\x00--mock\x00--dev\x00"
	expected := "/usr/bin/claude --mock --dev"
	got := cleanCmdline(input)
	if got != expected {
		t.Errorf("cleanCmdline() = %q, want %q", got, expected)
	}
}
