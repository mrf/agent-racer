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
	if offset == 0 {
		t.Error("expected non-zero offset from first parse")
	}

	// Second parse with same mtime should skip (no new data) but return same non-zero offset.
	update2, offset2, err := src.Parse(handle, offset)
	if err != nil {
		t.Fatal(err)
	}
	if update2.HasData() {
		t.Error("expected no data on re-read with same mtime")
	}
	if offset2 != offset {
		t.Errorf("offset changed from %d to %d on re-parse", offset, offset2)
	}
}

func TestGeminiSourceParseRetrackAfterPreviousParse(t *testing.T) {
	// Regression test for re-tracking bug: if a file was parsed in a previous
	// tracking cycle, then the monitor stops tracking it, then rediscovers it,
	// Parse() with offset=0 should return a non-zero offset even when skipping
	// the parse (because mtime hasn't changed).
	dir := t.TempDir()
	path := filepath.Join(dir, "session-2026-01-30T10-00-retrack.json")

	data := `[{"role":"user","content":{"parts":[{"text":"hello"}]}},{"role":"model","content":{"parts":[{"text":"hi"}]}}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewGeminiSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "retrack",
		LogPath:   path,
		Source:    "gemini",
	}

	// First parse: file is new to the source.
	update1, offset1, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !update1.HasData() {
		t.Error("expected data from first parse")
	}
	if offset1 == 0 {
		t.Error("expected non-zero offset from first parse")
	}

	// Simulate monitor stopping tracking (source keeps lastParsed state).
	// Now monitor rediscovers the session and calls Parse() with offset=0.
	update2, offset2, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}

	// The file hasn't changed, so Parse() should skip and return no data.
	// But crucially, it must return a non-zero offset, not 0, so the monitor
	// knows the file has been processed. This prevents the session from
	// appearing stale (lastDataTime never set) and being immediately removed.
	if update2.HasData() {
		t.Error("expected no data when re-tracking unchanged file")
	}
	if offset2 == 0 {
		t.Error("REGRESSION: offset=0 returned when re-tracking, should return current mtime")
	}
	if offset2 != offset1 {
		t.Errorf("offset mismatch: first=%d, retrack=%d", offset1, offset2)
	}
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

func TestGeminiSourceParseConversationWrapper(t *testing.T) {
	// Session file as a wrapper object with "conversation" field.
	data := []byte(`{
		"conversation": [
			{"role": "user", "content": {"parts": [{"text": "hello"}]}},
			{"role": "model", "content": {"parts": [{"text": "hi"}]}, "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}}
		]
	}`)

	update := parseGeminiSession(data)

	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}
	if update.TokensIn != 10 {
		t.Errorf("TokensIn = %d, want 10", update.TokensIn)
	}
}

func TestGeminiSourceParseHistoryWrapper(t *testing.T) {
	data := []byte(`{
		"history": [
			{"role": "user", "content": {"parts": [{"text": "test"}]}},
			{"role": "model", "content": {"parts": [{"functionCall": {"name": "read_file", "args": {"path": "main.go"}}}]}}
		]
	}`)

	update := parseGeminiSession(data)

	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", update.MessageCount)
	}
	if update.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", update.ToolCalls)
	}
	if update.LastTool != "read_file" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "read_file")
	}
}

func TestGeminiSourceParseInvalidJSON(t *testing.T) {
	update := parseGeminiSession([]byte(`not json`))
	if update.HasData() {
		t.Error("expected no data from invalid JSON")
	}
}

func TestGeminiSourceParseWithModelField(t *testing.T) {
	data := []byte(`[
		{"role": "user", "content": {"parts": [{"text": "hi"}]}},
		{"role": "model", "model": "gemini-2.5-flash", "content": {"parts": [{"text": "hello"}]}}
	]`)

	update := parseGeminiSession(data)
	if update.Model != "gemini-2.5-flash" {
		t.Errorf("Model = %q, want %q", update.Model, "gemini-2.5-flash")
	}
}

func TestGeminiContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gemini-2.5-pro", 1_048_576},
		{"gemini-2.5-flash", 1_048_576},
		{"gemini-2.0-flash", 1_048_576},
		{"gemini-2.0-flash-exp", 1_048_576},
		{"gemini-3-pro-preview", 1_000_000},
		{"gemini-3-flash-preview", 1_000_000},
		{"gemini-1.5-pro-latest", 2_097_152},
		{"gemini-1.5-flash-latest", 1_048_576},
		{"some-unknown-model", 1_048_576},
	}

	for _, tt := range tests {
		got := geminiContextWindow(tt.model)
		if got != tt.want {
			t.Errorf("geminiContextWindow(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestGeminiSessionSetsMaxContextTokens(t *testing.T) {
	data := []byte(`[
		{"role": "user", "content": {"parts": [{"text": "hi"}]}},
		{"role": "model", "model": "gemini-2.5-pro", "content": {"parts": [{"text": "hello"}]}, "usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150}}
	]`)

	update := parseGeminiSession(data)
	if update.MaxContextTokens != 1_048_576 {
		t.Errorf("MaxContextTokens = %d, want 1048576", update.MaxContextTokens)
	}
}

func TestIsGeminiProcess(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"gemini binary", []string{"gemini", "--prompt", "hello"}, true},
		{"node running gemini", []string{"node", "/usr/lib/gemini/cli.js"}, true},
		{"npx running gemini", []string{"npx", "gemini", "--help"}, true},
		{"claude binary", []string{"claude", "--help"}, false},
		{"unrelated node", []string{"node", "/usr/lib/something/main.js"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGeminiProcess(tt.args)
			if got != tt.want {
				t.Errorf("isGeminiProcess(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
