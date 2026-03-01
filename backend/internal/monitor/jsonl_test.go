package monitor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Simple directories (no hyphens in components)
	simpleDirs := []string{
		filepath.Join(tmpBase, "simple"),
		filepath.Join(tmpBase, "nested", "deep"),
	}

	// Directories with hyphens in components (the ambiguous case)
	hyphenDirs := []string{
		filepath.Join(tmpBase, "my-project"),
		filepath.Join(tmpBase, "a-b", "c-d"),
	}

	for _, dir := range append(simpleDirs, hyphenDirs...) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("round-trip simple paths", func(t *testing.T) {
		for _, dir := range simpleDirs {
			encoded := encodeProjectPath(dir)
			got := DecodeProjectPath(encoded)
			if got != dir {
				t.Errorf("round-trip %q: encoded=%q, decoded=%q", dir, encoded, got)
			}
		}
	})

	t.Run("round-trip hyphenated paths", func(t *testing.T) {
		for _, dir := range hyphenDirs {
			encoded := encodeProjectPath(dir)
			got := DecodeProjectPath(encoded)
			if got != dir {
				t.Errorf("round-trip %q: encoded=%q, decoded=%q", dir, encoded, got)
			}
		}
	})

	t.Run("non-existent falls back to all slashes", func(t *testing.T) {
		got := DecodeProjectPath("-nonexistent-path-my-project")
		if got != "/nonexistent/path/my/project" {
			t.Errorf("fallback = %q, want /nonexistent/path/my/project", got)
		}
	})

	t.Run("single segment", func(t *testing.T) {
		got := DecodeProjectPath("-foo")
		if got != "/foo" {
			t.Errorf("got %q, want /foo", got)
		}
	})

	t.Run("no leading dash passthrough", func(t *testing.T) {
		got := DecodeProjectPath("noleadingdash")
		if got != "noleadingdash" {
			t.Errorf("got %q, want noleadingdash", got)
		}
	})
}

// TestDecodeProjectPathWithHyphensInComponentNames tests decoding of paths
// where directory components themselves contain hyphens. These are ambiguous
// cases where the same encoded path could map to multiple directory structures.
// The decoder must try all possibilities and find the one that exists.
func TestDecodeProjectPathWithHyphensInComponentNames(t *testing.T) {
	tmpBase := t.TempDir()

	tests := []struct {
		name     string
		origPath string
	}{
		{
			name:     "single component with hyphens",
			origPath: filepath.Join(tmpBase, "my-cool-app"),
		},
		{
			name:     "multiple components with single hyphens each",
			origPath: filepath.Join(tmpBase, "my-project", "src-code"),
		},
		{
			name:     "deeply nested with hyphens",
			origPath: filepath.Join(tmpBase, "my-company", "my-team", "my-project"),
		},
		{
			name:     "mixed: some with hyphens some without",
			origPath: filepath.Join(tmpBase, "project", "my-build", "output"),
		},
		{
			name:     "component with multiple hyphens",
			origPath: filepath.Join(tmpBase, "my-cool-app-v2"),
		},
		{
			name:     "leading component with hyphens followed by plain",
			origPath: filepath.Join(tmpBase, "user-name", "projects"),
		},
	}

	// Create all test directories
	for _, tt := range tests {
		if err := os.MkdirAll(tt.origPath, 0755); err != nil {
			t.Fatalf("failed to create test directory %q: %v", tt.origPath, err)
		}
	}

	// Use traditional for loop as per requirements
	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeProjectPath(tt.origPath)
			decoded := DecodeProjectPath(encoded)

			if decoded != tt.origPath {
				t.Errorf("failed to decode path with hyphens in component names:\n  original: %q\n  encoded:  %q\n  decoded:  %q",
					tt.origPath, encoded, decoded)
			}
		})
	}
}

// TestDecodeProjectPathAmbiguousPaths verifies that when multiple valid
// interpretations of an encoded path exist on the filesystem, the decoder
// returns one of them (filesystem walk picks the first match).
func TestDecodeProjectPathAmbiguousPaths(t *testing.T) {
	tmpBase := t.TempDir()

	// Create two different directory structures that could both be valid
	// interpretations of the same encoded string.
	// Path 1: /tmp/.../home/user-name/project
	// Path 2: /tmp/.../home-user/name/project (if it existed)
	// When we encode Path 1, we get: -home-user-name-project
	// Path 1 exists, so DecodeProjectPath should find it.

	path1 := filepath.Join(tmpBase, "home", "user-name", "project")
	if err := os.MkdirAll(path1, 0755); err != nil {
		t.Fatalf("failed to create path1: %v", err)
	}

	encoded := encodeProjectPath(path1)
	decoded := DecodeProjectPath(encoded)

	if decoded != path1 {
		t.Errorf("ambiguous path resolution failed:\n  expected: %q\n  got:      %q",
			path1, decoded)
	}
}

// TestDecodeProjectPathComponentBoundaries tests paths where hyphens in
// component names could be confused with path separators during decoding.
func TestDecodeProjectPathComponentBoundaries(t *testing.T) {
	tmpBase := t.TempDir()

	// Test case: the encoding is lossy, so we need to verify that
	// DecodeProjectPath correctly reconstructs when the path exists.
	tests := []struct {
		description string
		pathParts   []string
	}{
		{
			description: "two components with single hyphens",
			pathParts:   []string{"my-app", "src-code"},
		},
		{
			description: "three components with multiple hyphens",
			pathParts:   []string{"a-b-c", "d-e-f", "g-h-i"},
		},
		{
			description: "mixed hyphenated and plain components",
			pathParts:   []string{"my-app", "v1", "build-output"},
		},
	}

	// Use traditional for loop as per requirements
	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.description, func(t *testing.T) {
			// Build the full path
			fullPath := tmpBase
			for j := 0; j < len(tt.pathParts); j++ {
				fullPath = filepath.Join(fullPath, tt.pathParts[j])
			}

			// Create the directory structure
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				t.Fatalf("failed to create directory: %v", err)
			}

			// Encode and decode
			encoded := encodeProjectPath(fullPath)
			decoded := DecodeProjectPath(encoded)

			if decoded != fullPath {
				t.Errorf("component boundary test failed:\n  original: %q\n  encoded:  %q\n  decoded:  %q",
					fullPath, encoded, decoded)
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

	result, offset, err := ParseSessionJSONL(path, 0, "", nil)
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
	result2, offset2, err := ParseSessionJSONL(path, offset, "", nil)
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

	result, _, err := ParseSessionJSONL(path, 0, "", nil)
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

	result, _, err := ParseSessionJSONL(path, 0, "", nil)
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

	result, offset, err := ParseSessionJSONL(path, 0, "", nil)
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
	result2, offset2, err := ParseSessionJSONL(path, offset, "", nil)
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

// TestParseSessionJSONLLargeLine verifies that lines larger than maxLineLength are
// skipped gracefully and subsequent lines are still parsed.
func TestParseSessionJSONLLargeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Create a line with content larger than maxLineLength (1 MB)
	largeContent := bytes.Repeat([]byte("x"), 2*1024*1024)

	line1 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}` + "\n"
	oversizedLine := `{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":"` + string(largeContent) + `","usage":{"input_tokens":100,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}` + "\n"
	line3 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"after big line"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:02.000Z"}` + "\n"

	content := line1 + oversizedLine + line3

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, newOffset, err := ParseSessionJSONL(path, 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should parse line1 and line3, skipping the oversized line
	if result.MessageCount != 2 {
		t.Errorf("expected 2 messages (skipping oversized line), got %d", result.MessageCount)
	}

	// Offset should cover all three lines (including skipped)
	expectedOffset := int64(len(line1) + len(oversizedLine) + len(line3))
	if newOffset != expectedOffset {
		t.Errorf("offset = %d, want %d", newOffset, expectedOffset)
	}
}

// TestParseSessionJSONLFileSizeLimit verifies that normal-sized files are accepted.
// Creating a real 500 MB+ file to test the rejection path is impractical in unit
// tests; the guard is validated by inspection and integration testing.
func TestParseSessionJSONLFileSizeLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Normal-sized file should parse fine
	result, _, err := ParseSessionJSONL(path, 0, "", nil)
	if err != nil {
		t.Fatalf("expected no error for normal file, got: %v", err)
	}
	if result.MessageCount != 1 {
		t.Errorf("expected 1 message, got %d", result.MessageCount)
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

	result1, offset1, err := ParseSessionJSONL(path, 0, "", nil)
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
	result2, offset2, err := ParseSessionJSONL(path, offset1, "", nil)
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
	result3, offset3, err := ParseSessionJSONL(path, offset2, "", nil)
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

// writeJSONLLines creates a temporary JSONL file from the given lines and returns its path.
// Each line is written with a trailing newline.
func writeJSONLLines(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// parseJSONL is a test helper that parses a JSONL file from offset 0 and fails on error.
func parseJSONL(t *testing.T, path string) *ParseResult {
	t.Helper()
	result, _, err := ParseSessionJSONL(path, 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// requireSubagent looks up a subagent by toolUseID and fails if not found.
func requireSubagent(t *testing.T, result *ParseResult, toolUseID string) *SubagentParseResult {
	t.Helper()
	sub, ok := result.Subagents[toolUseID]
	if !ok {
		t.Fatalf("expected subagent with ID %s", toolUseID)
	}
	return sub
}

// TestParseProgressEntry tests parsing of single type:"progress" JSONL entries,
// covering assistant messages, user messages, null data, and timestamp handling.
func TestParseProgressEntry(t *testing.T) {
	t.Run("assistant message with tool use", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"progress","toolUseID":"tool-1","parentToolUseID":"tool-1","sessionId":"test-sub","slug":"my-subagent","timestamp":"2026-01-30T10:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"sub-tool-1"}],"usage":{"input_tokens":50,"cache_creation_input_tokens":100,"cache_read_input_tokens":500,"output_tokens":25}}}}}`,
		)

		result := parseJSONL(t, path)
		if len(result.Subagents) != 1 {
			t.Fatalf("expected 1 subagent, got %d", len(result.Subagents))
		}

		sub := requireSubagent(t, result, "tool-1")

		if sub.ID != "tool-1" {
			t.Errorf("ID = %s, want tool-1", sub.ID)
		}
		if sub.ParentToolUseID != "tool-1" {
			t.Errorf("ParentToolUseID = %s, want tool-1", sub.ParentToolUseID)
		}
		if sub.Slug != "my-subagent" {
			t.Errorf("Slug = %s, want my-subagent", sub.Slug)
		}
		if sub.Model != "claude-opus-4-5-20251101" {
			t.Errorf("Model = %s, want claude-opus-4-5-20251101", sub.Model)
		}
		if sub.MessageCount != 1 {
			t.Errorf("MessageCount = %d, want 1", sub.MessageCount)
		}
		if sub.ToolCalls != 1 {
			t.Errorf("ToolCalls = %d, want 1", sub.ToolCalls)
		}
		if sub.LastTool != "Read" {
			t.Errorf("LastTool = %s, want Read", sub.LastTool)
		}
		if sub.LastActivity != "tool_use" {
			t.Errorf("LastActivity = %s, want tool_use", sub.LastActivity)
		}
		if sub.LatestUsage == nil {
			t.Fatal("expected non-nil LatestUsage")
		}
		if got, want := sub.LatestUsage.TotalContext(), 50+100+500; got != want {
			t.Errorf("TotalContext() = %d, want %d", got, want)
		}
	})

	t.Run("user message sets waiting activity", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"progress","toolUseID":"tool-2","parentToolUseID":"tool-2","sessionId":"test-sub","slug":"subagent-2","timestamp":"2026-01-30T10:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"do something"}]}}}}`,
		)

		sub := requireSubagent(t, parseJSONL(t, path), "tool-2")

		if sub.MessageCount != 1 {
			t.Errorf("MessageCount = %d, want 1", sub.MessageCount)
		}
		if sub.LastActivity != "waiting" {
			t.Errorf("LastActivity = %s, want waiting", sub.LastActivity)
		}
	})

	t.Run("null data creates subagent without messages", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"progress","toolUseID":"tool-6","parentToolUseID":"tool-6","sessionId":"test-sub","slug":"no-data","timestamp":"2026-01-30T10:00:00.000Z","data":null}`,
		)

		sub := requireSubagent(t, parseJSONL(t, path), "tool-6")

		if sub.ID != "tool-6" {
			t.Errorf("ID = %s, want tool-6", sub.ID)
		}
		if sub.Slug != "no-data" {
			t.Errorf("Slug = %s, want no-data", sub.Slug)
		}
		if sub.MessageCount != 0 {
			t.Errorf("MessageCount = %d, want 0", sub.MessageCount)
		}
	})

	t.Run("timestamp parsing", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"progress","toolUseID":"tool-7","parentToolUseID":"tool-7","sessionId":"test-sub","slug":"timestamp-test","timestamp":"2026-02-20T15:30:45.123456789Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[]}}}}`,
		)

		sub := requireSubagent(t, parseJSONL(t, path), "tool-7")

		if sub.FirstTime.IsZero() {
			t.Error("expected FirstTime to be set")
		}
		if sub.FirstTime != sub.LastTime {
			t.Error("with one entry, FirstTime should equal LastTime")
		}
		if sub.FirstTime.Year() != 2026 {
			t.Errorf("FirstTime.Year() = %d, want 2026", sub.FirstTime.Year())
		}
	})
}

// TestParseMultipleProgressEntries tests accumulating state across multiple
// progress entries for the same subagent: messages, tool calls, and timestamps.
func TestParseMultipleProgressEntries(t *testing.T) {
	path := writeJSONLLines(t,
		// assistant with Read tool
		`{"type":"progress","toolUseID":"tool-3","parentToolUseID":"tool-3","sessionId":"test-sub","slug":"my-task","timestamp":"2026-01-30T10:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"t1"}],"usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":1000,"output_tokens":50}}}}}`,
		// user reply
		`{"type":"progress","toolUseID":"tool-3","parentToolUseID":"tool-3","sessionId":"test-sub","slug":"my-task","timestamp":"2026-01-30T10:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"continue"}]}}}}`,
		// assistant with Write tool
		`{"type":"progress","toolUseID":"tool-3","parentToolUseID":"tool-3","sessionId":"test-sub","slug":"my-task","timestamp":"2026-01-30T10:00:02.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Write","id":"t2"}],"usage":{"input_tokens":150,"cache_creation_input_tokens":250,"cache_read_input_tokens":1500,"output_tokens":75}}}}}`,
	)

	result := parseJSONL(t, path)
	if len(result.Subagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(result.Subagents))
	}

	sub := requireSubagent(t, result, "tool-3")

	if sub.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3 (assistant + user + assistant)", sub.MessageCount)
	}
	if sub.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2 (Read + Write)", sub.ToolCalls)
	}
	if sub.LastTool != "Write" {
		t.Errorf("LastTool = %s, want Write", sub.LastTool)
	}
	if sub.LastActivity != "tool_use" {
		t.Errorf("LastActivity = %s, want tool_use", sub.LastActivity)
	}
	if sub.FirstTime.IsZero() || sub.LastTime.IsZero() {
		t.Fatal("expected both FirstTime and LastTime to be set")
	}
	if !sub.FirstTime.Before(sub.LastTime) {
		t.Error("expected FirstTime before LastTime")
	}
}

// TestCheckSubagentCompletion tests detection of subagent completion via tool_result
// entries that match (or do not match) the subagent's parentToolUseID.
func TestCheckSubagentCompletion(t *testing.T) {
	tests := []struct {
		name        string
		toolUseID   string
		parentID    string
		resultID    string // tool_use_id in the tool_result entry
		wantDone    bool
	}{
		{
			name:      "matching tool_result marks subagent completed",
			toolUseID: "tool-4",
			parentID:  "tool-4",
			resultID:  "tool-4",
			wantDone:  true,
		},
		{
			name:      "non-matching tool_result leaves subagent incomplete",
			toolUseID: "tool-5",
			parentID:  "tool-5",
			resultID:  "wrong-id",
			wantDone:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeJSONLLines(t,
				fmt.Sprintf(`{"type":"progress","toolUseID":"%s","parentToolUseID":"%s","sessionId":"test-sub","slug":"subagent","timestamp":"2026-01-30T10:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Bash","id":"t1"}]}}}}`, tt.toolUseID, tt.parentID),
				fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"%s","content":"output"}]},"sessionId":"test-sub","timestamp":"2026-01-30T10:00:01.000Z"}`, tt.resultID),
			)

			sub := requireSubagent(t, parseJSONL(t, path), tt.toolUseID)

			if sub.Completed != tt.wantDone {
				t.Errorf("Completed = %v, want %v", sub.Completed, tt.wantDone)
			}
		})
	}
}

// TestMultipleSubagentsIncrementalParsing tests that incremental parsing from a
// saved offset only returns newly appended subagents.
func TestMultipleSubagentsIncrementalParsing(t *testing.T) {
	path := writeJSONLLines(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"start"}]},"sessionId":"sess-1","timestamp":"2026-01-30T10:00:00.000Z"}`,
		`{"type":"progress","toolUseID":"sub-1","parentToolUseID":"sub-1","sessionId":"sess-1","slug":"sub1","timestamp":"2026-01-30T10:00:01.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"working"}]}}}}`,
	)

	result1, offset1, err := ParseSessionJSONL(path, 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result1.Subagents) != 1 {
		t.Fatalf("batch 1: expected 1 subagent, got %d", len(result1.Subagents))
	}

	// Append a second subagent entry
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"type":"progress","toolUseID":"sub-2","parentToolUseID":"sub-2","sessionId":"sess-1","slug":"sub2","timestamp":"2026-01-30T10:00:02.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"also working"}]}}}}` + "\n")
	f.Close()

	result2, offset2, err := ParseSessionJSONL(path, offset1, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result2.Subagents) != 1 {
		t.Fatalf("batch 2: expected 1 new subagent, got %d", len(result2.Subagents))
	}

	sub2 := requireSubagent(t, result2, "sub-2")
	if sub2.Slug != "sub2" {
		t.Errorf("Slug = %s, want sub2", sub2.Slug)
	}
	if offset2 <= offset1 {
		t.Errorf("expected offset to advance: %d -> %d", offset1, offset2)
	}
}

// TestCompactBoundaryDetection verifies that type:"system" subtype:"compact_boundary"
// entries are counted as compaction events.
func TestCompactBoundaryDetection(t *testing.T) {
	t.Run("single compaction event", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-compact","slug":"my-session","timestamp":"2026-01-30T10:00:00.000Z"}`,
			`{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"output_tokens":50}},"sessionId":"test-compact","slug":"my-session","timestamp":"2026-01-30T10:00:01.000Z"}`,
			`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","sessionId":"test-compact","slug":"my-session","timestamp":"2026-01-30T10:00:02.000Z","uuid":"abc-123","level":"info","isMeta":false,"compactMetadata":{"trigger":"auto","preTokens":167220}}`,
			`{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"continuing after compaction"}],"usage":{"input_tokens":50,"output_tokens":30}},"sessionId":"test-compact","slug":"my-session","timestamp":"2026-01-30T10:00:03.000Z"}`,
		)

		result := parseJSONL(t, path)

		if result.CompactionCount != 1 {
			t.Errorf("CompactionCount = %d, want 1", result.CompactionCount)
		}
		if result.MessageCount != 3 {
			t.Errorf("MessageCount = %d, want 3 (system entries are not messages)", result.MessageCount)
		}
	})

	t.Run("multiple compaction events", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-compact-2","slug":"my-session","timestamp":"2026-01-30T10:00:00.000Z"}`,
			`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","sessionId":"test-compact-2","slug":"my-session","timestamp":"2026-01-30T10:00:01.000Z","uuid":"abc-1","level":"info"}`,
			`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","sessionId":"test-compact-2","slug":"my-session","timestamp":"2026-01-30T10:00:02.000Z","uuid":"abc-2","level":"info"}`,
		)

		result := parseJSONL(t, path)

		if result.CompactionCount != 2 {
			t.Errorf("CompactionCount = %d, want 2", result.CompactionCount)
		}
	})

	t.Run("other system subtypes are not counted", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"system","subtype":"turn_duration","durationMs":5000,"sessionId":"test-sys","slug":"my-session","timestamp":"2026-01-30T10:00:00.000Z","uuid":"abc-3"}`,
			`{"type":"system","subtype":"stop_hook_summary","sessionId":"test-sys","slug":"my-session","timestamp":"2026-01-30T10:00:01.000Z","uuid":"abc-4"}`,
		)

		result := parseJSONL(t, path)

		if result.CompactionCount != 0 {
			t.Errorf("CompactionCount = %d, want 0", result.CompactionCount)
		}
	})

	t.Run("incremental parse detects compaction", func(t *testing.T) {
		path := writeJSONLLines(t,
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-inc","slug":"my-session","timestamp":"2026-01-30T10:00:00.000Z"}`,
		)

		_, offset, err := ParseSessionJSONL(path, 0, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Append compaction event
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","sessionId":"test-inc","slug":"my-session","timestamp":"2026-01-30T10:00:05.000Z","uuid":"abc-5","level":"info","compactMetadata":{"trigger":"auto","preTokens":167000}}` + "\n")
		f.Close()

		result, _, err := ParseSessionJSONL(path, offset, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.CompactionCount != 1 {
			t.Errorf("CompactionCount = %d, want 1", result.CompactionCount)
		}
	})
}

// TestSubagentActivityTransitions verifies that activity state tracks the latest
// progress entry: thinking -> tool_use -> waiting.
func TestSubagentActivityTransitions(t *testing.T) {
	path := writeJSONLLines(t,
		// thinking (text-only assistant message)
		`{"type":"progress","toolUseID":"tool-8","parentToolUseID":"tool-8","sessionId":"test-sub","slug":"activity-test","timestamp":"2026-01-30T10:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"thinking"}]}}}}`,
		// tool_use
		`{"type":"progress","toolUseID":"tool-8","parentToolUseID":"tool-8","sessionId":"test-sub","slug":"activity-test","timestamp":"2026-01-30T10:00:01.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"t1"}]}}}}`,
		// waiting (user message)
		`{"type":"progress","toolUseID":"tool-8","parentToolUseID":"tool-8","sessionId":"test-sub","slug":"activity-test","timestamp":"2026-01-30T10:00:02.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"waiting for input"}]}}}}`,
	)

	sub := requireSubagent(t, parseJSONL(t, path), "tool-8")

	if sub.LastActivity != "waiting" {
		t.Errorf("LastActivity = %s, want waiting", sub.LastActivity)
	}
	if sub.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", sub.MessageCount)
	}
	if sub.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", sub.ToolCalls)
	}
}
