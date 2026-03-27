package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// maxTailFileSize is the maximum JSONL file size the tail parser will read (500 MB).
const maxTailFileSize = 500 * 1024 * 1024

// maxTailLineLength is the maximum length of a single JSONL line (1 MB).
const maxTailLineLength = 1024 * 1024

// tailJSONLEntry is the top-level structure of a Claude JSONL line.
type tailJSONLEntry struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// tailMessageContent is the message object inside assistant/user entries.
type tailMessageContent struct {
	Content json.RawMessage `json:"content"`
}

// tailProgressEntry is a type:"progress" JSONL line.
type tailProgressEntry struct {
	ToolUseID string          `json:"toolUseID"`
	Slug      string          `json:"slug"`
	Data      json.RawMessage `json:"data"`
}

// tailProgressData wraps nested data.message in a progress entry.
type tailProgressData struct {
	Message struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	} `json:"message"`
}

// TailEntry is a single display-ready entry for the tail view.
type TailEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`     // "assistant", "user", "progress", "system"
	Activity  string    `json:"activity"` // "thinking", "tool_use", "tool_result", "text", "subagent", "compact", etc.
	Summary   string    `json:"summary"`  // one-line human-readable
	Detail    string    `json:"detail,omitempty"` // optional longer content
}

// TailResponse is the HTTP response for the tail endpoint.
type TailResponse struct {
	Entries []TailEntry `json:"entries"`
	Offset  int64       `json:"offset"`
}

// tailContentBlock extends contentBlock with fields needed for display.
type tailContentBlock struct {
	Type      string          `json:"type"`
	Name      string          `json:"name,omitempty"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // tool_result content
}

// ParseTailEntries reads a JSONL file from offset and returns display entries.
// limit caps the number of entries returned (0 = no limit).
func ParseTailEntries(path string, offset int64, limit int) ([]TailEntry, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() > maxTailFileSize {
		return nil, offset, fmt.Errorf("file size %d exceeds max %d", info.Size(), maxTailFileSize)
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, offset, err
		}
	}

	var entries []TailEntry
	reader := bufio.NewReader(f)
	parsedOffset := offset

	for {
		line, err := reader.ReadBytes('\n')

		if err != nil && err != io.EOF {
			return entries, parsedOffset, err
		}

		if len(line) == 0 {
			break
		}

		// Only process complete lines.
		if line[len(line)-1] != '\n' {
			if err == io.EOF {
				break
			}
			continue
		}

		if len(line) > maxTailLineLength {
			parsedOffset += int64(len(line))
			if err == io.EOF {
				break
			}
			continue
		}

		lineData := line[:len(line)-1]

		var entry tailJSONLEntry
		if jsonErr := json.Unmarshal(lineData, &entry); jsonErr != nil {
			parsedOffset += int64(len(line))
			if err == io.EOF {
				break
			}
			continue
		}

		parsedOffset += int64(len(line))

		var ts time.Time
		if entry.Timestamp != "" {
			if t, parseErr := time.Parse(time.RFC3339Nano, entry.Timestamp); parseErr == nil {
				ts = t
			}
		}

		switch entry.Type {
		case "assistant":
			entries = append(entries, parseTailAssistant(entry.Message, ts)...)
		case "user":
			entries = append(entries, parseTailUser(entry.Message, ts)...)
		case "progress":
			if e := parseTailProgress(lineData, ts); e != nil {
				entries = append(entries, *e)
			}
		case "system":
			entries = append(entries, parseTailSystem(entry, ts))
		}

		if limit > 0 && len(entries) >= limit {
			entries = entries[:limit]
			break
		}

		if err == io.EOF {
			break
		}
	}

	return entries, parsedOffset, nil
}

// parseTailAssistant extracts display entries from an assistant message.
func parseTailAssistant(raw json.RawMessage, ts time.Time) []TailEntry {
	if raw == nil {
		return nil
	}

	var msg tailMessageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	var blocks []tailContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}

	var entries []TailEntry
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			summary := truncateLine(text, 120)
			var detail string
			if len(text) > 120 {
				detail = truncateLine(text, 500)
			}
			entries = append(entries, TailEntry{
				Timestamp: ts,
				Type:      "assistant",
				Activity:  "text",
				Summary:   summary,
				Detail:    detail,
			})

		case "tool_use":
			summary := toolUseSummary(block.Name, block.Input)
			entries = append(entries, TailEntry{
				Timestamp: ts,
				Type:      "assistant",
				Activity:  "tool_use",
				Summary:   summary,
			})

		case "thinking":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			entries = append(entries, TailEntry{
				Timestamp: ts,
				Type:      "assistant",
				Activity:  "thinking",
				Summary:   truncateLine(text, 120),
			})
		}
	}

	return entries
}

// parseTailUser extracts display entries from a user message.
func parseTailUser(raw json.RawMessage, ts time.Time) []TailEntry {
	if raw == nil {
		return nil
	}

	var msg tailMessageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	var blocks []tailContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}

	var entries []TailEntry
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]
		switch block.Type {
		case "tool_result":
			summary := toolResultSummary(block.Content)
			entries = append(entries, TailEntry{
				Timestamp: ts,
				Type:      "user",
				Activity:  "tool_result",
				Summary:   summary,
			})

		case "text":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			entries = append(entries, TailEntry{
				Timestamp: ts,
				Type:      "user",
				Activity:  "text",
				Summary:   truncateLine(text, 120),
			})
		}
	}

	return entries
}

// parseTailProgress extracts a display entry from a progress JSONL line.
func parseTailProgress(line []byte, ts time.Time) *TailEntry {
	var entry tailProgressEntry
	if err := json.Unmarshal(line, &entry); err != nil || entry.ToolUseID == "" {
		return nil
	}

	slug := entry.Slug
	if slug == "" {
		slug = entry.ToolUseID[:8]
	}

	fallback := &TailEntry{
		Timestamp: ts,
		Type:      "progress",
		Activity:  "subagent",
		Summary:   fmt.Sprintf("[%s] progress", slug),
	}

	// Parse the nested data to determine what the subagent is doing.
	if entry.Data == nil {
		return fallback
	}

	var pd tailProgressData
	if err := json.Unmarshal(entry.Data, &pd); err != nil {
		return fallback
	}

	activity := "subagent"
	summary := fmt.Sprintf("[%s] progress", slug)

	switch pd.Message.Type {
	case "assistant":
		// Try to extract what tool the subagent is using.
		if pd.Message.Message != nil {
			var innerMsg tailMessageContent
			if json.Unmarshal(pd.Message.Message, &innerMsg) == nil {
				var blocks []tailContentBlock
				if json.Unmarshal(innerMsg.Content, &blocks) == nil {
					for j := 0; j < len(blocks); j++ {
						if blocks[j].Type == "tool_use" {
							activity = "tool_use"
							summary = fmt.Sprintf("[%s] %s", slug, toolUseSummary(blocks[j].Name, blocks[j].Input))
							break
						}
						if blocks[j].Type == "text" {
							text := strings.TrimSpace(blocks[j].Text)
							if text != "" {
								activity = "thinking"
								summary = fmt.Sprintf("[%s] %s", slug, truncateLine(text, 100))
							}
						}
					}
				}
			}
		}
	case "user":
		activity = "tool_result"
		summary = fmt.Sprintf("[%s] received result", slug)
	}

	return &TailEntry{
		Timestamp: ts,
		Type:      "progress",
		Activity:  activity,
		Summary:   summary,
	}
}

// parseTailSystem creates a display entry from a system JSONL line.
func parseTailSystem(entry tailJSONLEntry, ts time.Time) TailEntry {
	activity := "system"
	summary := "system event"

	if entry.Subtype != "" {
		switch entry.Subtype {
		case "compact_boundary":
			activity = "compact"
			summary = "context compacted"
		default:
			summary = entry.Subtype
		}
	}

	return TailEntry{
		Timestamp: ts,
		Type:      "system",
		Activity:  activity,
		Summary:   summary,
	}
}

// toolUseSummary returns a one-line description of a tool call.
func toolUseSummary(name string, input json.RawMessage) string {
	if input == nil {
		return name
	}

	// Try to extract meaningful parameters for common tools.
	var params map[string]json.RawMessage
	if err := json.Unmarshal(input, &params); err != nil {
		return name
	}

	switch name {
	case "Read", "Write", "Edit":
		if fp, ok := params["file_path"]; ok {
			var path string
			if json.Unmarshal(fp, &path) == nil {
				return fmt.Sprintf("%s %s", name, shortenPath(path))
			}
		}
	case "Bash":
		if cmd, ok := params["command"]; ok {
			var command string
			if json.Unmarshal(cmd, &command) == nil {
				return fmt.Sprintf("Bash: %s", truncateLine(command, 80))
			}
		}
	case "Glob":
		if pat, ok := params["pattern"]; ok {
			var pattern string
			if json.Unmarshal(pat, &pattern) == nil {
				return fmt.Sprintf("Glob %s", pattern)
			}
		}
	case "Grep":
		if pat, ok := params["pattern"]; ok {
			var pattern string
			if json.Unmarshal(pat, &pattern) == nil {
				return fmt.Sprintf("Grep %s", truncateLine(pattern, 60))
			}
		}
	case "Agent":
		if desc, ok := params["description"]; ok {
			var d string
			if json.Unmarshal(desc, &d) == nil {
				return fmt.Sprintf("Agent: %s", truncateLine(d, 80))
			}
		}
	case "WebSearch":
		if q, ok := params["query"]; ok {
			var query string
			if json.Unmarshal(q, &query) == nil {
				return fmt.Sprintf("WebSearch: %s", truncateLine(query, 80))
			}
		}
	case "WebFetch":
		if u, ok := params["url"]; ok {
			var urlStr string
			if json.Unmarshal(u, &urlStr) == nil {
				return fmt.Sprintf("WebFetch %s", truncateLine(urlStr, 80))
			}
		}
	}

	return name
}

// toolResultSummary extracts a brief summary from tool_result content.
func toolResultSummary(content json.RawMessage) string {
	if content == nil {
		return "result"
	}

	// Content can be a string or an array.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		lines := strings.SplitN(s, "\n", 2)
		return fmt.Sprintf("→ %s", truncateLine(lines[0], 100))
	}

	// Try as array of content blocks.
	var blocks []tailContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		for j := 0; j < len(blocks); j++ {
			if blocks[j].Type == "text" && blocks[j].Text != "" {
				lines := strings.SplitN(blocks[j].Text, "\n", 2)
				return fmt.Sprintf("→ %s", truncateLine(lines[0], 100))
			}
		}
	}

	return "result"
}

// SanitizeTailEntries strips sensitive content (assistant text, thinking,
// tool parameters, tool output) from tail entries, keeping only structural
// metadata: timestamps, types, activity names, and tool names.
func SanitizeTailEntries(entries []TailEntry) []TailEntry {
	result := make([]TailEntry, len(entries))
	for i := 0; i < len(entries); i++ {
		result[i] = sanitizeTailEntry(entries[i])
	}
	return result
}

func sanitizeTailEntry(e TailEntry) TailEntry {
	// Never expose extended content.
	e.Detail = ""

	switch e.Activity {
	case "text":
		if e.Type == "assistant" {
			e.Summary = "(text output)"
		} else {
			e.Summary = "(user input)"
		}
	case "thinking":
		prefix, _ := splitProgressPrefix(e.Summary)
		e.Summary = prefix + "(thinking)"
	case "tool_result":
		prefix, _ := splitProgressPrefix(e.Summary)
		e.Summary = prefix + "(result)"
	case "tool_use":
		prefix, rest := splitProgressPrefix(e.Summary)
		e.Summary = prefix + extractToolName(rest)
	}

	return e
}

// splitProgressPrefix splits a summary like "[slug] rest" into the prefix
// "[slug] " and the remainder "rest". If no [slug] prefix is present,
// the prefix is empty.
func splitProgressPrefix(s string) (string, string) {
	if len(s) > 0 && s[0] == '[' {
		if idx := strings.Index(s, "] "); idx >= 0 {
			return s[:idx+2], s[idx+2:]
		}
	}
	return "", s
}

// extractToolName returns just the tool name from a tool_use summary like
// "Bash: cat /etc/passwd" → "Bash", "Read tail.go" → "Read".
func extractToolName(summary string) string {
	if idx := strings.IndexAny(summary, " :"); idx >= 0 {
		return summary[:idx]
	}
	return summary
}

// truncateLine truncates a string to maxLen, adding "…" if truncated.
// Multi-line strings are collapsed to the first line.
func truncateLine(s string, maxLen int) string {
	// Collapse to first line.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)

	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

// shortenPath returns the last 2 components of a path for display.
func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

