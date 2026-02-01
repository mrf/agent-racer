# Codex CLI (OpenAI) Session/Log Storage Research

Research for agent-racer integration. Codex CLI is OpenAI's open-source coding
agent that runs in the terminal, built in Rust. Repository:
https://github.com/openai/codex

---

## 1. Local File Paths and Directory Structure

### CODEX_HOME

All Codex CLI local state lives under `CODEX_HOME`, which defaults to `~/.codex/`.
The environment variable `CODEX_HOME` can override this (e.g.,
`export CODEX_HOME="$XDG_CONFIG_HOME/codex"`).

**Codex does NOT follow XDG Base Directory spec.** There are open feature requests
(GitHub issues #1980, #4407) asking for XDG compliance, but as of January 2026
this has not been implemented. Everything goes into `~/.codex/`.

### Directory Layout

```
~/.codex/
  config.toml                          # User configuration
  AGENTS.md                            # Global agent instructions
  AGENTS.override.md                   # Temporary global override
  history.jsonl                        # Prompt/session history index
  log/
    codex-tui.log                      # TUI diagnostic log (INFO/DEBUG/ERROR/WARN)
  sessions/
    YYYY/MM/DD/
      rollout-{timestamp}-{uuid}.jsonl # Per-session conversation logs
  archived_sessions/                   # Archived/old sessions (moved here)
  skills/
    **/SKILL.md                        # User-defined skills
```

### Key Files for agent-racer

| File | Purpose | Relevance |
|------|---------|-----------|
| `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | Full per-session conversation logs | **Primary data source** -- contains all events, tool calls, token usage |
| `~/.codex/history.jsonl` | Prompt history index with session references | **Secondary** -- maps session IDs to working directories and prompts |
| `~/.codex/log/codex-tui.log` | TUI diagnostic log | Low -- debugging only, not structured for consumption |

---

## 2. Log Format

### Session Rollout Files (Primary)

**Format:** JSONL (JSON Lines), one JSON object per line.

**Location:** `~/.codex/sessions/YYYY/MM/DD/rollout-{timestamp}-{uuid}.jsonl`

**Structure:**
- **Line 1:** `SessionMeta` header -- contains session ID, source, timestamp, model provider
- **Line 2+:** `RolloutItem` events -- user turns, agent responses, tool calls, token counts, etc.

**Schema evolution note:** PR #3380 ("Introduce rollout items") changed the format.
Older files (pre-September 2025) contain bare `SessionMeta`/`ResponseItem` JSON.
Newer files use a `RolloutLine` envelope with `type`/`payload` fields. Codex
includes fallback parsing for the old format but the distinction matters for any
parser we build.

### history.jsonl (Index)

**Format:** JSONL, one entry per prompt/session interaction.

**Location:** `~/.codex/history.jsonl`

This file functions like shell command history for the TUI. It records each prompt
with metadata. Can grow very large (known issue #4963). Codex uses advisory file
locking for concurrent access.

### codex-tui.log (Diagnostic)

**Format:** Plain text with timestamps and log levels (`[INFO]`, `[DEBUG]`, etc.)

**Location:** `~/.codex/log/codex-tui.log`

Not useful for agent-racer -- this is application-level debug logging.

---

## 3. Schema and Available Fields

### SessionMeta (Rollout Header, Line 1)

Based on source analysis and documentation, the metadata header contains:

| Field | Type | Description |
|-------|------|-------------|
| `session_id` / `conversation_id` | UUID (string) | Unique session identifier |
| `source` | string | Where session was started (CLI, VS Code extension, etc.) |
| `timestamp` | ISO 8601 string | Session start time |
| `model_provider` | string | Provider ID (e.g., "openai") |
| `model` | string | Model used (e.g., "gpt-5-codex", "codex-mini-latest") |

### EnvironmentContext (Early in Rollout)

When a new session starts (`InitialHistory::New`), context items are persisted:

| Field | Description |
|-------|-------------|
| `cwd` (working directory) | The directory where Codex was launched |
| `approval_policy` | User's approval mode |
| `sandbox_policy` | Sandbox configuration |
| `user_shell` | User's shell |

### RolloutItem Events (Line 2+)

The `RolloutItem` enum defines five types of persistable items. The main event
categories observed in rollout files:

**event_msg variants:**

| `payload.type` | Description |
|-----------------|-------------|
| `user_message` | User input text |
| `agent_message` | Agent response text |
| `token_count` | Cumulative token usage snapshot |
| `session_configured` | Session configuration event (model, reasoning effort, rollout path) |

**response_item variants:**

| Type | Description |
|------|-------------|
| `message` | AI response message |
| `command_execution` | Sandboxed command with command, cwd, exit code, duration |
| `file_change` | File modifications |
| `reasoning` | Model reasoning/thinking content |
| `web_search` | Web search actions |
| `mcp_tool_call` | MCP tool invocations |
| `plan_update` | Plan/step updates |

### Token Count Fields

Each `token_count` event reports **cumulative** totals (delta must be computed
by subtracting previous values):

| Field | Description |
|-------|-------------|
| `input_tokens` | Total input tokens consumed |
| `cached_input_tokens` | Input tokens served from cache |
| `output_tokens` | Total output tokens generated |
| `reasoning_output_tokens` | Reasoning tokens (subset of output, not double-counted) |
| `total_tokens` | Sum of input + output (recomputed if absent) |

**Important:** Token count events were only added in commit `0269096` (2025-09-06).
Earlier sessions have no token metrics. Sessions from early September 2025 may
lack `turn_context` model metadata.

### Non-Interactive (exec --json) Event Schema

When running `codex exec --json`, a different JSONL stream is emitted to stdout:

```jsonl
{"type":"thread.started","thread_id":"0199a213-81c0-7800-8aa1-bbab2a035a53"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Done."}}
{"type":"turn.completed","usage":{"input_tokens":24763,"cached_input_tokens":24448,"output_tokens":122}}
```

Event types: `thread.started`, `turn.started`, `turn.completed`, `turn.failed`,
`item.started`, `item.updated`, `item.completed`, `error`.

---

## 4. Session Identification

### Session ID Format

UUIDs, likely v7 (time-ordered). Examples from documentation and issues:

- `0199e96c-7d0c-7403-bf30-395693cd1788`
- `0199a213-81c0-7800-8aa1-bbab2a035a53`
- `019b63ce-02a4-7223-8d1d-f5dc6a1b4877`

### Extracting Session ID

The session ID can be obtained from:

1. **Filename:** `rollout-{timestamp}-{SESSION_ID}.jsonl` -- parse the UUID from the filename
2. **SessionMeta header:** First line of the rollout JSONL file
3. **history.jsonl:** Entries reference session IDs
4. **CLI output:** Codex displays session ID on exit (Ctrl+C)
5. **`/status` command:** Shows active session info within the TUI

### Active vs Completed Session Detection

**There is no documented lock file or PID-based mechanism** for detecting whether
a session is currently active. This is a known gap.

Possible detection strategies for agent-racer:

| Strategy | Feasibility | Notes |
|----------|-------------|-------|
| File modification time | Good | Active sessions append to their rollout file; watch mtime |
| File size growth | Good | Active sessions grow as events are appended |
| `codex-tui.log` activity | Medium | Log activity correlates with active sessions |
| Process detection | Medium | Check for running `codex` processes with `ps`/`pgrep` |
| Advisory lock on history.jsonl | Low | Codex uses advisory locking, but this is internal |
| Tail the rollout file | Good | Watch for new JSONL lines being appended |

**Recommended approach for agent-racer:** Use inotify/fswatch on the
`~/.codex/sessions/` directory tree to detect new rollout files appearing, then
tail active files for real-time events. A session is likely "completed" when
no new lines have been appended for a configurable timeout period (similar to
how we handle Claude Code sessions).

---

## 5. Mapping Sessions to Working Directories

### From Rollout Files

The working directory (cwd) is stored in the `EnvironmentContext` item near the
start of each rollout file. When `InitialHistory::New` creates a session, it
persists the current working directory as part of the initial context.

### From history.jsonl

The `history.jsonl` file contains `cwd` fields that map prompts/sessions to
their working directories. Third-party tools like `codex-history-list` display
columns: `time | cwd | ask | path`.

### From the Resume Picker

The `list_conversations` function filters sessions by current working directory
when `--all` is not specified. This confirms that cwd is stored in the session
metadata and is queryable.

### Practical Approach for agent-racer

1. Scan `~/.codex/sessions/YYYY/MM/DD/` for rollout files
2. Parse the first few lines of each file to extract `SessionMeta` and `EnvironmentContext`
3. Match `cwd` against known project directories to associate sessions with racing lanes
4. For real-time monitoring, watch for new files and tail them

---

## 6. Comparison with Claude Code

| Aspect | Claude Code | Codex CLI |
|--------|-------------|-----------|
| Home directory | `~/.claude/` | `~/.codex/` |
| XDG compliance | No | No (open requests) |
| Session log format | JSONL | JSONL |
| Session log location | `~/.claude/projects/.../*.jsonl` | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |
| Session ID format | UUID | UUID (likely v7) |
| Working directory in logs | Yes (project path in directory structure) | Yes (EnvironmentContext in rollout file) |
| Token tracking | Yes | Yes (since Sept 2025) |
| Model identification | Yes | Yes (turn_context / SessionMeta) |
| Tool call logging | Yes | Yes (command_execution, mcp_tool_call, etc.) |
| Active session detection | File watching / process check | File watching / process check |
| Session resume | `claude -c -r` | `codex resume [session-id]` |

---

## 7. Unknowns and Gaps Requiring Hands-On Testing

### Must Verify Locally

1. **Exact SessionMeta JSON schema** -- The precise field names in the first line
   of a rollout file need to be verified by examining an actual file. Documentation
   references `session_id`, `conversation_id`, `source`, `timestamp`, and
   `model_provider`, but the exact keys and nesting are not formally documented.

2. **EnvironmentContext position and format** -- Where exactly in the rollout file
   the working directory appears (line 2? embedded in a specific event type?)
   needs hands-on verification.

3. **history.jsonl entry schema** -- The exact fields per line in history.jsonl
   are not formally documented. Need to inspect an actual file to confirm
   `session_id`, `cwd`, `prompt`, `model`, and `timestamp` fields.

4. **RolloutLine envelope format** -- Post-PR#3380 files use a `RolloutLine`
   wrapper with `type`/`payload`. The exact JSON structure needs verification:
   ```json
   {"type": "event_msg", "payload": {"type": "user_message", ...}}
   ```
   vs.
   ```json
   {"event_msg": {"type": "user_message", ...}}
   ```

5. **Filename timestamp format** -- The exact format of `{timestamp}` in
   `rollout-{timestamp}-{uuid}.jsonl` filenames (epoch millis? ISO 8601 compact?)
   needs verification.

6. **Active session signals** -- Whether there is any file-level indicator
   (lock file, .active marker, incomplete final line) that distinguishes an
   active session from a completed one.

7. **VS Code extension behavior** -- The Codex VS Code extension writes to the
   same `~/.codex/sessions/` directory. Need to verify whether its rollout files
   are distinguishable (different `source` field?).

8. **Session end markers** -- Whether rollout files contain an explicit
   "session ended" event or if completion is only inferrable from lack of new writes.

### Lower Priority

9. **Archived sessions** -- How and when sessions are moved to `archived_sessions/`.
10. **Advisory locking details** -- Whether the lock on history.jsonl could be
    used to detect active Codex processes.
11. **config.toml session settings** -- Whether `history.persistence = "none"`
    also suppresses rollout file creation or only affects history.jsonl.

---

## 8. Implementation Recommendations for agent-racer

### Monitoring Strategy

The Codex session monitor should follow a similar pattern to the existing Claude
Code monitor:

1. **Discovery:** Watch `~/.codex/sessions/` (respecting `CODEX_HOME` env var)
   for new `rollout-*.jsonl` files using filesystem events (inotify on Linux,
   fsnotify on macOS).

2. **Parsing:** Read the first few lines of each new rollout file to extract
   SessionMeta (session ID, model, provider) and EnvironmentContext (working
   directory).

3. **Real-time tailing:** Tail active rollout files for new JSONL lines to
   capture events as they happen (tool calls, token usage, agent messages).

4. **Session lifecycle:** Mark a session as "completed" after a configurable
   inactivity timeout (no new lines appended).

5. **Token tracking:** Parse `token_count` events to track cumulative and
   per-turn usage, subtracting previous values to get deltas.

### Parser Considerations

- Must handle both old format (bare `SessionMeta`/`ResponseItem` JSON) and
  new format (`RolloutLine` envelope with `type`/`payload`)
- Token count events report cumulative values, so delta calculation is required
- Sessions lacking `turn_context` metadata (early Sept 2025 builds) may need
  a model fallback

---

## Sources

- [OpenAI Codex CLI GitHub Repository](https://github.com/openai/codex)
- [Codex CLI Reference](https://developers.openai.com/codex/cli/reference/)
- [Codex CLI Features](https://developers.openai.com/codex/cli/features/)
- [Codex Configuration Reference](https://developers.openai.com/codex/config-reference/)
- [Codex Advanced Configuration](https://developers.openai.com/codex/config-advanced/)
- [Codex Non-Interactive Mode](https://developers.openai.com/codex/noninteractive/)
- [DeepWiki: Conversation History and Persistence](https://deepwiki.com/openai/codex/3.3-session-management-and-persistence)
- [DeepWiki: Session Resumption](https://deepwiki.com/openai/codex/4.4-session-resumption)
- [GitHub Issue #1980: XDG Base Directory Specification](https://github.com/openai/codex/issues/1980)
- [GitHub Issue #4407: Change hardcoded ~/.codex path](https://github.com/openai/codex/issues/4407)
- [GitHub Issue #4963: Log rotate history.jsonl](https://github.com/openai/codex/issues/4963)
- [GitHub PR #3380: Introduce rollout items](https://github.com/openai/codex/pull/3380)
- [GitHub PR #5658: Filter by model_provider](https://github.com/openai/codex/pull/5658)
- [ccusage Codex CLI Overview](https://ccusage.com/guide/codex/)
