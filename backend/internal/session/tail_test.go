package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkJSONL(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func assistantText(ts, text string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":"%s","message":{"content":[{"type":"text","text":"%s"}]}}`, ts, text)
}

func userText(ts, text string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":"%s","message":{"content":[{"type":"text","text":"%s"}]}}`, ts, text)
}

func systemEntry(ts, subtype string) string {
	return fmt.Sprintf(`{"type":"system","subtype":"%s","timestamp":"%s","message":{}}`, subtype, ts)
}

func progressEntry(ts, toolUseID, slug string, data string) string {
	if data == "" {
		return fmt.Sprintf(`{"type":"progress","timestamp":"%s","toolUseID":"%s","slug":"%s"}`, ts, toolUseID, slug)
	}
	return fmt.Sprintf(`{"type":"progress","timestamp":"%s","toolUseID":"%s","slug":"%s","data":%s}`, ts, toolUseID, slug, data)
}

const testTS = "2026-03-26T10:00:00Z"

func mustParseTS(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func writeTempJSONL(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestTruncateLine(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "truncated with ellipsis",
			input:  "hello world this is long",
			maxLen: 10,
			want:   "hello wor…",
		},
		{
			name:   "multiline collapsed to first line",
			input:  "first line\nsecond line",
			maxLen: 120,
			want:   "first line",
		},
		{
			name:   "multiline truncated",
			input:  "a long first line here\nsecond",
			maxLen: 10,
			want:   "a long fi…",
		},
		{
			name:   "maxLen 3 no ellipsis",
			input:  "abcdef",
			maxLen: 3,
			want:   "abc",
		},
		{
			name:   "maxLen 2 no ellipsis",
			input:  "abcdef",
			maxLen: 2,
			want:   "ab",
		},
		{
			name:   "whitespace trimmed",
			input:  "  hello  ",
			maxLen: 10,
			want:   "hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateLine(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateLine(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "long path shortened to last 2",
			input: "/home/user/projects/myapp/src/main.go",
			want:  "src/main.go",
		},
		{
			name:  "two components unchanged",
			input: "src/main.go",
			want:  "src/main.go",
		},
		{
			name:  "single component unchanged",
			input: "main.go",
			want:  "main.go",
		},
		{
			name:  "three components shortened",
			input: "a/b/c",
			want:  "b/c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenPath(tt.input)
			if got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToolUseSummary(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    json.RawMessage
		want     string
	}{
		{
			name:     "nil input returns name",
			toolName: "Read",
			input:    nil,
			want:     "Read",
		},
		{
			name:     "Read with file_path",
			toolName: "Read",
			input:    json.RawMessage(`{"file_path":"/home/user/project/src/main.go"}`),
			want:     "Read src/main.go",
		},
		{
			name:     "Write with file_path",
			toolName: "Write",
			input:    json.RawMessage(`{"file_path":"/a/b/c/test.ts"}`),
			want:     "Write c/test.ts",
		},
		{
			name:     "Edit with file_path",
			toolName: "Edit",
			input:    json.RawMessage(`{"file_path":"/x/y.go"}`),
			want:     "Edit x/y.go",
		},
		{
			name:     "Bash with command",
			toolName: "Bash",
			input:    json.RawMessage(`{"command":"go test ./..."}`),
			want:     "Bash: go test ./...",
		},
		{
			name:     "Bash with long command truncated",
			toolName: "Bash",
			input:    json.RawMessage(fmt.Sprintf(`{"command":"%s"}`, strings.Repeat("x", 100))),
			want:     "Bash: " + strings.Repeat("x", 79) + "…",
		},
		{
			name:     "Glob with pattern",
			toolName: "Glob",
			input:    json.RawMessage(`{"pattern":"**/*.go"}`),
			want:     "Glob **/*.go",
		},
		{
			name:     "Grep with pattern",
			toolName: "Grep",
			input:    json.RawMessage(`{"pattern":"func Test"}`),
			want:     "Grep func Test",
		},
		{
			name:     "Agent with description",
			toolName: "Agent",
			input:    json.RawMessage(`{"description":"explore codebase"}`),
			want:     "Agent: explore codebase",
		},
		{
			name:     "WebSearch with query",
			toolName: "WebSearch",
			input:    json.RawMessage(`{"query":"golang testing"}`),
			want:     "WebSearch: golang testing",
		},
		{
			name:     "WebFetch with url",
			toolName: "WebFetch",
			input:    json.RawMessage(`{"url":"https://example.com/api"}`),
			want:     "WebFetch https://example.com/api",
		},
		{
			name:     "unknown tool returns name",
			toolName: "CustomTool",
			input:    json.RawMessage(`{"foo":"bar"}`),
			want:     "CustomTool",
		},
		{
			name:     "invalid JSON input returns name",
			toolName: "Read",
			input:    json.RawMessage(`not json`),
			want:     "Read",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolUseSummary(tt.toolName, tt.input)
			if got != tt.want {
				t.Errorf("toolUseSummary(%q, ...) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

func TestToolResultSummary(t *testing.T) {
	tests := []struct {
		name    string
		content json.RawMessage
		want    string
	}{
		{
			name:    "nil content",
			content: nil,
			want:    "result",
		},
		{
			name:    "string content",
			content: json.RawMessage(`"file contents here"`),
			want:    "→ file contents here",
		},
		{
			name:    "string content multiline takes first line",
			content: json.RawMessage(`"first line\nsecond line"`),
			want:    "→ first line",
		},
		{
			name:    "array content with text block",
			content: json.RawMessage(`[{"type":"text","text":"output data"}]`),
			want:    "→ output data",
		},
		{
			name:    "array content with non-text block",
			content: json.RawMessage(`[{"type":"image","url":"x"}]`),
			want:    "result",
		},
		{
			name:    "invalid JSON",
			content: json.RawMessage(`not valid`),
			want:    "result",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolResultSummary(tt.content)
			if got != tt.want {
				t.Errorf("toolResultSummary(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTailAssistant(t *testing.T) {
	ts := mustParseTS(testTS)

	t.Run("nil raw returns nil", func(t *testing.T) {
		got := parseTailAssistant(nil, ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("text block", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"text","text":"hello world"}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Type != "assistant" || got[0].Activity != "text" {
			t.Errorf("type=%q activity=%q", got[0].Type, got[0].Activity)
		}
		if got[0].Summary != "hello world" {
			t.Errorf("summary=%q", got[0].Summary)
		}
		if got[0].Timestamp != ts {
			t.Errorf("timestamp mismatch")
		}
	})

	t.Run("empty text block skipped", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"text","text":"  "}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("long text gets detail", func(t *testing.T) {
		longText := strings.Repeat("a", 200)
		raw := json.RawMessage(fmt.Sprintf(`{"content":[{"type":"text","text":"%s"}]}`, longText))
		got := parseTailAssistant(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Detail == "" {
			t.Error("expected non-empty Detail for long text")
		}
		// "…" is 3 bytes in UTF-8, so max byte length is 119 + 3 = 122.
		if len(got[0].Summary) > 122 {
			t.Errorf("summary too long: %d bytes", len(got[0].Summary))
		}
	})

	t.Run("tool_use block", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/a/b/c.go"}}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Activity != "tool_use" {
			t.Errorf("activity=%q", got[0].Activity)
		}
		if got[0].Summary != "Read b/c.go" {
			t.Errorf("summary=%q", got[0].Summary)
		}
	})

	t.Run("thinking block", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"thinking","text":"considering options"}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Activity != "thinking" {
			t.Errorf("activity=%q", got[0].Activity)
		}
		if got[0].Summary != "considering options" {
			t.Errorf("summary=%q", got[0].Summary)
		}
	})

	t.Run("empty thinking skipped", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"thinking","text":""}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("multiple blocks", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"thinking","text":"hmm"},{"type":"text","text":"response"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}`)
		got := parseTailAssistant(raw, ts)
		if len(got) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(got))
		}
		if got[0].Activity != "thinking" {
			t.Errorf("[0] activity=%q", got[0].Activity)
		}
		if got[1].Activity != "text" {
			t.Errorf("[1] activity=%q", got[1].Activity)
		}
		if got[2].Activity != "tool_use" {
			t.Errorf("[2] activity=%q", got[2].Activity)
		}
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		raw := json.RawMessage(`not valid`)
		got := parseTailAssistant(raw, ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestParseTailUser(t *testing.T) {
	ts := mustParseTS(testTS)

	t.Run("nil raw returns nil", func(t *testing.T) {
		got := parseTailUser(nil, ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("tool_result string content", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"tool_result","content":"output here"}]}`)
		got := parseTailUser(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Type != "user" || got[0].Activity != "tool_result" {
			t.Errorf("type=%q activity=%q", got[0].Type, got[0].Activity)
		}
		if got[0].Summary != "→ output here" {
			t.Errorf("summary=%q", got[0].Summary)
		}
	})

	t.Run("tool_result array content", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"tool_result","content":[{"type":"text","text":"array output"}]}]}`)
		got := parseTailUser(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Summary != "→ array output" {
			t.Errorf("summary=%q", got[0].Summary)
		}
	})

	t.Run("text block", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"text","text":"user message"}]}`)
		got := parseTailUser(raw, ts)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].Activity != "text" {
			t.Errorf("activity=%q", got[0].Activity)
		}
		if got[0].Summary != "user message" {
			t.Errorf("summary=%q", got[0].Summary)
		}
	})

	t.Run("empty text skipped", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"text","text":""}]}`)
		got := parseTailUser(raw, ts)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		raw := json.RawMessage(`{broken`)
		got := parseTailUser(raw, ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestParseTailProgress(t *testing.T) {
	ts := mustParseTS(testTS)

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		got := parseTailProgress([]byte(`not json`), ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty toolUseID returns nil", func(t *testing.T) {
		got := parseTailProgress([]byte(`{"type":"progress","toolUseID":"","slug":"x"}`), ts)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("basic progress with slug", func(t *testing.T) {
		line := []byte(progressEntry(testTS, "tool123456", "my-agent", ""))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Activity != "subagent" {
			t.Errorf("activity=%q", got.Activity)
		}
		if got.Summary != "[my-agent] progress" {
			t.Errorf("summary=%q", got.Summary)
		}
	})

	t.Run("slug falls back to toolUseID prefix", func(t *testing.T) {
		line := []byte(fmt.Sprintf(`{"type":"progress","timestamp":"%s","toolUseID":"abcdef1234567890","slug":""}`, testTS))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Summary != "[abcdef12] progress" {
			t.Errorf("summary=%q", got.Summary)
		}
	})

	t.Run("nested assistant tool_use", func(t *testing.T) {
		data := `{"message":{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/a/b/c.go"}}]}}}`
		line := []byte(progressEntry(testTS, "tool12345678", "sub", data))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Activity != "tool_use" {
			t.Errorf("activity=%q", got.Activity)
		}
		if got.Summary != "[sub] Read b/c.go" {
			t.Errorf("summary=%q", got.Summary)
		}
	})

	t.Run("nested assistant text", func(t *testing.T) {
		data := `{"message":{"type":"assistant","message":{"content":[{"type":"text","text":"analyzing code"}]}}}`
		line := []byte(progressEntry(testTS, "tool12345678", "sub", data))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Activity != "thinking" {
			t.Errorf("activity=%q", got.Activity)
		}
		if got.Summary != "[sub] analyzing code" {
			t.Errorf("summary=%q", got.Summary)
		}
	})

	t.Run("nested user type", func(t *testing.T) {
		data := `{"message":{"type":"user","message":{}}}`
		line := []byte(progressEntry(testTS, "tool12345678", "sub", data))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Activity != "tool_result" {
			t.Errorf("activity=%q", got.Activity)
		}
		if got.Summary != "[sub] received result" {
			t.Errorf("summary=%q", got.Summary)
		}
	})

	t.Run("nil data returns fallback", func(t *testing.T) {
		line := []byte(progressEntry(testTS, "tool12345678", "sub", ""))
		got := parseTailProgress(line, ts)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Activity != "subagent" {
			t.Errorf("activity=%q", got.Activity)
		}
	})
}

func TestParseTailSystem(t *testing.T) {
	ts := mustParseTS(testTS)

	t.Run("compact_boundary", func(t *testing.T) {
		entry := tailJSONLEntry{Type: "system", Subtype: "compact_boundary", Timestamp: testTS}
		got := parseTailSystem(entry, ts)
		if got.Activity != "compact" || got.Summary != "context compacted" {
			t.Errorf("activity=%q summary=%q", got.Activity, got.Summary)
		}
		if got.Type != "system" {
			t.Errorf("type=%q", got.Type)
		}
	})

	t.Run("unknown subtype uses subtype as summary", func(t *testing.T) {
		entry := tailJSONLEntry{Type: "system", Subtype: "init", Timestamp: testTS}
		got := parseTailSystem(entry, ts)
		if got.Activity != "system" || got.Summary != "init" {
			t.Errorf("activity=%q summary=%q", got.Activity, got.Summary)
		}
	})

	t.Run("empty subtype", func(t *testing.T) {
		entry := tailJSONLEntry{Type: "system", Timestamp: testTS}
		got := parseTailSystem(entry, ts)
		if got.Activity != "system" || got.Summary != "system event" {
			t.Errorf("activity=%q summary=%q", got.Activity, got.Summary)
		}
	})
}

func TestParseTailEntries(t *testing.T) {
	t.Run("basic assistant text", func(t *testing.T) {
		data := mkJSONL(assistantText(testTS, "hello"))
		path := writeTempJSONL(t, data)

		entries, offset, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Summary != "hello" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
		if offset != int64(len(data)) {
			t.Errorf("offset=%d, want %d", offset, len(data))
		}
	})

	t.Run("multiple entry types", func(t *testing.T) {
		data := mkJSONL(
			assistantText(testTS, "thinking out loud"),
			userText(testTS, "user says hello"),
			systemEntry(testTS, "compact_boundary"),
		)
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		if entries[0].Type != "assistant" {
			t.Errorf("[0] type=%q", entries[0].Type)
		}
		if entries[1].Type != "user" {
			t.Errorf("[1] type=%q", entries[1].Type)
		}
		if entries[2].Type != "system" {
			t.Errorf("[2] type=%q", entries[2].Type)
		}
	})

	t.Run("offset seeking skips earlier content", func(t *testing.T) {
		line1 := assistantText(testTS, "first")
		line2 := assistantText(testTS, "second")
		data := mkJSONL(line1, line2)
		path := writeTempJSONL(t, data)

		// Offset past the first line (line + newline).
		offset1 := int64(len(line1) + 1)
		entries, offset2, err := ParseTailEntries(path, offset1, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Summary != "second" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
		if offset2 != int64(len(data)) {
			t.Errorf("offset=%d, want %d", offset2, len(data))
		}
	})

	t.Run("round-trip offset correctness", func(t *testing.T) {
		line1 := assistantText(testTS, "first")
		line2 := assistantText(testTS, "second")
		line3 := assistantText(testTS, "third")
		data := mkJSONL(line1, line2, line3)
		path := writeTempJSONL(t, data)

		// Read all from the start.
		entries1, off1, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("pass 1: %v", err)
		}
		if len(entries1) != 3 {
			t.Fatalf("pass 1: expected 3 entries, got %d", len(entries1))
		}

		// Read with returned offset — should get nothing.
		entries2, off2, err := ParseTailEntries(path, off1, 0)
		if err != nil {
			t.Fatalf("pass 2: %v", err)
		}
		if len(entries2) != 0 {
			t.Errorf("pass 2: expected 0 entries, got %d", len(entries2))
		}
		if off2 != off1 {
			t.Errorf("pass 2: offset changed from %d to %d", off1, off2)
		}

		// Append new data and re-read from offset.
		newLine := assistantText(testTS, "fourth") + "\n"
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("open for append: %v", err)
		}
		_, _ = f.WriteString(newLine)
		_ = f.Close()

		entries3, off3, err := ParseTailEntries(path, off2, 0)
		if err != nil {
			t.Fatalf("pass 3: %v", err)
		}
		if len(entries3) != 1 {
			t.Fatalf("pass 3: expected 1 entry, got %d", len(entries3))
		}
		if entries3[0].Summary != "fourth" {
			t.Errorf("pass 3: summary=%q", entries3[0].Summary)
		}
		if off3 != int64(len(data))+int64(len(newLine)) {
			t.Errorf("pass 3: offset=%d, want %d", off3, int64(len(data))+int64(len(newLine)))
		}
	})

	t.Run("limit caps entries", func(t *testing.T) {
		data := mkJSONL(
			assistantText(testTS, "one"),
			assistantText(testTS, "two"),
			assistantText(testTS, "three"),
		)
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Summary != "one" || entries[1].Summary != "two" {
			t.Errorf("entries[0]=%q, entries[1]=%q", entries[0].Summary, entries[1].Summary)
		}
	})

	t.Run("partial line at EOF is skipped", func(t *testing.T) {
		// Write a complete line + an incomplete line (no trailing newline).
		complete := assistantText(testTS, "complete") + "\n"
		partial := assistantText(testTS, "partial") // no newline
		path := writeTempJSONL(t, complete+partial)

		entries, offset, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry (partial skipped), got %d", len(entries))
		}
		if entries[0].Summary != "complete" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
		// Offset should only cover the complete line.
		if offset != int64(len(complete)) {
			t.Errorf("offset=%d, want %d", offset, len(complete))
		}
	})

	t.Run("line exceeding maxTailLineLength is skipped", func(t *testing.T) {
		// Build a valid JSON line longer than maxTailLineLength.
		longVal := strings.Repeat("x", maxTailLineLength+100)
		longLine := fmt.Sprintf(`{"type":"assistant","timestamp":"%s","message":{"content":[{"type":"text","text":"%s"}]}}`, testTS, longVal)
		normalLine := assistantText(testTS, "normal")
		data := longLine + "\n" + normalLine + "\n"
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry (long skipped), got %d", len(entries))
		}
		if entries[0].Summary != "normal" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
	})

	t.Run("invalid JSON lines are skipped", func(t *testing.T) {
		data := "this is not json\n" + assistantText(testTS, "valid") + "\n"
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Summary != "valid" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
	})

	t.Run("small file does not trigger size limit", func(t *testing.T) {
		data := mkJSONL(assistantText(testTS, "small"))
		path := writeTempJSONL(t, data)

		_, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("small file should not error: %v", err)
		}
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, _, err := ParseTailEntries("/nonexistent/path.jsonl", 0, 0)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("empty file returns no entries", func(t *testing.T) {
		path := writeTempJSONL(t, "")

		entries, offset, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
		if offset != 0 {
			t.Errorf("offset=%d, want 0", offset)
		}
	})

	t.Run("timestamp parsing", func(t *testing.T) {
		data := mkJSONL(assistantText("2026-03-26T15:04:05.123456789Z", "timestamped"))
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		want := mustParseTS("2026-03-26T15:04:05.123456789Z")
		if !entries[0].Timestamp.Equal(want) {
			t.Errorf("timestamp=%v, want %v", entries[0].Timestamp, want)
		}
	})

	t.Run("missing timestamp yields zero time", func(t *testing.T) {
		data := `{"type":"assistant","message":{"content":[{"type":"text","text":"no ts"}]}}` + "\n"
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if !entries[0].Timestamp.IsZero() {
			t.Errorf("expected zero time, got %v", entries[0].Timestamp)
		}
	})

	t.Run("unknown type is silently skipped", func(t *testing.T) {
		data := fmt.Sprintf(`{"type":"unknown","timestamp":"%s","message":{}}`, testTS) + "\n"
		data += assistantText(testTS, "real") + "\n"
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Summary != "real" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
	})

	t.Run("progress entry integration", func(t *testing.T) {
		progressData := `{"message":{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make test"}}]}}}`
		line := progressEntry(testTS, "tool12345678", "builder", progressData)
		data := mkJSONL(line)
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Type != "progress" {
			t.Errorf("type=%q", entries[0].Type)
		}
		if entries[0].Activity != "tool_use" {
			t.Errorf("activity=%q", entries[0].Activity)
		}
		if entries[0].Summary != "[builder] Bash: make test" {
			t.Errorf("summary=%q", entries[0].Summary)
		}
	})

	t.Run("limit with multi-block entries", func(t *testing.T) {
		// An assistant message with 3 blocks should produce 3 entries.
		// With limit=2, only 2 should be returned.
		raw := `{"type":"assistant","timestamp":"` + testTS + `","message":{"content":[{"type":"text","text":"one"},{"type":"text","text":"two"},{"type":"text","text":"three"}]}}`
		data := raw + "\n"
		path := writeTempJSONL(t, data)

		entries, _, err := ParseTailEntries(path, 0, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
	})
}
