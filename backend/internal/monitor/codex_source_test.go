package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexSourceName(t *testing.T) {
	src := NewCodexSource(10 * time.Minute)
	if src.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", src.Name(), "codex")
	}
}

func TestCodexSessionIDFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantID   string
	}{
		{
			"rollout-1738000000-0199e96c-7d0c-7403-bf30-395693cd1788.jsonl",
			"0199e96c-7d0c-7403-bf30-395693cd1788",
		},
		{
			"rollout-0199a213-81c0-7800-8aa1-bbab2a035a53.jsonl",
			"0199a213-81c0-7800-8aa1-bbab2a035a53",
		},
	}

	for _, tt := range tests {
		got := codexSessionIDFromFilename(tt.filename)
		if got != tt.wantID {
			t.Errorf("codexSessionIDFromFilename(%q) = %q, want %q", tt.filename, got, tt.wantID)
		}
	}
}

func TestCodexSourceParseNewEnvelope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-1738000000-01234567-abcd-ef01-2345-67890abcdef0.jsonl")

	// New RolloutLine envelope format.
	content := `{"type":"session_meta","payload":{"session_id":"01234567-abcd-ef01-2345-67890abcdef0","model":"o4-mini","timestamp":"2026-01-30T10:00:00.000Z","source":"cli"}}
{"type":"env_context","payload":{"cwd":"/home/user/project","approval_policy":"auto"}}
{"type":"event_msg","payload":{"type":"user_message","payload":{"text":"fix the bug"}}}
{"type":"event_msg","payload":{"type":"agent_message","payload":{"text":"I'll fix that"}}}
{"type":"response_item","payload":{"type":"command_execution","command":"grep -r TODO src/"}}
{"type":"event_msg","payload":{"type":"token_count","payload":{"input_tokens":5000,"cached_input_tokens":3000,"output_tokens":200,"total_tokens":5200}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "01234567-abcd-ef01-2345-67890abcdef0",
		LogPath:   path,
		Source:    "codex",
	}

	update, offset, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offset == 0 {
		t.Error("expected non-zero offset")
	}
	if update.SessionID != "01234567-abcd-ef01-2345-67890abcdef0" {
		t.Errorf("SessionID = %q, want UUID", update.SessionID)
	}
	if update.Model != "o4-mini" {
		t.Errorf("Model = %q, want %q", update.Model, "o4-mini")
	}
	if update.WorkingDir != "/home/user/project" {
		t.Errorf("WorkingDir = %q, want %q", update.WorkingDir, "/home/user/project")
	}
	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}
	if update.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", update.ToolCalls)
	}
	if update.LastTool != "Bash" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "Bash")
	}
	if update.TokensIn != 5000 {
		t.Errorf("TokensIn = %d, want 5000", update.TokensIn)
	}
	if update.TokensOut != 200 {
		t.Errorf("TokensOut = %d, want 200", update.TokensOut)
	}

	// Incremental parse should yield no new data.
	update2, offset2, err := src.Parse(handle, offset)
	if err != nil {
		t.Fatal(err)
	}
	if offset2 != offset {
		t.Errorf("offset changed on re-read: %d vs %d", offset2, offset)
	}
	if update2.HasData() {
		t.Error("expected no new data on re-read")
	}
}

func TestCodexSourceParseOldFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-test.jsonl")

	// Old bare format: first line is SessionMeta, rest are ResponseItems.
	content := `{"session_id":"old-session-123","model":"gpt-5-codex","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"message","text":"I'll help you with that"}
{"type":"command_execution","command":"ls -la"}
{"type":"file_change","path":"src/main.go"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "old-session-123",
		LogPath:   path,
		Source:    "codex",
	}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.SessionID != "old-session-123" {
		t.Errorf("SessionID = %q, want %q", update.SessionID, "old-session-123")
	}
	if update.Model != "gpt-5-codex" {
		t.Errorf("Model = %q, want %q", update.Model, "gpt-5-codex")
	}
	// 1 message + 1 command_execution + 1 file_change = 1 message, 2 tool calls
	if update.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", update.MessageCount)
	}
	if update.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", update.ToolCalls)
	}
}

func TestCodexSourceParseAllToolTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-tools.jsonl")

	// Exercise all bare-format tool types and response_item envelope variants.
	content := `{"session_id":"tools-test","model":"o3","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"reasoning","text":"Let me think..."}
{"type":"web_search","query":"golang testing"}
{"type":"mcp_tool_call","tool_name":"database_query","name":"db"}
{"type":"command_execution","command":"npm test"}
{"type":"file_change","path":"src/index.ts"}
{"type":"message","text":"Done"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "tools-test", LogPath: path, Source: "codex"}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.ToolCalls != 4 {
		t.Errorf("ToolCalls = %d, want 4 (web_search + mcp + command + file_change)", update.ToolCalls)
	}
	if update.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", update.MessageCount)
	}
}

func TestCodexSourceParseResponseItemEnvelope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-ri.jsonl")

	// Response items in new envelope format covering all variants.
	content := `{"type":"session_meta","payload":{"session_id":"ri-test","model":"o3"}}
{"type":"response_item","payload":{"type":"message","text":"hello"}}
{"type":"response_item","payload":{"type":"reasoning","text":"thinking"}}
{"type":"response_item","payload":{"type":"web_search","query":"test"}}
{"type":"response_item","payload":{"type":"file_change","path":"a.go"}}
{"type":"response_item","payload":{"type":"mcp_tool_call","tool_name":"slack_send","name":"slack"}}
{"type":"event_msg","payload":{"type":"session_configured","payload":{"model":"o4-mini"}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "ri-test", LogPath: path, Source: "codex"}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.Model != "o4-mini" {
		t.Errorf("Model = %q, want %q (session_configured should override)", update.Model, "o4-mini")
	}
	if update.ToolCalls != 3 {
		t.Errorf("ToolCalls = %d, want 3 (web_search + file_change + mcp)", update.ToolCalls)
	}
	if update.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", update.MessageCount)
	}
	if update.LastTool != "slack_send" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "slack_send")
	}
}

func TestCodexSourceParseContextWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-ctx.jsonl")

	content := `{"type":"session_meta","payload":{"session_id":"ctx-test","model":"gpt-5-codex"}}
{"type":"event_msg","payload":{"type":"turn_started","payload":{"model_context_window":272000}}}
{"type":"event_msg","payload":{"type":"token_count","payload":{"input_tokens":8000,"output_tokens":500,"model_context_window":272000}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "ctx-test", LogPath: path, Source: "codex"}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.MaxContextTokens != 272000 {
		t.Errorf("MaxContextTokens = %d, want 272000", update.MaxContextTokens)
	}
	if update.TokensIn != 8000 {
		t.Errorf("TokensIn = %d, want 8000", update.TokensIn)
	}
}

func TestCodexSourceParseContextWindowFromTokenCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-tc.jsonl")

	// Only token_count event (no turn_started), should still get context window.
	content := `{"session_id":"tc-test","model":"o3","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"token_count","input_tokens":3000,"output_tokens":100,"model_context_window":200000}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "tc-test", LogPath: path, Source: "codex"}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.MaxContextTokens != 200000 {
		t.Errorf("MaxContextTokens = %d, want 200000", update.MaxContextTokens)
	}
}

func TestCodexSourceParseNestedTokenFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-nested.jsonl")

	// Real Codex CLI format: token data nested under info.total_token_usage.
	content := `{"timestamp":"2026-02-03T00:59:08.310Z","type":"session_meta","payload":{"id":"nested-test","model":"gpt-5.2-codex","timestamp":"2026-02-03T00:59:08.300Z","source":"cli"}}
{"timestamp":"2026-02-03T00:59:09Z","type":"turn_context","payload":{"cwd":"/home/user/myproject","model":"gpt-5.2-codex"}}
{"timestamp":"2026-02-03T00:59:10Z","type":"event_msg","payload":{"type":"user_message","payload":{"text":"fix the bug"}}}
{"timestamp":"2026-02-03T00:59:11Z","type":"event_msg","payload":{"type":"agent_reasoning","payload":{}}}
{"timestamp":"2026-02-03T00:59:12Z","type":"event_msg","payload":{"type":"agent_message","payload":{"text":"on it"}}}
{"timestamp":"2026-02-03T00:59:13Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\"}"}}
{"timestamp":"2026-02-03T00:59:14Z","type":"response_item","payload":{"type":"function_call_output","output":"file.go"}}
{"timestamp":"2026-02-03T00:59:15Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":45000,"cached_input_tokens":30000,"output_tokens":1200,"reasoning_output_tokens":500,"total_tokens":46200},"model_context_window":258400},"rate_limits":{"primary":null}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "nested-test", LogPath: path, Source: "codex"}

	update, offset, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offset == 0 {
		t.Error("expected non-zero offset")
	}
	if update.TokensIn != 45000 {
		t.Errorf("TokensIn = %d, want 45000", update.TokensIn)
	}
	if update.TokensOut != 1200 {
		t.Errorf("TokensOut = %d, want 1200", update.TokensOut)
	}
	if update.MaxContextTokens != 258400 {
		t.Errorf("MaxContextTokens = %d, want 258400", update.MaxContextTokens)
	}
	if update.WorkingDir != "/home/user/myproject" {
		t.Errorf("WorkingDir = %q, want %q", update.WorkingDir, "/home/user/myproject")
	}
	if update.Model != "gpt-5.2-codex" {
		t.Errorf("Model = %q, want %q", update.Model, "gpt-5.2-codex")
	}
	// 1 user_message + 1 agent_message = 2 messages
	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}
	// 1 function_call (function_call_output is not a tool call)
	if update.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", update.ToolCalls)
	}
	if update.LastTool != "shell_command" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "shell_command")
	}
}

func TestCodexSourceParseNullInfoTokenCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-null.jsonl")

	// First token_count event can have info:null (before any API call completes).
	content := `{"type":"session_meta","payload":{"id":"null-info","model":"gpt-5.2-codex"}}
{"type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"primary":null}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":8000,"output_tokens":200},"model_context_window":258400},"rate_limits":{"primary":null}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "null-info", LogPath: path, Source: "codex"}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.TokensIn != 8000 {
		t.Errorf("TokensIn = %d, want 8000 (should use second token_count)", update.TokensIn)
	}
	if update.MaxContextTokens != 258400 {
		t.Errorf("MaxContextTokens = %d, want 258400", update.MaxContextTokens)
	}
}

func TestCodexSessionIDFromFilenameFallback(t *testing.T) {
	// Short filename that doesn't contain a full UUID.
	got := codexSessionIDFromFilename("rollout-short.jsonl")
	if got != "short" {
		t.Errorf("codexSessionIDFromFilename(short) = %q, want %q", got, "short")
	}
}

func TestCodexSourceDiscoverNoDir(t *testing.T) {
	// When CODEX_HOME points to a non-existent directory, Discover returns nil.
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "nonexistent"))
	src := NewCodexSource(10 * time.Minute)
	handles, err := src.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(handles) != 0 {
		t.Errorf("expected no handles, got %d", len(handles))
	}
}

func TestCodexSourceDiscoverFindsFiles(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CODEX_HOME", base)

	// Create session directory structure.
	sessDir := filepath.Join(base, "sessions", "2026", "01", "30")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollout := filepath.Join(sessDir, "rollout-1738000000-01234567-abcd-ef01-2345-67890abcdef0.jsonl")
	if err := os.WriteFile(rollout, []byte(`{"session_id":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handles, err := src.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected 1 handle, got %d", len(handles))
	}
	if handles[0].Source != "codex" {
		t.Errorf("Source = %q, want %q", handles[0].Source, "codex")
	}
	if handles[0].LogPath != rollout {
		t.Errorf("LogPath = %q, want %q", handles[0].LogPath, rollout)
	}
}

func TestCodexSourceParseRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-huge.jsonl")

	// Create a sparse file exceeding maxFileSize via Truncate rather than
	// allocating 500MB in memory.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"type":"session_meta","payload":{"session_id":"huge-test","model":"o3"}}` + "\n"
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Truncate(maxFileSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "huge-test", LogPath: path, Source: "codex"}

	_, _, err = src.Parse(handle, 0)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds max") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCodexSourceParseSkipsOversizedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-bigline.jsonl")

	// Build a line whose JSON payload exceeds maxLineLength.
	// We embed a huge string in a valid JSON envelope so the line ends with \n.
	bigValue := strings.Repeat("x", maxLineLength+10)
	oversizedLine := fmt.Sprintf(`{"type":"response_item","payload":{"type":"message","text":%q}}`+"\n", bigValue)

	normalLine := `{"type":"session_meta","payload":{"session_id":"bigline-test","model":"o3"}}` + "\n"
	content := normalLine + oversizedLine + `{"type":"env_context","payload":{"cwd":"/tmp/proj"}}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewCodexSource(10 * time.Minute)
	handle := SessionHandle{SessionID: "bigline-test", LogPath: path, Source: "codex"}

	update, offset, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if offset == 0 {
		t.Error("expected non-zero offset")
	}
	// The oversized line should be skipped; the normal lines should still parse.
	if update.WorkingDir != "/tmp/proj" {
		t.Errorf("WorkingDir = %q, want %q (normal lines after oversized line should parse)", update.WorkingDir, "/tmp/proj")
	}
}
