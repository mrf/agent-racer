package monitor

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CodexSource implements Source for OpenAI Codex CLI sessions. It discovers
// sessions by scanning ~/.codex/sessions/ for recently-modified rollout
// JSONL files and parses them incrementally.
//
// Codex CLI stores sessions at:
//
//	~/.codex/sessions/YYYY/MM/DD/rollout-{timestamp}-{uuid}.jsonl
//
// The CODEX_HOME environment variable can override the base directory.
type CodexSource struct {
	discoverWindow time.Duration
}

func NewCodexSource(discoverWindow time.Duration) *CodexSource {
	return &CodexSource{discoverWindow: discoverWindow}
}

func (c *CodexSource) Name() string { return "codex" }

// codexHomeDir returns the base Codex directory, respecting CODEX_HOME.
func codexHomeDir() string {
	if env := os.Getenv("CODEX_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func (c *CodexSource) Discover() ([]SessionHandle, error) {
	base := codexHomeDir()
	if base == "" {
		return nil, nil
	}

	sessionsDir := filepath.Join(base, "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil, nil
	}

	cutoff := time.Now().Add(-c.discoverWindow)
	var handles []SessionHandle

	// Walk YYYY/MM/DD directory structure.
	err := filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}

		sessionID := codexSessionIDFromFilename(d.Name())
		handles = append(handles, SessionHandle{
			SessionID: sessionID,
			LogPath:   path,
			Source:    "codex",
			StartedAt: info.ModTime(), // approximation; refined by parsing
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return handles, nil
}

func (c *CodexSource) Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error) {
	f, err := os.Open(handle.LogPath)
	if err != nil {
		return SourceUpdate{}, offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return SourceUpdate{}, offset, err
		}
	}

	var update SourceUpdate
	reader := bufio.NewReader(f)
	parsedOffset := offset // Track offset only after successfully parsing complete lines
	isFirstLine := (offset == 0)

	for {
		line, err := reader.ReadBytes('\n')

		// Handle read errors (except EOF for last incomplete line)
		if err != nil && err != io.EOF {
			return update, parsedOffset, err
		}

		// Empty read means we've reached EOF with nothing left
		if len(line) == 0 {
			break
		}

		// Only process lines that end with newline (complete lines).
		// Incomplete lines (no trailing newline) are preserved for next read.
		if line[len(line)-1] != '\n' {
			// Line is incomplete - don't parse or advance offset
			if err == io.EOF {
				break
			}
			continue
		}

		// Trim the newline for JSON parsing
		lineData := line[:len(line)-1]

		parsed := parseCodexLine(lineData, isFirstLine)
		mergeCodexParsed(&update, parsed)

		// Successfully processed complete line, advance offset
		parsedOffset += int64(len(line))
		isFirstLine = false

		if err == io.EOF {
			break
		}
	}

	if parsedOffset > 0 && update.HasData() {
		log.Printf("[codex] Parsed data from %s", handle.LogPath)
	}

	return update, parsedOffset, nil
}

// codexParsed holds fields extracted from a single Codex JSONL line.
type codexParsed struct {
	sessionID        string
	model            string
	workingDir       string
	activity         string
	lastTool         string
	tokensIn         int
	tokensOut        int
	maxContextTokens int
	messages         int
	toolCalls        int
	timestamp        time.Time
}

// parseCodexLine parses a single line from a Codex rollout JSONL file.
// It handles both the new RolloutLine envelope format (type/payload) and
// the older bare format.
func parseCodexLine(line []byte, firstLine bool) codexParsed {
	var parsed codexParsed

	// Try to detect format: new envelope has top-level "type" + "payload",
	// old format is bare SessionMeta or ResponseItem.
	var envelope struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return parsed
	}

	if envelope.Type != "" && envelope.Payload != nil {
		// New RolloutLine envelope format.
		return parseCodexEnvelope(envelope.Type, envelope.Payload)
	}

	// Old format or SessionMeta header.
	if firstLine {
		return parseCodexSessionMeta(line)
	}

	// Old format response item.
	return parseCodexBareItem(line)
}

func parseCodexEnvelope(typ string, payload json.RawMessage) codexParsed {
	var parsed codexParsed

	switch typ {
	case "session_meta":
		var meta struct {
			SessionID      string          `json:"session_id"`
			ConversationID string          `json:"conversation_id"`
			Model          json.RawMessage `json:"model"`
			ModelProvider  string          `json:"model_provider"`
			Timestamp      string          `json:"timestamp"`
			Source         string          `json:"source"`
		}
		if json.Unmarshal(payload, &meta) == nil {
			parsed.sessionID = meta.SessionID
			if parsed.sessionID == "" {
				parsed.sessionID = meta.ConversationID
			}
			parsed.model = parseCodexModel(meta.Model)
			if meta.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339Nano, meta.Timestamp); err == nil {
					parsed.timestamp = t
				}
			}
		}

	case "event_msg":
		var event struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(payload, &event) == nil {
			switch event.Type {
			case "user_message":
				parsed.messages = 1
				parsed.activity = "waiting"
			case "agent_message":
				parsed.messages = 1
				parsed.activity = "thinking"
			case "token_count":
				parseCodexTokenCount(event.Payload, &parsed)
			case "turn_started":
				parseCodexTurnStarted(event.Payload, &parsed)
			case "tool_call":
				parseCodexToolCall(event.Payload, &parsed)
			case "session_configured":
				var cfg struct {
					Model json.RawMessage `json:"model"`
				}
				if json.Unmarshal(event.Payload, &cfg) == nil {
					parsed.model = parseCodexModel(cfg.Model)
				}
			}
		}

	case "response_item":
		parseCodexResponseItem(payload, &parsed)

	case "env_context":
		var env struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(payload, &env) == nil && env.Cwd != "" {
			parsed.workingDir = env.Cwd
		}
	}

	return parsed
}

func parseCodexSessionMeta(line []byte) codexParsed {
	var parsed codexParsed
	var meta struct {
		SessionID      string          `json:"session_id"`
		ConversationID string          `json:"conversation_id"`
		Model          json.RawMessage `json:"model"`
		Timestamp      string          `json:"timestamp"`
	}
	if json.Unmarshal(line, &meta) == nil {
		parsed.sessionID = meta.SessionID
		if parsed.sessionID == "" {
			parsed.sessionID = meta.ConversationID
		}
		parsed.model = parseCodexModel(meta.Model)
		if meta.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, meta.Timestamp); err == nil {
				parsed.timestamp = t
			}
		}
	}
	return parsed
}

func parseCodexBareItem(line []byte) codexParsed {
	var parsed codexParsed
	var item struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(line, &item) != nil {
		return parsed
	}

	switch item.Type {
	case "message":
		parsed.messages = 1
		parsed.activity = "thinking"
	case "command_execution":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		var cmd struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(line, &cmd) == nil && cmd.Command != "" {
			parsed.lastTool = "Bash"
		}
	case "file_change":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = "FileEdit"
	case "mcp_tool_call":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		var mcp struct {
			ToolName string `json:"tool_name"`
			Name     string `json:"name"`
		}
		if json.Unmarshal(line, &mcp) == nil {
			parsed.lastTool = mcp.ToolName
			if parsed.lastTool == "" {
				parsed.lastTool = mcp.Name
			}
		}
	case "token_count":
		parseCodexTokenCount(line, &parsed)
	case "tool_call":
		parseCodexToolCall(line, &parsed)
	case "reasoning":
		parsed.activity = "thinking"
	case "web_search":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = "WebSearch"
	}

	return parsed
}

func parseCodexResponseItem(payload json.RawMessage, parsed *codexParsed) {
	var item struct {
		Type     string `json:"type"`
		Command  string `json:"command"`
		ToolName string `json:"tool_name"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(payload, &item) != nil {
		return
	}

	switch item.Type {
	case "message":
		parsed.messages = 1
		parsed.activity = "thinking"
	case "command_execution":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = "Bash"
	case "file_change":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = "FileEdit"
	case "mcp_tool_call":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = item.ToolName
		if parsed.lastTool == "" {
			parsed.lastTool = item.Name
		}
	case "tool_call":
		parseCodexToolCall(payload, parsed)
	case "reasoning":
		parsed.activity = "thinking"
	case "web_search":
		parsed.toolCalls = 1
		parsed.activity = "tool_use"
		parsed.lastTool = "WebSearch"
	}
}

func parseCodexTokenCount(payload json.RawMessage, parsed *codexParsed) {
	// Codex token counts are cumulative snapshots. The model_context_window
	// field reports the model's total context size when available.
	var tc struct {
		InputTokens         int `json:"input_tokens"`
		CachedInputTokens   int `json:"cached_input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		TotalTokens         int `json:"total_tokens"`
		ModelContextWindow  int `json:"model_context_window"`
	}
	if json.Unmarshal(payload, &tc) == nil {
		parsed.tokensIn = tc.InputTokens
		parsed.tokensOut = tc.OutputTokens
		if tc.ModelContextWindow > 0 {
			parsed.maxContextTokens = tc.ModelContextWindow
		}
	}
}

func parseCodexTurnStarted(payload json.RawMessage, parsed *codexParsed) {
	var ts struct {
		ModelContextWindow int `json:"model_context_window"`
	}
	if json.Unmarshal(payload, &ts) == nil && ts.ModelContextWindow > 0 {
		parsed.maxContextTokens = ts.ModelContextWindow
	}
}

func parseCodexToolCall(payload json.RawMessage, parsed *codexParsed) {
	if parsed == nil {
		return
	}
	var tool struct {
		Name     string `json:"name"`
		ToolName string `json:"tool_name"`
		Tool     struct {
			Name string `json:"name"`
		} `json:"tool"`
	}
	if json.Unmarshal(payload, &tool) != nil {
		return
	}
	parsed.toolCalls = 1
	parsed.activity = "tool_use"
	parsed.lastTool = tool.ToolName
	if parsed.lastTool == "" {
		parsed.lastTool = tool.Name
	}
	if parsed.lastTool == "" {
		parsed.lastTool = tool.Tool.Name
	}
}

func parseCodexModel(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var model string
	if json.Unmarshal(raw, &model) == nil {
		return model
	}
	var obj struct {
		Name  string `json:"name"`
		ID    string `json:"id"`
		Model string `json:"model"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.Name != "" {
			return obj.Name
		}
		if obj.ID != "" {
			return obj.ID
		}
		if obj.Model != "" {
			return obj.Model
		}
	}
	return ""
}

func mergeCodexParsed(update *SourceUpdate, parsed codexParsed) {
	if parsed.sessionID != "" {
		update.SessionID = parsed.sessionID
	}
	if parsed.model != "" {
		update.Model = parsed.model
	}
	if parsed.workingDir != "" {
		update.WorkingDir = parsed.workingDir
	}
	if parsed.activity != "" {
		update.Activity = parsed.activity
	}
	if parsed.lastTool != "" {
		update.LastTool = parsed.lastTool
	}
	// Token counts are cumulative snapshots — take the latest.
	if parsed.tokensIn > 0 {
		update.TokensIn = parsed.tokensIn
	}
	if parsed.tokensOut > 0 {
		update.TokensOut = parsed.tokensOut
	}
	if parsed.maxContextTokens > 0 {
		update.MaxContextTokens = parsed.maxContextTokens
	}
	// Messages and tool calls are deltas.
	update.MessageCount += parsed.messages
	update.ToolCalls += parsed.toolCalls
	if !parsed.timestamp.IsZero() {
		update.LastTime = parsed.timestamp
	}
}

// codexSessionIDFromFilename extracts the UUID from a rollout filename.
// Format: rollout-{timestamp}-{uuid}.jsonl
func codexSessionIDFromFilename(name string) string {
	// Strip extension.
	name = strings.TrimSuffix(name, ".jsonl")
	// Strip "rollout-" prefix.
	name = strings.TrimPrefix(name, "rollout-")

	// The remainder is "{timestamp}-{uuid}". The UUID is the last 36 chars
	// (standard UUID format: 8-4-4-4-12). If the string is long enough,
	// extract from the end.
	if len(name) >= 36 {
		candidate := name[len(name)-36:]
		// Basic UUID format check: contains dashes at expected positions.
		if len(candidate) == 36 && candidate[8] == '-' && candidate[13] == '-' {
			return candidate
		}
	}

	// Fallback: use the last dash-separated segment that looks like a UUID,
	// or the whole remainder.
	parts := strings.Split(name, "-")
	if len(parts) >= 5 {
		// Try to reconstruct UUID from last 5 segments.
		candidate := strings.Join(parts[len(parts)-5:], "-")
		if len(candidate) == 36 {
			return candidate
		}
	}

	// Last resort: use the full name as ID.
	return name
}
