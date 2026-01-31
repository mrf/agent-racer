package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/project", "-home-user-project"},
		{"/home/mrf/Projects/agent-racer", "-home-mrf-Projects-agent-racer"},
		{"/tmp/test", "-tmp-test"},
	}

	for _, tt := range tests {
		got := encodeProjectPath(tt.input)
		if got != tt.expected {
			t.Errorf("encodeProjectPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseSessionJSONL(t *testing.T) {
	// Create a temp JSONL file with test data
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"hi there"}],"usage":{"input_tokens":100,"cache_creation_input_tokens":500,"cache_read_input_tokens":2000,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"do something"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:02.000Z"}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"toolu_123","input":{}}],"usage":{"input_tokens":200,"cache_creation_input_tokens":600,"cache_read_input_tokens":3000,"output_tokens":80}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:03.000Z"}
`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, offset, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	if offset == 0 {
		t.Error("expected non-zero offset after parsing")
	}

	if result.SessionID != "test-123" {
		t.Errorf("expected sessionId test-123, got %s", result.SessionID)
	}

	if result.Model != "claude-opus-4-5-20251101" {
		t.Errorf("expected model claude-opus-4-5-20251101, got %s", result.Model)
	}

	if result.MessageCount != 4 {
		t.Errorf("expected 4 messages, got %d", result.MessageCount)
	}

	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", result.ToolCalls)
	}

	if result.LastTool != "Read" {
		t.Errorf("expected last tool Read, got %s", result.LastTool)
	}

	if result.LatestUsage == nil {
		t.Fatal("expected non-nil usage")
	}

	expectedTotal := 200 + 600 + 3000
	if result.LatestUsage.TotalContext() != expectedTotal {
		t.Errorf("expected total context %d, got %d", expectedTotal, result.LatestUsage.TotalContext())
	}

	if result.LastActivity != "tool_use" {
		t.Errorf("expected last activity tool_use, got %s", result.LastActivity)
	}

	// Test incremental parsing: parse from saved offset should yield no new entries
	result2, offset2, err := ParseSessionJSONL(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	if result2.MessageCount != 0 {
		t.Errorf("expected 0 new messages on re-read, got %d", result2.MessageCount)
	}
	if offset2 != offset {
		t.Errorf("expected offset unchanged, got %d vs %d", offset2, offset)
	}
}

func TestTokenUsageTotalContext(t *testing.T) {
	usage := TokenUsage{
		InputTokens:              100,
		CacheCreationInputTokens: 500,
		CacheReadInputTokens:     2000,
		OutputTokens:             50,
	}

	expected := 2600
	if usage.TotalContext() != expected {
		t.Errorf("TotalContext() = %d, want %d", usage.TotalContext(), expected)
	}
}

func TestSessionIDFromPath(t *testing.T) {
	path := "/home/user/.claude/projects/-home-user-proj/abc-123-def.jsonl"
	id := SessionIDFromPath(path)
	if id != "abc-123-def" {
		t.Errorf("SessionIDFromPath() = %q, want %q", id, "abc-123-def")
	}
}
