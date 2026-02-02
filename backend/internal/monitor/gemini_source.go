package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GeminiSource implements Source for Google Gemini CLI sessions. It discovers
// sessions by scanning ~/.gemini/tmp/*/chats/ for recently-modified session
// JSON files and parses them fully on each poll (since Gemini rewrites the
// entire JSON file on each update).
//
// The key challenge is that Gemini uses a one-way SHA-256 hash of the
// project directory as the folder name. We maintain a hash-to-path lookup
// built from running gemini processes to map session files back to working
// directories.
type GeminiSource struct {
	discoverWindow time.Duration

	// hashToPath maps SHA-256 hashes of project directories to their
	// original paths. Built from process scanning and persisted across polls.
	hashToPath map[string]string

	// lastParsed tracks the file modification time we last parsed for each
	// session file. Since Gemini rewrites the whole file, we use mtime to
	// skip unchanged files. We encode this as the "offset" by using the
	// mtime's UnixNano value.
	lastParsed map[string]time.Time
}

func NewGeminiSource(discoverWindow time.Duration) *GeminiSource {
	return &GeminiSource{
		discoverWindow: discoverWindow,
		hashToPath:     make(map[string]string),
		lastParsed:     make(map[string]time.Time),
	}
}

func (g *GeminiSource) Name() string { return "gemini" }

// geminiBaseDir returns the Gemini CLI data directory.
func geminiBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini")
}

func (g *GeminiSource) Discover() ([]SessionHandle, error) {
	base := geminiBaseDir()
	if base == "" {
		return nil, nil
	}

	tmpDir := filepath.Join(base, "tmp")
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		return nil, nil
	}

	// Build hash-to-path mappings from running gemini processes.
	g.refreshHashMappings()

	cutoff := time.Now().Add(-g.discoverWindow)
	var handles []SessionHandle

	projectDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, nil
	}

	for _, projEntry := range projectDirs {
		if !projEntry.IsDir() {
			continue
		}

		hash := projEntry.Name()
		chatsDir := filepath.Join(tmpDir, hash, "chats")
		chatFiles, err := os.ReadDir(chatsDir)
		if err != nil {
			continue
		}

		for _, f := range chatFiles {
			if f.IsDir() {
				continue
			}
			if !strings.HasPrefix(f.Name(), "session-") || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}

			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				continue
			}

			sessionID := geminiSessionIDFromFilename(f.Name())
			workingDir := g.hashToPath[hash]

			handles = append(handles, SessionHandle{
				SessionID:  sessionID,
				LogPath:    filepath.Join(chatsDir, f.Name()),
				WorkingDir: workingDir,
				Source:     "gemini",
				StartedAt:  info.ModTime(),
			})
		}
	}

	return handles, nil
}

func (g *GeminiSource) Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error) {
	info, err := os.Stat(handle.LogPath)
	if err != nil {
		return SourceUpdate{}, offset, err
	}

	// Use mtime encoded as UnixNano for the "offset". If the mtime hasn't
	// changed since last parse, skip re-parsing the whole file.
	currentMtime := info.ModTime()
	lastMtime := g.lastParsed[handle.LogPath]

	if !lastMtime.IsZero() && !currentMtime.After(lastMtime) {
		// File unchanged since last parse.
		return SourceUpdate{}, offset, nil
	}

	data, err := os.ReadFile(handle.LogPath)
	if err != nil {
		return SourceUpdate{}, offset, err
	}

	update := parseGeminiSession(data)

	// Use the new mtime as the offset (encoded as UnixNano).
	newOffset := currentMtime.UnixNano()
	g.lastParsed[handle.LogPath] = currentMtime

	if update.HasData() {
		update.LastTime = currentMtime
		log.Printf("[gemini] Parsed session from %s", handle.LogPath)
	}

	return update, newOffset, nil
}

// parseGeminiSession parses the complete Gemini session JSON file and
// returns a SourceUpdate with absolute counts. The monitor treats
// MessageCount and ToolCalls as deltas, so we track the difference
// ourselves. For now we return absolute values and let the monitor
// handle them as if they were all new (on first parse) or zero-delta
// (on re-parse with same data). The monitor accumulates, but we return
// counts only for new data (since we skip unchanged files via mtime).
func parseGeminiSession(data []byte) SourceUpdate {
	var update SourceUpdate

	// Gemini session files contain a JSON array of message objects, or
	// a wrapper object containing such an array. Try both.
	var messages []geminiMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		// Try wrapper object with common field names.
		var wrapper struct {
			Messages     []geminiMessage `json:"messages"`
			Conversation []geminiMessage `json:"conversation"`
			History      []geminiMessage `json:"history"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return update
		}
		if wrapper.Messages != nil {
			messages = wrapper.Messages
		} else if wrapper.Conversation != nil {
			messages = wrapper.Conversation
		} else if wrapper.History != nil {
			messages = wrapper.History
		}
	}

	for _, msg := range messages {
		role := msg.Role
		if role == "" {
			role = msg.Type
		}

		switch role {
		case "user":
			update.MessageCount++
			update.Activity = "waiting"
		case "model":
			update.MessageCount++
			update.Activity = "thinking"

			// Check content parts for tool calls.
			for _, part := range msg.Content.Parts {
				if part.FunctionCall != nil {
					update.ToolCalls++
					update.Activity = "tool_use"
					update.LastTool = part.FunctionCall.Name
				}
				if part.Thought != "" {
					update.Activity = "thinking"
				}
			}

			// Token usage from response metadata.
			if msg.UsageMetadata != nil {
				if msg.UsageMetadata.PromptTokenCount > 0 {
					update.TokensIn = msg.UsageMetadata.PromptTokenCount
				}
				if msg.UsageMetadata.CandidatesTokenCount > 0 {
					update.TokensOut = msg.UsageMetadata.CandidatesTokenCount
				}
			}
		}

		if msg.Model != "" {
			update.Model = msg.Model
		}
	}

	if update.Model == "" {
		update.Model = extractGeminiModel(data)
	}

	if update.Model != "" {
		update.MaxContextTokens = geminiContextWindow(update.Model)
	}

	return update
}

func extractGeminiModel(data []byte) string {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return ""
	}
	return findGeminiModel(root, 3)
}

func findGeminiModel(value any, depth int) string {
	if depth < 0 || value == nil {
		return ""
	}
	switch v := value.(type) {
	case map[string]any:
		for key, val := range v {
			switch key {
			case "model", "modelName", "model_name", "modelId", "model_id":
				if s, ok := val.(string); ok && s != "" {
					return s
				}
			}
		}
		for _, val := range v {
			if found := findGeminiModel(val, depth-1); found != "" {
				return found
			}
		}
	case []any:
		for _, item := range v {
			if found := findGeminiModel(item, depth-1); found != "" {
				return found
			}
		}
	}
	return ""
}

// geminiContextWindow returns the known context window size for a Gemini
// model. The Gemini CLI itself hardcodes 1,048,576 for all current models
// (see packages/core/src/core/tokenLimits.ts). We mirror that here and
// handle newer models as they appear.
func geminiContextWindow(model string) int {
	switch {
	case strings.HasPrefix(model, "gemini-2.5-"),
		strings.HasPrefix(model, "gemini-2.0-"):
		return 1_048_576
	case strings.HasPrefix(model, "gemini-3-"):
		return 1_000_000
	case strings.HasPrefix(model, "gemini-1.5-pro"):
		return 2_097_152
	case strings.HasPrefix(model, "gemini-1.5-flash"):
		return 1_048_576
	default:
		// Match the Gemini CLI default.
		return 1_048_576
	}
}

// geminiMessage represents a message in a Gemini session JSON file.
type geminiMessage struct {
	Role          string        `json:"role"`
	Type          string        `json:"type"`
	Model         string        `json:"model,omitempty"`
	Content       geminiContent `json:"content"`
	UsageMetadata *geminiUsage  `json:"usageMetadata,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string              `json:"text,omitempty"`
	Thought      string              `json:"thought,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// refreshHashMappings scans /proc for running gemini processes to build
// the hash-to-path lookup table.
func (g *GeminiSource) refreshHashMappings() {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}

		if !isGeminiProcess(string(cmdline)) {
			continue
		}

		cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
		if err != nil {
			continue
		}

		hash := hashProjectPath(cwd)
		if _, exists := g.hashToPath[hash]; !exists {
			g.hashToPath[hash] = cwd
			log.Printf("[gemini] Mapped hash %s -> %s", hash[:12], cwd)
		}
	}
}

// isGeminiProcess checks if a /proc cmdline belongs to a gemini CLI process.
func isGeminiProcess(cmdline string) bool {
	parts := strings.Split(cmdline, "\x00")
	if len(parts) == 0 {
		return false
	}

	exe := filepath.Base(parts[0])

	if exe == "gemini" {
		return true
	}

	// Gemini CLI is Node-based; check for node running gemini scripts.
	if exe == "node" || exe == "npx" {
		for _, part := range parts[1:] {
			if strings.Contains(part, "gemini") && !strings.Contains(part, "node_modules/.bin") {
				return true
			}
		}
	}

	return false
}

// hashProjectPath computes the SHA-256 hash of a directory path, matching
// Gemini CLI's project hash scheme.
func hashProjectPath(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h)
}

// geminiSessionIDFromFilename extracts the session ID from a Gemini
// session filename. Format: session-{date}T{time}-{short_hex}.json
func geminiSessionIDFromFilename(name string) string {
	name = strings.TrimSuffix(name, ".json")
	// The short hex ID is the last dash-separated segment.
	parts := strings.Split(name, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return name
}
