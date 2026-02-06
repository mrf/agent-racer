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

func TestDecodeProjectPath(t *testing.T) {
	// Create temp directories for testing
	tmpBase := t.TempDir()
	testDirs := []string{
		filepath.Join(tmpBase, "simple-project"),
		filepath.Join(tmpBase, "multi-dash-project"),
		filepath.Join(tmpBase, "no-dashes"),
	}
	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		encoded  string
		expected string
	}{
		// Paths that exist on filesystem - should return full decoded path
		{"existing simple-project", encodeProjectPath(testDirs[0]), testDirs[0]},
		{"existing multi-dash-project", encodeProjectPath(testDirs[1]), testDirs[1]},
		{"existing no-dashes", encodeProjectPath(testDirs[2]), testDirs[2]},
		// Paths that don't exist - should return basename as fallback
		{"non-existent with dashes", "-nonexistent-path-my-project", "my-project"},
		{"non-existent single segment", "-foo", "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeProjectPath(tt.encoded)
			if got != tt.expected {
				t.Errorf("DecodeProjectPath(%q) = %q, want %q", tt.encoded, got, tt.expected)
			}
		})
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

// TestParseSessionJSONLExtractsCwd verifies that the parser extracts the cwd
// field from JSONL entries and uses the latest value. This is a regression test:
// worktree sessions write to ~/.claude/projects/-home-mrf/ (home dir project)
// but the actual cwd changes to the worktree path after the first tool call.
// Without cwd extraction, all worktree sessions show "mrf" as their name.
func TestParseSessionJSONLExtractsCwd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Simulate a worktree session: cwd starts as /home/mrf, then changes
	content := `{"type":"user","cwd":"/home/mrf","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-cwd","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"assistant","cwd":"/home/mrf","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"hi"}]},"sessionId":"test-cwd","timestamp":"2026-01-30T10:00:01.000Z"}
{"type":"user","cwd":"/home/mrf/Projects/agent-racer--fix-foo","message":{"role":"user","content":[{"type":"text","text":"read a file"}]},"sessionId":"test-cwd","timestamp":"2026-01-30T10:00:02.000Z"}
{"type":"assistant","cwd":"/home/mrf/Projects/agent-racer--fix-foo","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"t1","input":{}}]},"sessionId":"test-cwd","timestamp":"2026-01-30T10:00:03.000Z"}
`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Should use the LATEST cwd, not the first one
	if result.WorkingDir != "/home/mrf/Projects/agent-racer--fix-foo" {
		t.Errorf("WorkingDir = %q, want %q (should use latest cwd)",
			result.WorkingDir, "/home/mrf/Projects/agent-racer--fix-foo")
	}
}

// TestParseSessionJSONLCwdEmptyWhenMissing verifies WorkingDir is empty
// when no cwd field is present in any JSONL entries.
func TestParseSessionJSONLCwdEmptyWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-no-cwd","timestamp":"2026-01-30T10:00:00.000Z"}
`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	if result.WorkingDir != "" {
		t.Errorf("WorkingDir = %q, want empty (no cwd in entries)", result.WorkingDir)
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

// TestParseSessionJSONLNoFinalNewline verifies that incomplete lines (no trailing newline) are NOT processed
// until they receive a newline. This prevents data loss during partial writes.
func TestParseSessionJSONLNoFinalNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Write JSONL without final newline - the second line is incomplete
	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, offset, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Should only parse the first complete line (with newline), not the incomplete second line
	if result.MessageCount != 1 {
		t.Errorf("expected 1 message (only complete lines), got %d", result.MessageCount)
	}

	// Now complete the file by adding a newline
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("\n")
	f.Close()

	// Re-read from offset - should now parse the previously incomplete line
	result2, offset2, err := ParseSessionJSONL(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	if result2.MessageCount != 1 {
		t.Errorf("expected 1 new message (now complete with newline), got %d", result2.MessageCount)
	}
	if offset2 == offset {
		t.Errorf("expected offset to advance after line completion, got same offset %d", offset)
	}
}

// TestParseSessionJSONLLargeLine verifies that lines larger than 1MB are handled correctly
func TestParseSessionJSONLLargeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Create a line with content larger than 1MB
	largeContent := make([]byte, 2*1024*1024)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	// Create a valid JSONL line with the large content field
	line1 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}` + "\n"
	line2Prefix := `{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":"` + string(largeContent) + `","usage":{"input_tokens":100,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}` + "\n"

	content := line1 + line2Prefix

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// This should not panic or fail with buffer overflow
	result, _, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Should have parsed at least the first line
	if result.MessageCount < 1 {
		t.Errorf("expected at least 1 message, got %d", result.MessageCount)
	}
}

// TestParseSessionJSONLPartialWrite simulates incremental writes without final newline
func TestParseSessionJSONLPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Write initial data with newline
	content1 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}

	result1, offset1, err := ParseSessionJSONL(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result1.MessageCount != 1 {
		t.Errorf("expected 1 message, got %d", result1.MessageCount)
	}

	// Append partial line (no newline) - simulates mid-write
	content2 := `{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content2)
	f.Close()

	// Re-read from offset - should not parse incomplete line
	result2, offset2, err := ParseSessionJSONL(path, offset1)
	if err != nil {
		t.Fatal(err)
	}
	if result2.MessageCount != 0 {
		t.Errorf("expected 0 messages (incomplete line), got %d", result2.MessageCount)
	}

	// Complete the line with newline
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("\n")
	f.Close()

	// Re-read from offset - should now parse the completed line
	result3, offset3, err := ParseSessionJSONL(path, offset2)
	if err != nil {
		t.Fatal(err)
	}
	if result3.MessageCount != 1 {
		t.Errorf("expected 1 message (now complete), got %d", result3.MessageCount)
	}
	if offset3 == offset2 {
		t.Errorf("expected offset to advance after completing line, got same offset %d", offset2)
	}
}
