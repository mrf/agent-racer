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

func TestIsAgentProcess(t *testing.T) {
	tests := []struct {
		name     string
		cmdline  string
		expected bool
	}{
		{"claude binary", "/usr/local/bin/claude\x00--help", true},
		{"claude-code binary", "/usr/bin/claude-code\x00", true},
		{"codex binary", "/usr/local/bin/codex\x00--help", true},
		{"gemini binary", "/usr/local/bin/gemini\x00run", true},
		{"node running claude", "node\x00/usr/lib/claude/cli.js\x00", true},
		{"node running codex", "node\x00/home/user/.npm/codex/main.js\x00", true},
		{"node running gemini", "node\x00/opt/gemini/server.js\x00", true},
		{"bash script", "bash\x00-c\x00ls", false},
		{"python", "/usr/bin/python3\x00script.py", false},
		{"unrelated node", "node\x00/usr/lib/something/server.js", false},
		{"node_modules bin", "node\x00/project/node_modules/.bin/codex\x00", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgentProcess(tt.cmdline)
			if got != tt.expected {
				t.Errorf("isAgentProcess(%q) = %v, want %v", tt.cmdline, got, tt.expected)
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

func TestParseCPUTicks(t *testing.T) {
	// Simulated /proc/<pid>/stat content.
	// Fields: pid (comm) state ppid pgrp session tty_nr tpgid flags
	//         minflt cminflt majflt cmajflt utime stime ...
	// utime is field 14, stime is field 15 (1-indexed).
	stat := "1234 (claude) S 1 1234 1234 0 -1 4194304 100 0 0 0 500 200 0 0 20 0 1 0 12345 67890 100 18446744073709551615"
	ticks := parseCPUTicks(stat)
	// utime=500, stime=200
	if ticks != 700 {
		t.Errorf("parseCPUTicks() = %d, want 700", ticks)
	}
}

func TestParseCPUTicksWithSpacesInComm(t *testing.T) {
	// comm field may contain spaces or parens
	stat := "5678 (my process) S 1 5678 5678 0 -1 4194304 50 0 0 0 1000 300 0 0 20 0 1 0 12345 67890 100 18446744073709551615"
	ticks := parseCPUTicks(stat)
	if ticks != 1300 {
		t.Errorf("parseCPUTicks() = %d, want 1300", ticks)
	}
}

func TestCountEstablishedInNetTCP(t *testing.T) {
	// Sample /proc/net/tcp content
	data := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:9C40 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 12346 1 0000000000000000 100 0 0 10 0
   2: 0100007F:9C41 0100007F:1F91 01 00000000:00000000 00:00000000 00000000  1000        0 12347 1 0000000000000000 100 0 0 10 0
   3: 0100007F:9C42 0100007F:1F92 06 00000000:00000000 00:00000000 00000000  1000        0 12348 1 0000000000000000 100 0 0 10 0`

	count := countEstablishedInNetTCP(data)
	// Two lines with state "01" (ESTABLISHED), one with "0A" (LISTEN), one with "06" (TIME_WAIT)
	if count != 2 {
		t.Errorf("countEstablishedInNetTCP() = %d, want 2", count)
	}
}

func TestCountEstablishedInNetTCPEmpty(t *testing.T) {
	data := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode`
	count := countEstablishedInNetTCP(data)
	if count != 0 {
		t.Errorf("countEstablishedInNetTCP(empty) = %d, want 0", count)
	}
}
