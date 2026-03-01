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

func TestReadFirstTimestamp(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid timestamp on first line", func(t *testing.T) {
		path := filepath.Join(dir, "valid.jsonl")
		content := `{"type":"user","sessionId":"abc","timestamp":"2026-01-30T10:00:00.000Z"}
{"type":"assistant","sessionId":"abc","timestamp":"2026-01-30T10:00:01.000Z"}
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, ok := readFirstTimestamp(path)
		if !ok {
			t.Fatal("readFirstTimestamp returned false, want true")
		}
		want := time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("StartedAt = %v, want %v", got, want)
		}
	})

	t.Run("first line timestamp used even when file was modified later", func(t *testing.T) {
		path := filepath.Join(dir, "old-session.jsonl")
		// First line has an old timestamp; file modtime would be recent.
		content := `{"type":"user","sessionId":"xyz","timestamp":"2020-06-15T08:30:00.000Z"}
{"type":"assistant","sessionId":"xyz","timestamp":"2020-06-15T08:30:01.000Z"}
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		got, ok := readFirstTimestamp(path)
		if !ok {
			t.Fatal("readFirstTimestamp returned false, want true")
		}
		want := time.Date(2020, 6, 15, 8, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("StartedAt = %v, want %v", got, want)
		}
		// Sanity check: modtime is definitely more recent than the parsed timestamp.
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().After(want) {
			t.Errorf("expected modtime %v to be after first-line timestamp %v", info.ModTime(), want)
		}
	})

	t.Run("empty file returns false", func(t *testing.T) {
		path := filepath.Join(dir, "empty.jsonl")
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
		_, ok := readFirstTimestamp(path)
		if ok {
			t.Error("expected false for empty file")
		}
	})

	t.Run("file with no timestamp field returns false", func(t *testing.T) {
		path := filepath.Join(dir, "no-ts.jsonl")
		if err := os.WriteFile(path, []byte(`{"type":"user","sessionId":"abc"}`+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, ok := readFirstTimestamp(path)
		if ok {
			t.Error("expected false when timestamp field is absent")
		}
	})

	t.Run("non-existent file returns false", func(t *testing.T) {
		_, ok := readFirstTimestamp(filepath.Join(dir, "does-not-exist.jsonl"))
		if ok {
			t.Error("expected false for non-existent file")
		}
	})
}
