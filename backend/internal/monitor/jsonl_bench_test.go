package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/jsonl"
)

// buildJSONLLine marshals a JSONL entry to a newline-terminated byte slice.
func buildJSONLLine(t testing.TB, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

// writeJSONLFile creates a temp JSONL file with the given lines and returns its path.
func writeJSONLFile(t testing.TB, lines [][]byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := f.Write(line); err != nil {
			f.Close()
			t.Fatal(err)
		}
	}
	f.Close()
	return path
}

// makeAssistantEntry creates a realistic assistant JSONL entry with tool use and text blocks.
func makeAssistantEntry(sessionID, slug, model string, ts time.Time, inputTokens, outputTokens int) map[string]any {
	return map[string]any{
		"type":      "assistant",
		"uuid":      "uuid-" + sessionID,
		"sessionId": sessionID,
		"slug":      slug,
		"timestamp": ts.Format(time.RFC3339Nano),
		"cwd":       "/home/user/project",
		"message": map[string]any{
			"model": model,
			"role":  "assistant",
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
			"content": []map[string]any{
				{"type": "text", "text": "Here is the implementation of the requested feature."},
				{"type": "tool_use", "id": "tool-1", "name": "Write"},
			},
		},
	}
}

// makeUserEntry creates a realistic user JSONL entry.
func makeUserEntry(sessionID, slug string, ts time.Time) map[string]any {
	return map[string]any{
		"type":      "user",
		"uuid":      "uuid-user-" + sessionID,
		"sessionId": sessionID,
		"slug":      slug,
		"timestamp": ts.Format(time.RFC3339Nano),
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "Please implement this feature."},
			},
		},
	}
}

// makeProgressEntry creates a realistic subagent progress entry.
func makeProgressEntry(sessionID, slug, toolUseID, parentToolUseID string, ts time.Time) map[string]any {
	return map[string]any{
		"type":            "progress",
		"toolUseID":       toolUseID,
		"parentToolUseID": parentToolUseID,
		"sessionId":       sessionID,
		"slug":            slug,
		"timestamp":       ts.Format(time.RFC3339Nano),
		"data": map[string]any{
			"type": "agent_progress",
			"message": map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"model": "claude-sonnet-4-6-20250514",
					"role":  "assistant",
					"usage": map[string]any{
						"input_tokens":  500,
						"output_tokens": 100,
					},
					"content": []map[string]any{
						{"type": "tool_use", "id": "sub-tool-1", "name": "Read"},
					},
				},
			},
		},
	}
}

// buildRealisticSession returns JSONL lines simulating a realistic session
// with the given number of assistant/user message pairs and subagent entries.
func buildRealisticSession(t testing.TB, messagePairs, subagentEntries int) [][]byte {
	t.Helper()
	sessionID := "sess-bench-001"
	slug := "mighty-cuddling-castle"
	model := "claude-opus-4-6-20250514"
	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	var lines [][]byte
	for i := 0; i < messagePairs; i++ {
		ts := base.Add(time.Duration(i*2) * time.Second)
		lines = append(lines, buildJSONLLine(t, makeUserEntry(sessionID, slug, ts)))
		ts = ts.Add(time.Second)
		lines = append(lines, buildJSONLLine(t, makeAssistantEntry(sessionID, slug, model, ts, 1000+i*100, 200+i*10)))
	}
	for i := 0; i < subagentEntries; i++ {
		ts := base.Add(time.Duration(messagePairs*2+i) * time.Second)
		toolID := fmt.Sprintf("tool-%d", i)
		parentID := fmt.Sprintf("parent-%d", i)
		lines = append(lines, buildJSONLLine(t, makeProgressEntry(sessionID, "sub-slug", toolID, parentID, ts)))
	}
	return lines
}

func BenchmarkParseSessionJSONL_Small(b *testing.B) {
	lines := buildRealisticSession(b, 5, 2)
	path := writeJSONLFile(b, lines)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ParseSessionJSONL(path, 0, "", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSessionJSONL_Medium(b *testing.B) {
	lines := buildRealisticSession(b, 50, 10)
	path := writeJSONLFile(b, lines)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ParseSessionJSONL(path, 0, "", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSessionJSONL_Large(b *testing.B) {
	lines := buildRealisticSession(b, 200, 50)
	path := writeJSONLFile(b, lines)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ParseSessionJSONL(path, 0, "", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSessionJSONL_IncrementalOffset(b *testing.B) {
	lines := buildRealisticSession(b, 100, 20)
	path := writeJSONLFile(b, lines)

	// Parse once to get offset at ~halfway
	halfLines := lines[:len(lines)/2]
	halfPath := writeJSONLFile(b, halfLines)
	_, midOffset, err := ParseSessionJSONL(halfPath, 0, "", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ParseSessionJSONL(path, midOffset, "mighty-cuddling-castle", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkForEachEntry(b *testing.B) {
	lines := buildRealisticSession(b, 100, 20)
	path := writeJSONLFile(b, lines)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jsonl.ForEachEntry(path, 0, func(entry *jsonl.Entry, line []byte) bool {
			return true
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAssistantMessage(b *testing.B) {
	entry := makeAssistantEntry("sess-1", "slug", "claude-opus-4-6-20250514",
		time.Now(), 5000, 1000)
	msgData, err := json.Marshal(entry["message"])
	if err != nil {
		b.Fatal(err)
	}
	raw := json.RawMessage(msgData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := &ParseResult{Subagents: make(map[string]*SubagentParseResult)}
		parseAssistantMessage(raw, result)
	}
}
