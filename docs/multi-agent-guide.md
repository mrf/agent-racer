# Multi-Agent Guide

Agent Racer monitors AI coding agent sessions and visualizes them as cars racing on a track. It supports multiple agent CLIs through a unified `Source` interface.

## Supported CLIs

| CLI | Status | Default | Session Logs | Token Data |
|-----|--------|---------|--------------|------------|
| **Claude Code** | Stable | Enabled | `~/.claude/projects/<encoded-path>/*.jsonl` | Real usage from API responses |
| **OpenAI Codex CLI** | Pre-alpha | Disabled | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | Cumulative snapshots via `token_count` events |
| **Google Gemini CLI** | Pre-alpha | Disabled | `~/.gemini/tmp/<sha256-hash>/chats/session-*.json` | Per-message `tokens` or `usageMetadata` fields |

### Claude Code

The primary and most mature source. Claude Code writes append-only JSONL session logs organized by project directory. Agent Racer reads these incrementally (seeking to the last byte offset each poll) for efficient real-time monitoring.

- **Session discovery**: Scans `~/.claude/projects/*/` for recently-modified `.jsonl` files.
- **Working directory**: Decoded from the encoded project path in the file path (e.g., `-home-user-project` decodes to `/home/user/project`).
- **Session lifecycle**: Supports Claude Code's `SessionEnd` hook for immediate completion detection. Falls back to inactivity timeout.
- **Token tracking**: Uses real `input_tokens + cache_read + cache_creation` from API responses. Always accurate.

### OpenAI Codex CLI

Codex CLI stores session logs as JSONL rollout files, organized by date. Like Claude, these are append-only and support incremental parsing.

- **Session discovery**: Walks `~/.codex/sessions/YYYY/MM/DD/` for `rollout-*.jsonl` files. Respects the `CODEX_HOME` environment variable.
- **Working directory**: Extracted from `env_context` or `turn_context` entries in the rollout file (contains a `cwd` field).
- **Session ID**: Derived from the UUID portion of the rollout filename.
- **Log format note**: The parser handles both the older bare-JSON format and the newer `RolloutLine` envelope format (`type`/`payload` wrapper introduced in PR #3380).
- **Token tracking**: Cumulative `token_count` events with `input_tokens` and `output_tokens`. Codex can also report `model_context_window` dynamically.

### Google Gemini CLI

Gemini CLI uses a different storage model: complete JSON files (not JSONL) that are rewritten on every update. This means the parser re-reads the entire file when it detects a modification.

- **Session discovery**: Scans `~/.gemini/tmp/<hash>/chats/` for `session-*.json` files. The `<hash>` is a SHA-256 of the project directory path.
- **Working directory**: Cannot be derived from the hash (one-way). Agent Racer scans running `gemini` processes to build a hash-to-path lookup table.
- **Session ID**: Short hex ID from the session filename.
- **Token tracking**: Uses `tokens.input`/`tokens.output` (CLI format) or `usageMetadata.promptTokenCount`/`usageMetadata.candidatesTokenCount` (API format) from model response messages.
- **Context window**: Hardcoded per model family (1M tokens for all current Gemini 2.x models).

## Configuration

### Enabling Sources

All source activation is controlled in `config.yaml` (default: `~/.config/agent-racer/config.yaml`):

```yaml
sources:
  claude: true    # Enabled by default
  codex: false    # Opt-in (pre-alpha)
  gemini: false   # Opt-in (pre-alpha)
```

### Model Context Limits

The `models` section maps model identifiers to their context window sizes. These are used as fallbacks when a source doesn't report the context window dynamically:

```yaml
models:
  # Claude
  claude-opus-4-5-20251101: 200000
  claude-sonnet-4-5-20250929: 200000
  claude-haiku-3-5-20241022: 200000
  # Codex (fallback; Codex can report this dynamically)
  gpt-5-codex: 272000
  # Gemini (fallback; hardcoded in source)
  gemini-2.5-pro: 1048576
  default: 200000
```

### Token Normalization

Controls how context utilization is derived per source:

```yaml
token_normalization:
  strategies:
    claude: usage       # Real token counts from API
    codex: usage        # Cumulative token_count events
    gemini: usage       # Token data from session messages
    default: estimate   # Fallback: estimate from message count
  tokens_per_message: 2000  # Used by estimate/message_count strategies
```

Strategies:
- **`usage`**: Use real token counts reported by the source. Falls back to estimation when no data is available yet.
- **`estimate`** / **`message_count`**: Always derive tokens from `message_count * tokens_per_message`. The `tokenEstimated` flag is set on the session so frontends can indicate the value is approximate.

### Privacy Controls

Privacy settings apply uniformly across all sources:

```yaml
privacy:
  mask_working_dirs: false    # Show only last path component
  mask_session_ids: false     # Replace with opaque hashes
  mask_pids: false            # Hide process IDs
  mask_tmux_targets: false    # Hide tmux pane info
  allowed_paths: []           # Allowlist (empty = all)
  blocked_paths: []           # Denylist
```

### Monitor Tuning

```yaml
monitor:
  poll_interval: 1s              # Source discovery + parse frequency
  session_stale_after: 2m        # Inactivity before marking lost
  completion_remove_after: 8s    # Display time after completion animation
  churning_cpu_threshold: 15.0   # CPU% for detecting active processing
```

## Architecture: Adding a New Source

Agent Racer's backend is designed to be **UI-agnostic**. The Go service produces a stream of `SessionState` objects over WebSocket -- any frontend can consume them. The `Source` interface is the only extension point needed to add a new agent CLI.

### The Source Interface

Defined in `backend/internal/monitor/source.go`:

```go
type Source interface {
    Name() string
    Discover() ([]SessionHandle, error)
    Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error)
}
```

- **`Name()`** returns a short lowercase identifier (e.g., `"claude"`, `"codex"`, `"gemini"`). Used as part of composite session keys and surfaced to the frontend.
- **`Discover()`** finds currently active sessions. Called every poll tick. Should be efficient (directory listing with recency filter).
- **`Parse()`** reads new data from a session log starting at a byte offset. Returns a `SourceUpdate` with normalized fields and the new offset.

### SessionHandle

Carries identity and location for a discovered session:

```go
type SessionHandle struct {
    SessionID  string
    LogPath    string
    WorkingDir string
    Source     string
    StartedAt  time.Time
}
```

### SourceUpdate

The normalized output from parsing. All fields are latest-wins or additive deltas:

```go
type SourceUpdate struct {
    SessionID        string    // From log content (confirms handle)
    Model            string    // e.g., "claude-opus-4-5-20251101"
    TokensIn         int       // Input/context token snapshot
    TokensOut        int       // Output token snapshot
    MessageCount     int       // Delta: new messages in this chunk
    ToolCalls        int       // Delta: new tool invocations
    LastTool         string    // Most recent tool name
    Activity         string    // "thinking", "tool_use", "waiting"
    LastTime         time.Time // Timestamp of latest entry
    WorkingDir       string    // If discovered from log content
    Branch           string    // Git branch if detectable
    MaxContextTokens int       // Source-reported context ceiling
}
```

### Step-by-Step: Adding a New Source

1. **Create `backend/internal/monitor/<name>_source.go`**. Implement the `Source` interface. Follow the existing pattern:
   - Constructor takes a `discoverWindow time.Duration` for recency filtering.
   - `Discover()` scans the CLI's session directory and returns handles for recently-modified files.
   - `Parse()` reads from the log file at the given offset and returns a `SourceUpdate`.

2. **Add config toggle**. In `backend/internal/config/config.go`, add a field to the `Sources` struct:
   ```go
   type Sources struct {
       Claude bool `yaml:"claude"`
       Codex  bool `yaml:"codex"`
       Gemini bool `yaml:"gemini"`
       NewCLI bool `yaml:"newcli"`  // Add here
   }
   ```

3. **Wire into main.go**. In `backend/cmd/server/main.go`, add the source conditionally:
   ```go
   if cfg.Sources.NewCLI {
       sources = append(sources, monitor.NewNewCLISource(discoverWindow))
   }
   ```

4. **Add model entries to config.yaml** if the CLI uses models with different context windows.

5. **Put shared parsing logic in `monitor/jsonl.go`** if the new source uses JSONL. Avoid duplicating parsing between sources.

### Design Rules

- **No rendering hints in backend structs.** The frontend derives visuals from `SessionState` fields (activity, source, model), not from source-specific metadata.
- **No frontend branching on source name.** All sources produce the same `SessionState` shape. The frontend should not have `if source === "codex"` blocks.
- **Shared parsing goes in `jsonl.go`.** Source-specific parsing stays in the source file, but common JSONL line reading and JSON unmarshalling belong in the shared parser.
- **Config-driven activation.** Every source must be toggleable via `config.yaml`. Pre-alpha sources default to disabled.

## Using the Backend with Alternative UIs

The Go backend is a standalone HTTP/WebSocket server. It exposes:

### WebSocket: `/ws`

Real-time session state stream. Message types:

| Type | Description | Payload |
|------|-------------|---------|
| `snapshot` | Full state of all sessions | `{ sessions: SessionState[] }` |
| `delta` | Changed sessions only | `{ updates: SessionState[], removed: string[] }` |
| `completion` | Session finished | `{ sessionId, activity, name }` |

Snapshots are sent on connect and every `snapshot_interval` (default 5s). Deltas are throttled to `broadcast_throttle` (default 100ms).

### REST: `GET /api/sessions`

Returns a JSON array of all current `SessionState` objects. Suitable for polling-based UIs or dashboards.

### REST: `GET /api/config`

Returns the server's sound configuration. Used by the default frontend to sync audio settings.

### SessionState Fields

Every session (regardless of source) produces the same state object:

```json
{
  "id": "claude:abc-123",
  "name": "my-project",
  "source": "claude",
  "model": "claude-opus-4-5-20251101",
  "activity": "thinking",
  "workingDir": "/home/user/my-project",
  "branch": "main",
  "tokensUsed": 142000,
  "maxContextTokens": 200000,
  "contextUtilization": 0.71,
  "tokenEstimated": false,
  "messageCount": 42,
  "toolCallCount": 18,
  "currentTool": "Read",
  "isChurning": true,
  "burnRatePerMinute": 8500.0,
  "pid": 12345,
  "tmuxTarget": "%5",
  "startedAt": "2026-01-30T10:00:00Z",
  "lastActivityAt": "2026-01-30T10:05:00Z",
  "lastDataReceivedAt": "2026-01-30T10:05:00Z",
  "completedAt": null,
  "lane": 0
}
```

An alternative UI only needs to connect to `/ws` and render sessions. The `contextUtilization` field (0.0-1.0) directly maps to "race progress."

## Manual Validation Checklist

Use this checklist to validate that each source is working correctly. Run one session of each CLI and verify each item.

### Claude Code Session

```
Prerequisites:
  [ ] Claude Code CLI installed and authenticated
  [ ] agent-racer running with sources.claude: true (default)

Start a session:
  [ ] Run `claude` in a project directory
  [ ] Session appears on the dashboard within 2 seconds

Activity tracking:
  [ ] Car shows "thinking" animation during model responses
  [ ] Car shows tool name + sparks during tool use (Read, Bash, etc.)
  [ ] Car shows hazard lights when waiting for user input
  [ ] Car moves to pit lane during idle periods

Token tracking:
  [ ] Token counter updates after each API response
  [ ] Car position advances as context fills up
  [ ] tokenEstimated is false (real usage data)

Session metadata:
  [ ] Session name matches project directory name
  [ ] Model badge shows correct model family
  [ ] Working directory shown in detail panel
  [ ] Git branch shown (if applicable)
  [ ] PID populated

Session lifecycle:
  [ ] Completion: exit session normally -> trophy + confetti
  [ ] Error: trigger an error -> spin-out animation
  [ ] Stale: leave idle for >2 minutes -> car fades (lost)
  [ ] SessionEnd hook: if installed, session completes immediately on exit
```

### OpenAI Codex CLI Session

```
Prerequisites:
  [ ] Codex CLI installed and authenticated
  [ ] agent-racer config: sources.codex: true

Start a session:
  [ ] Run `codex` in a project directory
  [ ] Session appears on the dashboard

Activity tracking:
  [ ] Car shows activity changes as Codex processes
  [ ] Tool use detected for command execution, file changes
  [ ] Tool names display (Bash, FileEdit, etc.)

Token tracking:
  [ ] Token counter updates (from token_count events)
  [ ] Context window may be reported dynamically (check maxContextTokens)
  [ ] If no token data yet, verify estimation fallback works

Session metadata:
  [ ] Source shows as "codex" in detail panel
  [ ] Working directory populated (from env_context/turn_context in rollout)
  [ ] Model name populated (from session_meta or turn_context)

Session lifecycle:
  [ ] Session completes after inactivity timeout
  [ ] Completion animation plays
  [ ] Session removed after completion_remove_after
```

### Google Gemini CLI Session

```
Prerequisites:
  [ ] Gemini CLI installed and authenticated
  [ ] agent-racer config: sources.gemini: true

Start a session:
  [ ] Run `gemini` in a project directory
  [ ] Session appears on the dashboard

Activity tracking:
  [ ] Car shows activity changes as Gemini processes
  [ ] Tool use detected for function calls
  [ ] Thinking detected when model includes thoughts

Token tracking:
  [ ] Token counter updates (from tokens or usageMetadata fields)
  [ ] Context window shows ~1M for Gemini 2.x models

Session metadata:
  [ ] Source shows as "gemini" in detail panel
  [ ] Working directory populated (requires running gemini process for hash lookup)
  [ ] Model name populated (from session data or process detection)

Known limitations:
  [ ] Working directory may be blank if gemini process exited before hash mapping
  [ ] Full JSON re-parse on each poll (performance may degrade for very long sessions)
  [ ] No session-end marker -- relies on inactivity timeout

Session lifecycle:
  [ ] Session completes after inactivity timeout
  [ ] Completion animation plays
```

### Cross-Source Validation

```
Run all three CLIs simultaneously:
  [ ] All sessions appear on the dashboard
  [ ] Each session has correct source identifier
  [ ] Sessions get unique lanes on the track
  [ ] No session ID collisions (composite keys: source:sessionID)
  [ ] Completion of one does not affect others
  [ ] Detail panel shows correct per-session data
  [ ] WebSocket snapshot includes all sessions
```
