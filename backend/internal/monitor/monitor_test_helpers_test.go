package monitor

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// defaultTestConfig returns a config suitable for testing, loaded from a
// nonexistent path so all values are defaults.
func defaultTestConfig() *config.Config {
	cfg, _, _ := config.LoadOrDefault("/tmp/nonexistent-agent-racer-test-config.yaml")
	return cfg
}

// testSource is a minimal Source implementation that returns nothing.
// Useful when a test only needs a valid Monitor instance but never polls data.
type testSource struct{}

func (s *testSource) Name() string { return "test" }

func (s *testSource) Discover(_ context.Context) ([]awsource.SessionHandle, error) {
	return nil, nil
}

func (s *testSource) Parse(_ context.Context, _ awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	return awsource.SourceUpdate{}, cursor, nil
}

// newPollTestMonitor creates a Monitor wired to the given source and config,
// returning the monitor, store, and broadcaster. Process and tmux enrichments
// are disabled. The caller must stop the broadcaster when done.
func newPollTestMonitor(src awsource.Source, cfg *config.Config) (*Monitor, *session.Store, *ws.Broadcaster) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 50*time.Millisecond, 10*time.Second, 0)
	m := NewMonitor(cfg, store, broadcaster, []awsource.Source{src})
	m.discoverProcessActivity = nil
	m.newTmuxResolver = nil
	return m, store, broadcaster
}

// --- JSONL integration test helpers ---

// newTestHandle creates a SessionHandle for integration tests.
func newTestHandle(id, path, workDir string, started time.Time) awsource.SessionHandle {
	return awsource.SessionHandle{
		ID:         id,
		Path:       path,
		WorkingDir: workDir,
		StartedAt:  started,
		Source:     "claude",
	}
}

// jsonlLine returns a single JSONL line representing a conversation message.
// For assistant messages, model and tool are encoded inside a nested "message"
// object matching the Claude JSONL schema that ParseSessionJSONL expects.
func jsonlLine(role, sessionID, timestamp, model, tool, cwd string) string {
	if role == "assistant" {
		// Build content blocks.
		var contentParts []string
		if tool != "" {
			contentParts = append(contentParts, fmt.Sprintf(`{"type":"tool_use","name":"%s","id":"t1"}`, tool))
		} else {
			contentParts = append(contentParts, `{"type":"text","text":"thinking"}`)
		}
		content := "[" + contentParts[0]
		for i := 1; i < len(contentParts); i++ {
			content += "," + contentParts[i]
		}
		content += "]"

		msg := fmt.Sprintf(`{"model":"%s","role":"assistant","content":%s}`, model, content)
		return fmt.Sprintf("{\"type\":\"assistant\",\"sessionId\":\"%s\",\"timestamp\":\"%s\",\"cwd\":\"%s\",\"message\":%s}\n",
			sessionID, timestamp, cwd, msg)
	}

	// User message — simple format, no nested message object needed for counting.
	return fmt.Sprintf("{\"type\":\"user\",\"sessionId\":\"%s\",\"timestamp\":\"%s\",\"cwd\":\"%s\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"hello\"}]}}\n",
		sessionID, timestamp, cwd)
}

// writeJSONL creates a JSONL file at path with the given content.
func writeJSONL(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// appendJSONL appends content to an existing JSONL file.
func appendJSONL(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// parseJSONLHandle parses a JSONL file referenced by a SessionHandle, using
// the cursor as the byte offset. This mirrors the code path production
// sources use.
func parseJSONLHandle(handle awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	var offset int64
	if cursor != "" {
		if _, err := fmt.Sscanf(string(cursor), "%d", &offset); err != nil {
			offset = 0
		}
	}

	result, newOffset, err := ParseSessionJSONL(handle.Path, offset, "", nil)
	if err != nil {
		return awsource.SourceUpdate{}, cursor, err
	}
	if newOffset == offset {
		// No new data.
		return awsource.SourceUpdate{}, cursor, nil
	}

	update := awsource.SourceUpdate{
		SessionID:         handle.ID,
		Model:             result.Model,
		WorkingDir:        handle.WorkingDir,
		MessageCountDelta: result.MessageCount,
		CurrentTool:       result.LastTool,
		LastActivityAt:    result.LastTime,
	}
	if result.ToolCalls > 0 {
		update.ToolCallCountDelta = result.ToolCalls
	}

	return update, awsource.Cursor(fmt.Sprintf("%d", newOffset)), nil
}
