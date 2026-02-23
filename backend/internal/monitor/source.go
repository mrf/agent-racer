package monitor

import "time"

// Source defines the interface for an agent session provider (e.g. Claude,
// Codex, Gemini). Each implementation knows how to discover active sessions
// on disk and incrementally parse their log files into a common format the
// monitor can consume.
//
// Implementations should be safe to call from a single goroutine (the
// monitor poll loop). They do not need to be safe for concurrent use.
type Source interface {
	// Name returns a short lowercase identifier for this agent source,
	// e.g. "claude", "codex", "gemini". Used as part of composite
	// session keys and surfaced to the frontend for display.
	Name() string

	// Discover finds sessions that are currently active (or recently
	// active) on the local machine. The returned handles uniquely
	// identify each session and carry enough context for subsequent
	// Parse calls.
	//
	// Discover is called on every poll tick. Implementations should be
	// efficient -- typically a directory listing with a recency filter.
	Discover() ([]SessionHandle, error)

	// Parse reads new data from a session log starting at the given byte
	// offset. It returns the parsed incremental results, the new byte
	// offset to use on the next call, and any error encountered.
	//
	// If there is no new data since offset, implementations should return
	// a zero-value SourceUpdate, the same offset, and nil error.
	//
	// The monitor calls Parse once per tracked session per poll tick.
	Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error)
}

// SessionHandle identifies a single agent session discovered by a Source.
// The monitor uses these as keys to track sessions and pass back into
// Source.Parse on subsequent polls.
type SessionHandle struct {
	// SessionID is a unique identifier for this session. For Claude this
	// is derived from the JSONL filename; other sources may use their
	// own ID scheme.
	SessionID string

	// LogPath is the absolute path to the session's log file on disk.
	// This is the file that Parse reads incrementally.
	LogPath string

	// WorkingDir is the project directory the agent session is operating
	// in, if known. May be empty if the source cannot determine it
	// during discovery; in that case Parse may populate it later via
	// SourceUpdate.WorkingDir.
	WorkingDir string

	// Source is the lowercase name of the agent source that produced
	// this handle (matches Source.Name()).
	Source string

	// StartedAt is the session start time if the source can determine
	// it during discovery (e.g. from file creation time). Zero value
	// means unknown.
	StartedAt time.Time

	// KnownSlug is the session's slug from a previous parse batch.
	// Populated by the monitor before each Parse call so that
	// incremental batches (which may contain only progress entries)
	// can still filter self-progress by slug.
	KnownSlug string

	// KnownSubagentParents maps parentToolUseID → toolUseID for
	// subagents already tracked in the session state. Populated by the
	// monitor before each Parse call to enable cross-batch completion
	// detection. Nil when no subagents are known.
	KnownSubagentParents map[string]string
}

// SourceUpdate contains the incremental data parsed from a session log
// since the last offset. All fields are additive/latest-wins: the monitor
// merges these into the cumulative SessionState.
type SourceUpdate struct {
	// SessionID from the log itself, if present. May differ from or
	// confirm the handle's SessionID. Empty means no ID was found in
	// this chunk.
	SessionID string

	// Slug is the internal session name (e.g. "mighty-cuddling-castle").
	// Extracted from the JSONL slug field. Empty means not yet seen.
	Slug string

	// Model is the model identifier seen in the latest parsed entries
	// (e.g. "claude-opus-4-5-20251101"). Empty means no model info was
	// found in this chunk.
	Model string

	// TokensIn is the total input/context token count from the most
	// recent usage record in this chunk. Zero means no usage data. This
	// represents the latest snapshot, not a delta.
	TokensIn int

	// TokensOut is the output token count from the most recent usage
	// record. Zero means no usage data.
	TokensOut int

	// MessageCount is the number of new messages (user + assistant)
	// found in this chunk. This is a delta to be added to the
	// cumulative count.
	MessageCount int

	// ToolCalls is the number of new tool invocations found in this
	// chunk. This is a delta to be added to the cumulative count.
	ToolCalls int

	// LastTool is the name of the most recently invoked tool in this
	// chunk (e.g. "Read", "Bash"). Empty if no tool calls were found.
	LastTool string

	// Activity is a normalized activity classification for the most
	// recent log entry: "thinking", "tool_use", "waiting", or empty
	// if no entries were parsed.
	Activity string

	// LastTime is the timestamp of the most recent log entry parsed.
	// Zero value means no timestamped entries were found.
	LastTime time.Time

	// WorkingDir may be set if the source discovers the working
	// directory from log contents rather than from discovery. Empty
	// means no new information.
	WorkingDir string

	// Branch is the git branch name for the session's working
	// directory, if detectable. Empty means unknown.
	Branch string

	// MaxContextTokens is the model's context window size if the
	// source can determine it from session data (e.g. a model metadata
	// field, API lookup, or configuration). Zero means unknown -- the
	// monitor will fall back to the static config.yaml lookup.
	// When non-zero, the monitor prefers this over the config value.
	MaxContextTokens int

	// Subagents contains parsed state for subagents (Task tool
	// invocations) discovered in this chunk. Keyed by toolUseID.
	// Only populated by sources that support subagent tracking
	// (currently Claude only).
	Subagents map[string]*SubagentParseResult
}

// HasData reports whether this update contains any meaningful data
// (i.e., at least one field was populated by parsing).
func (u SourceUpdate) HasData() bool {
	return u.SessionID != "" ||
		u.Model != "" ||
		u.TokensIn > 0 ||
		u.TokensOut > 0 ||
		u.MessageCount > 0 ||
		u.ToolCalls > 0 ||
		u.LastTool != "" ||
		u.Activity != "" ||
		!u.LastTime.IsZero() ||
		u.WorkingDir != "" ||
		u.Branch != "" ||
		u.MaxContextTokens > 0 ||
		len(u.Subagents) > 0
}
