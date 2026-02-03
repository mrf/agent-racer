package monitor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TokenUsage struct {
	InputTokens                int `json:"input_tokens"`
	CacheCreationInputTokens   int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens       int `json:"cache_read_input_tokens"`
	OutputTokens               int `json:"output_tokens"`
}

func (t TokenUsage) TotalContext() int {
	return t.InputTokens + t.CacheCreationInputTokens + t.CacheReadInputTokens
}

type jsonlEntry struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type messageContent struct {
	Model   string          `json:"model"`
	Role    string          `json:"role"`
	Usage   *TokenUsage     `json:"usage,omitempty"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type ParseResult struct {
	SessionID    string
	Model        string
	LatestUsage  *TokenUsage
	MessageCount int
	ToolCalls    int
	LastTool     string
	LastActivity string
	LastTime     time.Time
}

func FindSessionFile(workingDir string) (string, error) {
	encoded := encodeProjectPath(workingDir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", encoded)
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("reading project dir %s: %w", projectDir, err)
	}

	var bestPath string
	var bestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			bestPath = filepath.Join(projectDir, entry.Name())
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("no session files found in %s", projectDir)
	}
	return bestPath, nil
}

func ParseSessionJSONL(path string, offset int64) (*ParseResult, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, offset, err
		}
	}

	result := &ParseResult{}
	reader := bufio.NewReader(f)
	parsedOffset := offset // Track offset only after successfully parsing complete lines

	for {
		line, err := reader.ReadBytes('\n')

		// Handle read errors (except EOF for last incomplete line)
		if err != nil && err != io.EOF {
			return result, parsedOffset, err
		}

		// Empty read means we've reached EOF with nothing left
		if len(line) == 0 {
			break
		}

		// Only process lines that end with newline (complete lines).
		// Incomplete lines (no trailing newline) are preserved for next read.
		if len(line) == 0 || line[len(line)-1] != '\n' {
			// Line is incomplete - don't parse or advance offset
			if err == io.EOF {
				break
			}
			continue
		}

		// Trim the newline for JSON parsing
		lineData := line[:len(line)-1]

		var entry jsonlEntry
		if err := json.Unmarshal(lineData, &entry); err != nil {
			// Silently skip malformed lines but do advance offset
			parsedOffset += int64(len(line))
			if err == io.EOF {
				break
			}
			continue
		}

		// Successfully parsed complete line, advance offset
		parsedOffset += int64(len(line))

		if entry.SessionID != "" && result.SessionID == "" {
			result.SessionID = entry.SessionID
		}

		if entry.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				result.LastTime = t
			}
		}

		switch entry.Type {
		case "assistant":
			result.MessageCount++
			result.LastActivity = "thinking"
			parseAssistantMessage(entry.Message, result)

		case "user":
			result.MessageCount++
			result.LastActivity = "waiting"
		}

		if err == io.EOF {
			break
		}
	}

	return result, parsedOffset, nil
}

func parseAssistantMessage(raw json.RawMessage, result *ParseResult) {
	if raw == nil {
		return
	}

	var msg messageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	if msg.Model != "" {
		result.Model = msg.Model
	}

	if msg.Usage != nil {
		result.LatestUsage = msg.Usage
	}

	// Parse content blocks for tool use
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		if block.Type == "tool_use" {
			result.ToolCalls++
			result.LastTool = block.Name
			result.LastActivity = "tool_use"
		}
	}
}

func encodeProjectPath(path string) string {
	// Claude Code uses a simple encoding: replace / with -
	// But leading / is also replaced, so /home/user/proj becomes -home-user-proj
	clean := filepath.Clean(path)
	encoded := strings.ReplaceAll(clean, "/", "-")
	return encoded
}

func FindAllSessionFiles(workingDir string) ([]string, error) {
	encoded := encodeProjectPath(workingDir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", encoded)
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			paths = append(paths, filepath.Join(projectDir, entry.Name()))
		}
	}
	return paths, nil
}

func SessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

// FindRecentSessionFiles finds all active session files across all projects
// modified within the given duration
func FindRecentSessionFiles(within time.Duration) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	projectEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-within)
	var results []string

	for _, projEntry := range projectEntries {
		if !projEntry.IsDir() {
			continue
		}
		projPath := filepath.Join(projectsDir, projEntry.Name())
		files, err := os.ReadDir(projPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(cutoff) {
				results = append(results, filepath.Join(projPath, f.Name()))
			}
		}
	}

	return results, nil
}

// DecodeProjectPath reverses the encoding to get the original working dir
func DecodeProjectPath(encoded string) string {
	// encoded is like -home-user-proj
	// We need to figure out the original path
	// The encoding replaces / with -, so -home-mrf-Projects becomes /home/mrf/Projects
	// But this is ambiguous for dirs with hyphens. We check if the path exists.
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		decoded = encoded
	}

	// Try treating all dashes as path separators
	if strings.HasPrefix(decoded, "-") {
		candidate := strings.ReplaceAll(decoded, "-", "/")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		// If that didn't work, try progressively: check multiple combinations
		// by treating some dashes as path separators and others as literal dashes
		parts := strings.Split(decoded[1:], "-") // skip leading dash

		// Try replacing dashes from left to right, keeping last segments with dashes
		for numSlashes := len(parts) - 1; numSlashes > 0; numSlashes-- {
			// Join first numSlashes parts with /, rest with -
			pathParts := make([]string, numSlashes)
			for i := 0; i < numSlashes; i++ {
				pathParts[i] = parts[i]
			}
			candidate := "/" + strings.Join(pathParts, "/")

			if numSlashes < len(parts) {
				remaining := strings.Join(parts[numSlashes:], "-")
				candidate = candidate + "/" + remaining
			}

			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// Fallback: return basename (best effort)
	// Assume first 1-2 parts are directory path, rest is the basename
	parts := strings.Split(strings.TrimPrefix(decoded, "-"), "-")
	if len(parts) > 2 {
		// Assume structure like /home/user/... or /tmp/... where first 2 are dir
		return strings.Join(parts[2:], "-")
	} else if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return decoded
}

// FindSessionForProcess tries to find the most recent session file
// for a process working in the given directory
func FindSessionForProcess(workingDir string, processStartTime time.Time) (string, error) {
	sessionFile, err := FindSessionFile(workingDir)
	if err != nil {
		return "", err
	}

	// Verify the file was modified after (or around) process start
	info, err := os.Stat(sessionFile)
	if err != nil {
		return "", err
	}

	// Allow 30s tolerance
	if info.ModTime().Before(processStartTime.Add(-30 * time.Second)) {
		log.Printf("Session file %s is older than process start, may be stale", sessionFile)
	}

	return sessionFile, nil
}
