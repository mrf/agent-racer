package session

import (
	"testing"
	"time"
)

func TestSanitizeTailEntries(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	entries := []TailEntry{
		{Timestamp: ts, Type: "assistant", Activity: "text", Summary: "Here is the secret key: abc123", Detail: "Here is the secret key: abc123 and more sensitive content"},
		{Timestamp: ts, Type: "assistant", Activity: "thinking", Summary: "I need to find the password in config.yaml"},
		{Timestamp: ts, Type: "assistant", Activity: "tool_use", Summary: "Bash: cat /etc/passwd"},
		{Timestamp: ts, Type: "assistant", Activity: "tool_use", Summary: "Read session/tail.go"},
		{Timestamp: ts, Type: "assistant", Activity: "tool_use", Summary: "Grep secret_key"},
		{Timestamp: ts, Type: "assistant", Activity: "tool_use", Summary: "Agent: Find credentials"},
		{Timestamp: ts, Type: "user", Activity: "tool_result", Summary: "→ root:x:0:0:root:/root:/bin/bash"},
		{Timestamp: ts, Type: "user", Activity: "text", Summary: "Please help me with this sensitive task"},
		{Timestamp: ts, Type: "progress", Activity: "tool_use", Summary: "[sub1] Bash: rm -rf /"},
		{Timestamp: ts, Type: "progress", Activity: "thinking", Summary: "[sub1] analyzing the database credentials"},
		{Timestamp: ts, Type: "progress", Activity: "tool_result", Summary: "[sub1] received result"},
		{Timestamp: ts, Type: "progress", Activity: "subagent", Summary: "[sub1] progress"},
		{Timestamp: ts, Type: "system", Activity: "compact", Summary: "context compacted"},
	}

	sanitized := SanitizeTailEntries(entries)

	if len(sanitized) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(sanitized))
	}

	tests := []struct {
		name            string
		idx             int
		wantSummary     string
		wantDetail      string
		wantType        string
		wantActivity    string
	}{
		{"assistant text redacted", 0, "(text output)", "", "assistant", "text"},
		{"thinking redacted", 1, "(thinking)", "", "assistant", "thinking"},
		{"bash command redacted", 2, "Bash", "", "assistant", "tool_use"},
		{"read path redacted", 3, "Read", "", "assistant", "tool_use"},
		{"grep pattern redacted", 4, "Grep", "", "assistant", "tool_use"},
		{"agent desc redacted", 5, "Agent", "", "assistant", "tool_use"},
		{"tool result redacted", 6, "(result)", "", "user", "tool_result"},
		{"user text redacted", 7, "(user input)", "", "user", "text"},
		{"subagent tool_use redacted", 8, "[sub1] Bash", "", "progress", "tool_use"},
		{"subagent thinking redacted", 9, "[sub1] (thinking)", "", "progress", "thinking"},
		{"subagent tool_result redacted", 10, "[sub1] (result)", "", "progress", "tool_result"},
		{"subagent progress unchanged", 11, "[sub1] progress", "", "progress", "subagent"},
		{"system unchanged", 12, "context compacted", "", "system", "compact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := sanitized[tt.idx]
			if e.Summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", e.Summary, tt.wantSummary)
			}
			if e.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", e.Detail, tt.wantDetail)
			}
			if e.Type != tt.wantType {
				t.Errorf("type = %q, want %q", e.Type, tt.wantType)
			}
			if e.Activity != tt.wantActivity {
				t.Errorf("activity = %q, want %q", e.Activity, tt.wantActivity)
			}
		})
	}
}

func TestSanitizeTailEntry_PreservesTimestamp(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)
	entry := TailEntry{Timestamp: ts, Type: "assistant", Activity: "text", Summary: "sensitive text"}
	result := sanitizeTailEntry(entry)
	if !result.Timestamp.Equal(ts) {
		t.Errorf("timestamp changed: got %v, want %v", result.Timestamp, ts)
	}
}

func TestSplitProgressPrefix(t *testing.T) {
	tests := []struct {
		input      string
		wantPrefix string
		wantRest   string
	}{
		{"[sub1] Bash: cmd", "[sub1] ", "Bash: cmd"},
		{"[abc] text here", "[abc] ", "text here"},
		{"no prefix", "", "no prefix"},
		{"[incomplete", "", "[incomplete"},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			prefix, rest := splitProgressPrefix(tt.input)
			if prefix != tt.wantPrefix {
				t.Errorf("prefix = %q, want %q", prefix, tt.wantPrefix)
			}
			if rest != tt.wantRest {
				t.Errorf("rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bash: cat /etc/passwd", "Bash"},
		{"Read session/tail.go", "Read"},
		{"Grep secret_key", "Grep"},
		{"Agent: Find credentials", "Agent"},
		{"WebSearch: how to hack", "WebSearch"},
		{"WebFetch https://example.com", "WebFetch"},
		{"Glob **/*.go", "Glob"},
		{"Write", "Write"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractToolName(tt.input)
			if got != tt.want {
				t.Errorf("extractToolName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
