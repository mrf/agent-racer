# Gemini CLI Session/Log Storage Research

Research for agent-racer Gemini source implementation (issue agent-racer-7jo).

## 1. Local Storage Paths

### Primary Data Directory

Gemini CLI stores all local data under `~/.gemini/`. This is a custom path and does
**not** follow the XDG Base Directory specification (no use of `~/.config/gemini/`,
`~/.local/state/gemini/`, etc.).

### Directory Layout

```
~/.gemini/
  settings.json              # User configuration
  GEMINI.md                  # Global instruction file (like CLAUDE.md)
  tmp/
    <project_hash>/          # Per-project directory (SHA-256 of project path)
      chats/                 # Auto-saved session files
        session-<timestamp>-<short_id>.json
      logs.json              # User-typed command log (no model responses)
      shell_history          # Shell command history
      checkpoint-*.json      # Explicit /chat save snapshots
      otel/
        collector.log        # OpenTelemetry collector output (when enabled)
  telemetry.log              # Local telemetry output (when configured)
```

### Project Hash Mechanism

The `<project_hash>` is a **SHA-256 hash** of the absolute project directory path.
This provides a deterministic, collision-resistant mapping from any directory to a
unique folder name.

Example: if you run `gemini` from `/home/mrf/Projects/agent-racer`, the project
data lands in:
```
~/.gemini/tmp/4ebf39d8665452a73d0abdd7819b076253edbc27dce22d93f0e25ebf0862a44f/
```

**Critical limitation**: SHA-256 is a one-way hash. There is no built-in way to
reverse the hash back to the original directory path. This was raised in
[Discussion #2664](https://github.com/google-gemini/gemini-cli/discussions/2664).

### Comparison with Claude Code

| Aspect | Claude Code | Gemini CLI |
|--------|-------------|------------|
| Base dir | `~/.claude/` | `~/.gemini/` |
| Project mapping | Path encoding (`/` -> `-`) | SHA-256 hash of path |
| Session dir | `~/.claude/projects/<encoded-path>/` | `~/.gemini/tmp/<hash>/chats/` |
| XDG compliant | No | No |
| Reversible project ID | Yes (decode `-` back to `/`) | **No** (one-way hash) |

## 2. Log/Session File Format

### Current Format: JSON (single file per session)

Session files are stored as **complete JSON files** (not JSONL). Each session is a
single `.json` file in the `chats/` directory. The `ChatRecordingService` in
`packages/core/src/services/chatRecordingService.ts` manages session persistence.

**On every new message or update, the entire session file is rewritten.** This is
an O(N) operation relative to conversation length. For long sessions, files can grow
to hundreds of megabytes, with rewrites taking seconds.

### Session Filename Convention

```
session-<YYYY-MM-DD>T<HH-MM>-<short_hex_id>.json
```

Examples:
- `session-2025-09-18T02-45-3b44bc68.json`
- `session-2025-12-21T13-43-c27248ed.json`

The short hex ID appears to be an 8-character hex string (possibly truncated UUID).

### Planned Migration: JSONL (Issue #15292)

There is an [open proposal](https://github.com/google-gemini/gemini-cli/issues/15292)
to switch session storage from JSON to JSONL. The proposed record types:

```jsonl
{"type":"session_metadata","sessionId":"...","projectHash":"...","startTime":"..."}
{"type":"user","id":"msg1","content":[{"text":"Hello"}]}
{"type":"gemini","id":"msg2","content":[{"text":"Hi"}]}
{"type":"message_update","id":"msg2","tokens":{"input":10,"output":5}}
```

Performance benchmarks from the proposal: JSON rewrite ~6.8s/msg vs JSONL append
~0.75ms/msg for large sessions (>9000x improvement). New sessions would use `.jsonl`
extension with backward compatibility for reading old `.json` files.

**Impact on agent-racer**: If/when this ships, the Gemini source parser will need to
handle both `.json` (legacy) and `.jsonl` (new) formats.

### Additional File: logs.json

The `logs.json` file at the project-hash level records everything the user typed
across all sessions, but does **not** contain model responses. This file is less
useful for our monitoring purposes than the individual session files.

## 3. Schema / Available Fields

### Session File Structure (Current JSON)

Based on source code analysis and documentation, each session JSON file contains a
conversation array with message records. Each message includes:

| Field | Type | Description |
|-------|------|-------------|
| `type` / `role` | string | `"user"` or `"model"` (Gemini API convention) |
| `content` | object | Message content with `parts` array |
| `content.parts[].text` | string | Text content |
| `content.parts[].thought` | string | Reasoning/thinking content (when available) |
| `usageMetadata` | object | Token usage per response |
| `usageMetadata.promptTokenCount` | integer | Input/prompt tokens |
| `usageMetadata.candidatesTokenCount` | integer | Output/response tokens |
| `usageMetadata.totalTokenCount` | integer | Sum of prompt + candidate |

### Tool Call Representation

Tool usage appears in the content parts as function calls (Gemini API format):
- Tool calls show as `functionCall` parts with tool name and parameters
- Tool results show as `functionResponse` parts with output

### Headless Mode JSON Output (--output-format json)

When run with `--output-format json`, the CLI produces a structured result object:

```json
{
  "session_id": "abc123",
  "response": "...",
  "stats": {
    "models": {
      "<model-name>": {
        "prompt": 100,
        "candidates": 200,
        "total": 300,
        "cached": 50,
        "thoughts": 30,
        "tool": 20
      }
    },
    "tools": { ... },
    "files": { "linesAdded": 10, "linesRemoved": 5 }
  },
  "error": null
}
```

### Stream-JSON Output (--output-format stream-json)

JSONL event stream with these event types:

| Event Type | Key Fields | Description |
|-----------|------------|-------------|
| `init` | `session_id`, `model`, `timestamp` | Session start |
| `message` | `role`, `content`, `delta`, `timestamp` | User or assistant message |
| `tool_use` | `tool_name`, `tool_id`, `parameters`, `timestamp` | Tool invocation |
| `tool_result` | `tool_id`, `status`, `output`, `timestamp` | Tool result |
| `error` | error details | Non-fatal errors |
| `result` | `status`, `stats` (tokens, duration, tool_calls), `timestamp` | Session end |

Example stream:
```jsonl
{"type":"init","timestamp":"2025-10-10T12:00:00.000Z","session_id":"abc123","model":"gemini-2.0-flash-exp"}
{"type":"message","role":"user","content":"List files in current directory","timestamp":"..."}
{"type":"tool_use","tool_name":"Bash","tool_id":"bash-123","parameters":{"command":"ls -la"},"timestamp":"..."}
{"type":"tool_result","tool_id":"bash-123","status":"success","output":"file1.txt\nfile2.txt","timestamp":"..."}
{"type":"message","role":"assistant","content":"Here are the files...","delta":true,"timestamp":"..."}
{"type":"result","status":"success","stats":{"total_tokens":250,"input_tokens":50,"output_tokens":200,"duration_ms":3000,"tool_calls":1},"timestamp":"..."}
```

### OpenTelemetry Metrics (when enabled)

Telemetry data uses OpenTelemetry standard with these key attributes:
- `session.id` - Session identifier
- `installation.id` - CLI installation identifier
- `gen_ai.client.token.usage` - Token usage histogram
- `gemini_cli.tool_call` - Tool call events

## 4. Session Identification

### Session ID Format

Session IDs appear to use a short hex format (8 characters, e.g., `3b44bc68`,
`c27248ed`), embedded in the filename:
```
session-2025-09-18T02-45-3b44bc68.json
                          ^^^^^^^^ session ID
```

For the `--resume` flag, the CLI accepts:
- Integer index (e.g., `--resume 5`)
- Full UUID (e.g., `--resume a1b2c3d4-e5f6-7890-abcd-ef1234567890`)
- `latest` keyword

This suggests internal session IDs may be full UUIDs, with the filename using a
truncated form.

### Active vs Completed Session Detection

Gemini CLI does **not** provide a built-in mechanism to distinguish active from
completed sessions. The challenges:

1. **No explicit session-end marker**: Unlike Claude Code's JSONL which can be
   detected via process monitoring, Gemini session files are just JSON files that
   get rewritten. There is no append-only log to watch for new entries.

2. **File modification time is the primary signal**: An actively running session
   will have its JSON file rewritten frequently. A completed session will stop
   being modified.

3. **No lock files observed**: No documentation mentions `.lock` files or similar
   mechanisms.

4. **Process detection approach**: The most reliable method for detecting active
   Gemini sessions is to look for running `gemini` processes via `/proc` (similar
   to current Claude process detection), then correlate with session files via
   the working directory -> SHA-256 hash mapping.

### Listing Sessions

The CLI provides `--list-sessions` which outputs:
```
Available sessions for this project (3):
1. Fix bug in auth (2 days ago) [a1b2c3d4]
2. Refactor database schema (5 hours ago) [e5f67890]
3. Update documentation (Just now) [abcd1234]
```

This is interactive output, not useful for programmatic consumption. The
`--output-format json` flag may make this machine-readable, but this needs
hands-on testing.

## 5. Mapping Sessions to Working Directories

### The Core Challenge

Because the project hash is a **one-way SHA-256**, you cannot simply look at a
session file path and determine what working directory it belongs to. This is the
biggest architectural difference from Claude Code, where the path encoding is
reversible.

### Approaches for agent-racer

**Option A: Process-based discovery (recommended for active sessions)**

1. Scan `/proc` for running `gemini` processes
2. Read `/proc/<pid>/cwd` to get the working directory
3. Compute `SHA-256(cwd)` to find the matching `~/.gemini/tmp/<hash>/` directory
4. Read session files from `<hash>/chats/`

This mirrors the existing Claude Code approach but requires computing the hash
to locate files.

**Option B: Brute-force directory scan**

1. List all directories under `~/.gemini/tmp/`
2. For each, check `chats/` for recently-modified session files
3. Working directory is unknown unless we maintain a hash-to-path lookup table

**Option C: Build a hash lookup table**

1. On first discovery, scan `/proc` for gemini processes to learn `hash -> cwd` mappings
2. Cache these mappings persistently (e.g., in agent-racer state)
3. Use the cache for subsequent lookups
4. This handles both active and recently-completed sessions

**Option D: Telemetry-based**

1. Configure Gemini CLI to write telemetry locally via settings.json:
   ```json
   {
     "telemetry": {
       "enabled": true,
       "target": "local",
       "outfile": ".gemini/telemetry.log"
     }
   }
   ```
2. Parse telemetry logs for session IDs, token usage, etc.
3. Downside: requires user to enable telemetry; not a default behavior

**Option E: stream-json sidecar (most data-rich, requires wrapper)**

If users run gemini with `--output-format stream-json`, the output can be piped
to a file. This provides the richest real-time data (init event has session_id
and model, result event has full token stats). However, this requires wrapping
the gemini command, which is invasive.

### Recommended Implementation Strategy

Combine Options A and B:
1. **Active sessions**: Use process scanning to discover running gemini processes
   and their CWDs. Compute SHA-256 to find session files.
2. **Session files**: For all project-hash dirs under `~/.gemini/tmp/`, check
   for recently-modified session files. Parse them for data even if the
   working directory is unknown (it can be populated later when the process
   is seen, or left empty).
3. **Maintain a hash map**: Persist discovered `sha256(path) -> path` mappings
   so that even after a process exits, we know which directory a hash
   corresponds to.

## 6. Mapping to agent-racer Source Interface

The existing `Source` interface requires:

| Source Method | Gemini Feasibility | Notes |
|--------------|-------------------|-------|
| `Name()` | "gemini" | Straightforward |
| `Discover()` | Medium difficulty | Process scan + hash lookup needed |
| `Parse()` | Moderate difficulty | Must parse full JSON (not incremental JSONL) |

### Parse Challenges

The current session format (full JSON rewrite) makes incremental parsing
problematic. Unlike Claude Code's JSONL where you can seek to a byte offset
and read new lines, Gemini's JSON files are rewritten entirely on each update.

Approaches:
1. **Re-parse entire file each poll**: Simple but wasteful for large sessions.
   Use file modification time to skip unchanged files.
2. **Track file size/mtime**: Only re-parse when the file changes.
3. **Wait for JSONL migration**: If issue #15292 lands, the format becomes
   much more amenable to incremental parsing.

### SourceUpdate Field Mapping

| SourceUpdate Field | Gemini Session Data | Source |
|-------------------|--------------------|----|
| `SessionID` | Filename short hex ID or internal UUID | Filename parse |
| `Model` | Not in session file by default | Process args, settings.json, or telemetry |
| `TokensIn` | `usageMetadata.promptTokenCount` | Session JSON parse |
| `TokensOut` | `usageMetadata.candidatesTokenCount` | Session JSON parse |
| `MessageCount` | Count of message records | Session JSON parse |
| `ToolCalls` | Count of `functionCall` parts | Session JSON parse |
| `LastTool` | Name from last `functionCall` | Session JSON parse |
| `Activity` | Infer from last message type | Session JSON parse |
| `LastTime` | File modification time (no per-message timestamps in stored JSON) | File stat |
| `WorkingDir` | From process CWD or hash lookup table | Process scan |

## 7. Unknowns and Gaps (Requires Hands-On Testing)

### Must Verify

1. **Exact JSON schema of session files**: The internal structure of
   `chats/session-*.json` files needs to be examined with a real Gemini CLI
   installation. Documentation describes the API response format, but the
   on-disk session format may differ (ChatRecordingService may transform it).

2. **Model name in session data**: It is unclear whether the model name
   (e.g., `gemini-2.0-flash`) is stored inside the session JSON or only
   available from CLI args / settings.json.

3. **Per-message timestamps**: Session files may or may not include
   timestamps on individual messages. The stream-json format does, but the
   stored session format is unclear.

4. **Session end signal**: How reliably can we detect that a session file is
   "done" vs "in progress"? Need to test whether the file remains open/locked
   during an active session.

5. **JSONL migration status**: Issue #15292 may have landed already. Need to
   check if current versions use `.jsonl` for new sessions.

6. **Process executable name**: The exact process name for gemini CLI when
   running (is it `gemini`, `node` running a script, or something else?)
   needs to be verified for process scanning.

7. **Session ID in JSON content**: Whether the session ID / UUID is stored
   inside the JSON file itself or only in the filename.

8. **Hash algorithm details**: Need to verify it is specifically SHA-256 of
   the raw path string (vs normalized path, or path with trailing slash, etc.).

### Nice to Verify

9. **Working directory in session data**: Whether the project path or CWD
   appears anywhere inside the session JSON.

10. **Token counts per-turn vs cumulative**: Whether `usageMetadata` in the
    session file shows per-turn counts or cumulative totals.

11. **Tool names**: What tool names Gemini CLI uses (e.g., `run_shell_command`,
    `read_file`, etc.) and how they map to agent-racer's tool display.

12. **Concurrent session behavior**: What happens when multiple gemini
    sessions run in the same project directory simultaneously.

## 8. Sources

- [Gemini CLI GitHub Repository](https://github.com/google-gemini/gemini-cli)
- [Session Management Documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/session-management.md)
- [Configuration Documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/get-started/configuration.md)
- [Headless Mode Documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md)
- [Telemetry / Observability Documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/telemetry.md)
- [Issue #15292: Switch to JSONL](https://github.com/google-gemini/gemini-cli/issues/15292)
- [Issue #8944: Retrieve session_id](https://github.com/google-gemini/gemini-cli/issues/8944)
- [Issue #8203: stream-json format](https://github.com/google-gemini/gemini-cli/issues/8203)
- [PR #14504: session_id in JSON output](https://github.com/google-gemini/gemini-cli/pull/14504)
- [Discussion #2664: Temporary files directory hashing](https://github.com/google-gemini/gemini-cli/discussions/2664)
- [ChatRecordingService source](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/services/chatRecordingService.ts)
