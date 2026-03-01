package monitor

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"time"
)

// readFirstTimestamp opens path, reads its first line, and parses the JSON
// "timestamp" field as RFC3339. Returns a zero Time and false if the file
// cannot be read or contains no valid timestamp.
func readFirstTimestamp(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return time.Time{}, false
	}

	var entry struct {
		Timestamp string `json:"timestamp"`
	}
	if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Timestamp == "" {
		return time.Time{}, false
	}

	t, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ClaudeSource implements Source for Claude Code sessions. It discovers
// sessions by scanning ~/.claude/projects/ for recently-modified JSONL
// files and parses them incrementally using the existing JSONL parser.
type ClaudeSource struct {
	// discoverWindow controls how far back to look for session files.
	discoverWindow time.Duration
}

// NewClaudeSource creates a ClaudeSource that discovers session files
// modified within the given window (e.g., 10*time.Minute).
func NewClaudeSource(discoverWindow time.Duration) *ClaudeSource {
	return &ClaudeSource{discoverWindow: discoverWindow}
}

func (c *ClaudeSource) Name() string { return "claude" }

func (c *ClaudeSource) Discover() ([]SessionHandle, error) {
	paths, err := FindRecentSessionFiles(c.discoverWindow)
	if err != nil {
		return nil, err
	}

	handles := make([]SessionHandle, 0, len(paths))
	for _, path := range paths {
		sessionID := SessionIDFromPath(path)
		workingDir := workingDirFromFile(path)

		startedAt, _ := readFirstTimestamp(path)

		handles = append(handles, SessionHandle{
			SessionID:  sessionID,
			LogPath:    path,
			WorkingDir: workingDir,
			Source:     "claude",
			StartedAt:  startedAt,
		})
	}

	return handles, nil
}

func (c *ClaudeSource) Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error) {
	result, newOffset, err := ParseSessionJSONL(handle.LogPath, offset, handle.KnownSlug, handle.KnownSubagentParents)
	if err != nil {
		return SourceUpdate{}, offset, err
	}

	// No new data since last parse.
	if newOffset == offset {
		return SourceUpdate{}, offset, nil
	}

	update := SourceUpdate{
		SessionID:       result.SessionID,
		Slug:            result.Slug,
		Model:           result.Model,
		MessageCount:    result.MessageCount,
		ToolCalls:       result.ToolCalls,
		LastTool:        result.LastTool,
		Activity:        result.LastActivity,
		LastTime:        result.LastTime,
		WorkingDir:      result.WorkingDir,
		Subagents:       result.Subagents,
		CompactionCount: result.CompactionCount,
	}

	if result.LatestUsage != nil {
		update.TokensIn = result.LatestUsage.TotalContext()
		update.TokensOut = result.LatestUsage.OutputTokens
	}

	if handle.WorkingDir == "" && update.WorkingDir == "" {
		update.WorkingDir = workingDirFromFile(handle.LogPath)
	}

	log.Printf("[claude] Parsed %d new bytes from %s", newOffset-offset, handle.LogPath)

	return update, newOffset, nil
}
