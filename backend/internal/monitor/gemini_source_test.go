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
		// Standard format: session-{date}T{time}-{short_hex}.json
		{"session-2025-09-18T02-45-3b44bc68.json", "3b44bc68"},
		{"session-2025-12-21T13-43-c27248ed.json", "c27248ed"},

		// Edge cases: filenames without .json suffix
		{"session-2025-09-18T02-45-abc123", "abc123"},

		// Edge cases: filename with no dashes (single part)
		{"session.json", "session"},
		{"somefile", "somefile"},

		// Edge cases: filename with only dashes, no content after last dash
		{"session-", ""},
		{"---", ""},

		// Edge cases: filename with extra dots in the name
		{"session-2025.09.18T02-45-xyz789.json", "xyz789"},
		{"session.test-2025-01-01T00-00-abc.json", "abc"},

		// Edge cases: filename with path separators (should still work)
		// The function gets just the filename, not the full path
		{"def456", "def456"},

		// Edge cases: very short filenames
		{"a", "a"},
		{"", ""},

		// Edge cases: filename with multiple consecutive dashes
		{"session--2025-01-01T00-00--id.json", "id"},
		{"----id----", ""},

		// Edge cases: filename with only one dash
		{"session-id", "id"},
		{"x-y", "y"},

		// Realistic malformed filenames that might appear in the directory
		{"readme.txt", "readme.txt"},
		{".hidden", ".hidden"},
		{"session-corrupted-partial", "partial"},
		{"2025-01-01T00-00-00.json", "00"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := geminiSessionIDFromFilename(tt.filename)
			if got != tt.wantID {
				t.Errorf("geminiSessionIDFromFilename(%q) = %q, want %q", tt.filename, got, tt.wantID)
			}
		})
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

func TestGeminiSessionNoLongerSetsMaxContextTokens(t *testing.T) {
	// MaxContextTokens resolution moved to config prefix matching.
	// parseGeminiSession should no longer set it — the monitor falls
	// back to config.MaxContextTokens(model).
	data := []byte(`[
		{"role": "user", "content": {"parts": [{"text": "hi"}]}},
		{"role": "model", "model": "gemini-2.5-pro", "content": {"parts": [{"text": "hello"}]}, "usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150}}
	]`)

	update := parseGeminiSession(data)
	if update.MaxContextTokens != 0 {
		t.Errorf("MaxContextTokens = %d, want 0 (deferred to config)", update.MaxContextTokens)
	}
}

func TestParseGeminiSessionCLIFormat(t *testing.T) {
	// Real Gemini CLI session format: object with "messages" array,
	// type "user"/"gemini"/"info", content as plain string, toolCalls
	// and tokens at message level.
	data := []byte(`{
		"sessionId": "b5ebc5b2-594a-475d-99c9-3f8d336e3e9b",
		"messages": [
			{
				"type": "user",
				"content": "how do I check my usage?"
			},
			{
				"type": "gemini",
				"content": "",
				"toolCalls": [
					{"name": "delegate_to_agent"},
					{"name": "read_file"}
				],
				"thoughts": [
					{"subject": "Analyzing the question"}
				],
				"model": "gemini-2.5-pro",
				"tokens": {
					"input": 7831,
					"output": 25,
					"cached": 0,
					"thoughts": 100,
					"tool": 0,
					"total": 7956
				}
			},
			{
				"type": "user",
				"content": "show me the file"
			},
			{
				"type": "gemini",
				"content": "Here is the file content.",
				"model": "gemini-2.5-pro",
				"tokens": {
					"input": 15000,
					"output": 200,
					"total": 15200
				}
			}
		]
	}`)

	update := parseGeminiSession(data)

	if update.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want %q", update.Model, "gemini-2.5-pro")
	}
	if update.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", update.MessageCount)
	}
	if update.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", update.ToolCalls)
	}
	if update.LastTool != "read_file" {
		t.Errorf("LastTool = %q, want %q", update.LastTool, "read_file")
	}
	// Should use the last gemini message's tokens.input.
	if update.TokensIn != 15000 {
		t.Errorf("TokensIn = %d, want 15000", update.TokensIn)
	}
	if update.TokensOut != 200 {
		t.Errorf("TokensOut = %d, want 200", update.TokensOut)
	}
	if update.MaxContextTokens != 0 {
		t.Errorf("MaxContextTokens = %d, want 0 (deferred to config)", update.MaxContextTokens)
	}
}

func TestParseGeminiSessionCLIFormatInfoMessages(t *testing.T) {
	// "info" type messages should be skipped (not counted).
	data := []byte(`{
		"messages": [
			{"type": "info", "content": "Authentication succeeded"},
			{"type": "user", "content": "hello"},
			{"type": "gemini", "content": "hi", "model": "gemini-2.5-flash", "tokens": {"input": 500, "output": 10, "total": 510}}
		]
	}`)

	update := parseGeminiSession(data)

	if update.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2 (info messages should not count)", update.MessageCount)
	}
	if update.TokensIn != 500 {
		t.Errorf("TokensIn = %d, want 500", update.TokensIn)
	}
}

func TestGeminiSourceParseDeltaConversion(t *testing.T) {
	// Verify that Parse returns deltas, not absolute counts, when a
	// session file is re-parsed after modification.
	dir := t.TempDir()
	path := filepath.Join(dir, "session-2026-01-30T10-00-delta.json")

	// Initial file with 2 messages and 1 tool call.
	data1 := `{"messages":[{"type":"user","content":"hello"},{"type":"gemini","content":"","toolCalls":[{"name":"read_file"}],"model":"gemini-2.5-pro","tokens":{"input":500,"output":10,"total":510}}]}`
	if err := os.WriteFile(path, []byte(data1), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewGeminiSource(10 * time.Minute)
	handle := SessionHandle{
		SessionID: "delta",
		LogPath:   path,
		Source:    "gemini",
	}

	// First parse: should return full absolute counts as deltas (prev=0).
	update1, offset1, err := src.Parse(handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if update1.MessageCount != 2 {
		t.Errorf("first parse MessageCount = %d, want 2", update1.MessageCount)
	}
	if update1.ToolCalls != 1 {
		t.Errorf("first parse ToolCalls = %d, want 1", update1.ToolCalls)
	}
	if update1.TokensIn != 500 {
		t.Errorf("first parse TokensIn = %d, want 500", update1.TokensIn)
	}

	// Update file with 2 more messages and 1 more tool call (4 total, 2 tools).
	// We need a different mtime, so sleep briefly.
	time.Sleep(10 * time.Millisecond)
	data2 := `{"messages":[{"type":"user","content":"hello"},{"type":"gemini","content":"","toolCalls":[{"name":"read_file"}],"model":"gemini-2.5-pro","tokens":{"input":500,"output":10,"total":510}},{"type":"user","content":"run ls"},{"type":"gemini","content":"done","toolCalls":[{"name":"run_command"}],"tokens":{"input":8000,"output":50,"total":8050}}]}`
	if err := os.WriteFile(path, []byte(data2), 0644); err != nil {
		t.Fatal(err)
	}

	update2, _, err := src.Parse(handle, offset1)
	if err != nil {
		t.Fatal(err)
	}
	// Should return delta: 4-2 = 2 new messages, 2-1 = 1 new tool call.
	if update2.MessageCount != 2 {
		t.Errorf("second parse MessageCount = %d, want 2 (delta)", update2.MessageCount)
	}
	if update2.ToolCalls != 1 {
		t.Errorf("second parse ToolCalls = %d, want 1 (delta)", update2.ToolCalls)
	}
	// TokensIn is a snapshot (last model message), not a delta.
	if update2.TokensIn != 8000 {
		t.Errorf("second parse TokensIn = %d, want 8000", update2.TokensIn)
	}
}

func TestGeminiContentUnmarshalString(t *testing.T) {
	// Content as a plain string (Gemini CLI format).
	data := []byte(`"hello world"`)
	var c geminiContent
	if err := c.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	// Plain strings result in empty Parts (we don't need text content).
	if len(c.Parts) != 0 {
		t.Errorf("expected 0 parts for string content, got %d", len(c.Parts))
	}
}

func TestGeminiContentUnmarshalObject(t *testing.T) {
	// Content as an object with parts (Gemini API format).
	data := []byte(`{"parts": [{"text": "hello"}]}`)
	var c geminiContent
	if err := c.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if len(c.Parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(c.Parts))
	}
}

func TestGeminiSourceDiscoverPrunesStaleEntries(t *testing.T) {
	// Create a temporary ~/.gemini/tmp structure with one active session.
	tmpDir := t.TempDir()
	activeHash := hashProjectPath("/home/user/active-project")
	chatsDir := filepath.Join(tmpDir, activeHash, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(chatsDir, "session-2026-01-30T10-00-aaa111.json")
	if err := os.WriteFile(sessionFile, []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewGeminiSource(10 * time.Minute)

	// Seed all three maps with stale entries that won't be discovered.
	staleHash := hashProjectPath("/home/user/gone-project")
	src.hashToPath[staleHash] = "/home/user/gone-project"
	src.hashToPath[activeHash] = "/home/user/active-project"

	staleLogPath := "/old/path/session-old.json"
	src.lastParsed[staleLogPath] = time.Now()
	src.lastParsed[sessionFile] = time.Now()

	src.prevCounts[staleLogPath] = geminiAbsoluteCounts{Messages: 5, ToolCalls: 2}
	src.prevCounts[sessionFile] = geminiAbsoluteCounts{Messages: 3, ToolCalls: 1}

	// Use discoverFromDir directly to bypass geminiBaseDir and process scanning.
	handles := src.discoverFromDir(tmpDir)

	if len(handles) != 1 {
		t.Fatalf("expected 1 handle, got %d", len(handles))
	}

	// Stale hash should be pruned, active hash should remain.
	if _, ok := src.hashToPath[staleHash]; ok {
		t.Error("stale hash entry not pruned from hashToPath")
	}
	if _, ok := src.hashToPath[activeHash]; !ok {
		t.Error("active hash entry was incorrectly pruned from hashToPath")
	}

	// Stale log path should be pruned, active session file should remain.
	if _, ok := src.lastParsed[staleLogPath]; ok {
		t.Error("stale entry not pruned from lastParsed")
	}
	if _, ok := src.lastParsed[sessionFile]; !ok {
		t.Error("active entry was incorrectly pruned from lastParsed")
	}

	// Same pruning applies to prevCounts.
	if _, ok := src.prevCounts[staleLogPath]; ok {
		t.Error("stale entry not pruned from prevCounts")
	}
	if _, ok := src.prevCounts[sessionFile]; !ok {
		t.Error("active entry was incorrectly pruned from prevCounts")
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
