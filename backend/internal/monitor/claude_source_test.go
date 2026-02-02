package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeSourceName(t *testing.T) {
	src := NewClaudeSource(10 * time.Minute)
	if src.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", src.Name(), "claude")
	}
}

func TestClaudeSourceParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"test-123","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"cache_creation_input_tokens":500,"cache_read_input_tokens":2000,"output_tokens":50}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:01.000Z"}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"toolu_1","input":{}}],"usage":{"input_tokens":200,"cache_creation_input_tokens":600,"cache_read_input_tokens":3000,"output_tokens":80}},"sessionId":"test-123","timestamp":"2026-01-30T10:00:02.000Z"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewClaudeSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "test-123",
		LogPath:   path,
		Source:    "claude",
	}

	update, offset, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offset == 0 {
		t.Error("expected non-zero offset")
	}
	if update.SessionID != "test-123" {
		t.Errorf("SessionID = %q, want %q", update.SessionID, "test-123")
	}
	if update.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", update.Model, "claude-opus-4-5-20251101")
	}
	if update.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", update.MessageCount)
	}
	if update.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", update.ToolCalls)
	}
	if update.LastTool != "Read" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "Read")
	}
	if update.Activity != "tool_use" {
		t.Errorf("Activity = %q, want %q", update.Activity, "tool_use")
	}
	expectedTokens := 200 + 600 + 3000
	if update.TokensIn != expectedTokens {
		t.Errorf("TokensIn = %d, want %d", update.TokensIn, expectedTokens)
	}

	// Second parse from same offset should yield no new data.
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
