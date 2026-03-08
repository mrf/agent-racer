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

const (
	// maxFileSize is the maximum JSONL file size we'll parse (500 MB).
	// Files exceeding this are skipped to prevent OOM from runaway logs.
	maxFileSize = 500 * 1024 * 1024

	// maxLineLength is the maximum length of a single JSONL line (1 MB).
	// Lines exceeding this are skipped to prevent excessive memory use.
	maxLineLength = 1024 * 1024

	// maxDecodePathCandidates bounds ambiguous decode search so a long
	// hyphen chain cannot grow without limit.
	maxDecodePathCandidates = 4096
)

type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

func (t TokenUsage) TotalContext() int {
	return t.InputTokens + t.CacheCreationInputTokens + t.CacheReadInputTokens
}

type jsonlEntry struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	UUID      string          `json:"uuid"`
	SessionID string          `json:"sessionId"`
	Slug      string          `json:"slug"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`
}

type messageContent struct {
	Model   string          `json:"model"`
	Role    string          `json:"role"`
	Usage   *TokenUsage     `json:"usage,omitempty"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	ID        string `json:"id,omitempty"`          // tool_use block ID
	ToolUseID string `json:"tool_use_id,omitempty"` // tool_result references the tool_use
	Text      string `json:"text,omitempty"`        // text content block
}

// progressEntry is the top-level structure for type:"progress" JSONL entries
// emitted by Claude Code. These appear in the parent's JSONL file for both
// self-progress (hook/mcp/bash progress) and subagent progress (Agent tool
// invocations). Subagent entries have data.type=="agent_progress" and
// toolUseID != parentToolUseID. Self-progress entries have
// toolUseID == parentToolUseID.
type progressEntry struct {
	Type            string          `json:"type"`
	ToolUseID       string          `json:"toolUseID"`
	ParentToolUseID string          `json:"parentToolUseID"`
	SessionID       string          `json:"sessionId"`
	Slug            string          `json:"slug"`
	Timestamp       string          `json:"timestamp"`
	Data            json.RawMessage `json:"data"`
}

// progressDataHeader is used for fast pre-parsing of data.type to distinguish
// agent_progress (subagent) from hook_progress/mcp_progress (self-progress).
type progressDataHeader struct {
	Type string `json:"type"`
}

// progressData wraps the nested data.message structure inside a progress entry.
type progressData struct {
	Message struct {
		Type    string          `json:"type"` // "assistant" or "user"
		Message json.RawMessage `json:"message"`
	} `json:"message"`
}

// SubagentParseResult accumulates parsed data for a single subagent across
// all its progress entries. Keyed by toolUseID in ParseResult.Subagents.
type SubagentParseResult struct {
	ID              string
	ParentToolUseID string
	Slug            string
	Model           string
	LatestUsage     *TokenUsage
	MessageCount    int
	ToolCalls       int
	LastTool        string
	LastActivity    string
	FirstTime       time.Time
	LastTime        time.Time
	Completed       bool
}

// maxLastTextLen caps the text stored in LastAssistantText to avoid
// bloating session state with large message bodies.
const maxLastTextLen = 500

type ParseResult struct {
	SessionID         string
	Slug              string // Internal session name (e.g. "mighty-cuddling-castle")
	Model             string
	LatestUsage       *TokenUsage
	MessageCount      int
	ToolCalls         int
	LastTool          string
	LastActivity      string
	LastTime          time.Time
	WorkingDir        string
	Subagents         map[string]*SubagentParseResult // keyed by toolUseID
	CompactionCount   int                             // number of compact_boundary events in this chunk
	LastAssistantText string                          // last text content block from an assistant message
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

// ParseSessionJSONL incrementally parses a Claude JSONL session file from
// the given byte offset. knownSlug is the session's slug from a previous
// parse batch — it seeds the result so incremental batches can filter
// self-progress even when no non-progress entries appear. knownParents
// maps parentToolUseID → toolUseID for subagents already tracked in the
// session state, enabling cross-batch completion detection when a
// tool_result arrives in a batch with no new progress entries. Pass ""
// and nil when no prior state exists.
func ParseSessionJSONL(path string, offset int64, knownSlug string, knownParents map[string]string) (*ParseResult, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer func() { _ = f.Close() }()

	// Check file size before parsing to avoid OOM on huge files.
	info, err := f.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() > maxFileSize {
		log.Printf("[jsonl] Skipping %s: file size %d exceeds limit %d", path, info.Size(), maxFileSize)
		return nil, offset, fmt.Errorf("file size %d exceeds max %d", info.Size(), maxFileSize)
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, offset, err
		}
	}

	result := &ParseResult{
		Slug:      knownSlug,
		Subagents: make(map[string]*SubagentParseResult),
	}
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
		if line[len(line)-1] != '\n' {
			// Line is incomplete - don't parse or advance offset
			if err == io.EOF {
				break
			}
			continue
		}

		// Skip oversized lines to prevent excessive memory use during JSON parsing.
		if len(line) > maxLineLength {
			log.Printf("[jsonl] Skipping oversized line (%d bytes) in %s at offset %d",
				len(line), path, parsedOffset)
			parsedOffset += int64(len(line))
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

		// Capture session slug only from non-progress entries. Progress
		// entries carry subagent slugs, not the session's own slug.
		if entry.Slug != "" && result.Slug == "" && entry.Type != "progress" {
			result.Slug = entry.Slug
		}

		if entry.Cwd != "" {
			result.WorkingDir = entry.Cwd
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
			checkSubagentCompletion(entry.Message, result, knownParents)

		case "progress":
			parseProgressEntry(lineData, result)

		case "system":
			if entry.Subtype == "compact_boundary" {
				result.CompactionCount++
			}
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

	// Parse content blocks for tool use and text content.
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		switch block.Type {
		case "tool_use":
			result.ToolCalls++
			result.LastTool = block.Name
			result.LastActivity = "tool_use"
		case "text":
			if block.Text != "" {
				t := block.Text
				if len(t) > maxLastTextLen {
					t = t[:maxLastTextLen]
				}
				result.LastAssistantText = t
			}
		}
	}
}

// parseProgressEntry handles a type:"progress" JSONL line, accumulating
// subagent state into result.Subagents keyed by toolUseID.
func parseProgressEntry(line []byte, result *ParseResult) {
	var entry progressEntry
	if err := json.Unmarshal(line, &entry); err != nil || entry.ToolUseID == "" {
		return
	}

	// Determine if this is a subagent entry by checking data.type.
	// agent_progress entries are always subagent progress — they carry the
	// parent session's slug (or no slug), so slug-based filtering is wrong.
	isAgent := false
	if entry.Data != nil {
		var header progressDataHeader
		if json.Unmarshal(entry.Data, &header) == nil {
			isAgent = header.Type == "agent_progress"
		}
	}

	// Self-progress filter: skip non-agent entries whose slug matches the
	// session slug. Agent entries are never self-progress.
	if !isAgent {
		if entry.Slug != "" && result.Slug != "" && entry.Slug == result.Slug {
			return
		}
	}

	sub, exists := result.Subagents[entry.ToolUseID]
	if !exists {
		// For agent entries, always create a subagent even without a slug.
		// For non-agent entries, require a slug to avoid creating phantoms.
		if !isAgent && entry.Slug == "" {
			return
		}
		sub = &SubagentParseResult{
			ID:              entry.ToolUseID,
			ParentToolUseID: entry.ParentToolUseID,
		}
		result.Subagents[entry.ToolUseID] = sub
	}

	if entry.Slug != "" {
		sub.Slug = entry.Slug
	}

	if entry.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
			if sub.FirstTime.IsZero() {
				sub.FirstTime = t
			}
			sub.LastTime = t
		}
	}

	// Parse nested data.message.message for model, usage, content blocks.
	if entry.Data == nil {
		return
	}
	var pd progressData
	if err := json.Unmarshal(entry.Data, &pd); err != nil {
		return
	}

	switch pd.Message.Type {
	case "assistant":
		sub.MessageCount++
		if pd.Message.Message != nil {
			parseSubagentAssistantMessage(pd.Message.Message, sub)
		}
		// parseSubagentAssistantMessage may have set "tool_use"; only
		// downgrade to "thinking" if it did not.
		if sub.LastActivity != "tool_use" {
			sub.LastActivity = "thinking"
		}
	case "user":
		sub.MessageCount++
		sub.LastActivity = "waiting"
	}
}

// parseSubagentAssistantMessage extracts model, usage, and tool calls from
// a subagent's assistant message (the inner data.message.message object).
func parseSubagentAssistantMessage(raw json.RawMessage, sub *SubagentParseResult) {
	var msg messageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	if msg.Model != "" {
		sub.Model = msg.Model
	}
	if msg.Usage != nil {
		sub.LatestUsage = msg.Usage
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		if block.Type == "tool_use" {
			sub.ToolCalls++
			sub.LastTool = block.Name
			sub.LastActivity = "tool_use"
		}
	}
}

// checkSubagentCompletion scans a user message's content blocks for
// tool_result entries whose tool_use_id matches a known subagent's
// parentToolUseID, marking that subagent as completed. knownParents
// provides parentToolUseID → toolUseID mappings from prior batches,
// enabling cross-batch completion detection.
func checkSubagentCompletion(raw json.RawMessage, result *ParseResult, knownParents map[string]string) {
	if raw == nil {
		return
	}

	// Build a lookup: parentToolUseID → subagent toolUseID
	parentToSub := make(map[string]string, len(result.Subagents)+len(knownParents))
	for id, sub := range result.Subagents {
		if sub.ParentToolUseID != "" {
			parentToSub[sub.ParentToolUseID] = id
		}
	}
	// Merge known parents from prior batches (current batch takes precedence).
	for parentID, subID := range knownParents {
		if _, exists := parentToSub[parentID]; !exists {
			parentToSub[parentID] = subID
		}
	}
	if len(parentToSub) == 0 {
		return
	}

	var msg messageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return
	}

	for _, block := range blocks {
		if block.Type == "tool_result" && block.ToolUseID != "" {
			if subID, ok := parentToSub[block.ToolUseID]; ok {
				if sub, exists := result.Subagents[subID]; exists {
					sub.Completed = true
				} else {
					// Cross-batch: subagent not in current parse results.
					// Create a minimal entry to signal completion upstream.
					result.Subagents[subID] = &SubagentParseResult{
						ID:              subID,
						ParentToolUseID: block.ToolUseID,
						Completed:       true,
					}
				}
			}
		}
	}
}

func encodeProjectPath(path string) string {
	// Claude Code encodes project paths by replacing / with -.
	// Leading / is also replaced, so /home/user/proj becomes -home-user-proj.
	return strings.ReplaceAll(filepath.Clean(path), "/", "-")
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

// DecodeProjectPath reverses the encoding to get the original working dir.
// The encoding is lossy: slash-to-dash means /home/my-user/proj and
// /home/my/user-proj both encode to -home-my-user-proj. This function
// tries all possible interpretations recursively and returns the first
// filesystem-verified match. Falls back to all-dashes-as-slashes.
func DecodeProjectPath(encoded string) string {
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		decoded = encoded
	}

	if !strings.HasPrefix(decoded, "-") {
		return decoded
	}

	parts := strings.Split(decoded[1:], "-") // skip leading dash

	if result := decodeTryPaths(parts); result != "" {
		return result
	}

	// Fallback: treat all dashes as slashes (best effort).
	return "/" + strings.Join(parts, "/")
}

// decodeTryPaths iteratively builds candidate paths by choosing slash or
// hyphen at each boundary between parts. Candidates are kept in the same
// slash-first order as the prior recursive search, but we prune any branch
// whose fixed parent path cannot exist and cap the frontier size.
func decodeTryPaths(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	candidates := []decodePathState{{components: []string{parts[0]}}}
	parentExistsCache := make(map[string]bool)
	pathExistsCache := make(map[string]bool)

	for idx := 1; idx < len(parts); idx++ {
		nextCandidates := make([]decodePathState, 0, len(candidates)*2)
		seen := make(map[string]struct{}, len(candidates)*2)

		for i := 0; i < len(candidates); i++ {
			slashCandidate := candidates[i].withSlash(parts[idx])
			if decodePathParentExists(slashCandidate.components, parentExistsCache) {
				path := slashCandidate.path()
				if _, ok := seen[path]; !ok {
					nextCandidates = append(nextCandidates, slashCandidate)
					seen[path] = struct{}{}
				}
			}

			hyphenCandidate := candidates[i].withHyphen(parts[idx])
			if decodePathParentExists(hyphenCandidate.components, parentExistsCache) {
				path := hyphenCandidate.path()
				if _, ok := seen[path]; !ok {
					nextCandidates = append(nextCandidates, hyphenCandidate)
					seen[path] = struct{}{}
				}
			}

			if len(nextCandidates) > maxDecodePathCandidates {
				log.Printf("[jsonl] Aborting ambiguous path decode after %d candidates for %q",
					len(nextCandidates), "/"+strings.Join(parts, "-"))
				return ""
			}
		}

		if len(nextCandidates) == 0 {
			return ""
		}
		candidates = nextCandidates
	}

	for i := 0; i < len(candidates); i++ {
		path := candidates[i].path()
		if decodePathExists(path, pathExistsCache) {
			return path
		}
	}

	return ""
}

type decodePathState struct {
	components []string
}

func (s decodePathState) withSlash(part string) decodePathState {
	next := make([]string, len(s.components)+1)
	copy(next, s.components)
	next[len(s.components)] = part
	return decodePathState{components: next}
}

func (s decodePathState) withHyphen(part string) decodePathState {
	next := make([]string, len(s.components))
	copy(next, s.components)
	next[len(next)-1] = next[len(next)-1] + "-" + part
	return decodePathState{components: next}
}

func (s decodePathState) path() string {
	return "/" + strings.Join(s.components, "/")
}

func decodePathParentExists(components []string, cache map[string]bool) bool {
	if len(components) <= 1 {
		return true
	}

	parent := "/" + strings.Join(components[:len(components)-1], "/")
	if exists, ok := cache[parent]; ok {
		return exists
	}

	_, err := os.Stat(parent)
	exists := err == nil
	cache[parent] = exists
	return exists
}

func decodePathExists(path string, cache map[string]bool) bool {
	if exists, ok := cache[path]; ok {
		return exists
	}

	_, err := os.Stat(path)
	exists := err == nil
	cache[path] = exists
	return exists
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
