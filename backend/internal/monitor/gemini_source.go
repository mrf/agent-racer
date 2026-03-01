package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
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

	// prevCounts tracks the absolute message/tool counts from the previous
	// parse of each session file so we can return deltas to the monitor.
	prevCounts map[string]geminiAbsoluteCounts
}

// geminiAbsoluteCounts holds the absolute counts from the last full parse
// of a Gemini session file. Used to compute deltas for the monitor.
type geminiAbsoluteCounts struct {
	Messages  int
	ToolCalls int
}

func NewGeminiSource(discoverWindow time.Duration) *GeminiSource {
	return &GeminiSource{
		discoverWindow: discoverWindow,
		hashToPath:     make(map[string]string),
		lastParsed:     make(map[string]time.Time),
		prevCounts:     make(map[string]geminiAbsoluteCounts),
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

	return g.discoverFromDir(tmpDir), nil
}

// discoverFromDir scans a Gemini tmp directory for active sessions and prunes
// stale entries from internal maps. Extracted from Discover() for testability.
func (g *GeminiSource) discoverFromDir(tmpDir string) []SessionHandle {
	cutoff := time.Now().Add(-g.discoverWindow)
	var handles []SessionHandle

	activeHashes := make(map[string]bool)
	activeLogPaths := make(map[string]bool)

	projectDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
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

			activeHashes[hash] = true

			sessionID := geminiSessionIDFromFilename(f.Name())
			workingDir := g.hashToPath[hash]
			logPath := filepath.Join(chatsDir, f.Name())

			activeLogPaths[logPath] = true

			handles = append(handles, SessionHandle{
				SessionID:  sessionID,
				LogPath:    logPath,
				WorkingDir: workingDir,
				Source:     "gemini",
				StartedAt:  info.ModTime(),
			})
		}
	}

	// Prune stale entries from internal maps to prevent unbounded growth.
	for hash := range g.hashToPath {
		if !activeHashes[hash] {
			delete(g.hashToPath, hash)
		}
	}
	for path := range g.lastParsed {
		if !activeLogPaths[path] {
			delete(g.lastParsed, path)
		}
	}
	for path := range g.prevCounts {
		if !activeLogPaths[path] {
			delete(g.prevCounts, path)
		}
	}

	return handles
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
		// File unchanged since last parse. Return the current mtime as offset
		// so the monitor knows this file has been processed, even if we're
		// skipping the parse. This prevents newly-tracked sessions from having
		// offset=0 when the file was parsed in a previous tracking cycle.
		return SourceUpdate{}, currentMtime.UnixNano(), nil
	}

	data, err := os.ReadFile(handle.LogPath)
	if err != nil {
		return SourceUpdate{}, offset, err
	}

	update := parseGeminiSession(data)

	// Convert absolute counts to deltas. The monitor accumulates deltas,
	// but Gemini re-parses the entire file each time returning absolute
	// counts. We track previous values and return the difference.
	prev := g.prevCounts[handle.LogPath]
	current := geminiAbsoluteCounts{
		Messages:  update.MessageCount,
		ToolCalls: update.ToolCalls,
	}

	update.MessageCount = max(current.Messages-prev.Messages, 0)
	update.ToolCalls = max(current.ToolCalls-prev.ToolCalls, 0)
	g.prevCounts[handle.LogPath] = current

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
// returns a SourceUpdate with absolute counts. The caller (GeminiSource.Parse)
// converts these to deltas before returning to the monitor.
//
// The Gemini CLI session format is a JSON object with a "messages" array.
// Each message has "type" (not "role") with values "user", "gemini", or
// "info". Model responses use "gemini" and carry token data in a "tokens"
// field and tool calls in a "toolCalls" array -- both at the message level,
// not nested inside content parts.
func parseGeminiSession(data []byte) SourceUpdate {
	messages := unmarshalGeminiMessages(data)
	if messages == nil {
		return SourceUpdate{}
	}

	var update SourceUpdate

	for _, msg := range messages {
		// Resolve message kind: CLI uses "type", API uses "role".
		kind := msg.Role
		if kind == "" {
			kind = msg.Type
		}

		switch kind {
		case "user":
			update.MessageCount++
			update.Activity = "waiting"
		case "model", "gemini":
			update.MessageCount++
			update.Activity = "thinking"

			// Gemini CLI puts tool calls at the message level.
			for _, tc := range msg.ToolCallsList {
				update.ToolCalls++
				update.Activity = "tool_use"
				update.LastTool = tc.Name
			}

			// Gemini CLI puts thoughts at the message level.
			if len(msg.Thoughts) > 0 {
				update.Activity = "thinking"
			}

			// Gemini API puts tool calls inside content parts.
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

			// Token usage: prefer CLI "tokens" field, fall back to
			// API "usageMetadata" format.
			if msg.Tokens != nil {
				if msg.Tokens.Input > 0 {
					update.TokensIn = msg.Tokens.Input
				}
				if msg.Tokens.Output > 0 {
					update.TokensOut = msg.Tokens.Output
				}
			} else if msg.UsageMetadata != nil {
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

	return update
}

// unmarshalGeminiMessages extracts the message array from a Gemini session
// file. The file may be a bare JSON array or a wrapper object with a
// "messages", "conversation", or "history" field.
func unmarshalGeminiMessages(data []byte) []geminiMessage {
	var messages []geminiMessage
	if json.Unmarshal(data, &messages) == nil {
		return messages
	}

	var wrapper struct {
		Messages     []geminiMessage `json:"messages"`
		Conversation []geminiMessage `json:"conversation"`
		History      []geminiMessage `json:"history"`
	}
	if json.Unmarshal(data, &wrapper) != nil {
		return nil
	}

	switch {
	case wrapper.Messages != nil:
		return wrapper.Messages
	case wrapper.Conversation != nil:
		return wrapper.Conversation
	case wrapper.History != nil:
		return wrapper.History
	default:
		return nil
	}
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

// geminiMessage represents a message in a Gemini session JSON file.
// Supports both the real Gemini CLI format (type/tokens/toolCalls at
// message level, content as a plain string) and the Gemini API format
// (role/usageMetadata, content as {parts: [...]}).
type geminiMessage struct {
	Role          string             `json:"role"`
	Type          string             `json:"type"`
	Model         string             `json:"model,omitempty"`
	Content       geminiContent      `json:"content"`
	UsageMetadata *geminiUsage       `json:"usageMetadata,omitempty"`
	Tokens        *geminiTokens      `json:"tokens,omitempty"`
	ToolCallsList []geminiToolCall   `json:"toolCalls,omitempty"`
	Thoughts      []geminiThought    `json:"thoughts,omitempty"`
}

// geminiContent handles the content field which may be a plain string
// (Gemini CLI format) or an object with parts (Gemini API format).
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

func (c *geminiContent) UnmarshalJSON(data []byte) error {
	// Try as object with parts first (Gemini API format).
	type alias geminiContent
	var obj alias
	if err := json.Unmarshal(data, &obj); err == nil {
		*c = geminiContent(obj)
		return nil
	}
	// Accept plain string (Gemini CLI format) -- parts stay empty
	// since we only need structured content parts for tool calls
	// and thoughts.
	var ignore string
	if json.Unmarshal(data, &ignore) == nil {
		return nil
	}
	// Unknown shape -- ignore silently.
	return nil
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

// geminiUsage is the Gemini API response metadata format.
type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// geminiTokens is the Gemini CLI session token format.
type geminiTokens struct {
	Input    int `json:"input"`
	Output   int `json:"output"`
	Cached   int `json:"cached"`
	Thoughts int `json:"thoughts"`
	Tool     int `json:"tool"`
	Total    int `json:"total"`
}

// geminiToolCall represents a tool invocation in a Gemini CLI session.
type geminiToolCall struct {
	Name string `json:"name"`
}

// geminiThought represents a thinking step in a Gemini CLI session.
type geminiThought struct {
	Subject string `json:"subject"`
}

// refreshHashMappings scans running processes for gemini processes to build
// the hash-to-path lookup table.
func (g *GeminiSource) refreshHashMappings() {
	procs, err := process.Processes()
	if err != nil {
		return
	}

	for _, p := range procs {
		args, err := p.CmdlineSlice()
		if err != nil || len(args) == 0 {
			continue
		}

		if !isGeminiProcess(args) {
			continue
		}

		cwd, err := p.Cwd()
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

// isGeminiProcess checks if the command args belong to a gemini CLI process.
func isGeminiProcess(args []string) bool {
	if len(args) == 0 {
		return false
	}

	exe := filepath.Base(args[0])

	switch exe {
	case "gemini":
		return true
	case "node", "npx":
		for _, arg := range args[1:] {
			if strings.Contains(arg, "gemini") && !strings.Contains(arg, "node_modules/.bin") {
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
	parts := strings.Split(name, "-")
	return parts[len(parts)-1]
}
