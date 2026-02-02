package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGeminiSourceName(t *testing.T) {
	src := NewGeminiSource(10 * time.Minute)
	if src.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", src.Name(), "gemini")
	}
}

func TestGeminiSessionIDFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantID   string
	}{
		{"session-2025-09-18T02-45-3b44bc68.json", "3b44bc68"},
		{"session-2025-12-21T13-43-c27248ed.json", "c27248ed"},
	}

	for _, tt := range tests {
		got := geminiSessionIDFromFilename(tt.filename)
		if got != tt.wantID {
			t.Errorf("geminiSessionIDFromFilename(%q) = %q, want %q", tt.filename, got, tt.wantID)
		}
	}
}

func TestHashProjectPath(t *testing.T) {
	hash := hashProjectPath("/home/mrf/Projects/agent-racer")
	// SHA-256 produces a 64-char hex string.
	if len(hash) != 64 {
		t.Errorf("hashProjectPath returned %d chars, want 64", len(hash))
	}

	// Same input should produce same hash.
	hash2 := hashProjectPath("/home/mrf/Projects/agent-racer")
	if hash != hash2 {
		t.Error("hashProjectPath not deterministic")
	}

	// Different input should produce different hash.
	hash3 := hashProjectPath("/home/mrf/Projects/other")
	if hash == hash3 {
		t.Error("different paths produced same hash")
	}
}

func TestParseGeminiSessionArray(t *testing.T) {
	// Session file as a JSON array of messages.
	data := []byte(`[
		{
			"role": "user",
			"content": {"parts": [{"text": "hello"}]}
		},
		{
			"role": "model",
			"model": "gemini-2.5-pro",
			"content": {"parts": [{"text": "Hi there!"}]},
			"usageMetadata": {
				"promptTokenCount": 100,
				"candidatesTokenCount": 50,
				"totalTokenCount": 150
			}
		},
		{
			"role": "user",
			"content": {"parts": [{"text": "run ls"}]}
		},
		{
			"role": "model",
			"content": {
				"parts": [
					{"functionCall": {"name": "run_shell_command", "args": {"command": "ls"}}},
					{"text": "Running ls..."}
				]
			},
			"usageMetadata": {
				"promptTokenCount": 200,
				"candidatesTokenCount": 80,
				"totalTokenCount": 280
			}
		}
	]`)

	update := parseGeminiSession(data)

	if update.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want %q", update.Model, "gemini-2.5-pro")
	}
	if update.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", update.MessageCount)
	}
	if update.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", update.ToolCalls)
	}
	if update.LastTool != "run_shell_command" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "run_shell_command")
	}
	if update.TokensIn != 200 {
		t.Errorf("TokensIn = %d, want 200", update.TokensIn)
	}
	if update.TokensOut != 80 {
		t.Errorf("TokensOut = %d, want 80", update.TokensOut)
	}
}

func TestParseGeminiSessionWrapper(t *testing.T) {
	// Session file as a wrapper object with "messages" field.
	data := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": {"parts": [{"text": "hello"}]}
			},
			{
				"role": "model",
				"content": {"parts": [{"thought": "thinking about this..."}, {"text": "Here's my answer"}]},
				"usageMetadata": {"promptTokenCount": 50, "candidatesTokenCount": 30, "totalTokenCount": 80}
			}
		]
	}`)

	update := parseGeminiSession(data)

	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}
	// Last activity should be "thinking" because there's a thought part.
	if update.Activity != "thinking" {
		t.Errorf("Activity = %q, want %q", update.Activity, "thinking")
	}
	if update.TokensIn != 50 {
		t.Errorf("TokensIn = %d, want 50", update.TokensIn)
	}
}

func TestGeminiSourceParseMtimeSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-2026-01-30T10-00-abc123.json")

	data := `[{"role":"user","content":{"parts":[{"text":"hello"}]}},{"role":"model","content":{"parts":[{"text":"hi"}]},"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewGeminiSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "abc123",
		LogPath:   path,
		Source:    "gemini",
	}

	// First parse should return data.
	update, offset, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !update.HasData() {
		t.Error("expected data from first parse")
	}
	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}

	// Second parse with same mtime should skip (no new data).
	update2, offset2, err := src.Parse(handle, offset)
	if err != nil {
		t.Fatal(err)
	}
	if update2.HasData() {
		t.Error("expected no data on re-read with same mtime")
	}
	_ = offset2
}

func TestGeminiSourceParseEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-empty.json")

	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewGeminiSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "empty",
		LogPath:   path,
		Source:    "gemini",
	}

	update, _, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update.MessageCount != 0 {
		t.Errorf("expected 0 messages from empty session, got %d", update.MessageCount)
	}
}

func TestIsGeminiProcess(t *testing.T) {
	tests := []struct {
		cmdline string
		want    bool
	}{
		{"gemini\x00--prompt\x00hello", true},
		{"node\x00/usr/lib/gemini/cli.js", true},
		{"npx\x00gemini\x00--help", true},
		{"claude\x00--help", false},
		{"node\x00/usr/lib/something/main.js", false},
	}

	for _, tt := range tests {
		got := isGeminiProcess(tt.cmdline)
		if got != tt.want {
			t.Errorf("isGeminiProcess(%q) = %v, want %v", tt.cmdline, got, tt.want)
		}
	}
}
