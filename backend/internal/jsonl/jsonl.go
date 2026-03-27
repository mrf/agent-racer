// Package jsonl provides shared types and iteration for Claude Code JSONL
// session files. Both the monitor (session state extraction) and tail
// (display entry generation) parsers build on this foundation.
package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

const (
	// MaxFileSize is the maximum JSONL file size we'll parse (500 MB).
	// Files exceeding this are skipped to prevent OOM from runaway logs.
	MaxFileSize = 500 * 1024 * 1024

	// MaxLineLength is the maximum length of a single JSONL line (1 MB).
	// Lines exceeding this are skipped to prevent excessive memory use.
	MaxLineLength = 1024 * 1024
)

// TokenUsage represents API token usage from an assistant message.
type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// TotalContext returns the total context tokens (input + cache).
func (t TokenUsage) TotalContext() int {
	return t.InputTokens + t.CacheCreationInputTokens + t.CacheReadInputTokens
}

// Entry is the top-level structure of a Claude JSONL line.
type Entry struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	UUID      string          `json:"uuid"`
	SessionID string          `json:"sessionId"`
	Slug      string          `json:"slug"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`
}

// ParseTimestamp parses the entry's RFC3339Nano timestamp.
func (e *Entry) ParseTimestamp() (time.Time, bool) {
	if e.Timestamp == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// MessageContent is the message object inside assistant/user entries.
type MessageContent struct {
	Model   string          `json:"model"`
	Role    string          `json:"role"`
	Usage   *TokenUsage     `json:"usage,omitempty"`
	Content json.RawMessage `json:"content"`
}

// ContentBlock is a single block inside a message's content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id,omitempty"`          // tool_use block ID
	ToolUseID string          `json:"tool_use_id,omitempty"` // tool_result references the tool_use
	Text      string          `json:"text,omitempty"`        // text content block
	Input     json.RawMessage `json:"input,omitempty"`       // tool_use input
	Content   json.RawMessage `json:"content,omitempty"`     // tool_result content
}

// ProgressEntry is the top-level structure for type:"progress" JSONL entries.
type ProgressEntry struct {
	Type            string          `json:"type"`
	ToolUseID       string          `json:"toolUseID"`
	ParentToolUseID string          `json:"parentToolUseID"`
	SessionID       string          `json:"sessionId"`
	Slug            string          `json:"slug"`
	Timestamp       string          `json:"timestamp"`
	Data            json.RawMessage `json:"data"`
}

// ProgressDataHeader is used for fast type-checking of progress data.
type ProgressDataHeader struct {
	Type string `json:"type"`
}

// ProgressData wraps the nested data.message structure inside a progress entry.
type ProgressData struct {
	Message struct {
		Type    string          `json:"type"` // "assistant" or "user"
		Message json.RawMessage `json:"message"`
	} `json:"message"`
}

// EntryVisitor is called for each parsed JSONL entry. It receives the parsed
// entry header and the full raw line bytes (for re-parsing with richer types
// like ProgressEntry). Return false to stop iteration.
type EntryVisitor func(entry *Entry, line []byte) bool

// ForEachEntry reads a JSONL file from offset, calling visitor for each
// complete, parseable line. Returns the final byte offset.
func ForEachEntry(path string, offset int64, visitor EntryVisitor) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return offset, err
	}
	if info.Size() > MaxFileSize {
		slog.Warn("skipping oversized file", "source", "jsonl", "path", path, "size", info.Size(), "limit", MaxFileSize)
		return offset, fmt.Errorf("file size %d exceeds max %d", info.Size(), MaxFileSize)
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return offset, err
		}
	}

	reader := bufio.NewReader(f)
	parsedOffset := offset

	for {
		line, err := reader.ReadBytes('\n')

		if err != nil && err != io.EOF {
			return parsedOffset, err
		}

		if len(line) == 0 {
			break
		}

		// Only process complete lines (ending with newline).
		if line[len(line)-1] != '\n' {
			if err == io.EOF {
				break
			}
			continue
		}

		// Skip oversized lines.
		if len(line) > MaxLineLength {
			slog.Warn("skipping oversized line", "source", "jsonl", "bytes", len(line), "path", path, "offset", parsedOffset)
			parsedOffset += int64(len(line))
			if err == io.EOF {
				break
			}
			continue
		}

		lineData := line[:len(line)-1]

		var entry Entry
		if jsonErr := json.Unmarshal(lineData, &entry); jsonErr != nil {
			parsedOffset += int64(len(line))
			if err == io.EOF {
				break
			}
			continue
		}

		parsedOffset += int64(len(line))

		if !visitor(&entry, lineData) {
			break
		}

		if err == io.EOF {
			break
		}
	}

	return parsedOffset, nil
}
